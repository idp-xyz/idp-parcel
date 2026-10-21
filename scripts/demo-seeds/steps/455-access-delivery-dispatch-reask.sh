echo "== 派送重问的授予、准入对照与权威区间（access-identity、pilot-governance；票 demo-chain-gaps/70） =="
# 「派送重问」这一种运营决定（ADR-0230 决定四、七）：按派送待形成册批量重问与对单个（段，对象）补拍共用它。照 450 那一节的
# 约定，配置员持有它、演示租户在它上面登一行准入对照、本产品登一段权威区间——少一行，两口授予齐备也答 OUTSIDE_ADMISSION_SCOPE。
# 另起一节而不并进 450 与 400 的数据文件，是因为种子按节新增、不改既有各节；它排在 450 之后，因为授予要指名那里登的操作者。
# 证据只记 S。重问间隔不在这里登：它是租户取值，演示租户也不替自己选一个（到期重问如实答未配置）。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/45-authority-interval-operation-decision-delivery-dispatch-reask.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/09-grants-delivery-dispatch-reask.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/10-admission-scope-mapping-delivery-dispatch-reask.json"
