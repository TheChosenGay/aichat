# Week 14 — Redis Pub/Sub 路由压测与 Bug 修复

## 一、压测目标

Redis Pub/Sub 路由上线后，通过稳定性压测验证：

1. **goroutine 无泄漏**：500 连接断开后 goroutine 数能回落到基线
2. **Subscribe goroutine 生命周期正确**：用户断开时消费 goroutine 能正常退出

---

## 二、压测配置

```
脚本：benchmark/ws_stability.js
场景：500 VU，30s 爬升 → 保持 9min → 30s 降到 0
每 VU 每 5s 发一条消息（低频，模拟正常在线用户）
```

goroutine 观察点：

```bash
# 基线（压测前）
curl -s "http://127.0.0.1:6060/debug/pprof/goroutine?debug=1" | head -3

# 峰值（500 VU 全部建连后约 35s）
# 压测结束后（等 graceful stop 完成）
```

---

## 三、第一轮压测：发现两个 Bug

### 3.1 Bug 1：Subscribe goroutine 泄漏

**现象**

```
压测前基线：  10 goroutine
压测峰值：  2011 goroutine（正常）
压测结束后：2008 goroutine ← 没有回落！
```

pprof 显示 500 个 goroutine 卡在：

```
goroutine [chan receive, 9 minutes]:
github.com/TheChosenGay/aichat/service/router.(*RedisMsgRouter).Subscribe.func1()
    redis_router.go:38
```

**根因**

最初的 `Subscribe` 用 `for range pubsub.Channel()` 消费消息：

```go
go func() {
    for msg := range pubsub.Channel() {  // ← 问题所在
        ...
    }
}()
```

`pubsub.Channel()` 返回的是 go-redis 内部管理的 channel。调用 `pubsub.Close()` 只会关闭底层 TCP 连接，**不会关闭这个 channel**。`for range` 读一个永远不关闭的 channel，goroutine 永久阻塞。

同时 go-redis 内部的 `initHealthCheck` goroutine 也因此无法退出，每个订阅额外泄漏 1-2 个 goroutine。

**修复**

改用 `ReceiveMessage(ctx)` + `context.WithCancel`，通过取消 ctx 让 goroutine 退出：

```go
// 修复前
type RedisMsgRouter struct {
    userChannlsMap map[string]*redis.PubSub
}

func (s *RedisMsgRouter) Subscribe(userId string) error {
    pubsub := s.redisStore.SubUser(userId)
    s.mtx.Lock()
    s.userChannlsMap[userId] = pubsub
    s.mtx.Unlock()
    go func() {
        for msg := range pubsub.Channel() {  // ← 不会退出
            ...
        }
    }()
    return nil
}

func (s *RedisMsgRouter) Unsubscribe(userId string) error {
    ...
    return s.redisStore.UnsubUser(pubsub)  // Close() 不关闭 Channel()
}
```

```go
// 修复后
type pubsubContext struct {
    pubsub *redis.PubSub
    cancel context.CancelFunc
}

type RedisMsgRouter struct {
    userChannlsMap map[string]*pubsubContext  // value 改为包含 cancel
}

func (s *RedisMsgRouter) Subscribe(userId string) error {
    ctx, cancel := context.WithCancel(context.Background())
    pubsub := s.redisStore.SubUser(userId)
    s.mtx.Lock()
    s.userChannlsMap[userId] = &pubsubContext{pubsub: pubsub, cancel: cancel}
    s.mtx.Unlock()
    go func() {
        defer pubsub.Close()
        for {
            msg, err := pubsub.ReceiveMessage(ctx)  // ctx 取消时立即返回 error
            if err != nil {
                return  // 正常退出
            }
            ...
        }
    }()
    return nil
}

func (s *RedisMsgRouter) Unsubscribe(userId string) error {
    s.mtx.Lock()
    defer s.mtx.Unlock()
    sub, ok := s.userChannlsMap[userId]
    if !ok {
        return errors.New("user not found")
    }
    sub.cancel()  // 取消 ctx → ReceiveMessage 返回 error → goroutine 退出
    if err := sub.pubsub.Close(); err != nil {
        return err
    }
    delete(s.userChannlsMap, userId)
    return nil
}
```

**关键结论**

> `pubsub.Channel()` 返回的 channel 由 go-redis 内部管理，`pubsub.Close()` 不会关闭它。
> 需要用 `ReceiveMessage(ctx)` 配合 context 取消来控制 goroutine 退出。

---

### 3.2 Bug 2：WsConn.Close 自锁死锁

**现象**

修复 Bug 1 后重跑，压测结束后仍有 1008 goroutine 未回落。pprof 显示：

```
goroutine [sync.Mutex.Lock, 2 minutes]:
sync.(*Once).doSlow(...)
github.com/TheChosenGay/aichat/gateway/ws.(*WsConn).Close()
    ws_conn.go:67
github.com/TheChosenGay/aichat/gateway.(*ConnManager).RemoveConn()
    conn_manager.go:41
github.com/TheChosenGay/aichat/gateway/ws.(*WsServer).handleWs.func2()  ← onClose 回调
    ws_server.go:91
github.com/TheChosenGay/aichat/gateway/ws.(*WsConn).Close.func1()       ← Once 内部
```

**根因**

调用链形成环路：

```
WsConn.Close()
  → closeOnce.Do(func() {      ← Once 内部锁已持有
      close(closeCh)
      onClose(id)               ← ws_server 的 onClose 回调
        → ConnManager.RemoveConn(id)
            → conn.Close()      ← 同一个 WsConn！
                → closeOnce.Do(...)  ← Once 锁已被持有，永久阻塞
    })
```

`onClose` 回调里调了 `RemoveConn`，而 `RemoveConn` 里调了 `conn.Close()`，触发第二次 `closeOnce.Do`，因为第一次 `Do` 的内部锁还没释放，第二次 `Do` 永久等待。

**修复**

打破环路，拆分职责：

| 方法 | 职责 |
|------|------|
| `RemoveConn(id)` | 只从 map 里删除记录，**不关闭连接** |
| `CloseConn(id)` / `Clean(id)` | 找到连接调 `conn.Close()`，关闭由 `Close` 自己触发 `onClose → RemoveConn` |

```go
// 修复前：RemoveConn 调了 conn.Close()，与 onClose 回调形成环
func (c *ConnManager) RemoveConn(id string) error {
    c.mx.Lock()
    conn, ok := c.conns[id]
    ...
    delete(c.conns, id)
    c.mx.Unlock()
    conn.Close()  // ← 这里造成了环路
    return nil
}
```

```go
// 修复后：RemoveConn 只删 map
func (c *ConnManager) RemoveConn(id string) error {
    c.mx.Lock()
    defer c.mx.Unlock()
    delete(c.conns, id)  // 只删 map，不关连接
    return nil
}

// 主动踢连接（logout 场景）通过 Clean 触发 Close
func (c *ConnManager) Clean(userId string) error {
    conn, err := c.GetConn(userId)
    if err != nil {
        return err
    }
    return conn.Close()  // Close 内部会触发 onClose → RemoveConn（只删 map，安全）
}
```

两条路径都变成单向无环：

```
自然断开：
ReadPump 出错 → conn.Close() → onClose → RemoveConn（只删 map）→ done

主动 logout：
Clean → conn.Close() → onClose → RemoveConn（只删 map）→ done
```

**关键结论**

> `sync.Once` 的内部锁在 `Do` 的闭包执行期间持有，如果闭包里触发了第二次 `Do`（直接或间接），会永久阻塞。
>
> 设计含 `Close` 的对象时，`Close` 的 `onClose` 回调里**不能再调回 `Close`**，否则必然死锁。
> 解法：拆分"从外部容器移除"和"关闭自身"的职责，让两者各司其职、不互相调用。

---

## 四、第二轮压测：验证修复

### 数据对比

| 时间点 | 第一轮（有 Bug） | 第二轮（修复后） |
|--------|----------------|----------------|
| 压测前基线 | 10 | 10 |
| 压测峰值（500 VU） | 2011 | 2011 |
| 压测结束后 | **2008（泄漏）** | **8（正常）** |

### 结论

**无 goroutine 泄漏**。2011 → 8，完全回落到基线，两个 Bug 均已修复。

---

## 五、经验总结

### for range channel vs ReceiveMessage(ctx)

```go
// ❌ for range channel：channel 不关闭则永远阻塞
for msg := range pubsub.Channel() { ... }

// ✅ ReceiveMessage(ctx)：ctx 取消立即返回
for {
    msg, err := pubsub.ReceiveMessage(ctx)
    if err != nil { return }
    ...
}
```

**规则：需要外部控制 goroutine 退出时，用 context 取消而不是依赖 channel 关闭。**

### sync.Once 不能在闭包内再次触发 Do

```go
// ❌ 死锁：Once 闭包内（直接或间接）触发第二次 Do
once.Do(func() {
    callback()       // callback 内部再次调用持有 Once 的方法
})

// ✅ 把可能回调的逻辑移到 Once 外部
once.Do(func() {
    // 只做最小化的原子操作
    close(ch)
    conn.close()
})
callback()  // Once 执行完后再回调
```

### 对象 Close 设计原则

```
❌ Close → onClose → 外部容器.Remove → Close（环路死锁）

✅ 职责拆分：
   Close  → onClose → 外部容器.Remove（只删记录）
   Kick   → 外部容器.Remove（只删记录）→ Close（触发上面的链）
```

两个方向的操作最终都通过同一个 `Close` 入口，`onClose` 里只做"通知外部我关了"，不反过来触发关闭。
