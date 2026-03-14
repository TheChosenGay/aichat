/**
 * 场景三：稳定性压测
 *
 * 目标：验证长时间运行下服务无 goroutine/内存泄漏
 *
 * 场景：
 *   - 500 个用户保持 WebSocket 连接 10 分钟
 *   - 每个用户每 5s 发一条消息（低频，模拟正常聊天）
 *   - 压测期间通过 pprof 采集 goroutine 和 heap 快照
 *
 * 运行：
 *   k6 run benchmark/ws_stability.js
 *
 * pprof 采样（另开终端，压测进行中执行）：
 *   # goroutine 快照（压测开始后 1 分钟和结束前各采一次，对比是否增长）
 *   go tool pprof -png http://localhost:6060/debug/pprof/goroutine > /tmp/goroutine_start.png
 *   go tool pprof -png http://localhost:6060/debug/pprof/goroutine > /tmp/goroutine_end.png
 *
 *   # heap 快照
 *   go tool pprof -png http://localhost:6060/debug/pprof/heap > /tmp/heap_start.png
 *   go tool pprof -png http://localhost:6060/debug/pprof/heap > /tmp/heap_end.png
 *
 *   # CPU 剖析（采样 30s）
 *   go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
 */

import ws from 'k6/ws';
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Gauge } from 'k6/metrics';

const BASE_HTTP = 'http://127.0.0.1:8081';
const BASE_WS   = 'ws://127.0.0.1:8082';

const msgSent      = new Counter('stability_msgs_sent');
const msgReceived  = new Counter('stability_msgs_received');
const connActive   = new Gauge('stability_conn_active');

export const options = {
  scenarios: {
    stability: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 500 }, // 30s 内爬升到 500 连接
        { duration: '9m',  target: 500 }, // 保持 500 连接 9 分钟
        { duration: '30s', target: 0   }, // 30s 降到 0
      ],
    },
  },
  thresholds: {
    'ws_connecting':            ['p(99)<2000'],
    'stability_msgs_sent':      ['count>10000'],
  },
};

function login(vuIndex) {
  // 500 个 VU 映射到 bench_user_1..500（bench_user 用于稳定性测试，不做 sender/receiver 区分）
  const idx = ((vuIndex - 1) % 500) + 1;
  const email = `bench_user_${idx}@test.com`;
  const res = http.post(
    `${BASE_HTTP}/user/login`,
    JSON.stringify({ email, password: 'benchmark123' }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  if (res.status !== 200) {
    console.error(`login failed VU${vuIndex} (${email}): ${res.body}`);
    return null;
  }
  return res.json('data.jwtToken');
}

export default function () {
  const token = login(__VU);
  if (!token) {
    sleep(5);
    return;
  }

  ws.connect(`${BASE_WS}/ws?token=${token}`, {}, function (socket) {
    connActive.add(1);

    socket.on('open', () => {
      // 每 5s 发一条消息给自己（toId = fromId，测试服务端能正常处理）
      // 也可以发给固定的 bench_receiver_1，但自发自收更简单，不依赖特定用户在线
      socket.setInterval(() => {
        const msg = {
          Type:    0,
          MsgId:   generateUUID(),
          ToId:    __ENV.SELF_ID || '', // 留空则服务端验证失败，走 error 日志，不影响连接稳定性测试
          Content: JSON.stringify({ ts: Date.now() }),
        };
        socket.send(JSON.stringify(msg));
        msgSent.add(1);
      }, 5000);
    });

    socket.on('message', (data) => {
      try {
        const msg = JSON.parse(data);
        if (msg.Type === 0 && msg.MsgId) {
          // 收到消息回 ACK，防止 pender 积压
          socket.send(JSON.stringify({ Type: 4, MsgId: msg.MsgId, Content: 'ack' }));
          msgReceived.add(1);
        }
      } catch (_) {}
    });

    socket.on('error', (e) => {
      console.error(`VU${__VU} ws error: ${e}`);
    });

    socket.on('close', () => {
      connActive.add(-1);
    });

    // 保持连接 10 分钟
    socket.setTimeout(() => socket.close(), 600000);
  });
}

function generateUUID() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}
