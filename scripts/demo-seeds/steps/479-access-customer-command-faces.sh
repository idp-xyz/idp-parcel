echo "== 首方客户命令五口的授予、准入范围对照与权威区间（access-identity、pilot-governance；票 first-party-api/06，ADR-0310） =="
# 演示客户端（data/access/07 那一台）对五口各登一笔授予，五口的生产写在铸造侧判准入范围，所以每一口各登一行对照（能力
# CUSTOMER_COMMAND、事实类型取这一口的能力词）与一段权威区间；少一样，那一口铸造即答 403 范围外或归属未决，不代拟。对象范围沿用
# 客户命令那个试点范围（与送站预报两口同范围、不同事实类型，不撞键）。五口的领域 Intake 在 07–11 换上之前，授了也只答未配置。
# 另起一节而不并进 450 的授予文件与既有对照文件，是因为种子按节新增、不改既有各节。证据只记 S。
"$BIN/parcel-access-register" customer-client-grant -input "$SEEDS/access/18-customer-client-grants-command-faces.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/19-admission-scope-mappings-command-faces.json"
for interval in "$SEEDS"/governance/5[3-7]-authority-interval-customer-command-*.json; do
  "$BIN/parcel-governance-register" authority-interval -input "$interval"
done
