echo "== 委托提交的准入范围对照与权威区间（access-identity、pilot-governance；票 first-party-api/04，ADR-0308） =="
# 生产形态（不设写开关）下 PS 的生产归属目录读这一行对照换治理坐标，区间才答「谁在写生产」：两样少一样，首方口提交就答
# OWNERSHIP_UNRESOLVED（不代拟）。词是产品词（能力 CUSTOMER_COMMAND、事实类型 SHIPMENT_REQUEST_SUBMISSION），对象范围与权威串
# 带 SYN-。对象范围沿用委托受理那一个试点范围，与 07 号区间（写开关的合成坐标，随写开关退场）同范围、不同能力与事实类型，
# 不撞键。另起一节而不并进 450 与 400 的数据文件，是因为种子按节新增、不改既有各节。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/52-authority-interval-customer-command-shipment-request-submission.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/17-admission-scope-mapping-shipment-request-submission.json"
