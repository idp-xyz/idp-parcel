# 01 同一外部条码出现在两件实物：第二件也唯一关联到同一包裹，不形成身份冲突（AT-NO-019 字面场景没有实现）

Category: bug
Status: in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（分支 `claude-nri01`）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 序 0 研判）
Blocked by: 无（operator-channel/29 已进 main）
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判簇 C2 第三张草案（AT-NO-019）
地盘：
- internal/nodeoperations/application/receive_delivered_unit.go
- internal/nodeoperations/application/receive_delivered_unit_test.go
- internal/nodeoperations/ports/ports.go
- internal/nodeoperations/adapters/postgres/reception.go
- internal/nodeoperations/adapters/postgres/reception_test.go
- cmd/parcel-api/（新的真库用例文件，不与 assemble_reception_identity_test.go 同文件）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 出处

票 ps-external-mark-relations/02 评审遗留（operator-channel/26 序 0）研判时，权威一致性核验发现的漏项。会话 `idp-parcel-9b`，钉 main `8d22729f`；本会话复跑探针证实。

## 现象

真库实测（经生产装配 `buildReceptionOrchestration`，合成夹具，证据只记 `S`）：某个号在册里只标识一件包裹。两件不同的作业实物（作业实物标识不同）先后贴着这个号送站，两件都答`节点收寄已形成`，都关联到同一件包裹，各自建立控制，都不记身份冲突。

落点：

- `ReceiveDeliveredUnitHandler.receive` 只按候选数分支，恰一个就关联。`default` 分支的注释写「一码多物或多候选」，实际只覆盖了多候选。
- `receive_delivered_unit_test.go` 的 `TestUnknownAndConflictedIdentitiesStayPending` 头注点名了 `AT-NO-019`，但它的 `conflicting candidates` 子用例测的是一件实物两个候选。AT 覆盖盘点据此把 `AT-NO-019` 记成了已钉住。

## 为什么是缺陷

NO CONTEXT「实物身份与事实完整性」原话：「同一外部条码出现在多个实物，或同一实物存在相互冲突的外部标识时，各作业实物继续独立，形成身份冲突并暂停方向性作业。」UC-NO-002 验收 `AT-NO-019` 也写着：「两件实物保持独立，形成身份冲突并暂停方向性作业」。

它还与 ps-external-mark-relations/05 的唯一误关联叠加：真主人那件实物后到，也认不出冲突。

no-directional-basis/01 当前裁定的身份判断是「恰好关联到一个正式包裹、且没有一行记身份冲突，为成立」。这一格不补，两件实物都会被判成身份成立。

## 要裁的问（NO owner；难逆转的进 ADR）

1. 「同一外部条码出现在多个实物」怎么认？同租户内，同一件包裹已有另一件作业实物的当前收寄关联，算不算？按号码认，还是按关联到的包裹认？要不要限于先到那件仍在节点控制下？同一作业实物的重投是重放，不在此列。
2. 先到那件怎么办？它的收寄已形成，可能已被 PS 采用。在只追加、不覆盖的约束下，是给它补一版身份冲突判断，还是只让后到那件答冲突、交 VE 协调？
3. 与方向性作业门（no-directional-basis/01）怎么衔接？

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

按关联到的包裹认「同一外部条码出现在多个实物」：同租户内这件包裹已有另一件作业实物的当前客户送站收寄，后到那件答身份冲突、建待识别实物，不论先到那件是否仍在节点控制下；同一作业实物的重投仍是重放。先到那件只追加一版身份冲突判断，原判断照留（只追加、不覆盖）；两件在 no-directional-basis/01 的身份判断里都落成「身份冲突」。已被 PS 采用的那次采用不回滚，交 VE 协调。难逆转，出 NO ADR（用号 0196，会话已占）。实施排在 operator-channel/29 之后（同改 receive_delivered_unit.go）。

## 做什么（裁后）

- 照裁定在收寄判断里认出一码两物并答身份冲突；原观察与原判断照留。
- `receive` 里 `default` 分支的注释照实写。
- `TestUnknownAndConflictedIdentitiesStayPending` 的 `AT-NO-019` 点名，改成只认它实际钉住的那一半（照 AT 覆盖盘点「半边覆盖在点名处注明」的口径）。新用例按 `AT-NO-019` 的字面场景点名；AT 覆盖盘点按它的产出方式更正。

## 不做

- 唯一误关联（ps-external-mark-relations/05）、方向性作业门本身（no-directional-basis/01）、身份后来被确认（`AT-NO-026`）。

## 完成判据

- [ ] 要裁的问已裁，写进本票；难逆转的进 ADR。
- [ ] 一码两物按裁定答身份冲突。要有编排层用例，外加一条经生产装配的真库用例（另起测试文件，不与 ps-external-mark-relations/04 新建的 `assemble_reception_identity_test.go` 同文件）；零候选、多候选、唯一命中的既有用例照旧绿。
- [ ] `AT-NO-019` 的点名与实际覆盖一致。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`internal/architecture` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的实施子代理，分支 claude-nri01）

**落点**

- 编排：`internal/nodeoperations/application/receive_delivered_unit.go`——`receive` 在身份核对唯一指向一件包裹时，经收寄库问这件包裹是否已关联到同租户另一件作业实物；是则后到那件答`待识别`、记身份冲突、候选记那件包裹，不问 PS、不交意图；本次真落册后由 `appendIdentityConflicts` 给先到那件各追加一版待识别收寄（新结果版本、去关联、控制照原判断、记身份冲突），原判断不动。`default` 分支注释照实写成「多候选，或唯一指向的包裹已关联到另一件作业实物」。
- 端口与适配器：`ports.ReceptionStore.FindOtherUnitIntakes`；PG 适配器 `Receptions.FindOtherUnitIntakes`（行扫描抽成 `scanReception`，`FindByKey` 同用）。
- 点名：`TestUnknownAndConflictedIdentitiesStayPending` 头注改成 `AT-NO-019` 半边（一件实物多候选）；`.scratch/at-coverage-inventory.md` 的 `AT-NO-019` 一行照扫描口径列出新用例与半边。

**各格对应用例**

| 格 | 用例 |
|---|---|
| 一码两物：后到答冲突、先到追加冲突版、原判断照留、两件身份一格都含身份冲突 | 编排 `TestOneMarkOnTwoUnitsFormsAnIdentityConflict`；真库 `cmd/parcel-api/reception_one_mark_two_units_test.go::TestTheWiredReceptionTurnsOneMarkOnTwoUnitsIntoAnIdentityConflict`（生产装配 + `IntakeIdentities` + `JudgeIdentityBasis`） |
| 同一实物重投仍是重放；同一实物换来源再收不算冲突；冲突后重放后到那件不再追加 | 编排 `TestTheSameUnitDeliveredAgainIsNotAnIdentityConflict`；真库用例末段 |
| 新读口读不动不形成答案 | `TestDependencyFailuresFormNoAnswer/按包裹读别的实物收寄报错` |
| 读口只认同租户关联、排除自身实物、候选不算关联 | `adapters/postgres/reception_test.go::TestOtherUnitIntakesAreFoundByTheirParcelAssociation` |
| 零候选、多候选、唯一命中既有用例 | 照旧绿 |

**先红后绿**（基线 `7c42f850` 上先写用例）

- 编排：`TestOneMarkOnTwoUnitsFormsAnIdentityConflict` 红于「后到那件 outcome = "INTAKE_FORMED", want PENDING_IDENTIFICATION」；读口报错格红于「outcome = "INTAKE_FORMED", want error」。重放格修前即绿（守护格）。
- 真库：红于「后到那件 outcome = INTAKE_FORMED, want PENDING_IDENTIFICATION」——研判现象就此复现。
- 适配器读口用例与实现同写（新方法，修前只有编译红）。实现后全绿。

**复现不了的格**：无。

**没做（记在 ADR-0196 越权风险点）**：两件并发送站在两笔事务里互相看不到、可能都关联成立，未加锁；PS 已采用的那次不回滚，VE 侧协调未实现。

**ADR 与迁移**：ADR-0196（adr/README 已加行）。无迁移——按关联查的索引 `reception_intake_association_idx` 迁移 0004 已有。

**地盘外**：`internal/nodeoperations/adapters/http/receive_delivered_unit_test.go` 的收寄库替身补新读口方法（端口加方法所必需）；`docs/product/MECHANISM-INVENTORY.md` 重生成（cmd 测试 145→146）。

**门**：`gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1` 跑 `./internal/nodeoperations/...`、`./cmd/parcel-api`、`./cmd/parcel-frontline-import`、`./internal/architecture/...`、`./migrations/...` 全 ok；机制清点已重生成并单独提交。

**证据层级**：`S`（隔离合成夹具，`SYN-` 值）。

评审：待会话读 diff 审

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `312493a0`）

两轴合审，0 阻断。

- **Spec**：裁定四条逐一落到。按关联到的包裹认（`FindOtherUnitIntakes` 只认收寄本身带的关联，排除同一作业实物）；后到那件答待识别、记身份冲突、候选是那件包裹、不问 PS、不交意图；先到那件只追加、原判断原样读回；两件经 `JudgeIdentityBasis` 都含身份冲突（真库用例经生产装配核过）。追加与后到那件同一笔事务：两条接线（`transactionalReception`、受控批量导入）都包在事务里，追加失败整笔回滚、答 5xx，重投从头再来。并发输家读回赢家时 `commit` 走 `existingResult`，结果不是待识别，不会重复追加。
- **Standards**：注释中文、只写为什么；`scanReception` 抽出后 `FindByKey` 行为不变（错误前缀上移到调用方）；新读口走迁移 0004 已有的 `reception_intake_association_idx`，查询条件 `(intake ->> 'association') = $2` 对得上索引表达式。
- **非阻断（已修于本评审提交）**：冲突形成之后，先到那件换个来源再收，按包裹读不到后到那件（它的行不带关联），照常答`节点收寄已形成`并交意图；方向依据靠册上的冲突行照停。ADR-0196「能力边界」原先没写这一格，补了一条。
- **非阻断（另立票）**：两件实物并发送站互相看不到对方未提交的关联（ADR-0196 越权风险点 4）→ no-reception-identity/03。
- **与 ps-external-mark-relations/04 的交叠**：那张票的三条真库用例走多候选或首件，不受本票影响；ADR-0184 复核记录「其二」说的唯一命中，在本票之后只对先到的那件成立，后到的同包裹实物答身份冲突——ADR-0196 自己写了，那边不改。
