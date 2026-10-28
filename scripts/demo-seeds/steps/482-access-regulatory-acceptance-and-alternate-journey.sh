echo "== 监管运输处置承接与替代/退运旅程启动的授予、准入对照与权威区间（access-identity、pilot-governance；票 docs-consistency/code/43） =="
# 「监管运输处置承接」「替代或退运旅程启动」两种运营决定（ADR-0337 决定一）：运输运营对关务协作事项形成承接决定，与授权业务角色启动独立的备用或
# 退运旅程。两种各成一种授予，与运输委托提交、终止参与等都分开，也彼此分开。照 477 那一节的约定，配置员持有它们、演示租户在每一种上登一行准入
# 对照、本产品登一段权威区间——少一行，授予齐备也答 OUTSIDE_ADMISSION_SCOPE。另起一节、不并进 450 的数据文件，理由同 455；它排在 450 之后，
# 因为授予要指名那里登的操作者。谁持有这两种是租户取值，这里登的是演示租户的取值。种子不作任何承接、不启动任何旅程：协作事项由关务一侧形成，
# 决定要运营人员看过事项之后作出。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/61-authority-interval-operation-decision-regulatory-disposition-acceptance.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/62-authority-interval-operation-decision-alternate-journey-start.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/24-grants-regulatory-acceptance-and-alternate-journey.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/25-admission-scope-mapping-regulatory-acceptance-and-alternate-journey.json"
