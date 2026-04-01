package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"multi_node_platform/pkg/models"
)

// StartProxyLifecycleChecker 定期检测代理池存活并晋升
func StartProxyLifecycleChecker() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		var proxies []models.ProxyResource

		// 找出待检测的代理：观察池的代理，或者状态未知的正式池代理
		models.DB.Where("pool_type = 'observer' OR status = 'unknown'").Find(&proxies)

		for _, proxy := range proxies {
			go checkAndPromoteProxy(proxy)
		}
	}
}

func checkAndPromoteProxy(proxy models.ProxyResource) {
	address := fmt.Sprintf("%s:%d", proxy.Host, proxy.Port)
	start := time.Now()

	// 简单 TCP Dial 检测 (首期测活实现)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)

	latency := time.Since(start).Milliseconds()

	metrics := map[string]interface{}{
		"latency_ms": latency,
	}

	var status string
	var poolType = proxy.PoolType

	if err != nil {
		status = "offline"
		metrics["error"] = err.Error()
	} else {
		conn.Close()
		status = "online"
		// 观察池测试通过则晋升为正式池
		if poolType == "observer" {
			poolType = "formal"
			log.Printf("[ProxyLifecycle] 代理 %s (%s) 测试通过，延迟 %d ms，晋升正式池", proxy.ID, address, latency)
		}
	}

	metricsBytes, _ := json.Marshal(metrics)

	// 更新代理状态
	models.DB.Model(&proxy).Updates(map[string]interface{}{
		"status":          status,
		"pool_type":       poolType,
		"quality_metrics": string(metricsBytes),
	})
}
