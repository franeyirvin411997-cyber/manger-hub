package workers

import (
	"fmt"
	"log"
	"net"
	"time"

	"multi_node_platform/pkg/models"
)

const browserReadyGracePeriod = 60 * time.Second

type browserDialFunc func(network, addr string, timeout time.Duration) (net.Conn, error)

func EvaluateBrowserHealth(browser models.BrowserInstance, node models.Node, now time.Time, dialFn browserDialFunc) (string, string) {
	if browser.VNCPort == 0 || node.IP == "" {
		return browser.Status, browser.LastError
	}
	if !browser.CreatedAt.IsZero() && now.Sub(browser.CreatedAt) < browserReadyGracePeriod {
		return browser.Status, browser.LastError
	}
	if dialFn == nil {
		dialFn = net.DialTimeout
	}

	addr := net.JoinHostPort(node.IP, fmt.Sprintf("%d", browser.VNCPort))
	conn, err := dialFn("tcp", addr, 5*time.Second)
	if err != nil {
		return "error", "VNC 端口不可达"
	}
	conn.Close()
	return "running", ""
}

func StartBrowserChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		var browsers []models.BrowserInstance
		models.DB.Where("status = ?", "running").Find(&browsers)
		for _, b := range browsers {
			if b.VNCPort == 0 {
				continue
			}
			var node models.Node
			if models.DB.Where("id = ?", b.NodeID).First(&node).Error != nil {
				continue
			}

			status, lastError := EvaluateBrowserHealth(b, node, time.Now(), nil)
			if status == "error" {
				models.DB.Model(&b).Updates(map[string]interface{}{
					"status":     status,
					"last_error": lastError,
				})
				addr := net.JoinHostPort(node.IP, fmt.Sprintf("%d", b.VNCPort))
				log.Printf("[BrowserChecker] 浏览器 %s 不可达: %s", b.ID, addr)
			}
		}
	}
}
