echo "== 代收与清分登记（collection-remittance：分户账、指令、事实与记账） =="
# 一条 COD 指令走全程（票 admin-remainder-mechanism-batch/04）：渠道报收全额入账
# `渠道在途`；银行实际到账少于渠道报收，到账部分凭该到账事实搬进`待清分`，差额先登
# 短款差异事项、再凭它落进`短款`——差额不自动落账；到账部分凭 ALLOCATION（指向代收
# 指令）清分进`应付客户`，进应付客户不得凭代收事实（第四条依据门）。SGD 分户账只
# 开立不记账，让「已开立但当期无记账」与「未开立」分得开。CNY 账上两笔回汇批次：
# 一笔停在已归集，一笔交出汇付主张（主张不是付款，账上位置不变）。SGD 账仍无批次，
# 分户账页那一格继续显「未配置」。批次是演示租户的合成实例，与本段其余 SYN- 行同一类。
#
# 已灌过的库上重放本节会在记账处以 UNDERFUNDED（退出码 4）中止：写口的重放判定排在
# 余额守卫之后，来源位置被原记账清空后守卫先答余额不足，轮不到「已在册」（实测于
# d340014 对演示库重放）。非干净库复灌一律走 --reset，别指望本节重放幂等。
"$BIN/parcel-collection-register" subledger -input "$SEEDS/collection/01-subledger-account-01-cny.json"
"$BIN/parcel-collection-register" subledger -input "$SEEDS/collection/02-subledger-account-01-sgd.json"
"$BIN/parcel-collection-register" instruction -input "$SEEDS/collection/03-instruction-parcel-01.json"
"$BIN/parcel-collection-register" fact -input "$SEEDS/collection/04-fact-channel-report.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/05-posting-intake-in-transit.json"
"$BIN/parcel-collection-register" fact -input "$SEEDS/collection/06-fact-bank-credit.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/07-posting-settle-awaiting-allocation.json"
"$BIN/parcel-collection-register" discrepancy -input "$SEEDS/collection/08-discrepancy-shortfall.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/09-posting-shortfall.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/10-posting-allocation-payable.json"
"$BIN/parcel-collection-register" batch -input "$SEEDS/collection/11-batch-cny-open.json"
"$BIN/parcel-collection-register" batch -input "$SEEDS/collection/12-batch-cny-handed.json"
"$BIN/parcel-collection-register" hand-over -input "$SEEDS/collection/13-hand-over-cny-handed.json"
