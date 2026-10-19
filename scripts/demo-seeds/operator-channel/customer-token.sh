#!/usr/bin/env bash
# 替一台合成首方客户端向演示发行方要一枚访问令牌（票 first-party-api/00，ADR-0139 决定二）。许可是 OAuth 2.0 客户端凭据；
# 范围带 audience:server:client_id:idp-parcel-customer-api，令牌的 aud 才含本产品客户 API 的受众。发行方与集成客户端族
# 是同一只（integration-dex.yaml），受众不同。
#
# 用法（发行方已按 compose.yaml 起好）：
#   scripts/demo-seeds/operator-channel/customer-token.sh [client-id] [secret] [access_token|claims]
#
# 客户端与口令缺省取 integration-dex.yaml 里那台合成货主系统（syn-customer-client-01 / syn-customer-client-secret）。
# claims 只解码载荷、不验签，供读 iss、sub 与 aud。
set -euo pipefail
if [[ $# -gt 3 ]]; then
  printf '用法：%s [client-id] [secret] [access_token|claims]\n' "$0" >&2
  exit 1
fi
issuer="${IDP_PARCEL_DEMO_CUSTOMER_ISSUER:-http://127.0.0.1:5558/dex}"
client="${1:-syn-customer-client-01}"
secret="${2:-syn-customer-client-secret}"
response=$(curl -sS -u "$client:$secret" \
  --data-urlencode grant_type=client_credentials \
  --data-urlencode "scope=openid audience:server:client_id:idp-parcel-customer-api" \
  "$issuer/token")
python3 - "${3:-access_token}" "$response" <<'PY'
import base64
import json
import sys

mode, raw = sys.argv[1], sys.argv[2]
body = json.loads(raw)
if "access_token" not in body:
    sys.exit(f"customer-token: 发行方没签：{raw}")
token = body["access_token"]
if mode == "access_token":
    print(token)
elif mode == "claims":
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    print(json.dumps(json.loads(base64.urlsafe_b64decode(payload)), ensure_ascii=False, indent=2))
else:
    sys.exit(f"customer-token: 不认的输出 {mode}")
PY
