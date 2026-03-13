# Week 10 — 压测

## 一、工具选择

使用 **k6** 做 WebSocket 压测。

| 工具 | 适用场景 |
|------|---------|
| k6 | 支持 WebSocket，脚本灵活 ✅ |
| wrk | 只支持 HTTP，不适合 |
| go test -bench | 单元级别 benchmark，适合测试具体函数 |

---

## 二、压测脚本

脚本位置：`benchmark/`

| 文件 | 用途 |
|------|------|
| `seed.sh` | 批量创建 1100 个测试用户 |
| `ws_connect.js` | WebSocket 连接压测（1000 并发） |
| `ws_message.js` | 消息吞吐压测（50 sender + 50 receiver） |

### 运行步骤

```bash
# 1. 创建测试用户
bash benchmark/seed.sh

# 2. 连接压测
k6 run benchmark/ws_connect.js

# 3. 消息吞吐压测（需先填写 RECEIVER_ID）
k6 run benchmark/ws_message.js
```

---

## 三、连接压测结果（ws_connect.js）

### 压测配置

```
30s 爬升到 100 连接
30s 爬升到 500 连接
60s 爬升到 1000 连接
60s 保持 1000 连接
30s 降到 0
```

### 结果

```
✓ checks          rate=100.00%
✓ ws_connecting   p(95)=746µs   (阈值 <1s)
✓ ws_session_dur  p(95)=30s     (阈值 <35s)
```

| 指标 | 数值 |
|------|------|
| 峰值并发连接 | 1000 |
| WS 连接建立 p(95) | 746µs |
| WS 连接建立中位数 | 384µs |
| WS 连接建立最大值 | 22.53ms |
| HTTP 登录 p(95) | 63ms |
| HTTP 登录中位数 | 49ms |
| 失败率 | 0% |
| 总完成 iterations | 4838 |

### 结论

**1000 并发 WebSocket 连接下服务完全稳定**，连接建立极快（中位数 384µs），无任何失败。

---

## 四、阈值设置经验

压测第一次运行时 `ws_session_duration` 阈值设置不合理：

```js
// 错误：脚本主动保持 30s 连接，session 天然就是 30s，不可能 <5s
'ws_session_duration': ['p(95)<5000']

// 正确：阈值要大于主动保持时长
'ws_session_duration': ['p(95)<35000']
```

**经验：阈值要根据业务逻辑设置，不能脱离实际场景。**

---

## 五、k6 常用指标说明

| 指标 | 含义 |
|------|------|
| `ws_connecting` | WebSocket 握手建立时间 |
| `ws_session_duration` | 从连接建立到关闭的总时长 |
| `ws_sessions` | 总 WebSocket 会话数 |
| `http_req_duration` | HTTP 请求响应时间（不含连接建立） |
| `http_req_failed` | HTTP 请求失败率 |
| `checks` | 自定义检查点通过率 |
| `vus` | 当前活跃虚拟用户数 |

---

## 六、消息吞吐压测结果（ws_message.js）

### 压测配置

```
场景：1 个 receiver + 50 个 sender
sender 每 100ms 发一条消息 → 理论峰值 500 msg/s 全打向同一 receiver
```

### 修复过程（多轮调试）

| 问题 | 根因 | 修复 |
|------|------|------|
| `messages_received=0` | `types.MessageType` 是 `int`，脚本发 `type: 'chat'` 字符串导致 Unmarshal 失败 | 改为 `Type: 0`（MessageTypeText） |
| `messages_received=0` | `MessageService.userService` 未注入，nil pointer panic，服务崩溃 | `NewMessageService` 增加 `userService` 参数并在 `main.go` 传入 |
| `login failed bench_sender_51` | k6 VU 编号从 1 开始，receiver 占 VU1，sender VU 2-51 超出 seed 范围 | `vuIndex = ((__VU - 1) % 50) + 1` |
| Redis key typo | `user:onlien:` 拼写错误 | 改为 `user:online:` |

### 结果

```
messages_sent:     27051  (386 msg/s)
messages_received: 62617  (894 msg/s，含 pender 重试推送)
message_latency:   avg=8.29s  p(95)=21.82s  ❌ 超阈值
```

### 分析与结论

**延迟高的根本原因**：50 个 sender 以 100ms 为间隔向同一 receiver 发消息，理论峰值 **500 msg/s** 集中打向单个 receiver 连接。receiver 的 `writeCh`（buffer=20）很快饱和，Ping 被阻塞，pender 开始重试，形成正反馈，导致延迟爆炸。

**`messages_received > messages_sent`**：pender 超时重试机制将消息重复推送给 receiver，每条消息被推了多次。

**这是符合预期的结果**：单 receiver 是人为制造的极端场景（500 msg/s 单连接），生产中一个用户不会同时被 50 个人高频发消息。

**性能瓶颈**：
1. `writeCh` buffer=20 过小，50 路入流量下极易饱和
2. pender 重试只会加重拥堵，高压下应考虑背压（back-pressure）或丢弃策略

---

## 七、writeCh 优化（buffer 扩容 + Push 超时保护）

### 改动文件

`gateway/ws/ws_conn.go`

### 改动内容

**1. buffer 从 20 扩大到 256**

```go
// before
writeCh: make(chan []byte, 20)

// after
const writeChSize = 256
writeCh: make(chan []byte, writeChSize)
```

单个 receiver 面对 500 msg/s 时，buffer=20 约 40ms 就会打满；buffer=256 可容纳约 0.5s 的突发流量，为 Write goroutine 提供更多缓冲余量。

**2. Push() 改为非阻塞写，writeCh 满时立即丢弃**

```go
// before：writeCh 满时永久阻塞
func (c *WsConn) Push(data []byte) error {
    select {
    case c.writeCh <- data:
        return nil
    case <-c.closeCh:
        return errors.New("connection closed")
    }
}

// after：default 非阻塞，立即返回
func (c *WsConn) Push(data []byte) error {
    select {
    case c.writeCh <- data:
        return nil
    case <-c.closeCh:
        return errors.New("connection closed")
    default:
        // writeCh 满时立即丢弃，由 pender 重试兜底；避免 time.After 的 GC 压力
        return errors.New("write channel full, message dropped")
    }
}
```

**为什么不用 `time.After(100ms)` 超时：**  
高并发下每次调用 `Push()` 都会创建一个新的 timer 对象。大量超时时 timer 堆积，GC 压力显著上升。`default` 是立即返回的零开销操作，无任何额外分配。

**为什么丢弃是安全的：**  
pender 会在 RetryInterval（5s）内对未 ACK 的消息重试推送，所以 `Push` 丢弃本次推送不会造成消息永久丢失。

### 优化效果

| 指标 | buffer=20 | buffer=256 |
|------|-----------|------------|
| latency avg | 8.29s | 8.01s |
| latency p(95) | 21.82s | 18.03s |
| messages_received | 62617 | 72768 |

延迟略有改善，但幅度有限。

### 结论：buffer 不是瓶颈

真正的瓶颈是 **pender 重试雪上加霜**：积压的未 ACK 消息每 5s 重新 Route 一次，导致实际入流量是发送量的 2-3 倍（72768 received vs 27050 sent）。

`Push()` 加超时的价值更大：**buffer 满时快速失败而非永久阻塞**，保护服务端 goroutine 不因单个慢连接级联堆积。极端高压下宁可丢弃该次推送（由 pender 重试兜底），也不阻塞整个调用链。

---

## 八、性能优化方案分析

压测暴露了以下瓶颈，逐一分析方案和取舍。

---

### 8.1 每条消息查一次 Redis 在线状态

**问题**：`SendMessage` 每条消息都 `GetOnlineStatus` → 一次 Redis GET，高并发时 Redis 压力大。

**曾考虑**：改用 ConnManager 内存查询，O(1) 零网络开销。

**决定：不改，保持 Redis。**

原因：Redis 在线状态是为**分布式架构预留**的。多节点部署时，节点 A 上的用户发消息，目标用户可能在节点 B，ConnManager 只有本节点的连接，跨节点查不到。改成 ConnManager 会让单节点和多节点行为不一致，以后扩展时必须重新翻这里，得不偿失。

---

### 8.2 onConnect 同步拉历史消息阻塞连接建立

**问题**：大量用户同时上线时，`FetchHistoryMessages` 查 DB 阻塞 `onConnect`，连接建立被拖慢。

**方案**：goroutine 异步执行，连接立即可用。

```go
func(id string) {
    s.userService.SetOnlineStatus(id, true)
    go func() {
        if err := s.messageService.FetchHistoryMessages(id, 20, time.Now().Unix()); err != nil {
            slog.Warn("fetch history failed, user can pull manually", "id", id, "error", err)
        }
    }()
}
```

**失败了怎么办**：记日志，不做额外处理。历史消息拉取是服务端主动 push，失败了最坏结果是用户看不到最近 20 条历史，但消息已持久化在 DB，数据没丢。用户可以通过 HTTP 接口主动重拉。

**更好的长期方案**：改为客户端驱动——用户上线后主动发一个拉取请求，服务端响应。客户端控制时机，失败可重试，服务端逻辑更简单。当前阶段异步 goroutine 足够，等做客户端时再迁移。

---

### 8.3 群聊串行推送

**问题**：`sendGroupMessage` 逐个成员串行 `Route`，100 人群聊一条消息要等 100 次串行完成。

**方案**：goroutine 并发推送。

```go
g := errgroup.Group{}
for _, member := range members {
    member := member
    g.Go(func() error {
        return s.router.Route(cloneMessage(message, member.UserId))
    })
}
g.Wait()
```

**关于 goroutine 内存压力**：每个 goroutine 初始栈 2KB，1000 人群聊单次推送约 2MB，推送完立即回收，不是常驻内存。真正的压力是高频群消息下 goroutine 的**创建/销毁开销**（调度器压力），而不是内存。

**规模大了用 worker pool**：预创建固定数量 goroutine，通过 channel 分发推送任务，goroutine 数量恒定，调度压力可控。

```go
type pushWorkerPool struct {
    tasks chan pushTask
}

func newPushWorkerPool(size int) *pushWorkerPool {
    p := &pushWorkerPool{tasks: make(chan pushTask, 1000)}
    for i := 0; i < size; i++ {
        go func() {
            for task := range p.tasks {
                task.conn.Push(task.message)
            }
        }()
    }
    return p
}
```

**当前阶段**：群规模有限，直接并发 goroutine 足够，等群规模上来再引入 pool。

---

### 8.4 pender 全局锁竞争 → 分片锁

**问题**：pender 用一把全局 `sync.Mutex` 保护整个 msgList map，高并发 Pend/UnPend 时所有操作串行。

**分片锁原理**：

把一个大 map 拆成 N 个小 map，每个小 map 有自己独立的锁。操作时按 key 的哈希取模决定落到哪个分片，不同分片的操作互不干扰，可以真正并行。

```
msgId → FNV hash → % 32 → 落到某个分片 → 只锁该分片

一把锁（串行）:              32 把分片锁（并行）:
┌──────────────┐            ┌──┐┌──┐┌──┐     ┌──┐
│ msg1         │            │s0││s1││s2│ ... │s31│
│ msg2         │    →       └──┘└──┘└──┘     └──┘
│ msg3         │            不同分片同时操作互不影响
└──────────────┘
```

```go
const shardCount = 32

type pendShard struct {
    mx      sync.Mutex
    msgList map[string]*PendMessage
}

type defaultMessagePender struct {
    shards [shardCount]pendShard
}

func (p *defaultMessagePender) getShard(msgId string) *pendShard {
    h := fnv.New32a()
    h.Write([]byte(msgId))
    return &p.shards[h.Sum32()%shardCount]
}

func (p *defaultMessagePender) Pend(msg *types.Message) error {
    shard := p.getShard(msg.MsgId)
    shard.mx.Lock()
    defer shard.mx.Unlock()
    shard.msgList[msg.MsgId] = &PendMessage{msg: msg}
    return nil
}
```

**效果**：32 个分片 → 理论最多 32 个操作同时执行，锁竞争概率降为原来的 1/32。

**类似设计**：Go 标准库 `sync.Map` 内部 read/dirty 两层结构、Java `ConcurrentHashMap` 经典 16 分片，都是同样思路。

**当前阶段优先级较低**：pender 的每次操作（map 插入/删除）本身极快，锁持有时间很短，实际竞争不严重。瓶颈更多在 Redis 和 DB，分片锁可以留到后期再做。

---

### 8.5 pender 重试雪崩 → 背压控制

**问题**：clearUp 每 5s 无条件重试所有未 ACK 消息。高压下积压越多，重试量越大，writeCh 越堵，形成正反馈。

**方案**：重试前检查目标连接 writeCh 使用率，超过 80% 跳过本轮，等下一轮：

```go
OnMsgRetry: func(msg *types.Message) error {
    conn, _ := connManager.GetConn(msg.ToId)
    if conn == nil {
        return nil
    }
    if wsConn, ok := conn.(*WsConn); ok {
        if len(wsConn.writeCh) > writeChSize*8/10 {
            return nil // 背压，跳过本轮重试
        }
    }
    return s.router.Route(msg)
},
```

---

### 优先级汇总

| 优先级 | 方案 | 难度 | 理由 |
|--------|------|------|------|
| ⭐⭐⭐ | onConnect 异步拉历史 | 低 | 改动一行，消除连接建立阻塞 |
| ⭐⭐⭐ | 群聊并发推送 | 低 | 改动小，群聊场景收益直接 |
| ⭐⭐ | pender 背压控制 | 中 | 防止极端压力下雪崩 |
| ⭐ | pender 分片锁 | 中 | 当前锁持有时间短，竞争不严重 |
| — | ConnManager 替代 Redis | 不做 | 破坏分布式扩展性 |

---

## 九、后续可补充的压测场景

- **稳定性**：保持 500 连接运行 10 分钟，观察内存和 goroutine 是否泄漏（配合 pprof）
- **群聊广播**：100 人群聊，1 条消息触发 99 次推送，测试广播性能
- **合理负载**：每个 receiver 只被 1 个 sender 发消息，50 对并发，测试真实 QPS 和延迟
