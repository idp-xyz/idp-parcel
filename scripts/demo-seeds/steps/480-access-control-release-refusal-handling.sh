echo "== 释放拒收处理的授予、准入对照与权威区间（access-identity、pilot-governance；票 docs-consistency/code/95） =="
# 「释放拒收处理」这一种运营决定（ADR-0293 决定三）：运维对释放未受理册上一笔被结算确定不受理的占用，核对后在结算侧释放或标记已处理，并让接受链
# 重驱那份委托的当前版本。它与授权处置是两种授予（职责不同：授权处置是对一份受限委托去向的业务处置，这一种是对已越过业务决定的资金占用的运维
# 处理）。照 477 那一节的约定，配置员持有它、演示租户在它上面登一行准入对照、本产品登一段权威区间——少一行，授予齐备也答
# OUTSIDE_ADMISSION_SCOPE。另起一节、不并进 450 的数据文件，理由同 455；它排在 450 之后，因为授予要指名那里登的操作者。谁持有这一种是租户取值，
# 这里登的是演示租户的取值。种子不记任何释放拒收、不作任何处理：册上的行由撤回、拒绝、处置等路遇到结算拒收时自己落。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/58-authority-interval-operation-decision-control-release-refusal-handling.json"
"$BIN/parcel-access-register" operator-grant -input "$SEEDS/access/20-grants-control-release-refusal-handling.json"
"$BIN/parcel-access-register" admission-scope-mapping-register -input "$SEEDS/access/21-admission-scope-mapping-control-release-refusal-handling.json"
