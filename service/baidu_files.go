package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"kinh-desktop/global"
)

type BaiduFileThumbs struct {
	Icon  string `json:"icon,omitempty"`
	Image string `json:"image,omitempty"`
	URL1  string `json:"url1,omitempty"`
	URL2  string `json:"url2,omitempty"`
	URL3  string `json:"url3,omitempty"`
}

type BaiduFileItem struct {
	FsID           int64            `json:"fs_id"`
	Path           string           `json:"path"`
	Filename       string           `json:"filename"`
	ServerFilename string           `json:"server_filename"`
	Size           int64            `json:"size"`
	IsDir          int              `json:"isdir"`
	ServerMtime    int64            `json:"server_mtime"`
	Category       int              `json:"category"`
	MD5            string           `json:"md5,omitempty"`
	Thumbs         *BaiduFileThumbs `json:"thumbs,omitempty"`
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

func (a *App) GetBaiduFileList(dir string) (*BaiduFileListResult, error) {
	if dir == "" {
		dir = "/"
	}
	if currentBaiduCredential == nil || currentBaiduCredential.BDUSS == "" {
		return nil, fmt.Errorf("百度网盘账号未登录")
	}

	credential := currentBaiduCredential
	stoken := credential.SToken
	if stoken == "" {
		stoken = refreshStoken(credential.BDUSS, credential.PToken)
		if stoken != "" {
			credential.SToken = stoken
		}
	}

	cookie := "BDUSS=" + credential.BDUSS + ";PANPSC=;BAIDUID=1;ndut_fmt=" + getndut()
	if stoken != "" {
		cookie += ";STOKEN=" + stoken
	}

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
