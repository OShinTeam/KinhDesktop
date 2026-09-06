package service

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kinh-desktop/global"
)

// ==================== 文件下载地址获取与解析（参考 KinhWebEO handler/down.go） ====================
//
// 解析流程（本地模式）：
//  1. filemetas 接口（dlink=1）拿原始下载地址 odlink
//  2. 拼 rand 签名参数（wenku vipinfo 取 uid → SHA1 签名，参考 KinhWebEO utils/baidu.go Getrand）
//  3. 先尝试加速 Host 替换后的地址，请求取 Location 作为最终直链
//  4. 失败则回退原始 odlink 重试一次
//
// 解析流程（远程模式，设置中配置了加速链接时可用）：
//  1. POST 加速链接（acclink），携带 fid 与 vip 参数，Cookie 用 BDUSS+STOKEN
//  2. 响应 JSON 中直接返回 dlink（参考 KinhWebEO handler/down.go downRemote）
//
// TODO(下载功能实现时)：直链交给 OShinD 引擎执行多线程下载；
// 下载前可按文件大小与用户设置（线程数/目录）组装下载任务

// dlinkExtraParams 直链附加参数（与 KinhWebEO 保持一致）
const dlinkExtraParams = "&channel=0&version=8.4.0.103&"

// BaiduDownloadLinkResult 下载地址解析结果（返回给前端）
type BaiduDownloadLinkResult struct {
	Success  bool   `json:"success"`
	FsID     int64  `json:"fs_id"`
	Filename string `json:"filename"`
	Dlink    string `json:"dlink"`
	Message  string `json:"message,omitempty"`
}

type baiduFilemetasResponse struct {
	Errno int                  `json:"errno"`
	Info  []baiduFileMetaDlink `json:"info"`
}

type baiduFileMetaDlink struct {
	FsID           int64  `json:"fs_id"`
	ServerFilename string `json:"server_filename"`
	Filename       string `json:"filename"`
	Dlink          string `json:"dlink"`
}

// GetBaiduDownloadLink 本地解析文件下载直链（filemetas + rand 签名 + Location 跟踪）
func (a *App) GetBaiduDownloadLink(fsID int64) *BaiduDownloadLinkResult {
	return resolveWithRetry(fsID, fetchDownloadLinkOnce)
}

// GetBaiduDownloadLinkRemote 远程解析文件下载直链（加速链接，需在设置中配置）
// 未配置加速链接时返回失败；对齐 KinhWebEO handler/down.go downRemote
func (a *App) GetBaiduDownloadLinkRemote(fsID int64) *BaiduDownloadLinkResult {
	acclink := strings.TrimSpace(getSettings().DownloadAccLink)
	if acclink == "" {
		return &BaiduDownloadLinkResult{
			Success: false,
			FsID:    fsID,
			Message: "未配置加速链接，请先在设置中填写",
		}
	}
	return resolveWithRetry(fsID, func(fsID int64, bduss, stoken, ua string) (*BaiduDownloadLinkResult, int) {
		return fetchDownloadLinkRemoteOnce(fsID, bduss, stoken, acclink)
	})
}

// resolveWithRetry 解析执行器：STOKEN 缺失时先刷新，凭证失效类错误码自动刷新重试一次
func resolveWithRetry(fsID int64, once func(fsID int64, bduss, stoken, ua string) (*BaiduDownloadLinkResult, int)) *BaiduDownloadLinkResult {
	// 一次性取值拷贝，避免并发登出导致的空指针
	credential := currentCredentialSnapshot()
	if credential == nil || credential.BDUSS == "" {
		return &BaiduDownloadLinkResult{Success: false, FsID: fsID, Message: "百度网盘账号未登录"}
	}

	stoken := credential.SToken
	if stoken == "" {
		stoken = refreshStoken(credential.BDUSS, credential.PToken)
		if stoken != "" {
			updateCurrentCredentialStoken(stoken)
		}
	}

	// 请求使用的 UA 取下载设置中的默认 UA
	ua := getSettings().DownloadUserAgent

	result, errno := once(fsID, credential.BDUSS, stoken, ua)

	// 凭证失效特征：刷新一次 STOKEN 重试；仍失败则按正常错误返回
	if errno != 0 && isCredentialErrno(errno) {
		global.Log.Infof("下载地址解析返回 errno=%d，尝试刷新 STOKEN 后重试", errno)
		newStoken := refreshStoken(credential.BDUSS, credential.PToken)
		if newStoken != "" && newStoken != stoken {
			updateCurrentCredentialStoken(newStoken)
			updateSavedCredentialStoken(newStoken)
			result, _ = once(fsID, credential.BDUSS, newStoken, ua)
		}
	}
	return result
}

// fetchDownloadLinkOnce 使用指定 STOKEN 解析一次下载直链
// 返回结果与百度接口 errno（成功为 0），errno 供调用方判断凭证失效重试
func fetchDownloadLinkOnce(fsID int64, bduss, stoken, ua string) (*BaiduDownloadLinkResult, int) {
	result := &BaiduDownloadLinkResult{Success: false, FsID: fsID}

	cookie := "BDUSS=" + bduss + ";PANPSC=;BAIDUID=1;ndut_fmt=" + getndut() + ";STOKEN=" + stoken
	apiURL := "https://pan.baidu.com/api/filemetas?dlink=1&clienttype=8&rt=third&fsids=[%22" + strconv.FormatInt(fsID, 10) + "%22]"

	resp, err := baiduGetWithResponse(apiURL, "netdisk;Mo", cookie)
	if err != nil {
		global.Log.Errorf("请求 filemetas 接口失败: %v", err)
		result.Message = "获取下载地址失败: 网络请求异常"
		return result, -1
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Message = "获取下载地址失败: 读取响应失败"
		return result, -1
	}

	var meta baiduFilemetasResponse
	if err := json.Unmarshal(body, &meta); err != nil {
		global.Log.Errorf("解析 filemetas 响应失败: %v", err)
		result.Message = "获取下载地址失败: 响应解析异常"
		return result, -1
	}
	if meta.Errno != 0 {
		global.Log.Warnf("filemetas 返回 errno=%d (fs_id=%d)", meta.Errno, fsID)
		result.Message = fmt.Sprintf("获取下载地址失败: 百度接口返回错误 %d", meta.Errno)
		return result, meta.Errno
	}
	if len(meta.Info) == 0 {
		result.Message = "获取下载地址失败: 文件信息为空"
		return result, meta.Errno
	}

	info := meta.Info[0]
	odlink := info.Dlink
	if odlink == "" {
		result.Message = "获取下载地址失败: 未返回下载地址"
		return result, meta.Errno
	}
	result.Filename = info.ServerFilename
	if result.Filename == "" {
		result.Filename = info.Filename
	}

	// 拼签名参数后解析直链：先试加速地址，失败回退原始 odlink
	rand := getrand(bduss)
	if dlink := resolveDlink(acceleratedDlink(odlink)+dlinkExtraParams+rand, ua); dlink != "" {
		result.Success = true
		result.Dlink = dlink
		return result, 0
	}
	if dlink := resolveDlink(odlink+dlinkExtraParams+rand, ua); dlink != "" {
		result.Success = true
		result.Dlink = dlink
		return result, 0
	}

	global.Log.Warnf("下载直链解析失败 (fs_id=%d)", fsID)
	result.Message = "获取下载地址失败: 直链解析失败"
	return result, 99
}

// acceleratedDlink 生成加速地址：替换 PCS Host 并降级为 HTTP（参考 KinhWebEO down.go）
func acceleratedDlink(odlink string) string {
	replaced := strings.Replace(odlink, "d.pcs.baidu.com", "218.93.204.36/b/d.pcs.baidu.com", -1)
	return strings.Replace(replaced, "https", "http", 1)
}

// fetchDownloadLinkRemoteOnce 远程解析：POST 加速链接，响应 JSON 直接携带 dlink
// 对齐 KinhWebEO handler/down.go downRemote：fid + vip 参数，Cookie 用 BDUSS+STOKEN
func fetchDownloadLinkRemoteOnce(fsID int64, bduss, stoken, acclink string) (*BaiduDownloadLinkResult, int) {
	result := &BaiduDownloadLinkResult{Success: false, FsID: fsID}

	// vip 参数：vip_type 1(VIP)/2(SVIP) 均视为会员（远程服务用于选择解析通道）
	vip := "0"
	if credential := currentCredentialSnapshot(); credential != nil && credential.VipType > 0 {
		vip = "2"
	}

	req, err := http.NewRequest(http.MethodPost, acclink, strings.NewReader(
		"fid="+strconv.FormatInt(fsID, 10)+"&vip="+vip,
	))
	if err != nil {
		global.Log.Errorf("构造加速链接请求失败: %v", err)
		result.Message = "获取下载地址失败: 网络请求异常"
		return result, -1
	}
	req.Header.Set("User-Agent", getSettings().DownloadUserAgent)
	req.Header.Set("Cookie", "BDUSS="+bduss+";STOKEN="+stoken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := baiduNoRedirectClient.Do(req)
	if err != nil {
		global.Log.Errorf("请求加速链接失败: %v", err)
		result.Message = "获取下载地址失败: 网络请求异常"
		return result, -1
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Message = "获取下载地址失败: 读取响应失败"
		return result, -1
	}
	global.Log.Infof("加速链接返回: %s", string(body))

	var remote struct {
		Errno int    `json:"errno"`
		Dlink string `json:"dlink"`
	}
	if err := json.Unmarshal(body, &remote); err != nil {
		global.Log.Errorf("解析加速链接响应失败: %v", err)
		result.Message = "获取下载地址失败: 响应解析异常"
		return result, -1
	}
	if remote.Errno != 0 {
		global.Log.Warnf("加速链接返回 errno=%d (fs_id=%d)", remote.Errno, fsID)
		result.Message = fmt.Sprintf("获取下载地址失败: 加速服务返回错误 %d", remote.Errno)
		return result, remote.Errno
	}
	if remote.Dlink == "" {
		result.Message = "获取下载地址失败: 未返回下载地址"
		return result, remote.Errno
	}

	result.Success = true
	result.Dlink = remote.Dlink
	return result, 0
}

// resolveDlink 请求下载地址（不跟随重定向），取 Location 作为最终直链；失败返回空串
func resolveDlink(dl, ua string) string {
	req, err := http.NewRequest(http.MethodGet, dl, nil)
	if err != nil {
		global.Log.Errorf("构造直链请求失败: %v", err)
		return ""
	}
	req.Header.Set("User-Agent", ua)
	resp, err := baiduNoRedirectClient.Do(req)
	if err != nil {
		global.Log.Warnf("直链请求失败: %v", err)
		return ""
	}
	defer resp.Body.Close()
	return resp.Header.Get("Location")
}

// getrand 生成下载地址附加签名参数（rand/rand2/devuid/time）
// 算法与 KinhWebEO utils/baidu.go Getrand 一致：wenku vipinfo 取 uid，BDUSS 做 SHA1 链式签名
func getrand(bduss string) string {
	now := strconv.FormatInt(time.Now().Unix(), 10)
	fallback := "rand=0000&rand2=0000&devuid=114514&time=" + now

	resp, err := baiduGetWithResponse("https://wenku.baidu.com/customer/interface/vipinfo", "netdisk;11.0.0", "BDUSS="+bduss)
	if err != nil {
		global.Log.Warnf("获取 vipinfo 失败，使用降级 rand 参数: %v", err)
		return fallback
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fallback
	}

	var vipInfo struct {
		Data struct {
			UID int64 `json:"uid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &vipInfo); err != nil || vipInfo.Data.UID == 0 {
		return fallback
	}

	uid := strconv.FormatInt(vipInfo.Data.UID, 10)
	devUIDSum := sha1.Sum([]byte(bduss))
	devUID := hex.EncodeToString(devUIDSum[:])
	bdussSum := sha1.Sum([]byte(bduss))
	randSum := sha1.Sum([]byte(hex.EncodeToString(bdussSum[:]) + uid + "ebrcUYiuxaZv2XGu7KIYKxUrqfnOfpDF" + now + devUID))
	rand := hex.EncodeToString(randSum[:])
	return "rand=" + rand + "&rand2=" + rand + "&devuid=" + devUID + "&time=" + now
}
