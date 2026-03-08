# 数据库索引设计与 Migration 规范

## 一、本项目的索引全景

| 索引 | 所在表 | 覆盖查询 |
|------|--------|---------|
| `PRIMARY KEY (msg_id)` | messages | 幂等去重、按 msgId 查单条消息 |
| `idx_to_id_send_at (to_id, send_at)` | messages | 单聊历史消息分页 |
| `idx_from_id_send_at (from_id, send_at)` | messages | 查某用户发出的消息（消息漫游） |
| `idx_room_id_send_at (room_id, send_at)` | messages | 群聊历史消息分页 |
| `PRIMARY KEY (room_id, user_id)` | room_members | 防止重复加入、按 roomId 查成员 |
| `idx_user_id (user_id)` | room_members | 查某用户加入的所有群 |
| `PRIMARY KEY (id)` | users | 按 userId 查用户 |
| `UNIQUE (email)` | users | 登录时按 email 查用户 |

---

## 二、索引核心原则

### 原则 1：复合索引 vs 单列索引

**单列索引的问题：**

```sql
-- 只有 INDEX(to_id)
SELECT * FROM messages WHERE to_id = ? ORDER BY send_at DESC LIMIT 20
-- 执行过程：
-- 1. 用 to_id 索引找到所有匹配行（可能几千条）
-- 2. 对这几千条结果按 send_at 排序（filesort，额外开销）
-- 3. 取前 20 条
```

**复合索引的优势：**

```sql
-- 有 INDEX(to_id, send_at)
SELECT * FROM messages WHERE to_id = ? ORDER BY send_at DESC LIMIT 20
-- 执行过程：
-- 1. 在索引上直接定位到 to_id 对应的范围
-- 2. 该范围内 send_at 已经有序，直接读 20 条
-- 3. 无 filesort，性能高一个量级
```

把"过滤"和"排序"合并到一次索引扫描，是复合索引的核心价值。

### 原则 2：最左前缀原则

复合索引 `(a, b, c)` 只能从左开始使用：

```sql
WHERE a = ?                    -- ✅ 用到 a
WHERE a = ? AND b = ?          -- ✅ 用到 a, b
WHERE a = ? AND b = ? AND c = ?-- ✅ 用到 a, b, c
WHERE b = ?                    -- ❌ 跳过了 a，不走索引
WHERE a = ? AND c = ?          -- ⚠️ 只用到 a，c 不走索引
```

**在本项目的体现：**

`idx_to_id_send_at (to_id, send_at)` 之所以有效，是因为查询条件里总是先有 `WHERE to_id = ?`，再用 `send_at` 做游标过滤或排序。

### 原则 3：索引不是越多越好

每个索引都会让写入变慢（INSERT/UPDATE 需要同时维护索引树）。  
只在**高频查询的过滤/排序字段**上建索引。

---

## 三、游标分页 vs OFFSET 分页

### OFFSET 的问题

```sql
SELECT * FROM messages ORDER BY send_at DESC LIMIT 20 OFFSET 2000
```

MySQL 不是真的"跳过"2000 行，而是**读了再丢**：

```
第 1 页   → 读 20 行     ✅
第 100 页 → 读 2000 行   ⚠️
第 1000 页→ 读 20000 行  ❌
```

翻页越深，扫描行数线性增长，性能越来越差。

### 游标分页的方案

```sql
-- 客户端把上一页最后一条的 send_at 作为游标传来
SELECT * FROM messages
WHERE to_id = ? AND send_at < ?   -- 游标：只看比这个时间更早的消息
ORDER BY send_at DESC
LIMIT 20
```

配合 `idx_to_id_send_at (to_id, send_at)`：

1. 先在索引上定位到 `to_id` 的范围
2. 在该范围内 `send_at < cursor` 直接跳到游标位置（B+ 树二分查找）
3. 往前读 20 条，结束

**无论翻到哪页，每次只读 20 行，性能与页码无关。**

### 代价

只能顺序翻页（下一页/上一页），不能直接跳到第 N 页。  
IM 历史消息是"往上滑加载更多"，天然顺序翻页，完全适合。

### 单聊 vs 群聊的对称设计

```sql
-- 单聊历史消息（用 to_id 索引）
SELECT * FROM messages
WHERE to_id = 'user-B' AND send_at < ?
ORDER BY send_at DESC LIMIT 20

-- 群聊历史消息（用 room_id 索引）
SELECT * FROM messages
WHERE room_id = 'room-123' AND send_at < ?
ORDER BY send_at DESC LIMIT 20
```

两条查询完全对称，各自走独立索引，互不干扰。这正是新增独立 `room_id` 字段（而不是复用 `to_id`）的价值所在。

---

## 四、为什么单聊/群聊需要独立索引（不能复用 to_id）

如果群聊复用 `to_id` 存 roomId，`idx_to_id_send_at` 索引树里单聊消息和群聊消息混在一起：

```
索引树节点：
  to_id=user-A, send_at=1000  → 单聊消息
  to_id=room-1, send_at=1001  → 群聊消息
  to_id=user-A, send_at=1002  → 单聊消息
  to_id=room-1, send_at=1003  → 群聊消息
  ...
```

查询时虽然能通过 `to_id = 'room-1'` 过滤，但随着数据量增大，索引树节点膨胀，B+ 树层高增加，每次查询需要更多 I/O。

独立 `room_id` 字段后，`idx_room_id_send_at` 只存群聊消息，索引更紧凑，查询更快。

---

## 五、Migration 写法规范

### 什么是 Migration

数据库 schema 的版本控制。每次改表结构，不是直接改数据库，而是写一个 migration 文件，让工具（`golang-migrate`）按顺序执行，保证所有环境（本地/测试/生产）的表结构一致。

### 文件命名规范

```
{序号}_{描述}.up.sql    ← 正向变更（执行）
{序号}_{描述}.down.sql  ← 回滚（撤销 up 的操作）
```

**序号必须连续，4 位数字：**

```
000001_create_user_table.up.sql
000001_create_user_table.down.sql
000002_create_message_table.up.sql
000002_create_message_table.down.sql
000003_create_room_table.up.sql
000003_create_room_table.down.sql
000004_add_room_id_to_message.up.sql   ← 注意：描述要准确，不要有拼写错误
000004_add_room_id_to_message.down.sql
```

### up.sql vs down.sql 的对称关系

| up.sql 操作 | down.sql 对应操作 |
|------------|-----------------|
| `CREATE TABLE` | `DROP TABLE` |
| `ALTER TABLE ADD COLUMN` | `ALTER TABLE DROP COLUMN` |
| `ALTER TABLE ADD INDEX` | `ALTER TABLE DROP INDEX` |
| `INSERT` 初始数据 | `DELETE` 对应数据 |

**down.sql 是 up.sql 的精确逆操作，执行完 down 后数据库状态应该和执行 up 之前完全一样。**

### 建表 Migration 写法

```sql
-- up.sql：建表
CREATE TABLE IF NOT EXISTS rooms (
    id        VARCHAR(36) NOT NULL PRIMARY KEY,
    name      VARCHAR(255) NOT NULL,
    owner_id  VARCHAR(36) NOT NULL,
    create_at BIGINT NOT NULL,
    INDEX idx_create_at (create_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS room_members (
    room_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    join_at BIGINT NOT NULL,
    PRIMARY KEY (room_id, user_id),   -- 复合主键，防止重复加入
    INDEX idx_user_id (user_id)       -- 查某用户加入了哪些群
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

```sql
-- down.sql：删表（注意顺序：先删有外键依赖的表）
DROP TABLE IF EXISTS room_members;
DROP TABLE IF EXISTS rooms;
```

**常见错误：up 和 down 写反了**（本项目 000003 就出现过这个问题，up 写的是 DROP，down 写的是 CREATE）。

### 加字段 Migration 写法

```sql
-- up.sql：加字段 + 加索引（两条语句分开写）
ALTER TABLE messages ADD COLUMN room_id VARCHAR(36) DEFAULT NULL;
ALTER TABLE messages ADD INDEX idx_room_id_send_at (room_id, send_at);
```

```sql
-- down.sql：先删索引，再删字段
ALTER TABLE messages DROP INDEX idx_room_id_send_at;
ALTER TABLE messages DROP COLUMN room_id;
```

**常见错误 1：ADD COLUMN 和 INDEX 写在同一行**

```sql
-- ❌ 语法错误
ALTER TABLE messages ADD COLUMN room_id VARCHAR(36) NOT NULL INDEX idx_room_id_send_at(room_id, send_at);

-- ✅ 正确：分开写
ALTER TABLE messages ADD COLUMN room_id VARCHAR(36) DEFAULT NULL;
ALTER TABLE messages ADD INDEX idx_room_id_send_at (room_id, send_at);
```

**常见错误 2：新增字段用 NOT NULL 但没有 DEFAULT**

```sql
-- ❌ 表里已有数据时，NOT NULL 但没有 DEFAULT 会报错
ALTER TABLE messages ADD COLUMN room_id VARCHAR(36) NOT NULL;

-- ✅ 可为 NULL（单聊消息不需要 room_id）
ALTER TABLE messages ADD COLUMN room_id VARCHAR(36) DEFAULT NULL;
```

**常见错误 3：down.sql 关键字拼写错误**

```sql
-- ❌ ALERT 不是 SQL 关键字
ALERT TABLE messages DROP COLUMN room_id;

-- ✅
ALTER TABLE messages DROP COLUMN room_id;
```

### ENGINE 和 CHARSET 为什么这么写

```sql
ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
```

| 配置 | 说明 |
|------|------|
| `ENGINE=InnoDB` | 支持事务、行锁、外键，MySQL 默认引擎，必须显式指定保证一致性 |
| `CHARSET=utf8mb4` | 真正的 UTF-8（支持 emoji），不要用 `utf8`（MySQL 的 `utf8` 是残缺版，最多 3 字节） |
| `COLLATE=utf8mb4_unicode_ci` | 大小写不敏感的 Unicode 排序规则，`ci` = case insensitive |

---

## 六、本项目 Migration 执行顺序

```
000001: 创建 users 表
000002: 创建 messages 表（含 idx_to_id_send_at, idx_from_id_send_at）
000003: 创建 rooms + room_members 表
000004: messages 表新增 room_id 字段 + idx_room_id_send_at 索引
```

执行命令：

```bash
make migrate   # 创建新的 migration 文件
# 实际 migrate up 命令见 Makefile
```
