package tgbot

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"multi_node_platform/pkg/ai"
	"multi_node_platform/pkg/models"
)

type Bot struct {
	token    string
	adminIDs []int64
	offset   int
}

func StartBot() {
	for {
		token := models.GetConfig("tgbot_token")
		if token != "" {
			bot := &Bot{token: token}
			bot.loadAdminIDs()
			log.Println("[TGBot] 启动 Telegram Bot")
			bot.poll()
		}
		time.Sleep(30 * time.Second)
	}
}

func (b *Bot) loadAdminIDs() {
	idsStr := models.GetConfig("tgbot_admin_ids")
	var ids []int64
	json.Unmarshal([]byte(idsStr), &ids)
	b.adminIDs = ids
}

func (b *Bot) isAdmin(userID int64) bool {
	if len(b.adminIDs) == 0 {
		return true // 未配置则放行
	}
	for _, id := range b.adminIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func (b *Bot) poll() {
	for {
		updates, err := b.getUpdates()
		if err != nil {
			log.Printf("[TGBot] 获取更新失败: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= b.offset {
				b.offset = u.UpdateID + 1
			}
			if u.Message != nil && u.Message.Text != "" {
				go b.handleMessage(u.Message)
			}
		}
		time.Sleep(1 * time.Second)
	}
}

func (b *Bot) handleMessage(msg *TGMessage) {
	if !b.isAdmin(msg.From.ID) {
		b.sendMessage(msg.Chat.ID, "⛔ 无操作权限")
		return
	}

	text := strings.TrimSpace(msg.Text)
	parts := strings.SplitN(text, " ", 2)
	cmd := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}

	var reply string
	switch cmd {
	case "/start":
		reply = "🤖 多节点流量运营平台 Bot\n\n可用命令:\n/status - 系统概览\n/nodes - 节点列表\n/groups - 代理组 (可加状态筛选)\n/accounts - 账号汇总\n/browsers - 浏览器列表\n/ai <指令> - AI 助手\n/help - 帮助"
	case "/help":
		reply = "📋 命令列表:\n/status - 系统总览\n/nodes - 节点状态\n/groups [error|running|stopped] - 代理组\n/accounts [app名] - 账号\n/browsers - 浏览器\n/restart <组ID> - 重启代理组\n/stop <组ID> - 停止代理组\n/ai <自然语言> - AI 操作"
	case "/status":
		reply = b.cmdStatus()
	case "/nodes":
		reply = b.cmdNodes()
	case "/groups":
		reply = b.cmdGroups(arg)
	case "/accounts":
		reply = b.cmdAccounts(arg)
	case "/browsers":
		reply = b.cmdBrowsers()
	case "/restart":
		reply = b.cmdRestartGroup(arg)
	case "/stop":
		reply = b.cmdStopGroup(arg)
	case "/ai":
		if arg == "" {
			reply = "用法: /ai <你的指令>"
		} else {
			reply = b.cmdAI(arg)
		}
	default:
		reply = "未知命令，输入 /help 查看帮助"
	}
	b.sendMessage(msg.Chat.ID, reply)
}

func (b *Bot) cmdStatus() string {
	var nodesOn, nodesAll int64
	var proxyFormal, proxyUse int64
	var grpRun, grpErr, grpAll int64
	var accActive, accBan int64
	var brwRun int64
	var taskPend int64

	models.DB.Model(&models.Node{}).Count(&nodesAll)
	models.DB.Model(&models.Node{}).Where("online_state = ?", "online").Count(&nodesOn)
	models.DB.Model(&models.ProxyResource{}).Where("pool_type = ?", "formal").Count(&proxyFormal)
	models.DB.Model(&models.ProxyResource{}).Where("status = ?", "in_use").Count(&proxyUse)
	models.DB.Model(&models.GroupRuntime{}).Count(&grpAll)
	models.DB.Model(&models.GroupRuntime{}).Where("current_state = ?", "running").Count(&grpRun)
	models.DB.Model(&models.GroupRuntime{}).Where("current_state = ?", "error").Count(&grpErr)
	models.DB.Model(&models.AppAccount{}).Where("status = ?", "active").Count(&accActive)
	models.DB.Model(&models.AppAccount{}).Where("status = ?", "banned").Count(&accBan)
	models.DB.Model(&models.BrowserInstance{}).Where("status = ?", "running").Count(&brwRun)
	models.DB.Model(&models.Task{}).Where("status = ?", "pending").Count(&taskPend)

	return fmt.Sprintf("📊 系统状态\n━━━━━━━━━━━━\n🖥 节点: %d/%d 在线\n🔗 代理: %d 使用中 / %d 正式池\n📦 代理组: %d 运行 / %d 异常 / %d 总\n👤 账号: %d 活跃 / %d 封禁\n🌐 浏览器: %d 运行\n⏳ 待执行: %d\n━━━━━━━━━━━━\n🕐 %s",
		nodesOn, nodesAll, proxyUse, proxyFormal, grpRun, grpErr, grpAll, accActive, accBan, brwRun, taskPend,
		time.Now().Format("2006-01-02 15:04:05"))
}

func (b *Bot) cmdNodes() string {
	var nodes []models.Node
	models.DB.Find(&nodes)
	if len(nodes) == 0 {
		return "暂无节点"
	}
	var sb strings.Builder
	sb.WriteString("🖥 节点列表:\n")
	for _, n := range nodes {
		icon := "🟢"
		if n.OnlineState != "online" {
			icon = "🔴"
		}
		sb.WriteString(fmt.Sprintf("%s %s (%s) %s\n", icon, n.DisplayName, n.ID[:8], n.IP))
	}
	return sb.String()
}

func (b *Bot) cmdGroups(state string) string {
	var runtimes []models.GroupRuntime
	q := models.DB
	if state != "" {
		q = q.Where("current_state = ?", state)
	}
	q.Limit(30).Find(&runtimes)
	if len(runtimes) == 0 {
		return "无匹配的代理组"
	}
	var sb strings.Builder
	sb.WriteString("📦 代理组:\n")
	for _, r := range runtimes {
		icon := "🟢"
		if r.CurrentState == "error" { icon = "🔴" }
		if r.CurrentState == "stopped" { icon = "⏹" }
		if r.CurrentState == "pending" { icon = "⏳" }
		errMsg := ""
		if r.LastError != "" { errMsg = " - " + r.LastError }
		sb.WriteString(fmt.Sprintf("%s %s [%s]%s\n", icon, r.GroupID[:8], r.CurrentState, errMsg))
	}
	return sb.String()
}

func (b *Bot) cmdAccounts(appFilter string) string {
	var accounts []models.AppAccount
	q := models.DB
	if appFilter != "" {
		q = q.Where("app_identifier = ?", appFilter)
	}
	q.Limit(30).Find(&accounts)
	if len(accounts) == 0 {
		return "暂无账号"
	}
	var sb strings.Builder
	sb.WriteString("👤 账号列表:\n")
	for _, a := range accounts {
		icon := "✅"
		if a.Status == "banned" { icon = "🚫" }
		if a.Status == "suspended" { icon = "⚠️" }
		sb.WriteString(fmt.Sprintf("%s %s [%s] %s\n", icon, a.DisplayName, a.AppIdentifier, a.Status))
	}
	return sb.String()
}

func (b *Bot) cmdBrowsers() string {
	var browsers []models.BrowserInstance
	models.DB.Limit(20).Find(&browsers)
	if len(browsers) == 0 {
		return "暂无浏览器实例"
	}
	var sb strings.Builder
	sb.WriteString("🌐 浏览器:\n")
	for _, br := range browsers {
		icon := "🟢"
		if br.Status != "running" { icon = "🔴" }
		sb.WriteString(fmt.Sprintf("%s %s [%s] VNC:%d\n", icon, br.DisplayName, br.Status, br.VNCPort))
	}
	return sb.String()
}

func (b *Bot) cmdRestartGroup(groupID string) string {
	if groupID == "" {
		return "用法: /restart <代理组ID>"
	}
	result := ai.ExecuteTool("restart_group", fmt.Sprintf(`{"group_id":"%s"}`, groupID))
	return "🔄 " + result
}

func (b *Bot) cmdStopGroup(groupID string) string {
	if groupID == "" {
		return "用法: /stop <代理组ID>"
	}
	result := ai.ExecuteTool("stop_group", fmt.Sprintf(`{"group_id":"%s"}`, groupID))
	return "⏹ " + result
}

func (b *Bot) cmdAI(message string) string {
	agent := ai.NewAgent()
	result, err := agent.Chat(message, "tgbot")
	if err != nil {
		return "❌ AI 错误: " + err.Error()
	}
	return "🤖 " + result.Response
}

// ============================================================
// Telegram API 简易封装 (避免引入额外依赖)
// ============================================================

type TGUpdate struct {
	UpdateID int        `json:"update_id"`
	Message  *TGMessage `json:"message"`
}

type TGMessage struct {
	Chat TGChat `json:"chat"`
	From TGUser `json:"from"`
	Text string `json:"text"`
}

type TGChat struct {
	ID int64 `json:"id"`
}

type TGUser struct {
	ID int64 `json:"id"`
}

func (b *Bot) getUpdates() ([]TGUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", b.token, b.offset)
	resp, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	var result struct {
		OK     bool       `json:"ok"`
		Result []TGUpdate `json:"result"`
	}
	json.Unmarshal(resp, &result)
	return result.Result, nil
}

func (b *Bot) sendMessage(chatID int64, text string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.token)
	body := map[string]string{
		"chat_id": strconv.FormatInt(chatID, 10),
		"text":    text,
	}
	bodyBytes, _ := json.Marshal(body)
	httpPost(url, bodyBytes)
}
