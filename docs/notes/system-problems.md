# 系统问题清单与解决方案

> 这些是当前 aichat 项目中真正有挑战性的工程问题，比增删改查有意思得多。

---

## 问题一：Pending 消息重启丢失

### 现状

`defaultMessagePender` 是纯内存结构：

```go
// service/message.go
type defaultMessagePender struct {
    msgList map[string]*PendMessage  // 内存 map，重启清零
}
```

服务重启后，所有等待 ACK 的消息全部丢失，发送方不会收到 Failed 通知，接收方也收不到消息重试。

### 问题本质

这是**可靠性**问题：如何保证消息"至少送达一次"？

### 解决方案：用 Redis 持久化 Pending 队列

**数据结构设计**：

```
Redis Hash：pending:messages
  key:   msg_id
  value: JSON { msg, retry_count, last_retry_at, pend_at }

Redis ZSet：pending:timeout_index
  member: msg_id
  score:  expire_at (Unix 时间戳)
  用途：按过期时间排序，定时扫描时只取 score <= now 的消息
```

**流程变化**：

```
Pend(msg):
  1. HSET pending:messages {msg_id} {序列化消息}
  2. ZADD pending:timeout_index {expire_at} {msg_id}

UnPend(msgId):  // ACK 时调用
  1. HDEL pending:messages {msg_id}
  2. ZREM pending:timeout_index {msg_id}

clearUp 定时器:
  1. ZRANGEBYSCORE pending:timeout_index 0 {now}  ← 拿到所有超时消息
  2. 对每条消息执行 重试 或 失败 逻辑
  3. 重试：更新 retry_count，重置 score
  4. 失败：HDEL + ZREM，发送 Failed 通知
```

**好处**：
- 服务重启后，pending 消息仍在 Redis，重启后可以继续重试
- 为多进程部署打基础（多个实例共享同一个 pending 队列）

**需要注意**：
- 多实例时，需要用分布式锁（`SETNX`）防止多个实例同时处理同一条消息
- 消息结构需要可序列化（JSON），`types.Message` 需要加 JSON tag

---

## 问题二：unread_count 并发正确性

### 现状

当前 SQL：
```sql
-- store/conversation_db.go
ON DUPLICATE KEY UPDATE
unread_count = unread_count + VALUES(unread_count)
```

**这行 SQL 本身是原子的**，单条 SQL 的并发没有问题。

但问题出在 `updateConversation` 的流程：

```go
// service/message.go:260-296
conversation, err = s.conversationService.GetConversationByUserIdAndPeerId(...)
// ← 此处读出 unread_count = 3

// ... 中间有 userService.GetById() 的 IO 耗时 ...

conversation.UnreadCount = unreadCnt  // unreadCnt = 1（增量）
s.conversationService.UpdateConversation(conversation)
// ← upsert 时传入 unread_count = 1，数据库执行 3 + 1 = 4  ✓
```

**实际上是正确的**，因为传入的是增量（0 或 1），数据库层做累加，不存在"读-改-写"的竞态。

**但有一个隐患**：如果未来有人把 `conversation.UnreadCount = unreadCnt` 改成了
`conversation.UnreadCount = conversation.UnreadCount + unreadCnt`（看起来更"直觉"），
就会变成"读旧值 + 1"然后覆盖写，并发时就会出现丢失更新问题。

### 解决方案：让接口语义更清晰，杜绝误用

把 upsert 接口的语义明确化，不传整个 Conversation 对象，而是传明确的"增量"：

```go
// 更清晰的接口设计
type ConversationRepository interface {
    // 更新会话的最新消息信息，unreadDelta 是增量（0 或 1）
    UpdateLastMessage(userId, peerId, roomId string, msg *LastMessageInfo, unreadDelta int) error
}

type LastMessageInfo struct {
    MsgId      string
    Content    string
    SenderName string
    SendAt     int64
}
```

调用方不再持有 Conversation 对象，无法意外地做"读-改-写"。

---

## 问题三：消息与会话的一致性问题

### 现状与本质

```go
// service/message.go
go func() {
    s.updateConversation(...)  // 错误被静默丢弃
}()
```

表面上是"goroutine 错误没有处理"，但根本问题是**跨两张表的数据一致性**：

```
消息保存（messages 表）  ✓ 成功
会话更新（conversations 表）  ✗ 失败

→ 消息存在，但用户在会话列表里永远看不到这条消息
```

重试只能解决"临时错误"，解决不了"始终失败"的情况。
真正的问题是：**消息保存和会话更新这两个写操作不是原子的**。

### 理解会话数据的本质

`conversations` 表存的是从 `messages` 派生出来的"摘要视图"。
任何时候只要有消息记录，会话就可以被重建。这是解决一致性的关键。

### 一致性类型分析

**Outbox 模式 = 最终一致性**

消息保存成功后，会话更新由 Worker 异步执行，中间存在时间差（毫秒到秒级）。
在这个窗口期内，消息已存在但会话列表尚未更新，两者处于短暂不一致状态。

**如果要强一致性 = 同一事务**

```go
tx, _ := s.db.Begin()
defer tx.Rollback()
// 1. 写消息
messageStore.InsertTx(tx, message)
// 2. 写发送方会话
conversationStore.UpsertTx(tx, senderConv)
// 3. 写接收方会话
conversationStore.UpsertTx(tx, receiverConv)
tx.Commit() // 原子提交，要么全成功，要么全失败
```

| | 强一致（同事务） | 最终一致（Outbox） |
|---|---|---|
| 消息+会话是否同时可见 | 是，原子可见 | 否，有延迟 |
| 实现复杂度 | 低（一个事务） | 高（Worker + outbox 表） |
| 性能 | 主链路慢（锁更多行） | 主链路快，异步处理 |
| 跨服务/跨库 | **不支持** | 支持 |
| 适合场景 | 单体应用、同一数据库 | 微服务、分布式 |

**本项目的选择**：

当前是单体应用，messages 和 conversations 在同一 MySQL 实例，**同事务强一致性实现更简单且正确**。

但项目规划未来会**拆分 message 服务和 conversation 服务**，跨服务后无法共用同一事务，
届时必须切换到 Outbox 模式（最终一致性）。所以 Outbox 方案是最终形态的正确方向。

---

### 标准解法：Outbox 模式

**核心思想**：把"需要做的事"和"主数据"放在同一个事务里，保证两者原子写入，
再由独立 worker 异步消费任务。

**新增 `message_outbox` 表**：

```sql
CREATE TABLE message_outbox (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    msg_id      VARCHAR(36) NOT NULL,
    payload     JSON NOT NULL,      -- 序列化的 Message
    status      TINYINT DEFAULT 0,  -- 0=待处理 1=完成
    retry_count INT DEFAULT 0,
    created_at  BIGINT NOT NULL,
    INDEX idx_status_created (status, created_at)
);
```

**流程**：

```
SendMessage():
  BEGIN TRANSACTION
    INSERT INTO messages (...)         ← 保存消息（事实）
    INSERT INTO message_outbox (...)   ← 记录待处理任务
  COMMIT
  ← 两者要么都成功，要么都失败，永远不会出现"消息有、任务没有"

OutboxWorker（后台每秒运行）：
  SELECT * FROM message_outbox WHERE status=0 LIMIT 10
  FOR EACH record:
    执行 conversations upsert
    成功 → DELETE FROM message_outbox WHERE id=?
    失败 → retry_count+1，等待下次扫描重试
    retry_count > 5 → 打告警日志，人工介入
```

**与现在"goroutine 异步更新"的本质区别**：

| | 现在（goroutine） | Outbox 模式 |
|---|---|---|
| 消息和任务是否原子写入 | 否 | 是（同一事务） |
| 重启后任务是否丢失 | 是 | 否（持久化在 DB） |
| 失败后是否可重试 | 否 | 是（worker 重扫） |

### 实现要点

**store/message_db.go** — Save 改为接受事务：
```go
func (s *MessageDbStore) SaveWithOutbox(tx *sql.Tx, message *types.Message) error {
    _, err := tx.Exec(InsertMessageSql, ...)
    if err != nil { return err }
    payload, _ := json.Marshal(message)
    _, err = tx.Exec(
        `INSERT INTO message_outbox (msg_id, payload, status, created_at) VALUES (?, ?, 0, ?)`,
        message.MsgId, payload, time.Now().Unix(),
    )
    return err
}
```

**service/message.go** — SendMessage 用事务：
```go
tx, _ := s.db.Begin()
defer tx.Rollback()
s.messageStore.SaveWithOutbox(tx, message)
tx.Commit()
// 事务外路由消息
s.router.Route(message)
```

**service/outbox_worker.go** — 独立 worker：
```go
type OutboxWorker struct {
    outboxStore OutboxStore
    convService ConversationService
}

func (w *OutboxWorker) Start() {
    go func() {
        ticker := time.NewTicker(time.Second)
        for range ticker.C {
            w.process()
        }
    }()
}

func (w *OutboxWorker) process() {
    records, _ := w.outboxStore.FetchPending(10)
    for _, r := range records {
        var msg types.Message
        json.Unmarshal(r.Payload, &msg)
        if err := w.updateConversation(&msg); err != nil {
            w.outboxStore.IncrRetry(r.Id)
            slog.Error("outbox failed", "msg_id", r.MsgId, "retry", r.RetryCount)
        } else {
            w.outboxStore.Delete(r.Id)
        }
    }
}
```

### 会话数据的自愈能力

有了 outbox 之后，还可以加一个"兜底修复"接口：
当客户端发现会话列表数据不对时，可以主动触发重建，
从 `messages` 表重新聚合当前用户的所有会话。
这利用了"会话是消息的派生数据"这个特性，是最终的保底手段。

---

### 其他最终一致性方案

除了 Outbox，还有以下方案可以实现最终一致性：

#### 方案二：CDC（Change Data Capture）

**思路**：监听数据库 binlog，捕获 messages 表的变更事件，驱动会话更新。

```
MySQL binlog → Debezium/Canal → Kafka → ConversationService consumer
```

**与 Outbox 的核心区别**：
- Outbox：业务代码主动写任务，有侵入性
- CDC：基础设施层监听变更，**业务代码零侵入**，消息服务完全不需要知道会话服务的存在

**优点**：
- 业务代码不感知，服务间解耦最彻底
- 天然捕获所有变更，不会遗漏

**缺点**：
- 引入 Debezium/Canal + Kafka，基础设施复杂度高
- binlog 格式依赖数据库版本，运维成本高

---

#### 方案三：MQ 事务消息

**思路**：发消息时同时发一条 MQ 事件，ConversationService 订阅消费。

普通 MQ 有同样的原子性问题：
```
写 messages 成功
发 MQ 事件失败  ← 两者不原子，和 goroutine 一样
```

**RocketMQ 事务消息**解决了这个问题：
```
1. 发 half message（预发送，消费者不可见）
2. 写 messages 表
3. 成功 → commit → MQ 消息变为可消费
   失败 → rollback → MQ 丢弃 half message
4. 若长时间未 commit/rollback → MQ 主动回查业务方（checkLocalTransaction）
```

**优点**：
- 天然支持微服务，ConversationService 完全解耦
- 已有 MQ 基础设施时零额外成本

**缺点**：
- 强依赖 RocketMQ（Kafka 没有事务消息语义）
- 需要实现 `checkLocalTransaction` 回查逻辑

---

#### 方案四：Saga 模式

**思路**：把跨服务操作拆成一系列本地事务，每步失败有对应补偿操作。

```
Step 1: MessageService.SaveMessage()        ✓ → 继续
Step 2: ConversationService.Update()        ✗ → 触发补偿
Compensate: MessageService.DeleteMessage()  ← 回滚第 1 步
```

两种实现方式：
- **Choreography（编排）**：每个服务监听事件，自己决定下一步，去中心化
- **Orchestration（协调）**：中央协调器控制整个流程

**缺点**：
- 补偿逻辑复杂，"会话更新失败就删消息"语义上非常奇怪
- 不适合消息/会话这类场景，更适合订单、支付等有明确回滚语义的流程

---

### 方案对比总结

| 方案 | 侵入性 | 基础设施复杂度 | 适合场景 |
|------|--------|--------------|---------|
| 同一事务（强一致） | 低 | 无 | 单体应用、同库 |
| Outbox 模式 | 中（写 outbox 表） | 低（只要 DB） | 中小规模微服务 |
| CDC | 零 | 高（binlog + Kafka） | 大规模，零侵入要求 |
| MQ 事务消息 | 中（MQ SDK） | 中（RocketMQ） | 已有 MQ 基础设施 |
| Saga | 高（补偿逻辑） | 中高 | 长流程业务，有回滚语义 |

**本项目演进路径**：

```
现在（单体）           →   拆服务初期        →   成熟阶段
同一事务（强一致）     →   Outbox 模式       →   CDC + Kafka
                           成本最低，够用        基础设施成熟后迁移
```

---

## 问题四：历史消息拉取设计错误

### 现状

```go
// service/message.go:248-257
func (s *MessageService) FetchHistoryMessages(toId string, limit int, currentTime int64) error {
    messages, err := s.messageStore.FetchHistoryMessages(toId, currentTime, limit)
    // ...
    for _, message := range messages {
        s.router.Route(message)  // ← 把历史消息通过 WebSocket 推送出去
    }
    return nil
}
```

**根本问题**：历史消息拉取用的是"推"的模型（WebSocket Route），但它应该是"拉"的模型（HTTP 请求-响应）。

**当前的问题**：
1. 客户端无法知道消息什么时候推完（没有结束信号）
2. 消息和实时消息混在同一个 WebSocket 通道，客户端需要自己区分
3. 历史消息无法分页（当前实现拉出来就全推了）
4. 查询条件写死了 `to_id`，群聊历史消息无法拉取
5. `FetchHistoryMessages` 返回 `error` 但调用方拿不到消息列表，设计矛盾

### 解决方案：改为 HTTP 接口

历史消息拉取是典型的**请求-响应**模式，应该是 REST API：

```
GET /message/history?peer_id=xxx&cursor=0&limit=20
GET /message/history?room_id=xxx&cursor=0&limit=20
```

**响应结构**：
```json
{
  "code": 0,
  "data": {
    "messages": [...],
    "next_cursor": 1234567880,
    "has_more": true
  }
}
```

**Store 层修改**：

```go
// 当前只支持单聊，需要扩展
const FetchHistoryMessagesSql = `
SELECT msg_id, from_id, to_id, type, content, send_at, is_delivered, room_id
FROM messages
WHERE (
    (to_id = ? AND from_id = ?) OR   -- 单聊：双向查询
    (to_id = ? AND from_id = ?)
)
AND send_at < ?
ORDER BY send_at DESC
LIMIT ?
`

// 群聊
const FetchRoomHistoryMessagesSql = `
SELECT ... FROM messages
WHERE room_id = ? AND send_at < ?
ORDER BY send_at DESC LIMIT ?
`
```

**`FetchHistoryMessages` 原方法**：可以删除，或保留供内部使用但不走 WebSocket 路由。

---

## 总结：优先级排序

| 问题 | 影响 | 难度 | 建议优先级 |
|------|------|------|-----------|
| 历史消息拉取设计错误 | 功能不可用（群聊历史无法拉取） | 低 | ★★★ 最先做 |
| 会话更新静默失败 | 数据不一致，难以感知 | 低（Level 1）| ★★★ 最先做 |
| Pending 重启丢失 | 消息可能丢失 | 中 | ★★ 次优先 |
| unread_count 并发隐患 | 当前不是 bug，是未来风险 | 低（改接口）| ★ 可以后做 |
