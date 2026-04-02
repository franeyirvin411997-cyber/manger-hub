package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multi_node_platform/pkg/models"
)

func TestCreateBrowserUsesDedicatedTunnelPayload(t *testing.T) {
	router := setupManagerTestRouter(t)
	if err := models.DB.AutoMigrate(&models.ProxyResource{}, &models.BrowserInstance{}, &models.Operation{}, &models.Task{}, &models.SystemConfig{}); err != nil {
		t.Fatalf("auto migrate browser tables: %v", err)
	}

	node := models.Node{ID: "node-1", DisplayName: "node-1", Hostname: "node-1", Token: "node-token", OnlineState: "online"}
	proxy := models.ProxyResource{ID: "proxy-1", Protocol: "socks5", Host: "127.0.0.1", Port: 1080}
	cfg := models.SystemConfig{Key: "default_tunnel_type", Value: "tun2socks", Description: "默认隧道类型", Category: "proxy"}
	models.DB.Create(&node)
	models.DB.Create(&proxy)
	models.DB.Create(&cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/browsers", strings.NewReader(`{"display_name":"browser-a","node_id":"node-1","proxy_id":"proxy-1"}`))
	req.SetBasicAuth("admin", "admin")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var browser models.BrowserInstance
	if err := models.DB.Where("node_id = ?", "node-1").First(&browser).Error; err != nil {
		t.Fatalf("load browser: %v", err)
	}
	if browser.TunnelContainerName == "" {
		t.Fatal("expected tunnel container name to be stored")
	}

	var task models.Task
	if err := models.DB.Where("type = ?", "start_browser").First(&task).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["tunnel_container_name"] == "" {
		t.Fatalf("expected tunnel_container_name in payload: %v", payload)
	}
	if payload["tunnel_type"] != "tun2socks" {
		t.Fatalf("unexpected tunnel_type: %s", payload["tunnel_type"])
	}
}

func TestProxyBrowserVNCUsesReverseProxyPath(t *testing.T) {
	router := setupManagerTestRouter(t)
	if err := models.DB.AutoMigrate(&models.BrowserInstance{}); err != nil {
		t.Fatalf("auto migrate browser table: %v", err)
	}

	node := models.Node{
		ID:          "node-1",
		DisplayName: "node-1",
		Hostname:    "node-1",
		Token:       "node-token",
		IP:          "192.227.229.63",
		OnlineState: "online",
	}
	browser := models.BrowserInstance{
		ID:            "browser-1",
		NodeID:        "node-1",
		ContainerName: "browser-demo",
		VNCPort:       1025,
		VNCPassword:   "changeme",
		Status:        "running",
	}
	models.DB.Create(&node)
	models.DB.Create(&browser)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/browsers/browser-1/vnc", nil)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["vnc_url"] != "/vnc/192.227.229.63/1025/?path=vnc%2F192.227.229.63%2F1025%2Fwebsockify" {
		t.Fatalf("unexpected vnc_url: %v", resp["vnc_url"])
	}
}

func TestBrowserCheckerGracePeriodKeepsNewBrowserPending(t *testing.T) {
	if err := models.DB.AutoMigrate(&models.Node{}, &models.BrowserInstance{}); err != nil {
		t.Fatalf("auto migrate browser checker tables: %v", err)
	}

	node := models.Node{
		ID:          "node-1",
		DisplayName: "node-1",
		Hostname:    "node-1",
		IP:          "192.0.2.10",
		OnlineState: "online",
	}
	browser := models.BrowserInstance{
		ID:          "browser-1",
		NodeID:      "node-1",
		Status:      "pending",
		VNCPort:     1027,
		LastError:   "",
	}
	models.DB.Create(&node)
	models.DB.Create(&browser)

	status, lastError := evaluateBrowserHealth(browser, node, browser.CreatedAt.Add(30*time.Second), func(network, addr string, timeout time.Duration) (net.Conn, error) {
		return nil, fmt.Errorf("dial failed")
	})

	if status != "pending" {
		t.Fatalf("status = %s", status)
	}
	if lastError != "" {
		t.Fatalf("lastError = %s", lastError)
	}
}

func TestBrowserCheckerMarksRunningBrowserErrorAfterGracePeriod(t *testing.T) {
	if err := models.DB.AutoMigrate(&models.Node{}, &models.BrowserInstance{}); err != nil {
		t.Fatalf("auto migrate browser checker tables: %v", err)
	}

	node := models.Node{
		ID:          "node-1",
		DisplayName: "node-1",
		Hostname:    "node-1",
		IP:          "192.0.2.10",
		OnlineState: "online",
	}
	browser := models.BrowserInstance{
		ID:        "browser-1",
		NodeID:    "node-1",
		Status:    "running",
		VNCPort:   1027,
		LastError: "",
	}
	models.DB.Create(&node)
	models.DB.Create(&browser)

	status, lastError := evaluateBrowserHealth(browser, node, browser.CreatedAt.Add(2*time.Minute), func(network, addr string, timeout time.Duration) (net.Conn, error) {
		return nil, fmt.Errorf("dial failed")
	})

	if status != "error" {
		t.Fatalf("status = %s", status)
	}
	if lastError != "VNC 端口不可达" {
		t.Fatalf("lastError = %s", lastError)
	}
}
