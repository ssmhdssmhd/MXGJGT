package service

// ST 令牌：把「真实播放地址 + 有效期」用 AES-256-GCM 加密成不可读字符串。
// 客户端只能拿到密文（st=...），必须回到本服务才能解密使用，
// 从而避免真实地址 / 接口被他人直接盗用；令牌到期后自动失效。

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// STTokenVersion 令牌格式版本
const STTokenVersion = 1

// STTokenPayload ST 令牌明文载荷
type STTokenPayload struct {
	V int    `json:"v"` // 版本
	U string `json:"u"` // 原始地址（播放页 / m3u8）
	T string `json:"t"` // 类型：m3u8 / json
	E int64  `json:"e"` // 过期时间（unix 秒），0=永久
}

// GenerateSTSecret 生成 32 字节随机密钥（hex 编码，64 字符）
func GenerateSTSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 极端情况下退回时间戳，保证不返回空密钥
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(b)
}

// stAEAD 由密钥派生 AES-256-GCM
func stAEAD(secret string) (cipher.AEAD, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("未配置 ST 加密密钥")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// EncodeST 生成 ST 加密令牌
// rawURL：真实地址；typ：m3u8 / json；ttlSec：有效期秒数（<=0 表示永久）
func EncodeST(rawURL, typ, secret string, ttlSec int) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("缺少待加密地址")
	}
	gcm, err := stAEAD(secret)
	if err != nil {
		return "", err
	}
	p := STTokenPayload{V: STTokenVersion, U: rawURL, T: typ}
	if ttlSec > 0 {
		p.E = time.Now().Add(time.Duration(ttlSec) * time.Second).Unix()
	}
	plain, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// DecodeST 解密 ST 令牌并校验有效期
func DecodeST(token, secret string) (*STTokenPayload, error) {
	gcm, err := stAEAD(secret)
	if err != nil {
		return nil, err
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return nil, errors.New("st 令牌格式错误")
	}
	ns := gcm.NonceSize()
	if len(raw) <= ns {
		return nil, errors.New("st 令牌格式错误")
	}
	plain, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return nil, errors.New("st 令牌校验失败（密钥不匹配或已被篡改）")
	}
	var p STTokenPayload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, errors.New("st 令牌内容错误")
	}
	if strings.TrimSpace(p.U) == "" {
		return nil, errors.New("st 令牌缺少地址")
	}
	if p.E > 0 && time.Now().Unix() > p.E {
		return nil, errors.New("st 令牌已过期，请重新获取")
	}
	return &p, nil
}

// STExpireAt 由有效期推断过期时间（unix 秒，0=永久），供后台展示
func STExpireAt(ttlSec int) int64 {
	if ttlSec <= 0 {
		return 0
	}
	return time.Now().Add(time.Duration(ttlSec) * time.Second).Unix()
}
