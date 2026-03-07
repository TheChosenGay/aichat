# Docker 常用命令速查

## MySQL 容器

### 启动
```bash
docker run --name mysql \
  -e MYSQL_ROOT_PASSWORD=123456 \
  -p 3306:3306 \
  -d mysql:8
```

### 进入容器执行 MySQL 命令

```bash
# 交互式进入 MySQL shell
docker exec -it mysql mysql -uroot -p

# 直接执行命令（不进入交互模式）
docker exec -it mysql mysql -uroot -p123456 -e "SHOW DATABASES;"
```

### 常用 MySQL 命令

```sql
-- 查看所有数据库
SHOW DATABASES;

-- 选择数据库
USE aichat;

-- 查看所有表
SHOW TABLES;

-- 查看表结构
DESCRIBE users;
DESCRIBE messages;

-- 查询数据
SELECT * FROM users LIMIT 10;
SELECT * FROM messages LIMIT 10;

-- 查看表索引
SHOW INDEX FROM messages;

-- 查看建表语句
SHOW CREATE TABLE messages;
```

### 验证迁移是否成功

```bash
docker exec -it mysql mysql -uroot -p123456 -e "USE aichat; SHOW TABLES;"
```

---

## Redis 容器

### 启动
```bash
docker run --name redis \
  -p 6379:6379 \
  -d redis redis-server \
  --save 60 1 \
  --loglevel warning \
  --appendfsync everysec
```

### 进入容器执行 Redis 命令

```bash
# 交互式进入 redis-cli
docker exec -it redis redis-cli

# 指定 DB（默认 DB 0）
docker exec -it redis redis-cli -n 1

# 直接执行命令
docker exec -it redis redis-cli -n 0 DBSIZE
```

### 常用 Redis 命令

```bash
# 查看当前 DB 的 key 数量
DBSIZE

# 查看所有 key（数据量大时慎用）
KEYS "*"

# 查看各 DB 的 key 数量
INFO keyspace

# 查看 Hash 所有字段
HGETALL user:61201251-7838-4654-9137-fc0c02197acb

# 查看某个字段
HGET user:61201251-7838-4654-9137-fc0c02197acb email

# 查看 key 的剩余有效期（-1 永不过期，-2 不存在）
TTL user:61201251-7838-4654-9137-fc0c02197acb

# 删除 key
DEL user:jwt:rick@tencent.com
```

---

## 通用容器管理

```bash
# 查看运行中的容器
docker ps

# 查看所有容器（包括停止的）
docker ps -a

# 停止容器
docker stop mysql
docker stop redis

# 启动已停止的容器
docker start mysql
docker start redis

# 删除容器（需先停止）
docker rm mysql

# 强制删除（无需先停止）
docker rm -f mysql

# 查看容器日志
docker logs mysql
docker logs redis

# 实时查看日志
docker logs -f mysql
```

---

## 本项目的容器连接参数

### MySQL
```
Host:     127.0.0.1
Port:     3306
User:     root
Password: 见 .env 的 MYSQL_PASSWORD
Database: aichat
```

### Redis
```
Host:     127.0.0.1
Port:     6379
Password: 见 .env 的 REDIS_PASSWORD（默认为空）
DB:       见 .env 的 REDIS_USER_DB
```
