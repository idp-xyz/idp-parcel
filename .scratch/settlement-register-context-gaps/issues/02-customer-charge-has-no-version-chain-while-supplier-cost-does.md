# 同一条「不覆盖历史结果」，供应商侧有整条版本链，客户侧两条路都没有

Category: chore
Status: resolved——机制半边已落地（2026-09-01，见文末 Comment）：调整册、唯一创建用例
与所有权门禁齐备；`customer_statement.adjustment_lines` 退回纳入关系那一步另立票
Blocked by: 无

## CONTEXT 要求什么

`docs/domain/settlement-accounting/CONTEXT.md`「费用形成与证据」一节：

> 费用采用预估、暂估、确认和调整的追加式生命周期。**确认后出现新事实、源事实更正或规则适用性
> 变化时形成新版本或调整明细，不覆盖历史结果。**

「费用明细」生命周期一节把它说得更死：

> 已确认费用发现迟到事实、差错、客户费用退款、供应商费用贷项或其他有效变化时，由对应唯一创建
> 用例追加调整明细或新版本；原确认费用仍保留，**对账纳入和资金核销不能代替金额调整**。

这句给了**两条**合法出路：新版本，或调整明细。下面逐条看两侧各有哪条。

## 供应商侧：两条路里的「新版本」齐全（取证 `35564ad`）

`settlement_accounting.supplier_expected_cost`（`0008_supplier_expected_cost.sql`）主键含 `version`，
另有 `prior_version` 与 `correction_reason` 成对可空；`0012_same_currency_agree_for_all_versions.sql`
依 ADR-0067 撤掉了 0008 给纠错版本留的例外支，把「同币种两额必须相等」推到所有版本。CONTEXT 对这
一侧还有专门一句：

> 供应商预期成本的计价纠错是一个重述全额的新成本版本，不是带借/贷方向的差额调整。

**这一侧对得上，没有问题**，列在这里只是作为对照——同一条硬句在两侧的落法差得很远。

## 客户侧：两条路一条都没有

### 「新版本」这条：没有

`settlement_accounting.customer_charge`（`0003_customer_charge_advance.sql`）：

- `customer_charge_pkey` 是 **（`tenant_id`, `charge_id`）**——一条费用一行。
- 没有 `version` 列，没有 `prior_version`，没有 `correction_reason`。

写侧对覆盖是**拒绝**而不是追加：0003 文件头自注「已确认行不会被第二次确认覆盖——`ON CONFLICT`
后 `WHERE stage IS DISTINCT FROM 'CONFIRMED'`，零行译`已确认`（ADR-0031）」。拒绝覆盖是对的，
但它只做到了「不覆盖」，**没有给「新版本」留位置**：新版本无处可落。

### 「调整明细」这条：没有自己的册

`ChargeAdjustment` 在领域里是有的（`domain/customer_charge.go`），但**迁移里没有任何
`charge_adjustment` 表**。它落库的唯一形态是 `customer_statement.adjustment_lines` 这一列 jsonb，
元素形状见 `statementAdjustmentRow`（`adapters/postgres/customer_statement.go`）：调整标识、被调整的
费用、借贷方向、金额——**这是一条「对账单调整行」，不是调整本身的登记册**。

于是库上的因果是反的：**一笔客户费用调整只有先进了某张对账单，才在库上留下痕迹。** 而 CONTEXT
恰恰写着「对账纳入和资金核销**不能代替**金额调整」——调整应当先成立，再被纳入。今天不进对账单
它就不存在，这正好是那句话要防的顺序。

`subsequent_inclusion.adjustment` 也只是**指向**一个调整标识的引用（可空，`LATE_CHARGE` 时为空），
同样不构成调整的登记册。

（`recovery_adjustment` 与 `claim_amount_adjustment` 是追偿与索赔金额的**专用**调整册，各有自己的
用例与借贷方向封闭集，不是通用的客户费用调整册。）

## 这处出入在管理台上的样子

票 [admin-skeleton-closure-batch/04](../../admin-skeleton-closure-batch/issues/04-settlement-accounting-http-read-faces.md)
的对栏裁定里，「费用与计费」页的**版本**栏按「册级缺席」撤掉了，同页的**阶段**栏也从页面注释写的
四格（预估／暂估／确认／调整）改述为册上封闭三格（`ESTIMATED`／`PROVISIONAL`／`CONFIRMED`）——
「调整」不是阶段的第四个取值，它是追加的调整明细。两处处置都是照册说话，但合起来看就是：
**管理台上看不出一条客户费用被调整过**，因为册上确实没记。

## 补与不补，各自的连带

### 路一：给客户费用补版本链（照供应商侧的形状）

- 主键加 `version`，补 `prior_version` 与 `correction_reason` 成对可空，约束照
  `supplier_expected_cost` 那套写。
- 好处是两侧同形，读面与页面各加一栏即可，`0012` 那类「整组重述」的口径也可以照搬。
- 但 CONTEXT 明写供应商侧的计价纠错「不是带借/贷方向的差额调整」，而客户侧的调整**是**带借贷方向
  的（`statementAdjustmentRow.Direction`、`recovery_adjustment_direction_closed`）。两侧语义本就不同，
  照搬形状要先确认不是把供应商侧的口径硬套过来——CONTEXT 那句「普通客户费用的计价纠错形状不同，
  两者只共用调整原因这一个词」正是在警告这一点。

### 路二：给客户费用调整补一张自己的册

- 更贴 CONTEXT 的措辞：调整明细本就是独立对象，有自己的调整类型、业务原因、权威依据与
  **唯一创建用例**（CONTEXT「调整类型与唯一所有权」表）。
- 落成册之后，对账单的 `adjustment_lines` 从「调整的唯一存身处」退回它本来的角色——**纳入关系**，
  与 `subsequent_inclusion` 一致。
- 连带最大的一块是「唯一所有权」：每类调整只能由指定用例创建，补册时这道门要一并落，否则会开出
  一条谁都能造调整的路。

### 路三：明认由对账单承载

写明「客户费用调整不独立成册，只作为对账单调整行存在，理由是 X」。**若走这条，CONTEXT 那两句
要改**——「不能代替」现在读起来就是禁止这个安排。

## 边界

本票**不改表、不建列、不写迁移、不动领域模型**。不构成票 04 的阻断。表当前 0 行，无论走哪条路
都不需要数据清洗；补列须新开序号文件，不改写已施加的迁移。

## Comments

- 2026-09-01 · MCP-1：**按路二实施完毕（ADR-0087 决定二），机制半边收口。** 本票上文
  「客户侧两条路一条都没有」自此描述的是补齐**之前**的状态。

  - **库**：迁移 `0015_customer_charge_adjustment_register.sql` 立 `charge_adjustment`。
    主键（租户+调整）一次成形；`kind` 封闭在 `PRICING_CORRECTION`/`COMMERCIAL_CONCESSION`；
    依据按种类分格且有此无彼；三件组两道门与 `customer_charge` 同款。`charge_id` 以标识
    引用不设外键，照本模块既有做法。
  - **端口与适配器**：`ports.ChargeAdjustmentStore`（幂等键 + 只追加写入代数，同 ADR-0031）；
    postgres 写口 `ON CONFLICT DO NOTHING` 答`已登记`，读口重建时走 `FormChargeAdjustment`
    而不旁路形成门——库上的 CHECK 与领域形成门是同一组判据的两份，任一份先松掉当场露出来。
  - **应用**：`RecordChargeAdjustmentHandler`（UC-SA-002）。调整只挂已确认费用，预估/暂估
    停专格 `CHARGE_NOT_CONFIRMED`；形成时点由时钟给出，不由调用方声称；重复触发返回原结果；
    册答`已登记`却读不回原件时答未决，不拿刚形成的那份冒充原件。原费用的写口一次都不碰。
  - **唯一创建用例门**：`internal/architecture` 新增
    `TestOnlyTheOwningUseCaseReachesTheChargeAdjustmentRegister`，守住编排包内只有
    `record_charge_adjustment.go` 够得着 `ChargeAdjustmentStore`。

  **为什么门不落在种类封闭集上，也不落在一列 `created_by_use_case` 上**（这一段是本次
  实施里唯一的新裁量，记下来免得日后重走）：种类封闭集管的是「册上表达得出什么类型」，
  而 CONTEXT「调整类型与唯一所有权」表通篇管的是「谁能写」——`UC-SA-003` 只要依赖那个端口
  就能写一条完全合法的 `PRICING_CORRECTION`，CHECK 与领域都会收下。`created_by_use_case`
  列的值由写入方自己填，与决定一里被否决的「七项进命令」是同一个毛病，且今天只可能有一个
  取值，是个看着像门却什么都不守的列。Go 没有 friend 可见性、编排又同在一个包，同包所有权
  只有扫源码守得住，故落成文件级闸门；门禁自带「这门真能红」的合成用例，照本包约定。

  **未做**：`customer_statement.adjustment_lines` 尚未退回「纳入关系」的角色——册立起来了，
  但对账单那一列还照旧，属另一片。管理台费用页的调整栏同理可按册加回。生产装配未做：
  `NewRecordChargeAdjustmentHandler` 至今只在测试里被调用，未进 `cmd/parcel-api`。

  **验证**（父提交 `a573551`，本笔改动尚未提交时实测）：`gofmt -l` 无输出、`go build ./...`、
  `go vet ./...` 绿；`go test -count=1 ./...` 全仓 89 包 ok、0 FAIL；
  `settlementaccounting/adapters/postgres` 单跑 `-v` 得 108 PASS / 0 SKIP / 0 FAIL，SKIP 为零
  即证明不是未设 DSN 跳过冒充的绿。实施过程中被既有门禁拦下两次，均照其指示处置：写口缺
  `ErrTransactionRequired` 负向证据（补断言，原先只写 `err == nil` 等于没测）、接线棘轮报
  `FormChargeAdjustment` 已走出基线（按门禁要求先搜同名声明排除误判，确认是真接上生产
  调用路径，剪掉基线该行）。

- 2026-09-01 · MCP-5：**重新取证于 `d11e0f0`，票面结论一字未变。** `customer_charge` 仍无
  `version`／`prior_version`／`correction_reason`，主键仍是（租户, 费用标识）一条费用一行；
  `settlement_accounting` 下也仍没有客户侧调整明细册。供应商侧那条对照（`supplier_expected_cost`
  的整条版本链）同样照旧。本票仍 `draft`，待裁三选一原样有效。
