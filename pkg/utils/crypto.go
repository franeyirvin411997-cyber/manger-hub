package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

var encryptionKey []byte

func init() {
	key := os.Getenv("ENCRYPTION_KEY")
	if key == "" {
		key = "mnp-default-key-change-in-prod!!" // 32 bytes for AES-256
	}
	if len(key) < 32 {
		key = key + strings.Repeat("0", 32-len(key))
	}
	encryptionKey = []byte(key[:32])
}

func Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// EncryptJSON 加密 JSON map
func EncryptJSON(data map[string]string) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return Encrypt(string(b))
}

// DecryptJSON 解密 JSON map
func DecryptJSON(encoded string) (map[string]string, error) {
	plaintext, err := Decrypt(encoded)
	if err != nil {
		return nil, err
	}
	var result map[string]string
	err = json.Unmarshal([]byte(plaintext), &result)
	return result, err
}

// MaskCredentials 将凭证值掩码化
func MaskCredentials(creds map[string]string) map[string]string {
	masked := make(map[string]string)
	for k, v := range creds {
		if len(v) <= 4 {
			masked[k] = "****"
		} else {
			masked[k] = v[:2] + "****" + v[len(v)-2:]
		}
	}
	return masked
}
