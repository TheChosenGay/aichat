# Week 04 — 单聊消息协议与路由

## 一、WS 消息帧协议

### 客户端 → 服务端

```json
{
  "msgId": "550e8400-e29b-41d4-a716-446655440000",
  "toId":  "接收方 userId",
  "type":  0,
  "content": "hello",
  "sendAt": 1234567890
}
```

### 服务端 → 客户端（推送）

```json
{
  "msgId":  "550e8400-e29b-41d4-a716-446655440000",
  "fromId": "发送方 userId",
  "toId":   "接收方 userId",
  "type":   0,
  "content": "hello",
  "sendAt": 1234567890
}
```

### 字段设计原则

| 字段 | 由谁提供 | 原因 |
|------|---------|------|
| `msgId` | 客户端生成 UUID | 保证幂等性，弱网重传不重复 |
| `fromId` | **服务端从 token 注入** | 客户端传的不可信，防伪造发送方 |
| `toId` | 客户端传 | 发给谁由客户端决定 |
| `sendAt` | 客户端传 | 用客户端时间保证排序符合用户感知 |

---

## 二、消息处理完整流程

```
客户端发 WS 文本帧（JSON）
    ↓
ws_server.onMessage 收到 []byte
    ↓
json.Unmarshal → types.Message（gorilla 保证是完整帧，无粘包问题）
    ↓
message.FromId = id（从 JWT context 注入，不信任客户端）
    ↓
validate.Struct(message)（校验字段合法性）
    ↓
MessageService.SendMessage(message)
    ├── messageStore.Save(message) → 写 MySQL
    └── router.Route(message)     → ConnManager.GetConn(toId) → conn.Push(data)
```

### 为什么 gorilla 不需要处理粘包

WebSocket 有帧（Frame）边界，`ReadMessage()` 返回的 `[]byte` 是**完整的一条消息**，gorilla 内部处理了分片（FIN=0）的情况，等所有分片到齐才返回。
TCP 才需要处理粘包，WebSocket 已经在协议层解决了。

---

## 三、validator 踩坑总结

### 坑1：`validator.Validate{}` 零值不可用

```go
// ❌ 零值，内部 sync.Pool 是 nil，调用 Struct() 直接 panic
validate: validator.Validate{}

// ✅ 必须用构造函数
validate: validator.New()
```

### 坑2：不存在的 validate 标签导致 panic

```go
// ❌ int64、boolean 都不是内置标签，panic: Undefined validation function
SendAt      int64 `validate:"required,int64,gt=0"`
IsDelivered bool  `validate:"required,boolean"`

// ✅ 正确写法
SendAt      int64 `validate:"gt=0"`
IsDelivered bool  // bool 不需要 validate 标签
```

### 坑3：`required` 对数值零值的判断

```go
// ❌ MessageType 的零值是 0（text），required 会拒绝 0
Type MessageType `validate:"required"`

// ✅ 去掉 required，允许零值
Type MessageType `json:"type"`
```

`required` 对数值类型的语义是"不等于零值"，`0` 会被认为未填写。

### 坑4：validate tag 里不能有空格

```go
// ❌ 有空格，tag 解析失败
validate:"required, uuid"
validate:"required, min=8, max=32"

// ✅ 逗号后不能有空格
validate:"required,uuid"
validate:"required,min=8,max=32"
```

---

## 四、消息路由设计

### 分层职责

```
ws_server（gateway 层）
    ↓ 调用
MessageService（service 层）
    ├── MessageStore（store 层）→ 持久化
    └── MessageRouter（接口）  → 路由推送
            ↑ 实现
        ConnManager（gateway 层）
```

### ConnManager 实现 MessageRouter

```go
// gateway/conn_manager.go
func (c *ConnManager) Route(message *types.Message) error {
    conn, err := c.GetConn(message.ToId)
    if err != nil {
        return nil  // 用户不在线，消息已持久化，Week 7 实现离线拉取
    }
    data, _ := json.Marshal(message)
    return conn.Push(data)
}
```

用户不在线时**不返回错误**，消息已经存库了，Week 7 实现离线消息拉取时用户上线后能拉到。

### main.go 组装方式

```go
db := store.NewMysqlInstance()
connManager := gateway.NewConnManager()
messageStore := store.NewMessageDbStore(db)
messageSrv := service.NewMessageService(messageStore, connManager)
wsServer := ws.NewWsServer(wsOpt, connManager, messageSrv)
```

`connManager` 同时传给 `WsServer`（管理连接）和 `MessageService`（路由消息），是同一个实例。

---

## 五、store/message_db.go 实现要点

### 游标分页查询

```sql
SELECT * FROM messages
WHERE to_id = ? AND send_at < ?   -- send_at < before（游标）
ORDER BY send_at DESC
LIMIT ?
```

**为什么用游标而不是 OFFSET：**

```sql
-- OFFSET 方式：翻到第 100 页要扫描前 100*20=2000 行
SELECT * FROM messages LIMIT 20 OFFSET 2000

-- 游标方式：直接从游标位置开始读，无论翻到哪页性能一样
SELECT * FROM messages WHERE send_at < ? ORDER BY send_at DESC LIMIT 20
```

游标分页性能与页码无关，大数据量下优势明显。

### 游标分页 vs OFFSET 详解

**OFFSET 的问题：翻页越深越慢**

```sql
SELECT * FROM messages ORDER BY send_at DESC LIMIT 20 OFFSET 2000
```

MySQL 不是真的"跳过"2000 行，而是**读了再丢**：
- 读出 2020 行 → 丢掉前 2000 行 → 返回 20 行
- 翻到第 N 页，就要读 N×20 行，性能随页码线性下降

```
第 1 页   → 读 20 行    ✅
第 100 页 → 读 2000 行  ⚠️
第 1000 页→ 读 20000 行 ❌
第 5000 页→ 读 100000 行 💀
```

**游标分页：无论翻到哪页，每次只读 20 行**

```sql
-- 客户端把上一页最后一条的 send_at 作为游标传来
SELECT * FROM messages
WHERE to_id = ? AND send_at < ?   -- 直接从游标位置开始
ORDER BY send_at DESC LIMIT 20
```

配合索引 `idx_to_id_send_at (to_id, send_at)`，MySQL 直接跳到游标位置，O(1) 定位，性能与页码无关。

**游标分页的代价：不支持跳页**

只能"下一页"，不能直接跳到第 50 页。IM 历史消息是"往上滑加载更多"，天然顺序翻页，完全适合游标分页。

---

### SELECT * 的隐患

```go
// 当前写法
const ListMessagesByToIdSql = `SELECT * FROM messages WHERE ...`

// 潜在问题：如果表加了新字段，Scan 的参数数量不匹配会 panic
// 建议显式列出字段名：
const ListMessagesByToIdSql = `
SELECT msg_id, from_id, to_id, type, content, send_at, is_delivered
FROM messages WHERE to_id = ? AND send_at < ? ORDER BY send_at DESC LIMIT ?
`
```
