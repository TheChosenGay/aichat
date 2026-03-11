# 解耦模式专题

> 起因：实现 `Logout` 时，用户服务需要清理 WebSocket 长连接，但 `UserService` 不应该直接依赖 `ConnManager`（gateway 层）。这个问题引出了几种通用的跨层解耦手段。

---

## 问题背景

分层架构中，高层模块不应该依赖低层模块，否则会破坏依赖方向、增加测试难度。

```
api
 └─ service        ← UserService 在这里
      └─ store

gateway
 └─ ConnManager    ← 需要清理连接，但它不属于 service 层
```

`UserService.Logout()` 需要调用 `ConnManager.RemoveConn()`，但 `service` 不能直接 import `gateway`。

---

## 方案一：接口隔离（推荐）

在 `service` 层定义一个窄接口，只声明自己需要的能力，由外部注入实现。

```go
// service/user.go
// 只声明 service 需要的能力，不关心谁实现
type SessionCleaner interface {
    RemoveConn(userId string) error
}

type defaultUserService struct {
    dbStore        store.UserStore
    redisStore     *store.UserRedisStore
    sessionCleaner SessionCleaner  // 可为 nil（测试时不需要）
}

func (s *defaultUserService) Logout(userId string) error {
    // 1. 使 JWT 失效（清 Redis）
    if err := s.redisStore.DeleteJwt(userId); err != nil {
        return err
    }
    // 2. 清理长连接（可选，不影响核心流程）
    if s.sessionCleaner != nil {
        _ = s.sessionCleaner.RemoveConn(userId)
    }
    return nil
}
```

```go
// main.go：ConnManager 天然满足 SessionCleaner 接口
connManager := gateway.NewConnManager()
userSrv := service.NewUserService(userDbStore, userRedisStore, connManager)
```

**优点：**
- `service` 完全不依赖 `gateway` 包
- `ConnManager` 无需任何改动，已有 `RemoveConn` 方法
- 测试时传 `nil` 或 mock，不需要启 WebSocket

**适用场景：** 跨层调用、需要可测试性时，这是最推荐的方式。这也是本项目中 `MessageRouter` 接口的设计思路——`ConnManager` 实现了 `service` 层定义的 `MessageRouter` 接口，方向完全一致。

---

## 方案二：回调函数注入

不定义接口，直接注入一个函数。比接口更轻量，适合只有一两个方法的场景。

```go
type defaultUserService struct {
    dbStore    store.UserStore
    redisStore *store.UserRedisStore
    OnLogout   func(userId string) error  // 可选回调
}

func (s *defaultUserService) Logout(userId string) error {
    if err := s.redisStore.DeleteJwt(userId); err != nil {
        return err
    }
    if s.OnLogout != nil {
        _ = s.OnLogout(userId)
    }
    return nil
}
```

```go
// main.go：直接绑定方法引用
userSrv := service.NewUserService(userDbStore, userRedisStore)
userSrv.OnLogout = connManager.RemoveConn  // 方法签名恰好匹配
```

**优点：** 极简，不需要定义新接口，适合临时扩展。

**缺点：** 意图不够明确，维护时容易忘记注入；多个回调时代码变乱。

---

## 方案三：事件总线 / 发布订阅

业务层发布事件，感兴趣的模块订阅处理。适合一对多的通知场景。

```go
// 定义事件
type UserLoggedOutEvent struct {
    UserId string
}

// service 只负责发布事件，不关心谁处理
func (s *defaultUserService) Logout(userId string) error {
    if err := s.redisStore.DeleteJwt(userId); err != nil {
        return err
    }
    s.eventBus.Publish(UserLoggedOutEvent{UserId: userId})
    return nil
}
```

```go
// gateway 层订阅事件
eventBus.Subscribe(func(e UserLoggedOutEvent) {
    connManager.RemoveConn(e.UserId)
})
```

**优点：** 彻底解耦，发布方完全不知道有多少订阅者；新增处理逻辑不需要改业务代码。

**缺点：** 引入额外复杂度，事件流难以追踪，不适合简单场景。

**适用场景：** 一个事件需要触发多个副作用时（如 logout 同时要：踢 WS 连接、记录日志、通知好友下线）。

---

## 方案四：直接注入具体类型（反例）

把 `ConnManager` 直接传给 `UserService`。

```go
// ❌ 不推荐
type defaultUserService struct {
    connManager *gateway.ConnManager  // service 层 import 了 gateway 层
}
```

**问题：**
- 打破分层方向，`service` → `gateway` 形成向下依赖
- 单元测试必须启动一个真实的 `ConnManager`
- `UserService` 和 `ConnManager` 强绑定，无法独立演化

---

## 并发场景：锁 + 回调的死锁陷阱与 Actor 模型

### 问题根源

回调函数会把"调用者不可见的依赖"带进来，当回调在持有锁的情况下被调用时，极易形成死锁。

**典型死锁场景（来自本项目 `message.go:UnPend`）：**

```
UnPend() 持有 mx 锁
  → 调用 OnMsgAcked 回调
    → 回调实现调用 SendMessage
      → SendMessage 调用 pender.Pend()
        → Pend() 尝试获取 mx 锁 → 死锁
```

这类 bug 的特点是：
- 代码审查时往往看不出来，因为问题跨越多个函数甚至多个文件
- 必须在特定的并发时序下才会触发
- 发生时进程静默挂起，日志没有任何报错

### 短期修复：回调永远在锁外调用

规则：**加锁、修改数据、解锁，然后再调用回调**，不允许在持有锁时调用任何外部函数。

```go
// ❌ 危险：锁内调用回调
func (p *defaultMessagePender) UnPend(msgId string) error {
    p.mx.Lock()
    defer p.mx.Unlock()
    pm := p.msgList[msgId]
    delete(p.msgList, msgId)
    p.opt.OnMsgAcked(pm.msg)  // 持有锁，危险
    return nil
}

// ✅ 安全：先释放锁，再调用回调
func (p *defaultMessagePender) UnPend(msgId string) error {
    p.mx.Lock()
    pm, ok := p.msgList[msgId]
    if !ok {
        p.mx.Unlock()
        return errors.New("message not found")
    }
    delete(p.msgList, msgId)
    p.mx.Unlock()           // 先解锁

    p.opt.OnMsgAcked(pm.msg)  // 再回调
    return nil
}
```

### 根本解法：Actor 模型（用 Channel 代替锁）

Actor 模型的核心思想：**每个 Actor 是一个独立的 goroutine，独占自己的状态，外部只能通过发消息来请求操作，不直接访问内部数据。**

这从根本上消灭了锁，因为数据只在一个 goroutine 里被访问，天然串行。

```
外部调用者                    Actor (单 goroutine)
─────────────────────────────────────────────────
Pend(msg)    ──── pendReq ──→  msgList[id] = msg
                                    ↓
             ←── nil/err ────  respCh <- result

UnPend(id)   ── unpendReq ──→  msg = msgList[id]
                                delete(msgList, id)
             ←── nil/err ────  respCh <- result
                                OnMsgAcked(msg)   // 回调在 Actor 内，无锁，安全
```

**实现：**

```go
// 用接口类型统一所有命令，loop 里用 type switch 分发
type pendCmd interface{ isPendCmd() }

type pendReq struct {
    msg    *types.Message
    respCh chan error
}
type unpendReq struct {
    msgId  string
    respCh chan error
}

func (pendReq) isPendCmd()   {}
func (unpendReq) isPendCmd() {}

type defaultMessagePender struct {
    opt   MessagePendOpts
    cmdCh chan pendCmd
}

func NewMessagePender(opt MessagePendOpts) MessagePender {
    p := &defaultMessagePender{
        opt:   opt,
        cmdCh: make(chan pendCmd, 32),
    }
    go p.loop()
    return p
}

// loop 是唯一操作 msgList 的地方，完全串行，零锁
func (p *defaultMessagePender) loop() {
    msgList := make(map[string]*PendMessage)
    ticker := time.NewTicker(p.opt.RetryInterval)
    defer ticker.Stop()

    for {
        select {
        case cmd := <-p.cmdCh:
            switch c := cmd.(type) {
            case pendReq:
                if _, ok := msgList[c.msg.MsgId]; ok {
                    c.respCh <- errors.New("message already pend")
                    continue
                }
                msgList[c.msg.MsgId] = &PendMessage{
                    msg:         c.msg,
                    lastRetryAt: time.Now().Unix(),
                }
                c.respCh <- nil

            case unpendReq:
                pm, ok := msgList[c.msgId]
                if !ok {
                    c.respCh <- errors.New("message not found")
                    continue
                }
                delete(msgList, c.msgId)
                c.respCh <- nil
                p.opt.OnMsgAcked(pm.msg)  // 回调安全：没有锁，不会死锁
            }

        case <-ticker.C:
            now := time.Now().Unix()
            var toRetry, toFail []*PendMessage
            for _, pm := range msgList {
                expired := now >= pm.msg.SendAt+int64(p.opt.TTL.Seconds())
                if expired || pm.retryCount >= p.opt.MaxRetry {
                    toFail = append(toFail, pm)
                } else if now >= pm.lastRetryAt+int64(p.opt.RetryInterval.Seconds()) {
                    toRetry = append(toRetry, pm)
                }
            }
            for _, pm := range toFail {
                delete(msgList, pm.msg.MsgId)
                p.opt.OnMsgFailed(pm.msg)
            }
            for _, pm := range toRetry {
                pm.lastRetryAt = now
                pm.retryCount++
                p.opt.OnMsgRetry(pm.msg)
            }
        }
    }
}

// 对外接口：发消息给 Actor，等待响应
func (p *defaultMessagePender) Pend(msg *types.Message) error {
    resp := make(chan error, 1)
    p.cmdCh <- pendReq{msg: msg, respCh: resp}
    return <-resp
}

func (p *defaultMessagePender) UnPend(msgId string) error {
    resp := make(chan error, 1)
    p.cmdCh <- unpendReq{msgId: msgId, respCh: resp}
    return <-resp
}
```

### 两种方案对比

| | 锁 + 回调 | Actor（Channel） |
|---|---|---|
| 并发控制 | `sync.Mutex` | 单 goroutine 串行 |
| 回调调用位置 | 锁持有期间（危险） | loop 内，无锁（安全） |
| 死锁可能性 | 高，回调引入不可见依赖 | 无，状态只在一处 |
| 调用方感知 | 竞争锁时阻塞，无超时 | 可加 context 超时，行为可控 |
| 可测试性 | 需要模拟并发时序 | 串行执行，行为确定 |
| 适用场景 | 简单的共享状态保护 | 有回调/事件、状态变化复杂 |

### 什么时候用锁，什么时候用 Actor

**用锁（`sync.Mutex`）：** 操作简单、持锁时间极短、锁内绝对不调用任何外部函数。

**用 Actor（Channel）：** 持有状态的同时还需要对外发通知（回调、事件）、状态变化逻辑复杂、需要定时任务与外部操作交织执行。

---

## 选择指南

| 场景 | 推荐方案 |
|------|----------|
| 跨层调用，需要可测试 | **接口隔离** |
| 单个回调，临时扩展 | **回调函数** |
| 一个事件 → 多个处理方 | **事件总线** |
| 同层调用，无需解耦 | 直接依赖即可 |
| 持有状态 + 需要回调/通知 | **Actor 模型** |
| 简单共享状态，无回调 | **锁（谨慎）** |

---

## 本项目的实践

| 解耦点 | 采用方案 | 接口位置 |
|--------|----------|----------|
| `MessageService` 推送消息 → `ConnManager` | 接口隔离 | `service/router.go` 定义 `MessageRouter` |
| `UserService` 登出 → 清理 WS 连接 | 接口隔离 | `service/user.go` 定义 `SessionCleaner` |
