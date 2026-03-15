# Week 12 — 调试与生产问题记录

## 一、问题 1：消息发送死锁

### 问题描述

发送消息后，客户端连接断开（code: 1006），服务端无任何响应。

### 排查过程

1. 加日志发现：消息收到后卡在 `pender.Pend()` 方法
2. `pend start` 日志有打印，但 `pend got lock` 没有
3. 说明 `p.mx.Lock()` 一直阻塞

### 根因分析

```go
// clearUp 持有锁后调用回调，回调又调用 SendMessage，SendMessage 调用 Pend 尝试获取锁
// 造成死锁：clearUp 持有锁 → 调用 OnMsgRetry → SendMessage → Pend → 等待锁（自己）
func (p *defaultMessagePender) clearUp() {
    p.mx.Lock()
    // ...
    p.opt.OnMsgRetry(msg.msg)  // 这里会调用 SendMessage，可能导致死锁
    // ...
}
```

### 解决方案

**先复制需要处理的消息，释放锁，再调用回调**：

```go
func (p *defaultMessagePender) clearUp() {
    // ...
    p.mx.Lock()

    // 1. 先复制需要处理的消息
    var toRetry []*PendMessage
    var toFail []*PendMessage
    for _, msg := range p.msgList {
        // ...判断逻辑
        toRetry = append(toRetry, msg)
    }
    p.mx.Unlock()  // 2. 释放锁

    // 3. 在锁外调用回调
    for _, msg := range toRetry {
        p.opt.OnMsgRetry(msg.msg)
    }
}
```

### 经验总结

- **在持锁时避免调用回调函数**，尤其是回调可能跨协程或调用其他加锁方法
- 设计模式：如果回调可能调用回加锁区域，**先复制数据，释放锁，再处理**

---

## 二、问题 2：SQL 字段顺序不匹配

### 问题描述

```
Error 1136 (21S01): Column count doesn't match value count at row 1
```

### 根因

SQL 语句字段顺序和 Go 代码传参顺序不一致。

### 解决方案

确保 INSERT 语句字段顺序和参数顺序一致：

```sql
-- SQL
INSERT INTO messages (msg_id, from_id, to_id, type, content, send_at, is_delivered, room_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
```

```go
// Go - 参数顺序必须对应
message.MsgId, message.FromId, message.ToId, message.Type,
message.Content, message.SendAt, message.IsDelivered, message.RoomId
```

### 经验总结

- MySQL INSERT 字段列表顺序要和 VALUES 参数顺序完全对应
- 迁移添加新字段后（如 room_id），注意检查代码中的 SQL 语句

---

## 三、问题 3：SendAt 字段缺失

### 问题描述

```
Key: 'Message.SendAt' Error:Field validation for 'SendAt' failed on the 'gt' tag
```

### 解决方案

在服务端自动填充 SendAt，不依赖客户端传入：

```go
message.FromId = id
message.SendAt = time.Now().Unix()  // 服务端生成时间戳
```

---

## 四、其他小问题

- **表名拼写错误**：查询时打错 `messages` 为 `messsages`
- **迁移未执行**：数据库表缺少 room_id 列，手动执行迁移后解决

---

## 五、调试技巧

1. **分层加日志**：在关键路径（receive → sendmessage → pend → route）逐层加日志定位卡点
2. **日志时间戳**：观察日志时间间隔判断卡在哪里（如 pend start 到 pend got lock 间隔 30s 说明死锁）
3. **错误码**：WebSocket 1006 表示异常断开，通常是服务端 panic 或处理出错

---

## 六、Week 6 功能完成：ACK + 重试机制

### 实现概述

已完成消息可靠送达机制：

1. **消息发送流程**：
   - 客户端发送消息 → 服务端存储 MySQL → 加入 pending → 路由给接收者
   - 接收者在线：实时推送
   - 接收者不在线：消息已持久化

2. **ACK 确认机制**：
   - 客户端收到消息后发送 ACK 帧：`{"type":"ack","msgId":"xxx"}`
   - 服务端收到 ACK 后：从 pending 移除，更新 `isDelivered = true`

3. **超时重试机制**：
   - 配置：TTL 30s，MaxRetry 3 次，RetryInterval 5s
   - 超时未 ACK：自动重发消息（最多 3 次）
   - 重试次数用完或超时：标记消息失败

### 核心代码

```go
// Pender 接口
type MessagePender interface {
    Pend(msg *types.Message) error  // 加入 pending
    UnPend(msgId string) error      // 移除 pending（收到 ACK）
}

// 重试回调 - 直接路由，不重复存储
OnMsgRetry: func(msg *types.Message) error {
    msg.SendAt = time.Now().Unix()
    return s.router.Route(msg)
}
```

### 经验总结

- **重试不走 SendMessage**：避免重复存储和重复 Pend，直接调用 `router.Route`
- **避免死锁**：clearUp 先复制消息，释放锁，再调用回调
- **服务端生成时间戳**：SendAt 由服务端填充，不依赖客户端

---

## 七、问题 N：ConnManager.RemoveConn 死锁（加锁后所有返回路径必须解锁）

### 问题描述

`ConnManager.RemoveConn` 加锁后，在 `!ok`（连接不存在）的提前返回分支忘记解锁，
后续任何调用该锁的代码全部阻塞，造成死锁。

### 根因

```go
// ❌ 错误：!ok 分支直接 return，忘记 Unlock
func (m *ConnManager) RemoveConn(id string) {
    m.mx.Lock()
    _, ok := m.conns[id]
    if !ok {
        return  // 锁没释放！后续所有 Lock() 永远阻塞
    }
    delete(m.conns, id)
    m.mx.Unlock()
}
```

### 解决方案

**方法一：每个提前返回前都手动 Unlock**

```go
func (m *ConnManager) RemoveConn(id string) {
    m.mx.Lock()
    _, ok := m.conns[id]
    if !ok {
        m.mx.Unlock()  // ✅ 每个 return 前都解锁
        return
    }
    delete(m.conns, id)
    m.mx.Unlock()
}
```

**方法二（推荐）：加锁后立即 defer Unlock，彻底消灭漏解锁**

```go
func (m *ConnManager) RemoveConn(id string) {
    m.mx.Lock()
    defer m.mx.Unlock()  // ✅ 函数任何路径退出都会执行

    _, ok := m.conns[id]
    if !ok {
        return  // defer 保证这里也会 Unlock
    }
    delete(m.conns, id)
}
```

### 经验总结

> **规则：加锁后，立即写 `defer Unlock()`。不要在函数末尾手动 Unlock，因为任何提前 return 都会漏掉它。**

- 只有一种例外：需要在 Unlock 后才能调用回调/外部函数时（避免锁内回调死锁），才手动控制 Unlock 时机（见问题 1）
- 其余情况一律 `defer Unlock()`，不给自己留漏洞

---

## 八、后续改进建议

1. **避免回调嵌套**：Pender 的回调设计容易造成死锁，考虑用 channel 替代
2. **统一错误日志**：Save、Route 等关键方法加错误日志
3. **单元测试**：针对 Pender 的并发场景写测试，避免死锁
4. **离线消息补偿**：用户重连后主动拉取未送达消息
