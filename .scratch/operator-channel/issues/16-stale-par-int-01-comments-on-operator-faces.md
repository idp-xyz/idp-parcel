# 16 过期注释：运营侧接入面的认证不再写「属 `PAR-INT-01` 待提供」

Category: enhancement
Status: resolved——2026-09-25 通道 4 做完并进 main（完成记录见文末 Comments）。此前 in-progress——2026-09-25 通道 2 崩后由通道 4 接手（用户令独立接手）；前半 22 个 Go 文件的注释改动已随 `70107171` 原样入库，余量见通道 1 的清单（登记写面各 `register_*.go` 与 `registrationjson`、目录查阅用例、一线作业事实与外部结果各口、`internal/platform/httpapi/router.go`、管理台 `apps/admin-web/src` 下带 `PAR-INT-01` 的运营侧注释）。此前 in-progress——2026-09-25 通道 2 随 ADR-0151 补立并认领（通道 4 于 2026-09-24 已改了一部分，未提交）
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md)
地盘：`cmd/parcel-api` 与各上下文 `adapters/http`、`adapters/registrationjson` 里的 Go 注释；只改注释，不改代码与测试逻辑。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定一（`PAR-INT-01` 只管客户生产委托接入渠道）；[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)；[ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) Consequences 第三条。

## 为什么

代码注释里运营侧各口的认证仍写着「属 `PAR-INT-01` 待提供」或「实例半边」。自 ADR-0100 起，`PAR-INT-01` 只管客户渠道，运营侧各族的渠道形态都归产品。这类注释会让读代码的 agent 以为这些口在等租户或外部证据，而它们等的是自家的操作者渠道票。

## 做什么

运营侧各口的注释改为指向各自的渠道族：

- 登记写面、目录与运营查阅面、商业发布：操作者渠道（ADR-0100）。
- 一线作业事实：操作者渠道的「作业事实登记」能力面（ADR-0149）。
- 外部结果与外部资金事实：集成客户端族（ADR-0149）；哪家监管或报关来源送回执，仍是租户取值 `PAR-INT-03`。
- 委托侧运营决定口与 TF 管理台上的决定与判断口：操作者渠道的「运营决定」能力面（ADR-0151）。
- 各包「未配置」哨兵：按提交方分族，各族真 Intake 都未就位。

## 不做

- 客户侧各口（提交、撤回、取消、补充、资料修订、客户追踪视图、索赔）照旧写 `PAR-INT-01`，直到 ADR-0139 有结论；端点表客户侧两行只把「（实例半边）」换成指向 ADR-0139 草案的说法。
- `internal/accessidentity` 里客户渠道那一半的注释（`doc.go` 那一段归 [03](./03-operator-envelope-and-answer-algebra.md) 第 4 条）。
- `PAR-COM-14` 等授权请求映射的「实例半边」口径（归 [psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)）。
- 与在途分支 `mcp2-psb17` 重叠的文件。

## 完成判据

- 运营侧各口的 Go 注释里不再把认证归给 `PAR-INT-01` 或「实例半边」；剩下的 `PAR-INT-01` 都属客户侧或上列「不做」。
- `gofmt -l` 无输出，`go build ./...` 与 `go vet ./...` 退 0。

## Comments

### 完成记录 ← 通道 4 · 2026-09-25

**前半**随 `70107171` 原样入库（22 个 Go 文件，前一通道 4 会话与通道 2 所改）。**后半**本笔改 37 个文件（44 行），只改注释，四类照票面「做什么」：
- 登记写面：各上下文 `register_*.go`（CC 两份、NR、PP 两份、PC 五份、VE），以及 CC、SA、VE 的 `registrationjson/translate.go`。一律改指操作者渠道（ADR-0100），「`PAR-INT-01` 待提供」改为「其真 Intake 未就位」。
- 目录与运营查阅面：SA、TF 的 `catalogue_intake.go`；PS 的渠道择优决定、面单交易与委托查阅三口；VE 的运营追踪；六个查阅测试里「真通道未登记（PAR-INT-01）」一句，以及 PP、PC 三个测试的 Covers 行。一律改指操作者渠道（ADR-0100）。
- 一线作业事实：NO 收件，TF 的交接、交付与移动事实。改指操作者渠道的「作业事实登记」能力面（ADR-0149）；移动事实写明自营走作业事实登记、外部走集成客户端族。
- 外部结果与外部资金事实：CC 回执、SA 外部资金事实。改指集成客户端族（ADR-0149）；哪家来源送回执仍是租户取值 `PAR-INT-03`。
- 另两处：`internal/platform/httpapi/router.go` 的「逐端点等各自的 Intake」改为按族列出三族；`cmd/parcel-api/assemble_settlement_registration_test.go` 一处改为操作者渠道。

**照旧不改的**（取证于本笔：Go 代码里剩 30 个文件、40 行）：客户侧各口——提交、撤回、受控补充、资料修订、取消、理赔、客户追踪视图、端点表客户渠道两行、VE 登记 CLI 里的客户自助；`internal/accessidentity` 客户渠道登记册那几处；PS 领域与应用里的客户载荷词表；以及写对了的引用——PP、PC 发布载荷那两句「它不是客户渠道载荷，那一半照旧等 PAR-INT-01」、PP 复核口对 ADR-0101 收窄的转述、PC 发布草稿「不是等 PAR-INT-01 契约，是去接信封」、CC 申报权威读口里的客户渠道认证、PC 产品渠道登记的渠道本体。

**不在本票地盘、记为后续**：管理台 `apps/admin-web/src` 里运营侧页面注释中的 `PAR-INT-01`（通道 1 列过约 27 个文件，例如 `pages/shipment-request/api.ts` 三份决定草案写「由 PAR-INT-01 的接入面从已认证的操作员身份翻译」）。本票地盘只写了 Go 注释；前端走 workflow.md「前端切片」那条路，宜另立一票。

**验证**：`gofmt -l` 空；`go build ./...`、`go vet ./...` 过；机制清点在本笔检出上重生成、无差（只改注释不改计数）；进 main 前在最终 SHA 上跑带 DSN 全量（见提交信）。推送方即作者，自审。

