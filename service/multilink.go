package service

import (
	"sync"
	"time"

	"kinh-desktop/global"
)

// ==================== 实验性下载 ====================
//
// 对选定的地址渠道（本地解析 / 加速链接）并发发起多次解析请求。
// ⚠️ 百度 API 每次请求都会重新生成下载地址，因此同一渠道多次请求
// 即可获得多个不同的地址 —— 这是多地址的唯一来源，无需去重。
// 拿到的多个地址交给引擎的 MultiSources 能力，从多个源同时拉取同一文件。
// 解析进度经事件 experimental-link-progress 推送到前端（弹窗展示）。

// multiLinkProgressEvent 解析进度事件名（前端 Events.On 监听）
const multiLinkProgressEvent = "experimental-link-progress"

// MaxMultiLinkSources 最多获取的地址数。
// 引擎按加权轮询使用多源（主 URL 权重更高），6 个源已能覆盖常见场景
const MaxMultiLinkSources = 6

// 多地址渠道取值（experimental_channel 字段）
const (
	ChannelAuto   = "auto"   // 程序决定：有加速链接用加速，否则走本地
	ChannelRemote = "remote" // 加速链接
	ChannelLocal  = "local"  // 本地解析
)

// multiLinkProgress 解析进度载荷（JSON 推给前端）
type multiLinkProgress struct {
	FsID     int64  `json:"fs_id"`
	FileName string `json:"file_name"`
	Done     int    `json:"done"` // 已完成的解析请求数
	Total    int    `json:"total"`
	Links    int    `json:"links"` // 已拿到的地址数
	Finished bool   `json:"finished"`
	Success  bool   `json:"success"`
	Message  string `json:"message,omitempty"`
}

// appRef 服务实例引用（NewApp 时注入；emitMultiLinkProgress 用于事件推送）
var appRef *App

// emitMultiLinkProgress 推送解析进度到前端（app 未注入时静默跳过，如测试环境）
func emitMultiLinkProgress(p multiLinkProgress) {
	if appRef == nil {
		return
	}
	appRef.app.Event.Emit(multiLinkProgressEvent, p)
}

// multiLinkChannel 实验性下载实际使用的地址渠道（remote / local）。
// experimental_channel 的语义：
//   - remote：加速链接（未配置加速链接时由 SaveSettings 强制回落 auto）
//   - local：本地解析
//   - auto（默认）：程序决定 —— 有加速链接走加速，没有走本地
func multiLinkChannel() string {
	s := getSettings()
	switch s.ExperimentalChannel {
	case ChannelRemote:
		// 兜底：加速链接被清空后设置值可能残留 remote，此时回落本地
		if !stringHasValue(s.DownloadAccLink) {
			return ChannelLocal
		}
		return ChannelRemote
	case ChannelLocal:
		return ChannelLocal
	default:
		if stringHasValue(s.DownloadAccLink) {
			return ChannelRemote
		}
		return ChannelLocal
	}
}

// ResolveMultiLink 并发解析文件下载地址（实验性下载入口）。
//
// 对选定渠道**并发发起 count 次解析请求**：百度 API 每次请求都会重新生成
// 下载地址，因此 N 次请求即得 N 个不同地址（无需去重）。全部请求完成后
// 统一收集，第一个地址作为主 URL，其余进引擎 MultiSources 多源下载。
//
// ⚠️ 加速链接未配置/已清空时渠道必为 local（multiLinkChannel 内有回落），
// 绝不会对空地址发起请求；并发调 resolveWithRetry 安全 ——
// 凭证读写有 credentialMu 保护（baidu_auth.go）。
func (a *App) ResolveMultiLink(fsID int64, fileName string, count int) *MultiLinkResult {
	start := time.Now()

	channel := multiLinkChannel()

	// 数量夹取：2 ~ MaxMultiLinkSources（1 无意义——单地址等价于普通下载）
	count = clampInt(count, 2, MaxMultiLinkSources)

	emitMultiLinkProgress(multiLinkProgress{
		FsID: fsID, FileName: fileName, Done: 0, Total: count, Links: 0,
	})

	var (
		mu      sync.Mutex
		urls    []string
		doneCnt int
	)

	// 每完成一次请求就推一次进度；等全部完成后才置 finished
	// （⚠️ 不能提前结束：并发请求就是为了拿多个不同地址，
	//   一部分完成就收工会导致地址数少于设置值 —— 0/4 只获取 2 个的根因）
	emit := func(success bool, msg string) {
		mu.Lock()
		doneCnt++
		done, links := doneCnt, len(urls)
		fin := doneCnt >= count
		mu.Unlock()
		emitMultiLinkProgress(multiLinkProgress{
			FsID: fsID, FileName: fileName,
			Done: done, Total: count, Links: links,
			Finished: fin, Success: success, Message: msg,
		})
	}

	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var r *BaiduDownloadLinkResult
			if channel == ChannelRemote {
				r = a.GetBaiduDownloadLinkRemote(fsID)
			} else {
				r = a.GetBaiduDownloadLink(fsID)
			}
			mu.Lock()
			if r.Success && r.Dlink != "" {
				urls = append(urls, r.Dlink)
			}
			ok := r.Success
			mu.Unlock()
			emit(ok, r.Message)
		}()
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(urls) == 0 {
		return &MultiLinkResult{Success: false, Message: "所有解析请求均未获取到下载地址"}
	}

	// 第一个地址作为主 URL，其余进 MultiSources
	result := &MultiLinkResult{
		Success:  true,
		FsID:     fsID,
		Filename: fileName,
		Dlink:    urls[0],
		Sources:  make([]string, 0, len(urls)-1),
		Elapsed:  time.Since(start).Round(time.Millisecond).String(),
	}
	for _, u := range urls[1:] {
		result.Sources = append(result.Sources, u)
	}
	global.Log.Infof("实验性下载地址解析完成: %s, 渠道=%s, %d 个地址, 耗时 %s",
		fileName, channel, len(urls), result.Elapsed)
	return result
}

// stringHasValue 非空判断（TrimSpace 后非空）
func stringHasValue(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

// MultiLinkResult 地址解析结果（返回给前端）
type MultiLinkResult struct {
	Success  bool     `json:"success"`
	FsID     int64    `json:"fs_id"`
	Filename string   `json:"filename"`
	Dlink    string   `json:"dlink"`   // 主地址（首个）
	Sources  []string `json:"sources"` // 其余地址（进引擎 MultiSources）
	Elapsed  string   `json:"elapsed"` // 解析耗时（展示用）
	Message  string   `json:"message,omitempty"`
}
