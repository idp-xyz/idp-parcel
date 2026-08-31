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
