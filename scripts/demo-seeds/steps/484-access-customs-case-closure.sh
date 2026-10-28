echo "== 关务案件关闭的授予、准入对照与权威区间（access-identity、pilot-governance；票 launch-wiring/02） =="
# 「关务案件关闭」运营决定（UC-CC-010 的关闭入口）：有权角色对进行中的关务案件请求关闭。与关务建案、授权确认、申报提交都分开授予；照 477 那一节的约定，配置员持有它、
# 演示租户登一行准入对照、本产品登一段权威区间——少一行，授予齐备也答 OUTSIDE_ADMISSION_SCOPE。另起一节、不并进 450 的数据文件，理由同 455；它排在 450 之后，
# 因为授予要指名那里登的操作者。谁持有它是租户取值，这里登的是演示租户的取值。种子不关任何案件：义务目录与逐项依据是租户取值（PAR-CUS-06）。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/65-authority-interval-operation-decision-customs-case-closure.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/28-grants-customs-case-closure.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/29-admission-scope-mapping-customs-case-closure.json"
