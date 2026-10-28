# ADR-0336：七项确认事实随预估客户费用同笔落下；「有效交付已终局」是内置的确认依据，依据到了而这一户登了确认时点就当场确认

Status: Accepted（2026-10-28，Claude Code 会话代 SA owner 决（代决）：用户 2026-10-27 对会话提出的三件决定答「按你的建议开始吧」；票 product-strategy-boundary/22 第 1 笔，起因 psb/05「预走」停点 1。越权风险点待 SA owner 复核。）<!-- 索引注记 -->｜**入口**：确认编排已装在 parcel-api，却永远走不到——七项确认事实只能按笔经受控登记口登，确认依据在生产上没有任何写入方。**取法**：形成预估客户费用时同笔从 SELL 请求与它指名的账户登七项事实；PS 终局信封与费用形成两头都给费用记「有效交付已终局」依据，这一户登了确认时点就当场确认；确认条件目录开受控登记口。
Date: 2026-10-28

## Context

取证于 main `c2a771cbb`，只记 `S`。

- 确认编排 `ConfirmChargeHandler` 依次要三样东西：确认条件目录说这个费用项目要哪种依据；依据登记说这笔费用到了那种依据；确认事实册上这笔费用的七项事实（责任法人、相对方、方向、结算账户、合同依据、主要计费范围、来源事实）。少一样就停在未决或条件未满足。
- 确认事实册只有受控登记口 `charge-confirmation-facts` 能写，一次一笔。SELL 形成预估客户费用时不写它。生产上让人按笔登七项事实不可行。
- 依据登记的写口 `RecordBasis` 在生产上没有任何调用方；确认条件目录的写口 `RegisterCondition` 也没有登记口。
- 2026-10-27 预走（psb/05「预走」）里包裹已走到 `EFFECTIVE_DELIVERY`，客户费用停在`预估`。
- ADR-0170 只定了确认先问时点册、没登记不做；谁带着什么依据来调确认没有定。

## Decision

**一、七项确认事实随预估客户费用同笔落下。** 包裹计费时点那一路（ADR-0190）形成预估客户费用之后、在请求册记结论之前，同一笔里把七项事实登上确认事实册：结算账户、主要计费范围（取 SELL 请求的计价范围）、来源事实取 SELL 请求；责任法人、相对方、方向取请求所指那一户的固定属性；合同依据取那一户的成立依据。都是请求提交时已经钉下的东西，不是 SA CONTEXT 禁的「按当前组织或客户属性临时推断」。册上已有同一笔的另一份内容（受控登记口先登过）不覆盖，确认照读册上那一行。请求指名的账户不在册上是两侧说法矛盾，整笔不作答、重投。受控登记口 `charge-confirmation-facts` 保留，给别的形成路径与补登用。

**二、「有效交付已终局」是产品内置的确认依据种类，依据从两头来。**

1. 种类字面 `EFFECTIVE_DELIVERY_FINALIZED`，与包裹计费时点同词，指同一件事：小包托运把包裹采用为终局「有效交付」。依据引用是那一份终局采用键，与 SELL 来源事实同形（`final-outcome/<包裹>/<种类>/<版本>`）。
2. PS 终局信封到达时，SA 另开一路消费（`settlement-accounting/record-confirmation-basis-on-final-outcome`，与发起 SELL 那一路分名）：按四维回读终局，只认已终局的有效交付，给这件包裹当前那份 SELL 请求已形成的费用记依据。费用还没形成就什么都不记。
3. 费用在终局之后才形成（例如按终局计费）的，形成那一笔回读 PS 当前终局，已终局为有效交付就同笔补记依据。
4. 两头读对方之前都先按（租户、包裹）取事务级咨询锁（`ParcelConfirmationSerializer`），锁到各自事务结束：一头正在形成或正在记依据时，另一头等它提交再读。
5. 依据记下之后问确认时点：这一户登了 `CONFIRM` 就当场调确认编排，确认信封随同一笔交出；没登就只记依据。确认条件目录与确认事实没登是租户取值缺席，入账不重投；册读写不动、或费用已确认而确认信封没交出去才重投（ADR-0029）。形成那一头的跟进只把等依赖的几格标成未决；请求指名的账户不在册上这类矛盾原样上抛，按矛盾那一格处置。
6. 用哪种依据确认哪个费用项目仍是租户取值。确认条件目录开受控登记口 `parcel-settlement-register charge-confirmation-condition`：同费用项目登同一种答重放，登另一种答冲突、不覆盖；只收产品认得的依据种类。

**三、运营确认口。** 运营决定口点名一笔费用确认，走同一道时点门与同一只确认编排；用于补拍，例如登确认时点之前已到依据的费用。决定种类、端点与载荷随本票第 2 笔落，本记录只定它存在、走同一条路。

## 候选与反方

- **七项事实在确认那一刻从请求与账户现取。** 确认时账户可能已追加新一版，现取的就不是形成时那一份。否决：落在形成那一笔，确认读的是形成时钉下的。
- **只从终局信封那一头记依据。** 按终局计费的那件费用在终局之后才形成，终局信封到时它不在，依据永远记不上。否决，两头都记。
- **终局信封直接调确认，不经依据登记。** 确认条件「只有一条真实登记的依据才算满足」（`charge_confirmation_condition` 迁移头注）就成了空话，条件目录也无从按费用项目选依据。否决。
- **在 SA 另立一本「包裹终局观察册」，形成那一头读它而不读 PS。** 多一份 PS 事实的复本。否决，经消费方适配器读 PS 当前终局（ADR-0025）。

## Consequences

- SA：`FormEstimatedCustomerChargeHandler` 多一个必备依赖 `FollowUp`；新编排 `ConfirmChargeOnBasisHandler`（`OnParcel` / `OnCharge`）与 `FormedChargeConfirmationFollowUp`；新端口 `SettlementAccountByIDView`、`ConfirmationBasisRecorder`、`EffectiveDeliveryFinalView`、`ChargeConfirmationConditionRegister`；领域类型 `ConfirmationBasisKind`。
- `cmd/parcel-dispatch`：终局路由多一路，名次 `positionBilling + 10`；SELL 评价已记录那一路的形成编排接上跟进。
- `cmd/parcel-settlement-register`：新命令 `charge-confirmation-condition`。
- SA PostgreSQL 适配器多一只 `ParcelConfirmationLocks`（锁空间 `settlement_accounting.charge_confirmation.parcel`）。
- 无迁移：依据表、条件表、事实表、账户表都已在。
- 演示租户要走到`已确认`，还须登确认条件与 `CONFIRM` 时点，随本票第 4 笔的种子落。

## 越权风险点

1. **主要计费范围取 SELL 请求的计价范围。** 两者在 CONTEXT 里是不同的词；首发每户一个计价范围，读法相同。若某户的主要计费范围比计价范围细，就要另一种取法。
2. **只认有效交付。** 退运完成、服务终止等终局不是确认依据，那几类费用的确认依据要另立种类。
3. **两头排队靠包裹确认锁。** 终局那一头读费用、形成那一头读终局各在自己的事务里；两头都先按（租户、包裹）取事务级咨询锁再读对方，否则两边恰在对方提交之前读，这一件就两头都错过。锁让同一件包裹上的这两步串行，代价是同一件包裹的终局消费与 SELL 形成互等。
4. **依据记下之后终局被更正或作废，已确认的费用不回退。** 迟到的变化按 CONTEXT 走调整，不由本记录处理。
5. **确认事实册上异内容不覆盖、也不报。** 受控登记口先登了不同的七项，确认照它确认，跟进那一步不出声。

## Links

- [ADR-0170](./0170-settlement-orchestrations-enter-through-parcel-api.md)：决定二（确认先问时点册），本记录不改
- [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)：包裹计费时点与形成编排外层
- [ADR-0087](./0087-settlement-registers-carry-the-facts-their-hard-sentences-require-checking.md)：决定一（确认事实与确认依据分两格）
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)、[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)：「费用形成与证据」
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)：`AT-SA-217`–`220`
- 票 [product-strategy-boundary/22](../../.scratch/product-strategy-boundary/issues/22-settlement-orchestrations-assembly-and-triggers.md)
