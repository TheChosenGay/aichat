# ADR-001: 用 Redis Hash 存储用户信息

## 背景
用户登录后需要快速读取用户基本信息，不想每次都查 MySQL。

## 决策
用 Redis Hash 存储用户信息，Key 格式为 `user:{userId}`，TTL 24 小时。
JWT 也用 Hash 存储：`user:jwt:{email}`，字段为 `cert`（token）和 `secret`（签名密钥）。

## 理由
- Hash 比 String+JSON 序列化更省内存，且可以只读取部分字段
- 相比用 String 存整个 JSON，Hash 更新单个字段不需要读-改-写

## 后果
- 用户信息更新时需要同步更新 Redis（或直接删除 key 让其自然过期）
- 目前 TTL 固定 24 小时，登录续期未实现
