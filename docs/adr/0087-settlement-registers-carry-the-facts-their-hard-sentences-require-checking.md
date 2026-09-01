# ADR-0087: 结算登记册补齐硬句所要求的可核对事实——确认费用固定八项、客户费用调整单列追加册、经营组成项带角色维

Status: Accepted（2026-09-01，用户经频道裁定三项；驱动票 settlement-register-context-gaps 01/02/03）
Date: 2026-09-01

## Context

`settlement-accounting` 的 `CONTEXT.md` 有三句写成**可核对要求**的硬句，而登记册今天都验不了它们
（三张诊断票各自取证，重新核于 `d11e0f0` 结论未变）：

- 「每条确认费用必须固定责任法人、结算相对方、收付方向、结算账户、合同或责任依据、结算币种、
  主要计费范围和来源事实……任何一项不能通过当前组织、当前客户属性或报表筛选临时推断」——
  `customer_charge` 八项里成列的只有结算币种，且 `CONFIRMED` 行不带其余任何一项也落得进册。
- 「确认后出现新事实、源事实更正或规则适用性变化时形成新版本或调整明细，不覆盖历史结果」——
  供应商侧有整条版本链，客户侧两条路都没有：无 `version`／`prior_version`，也没有调整明细册；
  调整今天唯一的落库形态是 `customer_statement.adjustment_lines`，于是因果是反的——一笔调整只有
  先进了某张对账单才存在，而 CONTEXT 恰恰写着「对账纳入和资金核销不能代替金额调整」。
- 「审核应付与贷项按各自借贷方向分别计入一次……不得把审核应付视为已净含贷项」——
  `operating_result.components` 的元素只有来源、方向、金额三键，没有角色维。

**第三句的空转程度此前被低估。** 诊断票留了「明认为写侧纪律」这条出路，前提是写口真有那道复验。
核于 `d11e0f0`：`domain.DeriveOperatingResult` 只校验组成非空、来源引用非空、方向落在封闭二向、
金额为正，`ResultComponent` 三个字段根本表达不出角色——**写口不可能复验它**。对照同一类型上真正
守着的那条纪律（毛利只由组成算出、类型上没有写毛利的字段），这一句两侧皆无守卫。

三张表当前都是 **0 行**，因此三条路都不涉及数据清洗；补的代价此刻处在历史最低点。

## Decision

**一、`customer_charge` 补齐确认时必须固定的事实，并由约束保证「确认」这一格名副其实。**
补责任法人、结算相对方、收付方向、结算账户、主要计费范围五列；「合同或责任依据」单独成列，
不与既有 `confirmation_basis` 合用——后者是**确认依据**（哪份依据让它可以确认），CONTEXT 要的是
**合同或责任依据**（这笔钱依据哪份合同该收付），两者不同物，合用会让其中一个永远说不出口。
「来源事实」同样单独成列，不与 `evaluation_ref` 合用：评价是依据，来源事实是评价的输入。

约束照既有 `customer_charge_confirmation_coupled` 的「同在或同缺」形状：`stage = 'CONFIRMED'` 的行
七项俱全，非确认行不作要求——CONTEXT 那句只约束确认费用，对预估与暂估行强加会让它们落不进册。

**收付方向自立封闭词表，不复用借贷方向。** 本模块已有 `recovery_adjustment_direction_closed`
用 `DEBIT`/`CREDIT`，但那是**借贷**方向；这里要的是**收付**方向（这笔钱是应收还是应付），
两者不是一回事。词表取 `RECEIVABLE`/`PAYABLE`，领域侧同步立封闭集，SQL 的 `CHECK` 镜像它。

**二、客户费用调整单列追加式登记册，不走版本链。** CONTEXT 给的两条出路里选调整明细这条，
理由是语义：供应商侧的计价纠错「是一个重述全额的新成本版本，不是带借/贷方向的差额调整」，
而客户侧的调整**带借贷方向**——照搬供应商侧的版本链形状等于把两种不同的东西塞进同一套表达。
`customer_statement.adjustment_lines` 随之退回它本来的角色：**纳入关系**，与 `subsequent_inclusion`
一致，不再是调整的唯一存身处。

**唯一创建用例这道门同笔落地。** CONTEXT「调整类型与唯一所有权」表规定每类调整只能由指定用例
创建；补册而不落这道门，就开出一条谁都能造调整的路，而那比没有册更坏——册在，看起来就像守着。

**三、`operating_result` 的组成项补角色维，封闭集按口径分组。** 元素加角色键，取值按 CONTEXT 三种
口径各自点名的采用物切分，**不切成一个跨口径的大平集**：角色集若不按口径分组，会出现「这个角色
在这个口径下不该出现」而册上拦不住，那等于把一处空转换成另一处。

约束落在**写侧重建复验**，不落 SQL `CHECK`：逐元素校验 jsonb 在本模块无先例，而写口复验有
（组成与毛利是否相符就是在那里守的）。落的判据是同一口径下「审核应付与贷项各至多一项，且贷项
在场时应付必须在场」——这条正是那句硬句可核对的形式。

**四、三条都补机制、都不改 CONTEXT。** 这三句不是模糊表述，是 CONTEXT 明确当成可核对要求写下的；
改文档等于把已经想清楚的约束降级。三张表 0 行是此刻补的理由，不是补的借口——晚补一天，代价只会
随行数上升。

## Consequences

- 迁移各新开序号文件，不改写已施加的 `0003`／`0005`（先例：`0012` 撤销 `0008` 的例外支）。
- 写侧连带最大：五项事实由谁在确认时钉上去、调整册的唯一创建用例、组成项角色由编排选料时给出，
  三处都要长出领域表达，不是加几列的事。
- 读面连带是好消息：票 admin-skeleton-closure-batch/04 按「册级缺席即撤栏」撤掉的那些栏
  （费用页的责任法人／结算相对方／收付方向／主要计费范围、版本栏，经营页的四栏）补册之后可以
  照册加回，读面仍不做判断、不代贴标签。
- `operating_result` 是 jsonb 列改元素形状，无需改表；但写侧与读侧必须同步，否则读回的旧形状译
  不出角色。表 0 行使这一点无历史负担。

## Alternatives considered

- **不补列，明说这些事实由别处拥有并被引用。** 否决：CONTEXT 那句的措辞是「固定」，读起来就是钉在
  费用行上；真要改成引用式，得先答出「别处」是哪一册，而今天没有那一册。
- **客户侧照搬供应商侧的版本链。** 否决，见 Decision 二：两侧语义不同，CONTEXT 专门警告过「普通
  客户费用的计价纠错形状不同，两者只共用调整原因这一个词」。
- **明认由对账单承载客户费用调整。** 否决：那要改 CONTEXT 两句，而库上现有的因果（先进对账单才
  存在）正是那两句要防的顺序。
- **把「审核应付与贷项各计一次」明认为写侧纪律。** 否决，且它此刻根本不成立：写口没有那道复验，
  角色在类型上都表达不出来（见 Context 末段）。「明认」要求先有一样东西可指。
- **角色约束落 SQL `CHECK`。** 否决（形式而非方向）：逐元素校验 jsonb 在本模块无先例，而写口复验
  有先例且离领域更近。日后若出现绕过写口的写入路径，这一条要重裁。

## Links

- [settlement-accounting CONTEXT.md](../domain/settlement-accounting/CONTEXT.md)：三句硬句的出处
  （「费用形成与证据」节与「经营指标、人工调整与系统边界」节）
- [ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：登记面写入代数
- [ADR-0067](./0067-cost-correction-restates-the-whole-evaluation-result.md)：供应商侧计价纠错重述全额，
  本裁决 Decision 二据以判定客户侧不照搬
- 驱动票：[settlement-register-context-gaps/01](../../.scratch/settlement-register-context-gaps/issues/01-customer-charge-does-not-fix-the-eight-confirmation-facts.md)、
  [02](../../.scratch/settlement-register-context-gaps/issues/02-customer-charge-has-no-version-chain-while-supplier-cost-does.md)、
  [03](../../.scratch/settlement-register-context-gaps/issues/03-operating-result-components-have-no-role-dimension.md)
