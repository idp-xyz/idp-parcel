#!/usr/bin/env bash
# 比对两版 seed.sh 的调用序列：把登记 CLI 与迁移助手换成只记「程序名 + 全部参数」的桩，分别跑基准与对照，
# 两份调用记录（连同 seed.sh 的标准输出）逐字节相同才退 0。
#
# 它守的是「只改 seed.sh 与 steps/ 的形状、不改任何一次调用」这一类改动（票 infra-speed/04 的拆分即是）。
# 加种子的票本来就会改调用序列，不拿它来比。
#
# 用法：
#   scripts/demo-seeds/compare-seed-calls.sh <基准 rev> [<对照 rev>]
#   不给对照 rev 时，对照的是当前工作树（含未提交、未跟踪的步骤文件）。
#   每一侧跑两遍：不带参数一遍、带 --reset 一遍。
#
# 桩怎么接上：seed.sh 自己 `go build -o "$BIN/" …` 编出各 CLI。这里在 PATH 最前面垫一个假的 `go`，它不编译，只按
# 每个包路径的末段在 $BIN 里写一个桩。被比的 seed.sh 因此一字不改、不带任何测试开关，任何一版（包括还没有本脚本的
# 旧版）都能当基准。假 go 只认 `go build -o <目录>/ <包>…` 这一种形状，seed.sh 换了编法它就报错退出，不会默默比两份空记录。
# 不连任何库：桩不读 DSN，seed.sh 只要求 DSN 非空，这里给一个连不上的占位串。
set -euo pipefail

usage="用法：compare-seed-calls.sh <基准 rev> [<对照 rev>]"
base_rev="${1:?$usage}"
head_rev="${2:-}"

root="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

mkdir "$work/fakebin"
cat >"$work/fakebin/go" <<'FAKE_GO'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != build || "${2:-}" != -o || "${3:-}" != */ ]]; then
  echo "假 go：只认 go build -o <目录>/ <包>…，收到：go $*" >&2
  exit 97
fi
out="$3"
shift 3
# $BIN 是 seed.sh 每跑一次新取的临时目录，记成字面量才两侧可比。
{ printf 'go build -o $BIN/'; printf ' %q' "$@"; printf '\n'; } >>"$SEED_CALL_LOG"
for pkg in "$@"; do
  cat >"$out${pkg##*/}" <<'STUB'
#!/usr/bin/env bash
{ printf '%q' "${0##*/}"; printf ' %q' "$@"; printf '\n'; } >>"$SEED_CALL_LOG"
STUB
  chmod +x "$out${pkg##*/}"
done
FAKE_GO
chmod +x "$work/fakebin/go"

# 取一版 scripts/demo-seeds 到临时目录，返回其中 seed.sh 的路径。seed.sh 先 cd 到自己的仓库根，参数里的数据路径
# 都是相对仓库根写的，所以临时目录里的那一版与工作树那一版记下来的路径同形。
materialize() {
  local rev="$1" dir="$work/tree-$2"
  mkdir "$dir"
  git -C "$root" archive "$rev" scripts/demo-seeds | tar -x -C "$dir"
  printf '%s\n' "$dir/scripts/demo-seeds/seed.sh"
}

run_side() {
  local label="$1" seed="$2" mode
  for mode in plain reset; do
    local flag=()
    [[ "$mode" == reset ]] && flag=(--reset)
    if ! SEED_CALL_LOG="$work/$label-$mode.calls" PATH="$work/fakebin:$PATH" \
      IDP_PARCEL_POSTGRES_DSN='stub://compare-seed-calls/不连库' \
      bash "$seed" "${flag[@]}" >"$work/$label-$mode.out" 2>"$work/$label-$mode.err"; then
      echo "$label（$mode）的 seed.sh 非零退出，标准错误：" >&2
      cat "$work/$label-$mode.err" >&2
      exit 1
    fi
    if [[ ! -s "$work/$label-$mode.calls" ]]; then
      echo "$label（$mode）一条调用也没记下：桩没接上，比对无意义" >&2
      exit 1
    fi
  done
}

base_seed="$(materialize "$base_rev" base)"
if [[ -n "$head_rev" ]]; then
  head_seed="$(materialize "$head_rev" head)"
  head_label="$head_rev"
else
  head_seed="$root/scripts/demo-seeds/seed.sh"
  head_label="工作树"
fi

run_side base "$base_seed"
run_side head "$head_seed"

status=0
for mode in plain reset; do
  for kind in calls out; do
    a="$work/base-$mode.$kind"
    b="$work/head-$mode.$kind"
    if cmp -s "$a" "$b"; then
      printf '相同  %-5s %-5s %4d 行  sha256 %s\n' "$mode" "$kind" "$(wc -l <"$a")" "$(sha256sum "$a" | cut -c1-16)"
    else
      printf '不同  %-5s %-5s（基准 %s ↔ 对照 %s）\n' "$mode" "$kind" "$base_rev" "$head_label"
      diff -u --label "基准 $base_rev" --label "对照 $head_label" "$a" "$b" || true
      status=1
    fi
  done
done
exit "$status"
