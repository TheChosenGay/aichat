# Week 11 — 架构改进：依赖注入、配置管理与事务

## 一、架构问题诊断

### 当前问题

1. **main.go 手动管理依赖** — 层层传递，难以测试
2. **配置散落** — `os.Getenv` 在多处调用，缺乏统一管理
3. **缺乏事务支持** — 多步骤操作（如消息存储+推送）无法保证一致性

---

## 二、依赖管理与生命周期

### 2.1 当前问题

```go
// main.go - 手动创建，层层传递
userDbStore := store.NewUserDbStore(db)
userRedisStore := store.NewUserRedisStore(...)
userSrv := service.NewUserService(userDbStore, userRedisStore)
apiServer := api.NewServer(...)
```

问题：
- 依赖关系不清晰
- 单元测试需要手动 mock
- 服务生命周期（启动/关闭）缺乏统一管理

### 2.2 推荐方案：Uber Fx

**Fx** 是 Uber 出品的依赖注入框架，完美适配 Go。

#### 核心概念

| 概念 | 说明 |
|------|------|
| `fx.Provide` | 提供依赖实例，Fx 自动创建 |
| `fx.Invoke` | 执行启动逻辑 |
| `OnStart` / `OnStop` | 生命周期钩子 |
| 构造函数注入 | 自动按参数类型匹配依赖 |

#### 示例代码

```go
package main

import (
    "go.uber.org/fx"
)

func main() {
    fx.New(
        // 提供依赖
        fx.Provide(
            NewMysqlDB,
            NewRedisClient,
            NewUserStore,
            NewUserService,
            NewHTTPServer,
        ),
        // 启动逻辑
        fx.Invoke(func(server *HTTPServer) {
            server.Run()
        }),
        fx.NopLogger,
    ).Run()
}

// Fx 自动注入参数
func NewUserService(
    db *sql.DB,
    redis *redis.Client,
) *UserService {
    return &UserService{
        dbStore: store.NewUserDbStore(db),
        redisStore: store.NewUserRedisStore(redis),
    }
}
```

#### 优势

- **依赖关系声明式** — 从构造函数自动推导
- **单元测试友好** — 可替换为 mock
- **生命周期管理** — 自动处理启动/关闭顺序
- **延迟初始化** — 只在需要时创建

---

## 三、Store 配置管理

### 3.1 当前问题

配置散落在多处：

```go
// store/user_redis.go
func NewUserRedisStoreConfig() UserRedisStoreConfig {
    db, _ := strconv.Atoi(os.Getenv("REDIS_USER_DB"))
    // ...
}
```

问题：
- 类型不安全（字符串转类型容易出错）
- 难以集中修改
- 缺乏默认值和校验

### 3.2 推荐方案：Viper + 统一配置结构

#### 定义配置结构

```go
// config/config.go
type Config struct {
    Server ServerConfig `mapstructure:"server"`
    MySQL  MySQLConfig  `mapstructure:"mysql"`
    Redis  RedisConfig  `mapstructure:"redis"`
}

type ServerConfig struct {
    HTTPPort string `mapstructure:"http_port"`
    WSPort   string `mapstructure:"ws_port"`
}

type MySQLConfig struct {
    Host     string `mapstructure:"host"`
    Port     int    `mapstructure:"port"`
    User     string `mapstructure:"user"`
    Password string `mapstructure:"password"`
    DBName   string `mapstructure:"dbname"`
}

type RedisConfig struct {
    Addr     string `mapstructure:"addr"`
    Password string `mapstructure:"password"`
    DB       int    `mapstructure:"db"`
}
```

#### Viper 加载配置

```go
// config/loader.go
import "github.com/spf13/viper"

func Load(path string) (*Config, error) {
    viper.SetConfigFile(path)
    viper.SetConfigType("yaml")
    
    if err := viper.ReadInConfig(); err != nil {
        return nil, err
    }
    
    var cfg Config
    if err := viper.Unmarshal(&cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
```

#### 配置 YAML 文件

```yaml
# config.yaml
server:
  http_port: "8080"
  ws_port: "8081"

mysql:
  host: "localhost"
  port: 3306
  user: "root"
  password: "password"
  dbname: "aichat"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0
```

#### 在 Fx 中使用

```go
func main() {
    fx.New(
        fx.Provide(func() *config.Config {
            cfg, _ := config.Load("config.yaml")
            return cfg
        }),
        fx.Provide(func(cfg *config.Config) *sql.DB {
            dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
                cfg.MySQL.User, cfg.MySQL.Password,
                cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.DBName)
            db, _ := sql.Open("mysql", dsn)
            db.SetMaxOpenConns(25)
            return db
        }),
    ).Run()
}
```

---

## 四、事务与 Saga 模式：分布式数据一致性

### 4.1 核心问题

分布式系统中，一个业务操作往往涉及多个系统：

```
用户发送消息 → 存储到 MySQL → 推送给在线用户
```

如果某个步骤失败，怎么保证数据一致？

### 4.2 本地事务（Local Transaction）

#### 什么是本地事务

单个数据库实例的事务，通过 ACID 保证：

| 特性 | 含义 |
|------|------|
| **A**tomic（原子性） | 全部成功或全部失败 |
| **C**onsistency（一致性） | 事务前后状态合法 |
| **I**solation（隔离性） | 并发事务互不干扰 |
| **D**urability（持久性） | 提交后数据不丢失 |

#### Go 中的事务写法

```go
func (s *UserService) CreateUserWithProfile(user *User, profile *Profile) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()  // 关键：失败时回滚

    // 步骤1：创建用户
    if err := s.userStore.WithTx(tx).Create(user); err != nil {
        return err
    }

    // 步骤2：创建用户资料（同一事务）
    if err := s.profileStore.WithTx(tx).Create(profile); err != nil {
        return err
    }

    // 步骤3：提交事务
    return tx.Commit()
}
```

#### 适用场景

- ✅ **单数据库操作** — 全部在同一个 MySQL 实例
- ✅ **简单业务** — 不涉及外部服务调用

#### 局限性

```
事务只能在单个数据库内有效！

场景：
  存储 MySQL ──✗──> 消息队列（Kafka）
         │
         └──✗──> 推送服务（WebSocket）
```

当涉及 **MySQL + Redis + 消息队列 + 第三方服务** 时，本地事务失效。

### 4.3 Saga 模式

#### 什么是 Saga

Saga 是一种**分布式事务**模式，核心思想：

> **将一个大事务拆成多个小事务，每个小事务都有对应的补偿操作。**

#### 两种执行方式

| 方式 | 说明 | 适用场景 |
|------|------|----------|
| **Choreography** | 各服务通过事件协作，无中心编排 | 简单流程 |
| **Orchestration** | 中央编排器控制流程 | 复杂流程、有回滚需求 |

#### 具体例子：消息发送

**业务流程（4 步）**

```
1. 消息存储（MySQL）
2. 写入消息队列（Redis Stream）
3. 消费者读取队列
4. 推送给在线用户（WebSocket）
```

**问题：如果第4步失败怎么办？**

```
1. ✅ 存储 MySQL 成功
2. ✅ 写入队列成功
3. ✅ 消费成功
4. ❌ 推送失败（用户断线）

   → 消息已持久化，但用户没收到
```

**Saga 解决方案：重试 + 补偿**

```go
// 定义消息状态
type MessageStatus int
const (
    StatusPending   MessageStatus = iota  // 待发送
    StatusStored                          // 已存储
    StatusQueued                          // 已入队
    StatusDelivered                       // 已送达
    StatusFailed                          // 发送失败
)

// 消息表需要增加状态字段
type Message struct {
    MsgId   string        `json:"msg_id"`
    Content string        `json:"content"`
    Status  MessageStatus `json:"status"`
}

// Saga 实现
type MessageSaga struct {
    store MessageStore
    queue Queue
    push  Pusher
}

func (s *MessageSaga) Send(ctx context.Context, msg *Message) error {
    // 步骤1：存储消息（主事务）
    msg.Status = StatusPending
    if err := s.store.Save(msg); err != nil {
        return err  // 直接失败，无需补偿
    }

    // 步骤2：写入队列
    if err := s.queue.Send(msg); err != nil {
        // 补偿：标记消息为失败
        s.store.UpdateStatus(msg.MsgId, StatusFailed)
        return err
    }

    // 步骤3：异步消费推送（独立重试）
    // 推送失败不影响消息存储，可后续补偿
    return nil
}

// 消费者：收到消息后推送
func (s *MessageSaga) ConsumeAndPush(msg *Message) {
    err := s.push.ToUser(msg.ToId, msg)
    if err != nil {
        // 补偿：标记为失败，可手动重试或用户拉取
        s.store.UpdateStatus(msg.MsgId, StatusFailed)
        return
    }
    s.store.UpdateStatus(msg.MsgId, StatusDelivered)
}
```

#### Saga 的核心原则

```
1. 每个步骤都有补偿操作
2. 步骤失败时，按相反顺序执行补偿
3. 补偿可以是"撤销"或"重试"
4. 最终一致性 > 强一致性
```

#### Saga vs 本地事务对比

| 特性 | 本地事务 | Saga |
|------|----------|------|
| 范围 | 单数据库 | 跨服务 |
| 一致性 | 强一致（ACID） | 最终一致 |
| 复杂度 | 低 | 高 |
| 性能 | 高 | 中 |
| 失败处理 | 自动回滚 | 手动补偿 |

### 4.4 IM 系统的最佳实践

```
┌─────────────────────────────────────────────────────────────┐
│  消息发送流程（推荐）                                        │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. [API] 接收消息                                           │
│         ↓                                                   │
│  2. [存储] 持久化到 MySQL（本地事务）                         │
│         ↓                                                   │
│  3. [队列] 写入 Redis Stream / Kafka                        │
│         ↓                                                   │
│  4. [消费者] 读取队列 → 推送给在线用户                         │
│         ↓                                                   │
│  5. [失败补偿] 推送失败 → 标记状态 + 重试队列                  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 4.5 什么时候用什么

| 场景 | 方案 |
|------|------|
| 单表操作 | 本地事务 |
| 用户注册（写 MySQL + 写 Redis） | 本地事务 + 双写 |
| 消息发送（MySQL + 队列 + 推送） | **Saga** |
| 支付（银行 + 商家 + 积分） | **Saga** + 人工介入 |

---

## 五、下一步改进计划

| 优先级 | 改进项 | 收益 |
|--------|--------|------|
| P0 | 实现 ws_conn.go 读写循环 + 心跳 | WebSocket 稳定性 |
| P1 | 引入 Viper 统一配置管理 | 可维护性 |
| P2 | 引入 Fx 依赖注入 | 可测试性 |
| P2 | 消息队列解耦（Redis Stream） | 高可用 |
| P2 | 消息状态管理 + Saga 补偿 | 数据一致性 |
