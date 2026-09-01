# `customer_charge` 固定不了 CONTEXT 要求的那八项——册上直接成列的只有一项

Category: chore
Status: resolved——机制半边已落地（2026-09-01，见文末 Comment）；实例半边留空，
`ConfirmedChargeFactsView` 无生产实现，确认在事实无处可取时停在
`CONFIRMATION_FACTS_UNCONFIGURED`
Blocked by: 无

## CONTEXT 要求什么

`docs/domain/settlement-accounting/CONTEXT.md`「费用形成与证据」一节，硬句原文：

> 每条确认费用必须固定责任法人、结算相对方、收付方向、结算账户、合同或责任依据、结算币种、
> 主要计费范围和来源事实。**任何一项不能通过当前组织、当前客户属性或报表筛选临时推断。**

同节另有一句把「费用明细」与规则物分开：

> 费用项目、商业价格政策/可执行价卡版本和费用明细是三种不同对象。修改费用项目、价格政策或价卡
> 不得覆盖已经形成的费用明细；结算必须保留实际 `PricingEvaluation` 和采用解释。

第二句在册上是成立的（见下）。**成问题的是第一句。**

## 登记册实际记了什么（取证 `35564ad`）

`settlement_accounting.customer_charge`（`migrations/settlement_accounting/0003_customer_charge_advance.sql`
建表，`0013_customer_charge_currency_triple.sql` 扩列）今天的列：

    tenant_id, charge_id, fee_item, evaluation_ref,
    currency, amount_minor,                                    -- 0003 原有
    original_currency, original_minor,                         -- 0013 补
    settlement_currency, settlement_minor, conversion_ref,     -- 0013 补
    stage, confirmation_basis, formed_at, confirmed_at, recorded_at, inserted_at

对着八项逐项点：

| CONTEXT 要求固定的项 | 册上 | 说明 |
|---|---|---|
| 责任法人 | **无列** | |
| 结算相对方 | **无列** | |
| 收付方向 | **无列** | |
| 结算账户 | **无列** | |
| 合同或责任依据 | **无列** | `confirmation_basis` 是**确认依据**（哪份依据让它可以确认），不是合同或责任依据；`charge_confirmation_basis` 那张表记的也是确认依据的种类与引用。两者不同物。 |
| 结算币种 | **有** | `settlement_currency`（0013） |
| 主要计费范围 | **无列** | CONTEXT 另有硬句「每条费用明细必须且只能有一个主要计费范围」，册上无处安放 |
| 来源事实 | **无列** | `evaluation_ref` 是采用的 `PricingEvaluation`，它对上的是上引第二句（保留实际评价与采用解释），不是这八项里的「来源事实」——评价是依据，来源事实是评价的输入。 |

**八项里直接成列的只有结算币种一项。**

## 差在哪儿不只是「少了几列」

要紧的是**约束这一层**。`customer_charge_stage_closed` 允许 `stage = 'CONFIRMED'`，而
`customer_charge_confirmation_coupled` 只耦合了 `confirmation_basis` 与 `confirmed_at` 两半。
也就是说：**册上今天落得进一条「已确认」的客户费用，而它不带责任法人、结算相对方、收付方向、
结算账户与主要计费范围中的任何一项**，没有任何约束会拦。

CONTEXT 那句的后半截——「任何一项不能通过当前组织、当前客户属性或报表筛选临时推断」——因此在册上
是**空转的**：它禁止的那条路（事后推断）恰恰是唯一走得通的路，因为册上没有别的地方放这些事实。

票 04 的读面把这一层如实暴露了：管理台「费用与计费」页骨架有责任法人、结算相对方、收付方向、
主要计费范围四栏，读面**一栏都填不出**，四栏按「册级缺席即撤栏」全撤（裁定与依据见票 04 Comments
「对栏裁定」一节）。撤栏是读面能做的最诚实的处置，但它没有消除这处出入，只是不再掩盖它。

## 补与不补，各自的连带

### 若补列

- 至少五列（责任法人、结算相对方、收付方向、结算账户、主要计费范围），外加「合同或责任依据」与
  「来源事实」两项是否单独成列的裁量。收付方向应入封闭集（本模块已有先例：`recovery_adjustment_direction_closed`
  用 `DEBIT`/`CREDIT`；但这里要的是**收付**方向，与借贷方向不是一回事，词表须先定）。
- 约束要跟上，否则补了列等于没补：`CONFIRMED` 行必须五项俱全，形状同现有
  `customer_charge_confirmation_coupled` 那种「同在或同缺」的写法。非 `CONFIRMED` 行是否也要求，
  须一并裁（CONTEXT 那句只约束**确认**费用）。
- 动的是已施加的迁移，按本仓惯例新开序号文件重建约束，不改写 0003（先例：0012 撤销 0008 的例外支）。
- **表当前 0 行**，无数据清洗。
- 写侧连带：`SaveConfirmed` 一路与领域模型要一并长出这五项，以及它们从哪里来（谁在确认时把它们
  钉上去）。这是本票里最大的一块，不是加几列的事。
- 读面连带：票 04 撤掉的四栏可以加回来，读面按册转写即可，无需新裁决。

### 若不补

- 需要有人明说：这五项**不由 `customer_charge` 拥有**，而是由别处（结算账户？合同解析结果？）
  拥有并被引用。若如此，CONTEXT 那句的措辞就该跟着改——「固定」现在读起来像是钉在费用行上。
- 代价是那句硬句继续空转：它禁止事后推断，但册上没有第二个地方存这些事实，于是任何要用到它们的
  下游（对账单按结算账户归集、经营指标按责任法人归因）都只能推断，而这正是它禁的。
- 管理台那四栏永久撤销，页面骨架的注释也应跟着改——它现在照 CONTEXT 词条写着这四栏，读起来像是
  接线没做完，实际是册上没有。

### 第三条路：明认分歧

把这处出入记进 CONTEXT 或一份 ADR，写明「已知册上不固定这八项，理由是 X，重启条件是 Y」。
本仓有先例把「今天不做」写成明认（不是默认沉默）。**这也是一个合格的结果**，比让硬句继续空转好。

## 边界

本票**不改表、不建列、不写迁移、不动领域模型**，只把差异摆到可裁的形状。不构成票 04 的阻断：
读面按册上实有的内容照实转写，补不补列都成立。

## Comments

- 2026-09-01 · MCP-1：**ADR-0087 决定一实施完毕，机制半边收口。** 本票上文的差异描述
  自此描述的是补齐**之前**的状态，不再是现状。

  落地的四层：

  - **领域**：`ConfirmedChargeFacts` 把七项打成一个类型，`Confirm(facts, basis, at)` 一步
    钉上、缺任一项返 `ErrInvalidCustomerCharge`，`ConfirmedFacts()` 只在已确认费用上给出。
    收付方向立 `ChargeDirection`（`RECEIVABLE`/`PAYABLE`），未复用同文件的 `AdjustmentDirection`
    借贷二值。合同或责任依据、来源事实各自成格，未与 `ConfirmationBasisReference`、
    `SellEvaluationReference` 合用。
  - **库**：迁移 `0014_customer_charge_confirmation_facts.sql` 补七列，
    `customer_charge_confirmation_facts_coupled` 按同在或同缺把门、
    `customer_charge_direction_closed` 镜像封闭词表。非确认行要求七项**全缺**而非「不作要求」，
    理由记在迁移注释里：领域在确认之前表达不出这些事实，放行带事实的预估行等于开出一条
    领域产不出也读不回的写入路径。
  - **应用**：七项由新端口 `ports.ConfirmedChargeFactsView` 交出，**不进 `ConfirmChargeCommand`**
    ——命令带得了它们，CONTEXT 禁的「临时推断」就只是换了个人做。事实无处可取与确认条件
    未配置分两格（`CONFIRMATION_FACTS_UNCONFIGURED` / `CONDITION_UNCONFIGURED`），判据是两者
    的恢复动作不同：一个等结算事实册，一个等确认条件目录。
  - **适配器**：写口七项与确认留痕同笔 INSERT、冲突分支同笔改写；读口重建时逐列走各自
    构造门装回。

  **未做且有意未做**：`ConfirmedChargeFactsView` 没有生产实现——那是实例半边，本仓无租户
  因而无册可读，不造默认值。`ConfirmChargeHandler` 至今只在测试里装配，未进 `cmd/parcel-api`。
  票 04 撤掉的四栏可以按册加回，属另一片。

  **验证**（父提交 `a573551`，本笔改动尚未提交时实测）：`gofmt -l` 无输出、`go build ./...`、
  `go vet ./...` 绿；`go test -count=1 ./...` 全仓 89 包 ok、0 FAIL，DSN 已设为门禁容器，
  `settlementaccounting/adapters/postgres` 单跑 `-v` 得 104 PASS / 0 SKIP / 0 FAIL——SKIP 为零
  即证明这不是未设 DSN 跳过冒充的绿。新增四格库面判据各自钉住**是哪条约束**拒的而不只是「拒了」——写成
  `err != nil` 的第一版当场放过了一个真缺陷：`confirmed_at` 取 Go 侧时间、`formed_at` 取库侧
  `now()`，两个时钟谁先谁后不定，行其实是被旧的 `customer_charge_confirmation_coupled` 拒的，
  单跑侥幸通过、全仓跑才翻出来。

- 2026-09-01 · MCP-5：**重新取证于 `d11e0f0`，票面结论一字未变。** `customer_charge` 的列今天
  仍是 `0003` 那批加 `0013` 的币种三件——责任法人、结算相对方、收付方向、结算账户、主要计费
  范围一列都没有，八项里成列的仍只有结算币种。重记这一条不是复述：本仓已两次出现「票面事实被
  后来的交付改变而无人回填」（参与方身份读面、外部标识的面单交易宿主），锚旧了的票会被当成现状
  引用。本票仍 `draft`，待裁三选一原样有效。
