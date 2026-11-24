echo "== 仓配履约：设备、操作者授予、准入范围对照与权威区间（access-identity、pilot-governance） =="
# 合成仓库设备两台绑 SYN-WH-01；配置员补齐仓配 18 种运营决定与 SYN-WH-01 作业事实登记授予，
# 并按 ADR-0384 决定八为 18 种运营决定（能力词=事实类型词=决定字面量）与 17 种仓配作业事实（能力词=WORK_FACT_REGISTRATION）
# 登记准入范围对照与权威区间，使管理台全部仓配弹窗与模拟 PDA 扫码作业台开箱即通。证据只记 S。
"$BIN/parcel-access-register" device-register -input "$SEEDS/access/47-devices-warehouse.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/48-grants-warehouse.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/49-admission-scope-mappings-warehouse.json"

for interval in "$SEEDS"/governance/80-authority-interval-warehouse-*.json; do
  "$BIN/parcel-governance-register" authority-interval -input "$interval"
done
