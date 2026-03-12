# Week 07 — 在线状态 & 历史消息拉取

## 一、设计思路

### 在线状态的作用

经过讨论，在线状态在现阶段真正有用的场景只有两个：

1. **实时推送判断**：发消息时判断用户是否在线，在线则推 WebSocket（`connManager.GetConn` 已能满足）
2. **Redis 对外暴露**：让其他服务或客户端查询某用户是否在线

以下功能现阶段**不需要**：
- `GET /user/status` 接口（没有真实 UI 需求）
- 复杂的离线消息队列（MySQL 持久化已经足够）

### 消息持久化与在线状态的关系

关键结论：**消息无论用户是否在线，都会持久化到 MySQL**。

```
发消息时：
  → 存 DB（无论在不在线）
  → 如果在线 → 顺便推 WebSocket（实时体验）
  → 如果不在线 → 等用户上线后主动拉取

用户上线时：
  → 拉取最近 N 条历史消息
  → 推给客户端
```

因此 `is_delivered` 字段对离线场景的判断意义有限，用**拉取最近 N 条**更简单实用。

---

## 二、实现改动

### 1. `store/user_redis.go` — 修复在线状态 bug

**问题**：存储 `bool` 类型时，`go-redis` 会序列化为 `"1"`，但读取时与 `"true"` 比较，永远返回 `false`。

```go
// 修复前
s.redis.Set(ctx, key, true, 60*time.Second)  // 存入 "1"
return result == "true", nil                  // ❌ 永远 false

// 修复后
s.redis.Set(ctx, key, "1", 60*time.Second)   // 明确存 "1"
return result != "", nil                      // ✅ key 存在即在线
```

**Key 命名**（同时修复 typo）：
```
user:online:{userId}   // 原来是 user:onlien:{userId}
```

### 2. `store/message_db.go` — 修复 NULL 扫描 + 新增历史消息查询

**问题**：单聊消息的 `room_id` 在 DB 中为 `NULL`，直接 `Scan` 到 `string` 会报错。

**解决方案**：`types.Message` 不改动，在 store 层用局部变量接收：

```go
var roomId sql.NullString
rows.Scan(..., &roomId)
message.RoomId = roomId.String  // NULL → ""，有值 → 实际值
```

**新增 `FetchHistoryMessages` 查询**：

```sql
SELECT msg_id, from_id, to_id, type, content, send_at, is_delivered, room_id
FROM messages
WHERE to_id = ?
  AND send_at <= ?
ORDER BY send_at DESC
LIMIT ?
```

充分利用现有索引 `idx_to_id_send_at (to_id, send_at)`：
- `to_id = ?` 等值匹配，定位到该用户消息
- `send_at <= ?` 范围扫描
- `ORDER BY send_at DESC` 利用索引有序，无需额外排序
- `LIMIT ?` 找到足够行数立即停止

### 3. `gateway/conn.go` — 新增回调类型

```go
type ConnConnectCallback func(id string)  // 连接建立时
type ConnPongCallback    func(id string)  // 收到 Pong 时（心跳续期）
```

### 4. `gateway/ws/ws_conn.go` — 新增 onConnect / onPong 回调

```go
type WsConn struct {
    onConnect gateway.ConnConnectCallback
    onPong    gateway.ConnPongCallback
    onClose   gateway.ConnCloseCallback
    onMessage gateway.ConnMessageCallback
    // ...
}
```

**`Start()` 执行顺序**：

```go
func (c *WsConn) Start() {
    c.Read()       // 先启动读 goroutine
    c.Write()      // 再启动写 goroutine
    c.onConnect()  // 最后触发初始化（确保读写就绪后再推消息）
}
```

**心跳续期**：

```go
c.conn.SetPongHandler(func(string) error {
    c.onPong(c.id)                                   // Redis EXPIRE 续期
    c.conn.SetReadDeadline(time.Now().Add(pongWait)) // 重置读超时
    return nil
})
```

`pingPeriod = 54s`，TTL = 60s，每 54s 续期一次，不会过期。

### 5. `gateway/ws/ws_server.go` — 接入在线状态和历史消息

```go
conn := NewWsConn(
    id, c,
    func(id string) {
        // 连接建立：标记在线 + 拉取历史消息
        s.userService.SetOnlineStatus(id, true)
        s.messageService.FetchHistoryMessages(id, 20, time.Now().Unix())
    },
    func(id string) {
        // 收到 Pong：续期在线状态
        s.userService.RenewOnline(id)
    },
    func(id string) {
        // 连接断开：移除连接 + 标记离线
        s.ConnManager.RemoveConn(id)
        s.userService.SetOnlineStatus(id, false)
    },
    func(data []byte) {
        // 收到消息处理...
    },
)
```

---

## 三、关键设计决策

### 为什么不用单独的离线消息队列？

| 方案 | 适用场景 |
|------|---------|
| MySQL `is_delivered` | 消息量中等，需要历史记录，当前阶段 ✅ |
| Redis List | 纯离线队列，不需要历史记录，高吞吐 |
| Kafka | 分布式、海量消息，Week 17+ 再考虑 |

消息历史和离线消息复用同一张表，不维护两份数据，简单可靠。

### 群聊消息的送达状态

群聊消息无法用单个 `is_delivered` 字段追踪每个成员的送达情况。

现阶段采用**方案 C：不追踪群消息的个人送达状态**：
- 群消息只记录是否发出
- 用户上线后根据 `room_id + send_at` 拉取历史消息自己补齐
- 等后续做"消息已读回执"时再引入 `message_receipts` 表

### `is_delivered` 的定位

当前 `is_delivered` 主要服务于**单聊 ACK 机制**（Week 6），离线消息场景直接用时间戳拉取更简单，不强依赖此字段。

---

## 四、验收标准

1. 用户 A 连接后，Redis 中 `online:{userId}` 存在，TTL 约 60s
2. 用户 A 断开后，Redis 中对应 key 被删除
3. 用户 A 连接后，自动收到最近 20 条历史消息
4. 用户 A 保持连接超过 54s，Redis TTL 被续期，不会过期变离线
5. 单聊消息（`room_id` 为 NULL）能正常扫描，不报错
