echo "== 商业权威发布：两位新客户从开户起声明销售价格来源（票 commercial-four-module-requirements/21） =="
# 不改 SYN-CONTRACT-01 / 02。那两户仍是没有销售价格约定的旧规范化版本，采用册继续带着计价范围。
# 给已登记客户换列表要发新版，解析键的锚点是登记下来的时刻；那一步在 product-strategy-boundary/06 解冻之前走不通。
# 所以这两位是新开的户：合同 v1 从 2026-01-01 起就带约定。21 只有公共范围 SYN-SCOPE-01；22 把专属范围排在前面。
"$BIN/parcel-commercial" publish -input "$SEEDS/commercial/publish-sell-price-contracts.json"
"$BIN/parcel-commercial" register-resolution-key \
  -input "$SEEDS/commercial/resolution-key-syn-account-21.json"
"$BIN/parcel-commercial" register-resolution-key \
  -input "$SEEDS/commercial/resolution-key-syn-account-22.json"
"$BIN/parcel-commercial" register-parties -input "$SEEDS/commercial/register-sell-price-parties.json"
