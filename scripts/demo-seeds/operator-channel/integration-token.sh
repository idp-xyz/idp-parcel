#!/usr/bin/env bash
# 替一台合成集成客户端向演示的集成客户端发行方要一枚访问令牌（票 product-strategy-boundary/31）。许可是 OAuth 2.0
# 客户端凭据；范围带 audience:server:client_id:idp-parcel-integration，令牌的 aud 才含本产品的集成受众。
#
# 用法（发行方已按 compose.yaml 起好）：
#   scripts/demo-seeds/operator-channel/integration-token.sh <client-id> <secret> [access_token|claims]
#
# 客户端与口令是 integration-dex.yaml 里的合成值。claims 只解码载荷、不验签，供读 iss、sub 与 aud。
set -euo pipefail
if [[ $# -lt 2 || $# -gt 3 ]]; then
  printf '用法：%s <client-id> <secret> [access_token|claims]\n' "$0" >&2
  exit 1
fi
issuer="${IDP_PARCEL_DEMO_INTEGRATION_ISSUER:-http://127.0.0.1:5558/dex}"
response=$(curl -sS -u "$1:$2" \
  --data-urlencode grant_type=client_credentials \
  --data-urlencode "scope=openid audience:server:client_id:idp-parcel-integration" \
  "$issuer/token")
python3 - "${3:-access_token}" "$response" <<'PY'
import base64
import json
import sys

mode, raw = sys.argv[1], sys.argv[2]
body = json.loads(raw)
if "access_token" not in body:
    sys.exit(f"integration-token: 发行方没签：{raw}")
token = body["access_token"]
if mode == "access_token":
    print(token)
elif mode == "claims":
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    print(json.dumps(json.loads(base64.urlsafe_b64decode(payload)), ensure_ascii=False, indent=2))
else:
    sys.exit(f"integration-token: 不认的输出 {mode}")
PY
