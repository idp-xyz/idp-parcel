# 仓配履约默认关。开关没开时不灌仓储计费方案与采用：那些是租户取值，留空。
# 开着时仍不在这里预填单价。登记口是 /pricing-warehouse-plans 与 /warehouse-fulfillment-charge-adoptions，金额只用 SYN-。
if [[ "${IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED:-}" != "true" ]]; then
  echo "== 仓配履约关着，不灌仓储计费方案与采用 =="
  return 0
fi
echo "== 仓配履约开着：仓储计费方案与采用走登记口，种子不预填租户取值 =="
