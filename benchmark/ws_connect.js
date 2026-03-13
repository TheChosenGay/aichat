/**
 * 场景一：WebSocket 连接压测
 *
 * 目标：验证 1000 个并发 WebSocket 连接下服务的稳定性
 *
 * 运行前准备：
 *   1. 启动服务：make run
 *   2. 在 DB 中准备一批测试用户（运行 benchmark/seed.sh）
 *   3. 将登录后拿到的 token 填入下方 TOKENS 数组
 *
 * 运行：
 *   k6 run benchmark/ws_connect.js
 */

import ws from 'k6/ws';
import { check, sleep } from 'k6';
import http from 'k6/http';

const BASE_HTTP = 'http://localhost:8081';
const BASE_WS   = 'ws://localhost:8082';

// 压测配置
export const options = {
  stages: [
    { duration: '30s', target: 100  }, // 30s 内爬升到 100 个连接
    { duration: '30s', target: 500  }, // 再 30s 爬升到 500
    { duration: '60s', target: 1000 }, // 再 60s 爬升到 1000
    { duration: '60s', target: 1000 }, // 保持 1000 个连接 60s
    { duration: '30s', target: 0    }, // 30s 内降到 0
  ],
  thresholds: {
    'ws_connecting':         ['p(95)<1000'], // 95% 连接建立时间 < 1s
    'ws_session_duration':   ['p(95)<35000'], // 95% session 时长 < 35s（主动保持 30s）
    'checks':                ['rate>0.95'],  // 95% 的检查通过
  },
};

// 每个 VU（虚拟用户）执行的逻辑
export default function () {
  // 第一步：登录拿 token
  // 使用 __VU（虚拟用户编号）区分不同用户，需提前创建足够数量的测试用户
  const email    = `bench_user_${__VU}@test.com`;
  const password = 'benchmark123';

  const loginRes = http.post(
    `${BASE_HTTP}/user/login`,
    JSON.stringify({ email, password }),
    { headers: { 'Content-Type': 'application/json' } },
  );

  const ok = check(loginRes, {
    'login status 200': (r) => r.status === 200,
    'login has token':  (r) => r.json('data.jwtToken') !== '',
  });

  if (!ok) {
    console.error(`VU ${__VU} login failed: ${loginRes.body}`);
    return;
  }

  const token = loginRes.json('data.jwtToken');

  // 第二步：建立 WebSocket 连接
  const url = `${BASE_WS}/ws?token=${token}`;
  const res = ws.connect(url, {}, function (socket) {
    socket.on('open', () => {
      check(socket, { 'ws connected': () => true });
    });

    socket.on('message', (data) => {
      // 收到服务端推送的历史消息，检查格式
      const msg = JSON.parse(data);
      check(msg, { 'msg has type': (m) => m.type !== undefined });
    });

    socket.on('error', (e) => {
      console.error(`VU ${__VU} ws error: ${e}`);
    });

    // 保持连接 30s 后断开
    socket.setTimeout(() => socket.close(), 30000);
  });

  check(res, { 'ws status 101': (r) => r && r.status === 101 });
}
