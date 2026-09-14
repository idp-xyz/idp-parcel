# CC 入向登记没有版本维：SA 合法的更正版本到 CC 侧落成「内容冲突」，只留 inbox 痕，到不了 UC-CC-009 的重新核对

Category: enhancement
Status: in-progress——2026-09-14 13:2x 通道 1 自办（用户「请你自决」两次；无在线通道可派；`/implement` › `/tdd`；分支 `mcp1-sacc13` 基远端 main `ace35ac3`（sa-cc/12 刚进 main，`0020` 已在），树 `D:/tops/idp-parcel-mcp1-sacc13`；迁移序号钉 `customs_compliance/0021`；形取**版本子表**——`duty_payment_verification` 外键钉在 `external_funds_fact (tenant_id, fact_ref)` 上，主键加版本会拆掉它，子表让事实身份行不动、版本各占一行）；此前 ready-for-agent——2026-09-14 10:2x 通道 1 按用户 10:1x「授权代裁」（CC owner 口径）裁「要裁的」1 / 2：**要版本维**（一版本一行，形由作者定）；**触发重核对不在本票**，本票只到「登记册看得见新版本与回指」，触发归核对那一族另立票；全文见文末「裁决」。「做法」与「完成判据」按裁决写实，取证锚仍是票面的 `9ddbafcf`，作者开工先在 main 重量。此前 draft——2026-09-10 22:0x 通道 4 立票（按通道 1 派单 task-d00c5556；sa-cc/03 非作者评审 Spec 非阻断 ① 的后继）。只写票面未动代码；取证锚 main `9ddbafcf`
Blocked by: 无（[03](03-cc-inbox-consumer-receives-external-funds-fact.md) 已进 main；本票要裁的归 CC owner；与 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 并列、互不阻塞）

## 缺口（取证于 `9ddbafcf`，逐符号名）

- SA 侧：更正 / 撤销是**同一事实的新版本**——`domain.ExternalFundsFact` 的 `Version()` / `Corrects()`，`CorrectAmount` 结构拷贝出新版本并回指前版；[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) 裁决 2 定更正复用同一事件类型 `settlement-accounting.external-funds-fact.adopted` 再发一封，ID `<租户>/funds-fact/<事实>/<版本>`，载荷带可缺席的 `corrects`。
- CC 侧：`ports.ExternalFundsFactRegistration` 只有 Fact / Source / Payer / Currency / AmountMinor / OccurredAt，**无版本、无回指**；`customs_compliance/0016` 的 `external_funds_fact` 主键 `(tenant_id, fact_ref)`——一事实一行。`ReceiveFundsFact` 按引用幂等：同引用同内容 → `FundsFactExisting`，同引用换内容 → `FundsFactContentConflict`。
- 于是 v2 更正版（同 `fact_ref`，金额或业务时间变了）到 CC：`ccinbox.ExternalFundsFactConsumer` 头注写「对首版与更正版一视同仁地交给处理方，重新核对是 UC-CC-009 的事，不在这里分路」；`ReceiveOnAdoptedFundsFactAdapter` 回查那一版、译六格交编排 → 编排答 `FundsFactContentConflict` → 消费门入账、不重投（`receiveConsumption`）。`receive_on_adopted_funds_fact_test.go` 的 `TestAnAdoptedFundsFactIsRegisteredOnceAndReplayOrConflictStillSettles` 第三段把「v2 更正到达」明确建模为内容冲突、CC 登记保留 v1 金额——这是 03 完成判据 2 与做法 3 的字面，代码没有错，是登记册的形装不下更正。
- 结果：SA 合法更正与真冲突（同引用两个来源各说一套）在 CC **不可区分**；更正在 CC 侧只留 inbox 已处理的痕迹，`VerifyPayment` 的调用方看不到有新版本，无处触发「新核对版本」。消费者头注那句「重新核对是 UC-CC-009 的事」在数据路径上到不了 UC-CC-009。
- 与 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 同根异题：12 是登记的一格（付款人）能不能缺席，本票是登记的键有没有版本维。03 评审判断题 ② ⑤ 点出「同根」：CC 读端口 `AdoptedFundsFact` 不带 Version / Corrects、消费者不译 `corrects`，都因为登记册没有那一维可放——不是在读端口上加字段能解，要登记册先有版本维。

## 语言从哪里来

- CC `CONTEXT.md`「税费付款核对」：「部分付款、超额付款、错误范围、错误币种、重复付款、资金退回和付款撤销都必须保留原事实并形成新的核对判断」；集成规则：「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态」。
- UC-CC-009 一致性节：「外部资金事实迟到、更正、资金退回或付款撤销时，形成新核对版本并保留原覆盖判断；不删除原付款、不按最后到达覆盖」；「保留全部版本和原事实」。
- 两句合起来要的是：更正版本**进得来**（原事实与新版本都保留）且**能成为重新核对的来源**。今天的 `内容冲突` 两样都没做到——原事实保住了，新版本被当作矛盾丢在 inbox。

## 做法（待裁后写实）

1. CC 入向登记加版本维：`ExternalFundsFactRegistration` 加 `Version`（与信封 / SA `FundsFactVersion` 同字面）与可缺席的 `Corrects`；`external_funds_fact` 改为一版本一行（主键加版本，或另起版本子表——形由作者定），新迁移序号重取，`0016` 不改；`ReceiveFundsFact` 幂等键随之变为（引用 + 版本）：同版本同内容 → `已存在`，同版本换内容 → `内容冲突`（真冲突），**新版本 → 新一行、回指前版**。
2. `ccports.AdoptedFundsFact` 读端口带出 Version / Corrects（03 评审 ② 所说「要用时再加」的那一刻到了）；`ReceiveOnAdoptedFundsFactAdapter` 把信封所指版本与回查到的回指译进登记；消费者仍只译不判。
3. 「新版本到达 → 重新核对」的触发落点：`VerifyPayment` 的三轴与关联依据由调用方交、调用方今天不在本进程（03 票面红线原句）。本票至少保证登记册上**看得见**新版本与回指（`LoadFundsFact` 或新读口按引用列全部版本）；触发重核对的编排是否随本票立，见「要裁的」2。
4. 03 判断题 ③ 的保留（`未受理` 在 CC 侧无落痕）不在本票，归 12。

## 红线

- 不改 SA 任何东西：版本与回指的权威在 SA，CC 只登引用 + 核对所需维度（03 票面红线）；不把 SA 的版本链复制成 CC 的第二份。
- 不按到达顺序覆盖：新版本追加，旧版本一行不动（UC-CC-009「不按最后到达覆盖」）。
- 不由消费者判「这是更正还是冲突」：消费者只译，判在编排（03 做法 3 原句）。
- 真实付款条件 / 关联规则属实例半边，一行不预填。

## 完成判据（待裁后写实）

1. 应用层：v1 已登记，v2 同引用、回指 v1、金额变 → `已接收`（新一行），按引用列出两版且 v2 回指 v1；同版本重投 → `已存在`；同版本换内容 → `内容冲突`。
2. 真库：新迁移往返；`0016` 一字未动；`cmd/parcel-dispatch` 真库装配用例的正例扩一格（v2 到达落第二行）。
3. 与 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 的交叉：付款人可缺席与版本维互不依赖，任一先落另一不重做（各改各的列）。
4. 消费者头注「重新核对是 UC-CC-009 的事」那句改为指向实际落点。

## 地盘

`internal/customscompliance/{ports,application,adapters/postgres,adapters/settlementaccounting,adapters/inbox}`、`migrations/customs_compliance/`（新序号）、`cmd/parcel-dispatch/assemble_test.go` 正例一格（共享文件，动前占号）。SA 侧不动。

## 要裁的

1. **CC 登记要不要版本维**——归 CC owner。不要的话，更正版本在 CC 的正确归宿是什么：a) 照旧 `内容冲突` 入账、靠运营从 SA 读面比对（等于承认 UC-CC-009「形成新核对版本」在 CC 侧无入口）；b) 消费者对带 `corrects` 的信封分路（违 03 做法 3「消费者只译不判」）。要的话，形取「主键加版本」还是「版本子表」由作者定，本票不裁形。
2. **（已裁，见「裁决」）** 范围题：新版本到达要不要在本票内触发重新核对（一个 CC 内部的「资金事实新版本 → 待重核对」编排），还是本票只到「登记册看得见新版本」、触发归核对那一族（[05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 邻接）——归 CC owner；裁前按后者写完成判据。

## 裁决（2026-09-14 10:2x，通道 1 推送方按用户「授权代裁」以 CC owner 口径裁）

1. **要版本维。** CC CONTEXT「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实」与 UC-CC-009「保留全部版本和原事实」两句要的是更正版本**进得来、原版本留得住**，今天的 `内容冲突` 两样都做不到——这是登记册的形装不下语言，不是消费者或读端口的事（03 评审 ② ⑤ 已点出同根）。**一版本一行**：主键加版本还是版本子表由作者定，判据是「同引用列全部版本、每版回指前版」在 SQL 上一问答得出、`0016` 不改；幂等键随之为（引用 + 版本）：同版本同内容 → `已存在`，同版本换内容 → `内容冲突`（真冲突——同一版本两个来源各说一套），新版本 → 新一行回指前版（做法 1 原句）。版本与回指的字面与 SA `FundsFactVersion` / 信封 `corrects` 同，CC 只登引用不复制 SA 的版本链（票面红线）。
2. **触发重核对不在本票。** 本票只到「登记册看得见新版本与回指」：`LoadFundsFact` 或新读口按引用列全部版本，`ccports.AdoptedFundsFact` 带出 Version / Corrects，消费者仍只译不判（做法 2 / 3 前半）。「新版本到达 → 形成新核对版本」的编排归核对那一族——`VerifyPayment` 的三轴与关联依据由调用方交、调用方今天不在本进程（03 票面红线），把触发塞进本票就是在登记票里长出第二只核对编排。**作者在完成记录里写清那张后继票该长什么样**（触发落点、要读哪几口、与 [05](05-cc-duty-reconciliation-hands-off-to-settlement-accounting.md) 的关系），推送方据此立票，本票不立不做。
3. **与 [12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md) 的交叉**照票面完成判据 3：互不依赖、各改各的列；两票若同期在途，迁移序号各自重取、动前占号。
4. **不改的**：不改 SA；不按到达顺序覆盖；不由消费者判更正还是冲突；真实付款条件 / 关联规则一行不预填（票面红线）。完成判据 4（消费者头注那句改指实际落点）照旧。

## 参照

[03](03-cc-inbox-consumer-receives-external-funds-fact.md)（评审 Spec ①、判断题 ② ⑤、进 main 记录「候选后继」）；[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) 裁决 2；[12](12-cc-funds-fact-payer-may-be-explicitly-unprovided.md)（同根异题）；`internal/customscompliance/application/reconcile_duty_payment.go`（`ReceiveFundsFact`）；`internal/customscompliance/ports/ports.go`（`ExternalFundsFactRegistration` / `AdoptedFundsFact` / `ExternalFundsFactRegister`）；`internal/customscompliance/adapters/inbox/external_funds_fact_consumer.go` 头注；`internal/customscompliance/adapters/settlementaccounting/receive_on_adopted_funds_fact.go`；`internal/settlementaccounting/domain/external_funds.go`（`Version` / `Corrects` / `CorrectAmount`）；`migrations/customs_compliance/0016_duty_payment_reconciliation.sql`；CC `CONTEXT.md` 税费付款核对与集成规则两句；UC-CC-009 一致性节。

## Comments

- 2026-09-10 22:0x · 通道 4（task-d00c5556，取证锚 main `9ddbafcf`）：立票。**只写票面，未动代码。** 能力边界：读过 03 全文（含评审与进 main 记录）、`reconcile_duty_payment.go` 的 `ReceiveFundsFact`、`ports.go` 的三个类型、`ccinbox` 消费者头注、`receive_on_adopted_funds_fact.go` 头注与其测试第三段、`0016` 表结构、SA `external_funds.go` 的版本 / 回指访问器、CC CONTEXT 与 UC-CC-009 引到的几句；**没读** SA `AdoptedFundsFactView` 的 SQL 与 05 票面全文——「要裁的」2 把触发落点归到 05 邻接是按 spec 子票表那一行推的，作者开工时核。本票不复述 12 的付款人题，只在「缺口」末段写两者为何同根。
