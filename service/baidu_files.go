package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"kinh-desktop/global"
)

type BaiduFileItem struct {
	FsID           int64  `json:"fs_id"`
	Path           string `json:"path"`
	Filename       string `json:"filename"`
	ServerFilename string `json:"server_filename"`
	Size           int64  `json:"size"`
	IsDir          int    `json:"isdir"`
	ServerMtime    int64  `json:"server_mtime"`
	Category       int    `json:"category"`
	MD5            string `json:"md5,omitempty"`
}

type BaiduFileListResult struct {
	Success bool            `json:"success"`
	Dir     string          `json:"dir"`
	List    []BaiduFileItem `json:"list"`
	Message string          `json:"message,omitempty"`
}

type baiduFileListResponse struct {
	Errno int             `json:"errno"`
	List  []BaiduFileItem `json:"list"`
}

// BaiduQuotaInfo 网盘容量信息
type BaiduQuotaInfo struct {
	Success  bool   `json:"success"`
	Total    int64  `json:"total"` // 总容量（字节）
	Used     int64  `json:"used"`  // 已用容量（字节）
	Message  string `json:"message,omitempty"`
}

type baiduQuotaResponse struct {
	Errno int64 `json:"errno"`
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
}

// GetBaiduQuota 获取网盘容量信息（GET /api/quota?checkfree=1&checkexpire=1）
func (a *App) GetBaiduQuota() (*BaiduQuotaInfo, error) {
	// 一次性取值拷贝，避免并发登出导致的空指针
	credential := currentCredentialSnapshot()
	if credential == nil || credential.BDUSS == "" {
		return nil, fmt.Errorf("百度网盘账号未登录")
	}

	cookie := "BDUSS=" + credential.BDUSS + ";PANPSC=;BAIDUID=1;ndut_fmt=" + getndut() + ";STOKEN=" + credential.SToken
	apiURL := "https://pan.baidu.com/api/quota?checkfree=1&checkexpire=1"

	resp, err := baiduGetWithResponse(apiURL, "netdisk;Mo", cookie)
	if err != nil {
		return nil, fmt.Errorf("获取容量信息失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取容量信息响应失败: %w", err)
	}

	var result baiduQuotaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析容量信息失败: %w", err)
	}
	if result.Errno != 0 {
		return &BaiduQuotaInfo{
			Success: false,
			Message: fmt.Sprintf("获取容量信息失败: %d", result.Errno),
		}, nil
	}

	return &BaiduQuotaInfo{
		Success: true,
		Total:   result.Total,
		Used:    result.Used,
	}, nil
}

// isCredentialErrno 判断是否为凭证失效类错误码（-6 身份验证失败 / 111 账号未登录）
func isCredentialErrno(errno int) bool {
	return errno == -6 || errno == 111
}

// fetchBaiduFileListOnce 使用指定 STOKEN 请求一次文件列表
func fetchBaiduFileListOnce(dir, bduss, stoken string) (*BaiduFileListResult, error) {
	cookie := "BDUSS=" + bduss + ";PANPSC=;BAIDUID=1;ndut_fmt=" + getndut() + ";STOKEN=" + stoken
	apiURL := "https://pan.baidu.com/api/listall?clienttype=0&app_id=250528&web=1&order=time&dir=" + url.QueryEscape(dir)

	resp, err := baiduGetWithResponse(apiURL, "netdisk;Mo", cookie)
	if err != nil {
		return nil, fmt.Errorf("获取文件列表失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取文件列表响应失败: %w", err)
	}

	var result baiduFileListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析文件列表失败: %w", err)
	}
	if result.Errno != 0 {
		return &BaiduFileListResult{
			Success: false,
			Dir:     dir,
			List:    []BaiduFileItem{},
			Message: fmt.Sprintf("百度网盘接口返回错误: %d", result.Errno),
		}, nil
	}
	if result.List == nil {
		result.List = []BaiduFileItem{}
	}
	return &BaiduFileListResult{
		Success: true,
		Dir:     dir,
		List:    result.List,
	}, nil
}

func (a *App) GetBaiduFileList(dir string) (*BaiduFileListResult, error) {
	if dir == "" {
		dir = "/"
	}
	// 一次性取值拷贝，避免并发登出导致的空指针
	credential := currentCredentialSnapshot()
	if credential == nil || credential.BDUSS == "" {
		return nil, fmt.Errorf("百度网盘账号未登录")
	}

	stoken := credential.SToken
	if stoken == "" {
		stoken = refreshStoken(credential.BDUSS, credential.PToken)
		if stoken != "" {
			updateCurrentCredentialStoken(stoken)
		}
	}

	cookie := "BDUSS=" + credential.BDUSS + ";PANPSC=;BAIDUID=1;ndut_fmt=" + getndut() + ";STOKEN=" + stoken

	apiURL := "https://pan.baidu.com/api/listall?clienttype=0&app_id=250528&web=1&order=time&dir=" + url.QueryEscape(dir)
	resp, err := baiduGetWithResponse(apiURL, "netdisk;Mo", cookie)
	if err != nil {
		global.Log.Errorf("获取百度网盘文件列表失败: %v", err)
		return nil, fmt.Errorf("获取文件列表失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取文件列表响应失败: %w", err)
	}

	var result baiduFileListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		global.Log.Errorf("解析百度网盘文件列表失败: %v", err)
		return nil, fmt.Errorf("解析文件列表失败: %w", err)
	}

	// STOKEN 失效特征：返回未登录类错误码时，刷新一次 STOKEN 重试；仍失败则按正常错误返回
	if result.Errno != 0 && isCredentialErrno(result.Errno) {
		global.Log.Infof("文件列表返回 errno=%d，尝试刷新 STOKEN 后重试", result.Errno)
		newStoken := refreshStoken(credential.BDUSS, credential.PToken)
		if newStoken != "" && newStoken != stoken {
			updateCurrentCredentialStoken(newStoken)
			updateSavedCredentialStoken(newStoken)
			return fetchBaiduFileListOnce(dir, credential.BDUSS, newStoken)
		}
	}

	if result.Errno != 0 {
		return &BaiduFileListResult{
			Success: false,
			Dir:     dir,
			List:    []BaiduFileItem{},
			Message: fmt.Sprintf("百度网盘接口返回错误: %d", result.Errno),
		}, nil
	}
	if result.List == nil {
		result.List = []BaiduFileItem{}
	}

	return &BaiduFileListResult{
		Success: true,
		Dir:     dir,
		List:    result.List,
	}, nil
}
