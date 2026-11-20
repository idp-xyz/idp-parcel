echo "== 面单渠道服务产品（演示租户合成采用，证据只记 S） =="
# SYN-PROD-CN-SG-LABEL：形态在下一行登记为 LABEL_CHANNEL_SERVICE。快递与经济两条仍是 NETWORK_SERVICE，这一步不改它们。
# 接单规则包与客户服务规则都只指名这一件产品，闭包才不会借快递那一份。经营口径采用仍是租户那一行
# （settlement/09-operating-basis-adoption.json 已经显式采用预估和已确认），这里不写产品默认。
# 渠道映射写 []：这一件还没有渠道候选，显式未配置，不替它选一个渠道。
# 排在 116 之后：合同 v2 已不再指名规则包，这一份规则包按申报的产品自己被选中。
"$BIN/parcel-commercial" publish -input "$SEEDS/commercial/publish-label-channel-product.json"
"$BIN/parcel-commercial" register-products -input "$SEEDS/commercial/register-products-label.json"
