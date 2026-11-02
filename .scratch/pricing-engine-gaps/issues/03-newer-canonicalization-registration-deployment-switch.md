# 03 登记 PPC-6 价卡没有代码里的门——滚动发布时旧进程遇到新卡整笔失败，只靠文档约束

Category: enhancement
Status: in-progress——2026-11-01 作者完工，待隔离评审与进 main，分支 claude-ppc6gate（代码 `9441f27e6`，其后文档一笔）。
Blocked by: 无（接 pricing-engine-gaps/02 即 ADR-0348、price-card-import/08 即 ADR-0349，均已在 main）

## 现象

取证于 main `5907d4cde`（冻结清单 2026-10-29 两条「登记 PPC-6 价卡前要求显式部署开关」，后一条因导入能造出 `PPC-6` 而升级）：

1. 只认 `PPC-5` 的旧进程遇到登记册里的 `PPC-6` 价卡：试算整次答价卡读不出，评价与售价预估答 unavailable，租户级的序列与目录登记 / 复核整笔失败，波及 `parcel-pricing-feed` 的周更燃油序列（ADR-0348 越权风险点 6）。
2. 「第一张 `PPC-6` 登记之前所有读价卡的进程必须已是新构建」只是文档约束；导入编辑器上线后，任何持登记写授予的操作者都能经导入、批准、发布走到登记（ADR-0349 越权风险点 5）。

## 做什么

设计与取舍在 [ADR-0352](../../../docs/adr/0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md)（会话代 PP owner 与运维接受，待复核；派单已定，不再裁）。

1. 门在 `RegisterPriceCardHandler.Handle`、仓储 `Register` 之前：方案记的规范化版本不是基础版（`CurrentCanonicalizationVersion()`）且开关没开即答 `NEWER_CANONICALIZATION_NOT_ENABLED`，不碰库、不交「参数已登记」；已在册的同一版重放也拒。
2. 开关 `IDP_PARCEL_PRICING_REGISTER_NEWER_CANONICALIZATION`，恰为 `true` 才开；`parcel-api` 与 `parcel-pricing-register` 装配各读一次，经 `RegisterPriceCardDeps` 注入。
3. HTTP 沿既有登记答复加这一码；草稿发布被拒时草稿留在已批准、可重试；CLI 加退出码 4 与帮助文字。
4. 管理台补这一码的中文业务话；种子登记 FULL-03 那一步设开关。
5. ADR-0348 风险点 6 加前向指针，UC-PP-001 与 AT 盘点补行。

## 完成判据

- 开关关 / 开 × 基础版 / 较新版四格；重放；批准发布被拒后重试；CLI 退出码；HTTP 答复形状；门在 `Register` 之前（假仓储与假交接口断言没被叫到）。
- 真库：在线登记编排与草稿发布编排、受控 CLI 各对种子 FULL-03 走关 / 开两格。
- 基础版的卡与既有测试行为不变；一次性库灌种子退 0。

## 完成记录（2026-11-01 作者）

起点 `5907d4cde`（origin/main）；分支 `claude-ppc6gate`。提交：`9441f27e6`（代码、测试、管理台、种子），其后文档一笔（ADR-0352、ADR-0348 / 0349 指针、UC-PP-001、AT 盘点、本票面）。

**改了什么**
- `internal/parcelpricing/application/register_price_card.go`：结果代数加 `PriceCardRegistrationNewerCanonicalizationNotEnabled`（`NEWER_CANONICALIZATION_NOT_ENABLED`，追加在末尾，既有各格数值不变）；`RegisterPriceCardDeps.NewerCanonicalizationEnabled`（零值即关）；门在交接口装配检查之后、`Catalog.Register` 之前。
- `approve_and_publish_price_card_draft.go`：只改注释（`发布未落定`的成因多列一格）；发布对这一格的处理就是对冲突、不可比的那条既有分支，没加代码。
- `cmd/parcel-api`：`pricingRegisterNewerCanonicalizationEnv` 与读法放在 `assemble_pricing_registration.go`；`run` 里读一次并打启动日志，经 `startupChannels` → `endpointSources.registerNewerCanonicalization` 交给 `buildPriceCardRegistrationOrchestration` 与 `buildPriceCardDraftPublication`（两函数各多一个布尔参数）。
- `cmd/parcel-pricing-register`：包注释与 `-h` 写明开关与退出码；`exitNotEnabled = 4`；`assemble(db, bool)`；未启用时 stderr 多一句怎么开。
- `apps/admin-web/src/pages/pricing/api.ts`：`registrationOutcomeLabels` 补这一码的中文业务话（登记口与发布口的登记那一格共用这张表）。
- `scripts/demo-seeds/steps/165-…sh` 只对登记 FULL-03 那一次调用设开关，带中文注释；`demo-run/up.sh` 起进程时设开关（同一次编译、没有旧构建在读），日志过滤多认一行；种子 README 那一行补一句。
- 测试：应用层新文件 `register_price_card_gate_test.go`（四格、重放、发布被拒后重试）与夹具 `totalBoundedPlan`、`registrationFixtureOf`、`approvedPriceCardDraftOf`（既有夹具改为委托给它们，行为不变）；HTTP 两张表各加一行；`cmd/parcel-api` 新文件 `assemble_pricing_register_switch_test.go`（环境读法、真库登记编排关 / 开 / 已在册后关着重放），`TestAFormDraftOfTheNewStructuresIsApprovedAndPublishedAsPPC6` 先在关着的编排上发布一次（未落定、草稿仍已批准、册上无卡）再在开着的上落定；`cmd/parcel-pricing-register` 新文件 `register_switch_test.go`（退出码表、真库关 4 / 开 0 / 基础版关着 0）；管理台 `price-card-import.test.ts` 加一条。既有 PPC-5 的装配测试传 `false`，等于顺带证了基础版在开关关着时照常。

**自验**（`HTTPS_PROXY` 与 DSN 照派单）
- `go build ./...`、`go vet ./internal/parcelpricing/... ./cmd/...` 退 0；改过的 `.go` 跑 `gofmt -l` 无输出。
- `go test -count=1 -p 1 ./internal/parcelpricing/... ./internal/platform/... ./internal/architecture/... ./cmd/...` 全 ok（含 PG，DSN 指 55432）。
- `apps/admin-web`：`npx tsc -b` 无输出，`pnpm test` 983 pass / 0 fail。
- 种子：一次性库 `seed_ppc6gate_tmp` 上 `scripts/demo-seeds/seed.sh` 退 0，FULL-03 那步 `price-card: RECORDED`；同库不带开关再跑一次 FULL-03 → `NEWER_CANONICALIZATION_NOT_ENABLED`、退出码 4、stderr 那句提示；库已删。
- `go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 只报生成物不一致（派单要求作者不重生成）；试跑生成器确认 ADR-0352 索引标签为〔代决·待 PP owner 复核〕、AT 盘点没有缺行多行，随即还原。

**判据对用例**
- AT-PP-048 → `TestRegisteringANewerCanonicalizationNeedsTheDeploymentSwitch`（四格，假仓储与假交接口断言不被叫到）、`TestAReplayOfANewerCanonicalizationIsRefusedWhileTheSwitchIsOff`、`TestTheNewerCanonicalizationSwitchOpensOnlyOnTrue`、真库 `TestTheWiredPriceCardRegistrationHonoursTheNewerCanonicalizationSwitch`、CLI `TestTheNewerCanonicalizationRefusalHasItsOwnExitCode` 与真库 `TestTheControlledRegistrationOfANewerCanonicalizationWaitsForTheSwitch`、HTTP `TestRegisterPriceCardEndpointTranscribesAnswersVerbatim`。
- AT-PP-049 → `TestAPublicationRefusedByTheSwitchCanBeRetriedOnceItIsOn`、真库 `TestAFormDraftOfTheNewStructuresIsApprovedAndPublishedAsPPC6`、HTTP `TestThePublicationEndpointHandsTheRegistrationAnswerThrough`、管理台 `price-card-import.test.ts`。

**本票层面的代决**
1. **ADR 号取 0352 而不是 0350**：origin/main 最大 0349，各 `claude-*` 分支没有 035x；但主工作树里另一会话未提交的文件已占 `0350`、`0351`（法人档案两份），避开。
2. **种子只在 165 一步设开关，不在 161**：派单写「FULL-02/03」，实测 `price-card-cn-sg-full-02.json` 记的是 `PPC-5`（读地址性质的卡「也写 PPC-6」仍是冻结清单里的待做），只有 FULL-03 是 `PPC-6`；161 不设，好让开关若误伤基础版时种子先红。
3. **`demo-run/up.sh` 设开关**：派单只点了种子步骤；up.sh 起的本地演示进程出自同一次编译，不设的话管理台编辑器做的 PPC-6 卡发布会被拒。共享预览环境（`:19080`）没碰。
4. **ADR-0349 越权风险点 5 记为「由 ADR-0352 回答，无条款停用」**：它只说开关「本记录不做」，没有可停的条款；照 adr/README 的回答写法挂在 ADR-0349 注记与 Links。
5. **AT 号取 048、049**：UC-PP-001 下一个空号；两行放在「价卡导入」之后新开一节。

**留给协调方**
- 共享预览 `parcel-api` 要加 `IDP_PARCEL_PRICING_REGISTER_NEWER_CANONICALIZATION=true`，否则重灌种子或在管理台发布 PPC-6 卡会被拒。
- 冻结清单 2026-10-29 两条里「登记 PPC-6 价卡前要求显式部署开关」进 main 后可标已处理；同条其余各格（试算费用行方向、导入暂不支持等）不在本票。
- 进 main 时重生成 ADR 索引、AT 盘点计数（新增 2 行，均入①）与机制清点。
- 冻结清单里「读 `ADDRESS_NATURE` 的方案也写 PPC-6」若日后落地，FULL-02 会变成 `PPC-6`，届时 161 那步也要设开关。

## 修订轮（2026-11-01 作者，应协调方转来的隔离评审意见，评审钉 `a074aee72`、Standards 阻断 1 条）

- **阻断（已修）**：ADR-0101 决定五列举答案代数六格「一格不改」，本票加了第七格，属部分停用。ADR-0101 `Status` 行索引注记与 Links 各加指向 ADR-0352 的前向指针（被停的是六格列举那一句），正文不动；ADR-0352 前段去向补「部分停用 ADR-0101 决定五一句」，决定三标题注明，Links 里转述 0101 的那句改掉。
- 非阻断 1：ADR-0352 决定一、Alternatives 与 `assemble_pricing_registration.go` 注释里的跨文件计数改成按名列举或去掉数字；ADR-0348 / 0352 里指 0348 风险点 6 被停句子的「三句」改成「那几句」。
- 非阻断 2：ADR-0352 越权风险点补第 6 条——开关关着时导入仍能把 `PPC-6` 草稿存进 `price_card_draft`，滚动窗口里旧构建 `ListPriceCardDrafts` 遇错整批返回会让该租户草稿列表整页失败；本门明示不管，由发布顺序兜。
- 非阻断 3：`docs/design/synthetic-demo-journey-script.md` 起后端那段环境变量补上开关（共享预览进程仍由协调方改）。
- 复验：`go build ./cmd/parcel-api`、`gofmt -l` 无输出；`adr-index -check` 仍只报生成物不一致，试跑生成器标签不变后还原。
