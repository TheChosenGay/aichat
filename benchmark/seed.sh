#!/bin/bash
# benchmark/seed.sh
#
# 在压测前批量创建测试用户
# 用法：bash benchmark/seed.sh
#
# 会创建以下用户：
#   bench_user_1@test.com ~ bench_user_1000@test.com    (连接压测用)
#   bench_sender_1@test.com ~ bench_sender_50@test.com  (消息压测 sender)
#   bench_receiver_1@test.com ~ bench_receiver_50@test.com (消息压测 receiver)

BASE_URL="http://localhost:8081"
PASSWORD="benchmark123"

create_user() {
  local email=$1
  local name=$2
  curl -s -X POST "${BASE_URL}/user/create" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${email}\",\"password\":\"${PASSWORD}\",\"name\":\"${name}\",\"sex\":true}" \
    > /dev/null
}

echo "创建连接压测用户 (bench_user_1 ~ bench_user_1000)..."
for i in $(seq 1 1000); do
  create_user "bench_user_${i}@test.com" "bench_user_${i}"
  if [ $((i % 100)) -eq 0 ]; then
    echo "  已创建 ${i}/1000"
  fi
done

echo "创建消息压测 sender (bench_sender_1 ~ bench_sender_50)..."
for i in $(seq 1 50); do
  create_user "bench_sender_${i}@test.com" "bench_sender_${i}"
done

echo "创建消息压测 receiver (bench_receiver_1 ~ bench_receiver_50)..."
for i in $(seq 1 50); do
  create_user "bench_receiver_${i}@test.com" "bench_receiver_${i}"
done

echo "完成！共创建 1100 个测试用户"
