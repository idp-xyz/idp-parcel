echo "== 手工录入称重量方的授予、准入对照与权威区间（access-identity、pilot-governance） =="
# 演示租户的配置员持有 MANUAL_MEASUREMENT_ENTRY。照既有各节的约定，授予、准入对照与权威区间三件齐，少一行也答 OUTSIDE_ADMISSION_SCOPE。
# 谁持有是租户取值，这里登的是演示租户的取值。种子不形成任何测量。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/78-authority-interval-operation-decision-manual-measurement-entry.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/39-grants-manual-measurement-entry.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/40-admission-scope-mapping-manual-measurement-entry.json"
