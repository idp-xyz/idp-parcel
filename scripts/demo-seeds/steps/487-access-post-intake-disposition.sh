echo "== 收寄后服务处置与待人工处理项登记的授予、准入对照与权威区间（access-identity、pilot-governance；票 launch-wiring/05） =="
# 两种运营决定：收寄后服务处置（形成继续、拦截、改送、退运、服务终止的决定，ADR-0332）与待人工处理项的处理结果登记（ADR-0340）。两者分开授予；照 477 那一节的约定，
# 配置员持有、演示租户登准入对照、本产品登权威区间——少一行，授予齐备也答 OUTSIDE_ADMISSION_SCOPE。另起一节、不并进 450 的数据文件，理由同 455；它排在 450 之后，
# 因为授予要指名那里登的操作者。谁持有它们是租户取值，这里登的是演示租户的取值。种子不形成任何决定、不登记任何结果。证据只记 S。
for interval in 69-authority-interval-operation-decision-post-intake-service-disposition 70-authority-interval-operation-decision-post-intake-manual-item-handling; do
  "$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/$interval.json"
done
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/34-grants-post-intake-disposition.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/35-admission-scope-mapping-post-intake-disposition.json"
