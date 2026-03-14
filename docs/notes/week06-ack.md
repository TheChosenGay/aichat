# Week 06 — ACK 可靠送达

## 一、方案选型

### 候选方案对比

| 方案 | 原理 | 优点 | 缺点 |
|------|------|------|------|
| **方案一：内存 pending map + 定时重推** | Push 后存内存 map，定时扫描超时重推，收到 ACK 删除 | 实现简单、延迟低、与现有结构契合 | 重启丢 pending；内存泄漏风险（需 TTL 兜底）；单机方案 |
| 方案二：Redis 存 pending | pending 状态写 Redis（EX 过期），goroutine 扫描重推 | 重启不丢状态；天然支持多实例 | 每条消息读写 Redis，复杂度高；扫描 key 性能差；当前单机场景过度设计 |
| 方案三：客户端自重传 | 服务端只记 ACK，客户端超时自己重发，服务端幂等去重 | 服务端逻辑最简 | 无客户端可验收（wscat 无法模拟）；服务端被动 |
| 方案四：Kafka/消息队列 | 消息写队列，Consumer 推送，ACK 对应 Offset Commit | 最可靠，天然支持重试和持久化 | 架构级重构，引入 Broker、Consumer Group；是 Week 17-18 的独立任务 |

### 选择：方案一（内存 pending map）

**理由：**

1. **与现有代码契合度最高**：pending map 放在 `WsConn` 里，每个连接管自己的 pending，结构清晰，改动范围局部。
2. **当前是单机部署**：Redis 方案的多实例优势现阶段用不上，引入会增加不必要的复杂度。
3. **重启丢 pending 可接受**：消息已持久化到 MySQL，Week 7 的离线拉取会补偿——用户重连后主动拉取未送达消息，两者配合构成完整可靠性保障。
4. **验收标准可测**：用 wscat 模拟断连重连，可以直接观察未 ACK 消息被重推的行为。
5. **与 Kafka 不冲突**：方案一是局部改动；方案四是 Week 17-18 的架构级重构，到时重新设计 ACK 在 Kafka 语义下的映射，两者是不同层次的问题。

**内存泄漏兜底策略：**
- 每条 pending 消息超过 **3 次重试**或超过 **30s** 后强制清除
- 不需要额外守护进程，在定时扫描 goroutine 里一并处理

---

## 二、实现设计

### 消息帧协议扩展

**客户端 ACK 帧（客户端 → 服务端）**

```json
{
  "type": "ack",
  "msgId": "550e8400-e29b-41d4-a716-446655440000"
}
```

### 核心数据结构

```go
// 单条 pending 记录
type pendingMsg struct {
    message *types.Message
    data    []byte    // 序列化好的 JSON，避免重推时重复序列化
    retries int       // 已重试次数
    sentAt  time.Time // 最后一次发送时间
}

// WsConn 新增字段
type WsConn struct {
    // ...existing fields...
    pending   map[string]*pendingMsg  // msgId → pendingMsg
    pendingMu sync.Mutex
}
```

### 流程

```
Push(msg)
    ↓
序列化 → conn.send channel
    ↓
同时写入 pending map（msgId → pendingMsg{retries:0, sentAt:now}）

客户端收到消息 → 发 ACK {"type":"ack","msgId":"xxx"}
    ↓
ws_server.onMessage 识别 type=="ack"
    ↓
delete(pending, msgId) + UPDATE messages SET is_delivered=true WHERE msg_id=?

定时器每 5s 扫描 pending map：
    sentAt 超过 10s → retries++ → 重推
    retries >= 3    → 放弃，清除 pending，记录 warn 日志
```

### 关键代码骨架

```go
// gateway/ws/ws_conn.go
func (c *WsConn) startAckTimer() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            c.retryPending()
        case <-c.done:
            return
        }
    }
}

func (c *WsConn) retryPending() {
    c.pendingMu.Lock()
    defer c.pendingMu.Unlock()
    now := time.Now()
    for msgId, p := range c.pending {
        if now.Sub(p.sentAt) < 10*time.Second {
            continue
        }
        if p.retries >= 3 {
            delete(c.pending, msgId)
            slog.Warn("message dropped after max retries", "msgId", msgId)
            continue
        }
        p.retries++
        p.sentAt = now
        c.send <- p.data  // 重推
    }
}
```

---

## 三、ACK 与离线消息的职责边界

### ACK 机制解决的问题

**场景：用户在线，但网络抖动导致消息丢失。**

用户已连接，Push 出去了，但 TCP 层或客户端侧出了问题，消息没有真正被用户"看到"。
ACK 机制通过超时重推来保证这条消息最终送达。

```
用户在线
    ↓
ConnManager.GetConn() 找到连接 → Push 消息 → 写入 pending map
    ↓
等待 ACK（最多 10s × 3 次）
    ↓
收到 ACK → 清除 pending，更新 is_delivered=1
未收到 ACK → 超时重推，直到 3 次后放弃
```

**ACK 无法解决的问题：用户根本不在线。**

```go
// gateway/conn_manager.go
func (c *ConnManager) Route(message *types.Message) error {
    conn, err := c.GetConn(message.ToId)
    if err != nil {
        // 用户不在线，直接返回，消息不进入 pending
        return nil
    }
    // 只有走到这里才会 Push + 写 pending
    ...
}
```

用户离线时，消息只写入了 MySQL（`is_delivered=0`），ACK 机制对这条消息完全不知情，没有任何重推行为。

---

### 离线消息解决的问题（Week 7）

**场景：用户离线期间别人给他发了消息，上线后需要收到。**

Week 7 在用户 WS 握手成功时，主动查询 MySQL 中未送达的消息批量推送：

```
用户上线（WS 握手成功）
    ↓
查询 MySQL：SELECT * FROM messages WHERE to_id=? AND is_delivered=0
    ↓
批量 Push 给客户端
    ↓
客户端逐条 ACK → 更新 is_delivered=1
```

---

### 两者职责对比

| 场景 | 问题描述 | 解决方案 | 周次 |
|------|---------|---------|------|
| 用户**在线**，网络抖动丢包 | 消息发出去但没送达 | ACK + 超时重推 | Week 6 |
| 用户**离线**，期间收到消息 | 消息存库了但无人推送 | 上线后主动拉取 | Week 7 |
| **群消息**，成员在线但推送失败 | 尽力投递，不做 ACK | 客户端主动拉取历史 | — |

**两者缺一不可：**
- 只有 ACK 没有离线拉取 → 离线用户永远收不到消息
- 只有离线拉取没有 ACK → 在线用户网络抖动时消息丢失无感知
- 两者组合 → 覆盖所有场景，构成完整的可靠性保障

---

## 五、群消息可靠性：为什么不做 ACK

### 1v1 pender 不能直接套用群消息

1v1 pender 以 `MsgId` 为 key，一条消息一条记录。群消息发给 N 个人，如果也要 ACK，需要 N 条独立 pending 记录，`MsgId` 必须区分（同一条消息对不同接收者是不同的 pending）。

实现上有两种路：

| 思路 | 做法 | 缺点 |
|------|------|------|
| 拆分消息 | 每个接收者克隆一条消息，生成新 MsgId | 存储膨胀，客户端 MsgId 与发送方不一致 |
| 独立 GroupPender | key = `MsgId:ToId` | 需要新增 pender 实现，复杂度高 |

### 当前决策：群消息尽力投递，不做 ACK

**理由：**
1. 群消息 ACK 实现成本比 1v1 高一个量级
2. 主流 IM（微信、钉钉）对群消息也是尽力投递，不是严格 ACK
3. 消息已持久化到 MySQL，客户端随时可以通过历史消息接口补拉

### 分层可靠性策略

| 消息类型 | 可靠性策略 | 兜底手段 |
|---------|-----------|--------|
| 1v1 消息 | 服务端 pender + ACK 重试 | 离线拉取 |
| 群消息 | 尽力投递（并发推送） | 客户端主动拉取历史消息 |

---

## 四、验收标准

1. 客户端 A 连接，B 正常在线 → A 发消息 → B 收到后发 ACK → MySQL `is_delivered` 更新为 1
2. B 断连（不发 ACK）→ 服务端 10s 后自动重推 → 重推 3 次后放弃 → 日志有 warn 记录
3. B 重连后（Week 7 实现前）暂不自动收到消息，Week 7 接入离线拉取后补全
