# Week 01 — WebSocket 连接层

## HTTP Upgrade 过程

WebSocket 连接必须通过 HTTP Upgrade 建立，原因是借用 80/443 端口穿过防火墙。

**握手流程：**

1. 客户端发 HTTP GET，带特殊头：
   ```
   Upgrade: websocket
   Connection: Upgrade
   Sec-WebSocket-Key: <随机 base64>
   Sec-WebSocket-Version: 13
   ```

2. 服务端返回 `101 Switching Protocols`，握手完成

3. 之后这条 TCP 连接不再是 HTTP，改用 WebSocket 帧格式通信

---

## WebSocket 帧格式

每条消息由一个或多个帧组成，关键字段：

| 字段 | 含义 |
|------|------|
| FIN | 是否是最后一帧 |
| opcode | 帧类型（Text/Binary/Close/Ping/Pong） |
| MASK | 客户端→服务端必须掩码，服务端→客户端不需要 |
| Payload len | 数据长度，≤125 直接存，否则用扩展字段 |

gorilla/websocket 把帧的细节都封装好了，只需要关心 `ReadMessage` / `WriteMessage`。

---

## gorilla/websocket Upgrader

```go
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        return true  // 开发阶段，生产要验证 Origin
    },
}

conn, err := upgrader.Upgrade(w, r, nil)
// 拿到 conn 后，w 和 r 就没用了
```

`Upgrade` 内部做了：检查请求头 → 验证 Origin → 发 101 响应 → 返回 `*websocket.Conn`

---

## 跨站 WebSocket 劫持（CSWSH）

**问题根源：** 浏览器不限制跨域发起 WebSocket 连接（和普通 HTTP 请求不同）

**攻击场景：**
```
用户登录了 bank.com（cookie 还在）
用户访问了 evil.com
evil.com 的 JS 悄悄建立 ws://bank.com/ws 连接
浏览器自动带上 cookie，服务端认为是合法用户
```

**防御：** `CheckOrigin` 检查请求头里的 `Origin` 字段（浏览器自动填，JS 无法伪造）

```go
// 生产环境
CheckOrigin: func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        return true  // 非浏览器客户端（App、wscat）直接放行
    }
    return origin == "https://your-app.com"
},
```

> CheckOrigin 只是第一道门，真正的身份验证还是靠 JWT

---

## 读写分离 goroutine

**为什么必须分离：**
- `ReadMessage()` 是阻塞的，没消息就一直等
- gorilla 规定：同一时刻只能一个 goroutine 读，一个 goroutine 写
- 读写放同一个 goroutine：读在阻塞时，写（包括心跳 Ping）永远没机会执行

**设计模式：**
```
readPump  goroutine ── 阻塞读，收到消息调 handler
writePump goroutine ── 监听 send channel + 定时发 Ping

外部发消息 ──Push()──▶ send channel ──▶ writePump ──▶ 客户端
```

**生命周期：**
- `readPump` 退出 = 连接结束（主生命周期）
- `readPump` 退出时关闭 `send` channel，`writePump` 检测到后也退出
- 两个 goroutine 都退出 = 无 goroutine 泄漏

---

## 心跳三个参数的关系

```go
writeWait  = 10s          // 每次写操作的超时
pongWait   = 60s          // 等待客户端 Pong 的超时（读超时）
pingPeriod = 54s          // 发 Ping 的间隔 = pongWait * 9/10
```

逻辑：每 54 秒发一次 Ping → 客户端收到后回 Pong → 服务端收到 Pong 后重置 60 秒读超时。
只要客户端还活着，读超时就永远不会触发。

```go
// readPump 里注册 PongHandler
conn.SetReadDeadline(time.Now().Add(pongWait))
conn.SetPongHandler(func(string) error {
    conn.SetReadDeadline(time.Now().Add(pongWait))  // 续期
    return nil
})
```

---

## ConnManager 设计

### 锁的性能影响

`sync.RWMutex` 加解锁耗时约 20-100ns，而一次 WebSocket 消息收发是毫秒级。
锁只在查找连接时短暂持有，**不是**在消息传输过程中持有，影响可忽略。

真正有问题是 10 万连接同时上下线时的锁竞争，解决方案是**分片锁**：
```go
// 按 userId 哈希分到 256 个独立 map，各自竞争互不影响
type ShardedConnManager struct {
    shards [256]*shard
}
```
现阶段不需要，等压测发现问题再优化。

### 连接断开的通知设计

**不推荐：** handleWs 里手动调 RemoveConn（职责分散，容易遗漏）

**推荐：** 构造时注入 `onClose` 回调，conn 自己触发
```go
type WsConn struct {
    onClose func(id string)  // 断开时的回调
}

// readPump 退出时
defer func() {
    c.Close()
    c.onClose(c.id)  // 自己通知，不依赖外部
}()
```

**依赖注入原则：** 构造时注入，不要创建后再 set（避免半初始化状态）

```go
// ✅ 构造器注入
conn := newWsConn(id, raw, onMessage, onClose)

// ❌ 创建后赋值，容易忘，容易 nil pointer panic
conn := newWsConn(id, raw)
conn.onClose = ...
```

### 未来演进：事件总线（Week 5+）

当断开连接需要触发多件事（广播下线、清理在线状态、保存离线消息），单个回调不够用，改成事件：
```go
type ConnEvent struct {
    Type   string  // "connected" / "disconnected" / "message"
    ConnId string
    Data   []byte
}
// ConnManager 暴露 Events chan ConnEvent，各消费者订阅自己关心的事件
```

---

## ID 生成方案

### conn id 直接用 userId

conn 代表"某个用户的当前连接"，用 userId 作为 key 路由最直接：

```go
conn, err := manager.GetConn(userId)
conn.Push(msg)
```

如果单独生成 conn id，还需要维护 `userId → connId` 映射，多一层查找，没有好处。

> 多端登录（同一账号手机+电脑同时在线）需要 `userId → [connId1, connId2]` 的一对多映射，Week 21-22 多实例部署时再重构。

---

### userId 生成方案对比

| 方案 | 有序 | 是否需要中心节点 | 适用场景 |
|------|------|----------------|---------|
| UUID v4 | ❌ 完全随机 | 不需要 | 简单场景，数据库索引性能稍差 |
| **UUID v7** | ✅ 时间前缀有序 | 不需要 | **推荐用于 userId** |
| 雪花算法 | ✅ 严格有序 | 需要 machineId（启动时获取一次） | **推荐用于 msgId** |
| 数据库自增 | ✅ | 每次都要请求 DB | ❌ 高并发瓶颈，不推荐 |

**UUID v7 示例：**
```go
// github.com/google/uuid v1.6+ 已支持
uid, _ := uuid.NewV7()
userId := uid.String()
// 输出：018e3b4a-1234-7xxx-xxxx-xxxxxxxxxxxx
//        ↑ 前48位是毫秒时间戳，天然有序
```

### 为什么 UUID/雪花不是性能瓶颈

两者都是**本地生成**，不需要网络请求，不需要锁协调：
- UUID v7：单核每秒生成数百万个
- 雪花算法：单核每秒生成 400 万+

只有每次都去请求中心节点（如数据库自增）才会成为瓶颈。UUID 和雪花正是为了解决"去中心化生成全局唯一 ID"而发明的。

---

## 关闭连接的三个关键点

### 一、关闭信号要广播，不能点对点

连接有 Read 和 Write 两个 goroutine，关闭时两个都要退出。

```go
// ❌ channel 发送：只能唤醒一个 goroutine，另一个泄漏
c.closeCh <- struct{}{}

// ✅ channel 关闭：所有监听它的 goroutine 同时收到信号
close(c.closeCh)
```

`close` 一个 channel 后，所有 `<-closeCh` 操作立即返回零值，不阻塞。
这是 Go 里做"广播退出"的标准模式。

---

### 二、`Close()` 必须幂等（sync.Once）

连接断开时，Read 和 Write goroutine 都可能同时发现错误，同时调 `Close()`。
不加保护会导致：

- `close(closeCh)` 调两次 → **panic: close of closed channel**
- `onClose(id)` 调两次 → 业务副作用重复（广播下线、清理在线状态）
- `conn.Close()` 调两次 → 底层报错

用 `sync.Once` 保证内部逻辑只执行一次：

```go
func (c *WsConn) Close() error {
    c.closeOnce.Do(func() {
        close(c.closeCh)  // 广播退出信号
        c.onClose(c.id)   // 通知外部清理
        c.conn.Close()    // 关闭底层 TCP 连接
    })
    return nil
}
```

无论 `Close()` 被调多少次，`Do` 里的代码只执行一次，后续调用静默忽略。

---

### 三、Push 不能在连接关闭后阻塞

连接关闭后 writePump 退出，`writeCh` 没人消费。
外部（比如群聊广播遍历所有连接）仍可能调 `Push`，无退出条件时永远阻塞，goroutine 泄漏。

```go
// ❌ 无退出条件，writePump 退出后永远阻塞
func (c *WsConn) Push(data []byte) error {
    c.writeCh <- data
    return nil
}

// ✅ select 同时监听关闭信号，连接已关闭立即返回错误
func (c *WsConn) Push(data []byte) error {
    select {
    case c.writeCh <- data:
        return nil
    case <-c.closeCh:
        return errors.New("conn closed")
    }
}
```

---

### 三件事的本质

| 问题 | 根因 | 解法 |
|------|------|------|
| goroutine 泄漏（Read/Write 未退出） | 关闭信号只能点对点 | `close(channel)` 广播 |
| 业务副作用重复执行 | Close 可能被多方同时调用 | `sync.Once` 保证幂等 |
| Push goroutine 泄漏 | channel 发送无退出条件 | `select` + `closeCh` |

---

## 单机 WebSocket 连接数

| 瓶颈 | 说明 |
|------|------|
| 文件描述符 | 默认 1024，可调到 100 万 |
| 内存 | 每个连接约 25KB（2 个 goroutine + channel + 缓冲区） |
| 网络带宽 | 10 万连接 × 1KB/s = ~1.6Gbps，普通网卡撑不住 |

**实际生产经验值：**
- 普通 IM（消息频率低）：5-10 万/台
- 游戏（高频状态同步）：1-3 万/台

**现阶段只需关心：连接断开后两个 goroutine 都能正常退出，无内存泄漏。**
Week 10 压测目标是 1000 并发连接，Week 21-22 才做多实例水平扩展。
