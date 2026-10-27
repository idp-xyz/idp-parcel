#!/usr/bin/env bash
# PR 上的生成物门：只核「生成器跑得动、输入没坏」，不要求生成物新鲜。
#
# 三件生成物——docs/product/MECHANISM-INVENTORY.md、docs/adr/README.md 的两个列表、
# .scratch/at-coverage-inventory.md 的总览两表——同一条规矩：票不在自己的笔上重生成，由推送方
# 在 main 的 tip 上重生成、单独成笔（docs/agents/parallel-sessions.md「生成物不占号」）。
# 所以 PR 上「与提交的不一致」是常态，不是缺陷；这里要求新鲜，每张新增文件、迁移、ADR 或移了
# AT 行的票都会按设计红，红就不再是信号。要求新鲜的门只挂在 main 的推送上（ci.yml）。
#
# 但有一类问题重生成救不了，必须作者自己修，这里照红：
#   adr-index、at-coverage-counts 的 -check：退 1 = 只是过期（放行）；退 2 及以上 = 生成区标记
#     找不到或不成对、ADR 读不进，或 UC 立了号而盘点分桶表缺行或多行（红，要补表行）。
#   机制清点没有 -check，改为生成到临时文件：生成器非零退出，或报告里带「包加载报错」（端口清点的
#     数不可信），为红；与提交的那份不同只打印一行。
#
# 两个 -check 工具先编成二进制再跑，不用 `go run`：go run 把子进程的任何非零退出都压成 1，
# 退 1（过期）与退 2（必须修）在它后面分不出来。
#
# 用法（本机与 CI 同一条命令，在任意检出里跑，不改工作区）：
#   bash scripts/ci/generated-artifacts-pr.sh
set -uo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
red=0

# 编不出来本身就是红；往下跑只会把同一个原因再报一遍。
go build -o "$work/adr-index" ./tools/adr-index || exit 1
go build -o "$work/at-coverage-counts" ./tools/at-coverage-counts || exit 1

# 标记区生成物：按退出码分格。
marked_region() {
  local name="$1" out rc
  out="$("$work/$name" -check -dir "$root" 2>&1)"
  rc=$?
  case "$rc" in
    0) echo "$name: 与提交的一致" ;;
    1) echo "$name: 过期。PR 不要求新鲜，由推送方在 main 的 tip 上重生成" ;;
    *)
      echo "$name: 退 $rc。这不是过期，重生成救不了，要先修输入：" >&2
      echo "$out" >&2
      red=1
      ;;
  esac
}

marked_region adr-index
marked_region at-coverage-counts

# 机制清点：生成到临时文件，工作区不动。
inventory="$work/MECHANISM-INVENTORY.md"
log="$(cd tools/mechanism-inventory && go run . -dir "$root" -out "$inventory" 2>&1)"
rc=$?
if [ "$rc" -ne 0 ]; then
  echo "mechanism-inventory: 生成器退 $rc，清点跑不动：" >&2
  echo "$log" >&2
  red=1
elif grep -q '包加载报错' "$inventory"; then
  echo "mechanism-inventory: 报告里带「包加载报错」，端口清点的数不可信：" >&2
  grep -A20 '包加载报错' "$inventory" >&2
  red=1
elif cmp -s "$inventory" docs/product/MECHANISM-INVENTORY.md; then
  echo "mechanism-inventory: 与提交的一致"
else
  echo "mechanism-inventory: 过期（或提交里没有这份文件）。PR 不要求新鲜，由推送方在 main 的 tip 上重生成"
fi

exit "$red"
