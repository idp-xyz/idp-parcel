echo "== 代收与清分登记（collection-remittance：分户账、指令、事实与记账） =="
# 一条 COD 指令走全程（票 admin-remainder-mechanism-batch/04）：渠道报收全额入账
# `渠道在途`；银行实际到账少于渠道报收，到账部分凭该到账事实搬进`待清分`；差额里经
# settlement-accounting 认可的协议抵扣（`agreed-offset` 登记认可引用、`posting` 凭它
# 落账，到账先移位才落得了）离开`渠道在途`落进`已抵扣`（ADR-0298），余下的先登短款
# 差异事项、再凭它落进`短款`——差额不自动落账；到账部分凭 ALLOCATION（指向代收指令）清分进`应付客户`，
# 进`应付客户`不得凭代收事实（未实际收到的代收款不得进入应付客户）。账上各位置之和
# 恒等于入账 150000：已抵扣 6000、短款 4000、应付客户 140000。SGD 分户账只开立不记账，
# 让「已开立但当期无记账」与「未开立」分得开。CNY 账上两笔回汇批次：一笔停在已归集，
# 一笔交出汇付主张（主张不是付款，账上位置不变）。SGD 账仍无批次，分户账页那一格继续
# 显「未配置」。批次是演示租户的合成实例，与本段其余 SYN- 行同一类。
#
# 银行到账（06）的证据引用 SYN-COD-FUNDS-BANK-01 是结算已采用、已整条认作代收本金的外部资金事实，由前一节
# 340-settlement-collected-principal-funds 先采用并认作（ADR-0274、ADR-0330）；到账同额 140000，登记时按引用读回
# 它此刻的链头，核币种、核各条到账之和不超过它。
#
# 已灌过的库上原样重放本节，每一行答已在册（EXISTING / ALREADY_HANDED_OVER，退出码 0）：
# 重放判定先于余额守卫。换了记账标识再跑的不是重放：凭代收事实的那两笔（入账、到账移位）
# 与凭认可抵扣的那一笔答内容冲突（退出码 2，一条事实、一条认可抵扣至多支撑一笔记账），
# 凭差异事项或清分的那两笔来源位置已被原记账移空，答余额不足（退出码 4）。
"$BIN/parcel-collection-register" subledger -input "$SEEDS/collection/01-subledger-account-01-cny.json"
"$BIN/parcel-collection-register" subledger -input "$SEEDS/collection/02-subledger-account-01-sgd.json"
"$BIN/parcel-collection-register" instruction -input "$SEEDS/collection/03-instruction-parcel-01.json"
"$BIN/parcel-collection-register" fact -input "$SEEDS/collection/04-fact-channel-report.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/05-posting-intake-in-transit.json"
"$BIN/parcel-collection-register" fact -input "$SEEDS/collection/06-fact-bank-credit.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/07-posting-settle-awaiting-allocation.json"
"$BIN/parcel-collection-register" agreed-offset -input "$SEEDS/collection/14-agreed-offset.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/15-posting-agreed-offset.json"
"$BIN/parcel-collection-register" discrepancy -input "$SEEDS/collection/08-discrepancy-shortfall.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/09-posting-shortfall.json"
"$BIN/parcel-collection-register" posting -input "$SEEDS/collection/10-posting-allocation-payable.json"
"$BIN/parcel-collection-register" batch -input "$SEEDS/collection/11-batch-cny-open.json"
"$BIN/parcel-collection-register" batch -input "$SEEDS/collection/12-batch-cny-handed.json"
"$BIN/parcel-collection-register" hand-over -input "$SEEDS/collection/13-hand-over-cny-handed.json"
