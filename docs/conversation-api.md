# 会话功能接口文档

## 概览

会话系统分两部分：
1. **REST API**：创建会话、拉取会话列表（HTTP）
2. **WebSocket 实时通知**：收到新消息时服务端主动推送会话更新事件

所有 REST 接口需要在 Header 携带 JWT Token：
```
Authorization: Bearer <token>
```

---

## 数据结构

### Conversation 对象

```json
{
  "CId": "550e8400-e29b-41d4-a716-446655440000",
  "UserId": "当前用户ID",
  "PeerId": "单聊对方用户ID（群聊时为空字符串）",
  "RoomId": "群聊房间ID（单聊时为空字符串）",
  "LastSenderName": "最后一条消息的发送者名字",
  "LastMsgId": "最后一条消息的ID",
  "LastMsgTime": 1234567890,
  "LastMsgContent": "消息内容预览",
  "UnreadCount": 3
}
```

**说明**：
- `PeerId` 和 `RoomId` 互斥，单聊时只有 `PeerId`，群聊时只有 `RoomId`
- `UnreadCount` 是当前用户在该会话中的未读消息数
- `LastMsgTime` 是 Unix 时间戳（秒级），用于游标分页

---

## REST 接口

### 1. 创建会话

**说明**：在发送第一条消息前，需要先创建会话。会话创建后，后续每条消息发送时服务端会自动更新会话内容，客户端无需重复创建。

```
POST /conversation/create
Content-Type: application/json
Authorization: Bearer <token>
```

**请求体**（`peer_id` 和 `room_id` 二选一）：
```json
{
  "peer_id": "对方用户ID（单聊）"
}
```
```json
{
  "room_id": "群聊房间ID（群聊）"
}
```

**响应**：
```json
{
  "code": 0,
  "data": null,
  "msg": "success"
}
```

**错误情况**：
| code | 说明 |
|------|------|
| 400 | `peer_id` 和 `room_id` 同时传 / 都没传 / 格式不是 UUID |
| 500 | 服务端错误 |

---

### 2. 获取会话列表

**说明**：按最后消息时间倒序返回当前用户的所有会话，支持游标分页。

```
GET /conversation/list?cookie=<cursor>&limit=<n>
Authorization: Bearer <token>
```

**Query 参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `cookie` | int64 | 否 | 游标，值为上一页最后一条会话的 `LastMsgTime`。不传则从最新开始 |
| `limit` | int | 否 | 每页条数，默认 10 |

**首次请求**（拉取最新）：
```
GET /conversation/list?limit=20
```

**翻页请求**（加载更多）：
```
GET /conversation/list?cookie=1234567880&limit=20
```

**响应**：
```json
{
  "code": 0,
  "data": [
    {
      "CId": "uuid",
      "UserId": "me",
      "PeerId": "userA",
      "RoomId": "",
      "LastSenderName": "张三",
      "LastMsgId": "msg-uuid",
      "LastMsgTime": 1234567890,
      "LastMsgContent": "你好",
      "UnreadCount": 2
    },
    {
      "CId": "uuid",
      "UserId": "me",
      "PeerId": "",
      "RoomId": "room-uuid",
      "LastSenderName": "李四",
      "LastMsgId": "msg-uuid",
      "LastMsgTime": 1234567880,
      "LastMsgContent": "群里有人吗",
      "UnreadCount": 5
    }
  ],
  "msg": "success"
}
```

**分页逻辑**：
- 响应数组最后一项的 `LastMsgTime` 作为下一次请求的 `cookie`
- 当响应数组长度 < `limit` 时，说明已经是最后一页

---

## WebSocket 实时通知

### 连接地址

```
ws://host:8082/ws?token=<jwt_token>
```

### 会话更新通知

当有新消息发送时，**消息的发送方和接收方都会收到**一条 `Type=6` 的通知消息。

客户端收到此通知后，应重新拉取对应会话的数据（调用 `/conversation/list` 或更新本地缓存）。

**通知消息格式**（与普通消息结构相同）：
```json
{
  "MsgId": "uuid",
  "FromId": "触发更新的对方用户ID",
  "ToId": "接收通知的当前用户ID",
  "RoomId": "",
  "Content": "",
  "Type": 6,
  "SendAt": 1234567890,
  "IsDelivered": false
}
```

**如何定位是哪个会话更新了**：
- 单聊：用 `FromId` 在本地会话列表中查找 `PeerId == FromId` 的会话
- 群聊：`RoomId` 不为空时，查找 `RoomId` 对应的会话

### 所有 WebSocket 消息类型

| Type 值 | 常量名 | 说明 |
|---------|--------|------|
| 0 | Text | 普通文本消息 |
| 1 | Image | 图片消息 |
| 2 | Audio | 语音消息 |
| 3 | System | 系统消息 |
| 4 | Ack | 消息送达确认（客户端发给服务端） |
| 5 | Failed | 消息发送失败通知（服务端发给客户端） |
| 6 | ConversationUpdate | 会话更新通知（服务端发给客户端） |

---

## 典型交互流程

### 场景 1：打开 App，进入会话列表页

```
1. GET /conversation/list?limit=20
   → 展示会话列表

2. WebSocket 保持连接
   → 收到 Type=6 通知时，刷新对应会话项（未读数、最后消息预览）
```

### 场景 2：首次和某人聊天

```
1. POST /conversation/create { "peer_id": "userA" }
   → 创建会话

2. WebSocket 发送消息
   → 服务端自动更新双方会话
   → 双方各收到 Type=6 通知
```

### 场景 3：下拉加载更多会话（分页）

```
1. 首次：GET /conversation/list?limit=20
   → 拿到 20 条，最后一条 LastMsgTime=1000

2. 下拉：GET /conversation/list?cookie=1000&limit=20
   → 拿到更早的 20 条

3. 继续下拉直到响应数量 < limit，停止翻页
```

### 场景 4：进入会话，清空未读数

目前服务端暂无清空未读数的专用接口，建议客户端进入会话后本地将该会话 `UnreadCount` 置为 0，并在下次刷新会话列表时以服务端数据为准。
