#!/usr/bin/env bash
# 测试分片的唯一定义处。CI 的每个测试 job 用 `list <片名>` 取自己的包清单，矩阵本身
# 用 `matrix` 取片名，静态 job 用 `check` 核对全部分片的并集恰好等于 `go list ./...`。
# 三处读的是同一份定义，分片、矩阵与核对因此不可能各说各话——把清单抄进 ci.yml 的
# 写法会在下一次加包、加片时留下一份没人记得改的副本。
#
# 分片按限界上下文切，不按字母序或包数均分：一片红了，job 名就说出是哪几个上下文；
# 同一上下文的包共享迁移计划与夹具，编译缓存命中也最高。分组依据是 2026-09-07 在
# `ffa6bd0e` 上本机 `-p 1 -count=1` 含 DSN 不带 `-race` 的实测：四片各 116–132 秒，真库
# 适配器包占了其中绝大部分（单包 12–61 秒），纯领域包都在百毫秒以下。
# 哪个上下文长出第二个重真库包、哪一片先撞上限，再挪——挪只改下面那张表。
#
# 最后一片 `rest` 不写清单，取「全部 − 显式片」：新加的包天然落进兑底片，不会因为没人
# 记得改分片表而从 CI 静默消失。显式片之间若重叠，`check` 会以并集多出一行的形式报出。
#
# 用法：
#   scripts/ci/test-shards.sh matrix         # 片名的 JSON 数组，喂给 GitHub Actions 的矩阵
#   scripts/ci/test-shards.sh list <片名>     # 该片的导入路径，一行一个；空片即非零退出
#   scripts/ci/test-shards.sh check          # 并集 == 全部、无重叠、无空片；违背即非零退出
set -euo pipefail

cd "$(dirname "$0")/../.."

# 显式片的片名，顺序即矩阵顺序。`rest` 不在此表，由 list_rest 现算。
EXPLICIT_SHARDS=(main-chain network-visibility customs-transport)

# 片名 → 目录前缀（相对模块根，空白分隔）。一个前缀覆盖该目录及其全部子包。
prefixes_of() {
  case "$1" in
    # 定价 → 运单 → 结算这条主链：三个上下文的真库包各自中等偏重，凑成一片正好一档。
    main-chain)         echo internal/parcelpricing internal/parcelshipment internal/settlementaccounting ;;
    # 网络、节点与可视化异常：VE 的真库与 inbox 两个重包占大头，NR/NO 补齐余量。
    network-visibility) echo internal/networkrouting internal/nodeoperations internal/visibilityexception ;;
    # 关务与运输履行：两个最重的真库包各占一半，再塞任何上下文都会让它先撞上限。
    customs-transport)  echo internal/customscompliance internal/transportfulfillment ;;
    *) echo "未知片名：$1" >&2; exit 2 ;;
  esac
}

# 全部包只在主 shell 里算一次，子 shell 继承算好的值。`go list ./...` 要解析模块图，是
# 几秒一次的开销；而下面的函数体几乎都跑在 `$(…)`、`<(…)` 与管道的子 shell 里，那里的
# 赋值回不到父 shell——惰性缓存若写在 all_packages 内部，每个子 shell 都各自重算一遍
# （`6f70c8d7` 上实测 `check` 一趟十六次）。所以由 list / check 的入口在主 shell 里先取；
# `matrix` 不经这里，它的 job 不装 Go。
load_all_packages() {
  if [ -n "${ALL_PACKAGES:-}" ] && [ -n "${MODULE_PATH:-}" ]; then
    return 0
  fi
  ALL_PACKAGES="$(go list ./... | LC_ALL=C sort)"
  MODULE_PATH="$(go list -m)"
}

all_packages() {
  printf '%s\n' "$ALL_PACKAGES"
}

# 按前缀过滤导入路径。匹配「等于前缀」或「前缀/…」两种，不匹配只是同名开头的目录
# （internal/parcelshipment 不能把 internal/parcelshipmentx 也捎上）。
filter_prefixes() {
  local rel pkg prefix
  while IFS= read -r pkg; do
    rel="${pkg#"$MODULE_PATH"/}"
    for prefix in "$@"; do
      if [[ "$rel" == "$prefix" || "$rel" == "$prefix"/* ]]; then
        printf '%s\n' "$pkg"
        break
      fi
    done
  done
}

list_explicit() {
  # shellcheck disable=SC2046  # 前缀按空白拆开正是这里要的
  all_packages | filter_prefixes $(prefixes_of "$1")
}

list_rest() {
  local shard
  # comm 要两侧都已排序：all_packages 已排，并集这里再排。
  LC_ALL=C comm -23 \
    <(all_packages) \
    <(for shard in "${EXPLICIT_SHARDS[@]}"; do list_explicit "$shard"; done | LC_ALL=C sort -u)
}

is_shard() {
  local shard
  for shard in "${EXPLICIT_SHARDS[@]}" rest; do
    [ "$shard" = "$1" ] && return 0
  done
  return 1
}

list_shard() {
  case "$1" in
    rest) list_rest ;;
    *) list_explicit "$1" ;;
  esac
}

cmd_matrix() {
  local shard json=""
  for shard in "${EXPLICIT_SHARDS[@]}" rest; do
    json="${json:+$json,}\"$shard\""
  done
  printf '[%s]\n' "$json"
}

cmd_list() {
  local packages
  # 先验片名再取清单：取清单跑在子 shell 里，那里的 exit 出不来，未知片名会被误报成空片。
  if ! is_shard "$1"; then
    echo "未知片名：$1（可用：${EXPLICIT_SHARDS[*]} rest）" >&2
    exit 2
  fi
  load_all_packages
  packages="$(list_shard "$1")"
  if [ -z "$packages" ]; then
    # 空清单展开给 `go test` 就是对当前目录跑测试——静默跑错对象比红更糟。
    echo "片 $1 为空：前缀大概写错了，或该上下文的包已全部挪走" >&2
    exit 1
  fi
  printf '%s\n' "$packages"
}

cmd_check() {
  local shard union count
  load_all_packages
  # 并集故意不去重：两片重叠会以「> 多出一行」露出来，去重就把它抹平了。
  union="$(for shard in "${EXPLICIT_SHARDS[@]}" rest; do list_shard "$shard"; done | LC_ALL=C sort)"
  if ! diff <(all_packages) <(printf '%s\n' "$union"); then
    echo "分片并集与 go list ./... 不一致：< 只在全部里（漏片），> 只在并集里（重叠）" >&2
    exit 1
  fi
  for shard in "${EXPLICIT_SHARDS[@]}" rest; do
    count="$(cmd_list "$shard" | wc -l | tr -d ' ')"
    echo "$shard: $count 个包"
  done
  echo "覆盖核对通过：$(all_packages | wc -l | tr -d ' ') 个包，无重叠、无遗漏、无空片"
}

case "${1:-}" in
  matrix) cmd_matrix ;;
  list)   cmd_list "${2:?用法：test-shards.sh list <片名>}" ;;
  check)  cmd_check ;;
  *)
    echo "用法：test-shards.sh matrix | list <片名> | check" >&2
    exit 2
    ;;
esac
