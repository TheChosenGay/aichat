---
name: aichat-backend-plan
description: 提供 aichat 项目 24 周后端成长计划，指导按周实现 WebSocket/IM/Agent/分布式。在规划下一步、实现功能、讨论路线图或问「本周/下一周做什么」时使用。
---

# aichat 后端成长计划

在 aichat 项目中推进功能或规划时，按本计划的 Phase 与周次执行；当前进度以项目内实际代码与文档为准。

## 路线图概览

| Phase | 周次 | 主题 | 核心交付 |
|-------|------|------|----------|
| **Phase 1** | 1-4 | 地基补强 | WS 连接、安全(JWT/bcrypt)、消息类型与存储、单聊 MVP |
| **Phase 2** | 5-10 | IM 核心 | 群聊、ACK、离线/历史、在线状态、API 规范、压测 |
| **Phase 3** | 11-16 | AI Agent | 大模型接入、记忆、工具调用、Planner、Agent 用户、RAG |
| **Phase 4** | 17-24 | 分布式 | Kafka、微服务拆分、docker-compose/Nginx、监控与收尾 |

## 当前状态（参考）

- **已完成**：Week 1–4（WsConn+心跳、bcrypt+JWT+黑名单、messages 表+MessageRouter、单聊收发+持久化）；登录已改为 email。
- **待定**：Week 5 单聊/群聊路由方案（客户端 `msgType` vs 服务端查 rooms 表）。
- **项目规则**：`.cursor/rules/`（project-overview、go-conventions、gateway-patterns、learning-notes）；笔记在 `docs/notes/`，ADR 在 `docs/adr/`。

## 按周任务速查

- **Week 1**：`gateway/ws/ws_conn.go` — 读/写循环、心跳（已完成）
- **Week 2**：bcrypt、JWT 固定 secret、受保护路由、WS 握手验签、Logout 黑名单（已完成）
- **Week 3**：`types/message.go`、messages 迁移、`store/message_db.go`（已完成）
- **Week 4**：WS 消息协议、单聊路由、持久化、ConnManager 并发安全（已完成）
- **Week 5**：`types/room.go`、rooms + room_members、`service/room.go`、群聊广播
- **Week 6**：ACK 帧、超时重推（最多 3 次）、`is_delivered`
- **Week 7**：离线拉取、`GET /message/history`、Redis 最近 100 条缓存
- **Week 8**：`online:{userId}` EX 60、心跳续期、`GET /user/status`
- **Week 9**：API 改 JSON Body、统一响应格式、单元测试
- **Week 10**：集成测试、1000 并发 WS 压测、pprof 优化
- **Week 11**：`agent/llm.go`、流式输出推 WS
- **Week 12**：`agent/memory.go`、Redis 对话历史、Token 窗口/摘要
- **Week 13**：`agent/tools.go`、Function Calling、工具调度循环
- **Week 14**：`agent/planner.go`、ReAct、`types/task.go`、tasks 表、任务 API
- **Week 15**：`IsAgent` 字段、多 Bot、`POST /agent/create`
- **Week 16**：向量库、`agent/rag.go`、基于文档的问答
- **Week 17-18**：Kafka、消息经 Kafka 再持久化+推送
- **Week 19-20**：user-service / message-service / gateway、gRPC
- **Week 21-22**：docker-compose、Nginx、Redis Pub/Sub 跨实例
- **Week 23-24**：Prometheus + Grafana、测试覆盖率 >70%、文档

## 关键文件（随 Phase 新增）

- Phase 1：`gateway/ws/ws_conn.go`、`types/message.go`、`store/message_db.go`
- Phase 2：`types/room.go`、`service/room.go`、`store/room_db.go`
- Phase 3：`agent/llm.go`、`agent/tools.go`、`agent/planner.go`、`agent/memory.go`、`agent/rag.go`
- 现有：`api/user.go`、`service/user.go`、`store/user_db.go`、`gateway/conn_manager.go`

## 使用方式

- 规划「下一周/下一阶段」：查上表对应周，结合 `reference.md` 的验收标准与任务细节。
- 实现某周功能：打开 `reference.md` 中该周的「项目任务」与「验收标准」逐项完成。
- 与用户对齐进度：先看代码与 `docs/notes/` 再对照本表，避免与已实现内容冲突。

## 详细内容

每周的技能目标、具体任务、验收标准、关键代码与后续升级说明见 **[reference.md](reference.md)**。
