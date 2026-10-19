# 54 前线导入铸出的来源标识越过意图信封上限时，报告说「重跑会重发」，重跑却永远交不出去

Category: bug
Status: in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（operator-channel/45 已进 main），分支 `claude-oc54`。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立并代决（票 operator-channel/45 评审遗留）
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

## 完成判据

- [x] 用例：批次与事实编号拼出的来源标识恰在上限内的一行照常形成收寄并交出意图（真库）；越过一个字节的那一行答坏行、册上不落、报告点名；带控制字符同。修前越界那一行先红。
- [x] `go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `cmd/parcel-frontline-import` 与 `internal/architecture` 全绿；机制清点重生成。
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
- 读码见到的同类缝，没实测，没立票：集运导入的 `unitRef` 原样做封装快照意图信封的 Subject（NO 的 `OutboxSealedSnapshotHandoff`），模板上没有同一道门。越界时编排同样「失败不翻封装留续办」，报告写「封装快照意图未交出，续办引用 …」，重跑仍会撞拒收。
