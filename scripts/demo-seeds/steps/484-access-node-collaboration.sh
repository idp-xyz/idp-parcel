echo "== 节点监管协作承接与执行事实的授予、准入对照与权威区间（access-identity、pilot-governance；票 launch-wiring/04） =="
# 「节点监管协作承接」一种运营决定（ADR-0338 决定一）与「节点监管协作执行事实」一种作业事实（决定二）。承接是节点作业责任角色对关务协作
# 事项形成的决定，授予按种类单列，与监管运输处置承接、送站交接确认都分开；执行事实走作业事实登记面，不需要另一种操作者授予，要的是准入范围
# 对照与权威区间。照 477、482 两节的约定，配置员持有承接授予、演示租户在两种上各登一行准入对照、本产品各登一段权威区间——少一行，授予齐备也
# 答 OUTSIDE_ADMISSION_SCOPE。另起一节，理由同 455；它排在 450 之后，因为授予要指名那里登的操作者。谁持有承接授予是租户取值，这里登的是演示租户的
# 取值。种子不作任何承接、不登记任何执行事实：协作事项由关务一侧形成，决定与现场事实要人和设备看过事项之后作出。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/65-authority-interval-operation-decision-node-collaboration-acceptance.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/66-authority-interval-work-fact-node-collaboration-execution.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/28-grants-node-collaboration-acceptance.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/29-admission-scope-mapping-node-collaboration.json"
