#!/usr/bin/env bash
# 步 0（票 seed-completeness/04）：对已经在跑的 parcel-api 打一次真提交，断言答复是已提交，
# 再从委托查阅把同一笔读回来。
#
# 不进 seed.sh。种子灌的是主数据；这一步是运行时命令，重复跑会多一笔委托（跨秒各落一笔）；同一 UTC 秒内再跑撞同一来源键，提交口答 200 EXISTING_RESULT，脚本因要求 201 失败退出、不落第二笔。
# 列表与详情在读 Intake 未配置时答 403（PAR-INT-01）。本脚本看到 403 就停，不换入口、不写库。
# 要让查阅答得上来，起 API 的方式与 parcel.sh 相同：隔离读、隔离写都设成 SYN-TENANT-01。
set -euo pipefail

base="${IDP_PARCEL_API_BASE:-}"
if [[ -z "$base" ]]; then
  printf 'submit-one-shipment: 需要 IDP_PARCEL_API_BASE，例如 http://127.0.0.1:19080\n' >&2
  exit 1
fi
base="${base%/}"

if ! command -v python3 >/dev/null 2>&1; then
  printf 'submit-one-shipment: 需要 python3 解析 JSON\n' >&2
  exit 1
fi

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
ref="SYN-CUSTREF-${stamp}"
parcel_ref="SYN-PCLREF-${stamp}"

body="$(python3 - "$ref" "$parcel_ref" <<'PY'
import json, sys
print(json.dumps({
    "customerShipmentReference": sys.argv[1],
    "requestedServiceProduct": "SYN-PROD-CN-SG-EXPRESS",
    "senderRelation": "SYN-SENDER-01",
    "senderAddress": "合成寄件地址",
    "recipientRelation": "SYN-RECIPIENT-01",
    "recipientAddress": "合成收件地址",
    "destinationServiceScope": "SYN-DEST/SG",
    "parcels": [{
        "customerParcelReference": sys.argv[2],
        "declaredWeightValue": "1.50",
        "declaredWeightUnit": "KG",
    }],
}, ensure_ascii=False))
PY
)"

post_file="$(mktemp)"
trap 'rm -f "$post_file"' EXIT
post_code="$(curl -sS -o "$post_file" -w '%{http_code}' -X POST \
  -H 'Content-Type: application/json' --data "$body" "${base}/shipment-requests")"

python3 - "$post_code" "$post_file" "$ref" <<'PY'
import json, sys
code, path, ref = sys.argv[1], sys.argv[2], sys.argv[3]
raw = open(path, encoding="utf-8").read()
if code == "403":
    sys.exit("提交口 403：隔离写未开。生产路径保持未配置（PAR-INT-01），本脚本不绕过。应答：\n" + raw)
if code != "201":
    sys.exit(f"提交口 HTTP {code}，要 201。应答：\n" + raw)
body = json.loads(raw)
if body.get("outcome") != "SUBMITTED":
    sys.exit(f"outcome = {body.get('outcome')!r}，要 SUBMITTED。应答：\n" + raw)
request_id = body.get("shipmentRequestId") or ""
if not request_id:
    sys.exit("201 里没有 shipmentRequestId。应答：\n" + raw)
open(path, "w", encoding="utf-8").write(request_id + "\n" + ref + "\n")
PY

request_id="$(sed -n '1p' "$post_file")"
ref="$(sed -n '2p' "$post_file")"

list_file="$(mktemp)"
trap 'rm -f "$post_file" "$list_file"' EXIT
list_code="$(curl -sS -o "$list_file" -w '%{http_code}' "${base}/shipment-request-views")"
python3 - "$list_code" "$list_file" "$request_id" <<'PY'
import json, sys
code, path, request_id = sys.argv[1], sys.argv[2], sys.argv[3]
raw = open(path, encoding="utf-8").read()
if code == "403":
    sys.exit("委托列表 403：读 Intake 仍未配置。提交已经落成已提交，查阅面不因写开关打开。本脚本不绕过。应答：\n" + raw)
if code != "200":
    sys.exit(f"委托列表 HTTP {code}，要 200。应答：\n" + raw)
body = json.loads(raw)
if body.get("outcome") != "LISTED":
    sys.exit(f"列表 outcome = {body.get('outcome')!r}，要 LISTED。应答：\n" + raw)
matched = [row for row in body.get("requests") or [] if row.get("shipmentRequestId") == request_id]
if len(matched) != 1 or matched[0].get("state") != "SUBMITTED":
    sys.exit(f"列表里没有状态为已提交的 {request_id}。应答：\n" + raw)
PY

detail_file="$(mktemp)"
trap 'rm -f "$post_file" "$list_file" "$detail_file"' EXIT
detail_code="$(curl -sS -o "$detail_file" -w '%{http_code}' \
  --get "${base}/shipment-request-views" --data-urlencode "shipmentRequestId=${request_id}")"
python3 - "$detail_code" "$detail_file" "$request_id" "$ref" <<'PY'
import json, sys
code, path, request_id, ref = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
raw = open(path, encoding="utf-8").read()
if code == "403":
    sys.exit("委托详情 403：读 Intake 仍未配置。本脚本不绕过。应答：\n" + raw)
if code != "200":
    sys.exit(f"委托详情 HTTP {code}，要 200。应答：\n" + raw)
body = json.loads(raw)
request = body.get("request") or {}
if body.get("outcome") != "REQUEST_VIEW" or request.get("shipmentRequestId") != request_id or request.get("state") != "SUBMITTED":
    sys.exit(f"详情不是这一笔已提交委托。应答：\n" + raw)
if request.get("sourceRequestKey") != ref:
    sys.exit(f"详情来源键 = {request.get('sourceRequestKey')!r}，要 {ref!r}。应答：\n" + raw)
print(f"已提交 {request_id} 来源键 {ref}；列表与详情都读到 SUBMITTED")
PY
