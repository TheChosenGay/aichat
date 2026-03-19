# Week 13 — Redis Pub/Sub 消息路由

## 一、为什么要用 Redis Pub/Sub 做路由

原有的 `ConnManager.Route` 直接在内存里查找连接并推送，是纯单机方案：

```
MessageService.SendMessage
    → ConnManager.Route
        → conns[toId].Push(data)  // 直接找本机内存中的连接
```

**问题**：多实例部署时，用户 A 连在实例 1，用户 B 连在实例 2，B 给 A 发消息时实例 2 的 `ConnManager` 找不到 A 的连接，消息丢失。

**Redis Pub/Sub 解法**：每个用户连接后订阅自己的 Redis Channel，发消息时 Publish 到目标用户的 Channel，无论目标用户连在哪个实例，都能收到。

```
实例 2：MessageService.SendMessage
    → RedisMsgRouter.Route
        → Redis PUBLISH channel:user:{toId}  ← 广播到 Redis
            ↓
        Redis 推给所有订阅者
            ↓
实例 1：Subscribe goroutine 收到消息
    → localRouter.Route (ConnManager)
        → conn.Push(data)  ← 写入本机 WebSocket
```

---

## 二、实现设计

### 整体架构

```
RedisMsgRouter
├── Route(msg)        → Publish 到 Redis Channel
├── RouteGroup(msg)   → 并发 Publish 给所有成员
├── Subscribe(userId) → 订阅 Channel + 启动消费 goroutine
└── Unsubscribe(userId) → 关闭订阅 + 从 map 删除

内部字段：
├── localRouter    → ConnManager（本机 WS 推送）
├── redisStore     → Redis 操作封装
└── userChannlsMap → userId → *redis.PubSub（订阅句柄）
```

### Channel 命名

```go
func (s *UserRedisStore) UserChannelName(userId string) string {
    return "channel:user:" + userId
}
```

与在线状态 key（`user:online:{userId}`）使用不同前缀，避免混用。

### 核心代码

```go
type RedisMsgRouter struct {
    redisStore     *store.UserRedisStore
    mtx            sync.Mutex
    userChannlsMap map[string]*redis.PubSub
    localRouter    service.MessageRouter  // 本机 WS 推送
}

// Subscribe：订阅 + 启动消费 goroutine
func (s *RedisMsgRouter) Subscribe(userId string) error {
    pubsub := s.redisStore.SubUser(userId)
    s.mtx.Lock()
    s.userChannlsMap[userId] = pubsub
    s.mtx.Unlock()  // 先释放锁，再启动 goroutine

    go func() {
        for msg := range pubsub.Channel() {
            var message types.Message
            if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
                slog.Error("Failed to unmarshal message", "error", err.Error())
                continue
            }
            s.localRouter.Route(&message)
        }
        // pubsub.Close() 后 Channel() 会关闭，goroutine 自然退出
    }()
    return nil
}

// Unsubscribe：关闭订阅 + 从 map 删除
func (s *RedisMsgRouter) Unsubscribe(userId string) error {
    s.mtx.Lock()
    defer s.mtx.Unlock()
    pubsub, ok := s.userChannlsMap[userId]
    if !ok {
        return errors.New("user not found")
    }
    if err := s.redisStore.UnsubUser(pubsub); err != nil {
        return err  // 关闭失败，不删 map，调用方可重试
    }
    delete(s.userChannlsMap, userId)
    return nil
}

// Route：Publish 单播
func (s *RedisMsgRouter) Route(message *types.Message) error {
    return s.redisStore.Publish(message.ToId, message)
}

// RouteGroup：并发 Publish 群播
func (s *RedisMsgRouter) RouteGroup(message *types.Message, memberIds []string) error {
    eg := errgroup.Group{}
    eg.SetLimit(40)  // 防止大群时 goroutine 爆炸
    for _, memberId := range memberIds {
        eg.Go(func() error {
            return s.redisStore.Publish(memberId, message)
        })
    }
    return eg.Wait()
}
```

### WsServer 接入点

```go
// 用户连接时：订阅
func(id string) {
    s.userService.SetOnlineStatus(id, true)
    if err := s.redisMsgRouter.Subscribe(id); err != nil {
        slog.Error("Failed to subscribe", "error", err.Error())
    }
    go func() {
        s.messageService.FetchHistoryMessages(id, 20, time.Now().Unix())
    }()
},

// 用户断开时：取消订阅
func(id string) {
    s.ConnManager.RemoveConn(id)
    s.userService.SetOnlineStatus(id, false)
    if err := s.redisMsgRouter.Unsubscribe(id); err != nil {
        slog.Error("Failed to unsubscribe", "error", err.Error())
    }
},
```

---

## 三、踩坑记录

### 坑 1：goroutine 在持有锁时启动

```go
// ❌ 错误：go func() 在 mtx.Lock() 和 mtx.Unlock() 之间启动
func (s *RedisMsgRouter) Subscribe(userId string) error {
    pubsub := s.redisStore.SubUser(userId)
    s.mtx.Lock()
    s.userChannlsMap[userId] = pubsub
    go func() {                      // ← 锁还没释放
        for msg := range pubsub.Channel() {
            s.localRouter.Route(...)  // ← 如果 Route 内部加锁，死锁
        }
    }()
    s.mtx.Unlock()
    return nil
}

// ✅ 正确：先 Unlock，再启动 goroutine
func (s *RedisMsgRouter) Subscribe(userId string) error {
    pubsub := s.redisStore.SubUser(userId)
    s.mtx.Lock()
    s.userChannlsMap[userId] = pubsub
    s.mtx.Unlock()  // 先释放锁

    go func() {     // 再启动 goroutine
        ...
    }()
    return nil
}
```

**规则：不要在持有锁的时候启动 goroutine，goroutine 内的代码可能回调任何地方，无法预判是否会再次加锁。**

### 坑 2：Unsubscribe 忘记从 map 删除

```go
// ❌ 错误：关闭了 pubsub 但没有从 map 删除
func (s *RedisMsgRouter) Unsubscribe(userId string) error {
    s.mtx.Lock()
    defer s.mtx.Unlock()
    pubsub, _ := s.userChannlsMap[userId]
    return s.redisStore.UnsubUser(pubsub)
    // map 里还留着这个 userId，内存泄漏
    // 用户重连时写入新 pubsub 虽然会覆盖，但时序不确定
}

// ✅ 正确：先关闭，成功后再删 map
func (s *RedisMsgRouter) Unsubscribe(userId string) error {
    s.mtx.Lock()
    defer s.mtx.Unlock()
    pubsub, ok := s.userChannlsMap[userId]
    if !ok {
        return errors.New("user not found")
    }
    if err := s.redisStore.UnsubUser(pubsub); err != nil {
        return err  // 关闭失败，不删 map
    }
    delete(s.userChannlsMap, userId)
    return nil
}
```

### 坑 3：不能用 defer 来"先操作后清理"

这是 `defer` 的一个常见误用场景：

```go
// ❌ 错误：用 defer delete 期望"最后执行删除"
func (s *RedisMsgRouter) Unsubscribe(userId string) error {
    s.mtx.Lock()
    defer s.mtx.Unlock()
    pubsub, _ := s.userChannlsMap[userId]
    defer delete(s.userChannlsMap, userId)  // ← 无论成功失败都会执行
    return s.redisStore.UnsubUser(pubsub)   // ← 如果这里返回 error
    // error 返回后，defer delete 仍然执行
    // 结果：pubsub 关闭失败，但 map 里的记录已经被删了
    // pubsub 泄漏：既没有被正确关闭，也没有人持有引用能再次关闭它
}
```

**`defer` 的执行是无条件的**，不管函数是正常返回还是 error 返回，所有 defer 都会执行。

**规则：`defer` 适合"无论如何都要做"的清理（释放锁、关闭文件）。如果清理动作依赖于前置操作是否成功，就不能用 `defer`，应该显式判断后再执行。**

```
defer 适合的场景：
  ✅ defer mu.Unlock()      — 无论如何都要解锁
  ✅ defer file.Close()     — 无论如何都要关文件
  ✅ defer cancel()         — 无论如何都要取消 context

defer 不适合的场景：
  ❌ defer delete(map, key) — 只有前置操作成功才应该删
  ❌ defer db.Commit()      — 只有无 error 才应该提交
```

---

## 四、Subscribe 未被调用时的消息命运

如果用户连接了 WebSocket，但没有执行 `Subscribe`（比如调用失败），`Route` 仍然会把消息 Publish 到 Redis Channel。因为没有订阅者，Redis 会直接丢弃这条消息（Pub/Sub 是 fire-and-forget，没有消息持久化）。

消息已经存入 MySQL，用户下次连接时通过 `FetchHistoryMessages` 可以补拉。但这意味着用户在本次连接期间发给他的消息，当次是收不到实时推送的。

**结论**：`Subscribe` 失败时，应该直接关闭这次 WebSocket 连接，让客户端重连，而不是继续建立一个"无法实时收消息"的半残连接。

---

## 五、与旧路由的对比

| 维度 | ConnManager.Route（旧） | RedisMsgRouter.Route（新） |
|------|------------------------|--------------------------|
| 路由方式 | 内存 map 查连接 | Redis Pub/Sub |
| 多实例支持 | ❌ 单机 | ✅ |
| 推送延迟 | 极低（纳秒级） | 低（Redis RTT，通常 < 1ms） |
| 用户不在线 | 静默跳过 | Publish 但无订阅者，Redis 丢弃 |
| 消息持久化 | 无 | 无（Pub/Sub 不持久化） |
| 实现复杂度 | 低 | 中 |
