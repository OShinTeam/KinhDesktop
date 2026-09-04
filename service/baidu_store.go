package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"kinh-desktop/global"
)

// ==================== 登录信息本地保存（data 目录，对称可逆加密） ====================
//
// 威胁模型（开源 + 本地存储的平衡取舍）：
//   - 防：直接拷贝 data/ 目录到其他机器解密使用（密钥含本机指纹绑定）
//   - 防：密文被篡改/损坏后仍被程序误用（AES-GCM 自带完整性校验）
//   - 不防：已获得本机文件读取权限的定向攻击者——该威胁需 OS 级凭据库（DPAPI/Keychain），
//     对本项目属过度设计；且此类攻击者可同样读取浏览器 Cookie 库，边际收益趋零
//
// 方案：AES-256-GCM 认证加密，密钥 = SHA256(机器指纹 + data/key.bin 随机盐)，
// 盐文件首次使用时生成，与凭证同目录、均不随程序分发。退出登录时全部清除。

const (
	baiduDataDir        = "data"
	baiduCredentialFile = "credential.dat"
	baiduKeyFile        = "key.bin"
	keySaltSize         = 32
)

// loadedCredKey 缓存派生密钥，credKeyMu 保护并发读写（登录/登出在不同 goroutine 触发）
var (
	credKeyMu     sync.RWMutex
	loadedCredKey []byte
)

type baiduSavedCredential struct {
	BDUSS  string `json:"bduss"`
	PToken string `json:"ptoken"`
	SToken string `json:"stoken"`
}

// machineFingerprint 采集机器指纹（主机名 + 主网卡 MAC），跨平台无额外依赖
func machineFingerprint() string {
	host, err := os.Hostname()
	if err != nil {
		global.Log.Warnf("获取主机名失败，指纹降级为仅 MAC: %v", err)
		host = ""
	}
	var macs []string
	if interfaces, err := net.Interfaces(); err == nil {
		for _, iface := range interfaces {
			// 排除回环与未启用的虚拟接口，取真实硬件网卡 MAC
			if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 || iface.HardwareAddr == nil {
				continue
			}
			mac := iface.HardwareAddr.String()
			if mac != "" {
				macs = append(macs, mac)
			}
		}
	}
	return strings.Join(append([]string{host}, macs...), "|")
}

// ensureCredKey 加载或生成加密密钥：SHA256(机器指纹 + 随机盐)
// 盐文件 data/key.bin 首次使用时生成；密钥文件丢失或指纹变化将无法解密旧记录
// 机器指纹绑定使得密文离开本机后无法解密（防搬运），但无法对抗已控制本机的攻击者
func ensureCredKey() ([]byte, error) {
	credKeyMu.RLock()
	if loadedCredKey != nil {
		key := loadedCredKey
		credKeyMu.RUnlock()
		return key, nil
	}
	credKeyMu.RUnlock()

	credKeyMu.Lock()
	defer credKeyMu.Unlock()
	// 双重检查：拿到写锁后可能已被其他 goroutine 填充
	if loadedCredKey != nil {
		return loadedCredKey, nil
	}

	if err := os.MkdirAll(baiduDataDir, 0755); err != nil {
		return nil, err
	}

	keyPath := filepath.Join(baiduDataDir, baiduKeyFile)
	salt, err := os.ReadFile(keyPath)
	if err != nil || len(salt) != keySaltSize {
		// 首次启动生成 32 字节随机盐
		salt = make([]byte, keySaltSize)
		if _, err := rand.Read(salt); err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, salt, 0600); err != nil {
			return nil, err
		}
		global.Log.Info("已生成新的密钥盐文件")
	}

	hash := sha256.Sum256(append([]byte(machineFingerprint()), salt...))
	loadedCredKey = hash[:]
	return loadedCredKey, nil
}

// pkcs7Pad PKCS7 填充（ndut_fmt 使用；凭证加密已改用 GCM 无需填充）
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padded := make([]byte, len(data)+padding)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	return padded
}

// blockFor 由 32 字节派生密钥构造 AES block
func blockFor(key []byte) cipher.Block {
	block, err := aes.NewCipher(key)
	if err != nil {
		// SHA256 输出恒为 32 字节，合法 AES-256 key，此处仅为接口兜底
		global.Log.Errorf("构造 AES cipher 失败: %v", err)
		return nil
	}
	return block
}

// credNonceSize GCM 标准 nonce 长度
const credNonceSize = 12

// encryptCredential AES-256-GCM 认证加密：随机 nonce 前置存储，密文自带完整性校验
func encryptCredential(plain []byte) (string, error) {
	key, err := ensureCredKey()
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(blockFor(key))
	if err != nil {
		return "", err
	}
	nonce := make([]byte, credNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptCredential AES-256-GCM 解密：篡改或指纹变化都会在校验阶段直接失败
func decryptCredential(text string) (*baiduSavedCredential, error) {
	key, err := ensureCredKey()
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blockFor(key))
	if err != nil {
		return nil, err
	}
	if len(data) < credNonceSize {
		return nil, os.ErrInvalid
	}
	nonce, ciphertext := data[:credNonceSize], data[credNonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	var saved baiduSavedCredential
	if err := json.Unmarshal(plain, &saved); err != nil {
		return nil, err
	}
	return &saved, nil
}

func credentialFilePath() string {
	return filepath.Join(baiduDataDir, baiduCredentialFile)
}

// saveBaiduCredentialData 将凭证加密后写入 data 目录
func saveBaiduCredentialData(saved *baiduSavedCredential) error {
	if err := os.MkdirAll(baiduDataDir, 0755); err != nil {
		return err
	}
	plain, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	text, err := encryptCredential(plain)
	if err != nil {
		return err
	}
	return os.WriteFile(credentialFilePath(), []byte(text), 0600)
}

// saveBaiduCredential 保存当前登录凭证（勾选记住登录时调用）
func saveBaiduCredential(login *BaiduLoginResult) error {
	return saveBaiduCredentialData(&baiduSavedCredential{
		BDUSS:  login.BDUSS,
		PToken: login.PToken,
		SToken: login.SToken,
	})
}

// loadBaiduCredential 读取本地保存的登录信息，未保存或解密失败返回 nil
func loadBaiduCredential() *baiduSavedCredential {
	data, err := os.ReadFile(credentialFilePath())
	if err != nil {
		return nil
	}
	saved, err := decryptCredential(string(data))
	if err != nil {
		global.Log.Warnf("解析本地登录信息失败: %v", err)
		return nil
	}
	return saved
}

// updateSavedCredentialStoken STOKEN 刷新成功后回写本地记录（未开启记住登录时不写）
func updateSavedCredentialStoken(stoken string) {
	saved := loadBaiduCredential()
	if saved == nil {
		return
	}
	saved.SToken = stoken
	if err := saveBaiduCredentialData(saved); err != nil {
		global.Log.Warnf("回写本地登录信息失败: %v", err)
	}
}

// clearBaiduCredential 清空本地密钥与登录信息（盐文件一并删除，下次使用时重新生成）
func clearBaiduCredential() {
	for _, file := range []string{baiduCredentialFile, baiduKeyFile} {
		path := filepath.Join(baiduDataDir, file)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			global.Log.Warnf("删除本地文件失败: %s: %v", path, err)
		}
	}
	// 密钥已失效，清空内存缓存，下次使用时重新派生
	credKeyMu.Lock()
	loadedCredKey = nil
	credKeyMu.Unlock()
}

// RestoreLogin 使用本地保存的登录信息自动登录（含 STOKEN 失效时刷新一次重试）
func (a *App) RestoreLogin() *BaiduLoginResult {
	saved := loadBaiduCredential()
	if saved == nil || saved.BDUSS == "" {
		return nil
	}

	global.Log.Info("检测到本地登录信息，尝试自动登录")
	login := &BaiduLoginResult{
		BDUSS:  saved.BDUSS,
		PToken: saved.PToken,
		SToken: saved.SToken,
	}
	if ok := verifyAndFinalize(login, true); !ok {
		global.Log.Warn("本地登录信息自动登录失败")
		return nil
	}
	return login
}
