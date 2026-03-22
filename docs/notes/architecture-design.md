# 架构设计优化思路笔记

## 一、思考框架（层层递进）

```
SOLID 原则        ← 最基础，告诉你"好代码的标准是什么"
     ↓
分层架构          ← 告诉你"代码应该怎么组织"
     ↓
依赖注入          ← 告诉你"依赖应该怎么传递"
     ↓
领域驱动设计(DDD) ← 告诉你"业务逻辑应该怎么建模"
```

---

## 二、SOLID 原则（最相关的三条）

### S — 单一职责（SRP）
**一个类/函数只有一个改变的理由。**

判断方法：能不能用一句话说清楚这个类的职责？如果要用"和"连接两件事，就违反了。

症状：方法越来越长，依赖越来越多。

### O — 开闭原则（OCP）
**对扩展开放，对修改关闭。**

理想状态：加功能不改老代码，而是"插入"新行为。事件驱动是实现 OCP 的手段之一。

### D — 依赖倒置（DIP）
**高层模块不依赖低层模块，两者都依赖抽象（接口）。**

```
错误方向：MessageService → ConversationService（具体实现）

正确方向：MessageService → OnMessageSavedHandler（接口）
                                ↑
               ConversationService 实现这个接口
```

---

## 三、分层架构

规则：**依赖只能从上往下，不能反向，不能跨层。**

```
┌─────────────────────────────┐
│      Presentation Layer      │  HTTP Handler、WebSocket Handler
│   （只负责输入输出转换）       │  不包含业务逻辑
├─────────────────────────────┤
│      Application Layer       │  UseCase（用例）
│   （编排业务流程）             │  知道"流程是什么"，不知道"怎么做"
├─────────────────────────────┤
│       Domain Layer           │  Service、Entity、业务规则
│   （核心业务逻辑）             │  不依赖任何外部框架
├─────────────────────────────┤
│    Infrastructure Layer      │  DB、Redis、消息队列、HTTP 客户端
│   （技术实现细节）             │  实现 Domain 层定义的接口
└─────────────────────────────┘
```

**本项目问题**：缺少 Application Layer。`MessageService` 同时充当了 Application（编排）
和 Domain（业务规则）两层，导致它需要依赖所有其他 Service。

---

## 四、依赖注入（DI）

**把"创建依赖"和"使用依赖"分开。**

本项目已经在用 DI（`NewXxxService` 传参），但传入的是具体类型而非接口：

```go
// 现在（具体类型）
type defaultConversationService struct {
    conversationDbStore *store.ConversationDbStore
}

// 应该是（接口）
type defaultConversationService struct {
    conversationStore ConversationRepository
}
```

---

## 五、DDD 中最实用的三个概念

### 1. 聚合根（Aggregate Root）
把相关的数据和行为封装在一起，通过一个入口访问。

```go
// 不好：外部 Service 操作字段
conversation.UnreadCount++
conversation.LastMsgContent = msg.Content
conversationService.Update(conversation)

// 好：聚合根封装行为，外部只表达意图
func (c *Conversation) ReceiveMessage(msg *Message, senderName string) {
    c.LastMsgContent = msg.Content
    c.LastMsgTime    = msg.SendAt
    c.LastSenderName = senderName
    c.UnreadCount++
}
conversation.ReceiveMessage(msg, senderName)
repo.Save(conversation)
```

### 2. 仓储模式（Repository）
把持久化细节从业务逻辑隔离出去。Domain 层定义接口，Infrastructure 层实现。

```go
// Domain 层定义（不知道 MySQL 的存在）
type ConversationRepository interface {
    FindByUserAndPeer(userId, peerId string) (*Conversation, error)
    Save(conv *Conversation) error
}

// Infrastructure 层实现（MySQL 细节在这里）
type mysqlConversationRepo struct{ db *sql.DB }
func (r *mysqlConversationRepo) Save(conv *Conversation) error { ... }
```

如果要换 PostgreSQL 或加 Redis 缓存，只改 Infrastructure 层。

### 3. 领域事件（Domain Event）
业务行为发生时发出事件，让其他模块自己决定是否响应。详见下方"三种解耦方式"。

---

## 六、同级模块间解耦的三种方式

> 问题背景：`MessageService` 和 `ConversationService` 是平级关系，
> 但发消息时需要更新会话，怎么解耦？

### 方式一：回调函数

```go
type MessageService struct {
    onMessageSaved func(msg *types.Message) error  // 注入回调
}

// 注册时传入
msgService := NewMessageService(store, router,
    func(msg *types.Message) error {
        return conversationService.HandleMessageSaved(msg)
    },
)
```

**优势**
- 实现最简单，Go 原生支持，零额外概念
- 适合"只有一个处理方"的场景
- 调用链清晰，出错容易追踪

**劣势**
- 只能注册一个回调（除非用 slice）
- 回调签名是具体类型，耦合在函数签名上
- 多个回调时需要手动管理注册/注销
- 适合小项目，扩展性差

**适用场景**：处理方固定且唯一，比如 Pender 里的 OnMsgFailed/OnMsgAcked，本项目已经在用。

---

### 方式二：接口（Observer 模式）

```go
// MessageService 定义接口（使用方定义，不是实现方）
type MessageSavedHandler interface {
    OnMessageSaved(msg *types.Message) error
}

type MessageService struct {
    handlers []MessageSavedHandler  // 可注册多个
}

func (s *MessageService) AddHandler(h MessageSavedHandler) {
    s.handlers = append(s.handlers, h)
}

// ConversationService 实现接口
func (s *ConversationService) OnMessageSaved(msg *types.Message) error {
    return s.updateBothSides(msg)
}

// main.go 组装
msgService.AddHandler(conversationService)
msgService.AddHandler(notificationService)  // 未来可以继续加
```

**优势**
- 可注册多个处理方，符合开闭原则（加功能不改 MessageService）
- 接口在使用方定义，依赖方向正确（DIP）
- 类型安全，编译期检查
- Go 惯用模式，团队容易理解

**劣势**
- 调用仍然是同步的，处理方阻塞会影响发送方
- 处理方之间有隐式顺序依赖
- 接口方法签名变了，所有实现都要改

**适用场景**：处理方有多个但数量可预期，需要同步完成，比如本项目当前阶段最合适用这种。

---

### 方式三：事件总线（Event Bus）

```go
// 定义事件（纯数据结构，不包含行为）
type MessageSentEvent struct {
    Message *types.Message
    Members []string
}

// 事件总线接口
type EventBus interface {
    Publish(event any)
    Subscribe(eventType any, handler func(event any))
}

// MessageService 只发布事件，不知道谁处理
func (s *MessageService) SendMessage(msg *types.Message) error {
    s.store.Save(msg)
    s.router.Route(msg)
    s.bus.Publish(MessageSentEvent{Message: msg})  // 发完不管
    return nil
}

// ConversationService 自己订阅
func (s *ConversationService) Register(bus EventBus) {
    bus.Subscribe(MessageSentEvent{}, func(e any) {
        event := e.(MessageSentEvent)
        s.updateBothSides(event.Message)
    })
}
```

**优势**
- 完全解耦：发布方不知道订阅方存在，双方可以独立演化
- 天然支持多订阅方，加功能完全不改发布方
- 事件可以持久化、重放、审计
- 为未来分布式架构（Kafka/RabbitMQ）打基础，替换成本低

**劣势**
- 调用链不直观，出错难追踪（"这个事件是谁发的？谁处理了？"）
- 需要额外实现或引入 EventBus
- 异步事件下需要考虑顺序性、幂等性、失败重试
- 过度使用会让系统变成"意大利面条"式的隐式依赖网络
- 小项目引入增加理解成本

**适用场景**：处理方不确定、未来会扩展、或需要异步处理时。

---

### 三种方式对比

| 维度 | 回调函数 | 接口（Observer） | 事件总线 |
|------|---------|----------------|---------|
| 实现复杂度 | 低 | 中 | 高 |
| 可扩展性 | 差（单一） | 中（有限多个） | 强（无限多个） |
| 耦合程度 | 中（耦合签名） | 低（耦合接口） | 极低（耦合事件类型） |
| 调用链可见性 | 清晰 | 清晰 | 不透明 |
| 同步/异步 | 同步 | 同步 | 两者皆可 |
| 适合阶段 | 原型/小项目 | 中型项目 | 大型/分布式系统 |

**本项目建议**：现阶段用**接口（Observer）**，理由：
1. 比回调更有扩展性（未来加推送通知直接 AddHandler）
2. 比事件总线更直观，出了问题容易调试
3. 接口定义在 `MessageService` 包里，依赖方向正确

未来如果要加消息推送（APNs/FCM）、审计日志、用户行为统计等多个处理方，
再升级成 EventBus，届时只需在 main.go 替换注册方式，业务代码不动。

---

## 七、本项目重构路线图

### 当前依赖图（有问题）
```
MessageService
  ├── UserService        ← 查在线状态、查用户名
  ├── RoomService        ← 查群成员
  ├── ConversationService
  │     └── UserService  ← 查用户名（重复依赖）
  └── MessageRouter
```

### 目标依赖图
```
SendMessageUseCase (新增 Application Layer)
  ├── MessageRepository     (接口，替代 MessageStore)
  ├── ConversationRepository (接口，替代 ConversationDbStore)
  ├── UserRepository        (接口，只查在线状态)
  ├── RoomRepository        (接口，只查成员 ID)
  └── MessageRouter         (接口，不变)

MessageService (瘦身后，只负责 Pending/ACK/Retry)
ConversationService (不再依赖 UserService)
```

### 分步重构计划

**Step 1（低风险）**：给所有 Store 加 Repository 接口
```
store/conversation_repository.go  ← 接口定义
store/conversation_db.go          ← 实现接口（代码基本不变）
```

**Step 2（中风险）**：用接口（Observer）解开 MessageService → ConversationService 依赖
```go
// service/message_hooks.go
type MessageSavedHandler interface {
    OnMessageSaved(msg *types.Message) error
}
// MessageService 持有 []MessageSavedHandler，发消息后遍历调用
// ConversationService 实现 OnMessageSaved
// main.go: msgService.AddHandler(conversationService)
```

**Step 3（中风险）**：ConversationService 中消除 UserService 依赖
```
// SendMessage 时由 UseCase 层查好 senderName，直接传入
// ConversationService.HandleMessageSaved(msg, senderName string)
```

**Step 4（较大重构）**：抽出 UseCase 层
```
usecase/
  send_message.go    ← 编排：保存→路由→触发 Handler
  open_conversation.go
  fetch_history.go
```

---

## 八、写代码前的检查清单

```
1. 职责边界
   □ 这个类/函数用一句话能说清职责吗？
   □ 它有几个"改变的理由"？超过1个就要拆分。

2. 依赖方向
   □ 我依赖的是接口还是具体类型？
   □ 高层模块（编排）有没有直接依赖低层模块（实现）？
   □ 两个平级模块之间有没有直接依赖？→ 考虑接口或事件

3. 分层
   □ 这段代码是"流程编排"还是"业务规则"还是"技术实现"？
   □ 分别放在 UseCase / Service / Repository 层

4. 接口设计
   □ 接口方法数量是否最小？
   □ 接口定义在"使用方"所在的包，不在"实现方"

5. 可测试性（最直观的验证手段）
   □ 不启动数据库能测试这个 Service 吗？
   □ 如果不能，说明依赖了具体实现而不是接口
   □ 可测试性差 = 依赖管理差，这是最直接的指标
```
