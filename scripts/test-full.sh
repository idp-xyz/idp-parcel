#!/usr/bin/env bash
# 本机含真库的全量测试：为这一跑起一只专用的一次性 PostgreSQL，经 Unix socket 连，包并发跑。
# 推送方「验的就是要推的那个对象」那一跑用它（见 docs/agents/parallel-sessions.md「全量只跑一次」）。
#
# 不直接对 55432 跑包并发，两条理由：
# - 本机（WSL）的临时端口区间窄（看 /proc/sys/net/ipv4/ip_local_port_range），经 Docker 端口
#   代理的连接一条占两个端口、关后在 TIME_WAIT 压一分钟；包并发时一跑要建的连接数足以把区间
#   耗尽，客户端读到 `connection reset by peer`。socket 不占端口。
# - 55432 是各会话共用的那只库，别人的建库删库与这一跑在同一实例上互相等。专用实例只服务这一跑。
#
# 包并发不改证据：pgtest 仍给每个用例一个物理库；实例与 compose.yaml 同镜像、同 tmpfs、不关 fsync。
# CI 的 `-p 1` 另有依据（磁盘库、-race，见 ci.yml Test 步），不随本脚本放开。
#
# 用法：
#   scripts/test-full.sh                      # go test -count=1 -p 8 ./...
#   scripts/test-full.sh -run Foo ./cmd/...   # 有参数时整串替换 ./...，包路径要自己写上
#   PARALLEL=4 scripts/test-full.sh           # 改包并发度
#   scripts/test-full.sh --detach             # 交给用户级 systemd，立刻返回；日志见下面的固定路径
#
# --detach 是给「Cursor 收掉 wsl.exe 时，前台这一跑会跟着死、专用库也被 EXIT 陷阱停掉」用的。
# 服务名 idp-parcel-test-full.service，日志 /tmp/idp-parcel-test-full.log，末行 FULL-EXIT=。
# 前台跑法的退出码仍是 go test 的，不印那一行。
set -euo pipefail

log=/tmp/idp-parcel-test-full.log
unit=idp-parcel-test-full.service

detach=0
if [[ "${1:-}" == "--detach" ]]; then
  detach=1
  shift
fi

cd "$(dirname "$0")/.."

# 非登录的 wsl 与 systemd --user 都不带这只 Go。本机就装在这里。
if [[ -x /usr/local/go/bin/go ]]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

if [[ "$detach" == 1 ]]; then
  if systemctl --user is-active --quiet "$unit"; then
    echo "已有一跑在进行：$unit，日志 $log" >&2
    exit 1
  fi
  systemctl --user reset-failed "$unit" 2>/dev/null || true
  : >"$log"
  # 用户级 systemd 是这台机器上一直活着的 init。Cursor 杀掉发起这次调用的 wsl.exe 时，服务不在那棵进程树上。
  systemd-run --user --collect --unit="${unit%.service}" \
    --working-directory="$PWD" \
    --setenv=IDP_PARCEL_TEST_FULL_MARK_EXIT=1 \
    --setenv="PARALLEL=${PARALLEL:-8}" \
    --setenv="PATH=$PATH" \
    --property="StandardOutput=append:$log" \
    --property="StandardError=append:$log" \
    bash "$PWD/scripts/test-full.sh" "$@"
  echo "日志 $log"
  exit 0
fi

# 与 compose.yaml、ci.yml 钉同一个补丁号：三处分叉时，本地绿而 CI 红就成了版本差异。
IMAGE=postgres:16.14
PARALLEL="${PARALLEL:-8}"

name="idp-parcel-test-full-$$"
# 挂进容器的是一个子目录：镜像入口会把 socket 目录 chown 给容器里的 postgres 用户（uid 999），
# 挂整个临时目录的话，收尾时宿主用户就删不掉它了。外层仍归自己，子目录空了即可连同删掉。
base="$(mktemp -d)"
sock="$base/run"
mkdir "$sock"
chmod 777 "$sock"

# 先正常停：PostgreSQL 关停时自己删 socket 与锁文件，子目录才是空的；强杀会把两个
# 归 uid 999 的文件留在里面，宿主用户删不动。
cleanup() {
  docker stop -t 10 "$name" >/dev/null 2>&1 || docker rm -f "$name" >/dev/null 2>&1 || true
  rm -rf "$base" 2>/dev/null || true
}
# 挂断不收尾：发起它的终端掉了，这一跑还要继续。INT/TERM 仍收尾，那是人要停。
trap '' HUP
trap cleanup EXIT INT TERM

docker run -d --rm --name "$name" \
  -e POSTGRES_USER=parcel \
  -e POSTGRES_PASSWORD=parcel \
  -e POSTGRES_DB=postgres \
  -e POSTGRES_INITDB_ARGS="--encoding=UTF8 --locale=C.UTF-8" \
  --tmpfs /var/lib/postgresql/data:size=2g \
  -v "$sock":/var/run/postgresql \
  "$IMAGE" postgres -c max_connections=200 >/dev/null

# 镜像初始化时会先起一只只听 socket 的临时实例跑 initdb 脚本、再停掉重起正式那只；只看
# pg_isready 会在临时实例上提前放行，用例随后撞上它关停。所以先等初始化完成那行日志。
ready=""
for _ in $(seq 1 120); do
  if docker logs "$name" 2>&1 | grep -q 'PostgreSQL init process complete' &&
    docker exec "$name" pg_isready -q -U parcel -d postgres; then
    ready=1
    break
  fi
  sleep 0.5
done
if [ -z "$ready" ]; then
  echo "专用测试库六十秒内未就绪：" >&2
  docker logs "$name" 2>&1 | tail -20 >&2
  exit 1
fi

export IDP_PARCEL_POSTGRES_DSN="postgres://parcel:parcel@/postgres?host=${sock}&port=5432&sslmode=disable"

if [ "$#" -eq 0 ]; then
  set -- ./...
fi

# go test 在管道上按块缓冲，Cursor 的采集要等块满才看得到下一个包。pty 上它逐行刷。
# script 的记录文件丢掉，只要它的刷新和子进程退出码。
set +e
if command -v script >/dev/null 2>&1; then
  script -qefc "$(printf '%q ' go test -count=1 -p "$PARALLEL" "$@")" /dev/null
  status=$?
else
  go test -count=1 -p "$PARALLEL" "$@"
  status=$?
fi
set -e
if [[ "${IDP_PARCEL_TEST_FULL_MARK_EXIT:-}" == 1 ]]; then
  echo "FULL-EXIT=$status"
fi
exit "$status"
