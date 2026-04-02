package models

import (
	"time"

	"gorm.io/gorm"
)

// ============================================================
// 基础设施层
// ============================================================

// Node 表示远端执行节点
type Node struct {
	ID              string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	DisplayName     string         `gorm:"type:varchar(128)" json:"display_name"`
	Hostname        string         `gorm:"type:varchar(128)" json:"hostname"`
	IP              string         `gorm:"type:varchar(64)" json:"ip"`    // 节点可达 IP（注册时上报）
	Token           string         `gorm:"type:varchar(256)" json:"-"`    // 不暴露给前端
	Capabilities    string         `gorm:"type:text" json:"capabilities"` // JSON 数组
	LastHeartbeatAt time.Time      `json:"last_heartbeat_at"`
	OnlineState     string         `gorm:"type:varchar(32)" json:"online_state"` // online, offline, unknown
	Version         string         `gorm:"type:varchar(64)" json:"version"`
	MaxGroups       int            `gorm:"default:50" json:"max_groups"` // 该节点最大部署组数
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// NodeEnrollment 表示节点一次性接入授权
type NodeEnrollment struct {
	ID              string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	DisplayName     string         `gorm:"type:varchar(128)" json:"display_name"`
	ManagerHTTPAddr string         `gorm:"type:varchar(255)" json:"manager_http_addr"`
	ManagerGrpcAddr string         `gorm:"type:varchar(255)" json:"manager_grpc_addr"`
	TokenHash       string         `gorm:"type:char(64)" json:"-"`
	ExpiresAt       time.Time      `json:"expires_at"`
	UsedAt          *time.Time     `json:"used_at"`
	UsedByNodeID    string         `gorm:"type:varchar(64)" json:"used_by_node_id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// ProxyResource 表示代理资源（观察池 → 正式池生命周期）
type ProxyResource struct {
	ID             string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	Protocol       string         `gorm:"type:varchar(32)" json:"protocol"`
	Host           string         `gorm:"type:varchar(128)" json:"host"`
	Port           int            `json:"port"`
	Username       string         `gorm:"type:varchar(64)" json:"username"`
	Password       string         `gorm:"type:varchar(64)" json:"-"` // API 脱敏
	Source         string         `gorm:"type:varchar(64)" json:"source"`
	PoolType       string         `gorm:"type:varchar(32)" json:"pool_type"` // formal, observer, deprecated
	QualityMetrics string         `gorm:"type:text" json:"quality_metrics"`
	Tags           string         `gorm:"type:text" json:"tags"`
	Status         string         `gorm:"type:varchar(32)" json:"status"` // online, offline, unknown, in_use
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// ProxyLease 代理资源分配关系
type ProxyLease struct {
	ID              string     `gorm:"primaryKey;type:varchar(64)" json:"id"`
	ProxyResourceID string     `gorm:"index;type:varchar(64)" json:"proxy_resource_id"`
	GroupID         string     `gorm:"index;type:varchar(64)" json:"group_id"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	IsValid         bool       `json:"is_valid"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ============================================================
// 编排层
// ============================================================

// GroupSpec 代理组期望配置
type GroupSpec struct {
	ID              string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	DisplayName     string         `gorm:"type:varchar(128)" json:"display_name"`
	NodeID          string         `gorm:"index;type:varchar(64)" json:"node_id"`
	ProxyLeaseID    string         `gorm:"type:varchar(64)" json:"proxy_lease_id"`
	ProxyResourceID string         `gorm:"type:varchar(64)" json:"proxy_resource_id"` // 冗余方便查询
	TunnelType      string         `gorm:"type:varchar(32)" json:"tunnel_type"`       // tun2socks, tun2proxy
	Apps            string         `gorm:"type:text" json:"apps"`                     // JSON 数组 app identifier
	AppConfigs      string         `gorm:"type:text" json:"app_configs"`              // 兼容旧格式
	AccountBindings string         `gorm:"type:text" json:"account_bindings"`         // JSON: {"app_id":"account_id",...}
	ExtConfigs      string         `gorm:"type:text" json:"ext_configs"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// GroupRuntime 代理组运行投影
type GroupRuntime struct {
	GroupID        string    `gorm:"primaryKey;type:varchar(64)" json:"group_id"`
	CurrentState   string    `gorm:"type:varchar(32)" json:"current_state"` // pending, deploying, running, partial, stopped, error, deleting
	TunnelState    string    `gorm:"type:varchar(32)" json:"tunnel_state"`
	AppStates      string    `gorm:"type:text" json:"app_states"`
	LastObservedAt time.Time `json:"last_observed_at"`
	LastError      string    `gorm:"type:text" json:"last_error"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ============================================================
// 应用与账号层
// ============================================================

// AppTemplate 应用模板
type AppTemplate struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	Identifier       string    `gorm:"uniqueIndex;type:varchar(64)" json:"identifier"`
	DisplayName      string    `gorm:"type:varchar(128)" json:"display_name"`
	DefaultImage     string    `gorm:"type:varchar(256)" json:"default_image"`
	SupportedConfigs string    `gorm:"type:text" json:"supported_configs"` // JSON 数组 ["email","password"]
	CommandTemplate  string    `gorm:"type:text" json:"command_template"`  // JSON 数组模板
	DriverType       string    `gorm:"type:varchar(64)" json:"driver_type"`
	RiskRules        string    `gorm:"type:text" json:"risk_rules"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AppAccount 应用平台账号（如某个 Honeygain 账号）
type AppAccount struct {
	ID            string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	AppIdentifier string         `gorm:"index;type:varchar(64)" json:"app_identifier"` // 关联 AppTemplate.Identifier
	DisplayName   string         `gorm:"type:varchar(128)" json:"display_name"`        // 如 "HG主号"
	Credentials   string         `gorm:"type:text" json:"-"`                           // AES 加密 JSON {"email":"x","password":"y"}
	Status        string         `gorm:"type:varchar(32)" json:"status"`               // active, suspended, banned, unknown
	Notes         string         `gorm:"type:text" json:"notes"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// AppAccountView 前端查看用（解密后脱敏展示）
type AppAccountView struct {
	AppAccount
	CredentialKeys []string `json:"credential_keys" gorm:"-"` // 只返回有哪些 key，不返回 value
	MaskedCreds    string   `json:"masked_credentials" gorm:"-"`
}

// AccountGroupBinding 账号与代理组中某 app 的绑定
type AccountGroupBinding struct {
	ID            string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	AccountID     string    `gorm:"index;type:varchar(64)" json:"account_id"`
	GroupID       string    `gorm:"index;type:varchar(64)" json:"group_id"`
	AppIdentifier string    `gorm:"type:varchar(64)" json:"app_identifier"`
	Status        string    `gorm:"type:varchar(32)" json:"status"` // bound, unbound
	CreatedAt     time.Time `json:"created_at"`
}

// ============================================================
// 浏览器层
// ============================================================

// BrowserInstance 远程浏览器实例
type BrowserInstance struct {
	ID                  string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	DisplayName         string         `gorm:"type:varchar(128)" json:"display_name"`
	NodeID              string         `gorm:"index;type:varchar(64)" json:"node_id"`
	ProxyResourceID     string         `gorm:"index;type:varchar(64)" json:"proxy_resource_id"` // 直接指定代理
	AccountID           string         `gorm:"index;type:varchar(64)" json:"account_id"`        // 绑定的账号
	ContainerName       string         `gorm:"type:varchar(128)" json:"container_name"`
	TunnelContainerName string         `gorm:"type:varchar(128)" json:"tunnel_container_name"`
	TunnelType          string         `gorm:"type:varchar(32)" json:"tunnel_type"`
	BrowserImage        string         `gorm:"type:varchar(256)" json:"browser_image"`
	VNCPort             int            `json:"vnc_port"`
	CDPPort             int            `json:"cdp_port"`
	VNCPassword         string         `gorm:"type:varchar(64)" json:"-"`
	Status              string         `gorm:"type:varchar(32)" json:"status"` // pending, running, stopped, error
	LastError           string         `gorm:"type:text" json:"last_error"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

// ============================================================
// 自动化规则与运维
// ============================================================

// Rule 自动化运维规则
type Rule struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	Name        string    `gorm:"type:varchar(128)" json:"name"`
	Description string    `gorm:"type:varchar(256)" json:"description"`
	Condition   string    `gorm:"type:text" json:"condition"`
	Action      string    `gorm:"type:text" json:"action"`
	IsEnabled   bool      `json:"is_enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Operation 关键业务变更操作
type Operation struct {
	ID           string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	Type         string    `gorm:"type:varchar(64)" json:"type"`
	TargetID     string    `gorm:"index;type:varchar(64)" json:"target_id"`
	Status       string    `gorm:"type:varchar(32)" json:"status"`
	ErrorMessage string    `gorm:"type:text" json:"error_message"`
	Operator     string    `gorm:"type:varchar(64)" json:"operator"` // admin, tgbot, ai
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Task 发往节点的执行动作
type Task struct {
	ID           string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	OperationID  string    `gorm:"index;type:varchar(64)" json:"operation_id"`
	NodeID       string    `gorm:"index;type:varchar(64)" json:"node_id"`
	Type         string    `gorm:"type:varchar(64)" json:"type"`
	Payload      string    `gorm:"type:text" json:"payload"`
	Status       string    `gorm:"type:varchar(32)" json:"status"`           // pending, running, success, failed, canceled
	ScopeKey     string    `gorm:"type:varchar(128);index" json:"scope_key"` // 防重复：group_id:action_type
	Timeout      int       `json:"timeout"`
	RetryCount   int       `json:"retry_count"`
	ErrorMessage string    `gorm:"type:text" json:"error_message"`
	Result       string    `gorm:"type:text" json:"result"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ============================================================
// 系统配置与 AI
// ============================================================

// SystemConfig 系统配置 KV 表
type SystemConfig struct {
	Key         string    `gorm:"primaryKey;type:varchar(128)" json:"key"`
	Value       string    `gorm:"type:text" json:"value"`
	Description string    `gorm:"type:varchar(256)" json:"description"`
	Category    string    `gorm:"type:varchar(64);index" json:"category"` // general, proxy, browser, tgbot, ai
	UpdatedAt   time.Time `json:"updated_at"`
}

// AIActionLog AI 操作日志
type AIActionLog struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	Source      string    `gorm:"type:varchar(32)" json:"source"` // web, tgbot
	UserInput   string    `gorm:"type:text" json:"user_input"`
	ParsedTools string    `gorm:"type:text" json:"parsed_tools"` // JSON: 解析出的工具调用
	Result      string    `gorm:"type:text" json:"result"`
	Status      string    `gorm:"type:varchar(32)" json:"status"` // success, failed, partial
	CreatedAt   time.Time `json:"created_at"`
}
