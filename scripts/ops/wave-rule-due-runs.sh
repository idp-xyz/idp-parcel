#!/usr/bin/env bash
# 参考脚本：把到点的成波规则放出去。随产品发布，给部署方照着配定时任务用；产品里没有定时器。
#
# 它只做一件事：带着一个持有「放波次」运营决定授予的操作者凭证，调一次「放到点的规则」那一口。
# 几点该放由成波规则上登的「每天几点放」定（按仓的业务时区）；这一口每次只放此刻已经到点、今天还没办过的时点，
# 所以调得比规则上的时点密没有坏处——每五分钟调一次，规则上登 09:00 的那一波会在 09:00 到 09:05 之间放出。
# 同一个时点只办一次：办成了、或没有单可放，都算办过；别的结果（比如没采用分配策略）不算，下一次再试。
#
# 要给的三样都是部署的取值，脚本不带默认：
#   IDP_PARCEL_API_URL               parcel-api 的地址，不带结尾的斜杠
#   IDP_PARCEL_OPERATOR_TOKEN_COMMAND 一条命令，标准输出只打印一枚此刻有效的操作者访问令牌。令牌怎么来取决于
#                                    租户的身份发行方（常见的是给一个专门的服务账号走客户端凭证或刷新令牌），
#                                    这一步各家不同，所以留成一条命令；令牌本身不写进这个文件、不进仓库。
#   IDP_PARCEL_WAVE_RULE_WAREHOUSE   可选：只放这一个仓的规则；不给是本租户全部的仓。
#
# 那个服务账号要先在「操作者与授予」登成操作者，授「运营决定 · WAVE_RELEASE」；放出的波次记在它名下。
#
# 退出码：0 是这一次调成了（放出了波次，或此刻没有到点的）；1 是没调成（网络、凭证、授予、后端没答出），
# 由定时任务的失败告警接住。哪条规则放了什么在标准输出里，是后端答复的原文。
#
# systemd 定时任务的样子（每五分钟一次）：
#
#   # /etc/systemd/system/idp-parcel-wave-rule-due-runs.service
#   [Service]
#   Type=oneshot
#   EnvironmentFile=/etc/idp-parcel/wave-rule-due-runs.env
#   ExecStart=/opt/idp-parcel/scripts/ops/wave-rule-due-runs.sh
#
#   # /etc/systemd/system/idp-parcel-wave-rule-due-runs.timer
#   [Timer]
#   OnCalendar=*:0/5
#   Persistent=true
#   [Install]
#   WantedBy=timers.target
#
# cron 的样子：*/5 * * * * . /etc/idp-parcel/wave-rule-due-runs.env && /opt/idp-parcel/scripts/ops/wave-rule-due-runs.sh
set -euo pipefail

: "${IDP_PARCEL_API_URL:?要给 parcel-api 的地址}"
: "${IDP_PARCEL_OPERATOR_TOKEN_COMMAND:?要给一条打印操作者访问令牌的命令}"

token="$(bash -c "$IDP_PARCEL_OPERATOR_TOKEN_COMMAND")"
if [ -z "$token" ]; then
  echo "取令牌的命令没有打印出令牌" >&2
  exit 1
fi

payload='{}'
if [ -n "${IDP_PARCEL_WAVE_RULE_WAREHOUSE:-}" ]; then
  # 仓编码原样进 JSON：只转义反斜杠与双引号，别的字符仓编码里不该有。
  escaped="${IDP_PARCEL_WAVE_RULE_WAREHOUSE//\\/\\\\}"
  escaped="${escaped//\"/\\\"}"
  payload="{\"warehouse\":\"${escaped}\"}"
fi

answer="$(mktemp)"
trap 'rm -f "$answer"' EXIT
status="$(curl --silent --show-error --max-time 120 --output "$answer" --write-out '%{http_code}' \
  --request POST "${IDP_PARCEL_API_URL}/warehouse-fulfillment-wave-rule-due-runs" \
  --header "Authorization: Bearer ${token}" --header 'Content-Type: application/json' --data "$payload")" || {
  echo "没有调通 ${IDP_PARCEL_API_URL}" >&2
  exit 1
}
cat "$answer"
echo
case "$status" in
  200 | 201) exit 0 ;;
  *)
    echo "后端答 HTTP ${status}，这一次没有办成" >&2
    exit 1
    ;;
esac
