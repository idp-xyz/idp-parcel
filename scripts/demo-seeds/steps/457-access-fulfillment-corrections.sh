echo "== 运输履约更正两口的授予、准入对照与权威区间（access-identity、pilot-governance；票 demo-chain-gaps/57） =="
# 「解除履约段采用」与「运输收费发生项失效」两种运营决定（ADR-0232 决定一、四）。照 450 那一节的约定，配置员持有它们、演示租户在
# 它们上面各登一行准入对照、本产品各登一段权威区间——少一行，授予齐备也答 OUTSIDE_ADMISSION_SCOPE。另起一节而不并进 450 与 400 的
# 数据文件，是因为种子按节新增、不改既有各节；它排在 450 之后，因为授予要指名那里登的操作者。证据只记 S。种子不解除任何采用、
# 不让任何发生项失效：两者都是运营人员按事实作出的更正。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/49-authority-interval-operation-decision-adoption-withdrawal.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/50-authority-interval-operation-decision-occurrence-invalidation.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/13-grants-fulfillment-corrections.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/14-admission-scope-mapping-fulfillment-corrections.json"
