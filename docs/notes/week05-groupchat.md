# Week 05 — 群聊实现

## 一、核心设计决策：独立 RoomId 字段

### 问题

实现群聊时需要区分单聊和群聊，有两种选择：

- **方案 A**：复用 `to_id`，单聊存 userId，群聊存 roomId
- **方案 B**：Message 新增独立 `room_id` 字段

### 选择方案 B 的理由

**1. 字段语义清晰**

复用 `to_id` 让一个字段承担两种语义，查询时需要额外判断类型，每加新功能都要带上这个判断。

**2. 索引不会退化**

复用 `to_id` 时，`idx_to_id_send_at` 索引树里单聊和群聊消息混杂，随数据量增大 B+ 树节点膨胀。独立字段让两条查询路径走各自的索引，互不干扰：

```sql
-- 单聊历史消息
WHERE to_id = 'user-B' AND send_at < ?   -- 走 idx_to_id_send_at

-- 群聊历史消息
WHERE room_id = 'room-123' AND send_at < ?  -- 走 idx_room_id_send_at
```

**3. 会话列表联表查询更干净**

复用 `to_id` 时 `conversations` 表的 `target_id` 无法直接知道对应 user 还是 room，需要同时 LEFT JOIN 两张表。独立字段后按是否为 NULL 直接判断。

**4. 路由逻辑解耦**

`MessageService.SendMessage` 可以按字段是否为空走完全独立的路径，单聊逻辑完全不受影响：

```go
func (s *MessageService) SendMessage(message *types.Message) error {
    if err := s.messageStore.Save(message); err != nil {
        return err
    }
    if message.RoomId != "" {
        return s.sendGroupMessage(message)  // 群聊路径
    }
    return s.router.Route(message)          // 单聊路径，原有逻辑不动
}
```

> 完整决策记录见 ADR-003。

---

## 二、数据模型

### 新增表

**rooms 表**

```sql
CREATE TABLE IF NOT EXISTS rooms (
    room_id   VARCHAR(36) PRIMARY KEY,
    name      VARCHAR(255) NOT NULL,
    owner_id  VARCHAR(36) NOT NULL,
    create_at BIGINT NOT NULL,
    INDEX idx_create_at(create_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**room_members 表**

```sql
CREATE TABLE IF NOT EXISTS room_members (
    room_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    join_at BIGINT NOT NULL,
    PRIMARY KEY (room_id, user_id),  -- 复合主键防止重复加入
    INDEX idx_user_id(user_id)       -- 查"我加入了哪些群"
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

`PRIMARY KEY (room_id, user_id)` 是复合主键，天然防止同一用户重复加入同一个群，不需要业务层额外去重。

**messages 表新增字段**

```sql
ALTER TABLE messages ADD COLUMN room_id VARCHAR(36) DEFAULT NULL;
ALTER TABLE messages ADD INDEX room_id_send_at(room_id, send_at);
```

`room_id` 允许 NULL，单聊消息不需要该字段，存量数据不受影响。

---

## 三、架构分层

```
HTTP API (api/room.go)
    ↓
RoomService (service/room.go)    — 业务规则：创建上限、成员数限制
    ↓
RoomStore  (store/room.go)       — 接口
    ↓
RoomDbStore (store/room_db.go)   — MySQL 实现
```

### 群聊消息路由分层

```
ws_server 收到消息
    ↓
MessageService.SendMessage
    ├── messageStore.Save()          — 持久化（单群聊相同）
    ├── RoomId == "" → router.Route()           — 单聊：推给 ToId
    └── RoomId != "" → sendGroupMessage()
                           ↓
                       roomStore.GetMembers()   — 拿成员列表
                           ↓
                       router.RouteGroup()      — 扇出推送
```

---

## 四、群聊消息扇出实现

### Week 10 优化：串行改并发

原始实现是串行推送，100 人群聊一条消息需要等 100 次 `Push` 依次完成：

```go
// ❌ 串行：N 个成员逐一推送
for _, memberId := range memberIds {
    conn.Push(data)
}
```

**Week 10 改为 errgroup 并发推送：**

```go
// gateway/conn_manager.go
func (c *ConnManager) RouteGroup(message *types.Message, memberIds []string) error {
    // json.Marshal 提到循环外，N 个成员只序列化一次
    data, err := json.Marshal(message)
    if err != nil {
        return err
    }

    g := errgroup.Group{}
    g.SetLimit(40)  // 限制并发 goroutine 数，防止大群时调度器压力过大
    for _, memberId := range memberIds {
        g.Go(func() error {
            if memberId == message.FromId {
                return nil  // 不推给发送者自己
            }
            conn, err := c.GetConn(memberId)
            if err != nil {
                return nil  // 不在线，静默跳过
            }
            conn.Push(data)
            return nil
        })
    }
    return g.Wait()
}
```

**关键细节：**

- `g.SetLimit(40)`：100 人群聊最多同时跑 40 个 goroutine，不是 100 个，保护调度器
- `g.Wait()`：必须等所有 goroutine 完成再返回，否则函数提前返回，goroutine 在后台飘着，`SetLimit` 的限制形同虚设
- `return nil`（不 return error）：群推送是广播语义，每个成员独立，一个失败不影响其他人
- Go 1.22+ 不需要 `memberId := memberId`，循环变量每次迭代自动独立

**为什么不用「一个失败全部取消」：**

群聊推送是广播，每个成员是独立接收者，没有事务依赖。`errgroup.WithContext` 的取消语义适合事务型操作（如转账），不适合广播。失败的成员由 pender 重试兜底，其他成员不应受影响。

**踩坑：`json.Marshal` 不能放在循环内**

群有 N 个成员，消息内容不变，放在循环内会序列化 N 次，纯粹浪费 CPU。提到循环外是固定模式。

---

## 五、成员列表查询：两种场景两个方法

消息扇出和展示群成员列表是不同场景，需求不同：

| 场景 | 方法 | 说明 |
|------|------|------|
| 消息扇出 | `GetMembers(roomId)` | 一次拿全部成员 ID，依赖业务层控制群成员上限 |
| 展示群成员 | `GetMembersPaged(roomId, afterUserId, limit)` | 游标分页，避免一次返回大量用户信息 |

**扇出为什么不分页：** 发消息需要推给所有人，分批拿成员会导致部分成员收到消息有时间差。控制成员上限（当前 `ROOM_MAX_LIMIT = 20` 个群，成员数后续加）是更合理的手段。

**展示列表的游标分页：** 以 `user_id` 字典序作游标：

```sql
SELECT user_id FROM room_members
WHERE room_id = ? AND user_id > ?   -- user_id 作游标
ORDER BY user_id ASC LIMIT ?
```

---

## 六、获取成员详情：批量查询避免 N+1

获取群成员列表时需要返回用户详情（名称、头像等），错误做法是逐个查：

```go
// ❌ N 次串行查询，100 个成员 = 100 次 DB 往返
for _, uid := range memberIds {
    user, _ := userStore.GetById(uid)
}
```

正确做法是批量查询，一条 SQL 搞定：

```go
// ✅ 一次查询
users, _ := userStore.GetUsersByIds(memberIds)
```

```sql
SELECT * FROM users WHERE id IN (?, ?, ?, ...)
```

**两步 Store 调用，无论群有多少人，始终是固定两次 DB 查询。**

> 后续 Week 7 引入 Redis 缓存后，可以进一步用 Pipeline 批量读取用户缓存，一次网络往返替代 N 次 HGETALL。

---

## 七、业务规则：创建群上限

```go
const ROOM_MAX_LIMIT = 20  // 每人最多创建 20 个群

func (s *defaultRoomService) CreateRoom(room *types.Room) error {
    count, err := s.roomStore.GetRoomCountByUserId(room.OwnerId)
    if err != nil {
        return err
    }
    if count >= ROOM_MAX_LIMIT {
        return errors.New("user room count limit reached")
    }
    return s.roomStore.CreateRoom(room)
}
```

业务规则放在 Service 层，Store 层只管读写，不做业务判断。

---

## 八、安全：OwnerId 从 JWT 取，不信任客户端

和 `fromId` 的原则一致，创建群时 `OwnerId` 不能从请求体取，必须从 JWT token 中注入：

```go
// ❌ 客户端可以伪造任意 OwnerId
type CreateRoomRequest struct {
    OwnerId string `json:"owner_id"`
    Name    string `json:"name"`
}

// ✅ 从 JWT context 取
func (s *RoomServer) createRoomHandler(w http.ResponseWriter, r *http.Request) {
    userId := r.Context().Value(middleware.UserIdKey).(string)
    room := &types.Room{
        OwnerId: userId,  // 服务端注入，不信任客户端
        ...
    }
}
```

---

## 九、golang-migrate 踩坑：每个文件只能有一条 SQL

`golang-migrate` 的 MySQL driver 默认**不支持单文件多条语句**，两条 SQL 写在同一个文件里会报语法错误：

```
Error 1064: You have an error in your SQL syntax ... near 'CREATE TABLE IF NOT EXISTS room_members'
```

**解决方案：一条语句一个文件。**

本项目最终 migration 结构：

```
001 - 建 users 表
002 - 建 messages 表
003 - 建 rooms 表
004 - 建 room_members 表        ← 从 003 拆出来
005 - messages 加 room_id 字段
006 - messages 加 room_id 索引  ← 从 005 拆出来
```

**Dirty database 处理：** migration 执行失败后数据库会被标记为 dirty，再次启动直接 panic。需要用 migrate CLI 强制回退版本：

```bash
migrate -path store/migrations \
        -database "mysql://user:pass@tcp(host)/dbname" \
        force <上一个成功的版本号>
```

---

## 十、群消息可靠性策略

### 现状：群消息无 pending

1v1 消息有完整的 pender + ACK 重试机制，但群消息目前没有。

### 为什么不直接套用 1v1 的 pender？

1v1 pender 以 `MsgId` 为 key，一条消息对应一条 pending 记录。群消息发给 N 个人，如果按 `toId` 分别追踪，需要 N 条 pending 记录，`MsgId` 必须区分（同一条消息对不同接收者是不同的 pending）。

两种实现思路：

| 思路 | 做法 | 优点 | 缺点 |
|------|------|------|------|
| 拆分消息 | 每个接收者克隆一条消息，生成新 MsgId | 复用现有 pender 逻辑 | 存储膨胀（100 人群存 100 条）；客户端 MsgId 与发送方不一致 |
| 独立 GroupPender | key = `MsgId:ToId`，单独实现 | 存储不膨胀，语义清晰 | 需要新增 pender 实现 |

### 当前决策：不做群消息 pending，依靠客户端拉取兜底

**理由：**

1. 群消息 ACK 实现成本比 1v1 高一个量级
2. 主流 IM（微信、钉钉）对群消息的可靠性保证也是**尽力投递**，不是严格 ACK
3. 消息已持久化到 MySQL，用户随时可以通过历史消息接口补拉

### 分层可靠性策略

| 消息类型 | 可靠性策略 | 兜底手段 |
|---------|-----------|--------|
| 1v1 消息 | 服务端 pender + ACK 重试 | 离线拉取 |
| 群消息 | 尽力投递（并发推送） | 客户端主动拉取历史消息 |

### 未来如果要做群消息 pending

推荐「独立 GroupPender」方案，key = `MsgId:ToId`：

```go
type GroupPendMessage struct {
    msgId      string
    toId       string
    retryCount int
    lastRetryAt int64
    msg        *types.Message
}
// map key: msgId + ":" + toId
```

---

## 十一、未来演进路径

| 阶段 | 问题 | 方案 |
|------|------|------|
| 当前 | 群成员上限未限制 | Service 层加成员数校验（类似创建群上限） |
| Week 7 | 离线消息拉取 | 群聊历史消息走 `WHERE room_id = ? AND send_at < ?` + Redis 缓存最近 100 条 |
| Week 9 | 会话列表 | `conversations` 表按 `room_id` 是否为 NULL 区分单群聊，避免双 JOIN |
| 中期 | 大群推送性能 | 引入 Kafka，消息扇出改为 Consumer 并行消费 |
| 长期 | 超大群（万人+） | 在线用户推送 + 离线用户只存库，放弃全量推模型 |
