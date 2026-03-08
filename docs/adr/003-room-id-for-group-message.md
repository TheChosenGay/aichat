# ADR-003: 群聊消息使用独立 RoomId 字段，不复用 ToId

## 背景

实现群聊功能时，需要决定如何在 Message 结构中区分单聊和群聊目标：
- **方案 A**：复用 `to_id` 字段，单聊时存 userId，群聊时存 roomId
- **方案 B**：新增独立 `room_id` 字段，单聊时 `room_id` 为空，群聊时 `to_id` 为空

## 决策

新增独立的 `room_id` 字段，`to_id` 只用于单聊目标 userId，不做复用。

```go
type Message struct {
    MsgId   string
    FromId  string      // 服务端从 JWT 注入
    ToId    string      // 单聊：接收方 userId；群聊：留空
    RoomId  string      // 群聊：目标 roomId；单聊：留空
    Content string
    Type    MessageType
    SendAt  int64
    ...
}
```

## 理由

**复用 ToId 的核心问题：一个字段承担两种语义，查询层成本持续放大。**

### 1. 索引退化

`to_id` 上的联合索引 `idx_to_id_send_at` 若同时承载单聊消息和群聊消息，随数据量增大，索引树中两类数据混杂，扫描效率下降。独立字段则让两条查询路径互不干扰。

### 2. 会话列表联表查询变复杂

复用 `to_id` 时，`conversations` 表的 `target_id` 无法直接知道对应的是 user 还是 room，查询会话列表时必须双 JOIN：

```sql
-- 方案 A：每次都需要 LEFT JOIN 两张表，应用层再判断哪个命中
LEFT JOIN users u ON c.target_id = u.id
LEFT JOIN rooms r ON c.target_id = r.id
```

独立 `room_id` 后，字段是否为空即可判断类型，JOIN 目标明确。

### 3. 路由逻辑解耦

服务层可以按字段是否为空走完全独立的路径，单聊逻辑不受影响：

```go
func (s *MessageService) SendMessage(message *types.Message) error {
    if message.RoomId != "" {
        return s.sendGroupMessage(message)  // 群聊：扇出给所有成员
    }
    return s.sendDirectMessage(message)     // 单聊：现有逻辑不动
}
```

### 4. 扩展成本

后续加消息撤回、已读状态、@功能时，单聊和群聊的业务规则往往不同。字段分离后各自独立演进，复用字段则每个新功能都要带上类型判断。

## 后果

- Message 表新增 `room_id VARCHAR(36)` 字段（可为 NULL）
- 需要新增索引 `idx_room_id_send_at (room_id, send_at)` 供群聊历史消息查询
- 客户端发送群聊消息时填 `roomId`，单聊时填 `toId`，两者互斥

## 相关决策

- ADR-002：MySQL 存储消息
- Week 5：群聊功能实现
- Week 9：会话列表（conversations 表设计依赖本决策）
