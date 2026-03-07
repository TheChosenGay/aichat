# Week 02 — 安全加固：bcrypt + JWT

## 一、密码哈希（bcrypt）

### 为什么不能明文存储

数据库一旦泄露，明文密码直接暴露。用户在其他网站用同样密码也会被连带攻击。

### 为什么用 bcrypt，不用 MD5/SHA256

MD5/SHA256 速度太快，GPU 每秒可暴力穷举几十亿次。
bcrypt 专门设计得**很慢**，内置 `cost` 参数控制计算量（cost=12 时单次约 300ms）。
此外 bcrypt 每次生成的哈希内置随机 salt，相同密码存储结果也不同，无法通过对比哈希找出相同密码的用户。

### 代码

```go
import "golang.org/x/crypto/bcrypt"

// 注册时：哈希后存库
hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
user.Password = string(hashed)  // 存 "$2a$10$..." 格式

// 登录时：不能用 ==，用专门的比对函数
err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(inputPassword))
if err != nil {
    // 密码错误
}
```

`CompareHashAndPassword` 内部自动从哈希值里提取 salt 重新计算，不需要手动处理 salt。

---

## 二、JWT 设计

### JWT 的结构

```
eyJhbGci....  eyJ1c2VyS....  SflKxwRJS....
    ↑               ↑               ↑
  Header         Payload         Signature
（算法类型）   （存的数据）    （签名，防篡改）
```

三段用 `.` 连接，Base64 编码，**不是加密**，Payload 内容任何人都能解码看到。
所以 JWT 里**不要放密码、手机号等敏感信息**。

### 固定 secret 方案（当前使用）

```
登录 → 用固定 JWT_SECRET 签名 → 返回 token
验证 → 用同一个 JWT_SECRET 验签 → 不需要查数据库或 Redis
```

secret 放在 `.env` 里，不提交 git：
```bash
JWT_SECRET=随机生成的64位以上字符串
```

### 生成 JWT

```go
func GenerateJwt(user *types.User) (string, error) {
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
        "userId": user.Id,
        "email":  user.Email,
        "exp":    time.Now().Add(24 * time.Hour).Unix(),  // 必须设置过期时间
    })
    secret := os.Getenv("JWT_SECRET")
    return token.SignedString([]byte(secret))
}
```

### 验证 JWT — 三层检查

```go
func VerifyJwt(jwtToken string) (string, error) {
    secret := os.Getenv("JWT_SECRET")

    // 第一层：jwt.Parse 自动验签（签名错误/被篡改直接报错）
    token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
        return []byte(secret), nil
    })
    if err != nil {
        return "", err  // 包含签名错误、过期等
    }

    // 第二层：jwt.Parse 自动检查 exp 字段是否过期
    // （前提：生成时写了 "exp" 字段，否则不检查）

    // 第三层：业务层验证 — claims 内容是否合法
    userId, ok := token.Claims.(jwt.MapClaims)["userId"].(string)
    if !ok || userId == "" {
        return "", errors.New("userId not found in token")
    }

    return userId, nil
}
```

> ⚠️ **不要手动判断 `exp`**，原因见下方。

### jwt.Parse 自动处理过期的原理

`exp` 不是随便起的名字，是 **JWT 规范（RFC 7519）预定义的 Registered Claims**：

| 字段 | 含义 | jwt 库是否自动处理 |
|------|------|----------------|
| `exp` | Expiration Time，过期时间 | ✅ 自动检查 |
| `nbf` | Not Before，生效时间 | ✅ 自动检查 |
| `iat` | Issued At，签发时间 | 不检查，只记录 |
| `iss` | Issuer，签发方 | 需手动验证 |
| `sub` | Subject，主题/用户 | 不检查 |

`jwt.Parse` 内部调用 `MapClaims.Valid()`，自动读取 `exp` 字段和当前时间比较：

```go
// jwt 库内部逻辑（简化）
func (m MapClaims) Valid() error {
    if !m.VerifyExpiresAt(time.Now().Unix(), false) {
        return &ValidationError{text: "Token is expired"}
    }
    return nil
}
```

所以 `jwt.Parse` 返回 `err != nil` 时，已经包含了过期的情况，**不需要也不应该手动再判断 `exp`**。

### 手动判断 exp 的陷阱

JWT 库内部用 JSON 反序列化，所有数字默认是 `float64`：

```go
// ❌ 会 panic：JWT 里的数字是 float64，不是 int64
expireClaim.(int64)

// ✅ 如果真的要手动取（不推荐），要用 float64
expireClaim.(float64)
```

正确做法：直接信任 `jwt.Parse` 的返回值，它报错就是无效 token，不需要自己再检查。

---

## 三、JWT 中间件

### 如何向下游传递 userId

中间件验证完 token 拿到 userId，通过 `context` 传给下游 handler：

```go
// middleware/jwt.go

type contextKey string
const UserIdKey contextKey = "userId"  // 用私有类型做 key，避免和其他包冲突

func JwtMiddleware(next HttpFunc) HttpFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 先从 query param 取（WebSocket 用）
        token := r.URL.Query().Get("token")
        // 再从 Header 取（HTTP 接口用）
        if token == "" {
            token = r.Header.Get("Authorization")
        }
        if token == "" {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        userId, err := utils.VerifyJwt(token)
        if err != nil {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        // 注入 context
        ctx := context.WithValue(r.Context(), UserIdKey, userId)
        next(w, r.WithContext(ctx))
    }
}
```

### 下游 handler 取 userId

```go
// ✅ 带检查的断言，不会 panic
userId, ok := r.Context().Value(middleware.UserIdKey).(string)
if !ok || userId == "" {
    http.Error(w, "Unauthorized", http.StatusUnauthorized)
    return
}

// ❌ 强制断言，Value 为 nil 时直接 panic
userId := r.Context().Value(middleware.UserIdKey).(string)
```

### context key 为什么要用私有类型

```go
// ❌ 用 string 做 key，其他包也可能用 "userId" 导致冲突
context.WithValue(ctx, "userId", value)

// ✅ 用私有类型，类型不同即使字符串相同也不会冲突
type contextKey string
const UserIdKey contextKey = "userId"
context.WithValue(ctx, UserIdKey, value)
```

`context.Value` 比较 key 时是严格类型匹配，`contextKey("userId") != string("userId")`。

---

## 四、token 传递方式

| 场景 | 传递方式 | 示例 |
|------|---------|------|
| HTTP 接口 | Header | `Authorization: Bearer eyJhbGci...` |
| WebSocket 握手 | Query Param | `ws://host/ws?token=eyJhbGci...` |

WebSocket 握手本质是 HTTP 请求，但浏览器原生 `WebSocket` API 和 wscat 都**不支持自定义 Header**，所以 WebSocket 用 query param 传 token。

---

## 五、HTTP 接口参数规范

### 密码为什么必须放 Body，不能放 URL

**原因一：URL 会被多处记录**

```
GET /user/login?password=123456
```
这条 URL 会出现在：
- 服务器 access log（nginx/apache 默认记录完整 URL）
- 浏览器历史记录
- CDN / 反向代理的日志
- 终端 shell 历史（curl 命令）

HTTP Body 不会出现在任何日志，除非你主动打印。

**原因二：URL 有长度限制**

不同浏览器/服务器限制不同（通常 2KB-8KB），Body 理论上无限制。密码、头像等大字段放 URL 随时溢出。

**原因三：HTTP 方法语义**

| 方法 | 语义 | 参数位置 |
|------|------|---------|
| GET | 查询，幂等，可缓存 | URL Query |
| POST | 创建/提交，有副作用 | JSON Body |
| PUT/PATCH | 修改 | JSON Body |
| DELETE | 删除 | URL Path |

登录会产生 token（有副作用），应该用 `POST` + Body。
查询用 `GET` + Query 完全合理（`/user/list?limit=10`）。

### 参数位置总结

| 场景 | 参数位置 |
|------|---------|
| 查询条件（GET） | URL Query `?limit=10&page=1` |
| 资源标识（任意方法） | URL Path `/user/{id}` |
| 创建/修改的数据（POST/PUT） | JSON Body |
| **密码、token 等敏感信息** | **必须放 Body，禁止放 URL** |

### 读取 Body 的标准写法

```go
// 定义请求结构体
type LoginRequest struct {
    Id       string `json:"id"       validate:"required,uuid"`
    Password string `json:"password" validate:"required,min=8,max=32"`
}

// handler 里解码
var req LoginRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    http.Error(w, "invalid request body", http.StatusBadRequest)
    return
}
```

---

## 六、secret 管理方案对比

| 方案 | 安全性 | 适用阶段 |
|------|-------|---------|
| **固定 secret（当前）** | 中，secret 泄露可伪造任意 token | Week 1-18 |
| **RS256 非对称加密** | 高，私钥只在认证服务，gateway 只持有公钥 | Week 19 微服务拆分时 |
| **refresh token** | 高，access token 15 分钟过期，损失窗口小 | Week 9 API 规范化时 |

### Logout 与 token 黑名单

固定 secret 无法让单个 token 立即失效，解决方案：

```
登出时：把 token 存入 Redis 黑名单
    SET logout:token:{token} 1 EX {token剩余有效期}

验证时：验签通过后，再查一次黑名单
    EXISTS logout:token:{token}
    如果存在 → 拒绝，token 已登出
```

token 过期后 Redis key 自动删除，不需要手动清理。
