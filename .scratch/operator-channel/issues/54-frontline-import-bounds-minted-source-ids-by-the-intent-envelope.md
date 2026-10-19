# 54 前线导入铸出的来源标识越过意图信封上限时，报告说「重跑会重发」，重跑却永远交不出去

Category: bug
Status: in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工（含裁定补），待评审。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（operator-channel/45 已进 main），分支 `claude-oc54`。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立并代决（票 operator-channel/45 评审遗留）
Blocked by: operator-channel/45 进 main
父票：[45](./45-http-reception-does-not-swallow-a-failed-intent-enqueue.md)
出处：票 45 实施子代理回报的「没做」与会话评审。
地盘：
- cmd/parcel-frontline-import/intake_template.go 及其测试
- cmd/parcel-frontline-import/intake_import.go（只在报告措辞要跟着改时）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（读码，未实测）

收寄意图信封的 Subject 原样取来源标识（NO 的 `OutboxNodeIntakeHandoff`），越过框架的 `eventing.MaxSubjectLength` 或带控制字符就被确定性拒收。票 45 让 HTTP 设备路在接入时按这条上限拒成坏报文；前线导入路的来源标识由 `intakeSourceID` 从批次与事实两格铸出，没有同样的门。超长时收寄照常落册，意图入队失败被编排吞成续办引用，报告写「意图未交出（…），重跑同一文件会重发同一份」——而重跑每次都会撞同一道拒收。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制，不涉租户取值）

在模板解码那一步按铸出的来源标识判：越过 `eventing.MaxSubjectLength` 或带控制字符，这一行按坏行进报告、不形成收寄，措辞让内勤知道是批次或事实编号太长。上限直接引 `eventing.MaxSubjectLength`，不另写数。不改编排吞错的写法（导入路仍要续办）。

## 裁定补（2026-10-19，会话 idp-parcel-9b 代决；机制，不涉租户取值）

首轮评审 0 阻断；作者在「没做」里记的同类缝并入本票，同一分支接着做。理由：缝同形，同一个导入口，同一种「报告让人重跑、重跑永远交不出去」，分票只多一轮认领与评审。

- 集运导入的 `unitRef` 原样做封装快照意图信封的 Subject（NO 的 `OutboxSealedSnapshotHandoff`）。照收寄那一半同法：在 `consolidation_template.go` 译装时判进信封的那一串，越界或带控制字符整批拒，报告点名该改的列。上限引框架常量，不另写数。编排吞错那一半不动。
- 一并查有没有 HTTP 路（操作者渠道或设备口）把同一个 unitRef 或别的设备自报串原样放进 NO 意图信封的 Subject、而接线在入队失败时回滚答 5xx（票 45 那种）。有就照 `receptionCommand` 的写法在接入处拒成 400；没有就写明查过、没有。
- 地盘随之加：`cmd/parcel-frontline-import/consolidation_template.go` 及其测试；HTTP 路若有，加那条路的接入处。

## 完成判据

- [x] 用例：批次与事实编号拼出的来源标识恰在上限内的一行照常形成收寄并交出意图（真库）；越过一个字节的那一行答坏行、册上不落、报告点名；带控制字符同。修前越界那一行先红。
- [x] `go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `cmd/parcel-frontline-import` 与 `internal/architecture` 全绿；机制清点重生成。
- [x] 裁定补：单元号恰在封装快照意图信封上限内照常封装并交出意图（真库）；越过一个字节、带控制字符的那一份文件整批拒、单元不在册、报告点名 unitRef。修前越界那一行先红。HTTP 路查过并写明。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-oc54`）

**落点**

- `cmd/parcel-frontline-import/intake_template.go`：新增 `intakeSourceIDCarriable`，`intakeRowFrom` 第一步铸出来源标识后即过它。铸出的整条标识越过 `eventing.MaxSubjectLength` 或带控制字符，这一行报错，经 `decodeTemplate` 包成 `收寄模板不合格：第 N 行：…`，整批不开工、退出码 1，不连库。上限直接引框架常量，与设备收寄口 `receptionCommand` 同源，不另写数。头注写了理由：编排把入队失败吞成续办，可这一行重跑多少次都撞同一道拒收。编排吞错的写法没动（裁定末句）。
- `intake_import.go` 未改：坏行不进编排，报告措辞不用跟着改。

**作者自定的判断项**

1. **坏行照本包现有写法整批拒，不另起逐行「被拒」。** 裁定写「在模板解码那一步判」，而本包在译装这一步的坏行一律整批拒：`decodeTemplate` 的头注写明，部分行进库、部分行没进，内勤对着一张表分不出哪几行是哪种。所以「册上不落、报告点名」落成：文件整批不开工，标准错误输出点名行号与该改的格。
2. **判铸出的整条标识，不分别判两格。** 上限压在拼好的那一串上，`FTI/INTAKE-1/` 前缀也占字节。控制字符只能来自批次号或行事实号两格（其余各段是本口常量），报文点名是哪一格；单元格内换行（Excel 的 Alt+Enter，CSV 里是带引号的字段）也算控制字符。
3. **措辞**（实跑 CLI 取的原话）：
   - 越界：`parcel-frontline-import: 收寄模板不合格：第 2 行：batchRef 与 factRef 合起来太长：拼出的来源标识 513 字节，收寄意图最多带 512 字节（batchRef 11 字节，factRef 488 字节）——请缩短批次号或行事实号`
   - 控制字符：`parcel-frontline-import: 收寄模板不合格：第 2 行：factRef="SYN-SLIP-\n0001" 带控制字符（单元格内换行、Tab 之类），拼出的来源标识交不出收寄意图——请删掉批次号或行事实号里的不可见字符`

**各格对应用例**

| 格 | 用例 |
|---|---|
| 恰在上限内（铸出的标识恰 `eventing.MaxSubjectLength` 字节）照常形成收寄并交出意图（真库：INTAKE_FORMED、明细不提意图未交出、落册、意图一封） | `cmd/parcel-frontline-import/intake_source_id_bound_test.go::TestFrontlineImportHandsOffTheIntentForASourceIDRightAtTheEnvelopeLimit` |
| 越过一个字节的那一行答坏行、册上不落、意图零封、报文带收寄模板不合格与第 2 行；带控制字符同（真库） | `cmd/parcel-frontline-import/intake_source_id_bound_test.go::TestFrontlineImportRejectsASourceIDTheIntentEnvelopeCannotCarry`（子格「越过一个字节」「带控制字符」） |
| 译装层：恰在上限照常译装、标识逐字等于前缀加行事实号；越一个字节、行事实号带控制字符、单元格内换行、批次号带控制字符，各答整批拒，报文点名行号与格 | `cmd/parcel-frontline-import/intake_template_test.go::TestDecodeTemplateBoundsTheMintedSourceIDByTheIntentEnvelope` |

真库用例照 `intake_handoff_continuation_test.go` 的 `importerWithSwitchableHandoff` 装，意图交付开着，没改那份文件。

**先红后绿**

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc54-red.log`（钉 `b91a580f`，只加了用例）。现象原话，真库越界子格：「译装放过了信封装不下的来源标识（513 字节），导入答：第 2 行 → INTAKE_FORMED [已落地] 收寄版本 …，关联 SYN-PARCEL-0001，…；意图未交出（CONT-63a8756ade235527），重跑同一文件会重发同一份」，即票面现象。控制字符子格同形（39 字节）。译装层四个子格答「应整批拒绝，却译装出来源标识 …」。恰在上限的真库用例修前即绿，是守护格，防这道门差一个字节。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc54-green.log`。

**门**（钉代码笔 `d7f88d75`）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./cmd/parcel-frontline-import/... ./internal/nodeoperations/... ./internal/architecture/...` 全 ok（日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc54-gate.log`）。机制清点重生成，差一行（cmd 测试文件 154 → 155），单独一笔 `c7f99813`。

**证据层级**：`S`（隔离合成夹具，`SYN-` 值）。

**越出地盘**：`cmd/parcel-frontline-import/intake_source_id_bound_test.go` 是新文件，算作 `intake_template.go` 的测试，复用 `intake_handoff_continuation_test.go` 与 `vertical_test.go` 的夹具但没改它们。另有 `docs/product/MECHANISM-INVENTORY.md`（判据要求的重生成）。

**没做**

- 「走 Go 那条路」的评审与推送方全量未到。
- ~~读码见到的同类缝：集运导入的 `unitRef` 原样做封装快照意图信封的 Subject，模板上没有同一道门。~~ 已并入裁定补，见下节。

### 裁定补这一半（2026-10-19，同一作者子代理，同一分支）

**落点**

- `cmd/parcel-frontline-import/consolidation_template.go`：新增 `consolidationUnitCarriable`，`consolidationRowFrom` 构造出单元号后即过它。单元号越过上限或带控制字符，这一行报错，经 `decodeTemplate` 包成 `集运模板不合格：第 N 行：…`，整批不开工、退出码 1。编排吞错那一半（`handOffSnapshot` 交回 `CONT-SNAPSHOT/…`）没动。
- `consolidation_import.go` 未改。

**作者自定的判断项**

1. **上限取 Subject 与分区键两者里紧的那个，不只判 Subject。** 封装快照意图的分区键是 `租户/单元`（`sealedSnapshotPartitionKey`），也原样带单元号，前面还多了租户号。框架两个上限今天都是 512，所以总是分区键先撑满：合成租户 `SYN-T1` 下单元号最多 505 字节。只判 Subject 会放过 506–512 字节这一截交不出去的号。修前红已实测：506 与 512 两格都照常封装，明细写「封装快照意图未交出」。两个数都取 `eventing.MaxSubjectLength` / `eventing.MaxPartitionKeyLength`；分区键的拼法（租户号加一个 `/`）在本口照写了一遍，头注点名 `sealedSnapshotPartitionKey`，由真库用例「恰在上限内照常交出」守着。那边一旦往键里多拼东西，这一格先红。没把拼法导出给本口调：那要动 NO 适配器的导出面，为一个长度判断不值。
2. **六种动作都判，不只封装行。** 单元号是同一个单元在六口上的同一个身份。只拦封装行，开启、移入照常落册，单元要到封装那一行才卡住，这时已经留下一个永远封装不出去的开放单元。
3. **措辞**（实跑 CLI 取的原话）：
   - 越界：`parcel-frontline-import: 集运模板不合格：第 2 行：unitRef 太长：506 字节，租户 SYN-T1 下封装快照意图最多带 505 字节——请缩短单元号`
   - 控制字符：`parcel-frontline-import: 集运模板不合格：第 2 行：unitRef="SYN-CU-\n1" 带控制字符（单元格内换行、Tab 之类），封装快照意图交不出——请删掉单元号里的不可见字符`

**HTTP 路：查过，没有要拒的。** NO 的四只意图交付里，生产装配只接了两只：收寄意图交付 `OutboxNodeIntakeHandoff`，装在 `parcel-api` 的收寄口和前线导入收寄；封装快照意图交付 `OutboxSealedSnapshotHandoff`，只装在前线导入集运（`buildConsolidationImporter` 头注：`parcel-api` 没有集运端点）。承接意图与执行事实意图（`OutboxCollaborationAcceptanceHandoff`、`OutboxExecutionFactHandoff`）在任何 `cmd/` 里都没装配。NO 的 HTTP 写路只有 `/node-operations/receptions`，设备口与操作者渠道 Intake（`WorkFactReceptionIntake`）都经 `receptionCommand`，票 45 已按 Subject 上限与控制字符拒成 400。收寄信封的其余各维：ID 与分区键是定长指纹，Scope 取认证结果的租户，都不随设备自报串变。所以没有第二条路要拒。只查了 NO 的意图信封；别的上下文的 HTTP 口不在这次范围。

**各格对应用例**

| 格 | 用例 |
|---|---|
| 单元号恰在上限内（合成租户下 `MaxPartitionKeyLength − len("SYN-T1/")` 字节），开启、移入、封装三行照常落地，单元已封装，封装行明细不提意图未交出，快照意图一封（真库） | `cmd/parcel-frontline-import/consolidation_unit_bound_test.go::TestFrontlineConsolidationHandsOffTheSnapshotForAUnitRefRightAtTheEnvelopeLimit` |
| 越过一个字节、恰为 Subject 上限而分区键越界、带控制字符：译装整批拒，报文带集运模板不合格与「第 2 行：unitRef」，单元不在册，快照意图零封（真库） | `cmd/parcel-frontline-import/consolidation_unit_bound_test.go::TestFrontlineConsolidationRejectsAUnitRefTheSnapshotEnvelopeCannotCarry` |
| 译装层：恰在上限照常、单元号逐字进命令；越一个字节、恰为 Subject 上限、封装行同样拒、控制字符、单元格内换行，各答整批拒并点名 unitRef | `cmd/parcel-frontline-import/consolidation_template_test.go::TestDecodeConsolidationTemplateBoundsTheUnitRefByTheSnapshotEnvelope` |

**先红后绿**

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc54-red2.log`（钉 `c8ec42a9`，只加了用例）。现象原话，真库越界子格：「译装放过了信封装不下的单元号（506 字节），封装行答：第 4 行 → SEALED [已落地] 单元 <unitRef> 已封装，成员 1，快照 1；封装快照意图未交出，续办引用 CONT-SNAPSHOT/<unitRef>/SYN-SEAL-1」。512 字节与控制字符两格同形。译装层五个子格答「应整批拒绝」。恰在上限的真库用例修前即绿，是守护格。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc54-green2.log`。

**门**（钉代码笔 `79e1470d`）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./cmd/parcel-frontline-import/... ./internal/nodeoperations/... ./internal/architecture/...` 全 ok（日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc54-gate2.log`）。没动 `cmd/parcel-api`，没加跑它。机制清点重生成，差一行（cmd 测试文件 155 → 156），单独一笔 `d6701d01`。

**证据层级**：`S`。

**越出地盘**：新文件 `cmd/parcel-frontline-import/consolidation_unit_bound_test.go`，算作 `consolidation_template.go` 的测试，复用 `consolidation_vertical_test.go` 的 `consolidationLineFor`、`findUnit` 与 `importConsolidationTemplate`，没改那份文件。另有 `docs/product/MECHANISM-INVENTORY.md`。HTTP 路没有要改的，没动。

**没做**

- 「走 Go 那条路」同上。
- 集运模板的 `sealRef` 只进封装快照意图的载荷与定长指纹 ID，载荷上限 1 MiB（`eventing.MaxPayloadBytes`），CSV 一格到不了，没设门。
- Scope 取 CLI 给的租户号（收寄、集运两口同），超过 `eventing.MaxScopeLength` 时整批信封都会被拒。租户由受控运维给出，不在内勤改文件能解决的范围，没设门，记此一句。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `ac8236ea`）

两轴合审，0 阻断。

- **Spec**：收寄一半判铸出的整条来源标识（标记前缀与模板版本也占字节），上限与 `receptionCommand` 同取 `eventing.MaxSubjectLength`；集运一半（裁定补并入）六种动作都判单元号，取 Subject 与分区键两道上限里紧的那个——作者多判的分区键一格有实测撑着（506 与 512 字节修前都照常封装、意图交不出），认可。坏行整批拒、报告点名该改的列；编排吞错那半与两份 `*_import.go` 未动。HTTP 路查过：NO 写路只有收寄口，票 45 已拒，封装快照意图不在 parcel-api 装配。
- **Standards**：上限都取框架常量，注释只写为什么。
- **评审修补（已做，`c77d7807`）**：集运那一半在 cmd 里照抄了分区键拼法 `租户/单元`，成了第二处定义。改为 NO 适配器导出 `SealedSnapshotPartitionKey`，cmd 用空单元号拼出前缀算余量；`OutboxSealedSnapshotHandoff` 照用同一函数。改后本包与 NO PG 适配器带 DSN 照绿，五条边界用例 PASS。
- **未设门、只记**（作者已写进票面）：两口的 Scope 取 CLI 给的租户号，越过 `eventing.MaxScopeLength` 时整批信封被拒——租户由受控运维给出，内勤改文件解决不了，不在本票。
