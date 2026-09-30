package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kinh-desktop/global"
)

// ==================== 手机号（短信验证码）登录 ====================
//
// 走百度 wappass 的短信登录链路，分两步：
//   1. POST /wp/api/login/sms         仅需 username=手机号，触发下发短信
//      命中风控时响应会带上 vcodestr/vcodesign，需先过图形验证码再重发
//   2. POST /wp/api/login?v=<ts>      带 smsvc=短信验证码 完成登录，响应回传 BDUSS/PTOKEN
//
// 拿到 BDUSS/PTOKEN 后与 Cookie 登录完全同路：refreshStoken 换 STOKEN →
// verifyAndFinalize 验证并落库，因此无需另写一套凭证处理。
//
// 注意：该链路要求移动端 UA。用桌面 UA 请求会被 passport 判成账号密码登录
// （返回 errno 200003「请输入登录密码」），拿不到短信流程。

const (
	baiduSMSSendEndpoint  = "https://wappass.baidu.com/wp/api/login/sms"
	baiduSMSLoginEndpoint = "https://wappass.baidu.com/wp/api/login"
	baiduSMSVCodeImageURL = "https://wappass.baidu.com/cgi-bin/genimage"
	baiduSMSReferer       = "https://wappass.baidu.com/"

	// 移动端 UA：见上方说明，桌面 UA 会被引导到密码登录流程
	baiduSMSUserAgent = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"

	// 发送验证码 / 登录响应的 errInfo.no 取值
	baiduErrOK          = "0"      // 成功
	baiduErrSMSCodeBad  = "400011" // 短信验证码错误
	baiduErrTooFrequent = "50014"  // 操作过于频繁
)

// 大陆手机号：1 开头共 11 位。这里只做长度与首位校验，
// 号段有效性交给百度判定（本地按号段拦会把携号转网等新号段误伤）
var baiduPhonePattern = regexp.MustCompile(`^1\d{10}$`)

// baiduSMSClient 带 Cookie Jar：wappass 会把 BAIDUID 等写进 Cookie，
// 发送与登录两次请求需要落在同一会话里
var baiduSMSClient = func() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: false,
		},
		Jar: jar,
	}
}()

// BaiduSMSCodeResult 发送短信验证码的结果
type BaiduSMSCodeResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	CodeLength int    `json:"code_length"` // 短信验证码位数，默认 6
	NeedVCode  bool   `json:"need_vcode"`  // 需要先填写图形验证码
	VCodeImage string `json:"vcode_image"` // 图形验证码图片（Data URL）
	VCodeStr   string `json:"vcode_str"`   // 图形验证码会话串，重发时原样回传
	VCodeSign  string `json:"vcode_sign"`  // 图形验证码签名，重发时原样回传
}

// 短信发送 / 登录的响应体（两接口结构一致，字段按需取用）
type baiduWapPassResp struct {
	ErrInfo struct {
		No  string `json:"no"`
		Msg string `json:"msg"`
	} `json:"errInfo"`
	Data struct {
		VCodeStr      string `json:"vcodestr"`
		VCodeSign     string `json:"vcodesign"`
		SMSCodeLength int    `json:"smsCodeLength"`
		BDUSS         string `json:"bduss"`
		PToken        string `json:"ptoken"`
		UserID        string `json:"userid"`
		Phone         string `json:"phone"`
	} `json:"data"`
}

// SendBaiduSMSCode 向指定手机号发送登录短信验证码。
// verifyCode / vcodeStr / vcodeSign 仅在上一轮返回 need_vcode 时填写：
// vcodeStr、vcodeSign 原样回传上一轮的值，verifyCode 是用户看图填的字符。
func (a *App) SendBaiduSMSCode(phone string, verifyCode string, vcodeStr string, vcodeSign string) *BaiduSMSCodeResult {
	phone = strings.TrimSpace(phone)
	if !baiduPhonePattern.MatchString(phone) {
		return &BaiduSMSCodeResult{Success: false, Message: "请输入正确的手机号"}
	}

	form := url.Values{"username": {phone}}
	if vcodeStr != "" {
		form.Set("verifycode", strings.TrimSpace(verifyCode))
		form.Set("vcodestr", vcodeStr)
		form.Set("vcodesign", vcodeSign)
	}

	body, err := baiduSMSPost(baiduSMSSendEndpoint, form)
	if err != nil {
		global.Log.Errorf("发送短信验证码失败: %v", err)
		return &BaiduSMSCodeResult{Success: false, Message: fmt.Sprintf("请求失败: %v", err)}
	}

	var resp baiduWapPassResp
	if err := json.Unmarshal(body, &resp); err != nil {
		global.Log.Errorf("解析短信发送响应失败: %v", err)
		return &BaiduSMSCodeResult{Success: false, Message: "解析响应失败"}
	}

	// 风控要求图形验证码：把图片与凭证交回前端，用户填完再重发一次
	if resp.Data.VCodeStr != "" {
		global.Log.Info("短信登录命中图形验证码风控")
		return &BaiduSMSCodeResult{
			Success:    false,
			Message:    resp.ErrInfo.Msg,
			NeedVCode:  true,
			VCodeImage: fetchBaiduVCodeImage(resp.Data.VCodeStr),
			VCodeStr:   resp.Data.VCodeStr,
			VCodeSign:  resp.Data.VCodeSign,
		}
	}

	if resp.ErrInfo.No != baiduErrOK {
		msg := resp.ErrInfo.Msg
		if msg == "" {
			msg = "验证码发送失败"
		}
		global.Log.Warnf("发送短信验证码失败: [%s] %s", resp.ErrInfo.No, msg)
		return &BaiduSMSCodeResult{Success: false, Message: msg}
	}

	length := resp.Data.SMSCodeLength
	if length <= 0 {
		length = 6
	}
	global.Log.Infof("短信验证码已发送: %s", maskPhone(phone))
	return &BaiduSMSCodeResult{Success: true, Message: "验证码已发送", CodeLength: length}
}

// LoginWithBaiduSMS 使用手机号 + 短信验证码登录。
// rememberLogin 为 true 时登录成功后保存登录信息到本地。
func (a *App) LoginWithBaiduSMS(phone string, smsCode string, rememberLogin bool) *BaiduLoginResult {
	phone = strings.TrimSpace(phone)
	smsCode = strings.TrimSpace(smsCode)

	if !baiduPhonePattern.MatchString(phone) {
		return &BaiduLoginResult{Success: false, Message: "请输入正确的手机号"}
	}
	if smsCode == "" {
		return &BaiduLoginResult{Success: false, Message: "请输入短信验证码"}
	}

	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	form := url.Values{
		"clientfrom": {"native"},
		"tpl":        {"netdisk"},
		"client":     {"android"},
		"adapter":    {"3"},
		"t":          {ts},
		"gid":        {baiduRandomGID()},
		"traceid":    {baiduRandomTraceID()},
		"username":   {phone},
		"mobilenum":  {phone},
		"sms":        {"1"},
		"smsverify":  {"1"},
		"smsvc":      {smsCode},
	}

	body, err := baiduSMSPost(baiduSMSLoginEndpoint+"?v="+ts, form)
	if err != nil {
		global.Log.Errorf("短信登录请求失败: %v", err)
		return &BaiduLoginResult{Success: false, Message: fmt.Sprintf("请求失败: %v", err)}
	}

	var resp baiduWapPassResp
	if err := json.Unmarshal(body, &resp); err != nil {
		global.Log.Errorf("解析短信登录响应失败: %v", err)
		return &BaiduLoginResult{Success: false, Message: "解析响应失败"}
	}

	if resp.ErrInfo.No != baiduErrOK || resp.Data.BDUSS == "" {
		msg := resp.ErrInfo.Msg
		if msg == "" {
			msg = "登录失败"
		}
		global.Log.Warnf("短信登录失败: [%s] %s", resp.ErrInfo.No, msg)
		return &BaiduLoginResult{Success: false, Message: msg}
	}

	login := &BaiduLoginResult{
		BDUSS:  resp.Data.BDUSS,
		PToken: resp.Data.PToken,
	}
	login.SToken = refreshStoken(login.BDUSS, login.PToken)
	if login.SToken == "" {
		return &BaiduLoginResult{Success: false, Message: "获取 STOKEN 失败"}
	}
	if ok := verifyAndFinalize(login, rememberLogin); !ok {
		return &BaiduLoginResult{Success: false, Message: "登录状态验证失败"}
	}
	return login
}

// baiduSMSPost 以表单方式 POST 到 wappass 接口
func baiduSMSPost(endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", baiduSMSUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", baiduSMSReferer)

	resp, err := baiduSMSClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// fetchBaiduVCodeImage 下载图形验证码图片并编码为 Data URL，失败返回空串
// （图片拿不到不阻断流程：前端会退化成让用户直接输入）
func fetchBaiduVCodeImage(vcodestr string) string {
	req, err := http.NewRequest("GET", baiduSMSVCodeImageURL+"?"+vcodestr, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", baiduSMSUserAgent)
	req.Header.Set("Referer", baiduSMSReferer)

	resp, err := baiduSMSClient.Do(req)
	if err != nil {
		global.Log.Warnf("下载图形验证码失败: %v", err)
		return ""
	}
	defer resp.Body.Close()

	img, err := io.ReadAll(resp.Body)
	if err != nil || len(img) == 0 {
		global.Log.Warn("下载图形验证码失败: 响应为空")
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(img)
}

// baiduRandomGID 生成 32 位大写十六进制 gid（百度客户端登录的设备标识参数）
func baiduRandomGID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strings.ToUpper(strconv.FormatInt(time.Now().UnixNano(), 16))
	}
	return strings.ToUpper(hex.EncodeToString(buf))
}

const baiduTraceIDChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// baiduRandomTraceID 生成 8 位大写 traceid
func baiduRandomTraceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "AAAAAAAA"
	}
	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = baiduTraceIDChars[int(b)%len(baiduTraceIDChars)]
	}
	return string(out)
}

// maskPhone 手机号脱敏，用于日志（138****8000）
func maskPhone(phone string) string {
	if len(phone) != 11 {
		return "***"
	}
	return phone[:3] + "****" + phone[7:]
}
