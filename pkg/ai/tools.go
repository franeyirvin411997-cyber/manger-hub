package ai

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"multi_node_platform/pkg/models"
	"multi_node_platform/pkg/utils"
)

func GetToolDefinitions() []Tool {
	return []Tool{
		mkTool("get_system_status", "获取系统总览统计信息", map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}),
		mkTool("list_nodes", "获取节点列表", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status": map[string]interface{}{"type": "string", "description": "筛选: online, offline, 或留空返回全部"},
			},
		}),
		mkTool("list_groups", "获取代理组列表", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"state":   map[string]interface{}{"type": "string", "description": "筛选状态: running, error, stopped, pending"},
				"node_id": map[string]interface{}{"type": "string", "description": "筛选节点 ID"},
			},
		}),
		mkTool("list_proxies", "获取代理资源列表", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pool_type": map[string]interface{}{"type": "string", "description": "formal, observer"},
				"status":    map[string]interface{}{"type": "string", "description": "online, offline, in_use"},
			},
		}),
		mkTool("list_accounts", "获取应用账号列表", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"app_identifier": map[string]interface{}{"type": "string"},
				"status":         map[string]interface{}{"type": "string"},
			},
		}),
		mkTool("list_browsers", "获取浏览器实例列表", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"node_id": map[string]interface{}{"type": "string"},
				"status":  map[string]interface{}{"type": "string"},
			},
		}),
		mkTool("restart_group", "重启指定代理组", map[string]interface{}{
			"type":     "object",
			"required": []string{"group_id"},
			"properties": map[string]interface{}{
				"group_id": map[string]interface{}{"type": "string", "description": "代理组 ID"},
			},
		}),
		mkTool("stop_group", "停止指定代理组", map[string]interface{}{
			"type":     "object",
			"required": []string{"group_id"},
			"properties": map[string]interface{}{
				"group_id": map[string]interface{}{"type": "string", "description": "代理组 ID"},
			},
		}),
		mkTool("replace_proxy", "为代理组更换代理", map[string]interface{}{
			"type":     "object",
			"required": []string{"group_id"},
			"properties": map[string]interface{}{
				"group_id": map[string]interface{}{"type": "string", "description": "代理组 ID"},
			},
		}),
	}
}

func mkTool(name, desc string, params interface{}) Tool {
	return Tool{Type: "function", Function: ToolDefinition{Name: name, Description: desc, Parameters: params}}
}

func ExecuteTool(name string, argsJSON string) string {
	var args map[string]interface{}
	json.Unmarshal([]byte(argsJSON), &args)

	switch name {
	case "get_system_status":
		return toolGetSystemStatus()
	case "list_nodes":
		return toolListNodes(strArg(args, "status"))
	case "list_groups":
		return toolListGroups(strArg(args, "state"), strArg(args, "node_id"))
	case "list_proxies":
		return toolListProxies(strArg(args, "pool_type"), strArg(args, "status"))
	case "list_accounts":
		return toolListAccounts(strArg(args, "app_identifier"), strArg(args, "status"))
	case "list_browsers":
		return toolListBrowsers(strArg(args, "node_id"), strArg(args, "status"))
	case "restart_group":
		return toolRestartGroup(strArg(args, "group_id"))
	case "stop_group":
		return toolStopGroup(strArg(args, "group_id"))
	case "replace_proxy":
		return toolReplaceProxy(strArg(args, "group_id"))
	default:
		return fmt.Sprintf("未知工具: %s", name)
	}
}

func strArg(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func toolGetSystemStatus() string {
	var nodesOnline, nodesTotal int64
	var proxiesFormal, proxiesObserver, proxiesInUse int64
	var groupsRunning, groupsError, groupsTotal int64
	var accountsActive, accountsBanned int64
	var browsersRunning int64
	var tasksPending int64

	models.DB.Model(&models.Node{}).Count(&nodesTotal)
	models.DB.Model(&models.Node{}).Where("online_state = ?", "online").Count(&nodesOnline)
	models.DB.Model(&models.ProxyResource{}).Where("pool_type = ?", "formal").Count(&proxiesFormal)
	models.DB.Model(&models.ProxyResource{}).Where("pool_type = ?", "observer").Count(&proxiesObserver)
	models.DB.Model(&models.ProxyResource{}).Where("status = ?", "in_use").Count(&proxiesInUse)
	models.DB.Model(&models.GroupRuntime{}).Where("current_state = ?", "running").Count(&groupsRunning)
	models.DB.Model(&models.GroupRuntime{}).Where("current_state = ?", "error").Count(&groupsError)
	models.DB.Model(&models.GroupRuntime{}).Count(&groupsTotal)
	models.DB.Model(&models.AppAccount{}).Where("status = ?", "active").Count(&accountsActive)
	models.DB.Model(&models.AppAccount{}).Where("status = ?", "banned").Count(&accountsBanned)
	models.DB.Model(&models.BrowserInstance{}).Where("status = ?", "running").Count(&browsersRunning)
	models.DB.Model(&models.Task{}).Where("status = ?", "pending").Count(&tasksPending)

	return toJSON(map[string]interface{}{
		"nodes_online": nodesOnline, "nodes_total": nodesTotal,
		"proxies_formal": proxiesFormal, "proxies_observer": proxiesObserver, "proxies_in_use": proxiesInUse,
		"groups_running": groupsRunning, "groups_error": groupsError, "groups_total": groupsTotal,
		"accounts_active": accountsActive, "accounts_banned": accountsBanned,
		"browsers_running": browsersRunning, "tasks_pending": tasksPending,
	})
}

func toolListNodes(status string) string {
	var nodes []models.Node
	q := models.DB
	if status != "" {
		q = q.Where("online_state = ?", status)
	}
	q.Find(&nodes)
	type nodeInfo struct {
		ID string `json:"id"`; Name string `json:"name"`; State string `json:"state"`; IP string `json:"ip"`
	}
	var result []nodeInfo
	for _, n := range nodes {
		result = append(result, nodeInfo{n.ID, n.DisplayName, n.OnlineState, n.IP})
	}
	return toJSON(result)
}

func toolListGroups(state, nodeID string) string {
	var runtimes []models.GroupRuntime
	q := models.DB
	if state != "" {
		q = q.Where("current_state = ?", state)
	}
	q.Find(&runtimes)
	type groupInfo struct {
		GroupID string `json:"group_id"`; State string `json:"state"`; Error string `json:"last_error"`; NodeID string `json:"node_id"`
	}
	var result []groupInfo
	for _, r := range runtimes {
		var spec models.GroupSpec
		if nodeID != "" {
			if models.DB.Where("id = ? AND node_id = ?", r.GroupID, nodeID).First(&spec).Error != nil {
				continue
			}
		} else {
			models.DB.Where("id = ?", r.GroupID).First(&spec)
		}
		result = append(result, groupInfo{r.GroupID, r.CurrentState, r.LastError, spec.NodeID})
	}
	return toJSON(result)
}

func toolListProxies(poolType, status string) string {
	var proxies []models.ProxyResource
	q := models.DB
	if poolType != "" { q = q.Where("pool_type = ?", poolType) }
	if status != "" { q = q.Where("status = ?", status) }
	q.Limit(50).Find(&proxies)
	type proxyInfo struct {
		ID string `json:"id"`; Host string `json:"host"`; Port int `json:"port"`; Pool string `json:"pool"`; Status string `json:"status"`
	}
	var result []proxyInfo
	for _, p := range proxies {
		result = append(result, proxyInfo{p.ID, p.Host, p.Port, p.PoolType, p.Status})
	}
	return toJSON(result)
}

func toolListAccounts(appID, status string) string {
	var accounts []models.AppAccount
	q := models.DB
	if appID != "" { q = q.Where("app_identifier = ?", appID) }
	if status != "" { q = q.Where("status = ?", status) }
	q.Find(&accounts)
	type accInfo struct {
		ID string `json:"id"`; Name string `json:"name"`; App string `json:"app"`; Status string `json:"status"`
	}
	var result []accInfo
	for _, a := range accounts {
		result = append(result, accInfo{a.ID, a.DisplayName, a.AppIdentifier, a.Status})
	}
	return toJSON(result)
}

func toolListBrowsers(nodeID, status string) string {
	var browsers []models.BrowserInstance
	q := models.DB
	if nodeID != "" { q = q.Where("node_id = ?", nodeID) }
	if status != "" { q = q.Where("status = ?", status) }
	q.Find(&browsers)
	type brwInfo struct {
		ID string `json:"id"`; Name string `json:"name"`; Node string `json:"node_id"`; Status string `json:"status"`; VNC int `json:"vnc_port"`
	}
	var result []brwInfo
	for _, b := range browsers {
		result = append(result, brwInfo{b.ID, b.DisplayName, b.NodeID, b.Status, b.VNCPort})
	}
	return toJSON(result)
}

func toolRestartGroup(groupID string) string {
	var spec models.GroupSpec
	if models.DB.Where("id = ?", groupID).First(&spec).Error != nil {
		return `{"error":"代理组不存在"}`
	}
	payloadMap := map[string]string{"group_id": groupID, "apps": spec.Apps}
	payloadBytes, _ := json.Marshal(payloadMap)
	op := models.Operation{ID: uuid.New().String(), Type: "restart_group", TargetID: groupID, Status: "pending", Operator: "ai"}
	task := models.Task{ID: uuid.New().String(), OperationID: op.ID, NodeID: spec.NodeID, Type: "restart_group", Payload: string(payloadBytes), Status: "pending", Timeout: 300}
	models.DB.Create(&op); models.DB.Create(&task)
	models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", groupID).Update("current_state", "pending")
	return fmt.Sprintf(`{"success":true,"message":"重启指令已下发","group_id":"%s"}`, groupID)
}

func toolStopGroup(groupID string) string {
	var spec models.GroupSpec
	if models.DB.Where("id = ?", groupID).First(&spec).Error != nil {
		return `{"error":"代理组不存在"}`
	}
	payloadMap := map[string]string{"group_id": groupID, "apps": spec.Apps}
	payloadBytes, _ := json.Marshal(payloadMap)
	op := models.Operation{ID: uuid.New().String(), Type: "stop_group", TargetID: groupID, Status: "pending", Operator: "ai"}
	task := models.Task{ID: uuid.New().String(), OperationID: op.ID, NodeID: spec.NodeID, Type: "stop_group", Payload: string(payloadBytes), Status: "pending", Timeout: 300}
	models.DB.Create(&op); models.DB.Create(&task)
	models.DB.Model(&models.GroupRuntime{}).Where("group_id = ?", groupID).Update("current_state", "stopped")
	return fmt.Sprintf(`{"success":true,"message":"停止指令已下发","group_id":"%s"}`, groupID)
}

func toolReplaceProxy(groupID string) string {
	var spec models.GroupSpec
	if models.DB.Where("id = ?", groupID).First(&spec).Error != nil {
		return `{"error":"代理组不存在"}`
	}
	var newProxy models.ProxyResource
	if models.DB.Where("status = ? AND pool_type = ?", "online", "formal").First(&newProxy).Error != nil {
		return `{"error":"无可用代理"}`
	}
	// 简化：标记代理
	models.DB.Model(&newProxy).Update("status", "in_use")
	auth := ""
	if newProxy.Username != "" && newProxy.Password != "" {
		auth = fmt.Sprintf("%s:%s@", newProxy.Username, newProxy.Password)
	}
	proxyURL := fmt.Sprintf("%s://%s%s:%d", newProxy.Protocol, auth, newProxy.Host, newProxy.Port)

	// 解析应用
	var appList []string
	json.Unmarshal([]byte(spec.Apps), &appList)
	var appConfigs map[string]string
	json.Unmarshal([]byte(spec.AppConfigs), &appConfigs)
	resolvedApps := resolveFromBindingsOrConfigs(spec, appList, appConfigs)
	resolvedBytes, _ := json.Marshal(resolvedApps)

	payloadMap := map[string]string{
		"group_id": groupID, "proxy_url": proxyURL,
		"resolved_apps": string(resolvedBytes), "apps": spec.Apps,
	}
	payloadBytes, _ := json.Marshal(payloadMap)
	op := models.Operation{ID: uuid.New().String(), Type: "replace_proxy", TargetID: groupID, Status: "pending", Operator: "ai"}
	task := models.Task{ID: uuid.New().String(), OperationID: op.ID, NodeID: spec.NodeID, Type: "replace_proxy", Payload: string(payloadBytes), Status: "pending", Timeout: 300}
	models.DB.Create(&op); models.DB.Create(&task)
	return fmt.Sprintf(`{"success":true,"message":"换代理指令已下发","new_proxy":"%s"}`, proxyURL)
}

// resolveFromBindingsOrConfigs 从 AccountBindings 或 AppConfigs 解析应用命令
func resolveFromBindingsOrConfigs(spec models.GroupSpec, appList []string, appConfigs map[string]string) []map[string]interface{} {
	// 尝试从 account bindings 获取凭证
	var bindings map[string]string
	json.Unmarshal([]byte(spec.AccountBindings), &bindings)

	mergedConfigs := make(map[string]string)
	for k, v := range appConfigs {
		mergedConfigs[k] = v
	}
	mergedConfigs["device"] = spec.ID
	mergedConfigs["group_id"] = spec.ID

	// 从账号解密凭证注入
	for appID, accountID := range bindings {
		var account models.AppAccount
		if models.DB.Where("id = ?", accountID).First(&account).Error == nil {
			if creds, err := utils.DecryptJSON(account.Credentials); err == nil {
				for k, v := range creds {
					mergedConfigs[k] = v
					mergedConfigs[appID+"_"+k] = v
				}
			}
		}
	}

	var resolved []map[string]interface{}
	for _, id := range appList {
		var tmpl models.AppTemplate
		if models.DB.Where("identifier = ?", id).First(&tmpl).Error != nil {
			resolved = append(resolved, map[string]interface{}{"identifier": id, "run_args": []string{"alpine", "sleep", "3600"}})
			continue
		}
		var argsTemplate []string
		json.Unmarshal([]byte(tmpl.CommandTemplate), &argsTemplate)
		var finalArgs []string
		for _, arg := range argsTemplate {
			a := arg
			for key, val := range mergedConfigs {
				a = replaceAll(a, "{{"+key+"}}", val)
				a = replaceAll(a, "{{"+id+"_"+key+"}}", val)
			}
			finalArgs = append(finalArgs, a)
		}
		resolved = append(resolved, map[string]interface{}{"identifier": id, "run_args": finalArgs})
	}
	return resolved
}

func replaceAll(s, old, new string) string {
	for {
		i := indexOf(s, old)
		if i < 0 { return s }
		s = s[:i] + new + s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub { return i }
	}
	return -1
}
