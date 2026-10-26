echo "== 授权改路的授予、准入对照与权威区间（access-identity、pilot-governance；票 docs-consistency/code/29） =="
# 「授权改路」这一种运营决定（ADR-0284 决定一）：授权角色据一份改路建议择一候选、形成改路决定与新计划。照 455 那一节的约定，配置员持有
# 它、演示租户在它上面登一行准入对照、本产品登一段权威区间——少一行，授予齐备也答 OUTSIDE_ADMISSION_SCOPE。另起一节、不并进 450 与 400
# 的数据文件，理由同 455；它排在 450 之后，因为授予要指名那里登的操作者。谁持有这一种是租户取值，这里登的是演示租户的取值。种子不作任何
# 改路决定：建议由复核形成，决定要运营人员看过建议之后作出。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/51-authority-interval-operation-decision-authorized-reroute.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/15-grants-authorized-reroute.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/16-admission-scope-mapping-authorized-reroute.json"
