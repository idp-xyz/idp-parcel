# `ChargeAdjustment` 是否受币种三件组约束未裁，且没有评价引用

Category: bug
Status: ready-for-human

由 [`04`](./04-customer-charge-single-currency-contradicts-context.md) 的裁断划出：owner
2026-08-21 裁定客户**费用**受三件组约束（[SA CONTEXT](../../../docs/domain/settlement-accounting/CONTEXT.md)
「每条费用分别保存原币金额、合同结算币金额及换算依据」按字面执行），并明确**费用调整
是否同受约束另票单裁**。本票即那张票。

## 病灶

`internal/settlementaccounting/domain` 的 `ChargeAdjustment` 只带单一币种与单一金额
（`Currency` + `AmountMinor`），带借/贷方向。两个未裁问题：

1. CONTEXT 那句「每条费用」是否覆盖调整——调整是引用费用的差额新对象，不是费用本体；
   但它的金额进入对账单（`StatementAdjustmentLine` 复述其金额入快照），币种表达问题
   同样成立。
2. 若三件组罩住调整，`ChargeAdjustment` 今天**连评价引用都没有**：计价纠错类调整的
   金额语义上出自新的 SELL 评价，「三件从同一个评价采用来的一组」在这个类型上无处挂。
   商业让利类调整的金额是否出自评价、还是出自商业授权本身，亦未裁。

## 与 ADR-0067 的边界

[ADR-0067](../../../docs/adr/0067-cost-correction-restates-the-whole-evaluation-result.md)
决定一只裁了**形状**——客户侧调整是带借/贷方向的差额调整，不是重述全额的新版本——
没有裁调整金额的币种表达。不要把该 ADR 当本票的答案引用。

## 不在本票内

- 本票只记录未裁问题，不提方案；属领域 owner 裁断。
- 客户费用本体的三件组已由 `04` 裁定并实现，不在本票。

## 影响面

初步 grep：无 `charge_adjustment` 独立表，调整金额以对账单行快照（迁移 `0002`）落库；
`FormChargeAdjustment` 的调用方与完整落库面 triage 时取证。

> **已取证（2026-08-21 对 `d5e5d20`，见 Comments 第一节）。** 两句均属实。补三件：调用方
> 全仓两处且都是领域测试（零生产调用方、零应用层编排）；快照类型 `StatementAdjustmentLine`
> 四字段无币种列；**币种不是被静默丢弃的**——`CutStatementDraft` 先核过调整币种必须等于
> 对账单立单币才丢。第三件的后果是**单一币种今天已被钉死为结算币**，因此跨币种调整要么
> 进不了对账单，要么进得去而原币与换算依据无处记载。

## Comments

- 2026-08-21 · MCP-2：随 `04` 裁断创建，防「另票」落空。

- 2026-08-21 · MCP-2（T3 裁断取证，基线 `d5e5d20`，零代码；只记未裁问题与证据，不提方案）：

### 一、`FormChargeAdjustment` 的调用方与完整落库路径

**调用方：全仓两处，都是领域测试。** `customer_charge_test.go` 的
`TestAdjustmentsDemandTheirSemanticKind`，与 `customer_statement_test.go` 的 `creditAdjustment`
辅助函数。**零生产调用方，且零应用层编排**——`internal/settlementaccounting/application/` 下与
「adjustment」有关的只有 `settle_claim_amounts.go` 与 `assess_advance_recovery.go`，那两处是索赔
金额与预付回收的调整，不是 `ChargeAdjustment`。**今天没有任何用例形成一笔费用调整。**

**落库路径复核：票面那句属实，但要补三点，其中第三点改变本票的问题形状。**

1. 确无 `charge_adjustment` 独立表。
2. 快照类型 `StatementAdjustmentLine` 只有四个字段——`Adjustment` / `Charge` / `Direction` /
   `AmountMinor`，**无币种列**；持久化侧 `statementAdjustmentRow` 同形（`customer_statement.go`
   适配器逐字段读写，迁移 `0002`）。
3. **但币种不是被静默丢弃的。** `CutStatementDraft` 逐条核对
   `adjustment.Amount()` 交回的币种必须等于对账单立单币 `spec.Currency`，不等即
   `ErrInvalidStatementDraft`（与它上一段核费用 `charge.SettlementAmount()` 币种是同一道门）。
   核过之后 `PublishStatement` 才用 `_, amount := adjustment.Amount()` 丢掉币种——此时它与
   `PublishedStatement.currency` 冗余。**这一步是对的，不构成缺陷**，取证时我一度按「静默丢弃」
   立论，读到草稿门才纠正。

**第 3 点的后果，是本轮取证里对本票最有用的一件：**

因为草稿门要求调整币种等于对账单立单币，而对账单以**合同结算币**立单，所以今天
`ChargeAdjustment` 那个单一币种**在语义上已经被钉死为结算币**——只是这个约束不在类型上，而在
`CutStatementDraft` 里。于是：

> **一笔原币与结算币不同的调整，今天要么进不了对账单，要么进得去但原币金额与换算依据无处
> 记载。** 按原币填 → `ErrInvalidStatementDraft` 挡掉；按结算币填 → 进得去，而那笔调整原本是
> 以什么币、按什么换算得出这个数的，全域没有一个字段能装。

这与票 `04` 实现后的 `CustomerCharge` 构成**对称缺失**：费用可以原币 USD、结算币 CNY、带
`Conversion` 一起进 CNY 对账单（草稿门比的是 `SettlementAmount()` 的币种，不比原币）；而它的调整
没有这一对。**同一条链上，费用能表达的跨币种，它的调整表达不了。** 票面原写「币种表达问题同样
成立」，实测下来比这句更具体也更硬。

### 二、票 `04` 实现后的实际形状，与 `ChargeAdjustment` 的逐件差异

三件组在 `CustomerCharge` 上的落地形状（`FormCustomerCharge` 与 `CustomerChargeSpec`）：

| 件 | `CustomerCharge` 今天 | `ChargeAdjustment` 今天 |
|---|---|---|
| 原币金额 | `originalCurrency` + `originalMinor` | **无** |
| 结算币金额 | `settlementCurrency` + `settlementMinor` | `currency` + `amountMinor`——**在，但无名**：类型上看不出它是结算币，那个身份只活在 `CutStatementDraft` 的比对里 |
| 换算依据 | `conversion ConversionStepReference` | **无** |
| 三件同源 | `evaluation SellEvaluationReference`；`CustomerChargeSpec` 注释原话「原币金额、合同结算币金额与换算依据是从它引用的那一个 SELL 评价采用来的一组……**不是可各自另取的三件**」 | **无评价引用** |
| 跨币种守门 | `ErrConversionStepMissing`——原币≠结算币而无换算即拒 | 无第二币种，无从守 |
| 同币种守门 | 两额不等即 `ErrInvalidCustomerCharge`，注释「没有换算却造出了第二个数」 | 只有一个数，无从守 |

**差得具体是：四件里缺三件（原币对、换算依据、评价引用），第四件在但被降维成无名的单一币种。**
并且两道守门在调整上都不是「被省略」而是「无处可守」——它们守的是两个数之间的关系，而调整只有
一个数。

### 三、计价纠错类与商业让利类，代码里分不分得开

**分得开，靠 `AdjustmentKind` 封闭二值。** `PricingCorrection` / `CommercialConcession` 两格，
`FormChargeAdjustment` 里 `!spec.Kind.valid()` 直接拒收，`AT-SA-056`（「人工提交『冲销』但未说明
语义」）钉的就是这一格；`AdjustmentKind` 的类型注释还写明赔付、索赔退款、追偿与供应商贷项在这里
**没有格**，各归其唯一创建用例。

**但票面第 2 问真正卡住的地方比「分不分得开」更靠后一步，这一层票面没写：种类分得开，两类各自的
依据却挤在同一个字段。** `AdjustmentAuthorityReference` 的构造注释原话是「指名调整的证据或授权
（**纠错的证据、让利的有效商业授权**）」——同一个字段承载两种语义不同的东西，`Kind` 之外没有第二
个维能分辨那个字符串指的是评价还是商业授权。

于是纠错类要挂的「新 SELL 评价」若塞进 `Authority`，它就与让利类的商业授权共用一格。而
**`CustomerCharge` 恰恰是把两者分开建的**：`evaluation SellEvaluationReference` 与
`confirmation ConfirmationBasisReference` 各占一格。**同一个包里两个相邻类型对「依据」的建模
粒度不一致**——这是本票落地时必须一并回答的问题，票面只列了两个未裁问题，这是第三个。

### 四、未裁问题清单（本票只记，不提方案；均属领域 owner 裁断）

1. （票面原有）SA CONTEXT「每条费用分别保存原币金额、合同结算币金额及换算依据」是否覆盖调整。
   调整是引用费用的差额新对象，不是费用本体。
2. （票面原有）若覆盖，评价引用往哪挂；商业让利类的金额是否出自评价、还是出自商业授权本身。
3. **（本轮新增）** `AdjustmentAuthorityReference` 一格承载「纠错证据」与「商业授权」两类语义，
   要不要按 `Kind` 拆成两格——与 `CustomerCharge` 把 `evaluation` 与 `confirmation` 分开建保持
   一致，还是维持现状。
4. **（本轮新增）** 单一币种今天被 `CutStatementDraft` 钉死为结算币，而这一身份在
   `ChargeAdjustment` 类型上不可见。该不该把它上升到类型本身（哪怕不上三件组，也先给那个字段
   一个名字），是一个独立于第 1 问的选择：第 1 问答「否」时它依然成立。

**边界守住**：未把 ADR-0067 当本票答案引——该 ADR 决定一只裁了形状（带借/贷方向的差额调整，不是
重述全额的新版本），没有裁币种表达，本轮取证一次都没有据它推断币种结论。

### 五、Status 与 Category

`needs-triage` → `ready-for-human`，已改到票面。四个未裁问题全属领域裁断，无可由 agent 直接开工
的机制件；且 `FormChargeAdjustment` 零生产调用方，也不存在「先改了再说」的紧迫性。

**`Category` 未动，但我认为它值得复议，理由写在这里由 owner 或协调岗定**：按「缺席 vs 在场且错」
那条界线，本票两边都不完全落座——`ChargeAdjustment` 在场，它与相邻的 `CustomerCharge` 对同一个
问题（币种表达）给出不一致的建模，但**那算不算错取决于第 1 问怎么裁**，而第 1 问正是本票要问的。
把它记成 `bug` 等于预设了第 1 问答「是」。我没有单方面改，因为改成 `enhancement` 同样是预设
（预设答「否」）。**若要一个不预设的标签，这张票的实际形状是「待裁」。**
