# Week 03 — 消息系统与数据库存储

## 一、消息数据模型

### Message 结构体

```go
type MessageType int

const (
    MessageTypeText   MessageType = iota
    MessageTypeImage
    MessageTypeAudio
    MessageTypeSystem
)

type Message struct {
    MsgId       string      `validate:"required,uuid"`   // 消息唯一 ID，用于 ACK 和幂等去重
    FromId      string      `validate:"required,uuid"`   // 发送方 userId
    ToId        string      `validate:"required,uuid"`   // 单聊=对方 userId，群聊=roomId
    Type        MessageType `validate:"required"`
    Content     string      `validate:"required,min=1,max=1000"`
    SendAt      int64       `validate:"gt=0"`            // Unix 纳秒，发送时间
    IsDelivered bool                                     // Week 6 ACK 机制用
}
```

**关键设计决策：**
- `ToId` 单聊和群聊共用一张表，单聊存 userId，群聊存 roomId，Week 5 群聊时不需要改表结构
- `MsgId` 由客户端生成（UUID v7），保证幂等性——重复提交同一 msgId 不会产生重复消息
- `SendAt` 用客户端时间而非服务端时间，保证消息排序符合用户感知

---

## 二、消息路径设计

```
客户端发消息
    ↓
[1] ws_server 收到 WS 帧
    ↓
[2] 通过 MessageService 接口处理（不直接写库）
    ↓
[3] 同步写 MySQL 持久化
    ↓
[4] 通过 ConnManager 找到接收方，Push

（Week 17 引入 Kafka 后，[3][4] 改为发到 Kafka，由 Consumer 消费）
```

**为什么通过 MessageService 接口而不直接写库：**

等 Week 17 引入 Kafka 时，只需要换 MessageService 的实现，`ws_server.go` 完全不改。这是面向接口编程的价值体现。

---

## 三、为什么用 MySQL 存消息，不用时序数据库

### 时序数据库的适用场景

时序数据库（InfluxDB、TimescaleDB）专为"按时间写入、按时间范围查询"设计，优势：
- 极高写入吞吐量
- 时间范围查询快
- 自动数据压缩

### IM 消息不完全适合时序数据库

| 查询需求 | 时序数据库 | MySQL |
|---------|-----------|-------|
| 按时间范围拉取消息 | ✅ 擅长 | ✅ 加索引也很快 |
| 按 userId 查历史消息 | ⚠️ 需要 tag 过滤 | ✅ 直接索引 |
| 更新消息状态（已读/已送达）| ❌ 不擅长，数据通常不可变 | ✅ UPDATE 自然 |
| 消息撤回（删除特定消息）| ❌ 不擅长 | ✅ DELETE |
| 关联查询（消息 + 用户信息）| ❌ 无 JOIN | ✅ JOIN |

IM 消息需要更新和删除，是时序数据库的弱项。

### 真实大厂的方案

**微信 / 钉钉：**
```
MySQL / TiDB   → 消息元数据（msgId、状态、内容）
对象存储（COS）→ 图片、视频、文件
Redis          → 缓存最近 100 条消息（List）
```

消息量大了之后按**时间分表**，而不是换数据库：
```sql
messages_2024_01
messages_2024_02  -- 每月一张表，历史数据归档
```

**Discord（特殊案例）：**
用 Cassandra（列式存储），因为超大群（数十万人）、写入量极大、查询模式固定（按 channelId + 时间范围）、几乎不需要更新，完美匹配列式存储。

---

## 四、三种存储类型的定位

### 行式存储（MySQL）

数据按行存在磁盘，每行包含所有字段：
```
[id=1, name=Alice, age=25, city=北京]
[id=2, name=Bob,   age=30, city=上海]
```

**优势：** 读写整行快，支持事务、JOIN、UPDATE、DELETE
**适用：** 结构化业务数据，需要关联查询和更新的场景

---

### 对象存储（COS / OSS / S3）

专门存大文件，每个文件有唯一 URL：
```
https://bucket.cos.ap-beijing.myqcloud.com/chat/img/abc123.jpg
```

**优势：**
- 比数据库便宜 10-100 倍（1TB ≈ 几十元/月）
- 无限扩容，云厂商自动管理
- 直接对接 CDN，就近下载
- 数据库只存 URL，减负

**IM 里的用法：**
```
客户端发图片
    ↓
先上传图片到 COS → 拿到 URL
    ↓
WS 消息只传 URL：{"type":"image","content":"https://cos.../abc.jpg"}
    ↓
数据库存这个 URL，几十字节
```

---

### 列式存储（Cassandra / ClickHouse）

数据按列存，同一列的数据连续存放：
```
id列:   [1, 2, 3]
name列: [Alice, Bob, Carol]
age列:  [25, 30, 28]
```

**优势：**
- 只查需要的列，I/O 极少（`SELECT age` 只读 age 列）
- 写入吞吐量极高（Cassandra 百万/秒）
- 同类型数据压缩率极高

**劣势：**
- 更新单行很慢
- 不支持复杂 JOIN

**适用：** 超高写入量、查询模式固定、几乎不需要更新的场景（日志、监控、超大群聊）

---

## 五、消息表迁移设计

### 完整建表 SQL

```sql
CREATE TABLE IF NOT EXISTS messages (
    msg_id       VARCHAR(36)   NOT NULL PRIMARY KEY,
    from_id      VARCHAR(36)   NOT NULL,
    to_id        VARCHAR(36)   NOT NULL,
    type         TINYINT       NOT NULL DEFAULT 0,
    content      VARCHAR(1000) NOT NULL,
    send_at      BIGINT        NOT NULL,
    is_delivered BOOLEAN       NOT NULL DEFAULT FALSE,
    INDEX idx_to_id_send_at (to_id, send_at),
    INDEX idx_from_id_send_at (from_id, send_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### 字段设计决策

| 字段 | 类型 | 理由 |
|------|------|------|
| `msg_id` | VARCHAR(36) PRIMARY KEY | 存 UUID，客户端生成，保证幂等性 |
| `type` | TINYINT | 对应 Go 的 `MessageType int`（0=text/1=image/2=audio/3=system），比 VARCHAR 省空间 |
| `content` | VARCHAR(1000) | 与 Go struct validate `max=1000` 对齐，图片消息存 URL |
| `send_at` | BIGINT | Unix 纳秒，客户端时间，保证排序符合用户感知 |
| `is_delivered` | BOOLEAN | Week 6 ACK 机制需要，提前加上 |

### 为什么 msg_id 用客户端生成而不是数据库自增

```
用户弱网环境下发消息
    ↓
消息到达服务端，写库成功
    ↓
响应在回传途中丢失，客户端以为失败
    ↓
客户端重试，再次发送同一条消息

如果用数据库自增 ID：
    → 插入两条记录，消息重复 ❌

如果用客户端生成的 UUID（msg_id）：
    → 第二次插入时 PRIMARY KEY 冲突，自动幂等去重 ✅
```

### 索引设计

**核心索引：`idx_to_id_send_at (to_id, send_at)`**

覆盖最常见的查询——拉取某个用户/群聊的历史消息：

```sql
SELECT * FROM messages
WHERE to_id = ?
ORDER BY send_at DESC
LIMIT 20
```

没有这个索引，每次查询都是全表扫描。

**辅助索引：`idx_from_id_send_at (from_id, send_at)`**

覆盖"查某个用户发出的消息"场景（消息漫游 Week 7 需要）。

### 为什么是复合索引而不是单列索引

```sql
-- 如果只有 INDEX(to_id)
WHERE to_id = ? ORDER BY send_at DESC
-- MySQL 先用 to_id 过滤，再对结果集排序（filesort）

-- 有了 INDEX(to_id, send_at)
WHERE to_id = ? ORDER BY send_at DESC
-- MySQL 直接在索引上按顺序读，无需额外排序（Using index）
```

复合索引把"过滤"和"排序"合并到一次索引扫描里，性能高一个量级。

### 索引的左前缀原则

复合索引 `(to_id, send_at)` 能覆盖以下查询：
```sql
WHERE to_id = ?                          -- ✅ 用到第一列
WHERE to_id = ? AND send_at > ?          -- ✅ 用到两列
WHERE send_at > ?                        -- ❌ 不走索引（没有 to_id）
```

必须从左边的列开始用，不能跳过。

---

## 六、消息路由设计（MessageRouter）

### 问题：MessageService 怎么把消息推给接收方

`MessageService` 存完消息后需要推送给接收方，但接收方的连接在 `ConnManager`（gateway 层），直接依赖 gateway 会破坏分层。

### 错误做法

```go
// ❌ service 层直接依赖 gateway 层，违反分层原则
type MessageService struct {
    connManager *gateway.ConnManager  // 不应该这样
}
```

### 正确做法：依赖倒置（DIP）

**接口定义在使用方（service 包），实现方去适配接口**：

```
service 包定义 MessageRouter 接口
    ↑ 隐式实现（Go 不需要 implements 关键字）
gateway.ConnManager
```

```go
// service/router.go — 接口定义在使用方
type MessageRouter interface {
    Route(toId string, msg *types.Message) error
}

// service/message.go — 依赖接口，不依赖实现
type MessageService struct {
    messageStore store.MessageStore
    router       MessageRouter  // 接口类型
}

func (s *MessageService) SendMessage(msg *types.Message) error {
    s.messageStore.Save(msg)
    return s.router.Route(msg.ToId, msg)  // 不知道底层是 WS 还是 Kafka
}
```

```go
// gateway/conn_manager.go — 实现接口，无需 import service
func (c *ConnManager) Route(toId string, msg *types.Message) error {
    conn, err := c.GetConn(toId)
    if err != nil {
        return nil  // 用户不在线，Week 7 处理离线消息
    }
    data, _ := json.Marshal(msg)
    return conn.Push(data)
}
```

```go
// main.go — 在最外层组装，把实现注入给接口
connManager := gateway.NewConnManager()
messageSrv := service.NewMessageService(messageStore, connManager)
//                                                    ↑ ConnManager 自动匹配 MessageRouter 接口
```

### 为什么 Route 传 *types.Message 而不是 []byte

```go
// ❌ 传 []byte：调用方需要先序列化，序列化方式固定死了
Route(toId string, data []byte) error

// ✅ 传 *types.Message：由 Router 实现方决定序列化格式
Route(toId string, msg *types.Message) error
// WS Router：json.Marshal
// Kafka Router（Week 17）：protobuf 或 json，由实现方决定
```

### 依赖关系图

```
main.go
  ├── import service
  └── import gateway
        ↓ 组装时注入
service.MessageService
  └── 依赖 MessageRouter 接口（service 包内定义）
        ↑ 实现（隐式）
gateway.ConnManager
  └── 不需要 import service
```

### 未来扩展（Week 17）

换 Kafka 时只需要新增一个实现，`MessageService` 完全不改：

```go
type KafkaRouter struct { producer *kafka.Producer }
func (r *KafkaRouter) Route(toId string, msg *types.Message) error {
    // 发到 Kafka topic
}
// main.go 里把 connManager 换成 kafkaRouter 即可
```

---

## 七、本项目的存储方案

| 数据 | 存储 | 原因 |
|------|------|------|
| 消息元数据 | MySQL | 需要更新状态（已读/已送达）、支持关联查询 |
| 图片/视频/文件 | 对象存储（Week 11+ 引入）| 便宜、CDN 友好 |
| 最近 100 条消息 | Redis List | 热数据缓存，Week 7 实现 |
| 消息队列 | Kafka（Week 17 引入）| 解耦、削峰、多实例路由 |

> MySQL 消息量到千万级别前完全够用，之后考虑按月分表或换 TiDB（MySQL 协议兼容，代码几乎不改）
