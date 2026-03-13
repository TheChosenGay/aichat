# Week 09 — API 标准化

## 一、问题梳理

改动前各接口响应格式混乱：

| 问题 | 示例 |
|------|------|
| 响应字段不统一 | `jwtToken`、`users`、`room`、`members` 各自为政 |
| 错误响应混用两种方式 | `http.Error`（纯文本）和 `WriteToJson`（JSON）并存 |
| HTTP 状态码不正确 | 部分错误返回 200，body 里才有错误信息 |
| 没有设置 Content-Type | 客户端无法识别响应类型 |
| `WriteToJson` 参数是 `io.Writer` | 无法设置状态码和 Header |

---

## 二、统一响应标准

### 响应结构

```go
type APIResponse struct {
    Code int    `json:"code"`
    Data any    `json:"data"`
    Msg  string `json:"msg"`
}
```

### 响应码

| Code | HTTP 状态码 | 含义 |
|------|-------------|------|
| 0    | 200         | 成功 |
| 400  | 400         | 参数错误 |
| 401  | 401         | 未授权 |
| 500  | 500         | 服务器内部错误 |

### 响应示例

```json
// 成功
{"code": 0, "data": {"jwtToken": "xxx"}, "msg": "success"}

// 失败
{"code": 400, "data": null, "msg": "参数错误"}
{"code": 500, "data": null, "msg": "服务器错误"}
```

---

## 三、实现改动

### 新建 `api/response.go`

核心是 `writeJSON` 内部方法，统一处理 Header 和状态码：

```go
func writeJSON(w http.ResponseWriter, httpStatus int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(httpStatus)
    json.NewEncoder(w).Encode(v)
}

func OK(w http.ResponseWriter, data any) {
    writeJSON(w, http.StatusOK, APIResponse{Code: 0, Data: data, Msg: "success"})
}

func BadRequest(w http.ResponseWriter, msg string) { ... }   // 400
func Unauthorized(w http.ResponseWriter, msg string) { ... } // 401
func InternalError(w http.ResponseWriter, msg string) { ... } // 500
```

**关键设计决策**：参数用 `http.ResponseWriter` 而不是 `io.Writer`，才能设置 Header 和状态码。

### `api/user.go` 和 `api/room.go`

废弃 `http.Error` 和旧的 `WriteToJson`，全部改用新的工具函数：

```go
// 改前
http.Error(w, service.NewError(...).String(), http.StatusBadRequest)
WriteToJson(w, map[string]any{"code": 1, "error": err.Error()})

// 改后
BadRequest(w, service.NewError(...).Error())
InternalError(w, service.NewError(...).Error())
OK(w, map[string]any{"jwtToken": token})
```

---

## 四、一个细节：`WriteHeader` 必须在 `Write` 之前调用

```go
w.Header().Set("Content-Type", "application/json")  // 1. 先设置 Header
w.WriteHeader(httpStatus)                            // 2. 再写状态码
json.NewEncoder(w).Encode(v)                         // 3. 最后写 body
```

一旦调用 `Write`（或 `Encode`），Header 就会被自动刷出去，之后再调用 `WriteHeader` 或修改 Header 都无效。

---

## 五、改动文件汇总

| 文件 | 改动 |
|------|------|
| `api/response.go` | 新建，定义 `APIResponse` 结构体和工具函数 |
| `api/user.go` | 所有 handler 改用 `OK` / `BadRequest` / `InternalError` |
| `api/room.go` | 所有 handler 改用 `OK` / `BadRequest` / `InternalError` |
