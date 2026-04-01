package utils

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
)

// SendNotification 发送 Webhook 通知
func SendNotification(title, message string) {
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		log.Printf("[通知-未配置] 收到消息: [%s] %s", title, message)
		return
	}

	payload := map[string]string{
		"title": title,
		"text":  message,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("[通知-失败] 无法发送 Webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("[通知-失败] Webhook 返回状态码: %d", resp.StatusCode)
	} else {
		log.Printf("[通知-成功] 已发送 Webhook: [%s]", title)
	}
}
