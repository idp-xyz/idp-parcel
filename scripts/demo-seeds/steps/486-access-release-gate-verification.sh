echo "== 放行门禁核对的授予、准入对照与权威区间（access-identity、pilot-governance；票 launch-wiring/03） =="
# 「放行门禁核对」运营决定（UC-CC-009）：有权角色对（申报范围，拟执行动作，监管边界）请求形成放行门禁核对。与关务建案、申报提交、关案都分开授予；照 477 那一节的约定，
# 配置员持有它、演示租户登一行准入对照、本产品登一段权威区间——少一行，授予齐备也答 OUTSIDE_ADMISSION_SCOPE。另起一节、不并进 450 的数据文件，理由同 455；它排在 450 之后，
# 因为授予要指名那里登的操作者。谁持有它是租户取值，这里登的是演示租户的取值。种子不请求任何核对：门禁目录登了（250 那一节），「税费付款」那一道的规则行没有
# 种——它的取值是租户取值，本产品也没有登记面，核对在那之前答 DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/68-authority-interval-operation-decision-release-gate-verification.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/32-grants-release-gate-verification.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/33-admission-scope-mapping-release-gate-verification.json"
