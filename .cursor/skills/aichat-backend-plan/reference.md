# aichat 后端成长计划 — 详细参考

本文档为 SKILL.md 的详细补充：每周技能目标、项目任务、验收标准及关键代码说明。

---

## Phase 1：地基补强（第 1-4 周）

### Week 1 — WebSocket 连接实现

**技能目标：** WebSocket 协议、gorilla/websocket、Go 读写分离 goroutine 模式

**项目任务：**
- 实现 `gateway/ws/ws_conn.go` 中的 `WsConn`，实现 `Conn` 接口
- 读循环（`readPump`）、写循环（`writePump`）
- 心跳（PingPong）
- `main.go` 中并行启动 HTTP 与 WS 网关

**WsConn 核心字段：** `conn *websocket.Conn`、`id string`、`send chan []byte`、`done chan struct{}`

**验收标准：** wscat 连 `/ws` 保持不断线，服务端有心跳日志

---

### Week 2 — 安全加固 + JWT 中间件

**技能目标：** bcrypt、HTTP 中间件链、JWT 固定 secret

**项目任务：**
- 密码：`CreateUser` 用 bcrypt 哈希，`LoginByPassword` 用 `bcrypt.CompareHashAndPassword`
- JWT：`.env` 中 `JWT_SECRET`，`utils/jwt.go` 用 `os.Getenv("JWT_SECRET")`，中间件不查 Redis、从 claims 取 userId
- JWT 挂到受保护路由（如 `/user/list`）
- WS 握手从 query 取 token 验证
- Logout：Redis 黑名单 `logout:token:{token}` EX 剩余有效期，验证时检查黑名单

**验收标准：** 无 token 访问 `/user/list` 返回 401，WS 无 token 401，密码为 `$2a$...`

**后续（Week 19）：** RS256、refresh token

---

### Week 3 — 消息类型 + 数据库迁移

**技能目标：** 数据建模、消息幂等、golang-migrate

**项目任务：**
- `types/message.go`：`Message`（Id, FromId, ToId, Type, Content, SendAt, IsDelivered）
- 迁移：`make migrate NAME=create_messages_table`
- `store/message_db.go`：`Save`、`ListByUserId`

**验收标准：** messages 表存在，能插入并查询

---

### Week 4 — 消息协议 + 单聊 MVP

**技能目标：** JSON 协议帧、并发安全 map

**项目任务：**
- WS 消息协议：`{"type":"chat","msgId":"uuid","toId":"userId","content":"hello"}`
- `gateway/ws/ws_server.go`：握手验 JWT → 收消息 → 路由到目标 `WsConn.Push()` → 持久化 MySQL
- `gateway/conn_manager.go`：map 用 `sync.RWMutex` 保护

**验收标准：** 两个客户端互发消息可收、MySQL 有记录

---

## Phase 2：IM 核心（第 5-10 周）

### Week 5 — 群聊/房间

**技能目标：** 多对多建模、关系表设计

**项目任务：**
- `types/room.go`（Room）
- 迁移：`rooms`、`room_members`
- `service/room.go`：创建房间、加入/退出、广播

**验收标准：** 建房间、3 人加入，任一人发消息其他人收到

---

### Week 6 — ACK 机制

**技能目标：** 端到端 ACK、channel 超时重试

**项目任务：**
- 客户端 ACK 帧：`{"type":"ack","msgId":"xxx"}`
- 服务端未 ACK 超时重推（最多 3 次）
- 更新 `is_delivered`

**验收标准：** 断连重连后未收消息被重推

---

### Week 7 — 离线消息 + 历史

**技能目标：** 离线策略、游标分页

**项目任务：**
- 上线后按 `SendAt` 拉取离线消息
- `GET /message/history?toId=xxx&before=timestamp&limit=20`
- Redis List 缓存最近 100 条（LPUSH + LTRIM）

**验收标准：** 重连收到离线消息，历史分页正常

---

### Week 8 — 在线状态

**技能目标：** Redis 过期键、多实例状态

**项目任务：**
- 上线：`SET online:{userId} 1 EX 60`，心跳续期
- 下线：删 key
- `IsOnline(userId)`；投递前查在线，离线入队

**验收标准：** `GET /user/status?userId=xxx` 正确

---

### Week 9 — API 规范化

**技能目标：** RESTful、JSON Body、统一响应

**项目任务：**
- 参数改为 JSON Body
- 统一 `{"code":0,"msg":"ok","data":{...}}`
- `service/error.go` 错误码、群组相关码
- 5 个关键接口单元测试

**验收标准：** 接口 JSON Body，覆盖率 > 60%

---

### Week 10 — 集成测试 + 压测

**技能目标：** httptest、WS 集成测试、pprof

**项目任务：**
- HTTP 集成测试（httptest）
- WS 集成测试（100 并发）
- pprof 找热点，优化 ConnManager 锁

**验收标准：** 1000 并发 WS 稳定、无 goroutine 泄漏

---

## Phase 3：AI Agent（第 11-16 周）

### Week 11 — 大模型 API

**技能目标：** HTTP 客户端、流式 SSE/Stream

**项目任务：**
- `agent/llm.go` 封装 DeepSeek/OpenAI，流式 `stream: true`，推 WS
- `.env.example` 增加 `LLM_API_KEY`

**验收标准：** 给 AI Bot 发消息收到流式回复

---

### Week 12 — 对话记忆

**技能目标：** 对话历史、Token 窗口、Redis 持久化

**项目任务：**
- `agent/memory.go`：每 session 最近 N 轮
- Redis `agent:memory:{userId}` (List)
- 超 Token 时 LLM 摘要压缩

**验收标准：** 多轮对话 AI 能记住前文

---

### Week 13 — Function Calling

**技能目标：** OpenAI Function Calling、工具定义

**项目任务：**
- `agent/tools.go`：`send_message`、`create_reminder`、`query_user`
- 循环：tool_call → 执行 → 结果回 LLM → 最终回复

**验收标准：** 「帮我给张三发消息说我晚点到」能自动发送

---

### Week 14 — 任务规划器（ReAct）

**技能目标：** ReAct、任务拆解

**项目任务：**
- `agent/planner.go`：ReAct（Thought → Action → Observation）
- `types/task.go`，status: pending/running/done/failed
- 迁移 `tasks` 表
- `GET /task/list`、`GET /task/{id}/status`

**验收标准：** 「调研竞品并总结报告」被拆成多步执行

---

### Week 15 — Agent 用户系统

**技能目标：** Bot 账号、事件驱动

**项目任务：**
- `types/user.go` 增加 `IsAgent`，迁移
- Agent 用户收消息走 Agent 流程
- 多 Bot（通用/任务/代码），`POST /agent/create`（system prompt + 工具集）

**验收标准：** 多 Bot、行为与工具集独立

---

### Week 16 — RAG 基础

**技能目标：** 向量库、向量化、相似度检索

**项目任务：**
- 向量库（Qdrant/pgvector，Docker）
- `agent/rag.go`：向量化存储 + 检索
- Bot 可答「基于上传文档」的问题

**验收标准：** 上传 PDF 后 AI 能答文档内问题

---

## Phase 4：分布式（第 17-24 周）

### Week 17-18 — Kafka 解耦

**技能目标：** Kafka Topic/Partition/Consumer Group

**项目任务：**
- Docker 起 Kafka
- 流程：WS 收消息 → Kafka → Consumer 持久化 + 推送

**验收标准：** 消息经 Kafka 正常收发，重启 Consumer 能消费积压

---

### Week 19-20 — 微服务拆分

**项目任务：**
- user-service、message-service、gateway
- 服务间 gRPC（.proto）

**验收标准：** 三服务独立部署、调用链正常

---

### Week 21-22 — 高可用部署

**项目任务：**
- `docker-compose.yml`（MySQL、Redis、Kafka 等）
- Nginx 反向代理 + WS 代理
- 多实例 gateway 用 Redis Pub/Sub 跨实例路由

**验收标准：** `docker-compose up` 全量启动，gateway 2 实例路由正常

---

### Week 23-24 — 监控与收尾

**项目任务：**
- Prometheus 指标（连接数、QPS、P99）
- Grafana Dashboard
- 测试覆盖率 > 70%
- README：架构图、接口文档、部署说明

**验收标准：** Grafana 实时指标，5000 并发 WS 稳定

---

## 关键文件一览

- 现有：`api/user.go`、`service/user.go`、`store/user_db.go`、`gateway/conn_manager.go`
- Phase 1：`gateway/ws/ws_conn.go`、`types/message.go`、`store/message_db.go`
- Phase 2：`types/room.go`、`service/room.go`、`store/room_db.go`
- Phase 3：`agent/llm.go`、`agent/tools.go`、`agent/planner.go`、`agent/memory.go`、`agent/rag.go`

## 工作量参考

- 学习 5–8 h/周，编码 8–12 h/周，合计 15–20 h/周；建议工作日 2 h、周末 4–6 h 集中验收。
