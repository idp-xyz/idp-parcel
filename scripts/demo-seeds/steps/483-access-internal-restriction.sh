echo "== 内部合规限制建立与决定的授予、准入对照与权威区间（access-identity、pilot-governance；票 launch-wiring/01） =="
# 「内部合规限制建立」「内部合规限制决定」两种运营决定（ADR-0334 决定七）：运营人员形成一份内部合规限制，与限制责任来源对限制自己的覆盖项追加确认或解除决定。
# 两种各成一种授予，彼此分开。照 477 那一节的约定，配置员持有它们、演示租户在每一种上登一行准入对照、本产品登一段权威区间——少一行，授予齐备也答
# OUTSIDE_ADMISSION_SCOPE。另起一节、不并进 450 的数据文件，理由同 455；它排在 450 之后，因为授予要指名那里登的操作者。谁持有这两种是租户取值，这里登的
# 是演示租户的取值。种子不建立任何限制、不作任何决定：限制来源、依据、覆盖项与解除条件都是租户取值。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/63-authority-interval-operation-decision-internal-restriction-establishment.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/64-authority-interval-operation-decision-internal-restriction-decision.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/26-grants-internal-restriction.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/27-admission-scope-mapping-internal-restriction.json"
