#!/usr/bin/env bash
# 合成 S 主数据种子包一键灌入（票 master-data-wiring/08）。**仅限隔离环境**：
# 种子全部是 SYN- 前缀的合成实例（证据层级 S），不影射任何真实企业；本脚本
# 绝不能指向生产库——它信任 IDP_PARCEL_POSTGRES_DSN，不做「这是不是演示库」的猜测。
#
# 用法：
#   IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable \
#     ./scripts/demo-seeds/seed.sh [--reset]
#
#   --reset  先 DROP 全部 parcel schema 再重迁重灌（干净库复灌用，破坏性，仅限演示库）。
#
# 干净库上全程零报错；各登记 CLI 的非零退出码会经 set -e 中止脚本并如实透出。
set -euo pipefail

cd "$(dirname "$0")/../.."

: "${IDP_PARCEL_POSTGRES_DSN:?IDP_PARCEL_POSTGRES_DSN 未设置——种子脚本不猜连接串}"

RESET_FLAG=""
if [[ "${1:-}" == "--reset" ]]; then
  RESET_FLAG="-reset"
fi

SEEDS=scripts/demo-seeds/data
BIN="$(mktemp -d)"
trap 'rm -rf "$BIN"' EXIT

echo "== 编译登记 CLI 与迁移助手 =="
go build -o "$BIN/" \
  ./cmd/parcel-pricing-register \
  ./cmd/parcel-network-register \
  ./cmd/parcel-customs-register \
  ./cmd/parcel-commercial \
  ./cmd/parcel-ve-register \
  ./cmd/parcel-collection-register \
  ./cmd/parcel-governance-register \
  ./cmd/parcel-access-register \
  ./cmd/parcel-settlement-register \
  ./cmd/parcel-transport-register \
  ./cmd/parcel-node-operations-register \
  ./scripts/demo-seeds/migrate

echo "== 施加迁移计划（${RESET_FLAG:-不重置}） =="
"$BIN/migrate" $RESET_FLAG

# 灌入步骤一节一个文件，放在 steps/ 下，这里逐个 source 进本 shell：步骤直接用上面的 $BIN 与 $SEEDS，
# 任一条命令非零照样经 set -e 中止整个脚本。先后只看文件名，按 C 字典序排，不随调用者的区域设置变；
# 取号与加种子的规矩写在 README「加一条种子」。
shopt -s nullglob
steps=(scripts/demo-seeds/steps/*.sh)
shopt -u nullglob
if ((${#steps[@]} == 0)); then
  echo "scripts/demo-seeds/steps/ 下没有步骤文件：一条种子也不会灌，不当作成功" >&2
  exit 1
fi
for step in "${steps[@]}"; do
  # 没有三位数前缀的文件按 C 序会排到所有数字之后，跑是能跑，排在哪却不是作者选的。
  if [[ ! "${step##*/}" =~ ^[0-9]{3}-.+\.sh$ ]]; then
    echo "步骤文件名要以三位数前缀加连字符起头：$step" >&2
    exit 1
  fi
done
mapfile -t steps < <(printf '%s\n' "${steps[@]}" | LC_ALL=C sort)
for step in "${steps[@]}"; do
  source "$step"
done

echo "种子灌入完成：租户 SYN-TENANT-01，各上下文全部落库。"
