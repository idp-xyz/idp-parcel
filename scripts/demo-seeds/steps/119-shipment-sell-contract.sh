echo "== 商业权威发布：逐委托销售的新客户从开户起只声明一个计价范围（票 07） =="
# 不改 SYN-CONTRACT-01 / 02，也不改 21、22。那几户的卡仍是逐包裹。
# 这一户的合同只声明 SYN-SCOPE-SHIP-23。范围上只挂一张逐委托销售卡，不与逐包裹的卡叠在同一个范围里。
# 证据只记 S。
"$BIN/parcel-commercial" publish -input "$SEEDS/commercial/publish-shipment-sell-contract.json"
"$BIN/parcel-commercial" register-resolution-key \
  -input "$SEEDS/commercial/resolution-key-syn-account-23.json"
"$BIN/parcel-commercial" register-parties -input "$SEEDS/commercial/register-shipment-sell-parties.json"
