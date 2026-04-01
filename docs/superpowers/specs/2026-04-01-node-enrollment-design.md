# 节点接入设计

日期：2026-04-01

## 背景

当前系统中的节点由 `cmd/node` 进程启动后主动向 manager 发起 gRPC 注册，注册成功后才会出现在节点列表中。现有前端仅支持查看、编辑、删除节点，不支持引导其他服务器接入平台。

本次需求是在前端新增“添加节点”能力。管理员点击后生成一条安装指令，将其复制到其他 VPS 执行。远端安装完成并启动后，节点自动注册到 manager，随后出现在节点列表中。

## 目标

- 在节点管理页提供“添加节点”入口。
- 通过前端创建一次性节点接入授权，并返回一条可复制的安装命令。
- 安装命令可在常见 Linux 发行版上执行，自动安装依赖、部署 `node` 二进制并注册 `systemd` 服务。
- 节点首次注册必须携带一次性接入凭证，凭证 24 小时有效且只能使用一次。
- 节点只有在实际注册成功后才会出现在节点列表中。

## 非目标

- 不实现“手动预创建节点记录”。
- 不实现多次复用的长期接入密钥。
- 不实现安装命令再次查看明文密钥。
- 不实现 Windows 或 macOS 节点接入。

## 方案概述

采用“预授权接入”模型。manager 维护独立的节点接入授权记录 `node_enrollments`。前端创建授权后，后端返回包含授权 ID 和一次性密钥的安装命令。命令通过 manager 动态下发安装脚本，脚本自动在目标 VPS 上完成依赖安装、下载 `node` 二进制、写入环境文件、注册并启动 `systemd` 服务。

节点程序首次启动时，从环境变量读取 enrollment 信息并在 gRPC `Register` 请求中提交。manager 验证授权存在、未过期、未使用且密钥匹配后，创建或更新节点记录，同时将 enrollment 标记为已使用，然后返回正式 node token。后续心跳流程沿用现有机制。

## 数据模型

新增表 `node_enrollments`，字段如下：

- `id`：主键，UUID。
- `display_name`：管理员填写的节点备注，可为空。
- `manager_http_addr`：安装脚本使用的 manager HTTP 基地址。
- `manager_grpc_addr`：节点连接 manager 的 gRPC 地址，格式为 `host:port`。
- `token_hash`：一次性接入密钥哈希值，不保存明文。实现使用 `SHA-256`。
- `expires_at`：过期时间，创建后固定为 24 小时。
- `used_at`：实际消费时间，可为空。
- `used_by_node_id`：消费该授权的节点 ID，可为空。
- `created_at` / `updated_at`。

状态不单独存库，通过字段推导：

- `pending`：`used_at` 为空且当前时间未超过 `expires_at`
- `used`：`used_at` 非空
- `expired`：`used_at` 为空且当前时间已超过 `expires_at`

## HTTP 接口

### 1. 创建节点接入授权

`POST /api/v1/node-enrollments`

请求体：

```json
{
  "display_name": "tokyo-01",
  "manager_http_addr": "https://manager.example.com",
  "manager_grpc_addr": "manager.example.com:50051"
}
```

处理逻辑：

- 校验 `manager_http_addr` 非空且为合法 HTTP/HTTPS URL。
- 校验 `manager_grpc_addr` 非空。
- 生成 enrollment ID。
- 生成一次性随机明文 token。
- 保存 token 哈希。
- 设置 `expires_at = now + 24h`。
- 返回安装命令和过期时间。

响应体：

```json
{
  "data": {
    "id": "enroll_xxx",
    "expires_at": "2026-04-02T12:00:00Z",
    "install_command": "curl -fsSL 'https://manager.example.com/api/v1/node-enrollments/enroll_xxx/install.sh?token=plain_token' | sudo bash"
  }
}
```

约束：

- 明文 token 仅在创建成功时返回一次。
- 前端不应假设该命令可以再次查询。

### 2. 查询节点接入授权列表

`GET /api/v1/node-enrollments`

返回最近的授权记录，用于前端查看状态。返回字段包括：

- `id`
- `display_name`
- `manager_http_addr`
- `manager_grpc_addr`
- `expires_at`
- `used_at`
- `used_by_node_id`
- 推导后的 `status`
- `created_at`

该接口不返回明文 token，不返回完整安装命令。

### 3. 获取安装脚本

`GET /api/v1/node-enrollments/:id/install.sh?token=...`

处理逻辑：

- 校验 enrollment 存在。
- 校验 token 与哈希匹配。
- 校验 enrollment 未过期。
- 校验 enrollment 未使用。
- 通过后返回 shell 脚本文本。

该接口不会在返回脚本时将授权标记为已使用。真正的消费时机是节点完成首次注册，以避免仅下载脚本就耗尽一次性授权。

## gRPC 注册改造

扩展 `RegisterRequest`，新增字段：

- `display_name`
- `enrollment_id`
- `enrollment_token`

注册逻辑调整为：

1. 若未携带 enrollment 信息，拒绝注册。
2. 查询 `node_enrollments`。
3. 校验 enrollment 存在、未过期、未使用且 token 匹配。
4. 提取 gRPC peer IP 作为节点 IP。
5. 创建或更新 `nodes` 记录：
   - `id = req.node_id`
   - `display_name = req.display_name`，为空时回退到 enrollment `display_name`，仍为空时回退到 hostname
   - `hostname = req.hostname`
   - `ip = peer ip`
   - `version = req.version`
   - `capabilities = req.capabilities`
   - `online_state = online`
   - `last_heartbeat_at = now`
   - `token = 新生成的正式 node token`
6. 将 enrollment 标记为：
   - `used_at = now`
   - `used_by_node_id = req.node_id`
7. 返回正式 node token。

兼容性决策：

- 不保留匿名注册入口。
- 未携带 enrollment 的历史节点二进制将无法注册，需要重新按新方式安装。

## 安装脚本设计

安装脚本由 manager 动态生成，目标是在常见 Linux 发行版上尽可能稳定执行。脚本主要步骤如下：

1. 检测 root 权限。若当前用户非 root 且存在 `sudo`，则提示用户通过 `sudo bash` 执行。
2. 检测包管理器：
   - Debian/Ubuntu：`apt-get`
   - Fedora/RHEL 9+：`dnf`
   - CentOS/RHEL 7/8：`yum`
3. 安装最小依赖：
   - `curl`
   - `tar`
   - `docker`
   - `ca-certificates`
   - `systemd` 相关组件使用系统默认能力，不额外安装
4. 启动 Docker 并设置开机自启：
   - `systemctl enable --now docker`
5. 创建目录：
   - `/opt/mnp`
6. 从 manager 下载 node 二进制：
   - `${MANAGER_HTTP_ADDR}/download/node`
   - 保存为 `/opt/mnp/node`
   - `chmod +x /opt/mnp/node`
7. 写入环境文件 `/etc/mnp-node.env`：

```bash
MANAGER_IP=manager.example.com:50051
NODE_DISPLAY_NAME=tokyo-01
NODE_ENROLLMENT_ID=enroll_xxx
NODE_ENROLLMENT_TOKEN=plain_token
```

8. 写入 `systemd` 服务 `/etc/systemd/system/mnp-node.service`：

```ini
[Unit]
Description=MNP Node
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/mnp-node.env
ExecStart=/opt/mnp/node
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

9. 执行：
   - `systemctl daemon-reload`
   - `systemctl enable --now mnp-node`
10. 输出后续排查命令：

```bash
systemctl status mnp-node --no-pager
journalctl -u mnp-node -f
```

脚本兼容性策略：

- 优先支持存在标准包管理器和 `systemd` 的 Linux 发行版。
- 当前仅保证 `linux/amd64` 目标机器可用。
- 对无法识别的发行版直接失败，并输出“当前系统暂不支持自动安装”的错误。

## 前端设计

修改 `节点管理` 页面，保留当前节点列表，同时新增接入能力。

### 节点页顶部工具栏

新增按钮：

- `添加节点`

### 添加节点弹窗

表单字段：

- `节点备注`：可选
- `Manager HTTP 地址`：默认取当前访问来源
- `Manager gRPC 地址`：默认根据当前页面 host 推断并允许手动覆盖

表单下方展示固定说明：

- 安装命令仅展示一次，请及时复制。
- 授权 24 小时有效且只能使用一次。
- 目标服务器需支持 `systemd`。

### 创建成功结果态

同一弹窗切换为结果视图，展示：

- 过期时间
- 多行安装命令文本框
- 一键复制按钮
- 操作提示：在目标 VPS 上执行该命令，安装完成后节点会自动出现在列表

### 接入记录列表

在节点页中新增“接入记录”表格，字段：

- 备注名
- HTTP 地址
- gRPC 地址
- 状态
- 创建时间
- 过期时间
- 使用时间
- 已绑定节点

状态显示为：

- `待使用`
- `已使用`
- `已过期`

## 失败与错误处理

### 创建授权失败

- HTTP 或 gRPC 地址校验失败时返回 400。
- 前端展示后端错误消息，不生成命令。

### 安装脚本获取失败

- enrollment 不存在、已过期、已使用或 token 错误时返回 404/400。
- 输出简洁错误信息，避免泄露内部细节。

### 节点注册失败

- 缺少 enrollment 信息：返回“节点未授权接入”。
- token 不匹配：返回“接入授权无效”。
- 已过期：返回“接入授权已过期”。
- 已使用：返回“接入授权已被使用”。

### 脚本执行失败

- 依赖安装失败时直接退出。
- 下载二进制失败时直接退出。
- `systemd` 启动失败时提示查看 `journalctl -u mnp-node -f`。

## 测试策略

### 后端自动化测试

- 创建 enrollment 接口应返回安装命令和 24 小时后的过期时间。
- 查询 enrollment 列表应正确推导 `pending/used/expired` 状态。
- 安装脚本接口应拒绝：
  - 不存在的 enrollment
  - 错误 token
  - 已过期 enrollment
  - 已使用 enrollment
- gRPC 注册应拒绝：
  - 缺失 enrollment 的请求
  - 错误 token
  - 已过期 enrollment
  - 已使用 enrollment
- gRPC 注册成功后应：
  - 创建或更新 node
  - 标记 enrollment 为 used
  - 返回正式 node token

### 前端自动化测试

- 节点页应渲染“添加节点”按钮。
- 点击后应能打开弹窗。
- 提交成功后应展示安装命令和过期时间。
- 接入记录表格应展示状态标签。

### 手工验证

- 在测试 Linux 机器执行安装命令。
- 验证 `/opt/mnp/node`、`/etc/mnp-node.env`、`/etc/systemd/system/mnp-node.service` 被正确创建。
- 验证 `systemctl status mnp-node` 正常。
- 验证节点完成注册后在节点列表中出现。

## 实施范围

预计修改区域：

- `api/proto/platform.proto`
- `cmd/manager/grpc_server.go`
- `cmd/manager/http_server.go`
- `pkg/models/models.go`
- `pkg/models/db.go`
- `cmd/node/main.go`
- `web/src/views/NodeList.vue`

可选新增文件：

- manager 侧 enrollment 相关辅助函数或 handler 文件
- 前端测试文件

## 风险与注意事项

- 切换到强制 enrollment 注册后，旧版 node 将无法继续注册，需配套升级说明。
- 安装脚本依赖 manager 可通过公网访问 HTTP 下载二进制和脚本。
- `Manager HTTP 地址` 与 `Manager gRPC 地址` 由管理员填写，若填写内网地址，将导致外部 VPS 无法接入。
- 远端机器必须具备 Docker 运行条件，否则 node 无法执行后续任务。
- 当前方案默认远端节点为 `linux/amd64`，其他架构需要额外二进制产物支持。
