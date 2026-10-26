# 88 SA 核销与核销撤销的事务里不重算入账余额与已确认未结

Category: bug
Status: ready-for-agent——R2（上线后尽快）：code/46 起核销与核销撤销经受控 CLI 进得来，指向客户对账单的核销分配却不在自己的事务里重算该作用域的入账余额与已确认未结，要到同作用域下一次映射才进重算；可用余额不受影响（两列等额移动），以入账余额为基数的比例额度与读面上的两列在这期间停在旧值。另有一格超额对账单分配会让此后同作用域的映射重算失败、答未决，续办是撤销那笔核销。看得见的卡住、不答错可用余额。2026-10-25 票 code/46 落地时立（用户「你作为业务和系统专家自决」授权），分支 claude-code12。
Blocked by: 无（「要裁的」第一条若裁成要新 ADR，先停在那里）
出处：票 [code/46](./46-settlement-application-and-reversal-have-no-process-entry.md) 落地时读码，取证于分支 claude-code12（基 main `d19eeaafc`）。

## 现象

- SA CONTEXT「运营余额、收付款与坏账」：「入账余额在形成它的那次 `UC-SA-005` 事务里，按该作用域已采用并已映射的客户方向真实收付净额减去指向该作用域客户应收的核销分配净额重算并推进……已核销分配等额离开已确认未结，所以可用余额在核销前后不变。」[ADR-0181](../../../docs/adr/0181-posted-balance-is-recomputed-in-the-funds-mapping-transaction.md) 决定一：「`UC-SA-005` 在采用并映射、核销、核销撤销各自的事务里，按下面的求法重算该作用域的入账余额与已确认未结」。
- 代码：`application.MapExternalFundsHandler` 的 `Map` 在落映射之后调 `advanceCustomerReceipt`；`Apply` 与 `Reverse` 不调。`MapExternalFundsDeps` 头注写着「核销与确认费用不从这里挂」，与 ADR-0181 决定六「首切只挂决定一在客户收款映射及其撤销上的那一支」一致。`CustomerReceiptPostings.CustomerDirectionNets` 的 W 只加总尚未撤销、指向本作用域对账单的分配，所以下一次映射重算时那笔核销才进来。
- 对账单、审核应付、贷项三格没有按目标的未核销上限（code/12 只给客户赔付、追偿认可两格加了）。分给对账单的分配超出该作用域已确认应收时，W 大于已确认应收，此后同作用域每一次映射重算在 `domain.PostedAndUnsettled` 求出负的已确认未结（`ErrNegativeConfirmedUnsettled`），`Map` 整笔返 error，CLI `execute` 归未决 3。同一函数里账户目标收不成应收作用域的 `domain.ErrNotReceivableAccount` 也这样落到未决 3。两格同一份输入重投永远同一个结果，按 ADR-0029 不该答未决。
- `CustomerDirectionNets` 用读执行器、不锁作用域：同作用域两笔并发的映射（以及将来挂上的核销）各按自己的快照求 P、W，后提交的那笔以更晚的截至时点盖掉前一笔的贡献，要等下一次重算才对。

## 三类

机制。无产品策略；无租户取值。

## 规模

M：`Apply`、`Reverse` 在落行之后按指向对账单的分配所在作用域调同一个重算函数（ADR-0181 决定三「函数只有一个」）；超出已确认应收的对账单分配在落行之前判、答业务负向格；重算函数的两个确定性失败不再进未决；作用域锁与写同一事务。

## 最小的诚实修法

照 `advanceCustomerReceipt` 那条路：核销与撤销事务里按分配收出对账单的应收作用域、去重、逐个重算推进；分配会让已确认未结为负的那一笔不落行，答 `APPLICATION_IMBALANCE`；`execute` 的哨兵表给 `ErrNegativeConfirmedUnsettled`、`ErrNotReceivableAccount` 各一行归用法格，或者编排在落行之前判、不返 error。

## 要裁的

- 把核销与撤销加进重算调用点，是落 ADR-0181 决定一，还是扩成它的决定六（及 ADR-0182 对决定六的收窄）——后者算部分停用，要新 ADR。
- 超出已确认应收的对账单分配答哪一格、在落行之前判还是落行之后回滚；要不要给对账单也加按目标的未核销上限（以对账单总额为本体）。
- 同作用域重算的锁：锁 `operational_balance` 行，还是按作用域取咨询锁。
