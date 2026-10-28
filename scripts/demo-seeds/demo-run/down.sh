#!/usr/bin/env bash
# 收掉 up.sh 起的一切（票 demo-2026-10-30/03）：停 parcel-api、parcel-dispatch、合成报关服务商，删私有发行方容器，清运行态。
# 库默认保留（现场演示完可能还要看）；带 --drop-db 才删库——只删 up.sh 自己新建的库，沿用的库不删。
# 共享的演示发行方（5556、5558）别的会话也在用，不停。
#
# 用法：scripts/demo-seeds/demo-run/down.sh [--drop-db]
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

drop=""
[[ "${1:-}" == "--drop-db" ]] && drop=1

if [[ ! -f "$RUN_DIR/env.sh" ]]; then
  echo "没有在记的演示环境（.run/env.sh 不存在），无事可收"
  exit 0
fi
# shellcheck disable=SC1091
source "$RUN_DIR/env.sh"

for name in parcel-api parcel-dispatch declaration-broker; do
  pidfile="$RUN_DIR/$name.pid"
  if [[ -f "$pidfile" ]]; then
    pid="$(cat "$pidfile")"
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      for _ in $(seq 20); do kill -0 "$pid" 2>/dev/null || break; sleep 0.25; done
      kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
      echo "已停 $name（PID $pid）"
    else
      echo "$name 早已不在（PID $pid）"
    fi
  fi
done

if [[ -n "${IDP_PARCEL_DEMO_PRIVATE_ISSUER_CONTAINER:-}" ]]; then
  docker rm -f "$IDP_PARCEL_DEMO_PRIVATE_ISSUER_CONTAINER" >/dev/null 2>&1 && echo "已删私有发行方 $IDP_PARCEL_DEMO_PRIVATE_ISSUER_CONTAINER"
fi

if [[ -n "$drop" ]]; then
  if [[ "${IDP_PARCEL_DEMO_DB_CREATED:-0}" == 1 ]]; then
    docker exec "$IDP_PARCEL_DEMO_PG_CONTAINER" psql -U parcel -d postgres -c "DROP DATABASE IF EXISTS ${IDP_PARCEL_DEMO_DB} WITH (FORCE)" >/dev/null
    echo "已删库 ${IDP_PARCEL_DEMO_DB}"
  else
    echo "库 ${IDP_PARCEL_DEMO_DB} 不是 up.sh 新建的，不删"
  fi
else
  echo "库 ${IDP_PARCEL_DEMO_DB} 保留（要删带 --drop-db）"
fi

rm -rf "$RUN_DIR"
echo "运行态已清（.run/）"
