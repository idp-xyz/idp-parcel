# ADR-0181：入账余额在形成它的那次收付映射事务里按本上下文的账重算

Status: Accepted（2026-10-14，用户经 IDP 队列授权通道 1 自决，原话「你作为业务和系统专家，0181可以接受吗，你自决」。接受方的读取边界与逐条理由见文末「接受记录」。起草：2026-10-14，通道 2 随票 [product-strategy-boundary/29](../../.scratch/product-strategy-boundary/issues/29-operational-balance-and-credit-standing-have-no-production-writer.md) 分诊，派单 `task-4aaee735-bb59-4042-8ec2-06755e8087cd` ← 通道 1。起草方裁决能力边界：读过[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「切片的机制半边与实例半边」至 2026-10-13 补记、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一与决定二、[ADR-0007](./0007-separate-operational-settlement-from-statutory-finance.md) 的 Decision、[ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md) 的 Decision、[ADR-0127](./0127-credit-basis-is-handed-over-by-the-commercial-closure-resolution.md) 决定四、决定五与「登记额度与政策额度比对」那条否决、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一、SA `CONTEXT`「运营余额、收付款与坏账」、[UC-SA-005](../application/settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md) 的目的、步骤 5 与结果「运营余额已更新」、`migrations/settlement_accounting/0010_operational_position.sql` 头注、`BalancePosting` 头注与 `RecordBalance` 只准往前、`OperationalBalance.Available` 与 `Freeze` 用可用余额比较、`freezeFunds` / `exposeCredit`（额度取信用依据，无信用状况行即不可用）、演示种子 `SYN-FIN-CONTROL-01` 的两项控制与 `ALL_CONTROLS_PASS`。没读：UC-SA-002 全文、UC-SA-005 验收表全文、ADR-0129 全文、`MapExternalFundsHandler` 与确认费用编排的实现、通道 4 的 `mcp5-psb26`、合成演示动线全文、`bento-gate-reeval/01`。到期日只在 CONTEXT、GLOSSARY、SCENARIOS 与 UC-SA-005 的词里出现；`internal/settlementaccounting`、`internal/partycommercial` 与 `migrations/` 没有 `due_date` 字段。

Date: 2026-10-14

## Context

接受前预付冻结读运营余额。作用域没有登记行时，读取答未登记，编排折成`余额不可用`并保持未决。`RecordBalance` 只存在于适配器，生产路径没有调用者。于是任何租户的预付冻结都停在这一格。

SA `CONTEXT` 已经把话说过：外部系统拥有真实收付款；本上下文只有 `UC-SA-005` 可以在外部事实确认后形成真实收付映射、相应运营余额和核销关系。可用余额等于入账余额加当前有效授信额度，再扣除冻结金额和已确认未结应收。`UC-SA-005` 的结果「运营余额已更新」写明它不等于银行余额。ADR-0007 写明双方不共同维护一个可覆盖余额。

这些原话没有回答三件难逆转的事：入账余额按哪几本账求、写入方在映射事务里还是在事后的投影里、信用状况那一行在算不出逾期时能不能先落一行。票 29 的甲、乙、丙因此还没有已接受的裁定。

演示合同 `SYN-FIN-CONTROL-01` 第 1 项是预付冻结，第 2 项是信用检查，共同通过条件是全部通过。本记录把冻结那一支与信用状况分开裁，是因为后者今天没有到期日可读。

## Decision

**一、写入方取甲：在拥有那次变化的用例事务里重算并推进，不取乙，不取丙。** `UC-SA-005` 在采用并映射、核销、核销撤销各自的事务里，按下面的求法重算该作用域的入账余额与已确认未结，并推进 `operational_balance`。与事实同一笔落库。迟到的快照仍按现有「同键只准往前」交回未推进，不盖新值。

乙（异步投影）否决。冻结在估价形成时读余额。投影落后于已采用事实时，产品没有一格可答「账已采用、余额还没跟上」，硬读旧值就是拿过期的钱做冻结。

丙（把外部财务系统交来的余额快照当成入账余额采用）否决。那份快照是银行或法定账上的数。CONTEXT 只允许 `UC-SA-005` 在外部事实确认之后形成运营余额，并禁止用到账通知冒充运营余额。ADR-0007 禁止两边共同维护一个可覆盖余额。外部快照也答不出下面这几本账各自进哪一项。

**二、入账余额只由两本账求出。** 作用域是租户、责任法人、结算账户、币种。

- 已采用并已映射的客户方向真实收付：收款进入，退回与撤销按反向映射离开。净额记为 P。COD 代收本金不进，沿 UC-SA-005 既有禁则。
- 已核销分配：指向该作用域客户应收的分配进入，核销撤销离开。净额记为 W。
- 入账余额 = P − W。未分配不是第三本要再加一次的账：P − W 剩下的就是尚未核销分配的已映射收付。
- 已核销分配同时等额离开已确认未结。可用余额在核销前后不变，同一笔钱不会既留在入账里又从应收里拿掉。

已确认未结 = 当前有效的已确认客户运营应收 − 指向它们的核销分配净额。应收来自确认费用的那本账，不来自收付映射的总额。它不可为负；求出来为负是账不平，推进失败，不截成 0。

欠款体现在可用余额为负，不把「确认费用高于冻结」写进入账余额的负数。入账余额为负只表示已映射收付净额本身为负（退回大于收款）。冻结金额不进这一行，仍由冻结账本当场求和。

**三、已确认未结由这同一次重算求出，不由两处各推增量。** 调用点有两处，函数只有一个：确认费用（及会改当前有效客户应收的调整）的事务，以及决定一里 `UC-SA-005` 的事务。增量对推会漂，而同键只准往前，漂了不能用更晚的时点把错数盖回去。

**四、这一行的授信额度列不由本写入方填写成某个政策额度。** `operational_balance` 的可用余额会加上 `credit_minor`。把信用依据上的额度抄进这一列，预付冻结就会占用账期额度，与 ADR-0047「两本账互不借用」相冲突。插入新行时该列写 0，含义是这一行不承载授信额度，不是「该租户的授信是 0」。之后的推进只改入账余额、已确认未结和截至时点，不覆盖该列。额度仍只从信用依据取，沿 ADR-0127。

**五、信用状况行在算得出逾期之前不写。无行仍是信用状况不可用，不读成未逾期。** 逾期的判断方法：截至时点，该作用域仍有未核销金额、且该项到期日早于截至时点的已确认客户应收，则为逾期。不设宽限日。到期日与未核销金额是租户取值；「过期仍未核销即逾期」是判断方法。

到期日今天没有字段可存。在它有登记处之前，写入方不得插入 `credit_standing`。插一行并把逾期写成假，就是在没有账龄的时候回答未逾期。`limit_minor` 列保留，不删除；它不是额度依据。将来真写这一行时，该列只为满足非空而写 0，`exposeCredit` 继续用信用依据替换额度。

**六、首切只挂决定一在客户收款映射及其撤销上的那一支。** 演示动线越过预付冻结，要的是这一支：经既有登记面采用一笔外部收款事实并映射，同一事务推进入账余额，金额由那几本账求出，不直写表。演示租户走同一条路径。已确认未结的确认费用调用点、信用状况与逾期另拆，不并进这一支。`SYN-FIN-CONTROL-01` 要两项都通过，所以这一支只让冻结那一半有数；信用检查仍会停在信用状况不可用。

## 候选与反方

- **乙，映射之后异步投影。** 否决，见决定一。滞后要新的回答格，而冻结今天只认识有余额、余额不足、余额不可用。
- **丙，采用外部余额快照。** 否决，见决定一。
- **两处各把已确认未结加一笔或减一笔。** 否决，见决定三。
- **无信用状况行时按未逾期放行。** 否决，见决定五。那是改 `exposeCredit` 的判断语义。
- **把政策额度抄进 `operational_balance.credit_minor`，让预付可用余额含授信。** 否决，见决定四。

## Consequences

- 本记录已是依据。决定二的求法，以及决定四「这一行的授信额度列不承载当前有效授信额度」，已补进 SA `CONTEXT`「运营余额、收付款与坏账」。补的是入账余额怎样从账上算出来，以及这一行不把政策额度抄进可用余额。
- 实现不在本记录里：写端口、重算函数、两处调用、装配。读口仍是今天的 `LoadBalance`。从未推进过的作用域仍答余额不可用。
- 首切的实现只做决定六那一支。确认费用调用点、信用状况写入方各是后续票，本记录不立那些票。
- 以入账余额为比例基数的额度，会在这一行开始有生产写入之后从「尚无事实」变成有数。ADR-0129 的正文本记录没有改。

## 越权风险点

1. 首切不挂确认费用时，已有应收的账户在下一次映射之前，已确认未结停在旧值，可用余额偏大。演示账户在第一次冻结前没有已确认应收。归 settlement-accounting 的 owner。
2. 新行的授信额度列写 0，可能被读成租户取值。决定四写的是这一列不承载额度。归 settlement-accounting 的 owner。
3. 逾期在没有新事实、只是过了到期日时如何翻开，本记录没有裁。决定五把写行推迟到到期日有处可读，不另加定时扫描。归 settlement-accounting 的 owner。
4. 未读 ADR-0129 全文。接受前应确认：入账余额有了生产写入之后，比例额度的基数从「尚无事实」变成有数，是那条记录已经要的行为。归 settlement-accounting 的 owner。 **接受时关闭**（通道 1，理由见「接受记录」）
5. 未读映射编排的实现。决定钉在用例事务上。归实现票，不在本记录里指定函数签名。 **接受时维持**（通道 1 只核对了 `MapExternalFundsHandler` 的 `Map` / `Apply` / `Reverse` 是各自的入口，没有读编排正文）

## 接受记录（2026-10-14 通道 1）

用户经 IDP 队列授权通道 1 自决，原话「你作为业务和系统专家，0181可以接受吗，你自决」。接受方读过：本记录全文；SA `CONTEXT`「运营余额、收付款与坏账」；UC-SA-005 的目的禁则、步骤 5 与结果「运营余额已更新」；ADR-0129 决定二、决定三与「以可用余额作基数」「负入账余额停待判断」两条否决；ADR-0047 决定里「两本账互不借用」；`OperationalBalance.Available`；`MapExternalFundsHandler` 的入口划分（`Map`、`Apply`、`Reverse` 各是一次入口）。起草方列为没读的 UC-SA-002 全文、UC-SA-005 验收表全文、映射编排正文、`mcp5-psb26`、合成演示动线全文，接受方同样没读。

越权风险点逐条：

1. 维持。首切不挂确认费用，已有应收的账户在下一次映射之前已确认未结会停在旧值。演示账户在第一次冻结前没有已确认应收，所以这一支够解开冻结；有历史应收的账户不在首切里假装已结清。
2. 维持。新行授信额度列写 0 的含义已写进 SA `CONTEXT`：这一列不承载当前有效授信额度。读口若把 0 当成「该租户授信是 0」，是实现票要守的，不在本记录改 `Available`。
3. 维持。没有到期日字段之前不插信用状况行；过期翻开不在本记录里加定时扫描。
4. 关闭。ADR-0129 决定二写明 `POSTED_BALANCE` 尚无事实就是该作用域未登记运营余额（无行）；有行之后 `posted_minor` 就是基数。负入账余额如实交负数，折成 0 是领域的事，与决定二「入账余额为负只表示已映射收付净额本身为负」一致。生产写入让基数从「尚无事实」变成有数，是 0129 已经要的行为。
5. 维持。决定钉在用例事务上。`Map`、`Apply`、`Reverse` 已是各自的入口，实现票把重算放进调用方已经打开的那次事务即可，本记录不指定函数签名。

接受时的改动（起草方的六条决定不改）：

- SA `CONTEXT`「运营余额、收付款与坏账」补入账余额的求法，以及登记行上的授信额度列不承载额度。
- 索引一行从草案改为已接受。
- 票 product-strategy-boundary/29 从 needs-info 转为 ready-for-agent，范围收在决定六：只挂客户收款映射及其撤销。核销、确认费用调用点、信用状况另拆，不并进下一张实现票。

## Links

- [ADR-0007：分离物流运营结算与法定财务账](./0007-separate-operational-settlement-from-statutory-finance.md)
- [ADR-0047：账期方式的接受前控制形成信用暴露，不冒充资金冻结](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)
- [ADR-0127：信用依据随商业闭包解析交出](./0127-credit-basis-is-handed-over-by-the-commercial-closure-resolution.md)
- [ADR-0146：产品策略是机制与实例之间的第三类](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [ADR-0150：合成租户按真实租户对待](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)
- [SA CONTEXT](../domain/settlement-accounting/CONTEXT.md)「运营余额、收付款与坏账」
- [UC-SA-005](../application/settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md)
- 票 [product-strategy-boundary/29](../../.scratch/product-strategy-boundary/issues/29-operational-balance-and-credit-standing-have-no-production-writer.md)
