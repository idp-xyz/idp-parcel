# 25 受理前估价由结算形成：采用登记、按提交版本的逐件 SELL 试算、估价留痕与续办触发（ADR-0179 的实施）

Category: enhancement
Status: ready-for-agent——2026-10-12 20:4x 通道 1：[ADR-0179](../../../docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 已接受（用户授权通道 1 代裁）。接受时改了两处，都不动下面的「地盘」，「做什么」第 4 条与完成判据已照改：决定二多一格停法（任一份合计的币种不是控制作用域的币种也答未形成，结算不换算），决定六的留痕多存每份试算评价的版本清单；PP `CONTEXT` 那两句随接受已改，从本票的文档一格划掉。此前 needs-info——2026-10-12 通道 3 随[票 06](./06-ps-acceptance-and-label-selection-judgment-methods.md)第 2 项分诊立（派单 `task-85d7350d` ← 通道 1），等用户决定是否接受 ADR-0179
Blocked by: [24](./24-plan-given-estimate-evaluations-use-shared-reading-completion.md)（ADR-0179 已于 2026-10-12 接受）
地盘（按 ADR-0179 现稿）：
- `internal/settlementaccounting`：估价端口、采用登记册与迁移、施加编排在要金额时先形成估价、估价留痕；`adapters/parcelpricing` 下新的受理前试算消费侧适配器。
- `internal/parcelpricing`：「按委托提交版本形成受理前试算」入口；读 PS 提交版本申报的消费侧适配器。
- `internal/parcelshipment`：按（租户、委托、提交版本）作答的逐件申报测量与收寄邮编读口；`adapters/settlementaccounting` 的控制请求去掉金额；`adapters/parcelpricing` 的 `estimation_amount.go` 与其用例退场；收结算登记信封的续办门。
- 装配：`cmd/parcel-dispatch`（`acceptanceFinancialControl` 与续办门）、`cmd/parcel-api`（只接释放半边的几处装配注释）、`cmd/parcel-settlement-register`（登记命令）。
- 文档：UC-SA-002 输入表「财务控制请求」一格；CONTEXT-MAP 补 SA → PP 受理前试算、PP → PS 提交版本申报读口两条关系；开发主线 PN-02 行带日期的补记；参数登记册 `PAR-COM-15` 行「〔ADR-0146 拆分〕」一句收短。
出处：票 06「第 2 项分诊」；ADR-0179 Decision 与 Consequences。

## 做什么（按 ADR-0179 现稿，分笔提交）

1. **PS 读口。** 按（租户、委托、提交版本）逐件答申报测量与收寄两端邮编，不经接受基线。ADR-0171 的申报测量与地址要素读口只经接受基线的声明成员走，受理前答「无」，不拓宽它们，另开一口。
2. **PP 受理前试算入口。** 按 ADR-0179 决定三解析在用 `SELL`·客户费用价卡（`PriceCardInForceResolver`，时点取控制回显的判断时点），逐件用第 1 条读口的申报造声明，交[票 24](./24-plan-given-estimate-evaluations-use-shared-reading-completion.md)的入口形成试算评价。结算币种由调用方交入。不入册、不交付。未配置、适用冲突、缺件各自成格，不挑卡、不顶值。
3. **SA 采用登记。** 新册：键（租户、结算账户），值（形态、计价范围），不设修订（照 ADR-0158）。迁移、`parcel-settlement-register` 子命令；登记与登记行同一事务发「参数已登记」类信封（ADR-0094 决定四）。首版形态只有「逐件 SELL 试算合计」一种，词表外的形态登记时拒。
4. **SA 形成估价。** 施加编排在策略要求冻结或信用校验时，先按采用行经估价端口要试算，按 ADR-0179 决定二的停法合计；同一金额用于本次各控制项。币种取控制作用域，最小币单位按 ISO 4217（PP 领域 `Currency.MinorUnitScale`）。册上没有这一户时答未形成，原因指名金额未配置；估价随控制结果留痕，含每份试算评价的版本清单（决定六）。
5. **PS 控制请求去掉金额。** `ControlAmountSource`、`EstimationAmountSource`、`EstimationBasisSource` 退场。`acceptanceFinancialControl` 里那句「Amounts 留空：估价缝属实例半边，见 acceptanceChainConsumer 的注释」随之消失（票 06「顺带」点名它指向一段不存在的注释）；`ControlAmountSource` 与 `NewPreAcceptanceControlAdapter` 头注里「接通前属实例半边」一类说法一并改成机制说法。
6. **金额那一格的续办。** 按 [routing-first-cut/19](../../routing-first-cut/issues/19-financial-control-undecided-leaves-no-trace-on-the-task.md)「分诊」拆出的「未配置」端口格与未决原因，把金额未配置挪进`等待运营登记`，与第 3 条的信封、PS 新续办门同一笔落地。19 若还没落，本票先按 19 的分诊拆那一格，19 接着用。
7. **文档。** 地盘里列的那几处一并改，开发主线与登记册只追加补记或收短，不改写旧单元格。

## 不做

- 不登任何租户的行，不预选形态；演示租户的采用行、售价卡绑分区目录、演示委托带邮编都归[票 26](./26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md)。
- 不加第二种估价形态，不碰选卡走价格政策绑定那条路。
- 不改释放路径：释放按原关联认领，不问金额。

## 完成判据

- 空册：控制停在金额未配置；委托进`等待运营登记`，处理尝试落库，信封入账而不是 `ABANDONED`。登记采用行之后，信封重驱同一委托，越过金额格。带 DSN 的真库用例，写明 `-v` 下为 `PASS` 不是 `SKIP`。
- 绑燃油序列的卡、多件委托：冻结金额等于逐件试算合计；任一件缺测量或邮编、任一份评价没有合计、任一份合计的币种不是控制作用域的币种时不冻结，答未形成。
- 估价留痕可复算：同一输入、同一版本清单重算出同一合计。
- 释放路径的既有用例绿。
- 架构门禁绿；机制清点重生成并提交；带 DSN `go test -p 1 -count=1 ./...` 绿。
