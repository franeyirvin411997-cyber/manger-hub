package workers

import (
	"encoding/json"
	"log"
	"time"

	"github.com/Knetic/govaluate"
	"multi_node_platform/pkg/models"
)

// StartRuleEngine 启动自动化运维规则引擎
func StartRuleEngine() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		var rules []models.Rule
		models.DB.Where("is_enabled = ?", true).Find(&rules)

		if len(rules) == 0 {
			continue
		}

		evaluateProxies(rules)
		evaluateGroupRuntimes(rules)
	}
}

func evaluateProxies(rules []models.Rule) {
	var formalProxies []models.ProxyResource
	models.DB.Where("pool_type = ?", "formal").Find(&formalProxies)

	for _, proxy := range formalProxies {
		var metrics map[string]interface{}
		if proxy.QualityMetrics != "" {
			json.Unmarshal([]byte(proxy.QualityMetrics), &metrics)
		} else {
			metrics = make(map[string]interface{})
		}

		parameters := make(map[string]interface{})
		parameters["status"] = proxy.Status
		if lat, ok := metrics["latency_ms"]; ok {
			parameters["latency"] = lat
		} else {
			parameters["latency"] = 0.0
		}

		for _, rule := range rules {
			expression, err := govaluate.NewEvaluableExpression(rule.Condition)
			if err != nil {
				continue
			}

			result, err := expression.Evaluate(parameters)
			if err != nil {
				continue
			}

			if isMatch, ok := result.(bool); ok && isMatch {
				log.Printf("[RuleEngine] 规则命中: 代理 %s 命中规则 %s (动作: %s)", proxy.ID, rule.Name, rule.Action)
				executeProxyAction(proxy, rule.Action)
			}
		}
	}
}

func evaluateGroupRuntimes(rules []models.Rule) {
	var runtimes []models.GroupRuntime
	models.DB.Find(&runtimes)

	for _, rt := range runtimes {
		parameters := make(map[string]interface{})
		parameters["state"] = rt.CurrentState
		// 将运行了多久作为条件
		parameters["uptime_seconds"] = time.Since(rt.UpdatedAt).Seconds()

		for _, rule := range rules {
			expression, err := govaluate.NewEvaluableExpression(rule.Condition)
			if err != nil {
				continue
			}

			result, err := expression.Evaluate(parameters)
			if err != nil {
				continue
			}

			if isMatch, ok := result.(bool); ok && isMatch {
				log.Printf("[RuleEngine] 规则命中: 代理组 %s 命中规则 %s (动作: %s)", rt.GroupID, rule.Name, rule.Action)
				executeGroupAction(rt, rule.Action)
			}
		}
	}
}

// executeProxyAction 执行代理相关预设动作
func executeProxyAction(proxy models.ProxyResource, action string) {
	switch action {
	case "mark_offline":
		if proxy.Status != "offline" {
			models.DB.Model(&proxy).Update("status", "offline")
			log.Printf("[RuleEngine] 动作执行完毕: 已将代理 %s 标记为离线", proxy.ID)
		}
	case "demote_proxy":
		models.DB.Model(&proxy).Updates(map[string]interface{}{
			"pool_type": "observer",
			"status":    "unknown",
		})
		log.Printf("[RuleEngine] 动作执行完毕: 已将代理 %s 降级回观察池", proxy.ID)
	}
}

// executeGroupAction 执行代理组相关动作
func executeGroupAction(rt models.GroupRuntime, action string) {
	switch action {
	case "restart_app":
		if rt.CurrentState == "error" || rt.CurrentState == "stopped" {
			// 在这儿应该创建一个 restart_group 的 Task (省略详细 Payload 组装)
			log.Printf("[RuleEngine] 动作执行: 应该重启代理组 %s", rt.GroupID)
			models.DB.Model(&rt).Update("current_state", "pending")
		}
	}
}
