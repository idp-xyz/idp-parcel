# 12 customs-compliance：按路由候选作答的关务适用性判断口

Category: enhancement
Status: in-progress · 完工待评审与重放——2026-10-09 通道 3 收口（派单 `task-5351ee37` ← 通道 1）：代码笔自基线 `993995e7` 至 `0c17d9b5`，清点 `6e441e43`，完成记录见文末；评审由通道 1 另派非作者，作者不自评。此前：in-progress——2026-10-08 TraeCode 会话认领（用户令「开始接下一张票」；阻塞均无，文末分诊裁定即本票口径）。此前：ready-for-agent——2026-10-08 TraeCode 会话按用户令代 CC owner 分诊，「待 CC owner 定」四问裁定见文末「关务适用性判断分诊」。更早的 needs-triage——2026-09-24 通道 3 立（派单 `task-1f910231` ← 通道 1；出自 02 的取证与 [psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 格 6）。建在 [ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)（Proposed）决定三上，判断口的形状与作答层级归 CC owner（0148 越权风险点 4）：0148 接受、CC owner 在分诊时定下文「待 CC owner 定」各问之后，转 ready-for-agent
Blocked by: 无（原 02、07 均已进 main；接 NR 取数侧那一项的地盘另含 NR owner，轮次 4 前报窗口）
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「演示租户上一票已接受的委托能形成初始路由」那条关键路径上挡路的缝（切片计划的子票表由通道 5 补入）
归档：psb/04 与 ADR-0148 决定三都写这张票「另立、不在本票族里补」，指的是 NR 取数侧各票不代 CC 补执行器；本票就是那张另立的票，地盘在 CC。放在本目录、编号 12 是派单的归档选择（通道 1 与通道 5 协调）。
地盘：customs-compliance 的领域、应用与端口（新判断口）及其读侧适配器；customs-compliance `CONTEXT.md` 相关词条与规则（经 CC owner）；「接到 NR 取数侧」那一项另含 network-routing 的 `adapters/customscompliance` 消费方适配器。
出处：ADR-0148 决定一（端口取回的事实带出处、与判断一并留痕）、决定三与越权风险点 4；[CONTEXT-MAP](../../../docs/domain/CONTEXT-MAP.md)「customs-compliance ↔ network-routing」；customs-compliance [`CONTEXT.md`](../../../docs/domain/customs-compliance/CONTEXT.md)「Boundaries and ownership」「本上下文拥有合规候选区域、口岸、申报路径和关务适用性判断」；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二、三。

## 现状

CC 有口岸目录与申报路径目录两本登记册（`ports.PortsPathsRegistry` 写、`ports.PortsPathsView` 按时点点读），没有按路由候选作答的判断口。口岸目录只登口岸标识与生效区间，`CandidatePortEntry` 的注释写明「所属区域与适用性判断都不在册（区域维未建模，适用性是判断链的产物）」；申报路径是口岸、方向、申报模式三维。CC `CONTEXT.md` 里也还没有「关务适用性判断」这个词条。ADR-0148 决定三定：补上之前，NR 取数侧对候选的关务一格答状态未知，初始路由停在路由判断未决。CN→SG 的候选都含关务段，所以它挡在演示动线上。

## 产品与租户的分界（ADR-0146 决定二）

- **产品策略**：判断方法——候选要过关务边界时，对照该租户在判断时点在册的口岸与申报路径（以及裁定并入的限制）逐候选作答；判断口的形状——输入的候选关务投影、答案代数、出处字段；「不可用」与「状态未知」分得开，状态未知不折成可用。
- **租户取值**：该租户的口岸目录与申报路径目录内容（用哪些口岸、走哪种方向与申报模式）、内部合规限制实例、报关服务方的选择、采用哪一版参考配置。
- **参考配置**：若判断要用公开标准的实例数据（例如某国公开的口岸代码及其所属关务区域），按 ADR-0146 决定三出参考配置、显式采用才生效；采用路径按 ADR-0147（通道 4 在票 [psb/03](../../product-strategy-boundary/issues/03-reference-configuration-adoption-pattern.md) 中定，待重放进 main）。首版随附哪些，归 CC owner 与用户定。

## 待 CC owner 定（分诊时）

1. **作答层级**：答到区域、口岸还是申报路径。答到区域要先建模区域维——今天口岸目录没有它。
2. **候选关务投影由谁组装、含哪些格**：NR 按候选段链与目录折出（节点与口岸的对应落在 NR 目录还是 CC 目录），还是 CC 按 NR 交来的段链自行判断跨境点。这一问牵动 NR 目录内容列（07）与 CC 目录的边界，需与 NR owner 一起定。
3. **答案代数**：至少分得开可用、不可用（带理由）与状态未知；CONTEXT-MAP 那条边写关务提供「合规候选区域、口岸、申报路径、限制及解除结果」——内部合规限制的覆盖并入本判断，还是另答。
4. **留不留判断记录**：作答落成 CC 的版本化判断（同「合规判断」词条：带规则版本、依据、决定方式），还是只由读口即时作答、出处由 NR 随路由判断留痕。

## 做什么

1. customs-compliance `CONTEXT.md` 补「关务适用性判断」词条与规则，按上面各问的裁定写；先改 CONTEXT，再改引用它的用例。
2. 领域与应用：按裁定的判断方法逐候选作答。口岸未登记或判断时点未生效、申报路径三维对不上 → 不可用并说清是哪一格；目录为空或依赖读不到 → 状态未知，不从「查无记录」推出可用（与 CC Rules「不得按无记录……推导无内部限制」同一纪律）。
3. 端口：按裁定的形状出判断口；答案带出处（判断标识、所依目录版本与规则版本），供 NR 随路由判断留痕（ADR-0148 决定一）。同一租户、同一时点、同一候选投影，答案一致。
4. 接到 NR 取数侧：network-routing 的消费方适配器调用本判断口，把 07 里候选关务一格的状态未知换成真答；证据结构的出处字段照 07 已定的形状。
5. 若裁定随附参考配置：按 ADR-0147 的采用路径出一份样板，演示租户显式采用；不随附就不做这一项。

## 与 psb/10 的分工

[psb/10](../../product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) 是 CC 的申报侧：申报发送通道、公开监管结果代码与法规税则等参考配置、法定义务目录、立案与提交的触发面。本票是 CC 对路由的答复口：按候选的关务适用性。两票不共用可执行项。相邻的只有一处：本票若随附公开口岸与关务区域的参考配置，与 psb/10「公开法规规则源、税则与税费版本」那一项的按地区参考配置走同一条采用路径，但各出各的配置，不合成一份。

## 不做

- 不定任何租户取值：口岸与申报路径目录内容、区域归属、限制实例、报关服务方；不预置默认口岸，目录为空也不答可用。
- 不做申报就绪判断（那是按申报范围的判断，CC 已有），也不做 psb/10 各项。
- 不改 NR 的候选生成、排序、冻结与改路（03–07、09、10）；改路改变关务区域、口岸或报关服务方时重新请求判断，由 NR 那几张票负责发起，本票只保证判断口不给「沿用上次答案」的捷径。
- 本票不附带 ADR；判断口形状若经 CC owner 判为难逆转取舍，由其另议。

## 完成判据

- [x] CC `CONTEXT.md` 有「关务适用性判断」词条与规则，出自 CC owner 的分诊裁定。
- [x] 真库用例：合成口岸与申报路径目录上，候选得出可用、不可用（口岸未登记或未生效、申报路径三维对不上；限制若并入再加一条）与状态未知（目录为空、依赖读不到）各一；状态未知不折成可用。
- [x] 同输入重复作答结果一致，答案带出处。
- [x] 接到 NR 取数侧后，合成网络上含关务段的候选不再停在状态未知；psb/05 格 6 的取证据此更新（证据只记 `S`）。
- [x] 不进参数登记册；演示数据全为 `SYN-` 合成值。

凭据逐格见文末「完成记录」。

## 关务适用性判断分诊（2026-10-08，TraeCode 会话代 CC owner 裁定；CC owner 若在别处就位可 supersede）

**一、作答层级——口岸 + 申报路径两级，区域首版不建模。** 口岸与申报路径两本册已在（有生效区间与三维结构）；区域维今天没有建模（`CandidatePortEntry` 注释明说），要建先得定「口岸→区域归属」的数据来源，那正是 psb/10 的公开监管参考配置族（口岸代码与所属关务区域），不并进本票。作答粒度是候选级：这个候选含关务段时，能否用该租户判断时点在册的口岸与申报路径过关，不做段级口岸匹配。

**二、候选关务投影——由 NR 组装，形状 = 段链（现有 `CustomsCandidate` 形状）+ 两端国家/地区（寄件/收件国，ADR-0148 决定二的地理解析投影随请求携带、已到 NR 证据）。** CC 不读 NR 目录、不从节点猜跨境点；申报方向由两端国家对确定。某一侧缺国家码时，CC 按「证据不足」答状态未知（与决定二服务区域同一纪律）。这一格牵 NR owner——本分诊代裁，NR owner 可 supersede，落到轮次 4（消费方适配器）时眼见为实。

**三、答案代数——封闭三格加理由：可用 / 不可用（理由三格分别指：口岸未登记、口岸在判断时点未生效、申报路径三维对不上，方向与申报模式可各自指）/ 状态未知（目录为空或依赖读不到）。** 状态未知不折成可用。内部合规限制的覆盖与解除**不并入**本口：那属于既有限制与放行门禁判断链的答案（CONTEXT-MAP 那条边的「限制及解除结果」由既有机制另答）。不可用落到 NR 硬约束哪一格，由轮次 4 与 NR owner 定，本票不先裁。

**四、判断记录——首版即时作答，不落判断史库。** 出处 = 判断标识（按 租户 + 时点 + 候选 + 目录版本 铸成、可重算）+ 口岸/申报路径目录版本引用，NR 随路由判断留痕（ADR-0148 决定一）。CC「合规判断」词条的强制保存规则针对「明确申报范围」的判断链；本口是新族「关务适用性判断」，无申报范围，首版不并进那条强制——将来要争「当时为什么这么判」另立版本化判断史扩展票。同输入重复作答一致由目录版本可重放性保证（判据三）。

## 完成记录（2026-10-09，通道 3，派单 `task-5351ee37`；分支 `mcp3-rfc12` 基 `993995e7`；待评审与重放）

**落点**

| 笔 | 内容 |
|---|---|
| `c39d4d7f` | 票面：认领 |
| `6cb57bb9` | party-commercial 两文件补尾换行（gofmt，非本票语义）；已挑进 main 为 `187dccd2`，重放时跳过 |
| `4b967223` | customs-compliance：CONTEXT「关务适用性判断」词条与规则；领域按口岸 + 申报路径两级折三格，判断标识按 租户 + 时点 + 候选 + 目录版本 铸成；端口快照读形状；应用层即时作答、不落史库；postgres 全量快照读口；真库用例 |
| `1183f7cc` | network-routing：消费方适配器 `adapters/customscompliance`（投影带两端国家，三格译回硬约束，出处随判断留痕）；可达性与初始路由两条证据视图接上；迁移 network_routing `0017`（可达性判断记录加 `customs_citations` 列），初始路由计划 jsonb 带同形出处；`parcel-dispatch` 装配根换成真判断服务 |
| `0873a719` | CC：作答装配输入改为包内未导出类型，类型可达性棘轮转绿，行为不变 |
| `5b81e938` | CC CONTEXT 规则三、四改写（判断项 1） |
| `79614fd8` | CC：目录读不到时缺码与两端同国照常作答，读不到以 `CATALOG:UNREADABLE` 铸判断标识；对照检查 `customs_applicability_criteria_test.go` |
| `40db1d3d` | 判据四取证：`cmd/parcel-dispatch` 合成网络经生产装配取关务真答两条真库用例；NR 初始路由计划关务出处真库往返 |
| `0c17d9b5` | psb/05 格 6 补 rfc/12 取证，只记 `S` |
| `6e441e43` | 清点：在 `0c17d9b5` 的干净检出上重生成 |
| 本笔 | 票面：判据勾选、完成记录、Status |

**完成判据**

- ✅ CC CONTEXT 词条与规则：词条「关务适用性判断」与「### 关务适用性判断」各条规则落在 `4b967223`，规则三、四在收口时改写（`5b81e938`）。与分诊四问逐问对得上：口岸 + 申报路径两级、区域不建模、候选级作答（裁定一）；申报方向由两端国家对定，缺码答状态未知（裁定二）；答案闭合三格，内部合规限制不并入（裁定三）；即时作答不落史库，出处为判断标识加目录版本引用（裁定四）。
- ✅ 真库三格：`customs_applicability_criteria_test.go` 的 `TestApplicabilityCriterionRealCatalogAnswersEveryCell`（可用；不可用分口岸未登记、口岸未生效、进口一侧无路径、进口路径未生效；状态未知分目录为空与三种缺码；多个合成租户共一库，租户隔离一并受检）、`TestApplicabilityCriterionUnreadableCatalogIsStatusUnknownNotAnError`（依赖读不到）、`TestApplicabilityCriterionCellsNeedingNoCatalogIgnoreItsReadability`；另有 `4b967223` 的 `TestCustomsApplicabilityJudgesFromRealCatalog`。状态未知没有一格折成可用。限制按裁定三不并入，不加那一条。
- ✅ 同输入一致、带出处：`TestApplicabilityCriterionRepeatedJudgmentIsIdenticalAndBoundToItsInputs`（同输入逐项一致；换候选、换时点、出口路径换版后判断标识都变，换版后目录版本引用也变）、`TestApplicabilityCriterionEmptyAndUnreadableCatalogsCiteDifferently`；领域层 `TestSameInputFoldsIdenticalJudgments`、`TestJudgmentIDTracksCatalogAndCandidate`、`TestUnreadableFoldCarriesJudgmentIDWithoutVersions`。
- ✅ 接到 NR 后不再停在状态未知，psb/05 格 6 已更新（`0c17d9b5`，只记 `S`）：`cmd/parcel-dispatch` 的 `TestSyntheticCrossBorderCandidatesGetCustomsAnswersThroughProductionWiring`（`SYN-LINE-CN-SG-01@1` 关务事实 `SATISFIED`，出处带 CC 判断标识与 `PORT:`/`PATH:` 引用）与 `TestSyntheticNetworkCustomsAnswersFollowTheTenantsCatalog`（未登目录的租户 `STATUS_UNKNOWN`、缺口 `CUSTOMS_PORT_PATH_CATALOG_EMPTY`；缺进口路径的租户 `RESTRICTION_APPLIES`）。变异由本会话复现：在 `0c17d9b5` 的临时检出上把 `customsApplicabilitySource` 换回 `CustomsApplicabilityNotConnected`，两条全红（`STATUS_UNKNOWN`、缺口 `CUSTOMS_APPLICABILITY_NOT_CONNECTED`、没有出处），已还原、未提交。出处落库：可达性判断记录见 `TestReachabilityOverARealCatalogFormsEachOfTheThreeValues`（迁移 `0017` 那一列往返），初始路由计划见 `TestAPlanKeepsItsCustomsCitationsThroughTheStore`。
- ✅ 不进登记册、演示数据全 `SYN-`：自 `993995e7` 以来 `docs/product/PILOT-PARAMETER-REGISTER.md` 与 `scripts/demo-seeds/` 都没动；psb/05 格 6 所引两条用例的租户、网络、口岸、路径与委托标识全是 `SYN-` 值。领域与包级用例里另有 `tenant-c1`、`cand-cn-sg` 这类测试夹具标识，不进种子，也不作取证。

**门（钉 `0c17d9b5`，带 DSN，2026-10-09 04:21Z–04:24Z）**

- `go build ./...`、`go vet ./...` 全仓退 0；`gofmt -l .` 无输出。
- 开跑前单跑判据四两条真库用例，`-v` 下是 `PASS` 不是 `SKIP`。
- `go test -p 1 -count=1 -v`，`IDP_PARCEL_POSTGRES_DSN` 指 55432 门禁库。范围用 `go list` 反查：动过的包、它们的反向依赖（含只在测试里导入的）、全部 `pgtest` 使用方（迁移 `0017` 进了每个真库用例的建库），加 `./internal/architecture/...`，钉在此 SHA 上共 71 个包，其中 `cmd/*` 13 个。结果 69 个 `ok`、2 个无测试文件；`--- FAIL` 0；`--- SKIP` 1，是 `pgtest` 的 `TestHelperTemplateOwnerProcess`（只由子进程驱动的助手，与 DSN 无关）。
- `0c17d9b5` 之后只有清点与本票面两笔 `.md`，没动 `.go`/`.sql`。全仓 `-p 1` 全量留给重放台。

**判断项**（待评审；涉 CC 的可由 CC owner supersede）

1. **CONTEXT 规则三、四改写**（`5b81e938`）：分诊原文规则三没限定候选，与规则二「两端同国即可用」在「同国 + 依赖读不到」那一格字面相抵。改为缺码与两端同国两格不靠目录作答、读不到时照常作答，只有两端异国的候选在读不到时答状态未知；规则四补一句：读不到与目录为空的判断标识不得相同。
2. **申报模式这一维首版不评**：裁定三写「方向与申报模式可各自指」，而裁定二定的候选投影（段链 + 两端国家）不带申报模式，CONTEXT 规则一据此写明首版不评模式维。不可用理由于是只有口岸未登记、口岸未生效、方向对不上三种；要评模式，得先给投影补模式来源，另立票。
3. **不可用落到 NR 硬约束哪一格**（裁定三留给轮次 4 与 NR owner）：消费方适配器把不可用译成 `RESTRICTION_APPLIES`，限制引用为 `CUSTOMS_APPLICABILITY/<判断标识>/<理由>`；状态未知译成命名缺口（缺国家码指名一端 / 目录为空 / 目录读不到），各带重判触发。NR owner 没有单独复核，评审时眼见为实。
4. **目录版本引用取该租户两本目录的全量行**（含判断时点未生效的行），不只取作答用到的那几行。好处是出处能复原整份依据（未生效本身就是作答理由）；代价是该租户目录每登一行或换一版，它所有候选的判断标识都跟着换，哪怕那一行与候选无关。

**暂存测试的处置**：`/home/tops/workspace/.hold-mcp3-rfc12/customs_applicability_outcomes_test.go`（12,523 字节，sha256 以 `d79bc644` 开头）**不采用**，文件留在原处。理由：按判据逐格对，它十条用例钉的判据在本分支上都已有同判据的用例（`79614fd8` 的对照检查之外，还有 `4b967223` 的真库用例与领域用例），它独有的两片 red 与对照检查的两片 red 判据相同、已由 `79614fd8` 修掉，并进来只会在同一个包里多一份钉同样判据的文件；字面上只有它有的一格「缺出口路径、指名出口方向」，与已钉的进口一侧走同一个按方向参数化的 `sideCovered`。取证：本会话把它只拷进临时检出带 DSN 各跑一次——在 `0873a719` 上 8 条 `PASS`、2 条 `FAIL`（目录读不到时两端同国被答成 `STATUS_UNKNOWN`/`CATALOG_UNREADABLE`；读不到与目录为空共用判断标识），在 `0c17d9b5` 上 10 条全 `PASS`。

**未验 / 边界**：演示动线没有重走，演示种子那份（线路适用范围 `SYN-SCOPE-01`、服务区域未带覆盖国家）能否越过「墙三」未核，归 psb/05 的重走；初始路由能否落成计划还要看成本排序（rfc/10 已进 main），本票没量；全部取证只记 `S`。评审由通道 1 另派非作者，本会话是作者，不自评。
