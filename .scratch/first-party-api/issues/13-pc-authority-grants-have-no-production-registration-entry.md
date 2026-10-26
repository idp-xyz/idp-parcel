# 13 PC 授权册的授予没有生产登记入口，资料修订口在任何租户上都答未配置

Category: enhancement
Status: resolved——2026-10-26 进 main `2d9d87715`（见文末「进 main 记录」；票 10 据此补演示一笔）。此前 in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-fpa13（代码最后一笔 6918deeac）。此前：in-progress——2026-10-26 会话 `tops-20`（Claude Code）立并认领，分支 `claude-fpa13`。R1（上线前必修：用户 2026-10-26 定首发要资料修订口，见 [spec](../spec.md)「五口上线」）。
Blocked by: 无
出处：票 [10](./10-customer-source-data-amendment-face.md) 作者读码发现：`AuthorityGrantStore.SaveGrant` 非测试调用方为零（`ports.AuthorityGrantRegistered` 注释自承「今天没有任何生产编排发这一格」）；发布批 `AUTHORIZATION_RULE` 只带 `cancellationAuthority` 声明，`DeclarationChannel` 里没有授予通道；于是种子登不进任何授予，PC 对每次资料修订都答 `ErrAuthorityRulesNotConfigured`。

## 做什么

- PC 侧给授权册的授予一个生产登记入口：照该上下文既有的登记/发布写法选一条（发布批新增授予通道，或 `cmd/parcel-commercial` 子命令），带重放与内容冲突代数（照既有登记口），迁移若需要照规矩取号。
- 授予形状要能表达票 10 / ADR-0311 定的直客自助修订等级常量（法人、等级、范围逐字匹配）。
- 演示种子：给 `SYN-TENANT-01` 登修订授予（等级取 ADR-0311 的常量），并在发布批 `sourceDataAmendment` 允许矩阵补演示要走的意图格（补充、显式清空等，现只登了 `CORRECTION`）。
- 租户取值不写死：没登授予的租户如实答未配置。

## 完成判据

- [x] 生产入口能登授予，重放与冲突代数有用例（含真库）。
- [x] 演示种子 `seed.sh` 退 0，登进去之后 PC 对演示客户端的修订授权判断答允许（真库用例）。
- [ ] Go 门禁；全量用 `scripts/test-full.sh`（作者未跑全量，留评审/协调方）；新 AT 与盘点表行（已补，AT-PC-063/064）。

## 规模

M

## 完成记录（2026-10-26，作者）

- **起点** `2b4cb1f29`；**代码提交** `6918deeac`（分支 `claude-fpa13`，已推）。
- **入口选发布批声明通道**（不走子命令）：授予在库里的主键就是所挂授权规则版本（`authorization_grant` 主键 租户/对象/版本），生命周期与那一版同一（ADR-0261 决定一的截止/退役折法），与既有取消授权目录、合同委派同写法，同一发布同一事务，重放与冲突代数复用既有。子命令会把一个版本的两半拆在两个入口。
- **授予形状**：`CommercialDeclarations.AuthorityGrant`（动作、责任法人、等级、范围、有效区间），批文键 `declarations.authorityGrant`；一版一条；等级逐字串，PC 与翻译层不认任何值。新端口 `PublicationRegistry.SaveAuthorityGrant`，与授权册 `SaveGrant` 共用 `insertAuthorityGrant`；通道 `AUTHORITY_GRANT`，登记成功交 `AuthorityGrantRegistered`（原注释「没有生产编排发这一格」已改）。
- **迁移**：无（表与动作 CHECK 已含 SOURCE_DATA_AMENDMENT，0025/0032）。
- **顺手修的真缺陷**：授予行内容摘要原盖整份快照（含版本壳发布/取效时刻），同一批文重放会答内容冲突（整批种子重放首次撞出）。摘要改盖版本身份加五格授予内容；此前无生产调用方，无存量，无迁移。
- **种子**：`publish-batch.json` 新增 `SYN-AUTH-SOURCE-AMEND-01/v1`（动作 SOURCE_DATA_AMENDMENT、法人 SYN-LE-01、等级 `CUSTOMER_SELF_SERVICE`、范围 SYN-SCOPE-01、自 2026-01-01，批准引用 SYN-APPROVAL-2026-022）；`sourceDataAmendment` 矩阵在既有一格后追加：ACCEPTED_NOT_YET_RECEIVED 的 SUPPLEMENT/EXPLICIT_CLEAR 允许，CUSTOMS_SUBMITTED 的 SUPPLEMENT 允许、CORRECTION/EXPLICIT_CLEAR 不允许；规则包摘要照算出摘要重算。`publish_answer_test.go` 的整批回显（19 到 20 项）同步。
- **代决与越权风险点**：见 ADR-0312（授予不进规范化正文、摘要盖不住授予，故同版再带取消授权目录整项拒；一版一条；摘要口径改动）。另：矩阵里「已提交」取 `CUSTOMS_SUBMITTED` 并用 DISALLOWED 演示不允许，阶段/意图取舍请票 10 评审核对；`docs/adr/README.md` 由 `tools/adr-index` 生成（未手改）。
- **判据对用例**：入口重放/冲突/无事务拒 `TestAnAuthorityGrantPublishedThroughTheDeclarationChannelReplaysAndConflicts`（真库）；编排/交接/同版拒 `TestAnAuthorityGrantPublishesWithItsRuleVersion`；批文译装 `TestAnAuthorityGrantTranslatesItsFiveDimensions`；登后 Authorize 答允许、异等级不允许、无授予租户未配置 `TestAPublishedSourceDataAmendmentGrantAllowsItsLevelAndLeavesOtherTenantsUnconfigured`（真库）；整批种子首发/重放 `TestTheControlledPublishAnswersTheDemoSeedAlike`（真库）。新 AT：AT-PC-063/064（UC-PC-001），盘点表已补行。
- **自验**：`go build ./...`、`go vet ./internal/partycommercial/... ./cmd/...`、gofmt 干净；带 DSN `go test -count=1 -p 1 ./internal/partycommercial/... ./cmd/parcel-commercial/... ./cmd/parcel-api/... ./internal/architecture/... ./migrations/...` 全绿；一次性库 `fpa13_seed` 跑 `seed.sh --reset` 退 0、20 项全 PUBLISHED_EFFECTIVE、授予行读回正确，已删库；`at-coverage-counts -check` 只报生成物过期；`adr-index -check` 过。未跑全量。

## 评审记录

评审 ← tops-20 隔离子代理(sonnet) · 钉 `b4195f3b6`：阻断无（重点项「一版一条授予」核实不阻断：`LoadEffectiveGrants` 按（租户、范围、时点）筛、`Authorize` 按动作/法人/等级/范围逐字命中，不同（法人、范围）的授予挂各自的授权规则对象可同时生效；法人与范围相同的直客共用一条授予）。非阻断：1. 运营代价（每个组合一个对象；同组合换版须先截止或退役旧版，否则命中 2 条答适用冲突）——修订轮写进 ADR-0312 越权风险点 2。2. 演示种子 `CUSTOMS_SUBMITTED` 更正与显式清空不允许是设定、非文档推出——保留作演示被拒的一路。另：协调方据票 10 作者报告补演示允许矩阵三组（`SENDER_PLACE`、`DELIVERY_PLACE`、`DECLARED_MEASUREMENT`），修订轮收。无发现：摘要只去发布与取效时刻、不同内容不会判同一份；授予与取消授权目录同版并存才被拒、正常路径不撞；登记成功交 `AuthorityGrantRegistered`；等级不写死；号不撞。

## 进 main 记录（2026-10-26，推送方会话 tops-20）

- **main**：`2d9d87715`。出处：分支 `claude-fpa13` tip `594584b0c`（作者为 Sonnet 子代理；修订轮只收非阻断，推送方读了修订 diff）。无迁移；ADR-0312。
- **生成物**：三件推送方重生成，`-check` 退 0。
- **全量真库测试**：`scripts/test-full.sh` 跑在 `2d9d87715`：158 ok / 0 FAIL。**种子**：一次性库灌入退 0（发布批 20 项全生效），用后删库。
- **号**：`AT-PC-063`、`064`。
