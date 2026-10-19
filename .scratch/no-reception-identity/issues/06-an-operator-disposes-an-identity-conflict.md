# 06 身份冲突处置（UC-NO-004）：运营决定「身份冲突处置」逐链追加处置版、连带指明，按包裹的读面认处置截断，重核跳过被排除的包裹；管理台复核页加处置动作

Category: enhancement
Status: ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 04 拆出（代决，用户 2026-10-16 授权「你作为业务和系统专家你来自决」）
Blocked by:
- no-reception-source/03 进 main。处置之后可再经重核，决定九改的正是它落下的重核分格；它还同改收寄库 PG 适配器、复核目录与管理台待识别实物页。
- 05 进 main。处置要靠身份一格读链头才解得开，`AT-NO-053`、`AT-NO-054` 断言的就是解开之后的身份一格。
- 与 no-reception-source/04、05 同改收寄库 PG 适配器与迁移号，不设阻塞边：谁后进谁 rebase、取下一个空迁移号；动手前照[并行会话](../../../docs/agents/parallel-sessions.md)看清在途分支。
父票：[04](./04-identity-conflicts-have-a-disposition-and-release.md)
出处：票 04 裁定各点；[ADR-0210](../../../docs/adr/0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定一至四、六至九；[UC-NO-004](../../../docs/application/node-operations/UC-NO-004-DISPOSE-AN-IDENTITY-CONFLICT.md)。
地盘：
- internal/accessidentity/operation_decision.go 及其测试（决定种类多一种）
- scripts/demo-seeds/data/access/02-grants.json、03-admission-scope-mappings.json，scripts/demo-seeds/data/governance/ 下新起一份这一种运营决定的生产权威区间（照既有 `authority-interval-operation-decision-*` 那几份的形状）
- internal/nodeoperations/domain/（处置文档的形状与规范化、处置候选、两种结果的处置版构造）及其测试
- internal/nodeoperations/ports/ports.go（记录带处置文档，读口与写口）
- internal/nodeoperations/application/（新处置编排）及其测试；no-reception-source/03 落下的重核编排（决定九那一格）
- internal/nodeoperations/adapters/postgres/reception.go（按实物读各链当前有效版本、按派生键读处置、`FindOtherUnitIntakes` 截断）、parcel_containment_view.go（截断）、review_catalogue.go（待识别实物页要的链头与处置）及各自测试
- migrations/node_operations/（处置文档一列；下一个空号）
- internal/nodeoperations/adapters/http/（处置命令口与「运营决定」操作者 Intake；复核目录查询多带的格）及其测试
- internal/nodeoperations/ports/catalogue_read.go
- cmd/parcel-api（端点表、装配、真库用例新起文件）
- apps/admin-web/src/pages/operations/**（待识别实物页的处置动作）
- .scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md（补记末尾追加一行回写：本口凭哪条代数）
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（钉 main `4f11563c`，读码）

- 收寄册上记身份冲突的行没有任何入口能接续：重核跳过冲突（ADR-0199 决定三），收寄编排只在新的送站里追加。
- 操作者渠道「运营决定」能力面的决定种类（`accessidentity.DecisionKind` 的词表）没有身份冲突处置这一种；NO 没有任何运营决定口。
- `Receptions.FindOtherUnitIntakes` 与 `ParcelContainmentView.LoadParcelContainment` 按行读关联与候选，不看版本链；复核目录 `ReviewCatalogue.ListUnidentifiedItems` 逐行列待识别，分不出哪一版是当前有效版本。

## 做什么

照 ADR-0210 与 UC-NO-004：

1. **授予**。`accessidentity` 的决定种类加「身份冲突处置」（字面量本票定，建议 `IDENTITY_CONFLICT_DISPOSITION`）。演示种子给演示租户的配置操作者加这一种授予、登记准入范围对照与生产权威区间；生产装配不配任何默认，未授予答「无此决定种类的授予」。
2. **命令口**。端点表加一行（路径本票定，建议 `/node-operations/identity-dispositions`），挂「运营决定」操作者 Intake：照 `tfhttp.OperatorDecisionIntake`、`shipmenthttp.OperatorDecisionIntake` 的做法在 NO 的 HTTP 包里另起，或抽共用，实施定。信封各格照 ADR-0151 决定三；坏载荷 400 `MALFORMED_REQUEST` 的各种照 ADR-0210 决定六列全。
3. **编排**（ADR-0210 决定二至四、六、七）：按（租户，所处置的版本）认重放 → 先实物后包裹取锁 → 读主实物各链当前有效版本 → 核版本是否仍是链头、各链合起来是否判冲突 → 算处置候选、核指定的包裹 → 指定一件时按处置截断读此刻关联到它的别的实物、与连带实物对照 → 主实物与连带实物逐链追加处置版 → 指定一件的那几版照 ADR-0186 取服务结果 → 需要时备一封收寄发布意图 → 原子提交。
   - 答复格：身份冲突已处置、已有结果（`EXISTING_RESULT`）、已处置、版本已换代、无可处置的冲突、不在候选之内、关联对象另有归属。线上名本票定，写进完成记录；回显的内容照 UC-NO-004 结果表。
   - 第四期（no-reception-source/04）的实物锁已在 main 就取，不在就只取包裹锁；次序与收寄、重核同一口径。
   - no-reception-source/05（第三期）已在 main 时，「都不是」那一版写身份核对态「已核对」；不在时由它的迁移回填。
4. **存储**。收寄表加处置文档一列（jsonb，自带形状版本，ADR-0014），只在处置版上有；迁移头注写既有行为空的理由。派生来源标识 `IDENTITY-DISPOSITION/<前版的判断版本>`；内容指纹格写比对格（结果、指定的包裹、连带实物集合）的规范化摘要，同一次处置的各版同一个摘要；来源文档取所在链起点那一版的，起点没有就不带。
5. **处置截断**（ADR-0210 决定八）。`FindOtherUnitIntakes` 与 `ParcelContainmentView` 两路都不再算被处置版接续过的行；处置版自己带的关联照算。还没处置过的实物读法照旧，用例钉住。截断用沿前版回溯还是在处置版上记下它截断的各版，实施定，不得改写旧行。
6. **重核例外**（ADR-0210 决定九）。被重核那一版是「都不是」的处置版、PS 唯一命中的那件包裹在它排除之列时，不追加，照零候选办。其余分格照旧。
7. **复核目录与管理台**。待识别实物页按实物把各链当前有效版本列在一起，分得出冲突是否已处置、处置了什么（结果、指定的包裹或排除的那一组、操作者、依据引用、时刻）；「处置」动作：选指定一件或都不是、填依据引用；指定一件时，「关联对象另有归属」的回显带出须连带的实物，确认后一并提交。**走 Go 那条路**：随本票分支、非作者评审与推送方全量，不走前端切片的 main 直做。admin-web 三道门照 [workflow 本机环境](../../../docs/agents/workflow.md#本机环境) 的口径。
8. **逐口核**（ADR-0151 决定三）。本口生来自带代数，凭哪条写进完成记录，并在 operator-channel/15 补记末尾追加一行回写。
9. **AT 点名**：`AT-NO-053` 至 `AT-NO-060`；`.scratch/at-coverage-inventory.md` 照扫描口径更正。

## 不做

- 歧义信号消失后的自动解除（ADR-0210 决定一，后续方向，PS 要先交信封）。
- 对没有身份冲突的关联作单独更正口；「同一实物存在相互冲突的外部标识」那一半。
- PS 采用回滚、撤销信号；VE 侧的发现与协调（ADR-0196 决定五）。
- 向 `party-commercial` 问授权坐标（ADR-0210 决定一）。
- 身份一格读链头（票 05）。

## 完成判据

- [ ] 编排用例，一码两物（`AT-NO-053`）：后到那件指定那件包裹、先到那件列为连带实物 → 后到那件追加关联版、先到那件追加「不是它」的版本，原判断与冲突版逐字不变；身份一格后到那件成立、先到那件缺少·待识别；意图一封、属后到那件；先到那件已交出的意图不撤回。反过来，先处置先到那件、指定那件包裹（后到那件没有关联，不必连带）→ 先到那件此前已就那件包裹交出过，不再交意图；后到那件另行处置为都不是。
- [ ] 编排用例，确认都不是（`AT-NO-054`）：多候选与带歧义信号各一条 → 待识别版、不记冲突、记下排除的那一组，不交意图；之后重核仍只唯一命中被排除的包裹 → 不追加；号码另有新的包裹关联、重核唯一命中它 → 照常追加关联版。
- [ ] 编排用例，关联对象另有归属（`AT-NO-055`）：漏列、多列、连带实物版本过时、连带实物另牵连别的包裹，各一条，都不落版并回显此刻关联到指定包裹的实物与版本。
- [ ] 编排用例，代数（`AT-NO-056`）：原样重投答已有结果并回显；换处置依据引用或换操作者原样重投仍答已有结果、回显原值；换结果或换指定的包裹答已处置；所处置的版本被第三件到站的冲突版接续答版本已换代；三者都不追加、不耗版本号、不重复意图。
- [ ] 编排用例（`AT-NO-057`）：候选之外的包裹答不在候选之内；没有冲突的实物答无可处置的冲突。
- [ ] 编排用例，多链（`AT-NO-060`）：两条链的实物处置其中一版，两条链各追加一版。
- [ ] 读面用例（`AT-NO-059`）：处置之后贴同号的第三件到站，只与此刻关联到那件包裹的实物形成一码两物，被处置为「不是它」的实物不再被追加冲突版；`ParcelContainmentView` 不再把它算作那件包裹；还没处置过的实物两面读法照旧。
- [ ] HTTP 与装配用例（`AT-NO-058`）：发行方未配置、令牌无效、无此决定种类的授予、不在准入范围各答其格；请求体带租户或操作者、坏载荷各种答 400；跨租户指名与查无此物同答。
- [ ] 真库用例（cmd/parcel-api，经生产装配与演示种子同形的授予）：一码两物 → 处置 → 两件身份一格如上 → 意图一封入队 → 原样重投答已有结果、册上不多一行。
- [ ] 并发：处置与同一包裹上又一件实物的收寄并发，经包裹锁串行，不出现一版两个后继。
- [ ] admin-web 三道门退 0；待识别实物页的处置动作有组件用例。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/accessidentity/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
