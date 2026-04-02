package tgbot

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 35 * time.Second}

func httpGet(url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func httpPost(url string, body []byte) {
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("[TGBot] HTTP POST 失败: %v", err)
		return
	}
	resp.Body.Close()
}
