package service

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"kinh-desktop/global"
)

// ==================== 百度网盘账号登录（扫码参考 BaiduQRlogin，换 STOKEN 参考 KinhWebEO utils/baidu.go） ====================
//
// 网盘 API 需要 BDUSS + STOKEN 两个 Cookie：
//   - 扫码登录: qrbdusslogin 直接返回 BDUSS/PTOKEN/STOKEN
//   - Cookie 登录: 用户填写 BDUSS + PTOKEN，通过 plantcookie 换取 STOKEN
//   - STOKEN 可能过期，验证接口失败后自动刷新一次 STOKEN 并重试

var (
	// baiduLongPollClient 用于扫码 unicast 长轮询等普通 GET 请求（二维码图片下载、qrbdusslogin 复用此 client）
	baiduLongPollClient = &http.Client{
		Timeout: 35 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: false,
		},
	}
	// 换 STOKEN / 登录验证需要读取 302 Location 与 Set-Cookie，禁止自动跟随重定向
	baiduNoRedirectClient = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: false,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

// currentBaiduCredential 保存最近一次登录成功后的凭证（Cookie 登录 / 扫码登录均写入）
var currentBaiduCredential *BaiduLoginResult

type BaiduQRCode struct {
	QrBase64 string `json:"qr_base64"` // 二维码图片 Data URL（image/png;base64）
	Sign     string `json:"sign"`      // 扫码轮询凭证
	Prompt   string `json:"prompt"`
}

type BaiduPollResult struct {
	Status string `json:"status"` // waiting | scanned | success | error
	V      string `json:"v"`      // 登录凭证 v（status == success 时有效）
}

type BaiduLoginResult struct {
	Success  bool   `json:"success"`
	Username string `json:"username"`
	UserId   string `json:"user_id"`
	BDUSS    string `json:"bduss"`
	PToken   string `json:"ptoken"`
	SToken   string `json:"stoken"`
	BdStoken string `json:"bd_stoken"` // 网盘操作用的 bdstoken
	VipType  int    `json:"vip_type"`  // 0 无会员 / 1 VIP / 2 SVIP
	PhotoUrl string `json:"photo_url"` // 头像
	Message  string `json:"message"`
}

type baiduQRResp struct {
	Imgurl string `json:"imgurl"`
	Errno  int    `json:"errno"`
	Sign   string `json:"sign"`
	Prompt string `json:"prompt"`
}

type baiduPollResp struct {
	Errno     int    `json:"errno"`
	ChannelID string `json:"channel_id"`
	ChannelV  string `json:"channel_v"`
}

type baiduChannelV struct {
	Status int    `json:"status"`
	V      string `json:"v"`
}

type baiduLoginAPIResult struct {
	ErrInfo struct {
		No  string `json:"no"`
		Msg string `json:"msg"`
	} `json:"errInfo"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Session struct {
			BDUSS  string `json:"bduss"`
			PToken string `json:"ptoken"`
			SToken string `json:"stoken"`
		} `json:"session"`
		User struct {
			Username    string `json:"username"`
			UserID      string `json:"userId"`
			DisplayName string `json:"displayName"`
		} `json:"user"`
	} `json:"data"`
}

type baiduLoginStatus struct {
	Errno     int    `json:"errno"`
	ShowMsg   string `json:"show_msg"`
	LoginInfo struct {
		Username string `json:"username"`
		BdStoken string `json:"bdstoken"`
		UkStr    string `json:"uk_str"`
		PhotoUrl string `json:"photo_url"`
		VipType  string `json:"vip_type"`
	} `json:"login_info"`
}

// ==================== HTTP 基础 ====================

func baiduGet(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://passport.baidu.com/")
	resp, err := baiduLongPollClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func baiduGetWithResponse(url, ua, cookie string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Cookie", cookie)
	return baiduNoRedirectClient.Do(req)
}

// ==================== 扫码登录 ====================

// GetBaiduQR 获取百度登录二维码，返回 base64 图片与轮询凭证
func (a *App) GetBaiduQR() (*BaiduQRCode, error) {
	bs, err := baiduGet("https://passport.baidu.com/v2/api/getqrcode?lp=pc&qrloginfrom=pc")
	if err != nil {
		global.Log.Errorf("获取百度二维码失败: %v", err)
		return nil, err
	}

	var resp baiduQRResp
	if err := json.Unmarshal(bs, &resp); err != nil {
		return nil, fmt.Errorf("解析二维码响应失败: %w", err)
	}
	if resp.Errno != 0 || resp.Imgurl == "" {
		return nil, fmt.Errorf("获取二维码失败, errno: %d", resp.Errno)
	}

	imgData, err := baiduGet("https://" + strings.TrimPrefix(resp.Imgurl, "https://"))
	if err != nil {
		global.Log.Errorf("下载二维码图片失败: %v", err)
		return nil, fmt.Errorf("下载二维码图片失败: %w", err)
	}

	global.Log.Info("获取百度登录二维码成功")
	return &BaiduQRCode{
		QrBase64: "data:image/png;base64," + base64.StdEncoding.EncodeToString(imgData),
		Sign:     resp.Sign,
		Prompt:   resp.Prompt,
	}, nil
}

// PollBaiduQR 执行一次扫码状态轮询（前端循环调用，每次为一次长轮询请求）
// status: waiting | scanned | success | error | network_error
// network_error 表示网络异常（区别于二维码过期），由前端决定是否继续轮询
func (a *App) PollBaiduQR(sign string) *BaiduPollResult {
	bs, err := baiduGet("https://passport.baidu.com/channel/unicast?channel_id=" + sign)
	if err != nil {
		global.Log.Warnf("扫码轮询请求失败: %v", err)
		return &BaiduPollResult{Status: "network_error"}
	}

	var resp baiduPollResp
	if err := json.Unmarshal(bs, &resp); err != nil {
		global.Log.Warnf("解析扫码轮询响应失败: %v", err)
		return &BaiduPollResult{Status: "network_error"}
	}

	switch resp.Errno {
	case 1:
		// 尚无扫码事件，继续等待
		return &BaiduPollResult{Status: "waiting"}
	case 0:
		var cv baiduChannelV
		_ = json.Unmarshal([]byte(resp.ChannelV), &cv)

		if cv.V != "" {
			return &BaiduPollResult{Status: "success", V: cv.V}
		}
		if cv.Status == 1 {
			return &BaiduPollResult{Status: "scanned"}
		}
		return &BaiduPollResult{Status: "waiting"}
	default:
		// 其他 errno 通常表示二维码已过期或会话失效
		return &BaiduPollResult{Status: "error"}
	}
}

var baiduJSONCleaner = regexp.MustCompile(`\s+`)

// cleanBaiduJSON qrbdusslogin 返回的是类 JSON 文本（单引号、HTML 转义），先清洗再解析
func cleanBaiduJSON(s string) string {
	s = strings.ReplaceAll(s, "'", "\"")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	return baiduJSONCleaner.ReplaceAllString(s, " ")
}

// BaiduQRLogin 使用扫码得到的凭证 v 换取登录 Cookie，并验证网盘登录状态
// rememberLogin 为 true 时登录成功后保存登录信息到本地
func (a *App) BaiduQRLogin(v string, rememberLogin bool) *BaiduLoginResult {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	api := fmt.Sprintf(
		"https://passport.baidu.com/v3/login/main/qrbdusslogin?bduss=%s&qrcode=1&tpl=pp&apiver=v3&tt=%s&traceid=&time=%s&alg=v3&elapsed=1",
		v, ts, ts,
	)

	body, err := baiduGet(api)
	if err != nil {
		global.Log.Errorf("获取百度登录信息失败: %v", err)
		return &BaiduLoginResult{Success: false, Message: fmt.Sprintf("请求失败: %v", err)}
	}

	var result baiduLoginAPIResult
	if err := json.Unmarshal([]byte(cleanBaiduJSON(string(body))), &result); err != nil {
		global.Log.Errorf("解析百度登录信息失败: %v", err)
		return &BaiduLoginResult{Success: false, Message: fmt.Sprintf("解析登录信息失败: %v", err)}
	}

	if result.Data.Session.BDUSS == "" {
		msg := result.Message
		if msg == "" {
			msg = result.ErrInfo.Msg
		}
		return &BaiduLoginResult{Success: false, Message: "登录失败: " + msg}
	}

	// 扫码返回的 STOKEN 不能用于网盘接口，强制通过 plantcookie 刷新一次网盘 STOKEN
	stoken := refreshStoken(result.Data.Session.BDUSS, result.Data.Session.PToken)
	if stoken == "" {
		return &BaiduLoginResult{Success: false, Message: "获取 STOKEN 失败"}
	}

	login := &BaiduLoginResult{
		BDUSS:  result.Data.Session.BDUSS,
		PToken: result.Data.Session.PToken,
		SToken: stoken,
	}
	if ok := verifyAndFinalize(login, rememberLogin); !ok {
		return &BaiduLoginResult{Success: false, Message: "登录状态验证失败"}
	}
	return login
}

// ==================== Cookie 登录 ====================

// LoginWithCookie 使用用户手动填写的 Cookie 登录（仅需 BDUSS 与 PTOKEN，STOKEN 由 PTOKEN 换取）
// rememberLogin 为 true 时登录成功后保存登录信息到本地
func (a *App) LoginWithCookie(bduss string, ptoken string, rememberLogin bool) *BaiduLoginResult {
	bduss = strings.TrimSpace(bduss)
	ptoken = strings.TrimSpace(ptoken)

	if bduss == "" || ptoken == "" {
		return &BaiduLoginResult{Success: false, Message: "BDUSS 和 PTOKEN 均不能为空"}
	}

	stoken := refreshStoken(bduss, ptoken)
	if stoken == "" {
		return &BaiduLoginResult{Success: false, Message: "获取 STOKEN 失败，请检查 BDUSS / PTOKEN 是否有效"}
	}

	login := &BaiduLoginResult{
		BDUSS:  bduss,
		PToken: ptoken,
		SToken: stoken,
	}
	if ok := verifyAndFinalize(login, rememberLogin); !ok {
		return &BaiduLoginResult{Success: false, Message: "登录状态验证失败，请检查 BDUSS / PTOKEN 是否有效"}
	}
	return login
}

// ==================== STOKEN 与登录验证 ====================

// getndut 生成 plantcookie 所需的 ndut_fmt（AES-CBC 加密，参考 KinhWebEO utils/baidu.go）
// 该加密为百度接口约定格式（固定 key/iv），与本地凭证加密无关
func getndut() string {
	data := "time=" + strconv.FormatInt(time.Now().Unix(), 10) + ";ua=other"
	key := []byte("01hltm9JcnEfqy5t")
	iv := []byte("Fsadviz5BSekw310")

	block, err := aes.NewCipher(key)
	if err != nil {
		global.Log.Errorf("生成 ndut_fmt 失败: %v", err)
		return "-1"
	}
	crypted := make([]byte, len(pkcs7Pad([]byte(data), block.BlockSize())))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(crypted, pkcs7Pad([]byte(data), block.BlockSize()))
	return strings.ToUpper(hex.EncodeToString(crypted))
}

// refreshStoken 使用 BDUSS(+PTOKEN) 通过 plantcookie 换取 STOKEN
func refreshStoken(bduss, ptoken string) string {
	url := "https://pan.baidu.com/rest/2.0/xpan/file?method=plantcookie&type=stoken&source=pcs"
	cookie := "BDUSS=" + bduss + ";PANPSC=;BAIDUID=1;ndut_fmt=" + getndut()
	if ptoken != "" {
		cookie = "BDUSS=" + bduss + ";PTOKEN=" + ptoken + ";PANPSC=;BAIDUID=1;ndut_fmt=" + getndut()
	}

	resp, err := baiduGetWithResponse(url, "netdisk;Mo", cookie)
	if err != nil {
		global.Log.Errorf("获取 STOKEN 请求失败: %v", err)
		return ""
	}

	// 返回 302 说明需要先走一次 wappass 重新认证流程
	location := resp.Header.Get("Location")
	if location == "" || ptoken == "" {
		defer resp.Body.Close()
		return extractStokenFromResponse(resp)
	}
	resp.Body.Close()

	global.Log.Info("plantcookie 需要重新认证，尝试 wappass 流程")
	loginUrl := "https://wappass.baidu.com/v3/login/api/auth?notjump=1&return_type=3&tpl=netdisk&u=https%3A%2F%2Fpan.baidu.com%2Frest%2F2.0%2Fxpan%2Ffile%3Fmethod%3Dplantcookie%26source%3Dpcs%26callid%3D0.1%26type%3Dstoken%26from_module%3Dcloud-ui"
	resp2, err2 := baiduGetWithResponse(loginUrl, "netdisk;Mo", cookie)
	if err2 != nil {
		global.Log.Errorf("wappass 认证请求失败: %v", err2)
		return ""
	}
	location2 := resp2.Header.Get("Location")
	if location2 == "" {
		resp2.Body.Close()
		global.Log.Warn("wappass 认证失败: BDUSS 无效")
		return ""
	}
	if !strings.Contains(location2, "&stoken=") {
		resp2.Body.Close()
		global.Log.Warn("wappass 认证失败: PTOKEN 无效")
		return ""
	}
	resp2.Body.Close()

	resp, err = baiduGetWithResponse(location2, "netdisk;Mo", cookie)
	if err != nil {
		global.Log.Errorf("获取 STOKEN 请求失败: %v", err)
		return ""
	}
	defer resp.Body.Close()
	return extractStokenFromResponse(resp)
}

// extractStokenFromResponse 从响应的 Set-Cookie 头提取 STOKEN
func extractStokenFromResponse(resp *http.Response) string {
	for _, c := range resp.Header.Values("Set-Cookie") {
		if !strings.Contains(c, "STOKEN=") {
			continue
		}
		for _, p := range strings.Split(c, ";") {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, "STOKEN=") {
				global.Log.Info("获取 STOKEN 成功")
				return strings.TrimPrefix(p, "STOKEN=")
			}
		}
	}
	global.Log.Warn("未从响应中获取到 STOKEN")
	return ""
}

// verifyBaiduLogin 使用 BDUSS+STOKEN 访问网盘 loginStatus 接口验证登录
// 成功返回登录信息，失败返回 nil
func verifyBaiduLogin(bduss, stoken string) *baiduLoginStatus {
	url := "https://pan.baidu.com/api/loginStatus?clienttype=1&app_id=250528&web=1&channel=web&version=0"
	cookie := "BDUSS=" + bduss + ";STOKEN=" + stoken

	resp, err := baiduGetWithResponse(url, "netdisk;Mo", cookie)
	if err != nil {
		global.Log.Errorf("登录状态验证请求失败: %v", err)
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		global.Log.Errorf("读取登录状态响应失败: %v", err)
		return nil
	}
	var status baiduLoginStatus
	if err := json.Unmarshal(body, &status); err != nil {
		global.Log.Errorf("解析登录状态失败: %v", err)
		return nil
	}
	if status.Errno != 0 || status.LoginInfo.Username == "" {
		global.Log.Warnf("登录状态验证失败, errno: %d", status.Errno)
		return nil
	}
	return &status
}

// verifyAndFinalize 验证登录状态；验证失败（如 STOKEN 过期）时刷新一次 STOKEN 后重试
// rememberLogin 为 true 时登录成功后保存登录信息到本地
func verifyAndFinalize(login *BaiduLoginResult, rememberLogin bool) bool {
	status := verifyBaiduLogin(login.BDUSS, login.SToken)

	// 验证失败，刷新一次 STOKEN 再重试
	if status == nil {
		global.Log.Info("登录验证失败，尝试刷新 STOKEN 后重试")
		newStoken := refreshStoken(login.BDUSS, login.PToken)
		if newStoken == "" || newStoken == login.SToken {
			return false
		}
		login.SToken = newStoken
		status = verifyBaiduLogin(login.BDUSS, login.SToken)
		if status == nil {
			return false
		}
	}

	login.Success = true
	login.Username = status.LoginInfo.Username
	login.UserId = status.LoginInfo.UkStr
	login.BdStoken = status.LoginInfo.BdStoken
	login.PhotoUrl = status.LoginInfo.PhotoUrl
	// vip_type: "0" 无会员 / "1" VIP / "2" SVIP，解析失败按无会员处理
	if v, err := strconv.Atoi(status.LoginInfo.VipType); err == nil {
		login.VipType = v
	} else {
		login.VipType = 0
	}
	setCurrentCredential(login)
	// 勾选记住登录时保存到本地（data 目录，AES 加密）
	if rememberLogin {
		if err := saveBaiduCredential(login); err != nil {
			global.Log.Warnf("保存登录信息失败: %v", err)
		} else {
			global.Log.Info("登录信息已保存到本地")
		}
	}
	global.Log.Infof("百度网盘登录成功: %s (vip_type=%d)", login.Username, login.VipType)
	return true
}

// ==================== 凭证管理 ====================
//
// currentBaiduCredential 会被登录/登出/文件操作等多个 Wails 调用 goroutine 并发访问，
// 统一通过 credentialMu 读写锁保护；读取侧一律使用值拷贝，避免 TOCTOU 空指针。
var credentialMu sync.RWMutex

func setCurrentCredential(login *BaiduLoginResult) {
	credentialMu.Lock()
	defer credentialMu.Unlock()
	currentBaiduCredential = login
}

// currentCredentialSnapshot 返回当前凭证的值拷贝，未登录返回 nil
func currentCredentialSnapshot() *BaiduLoginResult {
	credentialMu.RLock()
	defer credentialMu.RUnlock()
	if currentBaiduCredential == nil {
		return nil
	}
	copied := *currentBaiduCredential
	return &copied
}

// updateCurrentCredentialStoken 原地更新当前凭证的 STOKEN（STOKEN 刷新后回写）
func updateCurrentCredentialStoken(stoken string) bool {
	credentialMu.Lock()
	defer credentialMu.Unlock()
	if currentBaiduCredential == nil {
		return false
	}
	currentBaiduCredential.SToken = stoken
	return true
}

// BaiduLogout 退出登录，清除凭证（同时删除本地保存的登录信息）
func (a *App) BaiduLogout() bool {
	setCurrentCredential(nil)
	clearBaiduCredential()
	global.Log.Info("已退出百度账号登录")
	return true
}
