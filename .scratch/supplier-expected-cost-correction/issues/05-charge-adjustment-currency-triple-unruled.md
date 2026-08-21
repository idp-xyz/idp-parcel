# `ChargeAdjustment` 是否受币种三件组约束未裁，且没有评价引用

Category: enhancement
Status: resolved

> **`enhancement` 不预设第一问答「否」。** 它说的是**能力尚不存在**，不是能力不必存在——跨币种
> 调整即便第一问裁为「是、必须支持」，它仍然是一项要新建的能力。需不需要由 `ready-for-human`
> 加票面承载，不由 Category 承载。判据见 Comments 第五节。

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

**`Category`：`bug` → `enhancement`（协调岗 2026-08-21 裁定，已改到票面）。**

我原先的两难是「记 `bug` 等于预设第 1 问答是，改 `enhancement` 等于预设答否」。**后半句错了**：
`enhancement` 的定义是 new capability or improvement，它说的是**能力尚不存在**，不是能力不必存在。
跨币种调整即便第 1 问裁为「是、必须支持」，它仍然是一项要新建的能力，而不是一处要修的坏行为。
需不需要由 `ready-for-human` 加票面承载，不由 Category 承载。

**实质判据出自本节上面第一节自己查到的那一条：草稿门先核调整币种必须等于立单币，不等即
`ErrInvalidStatementDraft`——我写了「这一步是对的」。那就是判据：今天没有任何东西给出错的答案，
门是拒绝，不是静默记错。**

与分区键碰撞票对照，两张票落在同一条线的两侧：

| 票 | 形状 | Category |
|---|---|---|
| partition-key-space-collision/01 | **能力在场且错**——两个分区键表达已逐字写在仓里，会静默共分区 | `bug` |
| 本票 | **能力不在场，在场的那部分行为正确** | `enhancement` |

我原先说「两边都不完全落座」——落座点是存在的，只是不在我找的那一维上：不是「这个缺口算不算
错」，而是**「今天跑着的代码给出错答案了吗」**。后者答否。

### 六、四问裁定（2026-08-21 MCP-3 受用户委托裁断，转 `ready-for-agent`）

裁定的权威落点是 [SA CONTEXT](../../../docs/domain/settlement-accounting/CONTEXT.md) 新增的
「费用调整同受上述三件组约束」一条（紧随同币种两额相等那条之后）；本节记裁定理由与四问对应，
实现以 CONTEXT 原句为准。

1. **三件组覆盖调整：是。** 理由取自本票 Comments 第一节自己的取证：调整金额与费用金额进的是
   同一张对账单、同一条审计链，而纠纷恰恰集中在更正处——费用能表达的跨币种，它的调整表达不了，
   审计链就在最要紧的地方开洞。「调整是差额新对象不是费用本体」改变的是对象形状，不改变
   金额进账的语义。
2. **依据按种类挂：** 计价纠错类的三件整组出自它引用的那一个新 SELL 评价（与
   `CustomerCharge` 的「三件同源」同款，评价引用是必备件）；商业让利类的金额与币种出自有效
   商业授权，不强造评价引用——让利不是重评价。授权以非结算币表达让利时，换算依据必须随授权
   内容保存，取不到即拒绝形成调整（不自行取汇率补算，与费用同币种规则同一条理由）。
3. **`AdjustmentAuthorityReference` 拆格：拆。** 与同包 `CustomerCharge` 的
   `evaluation`/`confirmation` 分槽保持一致——纠错类挂评价引用，让利类挂商业授权引用，
   `Kind` 与依据槽联动校验：有此无彼即拒。相邻类型对「依据」的建模粒度不一致没有任何领域
   理由，只有历史顺序。
4. **结算币身份上升到类型：该。** 独立于第 1 问成立；第 1 问既裁「是」，实现时随三件组一并
   落（`settlementCurrency` + `settlementMinor` 命名对齐 `CustomerCharge`），
   `CutStatementDraft` 的草稿门不动——类型可见性与门禁互为冗余是这里要的形状。

**实现边界**：`FormChargeAdjustment` 零生产调用方，本票实现只改领域类型与其测试，不建
编排、不建表（对账单行快照要不要补原币列，随第一个真要用它的编排一并裁，本票不预造）。
证据等级照旧只记 `S`。

- 2026-08-21 MCP-2：用户同轮亦授权本会话裁断，四问独立裁得与上节完全相同的结论（覆盖：是；
  纠错挂评价、让利挂授权；依据拆格；结算币上类型），落笔时未见上节——记此作独立第二来源，
  不另留一份口径。唯一差异在快照扩列时点（我裁同笔扩列、上节裁不预造），**从上节**：零生产
  调用方时预造列正是「未确认参数写成生产默认」要拦的形状，且上节已把权威句落进 SA CONTEXT，
  单一权威在彼处。我先写入的重复段已撤，防两份裁定漂移。

- 2026-08-21 MCP-2（**实现交付，本票转 resolved**）：按第六节裁定与实现边界落地——
  `ChargeAdjustment` 补齐三件组（原币对、结算币对、换算依据），两道门与 `FormCustomerCharge`
  一字不差（跨币种缺换算 `ErrConversionStepMissing`、同币种两额不等即拒）；依据按 Kind 拆两格：
  纠错挂 `SellEvaluationReference`、让利挂新铸 `CommercialAuthorizationReference`，有此无彼即拒，
  `AdjustmentAuthorityReference` 一格随之退场（零生产调用方，全部调用点同笔更新）；结算币身份
  上类型（`SettlementAmount()`，对账单域三处读点跟随改名），`CutStatementDraft` 草稿门原样。
  按实现边界**未**建编排、**未**建表、快照行**未**扩列。测试：语义种类用例更新，分格用例
  （`TestAdjustmentBasesAreSlottedByKind`）与三件组用例
  （`TestAdjustmentCurrencyTripleMirrorsTheChargeGate`）新增；SA 三包全绿（真库含）。
  证据等级照旧只记 `S`。
