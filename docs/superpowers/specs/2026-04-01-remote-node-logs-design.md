# 远程节点日志查看设计

日期：2026-04-01

## 背景

当前系统中的节点由 `cmd/node` 进程在远端 VPS 上运行，并通过 gRPC 向 manager 注册与心跳。前端可以查看节点、代理组、浏览器等资源，但无法直接从管理后台查看远端节点服务日志或业务容器日志。排障时只能登录远程机器手工执行 `journalctl -u mnp-node -f` 或 `docker logs`，效率低且不适合多人协作。

本次需求是在管理后台一次性补齐完整的远程日志查看能力，支持通过 manager 中转查看：

- `mnp-node` 服务日志
- 业务容器日志：`tunnel-*`、`app-*`、`browser-*`

日志查看要求支持实时 tail，并在建立实时连接后异步补充最近 200 行历史日志。历史日志和实时日志必须分开显示，不能混排。

## 目标

- 在节点管理页新增“查看日志”入口。
- 前端只能通过 manager 中转访问日志，不能直连 node。
- 支持查看节点服务日志与业务容器日志。
- 打开日志窗口时先建立实时流，再异步返回最近 200 行历史日志。
- 实时日志与历史日志分别展示。
- 提供开始、停止、清空、重新连接操作。
- manager 与 node 之间使用受控的流式协议传输日志。
- node 侧仅允许执行白名单日志命令，不允许任意命令执行。

## 非目标

- 不做中心化日志存储。
- 不做全文检索、标签检索或历史分页。
- 不开放前端直连 node。
- 不支持任意 systemd 服务名。
- 不支持任意容器名或任意 shell 命令。
- 不支持节点离线时补看历史日志。

## 总体方案

采用单向中转架构：

`Browser -> Manager SSE -> Manager gRPC Stream -> Node`

核心思路：

1. 前端打开日志弹窗并向 manager 发起一个受鉴权保护的 SSE 请求。
2. manager 校验当前登录态、节点状态、日志源类型与参数合法性。
3. manager 使用数据库中保存的正式 node token 向目标 node 建立 gRPC 流式日志请求。
4. node 根据白名单日志源类型在本机执行固定日志命令：
   - 节点服务日志：`journalctl`
   - 容器日志：`docker logs`
5. node 先启动实时日志流，实时连接建立后先向 manager 回传 `live_connected` 状态。
6. node 再异步抓取最近 200 行历史日志并以 `history` 类型回传。
7. node 持续回传实时日志行给 manager。
8. manager 将 gRPC 日志块转换为 SSE 事件转发给前端。
9. 前端将实时日志和历史日志分开渲染，支持停止与重连。

## 日志源模型

日志源分两类：

### 1. 节点服务日志

- `source_type = node_service`
- 固定对应 `mnp-node`
- 不允许前端指定其他 systemd 单元名

### 2. 容器日志

- `source_type = container`
- 需要携带 `container_name`
- `container_name` 必须匹配白名单前缀之一：
  - `tunnel-`
  - `app-`
  - `browser-`

不允许：

- 自定义 shell 命令
- 自定义 systemd unit
- 非白名单容器名前缀

## 前端到 manager 接口

新增 SSE 接口：

`GET /api/v1/nodes/:id/logs/stream?source_type=node_service&container_name=...`

### 鉴权

- 该接口必须保留在 manager 后台登录鉴权之下。
- 使用当前现有 Basic Auth 会话头访问。

### 请求参数

- `source_type`
  - 必填
  - 枚举值：
    - `node_service`
    - `container`
- `container_name`
  - 当 `source_type=container` 时必填
  - 当 `source_type=node_service` 时忽略

### 响应格式

使用 `text/event-stream`。

事件类型固定为：

- `status`
- `live`
- `history`
- `error`

事件数据使用 JSON，字段如下：

```json
{
  "type": "live",
  "content": "2026-04-01T12:00:00Z 注册成功",
  "timestamp": "2026-04-01T12:00:00Z"
}
```

### 事件语义

- `status`
  - 系统状态提示
  - 例如：`connecting`、`live_connected`、`stream_closed`
- `live`
  - 实时日志行
- `history`
  - 最近 200 行历史日志
- `error`
  - 参数非法、节点离线、容器不存在、执行失败等错误

### manager 参数校验

manager 在建立日志流前必须完成这些校验：

1. 当前用户已认证。
2. 目标节点存在。
3. 目标节点状态为 `online`。
4. `source_type` 合法。
5. 若 `source_type=container`：
   - `container_name` 非空
   - `container_name` 通过白名单前缀校验

若校验失败，SSE 直接返回错误并结束。

## manager 到 node 协议

在 `api/proto/platform.proto` 中新增流式 RPC：

```proto
rpc StreamLogs(StreamLogsRequest) returns (stream LogChunk);
```

### StreamLogsRequest

字段设计：

- `node_id`
- `token`
- `source_type`
- `container_name`

语义：

- `node_id`
  - manager 请求的目标 node id
- `token`
  - manager 从数据库读取的正式 node token
- `source_type`
  - `node_service` / `container`
- `container_name`
  - 仅容器日志时使用

### LogChunk

字段设计：

- `stream_type`
- `content`
- `timestamp`

`stream_type` 枚举语义：

- `status`
- `live`
- `history`
- `error`

## node 端执行模型

node 收到 `StreamLogs` 请求后，按日志源类型执行白名单命令。

### 节点服务日志命令

实时流：

```bash
journalctl -u mnp-node -f -n 0 --no-pager
```

历史日志：

```bash
journalctl -u mnp-node -n 200 --no-pager
```

### 容器日志命令

实时流：

```bash
docker logs --tail 0 -f <container_name>
```

历史日志：

```bash
docker logs --tail 200 <container_name>
```

### 顺序要求

日志流顺序必须满足：

1. node 启动实时日志命令。
2. 成功建立实时输出后，先发送：
   - `status: live_connected`
3. node 异步抓取最近 200 行历史日志。
4. 历史日志以 `history` 类型逐行发送。
5. 实时日志以 `live` 类型持续逐行发送。

注意：

- 历史日志与实时日志不要求时间顺序混排。
- 前端依靠 `stream_type` 分区渲染，而不是在单区按时间混流。

### 生命周期管理

每个日志会话最多持有一个实时子进程。

必须处理这些退出路径：

- 浏览器关闭 SSE 连接
- manager 中断 gRPC 流
- node 本地命令退出
- 参数校验失败
- 节点内部错误

一旦会话结束，node 必须：

- 终止对应实时子进程
- 释放相关 goroutine / pipe 读取器

## 安全要求

### manager 侧

- 只允许后台已认证用户查看日志。
- 不允许将日志查看接口暴露为匿名接口。
- 不允许透传任意命令到 node。

### node 侧

- 不允许使用 shell 字符串拼接执行命令。
- 必须使用 `exec.Command` 传固定参数数组。
- `source_type` 必须做枚举校验。
- `container_name` 必须做白名单前缀校验。

### 容器名白名单

合法容器名必须满足至少一个前缀：

- `tunnel-`
- `app-`
- `browser-`

若不满足则直接返回错误，不尝试调用 `docker logs`。

## 前端设计

日志功能放在 `节点管理` 页面，不新开独立路由。

### 节点列表操作

节点表格操作列新增：

- `日志`

点击后打开日志弹窗。

### 日志弹窗结构

顶部区域：

- 日志源切换
  - `节点服务日志`
  - `容器日志`
- 容器名输入框
  - 仅当选中容器日志时显示
- 状态提示条
  - 例如：
    - `正在连接实时日志...`
    - `实时流已连接`
    - `日志流已停止`
    - `节点离线`

中部区域分两块：

1. `实时日志`
   - 持续追加
   - 自动滚动到底部

2. `最近 200 行`
   - 在实时流连接后异步填充
   - 仅展示快照，不持续追加

底部操作：

- `开始`
- `停止`
- `清空`
- `重新连接`

### 页面行为

- 打开弹窗后默认不自动开始，用户点击 `开始` 建立连接。
- 若日志源为 `node_service`，无需容器名。
- 若日志源为 `container`，容器名为空时禁用开始按钮。
- 点击 `停止` 时关闭当前 SSE 连接。
- 点击 `清空` 时清空实时区与历史区内容，但不自动断开连接。
- 点击 `重新连接` 时先断开旧流，再重新建立新流。

### 默认显示行为

当实时流已连接但历史日志为空时：

- 历史区显示：
  - `暂无最近历史日志`

当日志流断开时：

- 实时区停止追加
- 状态条显示：
  - `日志流已断开`

## 错误处理

### manager 层错误

- 节点不存在
- 节点离线
- 非法 `source_type`
- 缺失容器名
- 容器名不合法

处理方式：

- SSE 发送 `error` 事件
- 随后关闭连接

### node 层错误

- `journalctl` 不存在或执行失败
- `docker` 不存在
- 容器不存在
- 子进程启动失败
- 读取 stdout/stderr 失败

处理方式：

- 向 manager 发出 `error` 类型日志块
- 结束流

### 前端错误

- SSE 连接断开
- 网络错误
- manager 返回 error 事件

处理方式：

- 状态条显示错误
- 保留当前已收到的日志内容
- 用户可点击 `重新连接`

## 测试策略

### manager 测试

- SSE 参数校验：
  - 非法 `source_type`
  - 容器日志缺失 `container_name`
  - 非白名单容器名
- 节点离线时拒绝建立日志流
- manager 能正确把 node 返回的 `live/history/status/error` 转成 SSE 事件

### node 测试

- `node_service` 日志命令构造正确
- `container` 日志命令构造正确
- 非法容器名被拒绝
- 日志会话结束时实时子进程被清理

### 前端测试

- 节点页可打开日志弹窗
- 可切换日志源
- 容器日志模式下必须输入容器名
- 能分别渲染实时区和历史区
- 收到 `status/live/history/error` 事件时 UI 行为正确

## 受影响文件范围

预计修改：

- `api/proto/platform.proto`
- `api/pb/platform.pb.go`
- `api/pb/platform_grpc.pb.go`
- `cmd/manager/http_server.go`
- `cmd/manager/grpc_server.go`
- `cmd/node/main.go`
- `web/src/views/NodeList.vue`

预计新增：

- manager 侧日志流 HTTP / gRPC 处理文件
- node 侧日志执行与白名单处理文件
- 对应测试文件

## 风险与注意事项

- `journalctl` 依赖远端系统为 `systemd` 环境。
- 容器日志依赖远端机器已安装可用的 Docker CLI。
- SSE 连接时长较长，需要确认 nginx/代理层超时配置不会过早切断连接。
- 实时日志会持续占用 manager 与 node 之间的一个流式连接，必须确保连接关闭后资源释放。
- 历史日志与实时日志分区展示是设计要求，不能混排，否则“先实时、后历史”的顺序会破坏时间线理解。
