# aichat API 文档

## HTTP 接口（端口 :8081）

### 用户注册

```
POST /user/create
Content-Type: application/json
```

**请求 Body**
```json
{
  "email": "rick@tencent.com",
  "password": "123456789",
  "name": "rick",
  "sex": true
}
```

**curl 示例**
```bash
curl -X POST localhost:8081/user/create \
  -H "Content-Type: application/json" \
  -d '{"email":"rick@tencent.com","password":"123456789","name":"rick","sex":true}'
```

**响应**
```json
{"code": 0}
```

---

### 用户登录

```
POST /user/login
Content-Type: application/json
```

**请求 Body**
```json
{
  "id": "61201251-7838-4654-9137-fc0c02197acb",
  "password": "123456789"
}
```

**curl 示例**
```bash
curl -X POST localhost:8081/user/login \
  -H "Content-Type: application/json" \
  -d '{"id":"61201251-7838-4654-9137-fc0c02197acb","password":"123456789"}'
```

**响应**
```json
{"code": 0, "jwtToken": "eyJhbGci..."}
```

---

### 用户列表（需要 JWT 鉴权）

```
GET /user/list/{limit}
Authorization: Bearer {token}
```

**curl 示例**
```bash
curl localhost:8081/user/list/10 \
  -H "Authorization: Bearer eyJhbGci..."
```

**响应**
```json
{"code": 0, "users": [...]}
```

---

## WebSocket 接口（端口 :8082）

### 连接

先登录拿到 token，再连接 WebSocket：

```
ws://localhost:8082/ws?token={jwtToken}
```

**wscat 示例**
```bash
# 1. 登录拿 token
curl -X POST localhost:8081/user/login \
  -H "Content-Type: application/json" \
  -d '{"id":"xxx","password":"xxx"}'

# 2. 用 token 建立 WS 连接（双引号内 ? 和 = 不需要转义）
wscat -c "ws://localhost:8082/ws?token=eyJhbGci..."
```

**注意：** URL 中 `?` 和 `=` 在双引号内不需要加反斜杠转义。

### 消息帧格式（Week 4 设计实现）

```json
// 客户端 → 服务端
{"type":"chat","msgId":"uuid","toId":"userId","content":"hello"}
{"type":"ack","msgId":"uuid"}

// 服务端 → 客户端
{"type":"chat","msgId":"uuid","fromId":"userId","content":"hello","sendAt":1234567890}
{"type":"system","content":"对方不在线"}
```

---

## 错误响应格式

```json
{"code": 1, "error": "错误描述"}
```

| HTTP 状态码 | 含义 |
|------------|------|
| 400 | 参数错误（格式不对、缺少必填字段） |
| 401 | 未授权（token 缺失或无效） |
| 500 | 服务器内部错误 |
