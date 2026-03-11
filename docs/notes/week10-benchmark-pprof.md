# Week 10 — 压测与性能分析（pprof）

## 一、为什么要做压测和性能分析

IM 系统的特点是**长连接 + 高并发**，功能跑通后不代表性能达标。常见问题：

- 1000 个连接时 CPU 飙高，原因不明
- 内存随时间缓慢增长，怀疑有泄漏
- 某个接口偶尔超时，不知道卡在哪

压测暴露问题，pprof 定位根因，两者配合是 Go 服务性能调优的标准流程。

---

## 二、压测工具

### 2.1 WS 并发压测 — 自写 Go 脚本

外部工具（如 k6、locust）对 WebSocket 支持有限，Go 原生 goroutine 模型天然适合写 WS 压测脚本。

```go
// scripts/ws_bench_test.go
func TestWsConcurrent(t *testing.T) {
    const clients = 1000
    var wg sync.WaitGroup
    wg.Add(clients)

    for i := 0; i < clients; i++ {
        go func(id int) {
            defer wg.Done()
            token := getToken(id) // 预先创建好用户并登录
            conn, _, err := websocket.DefaultDialer.Dial(
                "ws://localhost:9090/ws?token="+token, nil,
            )
            if err != nil {
                t.Logf("dial error: %v", err)
                return
            }
            defer conn.Close()

            // 持续收发 30s
            deadline := time.Now().Add(30 * time.Second)
            for time.Now().Before(deadline) {
                msg := fmt.Sprintf(`{"msgId":"%s","toId":"target","type":0,"content":"hello","sendAt":%d}`,
                    uuid.New(), time.Now().UnixMilli())
                conn.WriteMessage(websocket.TextMessage, []byte(msg))
                time.Sleep(100 * time.Millisecond)
            }
        }(i)
    }
    wg.Wait()
}
```

**观察指标：**
- 服务端 CPU 使用率
- 内存占用趋势
- 连接断开率（有无 panic/error 日志）

### 2.2 HTTP 接口压测 — wrk / hey

```bash
# 安装 hey（比 wrk 更简单）
go install github.com/rakyll/hey@latest

# 测登录接口：100 并发，共 10000 个请求
hey -n 10000 -c 100 -m POST \
    -H "Content-Type: application/json" \
    -d '{"email":"test@test.com","password":"123456"}' \
    http://localhost:8080/user/login

# 输出示例（关注这几行）：
# Requests/sec:  3241.5        ← QPS
# 99th percentile: 45ms        ← P99 延迟
# [200] 9998 responses         ← 成功率
```

```bash
# wrk：更精细，支持 Lua 脚本自定义请求
wrk -t4 -c100 -d30s --latency http://localhost:8080/user/list
# -t4: 4 个线程  -c100: 100 并发  -d30s: 持续 30 秒
```

---

## 三、pprof 使用方法

### 3.1 接入 pprof

```go
// main.go
import (
    _ "net/http/pprof"  // 副作用导入，自动注册 /debug/pprof/ 路由
    "net/http"
)

func main() {
    // 单独开一个端口暴露 pprof，不影响业务端口
    go func() {
        http.ListenAndServe(":6060", nil)
    }()
    // ...启动业务服务
}
```

访问 `http://localhost:6060/debug/pprof/` 可以看到所有可用的 profile 类型。

### 3.2 四种常用 Profile

#### CPU Profile — 找 CPU 热点

```bash
# 采集 30 秒的 CPU 使用数据（压测期间执行）
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 进入交互模式后
(pprof) top10        # 看 CPU 占用最高的 10 个函数
(pprof) list Route   # 看 Route 函数的逐行 CPU 占用
(pprof) web          # 生成 SVG 调用图（需要 graphviz）
```

```bash
# 更推荐：生成火焰图（可视化，一目了然）
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile?seconds=30
# 浏览器打开 localhost:8081 → Flame Graph
# 横轴是 CPU 时间占比，纵轴是调用栈，宽的块就是热点
```

#### Heap Profile — 找内存分配热点

```bash
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/heap

# 关键视图：
# alloc_space  → 历史总分配量（找频繁分配的地方）
# inuse_space  → 当前占用量（找内存泄漏）
```

#### Goroutine Profile — 检查 goroutine 泄漏

```bash
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/goroutine

# 或者直接浏览器看（更直观）
# http://localhost:6060/debug/pprof/goroutine?debug=1
```

**判断是否泄漏：**
```
压测开始前：goroutine 数 = N
1000 个连接建立后：goroutine 数 ≈ N + 3000（每连接 readPump + writePump + ackTimer）
1000 个连接断开后：goroutine 数应回到 ≈ N
如果断开后 goroutine 数持续增长 → 泄漏
```

#### Mutex Profile — 找锁竞争

```bash
# 需要先在代码里开启 mutex profiling
runtime.SetMutexProfileFraction(1)

go tool pprof -http=:8081 http://localhost:6060/debug/pprof/mutex
# 能看到哪把锁被争抢最严重，等待时间最长
```

---

## 四、本项目最可能出现的问题

### 问题 1：ConnManager 锁竞争（必须修）

**现象：** CPU profile 里 `sync.Mutex.Lock` 占用大量时间

**根因：** 当前用 `sync.Mutex`，读操作（`GetConn`）也加写锁，1000 个并发消息路由时全部排队

```go
// ❌ 现状：读也用独占锁
type ConnManager struct {
    mx    sync.Mutex      // 读写都排队
    conns map[string]Conn
}

func (c *ConnManager) GetConn(id string) (Conn, error) {
    c.mx.Lock()           // 读操作不需要写锁
    defer c.mx.Unlock()
    ...
}

// ✅ 优化：读写分离
type ConnManager struct {
    mx    sync.RWMutex    // 读锁允许并发，写锁独占
    conns map[string]Conn
}

func (c *ConnManager) GetConn(id string) (Conn, error) {
    c.mx.RLock()          // 多个读可以同时进行
    defer c.mx.RUnlock()
    ...
}

func (c *ConnManager) AddConn(conn Conn) error {
    c.mx.Lock()           // 写操作保持独占
    defer c.mx.Unlock()
    ...
}
```

**效果：** 消息路由（读多写少）并发度大幅提升

### 问题 2：Goroutine 泄漏（必须查）

**现象：** goroutine 数量只增不减，内存缓慢上涨

**根因：** WsConn 断开时 `done` channel 没有正确关闭，`readPump`/`writePump`/ACK 定时器 goroutine 永久挂起

```go
// ws_conn.go 断开时必须关闭 done channel
func (c *WsConn) Close() {
    c.once.Do(func() {       // sync.Once 保证只关闭一次，防止 panic
        close(c.done)
        c.conn.Close()
        c.onClose(c.id)
    })
}

// readPump/writePump/ackTimer 都要监听 done
select {
case <-c.done:
    return   // 连接关闭，goroutine 退出
}
```

### 问题 3：JSON 序列化开销（锦上添花）

**现象：** heap profile 里 `encoding/json.Marshal` 分配量大

**优化方案：**
```go
// 换高性能 JSON 库（API 兼容 encoding/json，一行改动）
import jsoniter "github.com/json-iterator/go"
var json = jsoniter.ConfigCompatibleWithStandardLibrary

// 或者用字节跳动的 sonic（ARM/x86 均支持）
import "github.com/bytedance/sonic"
```

### 问题 4：MySQL 连接池配置不当

**现象：** 压测时大量请求超时，日志出现 `driver: bad connection`

**优化：**
```go
db.SetMaxOpenConns(50)      // 最大连接数（根据 MySQL 配置调整）
db.SetMaxIdleConns(10)      // 空闲连接池大小
db.SetConnMaxLifetime(time.Hour)  // 连接最大存活时间，防止连接被 MySQL 服务端断掉
```

---

## 五、调优工作流程

```
1. 压测（1000 并发 WS）
       ↓
2. 观察：CPU / 内存 / goroutine 数 / 错误率
       ↓
3. 用 pprof 定位具体热点
       ↓
4. 针对性修改代码
       ↓
5. 重新压测，对比数据
       ↓
6. 重复直到满足目标（1000 并发稳定、无泄漏）
```

**目标基准：**
- 1000 并发 WS 连接稳定保持，无断线
- 停止压测后 goroutine 数量回落
- P99 消息延迟 < 100ms

---

## 六、延伸阅读

- Go 官方 pprof 文档：`go doc runtime/pprof`
- 火焰图原理：Brendan Gregg 发明，横轴=时间占比，纵轴=调用栈深度，宽块=热点
- `sync.RWMutex` vs `sync.Mutex`：读写比越高，RWMutex 收益越大；纯写场景两者无差异
