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

	"kinh-desktop/global"
)

// ==================== 登录信息本地保存（data 目录，对称可逆加密） ====================
//
// 勾选"记住登录"后，登录成功将 BDUSS/PTOKEN/STOKEN 以 AES-256-CBC 加密写入
// data/credential.dat；下次启动可通过 RestoreLogin 自动恢复登录。
// 加密密钥 = SHA256(机器指纹 + data/key.bin 随机盐)，首次使用时自动生成盐文件；
// 密钥文件与凭证记录存放在同一 data 目录，均不随程序分发。

const (
	baiduDataDir        = "data"
	baiduCredentialFile = "credential.dat"
	baiduKeyFile        = "key.bin"
	keySaltSize         = 32
)

var loadedCredKey []byte

type baiduSavedCredential struct {
	BDUSS  string `json:"bduss"`
	PToken string `json:"ptoken"`
	SToken string `json:"stoken"`
}

// machineFingerprint 采集机器指纹（主机名 + 主网卡 MAC），跨平台无额外依赖
func machineFingerprint() string {
	host, _ := os.Hostname()
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
func ensureCredKey() ([]byte, error) {
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

func encryptCredential(plain []byte) (string, error) {
	key, err := ensureCredKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	// PKCS7 填充
	padding := block.BlockSize() - len(plain)%block.BlockSize()
	padded := make([]byte, len(plain)+padding)
	copy(padded, plain)
	for i := len(plain); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key[:block.BlockSize()]).CryptBlocks(encrypted, padded)
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func decryptCredential(text string) (*baiduSavedCredential, error) {
	key, err := ensureCredKey()
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data)%block.BlockSize() != 0 {
		return nil, os.ErrInvalid
	}
	decrypted := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, key[:block.BlockSize()]).CryptBlocks(decrypted, data)

	// 去除 PKCS7 填充
	padding := int(decrypted[len(decrypted)-1])
	if padding <= 0 || padding > block.BlockSize() {
		return nil, os.ErrInvalid
	}

	var saved baiduSavedCredential
	if err := json.Unmarshal(decrypted[:len(decrypted)-padding], &saved); err != nil {
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
	loadedCredKey = nil
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
