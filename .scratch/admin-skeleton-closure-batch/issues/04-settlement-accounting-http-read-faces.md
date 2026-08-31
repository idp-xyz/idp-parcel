# 结算与核算四页读面——`adapters/http` 整个包从零建

Category: feature
Status: in-progress——MCP-4（阶段一已自验绿，待 MCP-1 装配广播后进阶段二）
Blocked by: 无

本批最大一块，但**零设计裁决**：形状可逐字照抄。

## 现状（取证于 `65b6cf2`）

`internal/settlementaccounting` 的 `domain`、`ports`、`application`、`adapters/postgres`、
`adapters/partycommercial` 都在，写适配器很齐——**但整个 `adapters/http` 目录不存在**，
所以四页无处发请求。`migrations/settlement_accounting/` 下 25 张表、`tenant_id` 出现 78 处，
**ADR-0077 的读面形状逐字成立，不需要新 ADR**。25 张表当前全部 0 行。

样板取 `internal/collectionremittance`——它是最新一份从零建到接线的完整样板（域 → 端口 →
真库适配器 → `adapters/http` 查询处理器 → 隔离读准入 → 装配 → 页），逐层可比。

## 四页与表的对应（起点，需按页面栏目与 CONTEXT 复核后定稿）

| 页 | 落在哪些表 |
|---|---|
| `charges-billing` 费用与计费 | `customer_charge`、`charge_confirmation_basis`、`charge_confirmation_condition`、`cost_allocation`、`supplier_expected_cost` |
| `reconciliation` 对账单 | `customer_statement`、`statement_dispute`、`supplier_bill_reception`、`subsequent_inclusion` |
| `settlement-application` 收付款核销 | `settlement_application`、`external_funds_fact`、`funds_mapping`、`funds_freeze` |
| `operating-metrics` 经营核算 | `operating_result`、`operational_balance` |

这张表是我按表名与页名推的**起点，不是裁定**。定稿前请对着
`docs/domain/settlement-accounting/CONTEXT.md`、`docs/application/settlement-accounting/UC-SA-*`
与四张页现有骨架的栏目逐格复核；对不上就以文档与页面栏目为准，并在 Comments 里记明改了哪几格
与依据。

## 表会一直是空的，这是预期不是缺陷

25 张表的写入方是事务链（费用由计价评价形成、对账单由账期归集、核销由真实收付映射），而事务链
在接入渠道墙后面——见 [spec 事实基线](../spec.md)。四页接完**仍是空册**。

**完成判据只能写成「空态文案说的是『读取入口已配置、登记册为空』，而不是『尚未接线』」，
不得写成「页面有数据」。** 尤其不要为了让经营核算页好看去造毛利或损失数字：那是经营口径的
业务事实，造出来直接破「实例值留空拒默认」。读适配器的真库测试照常写（测试内插行再读回）。

## 地盘的一处例外

`internal/settlementaccounting/adapters/partycommercial/`（`pre_acceptance_control_policy.go`
及其测试）**此刻有他会话的在途未提交改动**，不在本票地盘内，不要碰、也不要把它当自己的验证
失败——碰到那里的编译错按 parallel-sessions 的规矩不修不报。本票只在 `ports`、
`adapters/postgres`、新建的 `adapters/http` 三处落笔。

## 两阶段与次序

- **阶段一**：四组读端口 + 真库读适配器（含真库测试）+ 新建 `adapters/http` 包（四个
  `query_*.go` + 隔离读准入）。自验绿后向频道交**已验 SHA** 与四条端点行；**不自改**
  `cmd/parcel-api` 装配四件（占号在票 07）。
- **阶段二**：收到 MCP-1「已装配」广播后，四页接真，`liveIds` 加四行——**只加自己那四行，
  不动邻行**。注意 `ReconciliationPage` 与 `SettlementApplicationPage` 在
  `apps/admin-web/src/pages/governance/` 下，不在 `settlement/`。

## 完成判据

四页转 live；空态文案说「读取入口已配置、登记册为空」而非「尚未接线」；含真库全仓绿（注明）；
新建的 `adapters/http` 包与 `collectionremittance` 同族（隔离读准入三条判据入格一致）。

## Comments

### 四页与表的对应：定稿（MCP-4，复核过 CONTEXT.md、UC-SA-* 与四张页骨架）

起点表改了三格，其余照收。改动与依据：

| 表 | 起点 | 定稿 | 依据 |
|---|---|---|---|
| `cost_allocation` | `charges-billing` | `operating-metrics`（第二册） | UC-SA-006 同一个用例形成分摊与指标、经同一个 `OperatingIntent` 交下游；分摊只改变经营归因，不转移原债权债务责任。摆进费用页会让读者以为它动了应收应付。 |
| `operational_balance` | `operating-metrics` | 本批不上（无端点） | 运营结算余额是结算账户的头寸（入账余额／当前有效授信／冻结／已确认未结／可用），属账户面。经营核算页的行对象是「按口径、计算版本、币种与截至时点派生的指标快照」（页面自注 + CONTEXT「经营毛利」词条），头寸不是快照的一行。 |
| `funds_freeze` | `settlement-application` | 本批不上（无端点） | 冻结是接受前财务控制的产物，与真实收付是两条链；CONTEXT 明禁用通知、对方认可或金额确认冒充到账。与已确认外部资金事实并进一册，两条链在同一张表上会看起来同源。长出查阅语义时另立入口。 |

照收的：`customer_charge` + `charge_confirmation_basis` + `charge_confirmation_condition` 入费用页
（后两者以 `requiredBasisKind` 与 `confirmationBases` 两格分开上列，**不代算交集**——0009 分两张表正是
为了让「确认条件已满足」没有第三条成立路径）；`supplier_expected_cost` 入费用页第二册；
`customer_statement` + `statement_dispute` + `subsequent_inclusion` 与 `supplier_bill_reception` 入对账页；
`external_funds_fact` + `funds_mapping` + `settlement_application` 入核销页。

### 一处必须先说的出入：页面栏目比登记册宽

`charges-billing` 现有骨架的行对象是**一张合流的「费用明细」表**（12 栏，含「收付方向」），
库上却是两张形状不同的表。逐格对下来，这六栏在册上没有出处：

**主要计费范围、收付方向、责任法人、结算相对方、版本、计费重量采用**
（`customer_charge` 的列只有：费用、费用项目、评价引用、阶段、币种三件组（0013 补）、确认依据、
形成／确认／登记三个时刻）。

同页的 `supplier_expected_cost` 反过来：有原币／结算币两栏与版本链，**没有**阶段。
`operating-metrics` 页的「客户侧采用／供应商侧采用／其中：审核应付／其中：供应商费用贷项」四栏，
在册上是 `operating_result.components` 的数组元素（`source` / `effect` / `amount_minor`），
元素上没有「哪一侧」「是不是审核应付」这类角色名。

读面按 ADR-0077 Decision 一照实转写，**没有为这六栏造值，也没有把两册合流**：
合流要现编「收付方向」（库上 `customer_charge` 不存这一维，而 CONTEXT 明写确认费用必须固定收付方向、
且「不能通过当前组织、当前客户属性或报表筛选临时推断」——由读面按行落在哪张表反推，正是这条禁的东西），
并给预期成本现编一个「阶段」。共享壳的另一条路是空出半数字段或折成 `any`，判据逐字同 `visibilityhttp`
六种目录行那句。故两册各自成形、按 `registry` 分派（先例 `/customs-ports-paths`）。

**阶段二的页面改动因此不是「填数」而是对栏**：这六栏要么撤栏，要么以「未登记」如实呈现——
不得由页面或读面补默认值（`实例值留空拒默认`）。这一格留给阶段二，与 MCP-1 的装配广播一并处理。

### 阶段一交付（不含 `cmd/parcel-api` 装配四件，占号在票 07）

四条端点行，全部 `GET`、全部只消费存储读面、全部不下推任何处置参数：

| 页 | 端点 | 册（`registry`） |
|---|---|---|
| `charges-billing` | `GET /settlement-charges` | `customer-charge`、`supplier-expected-cost` |
| `reconciliation` | `GET /settlement-statements` | `customer-statement`、`supplier-bill-reception` |
| `settlement-application` | `GET /settlement-funds-applications` | 无（封闭集为一不设参数，判据同 `/collection-subledgers`） |
| `operating-metrics` | `GET /settlement-operating-results` | `operating-result`、`cost-allocation` |

装配点需要的两件：`settlementhttp.UnconfiguredIntake{}` 与
`settlementhttp.NewIsolatedOperationsReadIntake(scopeRef, tenant, limit)`，四个端点共用一个 Intake。

落笔只在 `ports`、`adapters/postgres`、新建的 `adapters/http` 三处，外加 `domain/operations_query_scope.go`
（本上下文自己的 `OperationsQueryScope`／`OperationsScopeReference`——ADR-0077 Decision 二「每上下文自立，
各自成形互不参数化」，不从别的上下文导入共享作用域类型）。`adapters/partycommercial/` 一字未动。

### 对栏裁定（阶段二照此执行；本节只动票面，未动代码）

上一节把「对栏」留给了阶段二。现在把它做完。**判据一条：栏的去留看缺席是行级还是册级。**

- **行级缺席**——这一册记得下，这一行还没有（如预估费用还没有确认依据）。**保留栏**，如实呈现
  「未登记」。读的人据它去催那一行的登记，那是真话，也指得出续办。
- **册级缺席**——这一册根本不记这件事（如 `customer_charge` 上没有责任法人这一列）。**撤栏**。
  整列永远是「未登记」不叫如实：它把「本册不记」说成「本册记漏了」，反过来招人去别处推断补齐，
  而 CONTEXT 恰恰明禁推断收付方向这类事实。
- 第三格：**换栏**——页面要的那件事册上有，但名字与切分不同。换成册上真有的那一栏，不折不并。

三格之外没有第四种处置，**任何一栏都不得由页面或读面补默认值**（`实例值留空拒默认`），
**两册也不合流**（理由见上一节）。

#### 一、`charges-billing` 费用与计费

现有骨架十二栏。对得上的六栏照接：费用明细标识（`charge_id`）、费用项目（`fee_item`）、阶段
（`stage`）、原币金额与合同结算币金额（0013 补的币种三件组）、依据（`evaluation_ref` +
`confirmation_basis` + 已到达的确认依据数组）。其余裁定：

| 栏 | 裁定 | 依据 |
|---|---|---|
| 主要计费范围 | **撤栏** | `customer_charge` 无此列（册级）。CONTEXT 硬句「每条费用明细必须且只能有一个主要计费范围」讲的是写侧不变量，不是读面可以推出来的东西。 |
| 收付方向 | **撤栏** | 册级缺席，且是六栏里最不能留的一栏：CONTEXT 明写确认费用必须固定收付方向，并且「不能通过当前组织、当前客户属性或报表筛选临时推断」。留着空栏就是在请人推断；按行落在哪张表反推更是直接违此句。 |
| 责任法人 | **撤栏** | 同上，册级缺席。 |
| 结算相对方 | **撤栏** | 同上，册级缺席。 |
| 版本 | **撤栏** | 册级缺席，且是结构性的：`customer_charge` 主键为（租户，费用），一费用一行，没有版本列也没有前版／纠错回指。与 `supplier_expected_cost` 恰成对照——那一册**有**完整版本链。这处不对称是真的，见下「登记册缺口」。 |
| 计费重量采用 | **撤栏** | 册级缺席。它是客户／供应商各自计量规则的采用结果，是被费用引用的另一个对象，不是费用行上的一列；要呈现须另立读面。 |
| 阶段（四格→三格） | **改页面说明** | 栏保留，但页面注释写的「预估／暂估／确认／调整」四格与册上封闭集三格（`ESTIMATED`／`PROVISIONAL`／`CONFIRMED`）不符。CONTEXT 生命周期把「调整」定为**追加的调整明细**（各由唯一创建用例形成，落 `recovery_adjustment`／`claim_amount_adjustment` 两册），不是阶段的第四个取值。阶段栏改述三格；调整不在本页本册出现。 |
| 依据 | **保留 + 行级未登记** | 这是四页里少数真正的行级缺席：预估／暂估行没有确认依据（库上 `customer_charge_confirmation_coupled` 守着两半同在或同缺）。空即「尚未确认」，读的人据它去催确认。 |

**`supplier_expected_cost` 缺阶段**：该册**不设阶段栏**。CONTEXT 明写「预期成本属预估口径，
从不进对账单」——整册同属一个口径，那是**册级事实**。补一个恒为「预估」的阶段栏，是把册级事实
伪装成行级取值，还会让读者以为它可能变成别的值。两册各自成节、各带自己的栏目集，这正是不合流的
用处；该册另有客户册没有的栏（采购规则版本、供应商协议、运输收费发生项与版本、前版与纠错原因），
一并按册上原样呈现。

#### 二、`reconciliation` 对账单

对得上的五栏照接：对账单号、结算账户、结算周期、币种、总额。三个状态栏裁定不同，不可一并处理：

| 栏 | 裁定 | 依据 |
|---|---|---|
| 单据状态 | **保留 + 行级二态** | 册上有作废留痕（`void_basis`／`voided_at`，成对约束守着），二态在库上完备：有留痕即已作废，无即已发布。这是同一事实换个词，不是新判断。作废不删行不改总额，故「已作废」是这一行上的留痕。 |
| 对账状态 | **撤栏** | 册级缺席。册上有异议（`statement_dispute`），但**异议不是对账状态**：没有异议不等于相对方已对账确认。由「有无未裁定异议」推出对账状态是读面替人下结论，且会把「还没人看」说成「已对账」。 |
| 结清状态 | **撤栏** | 册级缺席，且跨册。结清要把 `settlement_application`／`funds_mapping` 里指向本单的核销汇总，再对「全额／部分／未结」下判。汇总本身可以做，**下判不行**——那超出「派生只到求和与计数为止」这条线。真要这一栏，须先有一个写侧口径明确的结清结果，不是目录行现算。 |

另：读面已供的「费用行数／调整行数」与「异议、后续账期纳入」两组挂载物，骨架上没有对应栏。
它们不是缺口，是可以新增的栏——**行数**分得开「这张单里有几行」与「一行都没有」，接线时值得加上；
异议与纳入按详情面处置，不塞进列表（同页面自注）。

#### 三、`settlement-application` 收付款核销

| 栏 | 裁定 | 依据 |
|---|---|---|
| 方向（收款／付款） | **换栏 → 事实种类** | 册上是 `kind` 三格封闭集（`RECEIPT_CONFIRMED`／`PAYMENT_FAILED`／`FUNDS_RETURNED`），三格里只有一格是「收到了钱」。折成收／付两向会把「付款失败」与「资金退回」压进同一格。换成事实种类栏，照三格原样呈现。 |
| 对方 | **撤栏** | `external_funds_fact` 无此列（册级）。页面自注「身份由外部事实保存，本页不推断」——册上确实没保存，那就不该留栏请人推断。 |
| 责任法人 | **撤栏** | 同上，册级缺席。 |
| 结算账户 | **撤栏** | 同上，册级缺席。页面自注设想的是「供数方如实表达缺口」，但缺的是整列不是某行，故按册级处置。 |
| 已分配金额 | **保留** | 读面 `appliedAmount`，只加未撤销的核销。 |
| 未分配金额 | **保留** | 读面 `unappliedAmount` = 事实金额 − 已分配。与上一栏并列，正合页面自注「未分配是第一类结果不是尾差」。 |
| （新增）已撤销金额 | **加栏** | 读面已供 `reversedAmount`。不加这一栏，「从未核销过」与「核销过又撤销了」在页面上分不开，而这两态的续办相反。撤销不删历史，那一截金额必须仍看得见。 |
| 分配状态 | **撤栏** | 由三个金额栏如实呈现，不再给一个三态词。「未分配／部分核销／已核销」按已分配与总额的关系算得出，但它会把上一栏那两态压成同一格——恰是拆三个数要避免的。 |
| 原币金额／币种／业务时间 | **保留** | 分别接 `amount`／`currency`／`occurredAt`（业务发生时刻，不是写入时刻）。 |
| 操作「人工分配」 | **保持 disabled 不动** | 本票只建读面；核销由 `UC-SA-005` 形成，接线不取得创建权。 |

#### 四、`operating-metrics` 经营核算

对得上的六栏照接：分析范围、口径（`basis` 封闭三格与页面「预估／已确认／已结算」逐格对应）、
计算版本、币种、截至时点、经营毛利。

| 栏 | 裁定 | 依据 |
|---|---|---|
| 客户侧采用 | **撤栏，改为组成逐项呈现** | 四栏都是对 `operating_result.components` 按角色的切分，而元素上只有 `source`／`effect`／`amountMinor` **三个键，没有角色维**（见 `componentRow`）。要填这四栏，读面得先判断某个 source 属于客户侧还是供应商侧、是不是审核应付、是不是贷项——那正是 CONTEXT 硬要求「审核应付与贷项按各自借贷方向分别计入一次、不得净含贷项」想让人**看见**的东西，由读面代贴标签等于把它重新藏起来。改为按组成逐项呈现（来源、增减向、金额）。 |
| 供应商侧采用 | 同上 | 同上 |
| 其中：审核应付 | 同上 | 同上 |
| 其中：供应商费用贷项 | 同上 | 同上 |
| 经营损失 | **撤栏** | 与经营毛利是同一个数按正负分两栏，会让「零」落进两栏都不占的缝里。毛利一栏带符号呈现，负值即经营损失。 |

`cost_allocation` 作为本页第二册（见上一节改判）另立一节，栏目取册上原样：分摊标识、来源金额与
身份、规则版本、份额（逐项）、未分摊余额、版本与纠错回指。**未分摊余额单独成栏**——它是第一类
结果不是尾差，不折进份额里凑平。

#### 五、这些裁定合起来说明的一件事：差的是登记册，不是行数据

值得单记一笔：四页里对不上的栏，**几乎全是册级缺席或换栏，只有「依据」「单据状态」两处是行级**。
也就是说，页面骨架是照 CONTEXT 词条写的，登记册是照用例落的，两者的差不是「实例还没到」，
是登记册上就没有那一维。四页接完仍是空册，**接完之后这些栏也不会长出来**。

其中三处够得上「登记册缺口」，记在此处供写侧裁量（**不属本票范围，本票不改表不建列**）：

1. **`customer_charge` 与 CONTEXT 硬句 122 有实质出入**。该句要求每条确认费用固定责任法人、
   结算相对方、收付方向、结算账户、合同或责任依据、结算币种、主要计费范围与来源事实八项；
   表上只有结算币种（0013）与来源事实（`evaluation_ref`），其余六项一列都没有。
2. **客户费用没有版本链，供应商预期成本有**。CONTEXT 对两侧都写了「确认后形成新版本或调整明细，
   不覆盖历史结果」，但 `customer_charge` 一费用一行、无前版回指。
3. **`operating_result.components` 元素上没有角色维**，而 CONTEXT 要求审核应付与贷项按各自借贷
   方向分别计入一次并可查——没有角色名，这条要求在读面上验证不了。

阶段二只按上表对栏，**不碰以上三条**：改表是写侧的事，本票零设计裁决。
