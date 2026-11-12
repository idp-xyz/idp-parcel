echo "== 信用政策重发（party-commercial：演示账户授信额度随 FBA 头程调高） =="
# 演示租户的信用政策 SYN-CREDIT-01 v1 额度 500000（分）：小包一件几十元，够连发几十件；一票三十来箱的 FBA 货件按演示售价卡
# 两万多元，信用检查答 AVAILABLE_CREDIT_INSUFFICIENT（票 declared-goods-lines/05 本地实测）。做 FBA 头程的租户给货主的授信
# 本就按整票货值定，这里照真实租户的做法同起点重发 v2 调高额度，再按 ADR-0233 决定三把 v1 让位给 v2——不改 v1 原文：已登过
# v1 的库照旧认得它，旧暴露也照旧指着 v1。v2 单放一份发布批而不并进 100 那一份：那一份的逐项回显被受控发布口的测试逐字钉着，
# 是演示种子首发的样本，重发是之后的一次登记。排在 100 之后：v1 由那一节登进来。额度是演示租户的合成采用值，不是
# 产品默认，证据记 S。
"$BIN/parcel-commercial" publish -input "$SEEDS/commercial/publish-credit-policy-reissue.json"
"$BIN/parcel-commercial" register-version-closures \
  -input "$SEEDS/commercial/close-credit-policy-reissue.json"
