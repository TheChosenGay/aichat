# aichat API 文档

## HTTP 接口（端口 :8081）

### 用户注册
```
POST /user/create?email=xxx&password=xxx&name=xxx&sex=1|0
```
**响应**
```json
{"code": 0}
```

### 用户登录
```
POST /user/login?id={userId}&password=xxx
```
**响应**
```json
{"code": 0, "jwtToken": "xxx"}
```

### 用户列表（需要 JWT 鉴权 — 待实现）
```
GET /user/list/{limit}
Header: Authorization: Bearer {token}
Header: Email: {email}
```
**响应**
```json
{"code": 0, "users": [...]}
```

---

## WebSocket 接口（端口 :8082，待实现）

### 连接
```
ws://localhost:8082/ws?userId={userId}
```

### 消息帧格式（Week 4 设计）
```json
// 发送消息
{"type":"chat","msgId":"uuid","toId":"userId","content":"hello"}

// ACK
{"type":"ack","msgId":"uuid"}
```

---

## 错误码规范（待完善）
| code | 含义 |
|------|------|
| 0    | 成功 |
| 400  | 参数错误 |
| 401  | 未授权 |
| 500  | 服务器错误 |
