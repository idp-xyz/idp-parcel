echo "== 履约尝试待确认结果认定的授予、准入对照与权威区间（access-identity、pilot-governance；票 docs-consistency/code/90） =="
# 「揽收尝试结果认定」「派送尝试结果认定」两种运营决定（ADR-0297 决定二）：复核人核对来源之后，把一件对象在一次上门里还停在待确认的结果定成
# 确定结果。两侧各成一种授予，与建派送任务、终止参与、承运商首次有效收寄判断都分开。照 477 那一节的约定，配置员持有它们、演示租户在每一种上
# 登一行准入对照、本产品登一段权威区间——少一行，授予齐备也答 OUTSIDE_ADMISSION_SCOPE。另起一节、不并进 450 的数据文件，理由同 455；它排在
# 450 之后，因为授予要指名那里登的操作者。谁持有这两种是租户取值，这里登的是演示租户的取值。执行方在作业事实面的后补不需要这一节（作业事实登记
# 授予与在用设备已经在 450 登过）。种子不记任何待确认、不作任何认定：册上的待确认由一线设备报来的尝试自己落，列在
# GET /transport-fulfillment-records?registry=pending-attempt-result。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/59-authority-interval-operation-decision-pickup-attempt-result-confirmation.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/60-authority-interval-operation-decision-delivery-attempt-result-confirmation.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/22-grants-attempt-result-confirmation.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/23-admission-scope-mapping-attempt-result-confirmation.json"
