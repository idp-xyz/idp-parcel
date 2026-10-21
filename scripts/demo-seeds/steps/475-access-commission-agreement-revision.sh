echo "== 委托协议换版的授予、准入对照与权威区间（access-identity、pilot-governance；票 demo-chain-gaps/88） =="
# 「委托协议换版」这一种运营决定（ADR-0231 决定三）：指名一份运输委托自哪一业务时刻起改按同一协议对象的后继版本采购。照 455 那一节的
# 约定，配置员持有它、演示租户在它上面登一行准入对照、本产品登一段权威区间——少一行，授予齐备也答 OUTSIDE_ADMISSION_SCOPE。另起一节、
# 不并进 450 与 400 的数据文件，理由同 455；它排在 450 之后，因为授予要指名那里登的操作者。种子不换任何委托的版：演示动线的委托指名的
# 就是声明了条件的那一版，换版要等真有一份委托所凭的那一版被收尾或缺条件时由运营人员决定。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/48-authority-interval-operation-decision-transport-commission-agreement-revision.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/11-grants-commission-agreement-revision.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/12-admission-scope-mapping-commission-agreement-revision.json"
