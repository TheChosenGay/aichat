/**
 * 场景二：消息吞吐压测
 *
 * 目标：测试持续互发消息时的 QPS 和端到端延迟
 *
 * 场景：
 *   - 1 个 receiver 保持连接，接收所有消息并计算延迟
 *   - 50 个 sender 持续向 receiver 发消息
 *
 * 运行：
 *   k6 run benchmark/ws_message.js
 */

import ws from 'k6/ws';
import http from 'k6/http';
import { Counter, Trend } from 'k6/metrics';

const BASE_HTTP = 'http://localhost:8081';
const BASE_WS   = 'ws://localhost:8082';

// receiver 的 userId（bench_receiver_1@test.com 的 userId）
const RECEIVER_USER_ID = __ENV.RECEIVER_ID || '2a659932-cdcf-4bd7-8770-afb5459f846b';

// 自定义指标
const msgSent     = new Counter('messages_sent');
const msgReceived = new Counter('messages_received');
const msgLatency  = new Trend('message_latency_ms', true);

export const options = {
  scenarios: {
    // 1 个 receiver 保持连接
    receiver: {
      executor: 'constant-vus',
      vus: 1,
      duration: '50s',
      env: { ROLE: 'receiver' },
    },
    // 50 个 sender 持续发消息
    // startTime: 10s，等 receiver 连上并且 Redis key 写入稳定后再发
    // duration: 30s，远小于 TTL 60s，排除 key 过期问题
    senders: {
      executor: 'constant-vus',
      vus: 50,
      duration: '30s',
      startTime: '10s',
      env: { ROLE: 'sender' },
    },
  },
  thresholds: {
    'messages_sent':      ['count>100'],   // 至少发 100 条
    'messages_received':  ['count>100'],   // 至少收到 100 条
    'message_latency_ms': ['p(95)<500'],   // 95% 延迟 < 500ms
  },
};

// 登录工具函数
function login(email, password) {
  const res = http.post(
    `${BASE_HTTP}/user/login`,
    JSON.stringify({ email, password }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  if (res.status !== 200) {
    console.error(`login failed for ${email}: ${res.body}`);
    return null;
  }
  return res.json('data.jwtToken');
}

export default function () {
  const role = __ENV.ROLE;

  if (role === 'receiver') {
    const token = login('bench_receiver_1@test.com', 'benchmark123');
    if (!token) return;

    ws.connect(`${BASE_WS}/ws?token=${token}`, {}, function (socket) {
      socket.on('open', () => {
        console.log(`receiver connected at ${new Date().toISOString()}`);
        // 连上后查一下 Redis 在线状态是否写入（通过服务端日志侧面确认）
      });

      socket.on('message', (data) => {
        try {
          const msg = JSON.parse(data);
          // MessageTypeText=0, MessageTypeAck=4
          // 收到文本消息后立即回 ACK，否则服务端 pending 积压导致 writeCh 堵死
          if (msg.Type === 0 && msg.MsgId) {
            socket.send(JSON.stringify({ Type: 4, MsgId: msg.MsgId, Content: 'ack' }));
          }
          if (msg.Type === 0 && msg.Content) {
            const payload = JSON.parse(msg.Content);
            if (payload.sentAt) {
              const latency = Date.now() - payload.sentAt;
              msgLatency.add(latency);
              msgReceived.add(1);
            }
          }
        } catch (_) {}
      });

      socket.on('error', (e) => {
        console.error(`receiver ws error: ${e}`);
      });

      // 保持连接 70s（比 sender 多 10s，确保所有消息都能收到）
      socket.setTimeout(() => socket.close(), 70000);
    });

  } else {
    // sender：每个 VU 用独立账号登录，向 receiver 持续发消息
    // VU 编号从 2 开始（VU 1 是 receiver），映射到 bench_sender_1..50
    const vuIndex = ((__VU - 1) % 50) + 1;
    const email = `bench_sender_${vuIndex}@test.com`;
    const token = login(email, 'benchmark123');
    if (!token) return;

    ws.connect(`${BASE_WS}/ws?token=${token}`, {}, function (socket) {
      socket.on('open', () => {
        // 每 500ms 发一条，content 里带上客户端发送时间戳
        socket.setInterval(() => {
          const msg = {
            Type:    0,                  // MessageTypeText
            MsgId:   generateUUID(),
            ToId:    RECEIVER_USER_ID,
            Content: JSON.stringify({ sentAt: Date.now(), text: 'bench' }),
          };
          socket.send(JSON.stringify(msg));
          msgSent.add(1);
        }, 100);
      });

      socket.on('error', (e) => {
        console.error(`sender VU${__VU} ws error: ${e}`);
      });

      socket.setTimeout(() => socket.close(), 60000);
    });
  }
}

// 简单 UUID 生成
function generateUUID() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}
