package models

import (
	"time"

	"gorm.io/gorm"
)

// Node 表示远端执行节点
type Node struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)" json:"id"` // 唯一 ID
	DisplayName      string    `gorm:"type:varchar(128)" json:"display_name"`  // 显示名称
	Hostname         string    `gorm:"type:varchar(128)" json:"hostname"`      // 主机信息
	Token            string    `gorm:"type:varchar(256)" json:"token"`         // 接入身份凭证
	Capabilities     string    `gorm:"type:text" json:"capabilities"`          // 节点能力，建议用 JSON 数组存储
	LastHeartbeatAt  time.Time `json:"last_heartbeat_at"`                      // 最近心跳时间
	OnlineState      string    `gorm:"type:varchar(32)" json:"online_state"`   // 在线状态投影: online, offline, unknown
	Version          string    `gorm:"type:varchar(64)" json:"version"`        // 版本信息
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// ProxyResource 表示代理资源（正式池、观察池）
type ProxyResource struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)" json:"id"` // 唯一 ID
	Protocol         string    `gorm:"type:varchar(32)" json:"protocol"`       // 协议类型: socks5, http 等
	Host             string    `gorm:"type:varchar(128)" json:"host"`          // 地址信息
	Port             int       `json:"port"`                                   // 端口
	Username         string    `gorm:"type:varchar(64)" json:"username"`       // 认证信息：用户名
	Password         string    `gorm:"type:varchar(64)" json:"password"`       // 认证信息：密码
	Source           string    `gorm:"type:varchar(64)" json:"source"`         // 来源
	PoolType         string    `gorm:"type:varchar(32)" json:"pool_type"`      // 所属池类型: formal, observer, deprecated
	QualityMetrics   string    `gorm:"type:text" json:"quality_metrics"`       // 质量指标，JSON
	Tags             string    `gorm:"type:text" json:"tags"`                  // 标签，JSON
	Status           string    `gorm:"type:varchar(32)" json:"status"`         // 可用性状态: online, offline, unknown
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// ProxyLease 表示代理资源分配关系
type ProxyLease struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)" json:"id"` // 唯一 ID
	ProxyResourceID  string    `gorm:"index;type:varchar(64)" json:"proxy_resource_id"` // 使用的代理
	GroupID          string    `gorm:"index;type:varchar(64)" json:"group_id"`          // 使用该代理的代理组
	StartedAt        time.Time `json:"started_at"`                                      // 分配开始时间
	EndedAt          *time.Time `json:"ended_at"`                                        // 分配结束时间（为空表示正在使用）
	IsValid          bool      `json:"is_valid"`                                        // 分配是否有效
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Rule 表示自动化运维规则
type Rule struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)" json:"id"` // 唯一 ID
	Name        string    `gorm:"type:varchar(128)" json:"name"`         // 规则名称
	Description string    `gorm:"type:varchar(256)" json:"description"`  // 规则描述
	Condition   string    `gorm:"type:text" json:"condition"`            // 触发条件表达式 (例如: latency > 500)
	Action      string    `gorm:"type:text" json:"action"`               // 触发动作 (例如: mark_offline)
	IsEnabled   bool      `json:"is_enabled"`                            // 是否启用
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GroupSpec 表示代理组的期望配置
type GroupSpec struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)" json:"id"` // 唯一 ID
	NodeID           string    `gorm:"index;type:varchar(64)" json:"node_id"` // 目标节点
	ProxyLeaseID     string    `gorm:"type:varchar(64)" json:"proxy_lease_id"` // 目标代理分配
	TunnelType       string    `gorm:"type:varchar(32)" json:"tunnel_type"`    // 隧道类型，如 gost
	Apps             string    `gorm:"type:text" json:"apps"`                  // 应用清单（JSON 数组）
	AppConfigs       string    `gorm:"type:text" json:"app_configs"`           // 应用配置（JSON）
	ExtConfigs       string    `gorm:"type:text" json:"ext_configs"`           // 扩展配置
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// GroupRuntime 表示代理组的运行投影（实际状态）
type GroupRuntime struct {
	GroupID          string    `gorm:"primaryKey;type:varchar(64)" json:"group_id"` // 与 GroupSpec 一对一
	CurrentState     string    `gorm:"type:varchar(32)" json:"current_state"`       // 当前状态: pending, deploying, running, partial, stopped, error, deleting
	TunnelState      string    `gorm:"type:varchar(32)" json:"tunnel_state"`        // 当前隧道状态
	AppStates        string    `gorm:"type:text" json:"app_states"`                 // 当前应用实例状态，JSON
	LastObservedAt   time.Time `json:"last_observed_at"`                            // 最近观测时间
	LastError        string    `gorm:"type:text" json:"last_error"`                 // 最近错误信息
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AppTemplate 表示应用模板（受支持的应用类型）
type AppTemplate struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)" json:"id"` // 唯一 ID
	Identifier       string    `gorm:"uniqueIndex;type:varchar(64)" json:"identifier"` // 应用标识
	DisplayName      string    `gorm:"type:varchar(128)" json:"display_name"`          // 展示名称
	DefaultImage     string    `gorm:"type:varchar(256)" json:"default_image"`         // 默认镜像或运行源
	SupportedConfigs string    `gorm:"type:text" json:"supported_configs"`             // 支持的配置项（JSON 数组，如 ["email", "password"]）
	CommandTemplate  string    `gorm:"type:text" json:"command_template"`              // 启动命令模板（JSON 数组模板，如 ["-e", "EMAIL={{email}}", "image:latest"]）
	DriverType       string    `gorm:"type:varchar(64)" json:"driver_type"`            // 运行驱动类型
	RiskRules        string    `gorm:"type:text" json:"risk_rules"`                    // 日志风控默认规则
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Operation 表示关键业务变更操作
type Operation struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)" json:"id"` // 唯一 ID
	Type             string    `gorm:"type:varchar(64)" json:"type"`           // 操作类型: create_group, replace_proxy, migrate_node 等
	TargetID         string    `gorm:"index;type:varchar(64)" json:"target_id"`// 目标对象 ID
	Status           string    `gorm:"type:varchar(32)" json:"status"`         // 状态: pending, running, success, failed
	ErrorMessage     string    `gorm:"type:text" json:"error_message"`         // 错误信息
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Task 表示发往节点执行端的具体动作
type Task struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)" json:"id"` // 唯一 ID
	OperationID      string    `gorm:"index;type:varchar(64)" json:"operation_id"` // 所属操作 ID
	NodeID           string    `gorm:"index;type:varchar(64)" json:"node_id"`      // 目标节点
	Type             string    `gorm:"type:varchar(64)" json:"type"`               // 任务类型
	Payload          string    `gorm:"type:text" json:"payload"`                   // 任务载荷 JSON
	Status           string    `gorm:"type:varchar(32)" json:"status"`             // 任务状态: pending, running, success, failed, canceled
	ScopeKey         string    `gorm:"type:varchar(128)" json:"scope_key"`         // 作用域键
	Timeout          int       `json:"timeout"`                                    // 超时时间(秒)
	RetryCount       int       `json:"retry_count"`                                // 重试次数
	ErrorMessage     string    `gorm:"type:text" json:"error_message"`             // 错误信息
	Result           string    `gorm:"type:text" json:"result"`                    // 任务结果 JSON
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
