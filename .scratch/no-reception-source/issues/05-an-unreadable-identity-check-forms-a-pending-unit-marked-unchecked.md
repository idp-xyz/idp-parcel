# 05 第三期：身份核对读不动翻成`待识别实物已建立` + 身份核对态「未核对」——ADR-0186 决定二（草案 C1-T1）身份支的 5xx 由本票翻转

Category: enhancement
Status: resolved——2026-10-19 进 main（代码验证钉 `e2c6f836`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（04 已进 main），分支 `claude-nrs05`；与 no-reception-identity/06 并行，后者先进 main 时本分支 rebase。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 01 拆出（代决）；Blocked by 里原挂的用户决定已于 2026-10-19 由会话 idp-parcel-9b 代决
Blocked by:
- 04 进 main（前提「挡住同一作业实物重复形成控制起点」）；02、03 经它已在 main。
- ~~用户决定~~ 已解：未核对行由什么续上，2026-10-19 会话 idp-parcel-9b 代决取「落未核对行的同一笔事务自投重核门铃」，见 ADR-0199 决定三与越权风险点「未核对行由自投门铃续上」。
父票：[01](./01-node-reception-preserves-the-delivery-source.md)
出处：票 01 裁定分期之「三、身份读不动翻成『待识别 + 未核对』」与「核对态要不要放进设备响应」那一问；ADR-0199 决定五。票 01「做什么」点名的三条真库用例归本票。
地盘：
- internal/nodeoperations/application/receive_delivered_unit.go 及其测试
- internal/nodeoperations/ports/ports.go（身份核对态的封闭枚举）、ports/catalogue_read.go
- internal/nodeoperations/adapters/postgres/reception.go、review_catalogue.go、parcel_containment_view.go 及其测试；身份读口的 savepoint 装饰器（与 service_result_savepoint.go 同形，新文件或同文件由实施定）
- migrations/node_operations/（身份核对态一列：封闭两值、只有待识别带；既有待识别行回填「已核对」，推定依据写迁移头注）
- internal/nodeoperations/adapters/http/receive_delivered_unit.go、query_node_operations_records.go 及其测试
- cmd/parcel-api/assemble_reception.go、cmd/parcel-frontline-import/intake_import.go（两处装配给身份读口包 savepoint；前线导入明细）
- cmd/parcel-api 下新起真库用例文件
- cmd/parcel-dispatch/assemble.go（自投门铃的路由；另一会话的在途地盘，动手前先报文件名）
- apps/admin-web/src/pages/operations/**（待识别实物页显示身份核对态）
- docs/adr/0186-node-reception-records-physical-facts-when-the-service-result-read-fails.md（文末追加复核记录，决定原文不动）
- docs/adr/README.md（0186 一行标部分停用）
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 翻转前提今天各自齐不齐（钉 main `91ea0883`）

ADR-0199 决定五列的前提，缺一个都不翻：

| 前提 | 今天 | 卡在 |
|---|---|---|
| 来源已保全 | 不齐。收寄记录没有号码格，`Handle` 丢弃规范化文档 | 票 02 |
| 「未核对」处处可见（设备答复、前线导入明细、复核目录与管理台、`ParcelContainmentView`） | 不齐。核对态只有服务结果一格（ADR-0186；复核目录由 operator-channel/43、管理台由 operator-channel/44 透出），身份核对态不存在；`ParcelContainmentView` 对未核对在袋件的答法 ADR-0199 已定、未实现 | 本票自带，与翻转同票落地，不单独先落 |
| NO 主动重核 | 不齐。没有重核机制，`AT-NO-026` 是机制候选；且「关联早于读不动形成」那一格在裁定的触发下只能人工续上 | 票 03；那一格另卡在上面的用户决定 |
| 挡住同一作业实物重复形成控制起点，不只限于身份这一支 | 不齐。收寄键只有（租户，来源），同一实物换来源再收又起一个控制起点；no-reception-identity/03 的锁只串行同一包裹的唯一识别那一支 | 票 04 |

## 做什么

照 ADR-0199 决定五：

1. **编排**：身份核对读口交回错误时，落`待识别实物已建立`：收寄与控制照常、来源已保全、候选为空、身份核对态「未核对」，不问服务结果、不交意图；HTTP 首答 201。同一实物在同一节点已有当前收寄时照票 04 并入，不因身份读不动绕过。
2. **身份读口经 savepoint 隔离**：两处生产装配都包，ctx 不在事务里时直通，写法同 `SavepointedServiceResultView`；`transactionalReception` 与前线导入装配的头注随之改写。
3. **存储**：身份核对态一列，封闭 `CHECKED` / `UNCHECKED`，只有待识别行带；既有待识别行回填 `CHECKED`，迁移头注写推定依据（翻转前身份读不动从不落行：ADR-0186 决定二；ADR-0186 之前读不动答零行的待确认：票 operator-channel/29 现象；ADR-0196 的冲突版来自读口答复了的那次送站），写法同迁移 0006。
4. **处处可见**：HTTP 答复带 `identityCheck`（信息格，不进 outcome 词表，首答与重放都带）；前线导入明细写「身份未核对：候选缺席不代表查无此号」，这一行照常记成功、不再记未决；复核目录待识别行带核对态；管理台待识别实物页显示；`ParcelContainmentView` 在同租户有未核对在袋件时，对没有已关联在袋实物的包裹答`不可归属`。
5. **重核接上未核对**：票 03 的重核对象本就含不带关联、不记冲突的待识别；本票补「未核对 → 零候选」时追加「待识别 · 已核对」那一格的用例；落未核对行的同一笔事务自投重核门铃（ADR-0199 决定三），NO 的 inbox 消费者接住后走票 03 的同一份重核，只看那一件；身份核对仍读不动就交回 error、门铃退避重投。
6. **改随新答法变了的用例与注释**：ADR-0186 名下「身份核对读不动不形成答案」的用例（`TestDependencyFailuresFormNoAnswer` 里身份核对那一格，HTTP 与前线导入两侧的对应用例）改钉新答法；`ReceiveDeliveredUnitHandler.Handle` 与 `receive` 头注里「身份核对读不动…交回 error」改写。
7. **ADR-0186 复核记录**：决定二「身份核对读不动：不形成答案」与决定七「身份读口不隔离」自本票落地起部分停用（ADR-0199 决定五），决定二里「不拿空候选顶替」照旧、由核对态兑现；adr/README 的 0186 一行加「部分停用」注。
8. **AT 点名**：`AT-NO-051`。

## 不做

- 服务结果读不动那一支（ADR-0186 决定一）、拒收与仅扫描、零候选、多候选、身份唯一可读各支的 outcome。
- 方向依据代码：未核对行照待识别折（缺少·待识别），不另立原因（ADR-0199 决定五）。
- 一线作业端（票 01「不做」）。

## 完成判据

- [x] 真库用例一（经生产装配）：PS 身份读口读不动期间，同一来源重投，收寄不多出一条：答已有结果，`recordedOutcome` 是 `PENDING_IDENTIFICATION`、`identityCheck` 是 `UNCHECKED`，册上一行、一个控制起点、意图为零。
- [x] 真库用例二：PS 恢复后，重核落下关联，意图一封，业务时间是原收寄时刻。触发是自投门铃，不经人工：PS 恢复后门铃重投即落下关联；PS 仍读不动时门铃交回 error、册上不追加。
- [x] 真库用例三：换来源重扫同一作业实物，不再形成第二个控制起点（并入未核对的那一条，答已有结果）。
- [x] savepoint 用例：身份读口报 SQL 级错误时，收寄那笔事务照常提交、未核对行落下；ctx 不在事务里时直通（照 `service_result_savepoint_test` 的做法）。
- [x] 零候选（`CHECKED`）与未核对（`UNCHECKED`）在 HTTP 答复、前线导入明细、复核目录、管理台上分得开；`ParcelContainmentView` 对未核对在袋件答`不可归属`，对已关联在袋的包裹仍答`在`（`AT-NO-051`）。
- [x] 其余各支 outcome 不变，既有用例照旧绿。
- [x] ADR-0186 文末复核记录已追加、adr/README 0186 一行已加部分停用注；`git diff` 对 ADR-0186 只有追加行。
- [x] admin-web 三道门退 0（口径见 [workflow 本机环境](../../../docs/agents/workflow.md#本机环境)）。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-nrs05`）

起点 main `1167b4d9`；no-reception-identity/06 进 main 后 rebase 到 `08254c8c` 之上（那时本分支还没有提交，工作树改动整体搬过去，零冲突）。证据层级 **S**：夹具全为 `SYN-` 合成值，隔离合成库上跑。

**提交**

| 提交 | 内容 |
|---|---|
| `0b2b2ec8` | 翻转本体：编排、端口、迁移 0011、收寄库、身份读口 savepoint、自投门铃与它的消费门、两处生产装配、HTTP 与复核目录、前线导入明细、管理台、`ParcelContainmentView`，各层用例 |
| `469286a6` | ADR-0186 复核记录与 adr/README 0186 一行的部分停用注 |
| `ab795603` | 机制清点在 `469286a6` 的干净检出上重生成 |
| 本笔 | 本完成记录；AT 盘点追记十二 |

**落点**

- 编排 `ReceiveDeliveredUnitHandler.receive`：`identityJudge.judge` 交回带 `ErrParcelIdentityUnreadable` 的错时不再上抛，落`待识别实物已建立`——收寄与控制照常、来源文档照存、候选为空、不记冲突、不带关联、`IdentityCheck` 为未核对，不问服务结果、不交意图；真落了册（不是并发里读回的赢家）才经 `ports.IdentityRecheckDoorbell` 同一笔自投门铃，入不了队交回 error（不形成答案，归 ADR-0186 决定三那一格）。判断里按包裹读别的实物收寄的失败不带那个哨兵，照旧不形成答案。依赖多一格 `Doorbell`，缺了构造期拒。同一实物在同一节点已有当前收寄时照票 04 并入，在身份核对之前认出，不因读不动绕过。
- 读口答复了的待识别都记已核对：零候选、多候选、ADR-0196 冲突版（`appendIdentityConflicts`）、重核追加的冲突版、no-reception-identity/06 的处置版里不带关联的那几版（`dispositionVersion`）。关联收寄、拒收、待确认、并入不带。
- 重核 `RecheckPendingIdentityHandler.recheck`：零候选时，被重核那一版若是未核对就追加一版「待识别 · 已核对」（派生键 `IDENTITY-RECHECK/<版本>`，前版是未核对那一版，控制、接收时刻、来源文档照原判断，不交意图），新结果格 `RecheckIdentityNowChecked`（线上 `IDENTITY_NOW_CHECKED`，命令口 201、带 `judgmentVersion`）；已核对的照旧判断不变、不追加。门铃与人工两路都走它。
- 身份读口 savepoint：`internal/nodeoperations/adapters/postgres/identity_savepoint.go` 的 `SavepointedParcelIdentityView`，与 `SavepointedServiceResultView` 同形、各用各的 savepoint 名，ctx 不在事务里直通。parcel-api 的 `buildParcelIdentityView`（收寄与人工重核两处共用）与前线导入的 `parcelIdentityViewOf` 都包；`transactionalReception` 与 `buildIntakeImporter` 头注随之改写。parcel-dispatch 的重核装配没包：重核读不动本就整笔回滚。

**身份核对态的存储与回填**：迁移 `migrations/node_operations/0011_reception_identity_check.sql` 加 `identity_check text`。约束 `reception_identity_check_closed`（封闭 `CHECKED` / `UNCHECKED`）、`reception_identity_check_pending_only`（只有 `PENDING_IDENTIFICATION` 带）、`reception_unchecked_shaped`（未核对必无候选、不记冲突、必带来源文档）；待识别不带核对态库里不拦，理由同迁移 0006。既有待识别行回填 `CHECKED`，头注写推定依据并钉 SHA：`36aff6e9` 起读不动答不落册的待确认、`f70baa3a` 起交回 error 不落册（票 operator-channel/29 的「零行」现象），其余落待识别的路（`d783d4f8` 冲突版、`18167998` 重核冲突版、`47329657` 处置版）都在读口答复之后，唯一生产写方是 `Receptions.Save`。部分索引 `reception_unchecked_pending` 供容纳读面。处置版是待识别行、记已核对，与三条约束相容。

**门铃信封**：`OutboxIdentityRecheckDoorbell`（`identity_recheck_doorbell.go`），类型 `node-operations.unchecked-pending-identity.recorded`；ID 是 `FingerprintEventID("unchecked-pending-identity", 租户, 判断版本)`，同一版重放撞同一个 ID；分区键 `FingerprintEventID("unchecked-pending-identity-unit", 租户, 作业实物)`（分区主体登记表加一行「租户/作业实物」）；Source `idp-parcel/node-operations`、Scope 租户、Subject 判断版本；载荷只有 `tenantId` 与 `judgmentVersion`。消费：`noinbox.UncheckedPendingIdentityRecordedConsumer` 与 PS 门铃那扇门同用消费者名 `node-operations/recheck-pending-identities`（inbox 键含来源，两种门铃不相交），经 `RecheckNamedPendingIdentity` 交 `Recheck` 只看那一件；载荷缺租户或判断版本即毒丸。parcel-dispatch 的 `recheckPendingIdentitiesConsumer` 按事件类型把信封分给两扇门，外面同一层 `WithUndecidedSentinels(ErrParcelIdentityUnreadable)`：PS 仍读不动记 `dispatch.consumer_undecided`、退避重投。

**`cmd/parcel-dispatch/assemble.go` 只加了路由表一行**，不改既有行：

```go
			noinbox.UncheckedPendingIdentityRecordedEventType:      identityRechecks,
```

消费者沿用既有的 `identityRechecks`（同一个值挂两类），没有另起消费者登记。

**各读面怎样显示未核对**

- 设备答复：`identityCheck` 信息格（`CHECKED` / `UNCHECKED`），只随待识别记录出现，首答、重放与并入都带；`outcome` 仍是 `PENDING_IDENTIFICATION`，不进词表。人工重核答复同带这一格。
- 前线导入明细：未核对写「身份未核对：候选缺席不代表查无此号，待服务端重核」，记落地、不再记未决；零候选的已核对写「身份已核对：查无此号」。重跑同一文件答重放，明细仍写未核对。
- 复核目录：收寄册与待识别册各行带 `identityCheck`（只有待识别行，其余整键不出现）；待识别册的 `units` 那一半没加。
- 管理台：待识别实物区新列「身份核对」（未核对写「未核对：候选缺席不代表查无此号」、已核对写「已核对」、不带写「未带核对态：查没查过不知道」、未收录的码原样示出）；节点收寄区待识别行的「判断」一格写「待识别 · 身份未核对」；重核答复多一格 `IDENTITY_NOW_CHECKED` 的说法，算落了新版、重读列表。词放在新模块 `identity-check.ts`。
- `ParcelContainmentView`：同租户有身份核对态未核对、且没有后继（链头）的待识别此刻在未关闭单元里时，没有已关联实物在袋的包裹都答`不可归属`；已关联在袋照旧答`在`；被重核接续过的未核对行不再算。与 no-reception-identity/06 的 `succeededByDisposition` 截断并存。

**完成判据格 → 用例**

| 格 | 用例 |
|---|---|
| 真库用例一 | `cmd/parcel-api/reception_unchecked_identity_test.go::TestTheWiredReceptionFormsAnUncheckedPendingUnitWhileIdentityIsUnreadable` |
| 真库用例二 | `cmd/parcel-dispatch/node_identity_recheck_unchecked_test.go::TestTheWiredDispatchRechecksAnUncheckedPendingUnitByItsOwnDoorbell`（门铃经生产派发器与路由表；PS 仍读不动那一拍记未决、册上不追加，挪回后门铃重投落下关联、意图一封、业务时间是原收寄时刻）；路由与类型串另有 `TestAnUncheckedPendingIdentityDoorbellReachesTheRecheckConsumerThroughTheRouteTable`、`TestTheRecheckConsumerAndTheUncheckedDoorbellAgreeOnTheEventType` |
| 真库用例三 | `cmd/parcel-api/reception_unchecked_identity_test.go::TestTheWiredReceptionMergesTheSameUnitIntoItsUncheckedPendingUnit` |
| savepoint | `internal/nodeoperations/adapters/postgres/identity_savepoint_test.go` 三条（SQL 级错误不连带收寄、可读一支 RELEASE、事务外直通） |
| 分得开 / `AT-NO-051` | 编排 `TestAnUnreadableIdentityCheckFormsAPendingUnitMarkedUnchecked`、`TestAnAnsweredIdentityCheckMarksThePendingUnitChecked`；HTTP `TestTheIdentityCheckTravelsWithEveryPendingUnit`、`TestCatalogueRowsCarryTheIdentityCheck`；存储 `TestCataloguesCarryTheIdentityCheckOfPendingUnits`；前线导入 `TestFrontlineImportLandsTheReceivedRowUncheckedWhenIdentityIsUnreadable`；管理台 `identity-check.test.ts`；容纳 `TestAnUncheckedUnitInABagMakesEveryUnbaggedParcelUnattributable`、`TestAnAssociatedUnitInABagStaysContainedBesideAnUncheckedUnit`、`TestARecheckedOrForeignUncheckedUnitNoLongerBlursContainment` |
| 重核接上未核对 | `TestRecheckingAnUncheckedPendingUnitWithNoCandidateAppendsACheckedVersion`、`TestRecheckingAnUncheckedPendingUnitOnceReadableFormsTheIntake`、`TestRecheckingAnUncheckedPendingUnitThatIsStillUnreadableAppendsNothing`、`TestAppendedConflictVersionsAreMarkedChecked`；HTTP `TestAManualRecheckOfAnUncheckedUnitWithNoCandidateAnswersNowChecked`；消费门 `unchecked_pending_identity_recorded_consumer_test.go` |
| 其余 | 门铃 `identity_recheck_doorbell_test.go` 两条；存储 `reception_identity_check_test.go` 三条（往返、库面形状、迁移回填）；`TestReplayingAnUncheckedReceptionAnswersTheRecordedCheck`、`TestTheSameUnitAgainIsMergedIntoItsUncheckedPendingUnit`、`TestAnUncheckedReceptionWhoseDoorbellCannotBeRungFormsNoAnswer` |

ADR-0186 名下改钉的用例：`TestDependencyFailuresFormNoAnswer` 去掉身份核对那一格；`TestTransportProblemsCarryOnlyStableCodes` 的身份核对子用例改钉 200 未核对；`TestTheWiredReceptionFormsNoAnswerWhenIdentityIsUnreadable` 删去，由真库用例一接替；`TestFrontlineImportLeavesTheReceivedRowUndecidedWhenIdentityIsUnreadable` 改写为 `TestFrontlineImportLandsTheReceivedRowUncheckedWhenIdentityIsUnreadable`。

**先红后绿**：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs05-red.log` 有三段。第一段只写了用例，各包编译不过（`undefined: ports.IdentityRecheckDoorbellIntent` 等）。第二段只补了端口、依赖格与结果格的声明，编排包断言级红，原话如「handle: resolve parcel identity: node operations: parcel identity could not be read: parcel shipment unreachable——身份核对读不动不再不形成答案」「身份核对态 = "", want "CHECKED"」「缺重核门铃却构造成功」。第三段是管理台：用例先写、页面源码未动时 tsc 报 `Cannot find module './identity-check'`。绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/nrs05-green.log`。

**门**（钉 `ab795603`）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `internal/nodeoperations/...`、`internal/parcelshipment/...`、`cmd/parcel-api/...`、`cmd/parcel-dispatch/...`、`cmd/parcel-frontline-import/...`、`migrations/...`、`internal/architecture/...`、`tests/...`，34 个包全 ok。管理台在 Node v22.23.3 下三道门退 0（`tsc -b --noEmit`；`node scripts/run-tests.mjs` 565 过 0 败；`vite build`）。worktree 里先跑了一次 `pnpm install --frozen-lockfile`，lockfile 没动。全量 `./...` 没跑，留给推送方。

**越出地盘的改动**

- `internal/nodeoperations/application/recheck_pending_identity.go`：新结果格与「未核对 → 零候选」那一支（本票「做什么」第 5 条要的）；`RecheckOutcome` 与结果头注里按位置数格的说法改成不计数的写法。`identity_judgment.go` 只改注释。
- `internal/nodeoperations/application/dispose_identity_conflict.go`（no-reception-identity/06 的文件）：处置版不带关联的那几版记已核对、带关联的不带；它的用例夹具补门铃替身。
- `internal/nodeoperations/adapters/http/recheck_pending_identity.go`：新结果格答 201、答复加 `identityCheck`；头注随之改。
- `internal/nodeoperations/adapters/inbox/`：新消费门一份；`external_identifier_relation_formed_consumer.go` 只改消费者名那条注释。
- `internal/nodeoperations/adapters/postgres/`：新文件 `identity_recheck_doorbell.go`；`service_result_savepoint.go` 只改「只包服务结果读口」那句注释。
- `cmd/parcel-dispatch/assemble_node_identity_recheck.go`：重核消费者分两扇门。
- `internal/architecture/partition_subject_registry_test.go`：分区主体登记一行。
- `cmd/parcel-api/assemble_reception_service_results_test.go`：删去被真库用例一接替的那条旧用例；`cmd/parcel-dispatch/node_identity_recheck_test.go`、`cmd/parcel-frontline-import/intake_handoff_continuation_test.go`、`internal/nodeoperations/adapters/http/work_fact_intake_test.go`：手装的收寄依赖补门铃。
- 管理台 `identity-recheck-api.ts`、`records-api.ts`、`reception-rows.ts`、`identity-recheck.ts` 各补一格；新模块 `identity-check.ts`。
- ADR-0186 的 Status 行是**在原文之后追加**前向指针，`git diff` 上显示为这一行改动（旧文是新行的前缀），照 adr/README「部分停用」约定与 ADR-0196、0199 的先例；其余是文末追加的复核记录与新立的 Links 节，决定原文不动。

**与票面不一致处（按 ADR 办）**：票面「做什么」第 1 条写「HTTP 首答 201」。实现答 **200**：待识别一格今天就答 200，只有 `INTAKE_FORMED` 答 201（`writeOutcome` 的映射）；ADR-0199 决定五「未核对与已核对的待识别对设备是同一种处置」，状态码随核对态分叉就是让信息格改了处置。用例 `TestTheIdentityCheckTravelsWithEveryPendingUnit` 与真库用例一钉 200。

**未做**

- UC-NO-002 `AT-NO-051` 一行末尾的「ADR-0199 的翻转前提齐备之前，照 ADR-0186 不形成答案」没改：说的是翻转前那一段，仍是真话，也不在地盘。
- 待识别册答复的 `units`（按实物列链头，供处置用）没带 `identityCheck`：未核对行不记冲突，不进处置。
- parcel-dispatch 的重核装配没给身份读口包 savepoint，理由见上。
- 非作者评审与推送方在干净检出上带 DSN 跑全量，不在本作者地盘内。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `c68efed1`）

两轴合审，0 阻断。

- **Spec**：ADR-0199 决定五与决定三第三触发落到。身份读不动落「待识别 · 未核对」，接收、控制与来源照常，不交意图，同一笔自投重核门铃；门铃入不了队整笔不形成答案（一条没有门铃的未核对行只能等人工，比翻转前倒退），不吞成续办。核对态 `identity_check` 只在待识别行、未核对行无候选不记冲突带来源，既有待识别行回填已核对并在迁移头注钉 SHA 写推定依据；06 的处置版补了已核对。门铃信封 ID 由（租户，判断版本）认领、分区键按实物，NO 同一个消费者分两扇门，仍读不动记等依赖退避重投。身份读口经 `SavepointedParcelIdentityView` 隔离，两处生产装配都包。各读面（设备答复、前线导入明细、复核目录、管理台、`ParcelContainmentView`）都分得开未核对与已核对。ADR-0186 决定二、决定七按约定部分停用。票 01 点名的三条真库用例齐。
- **作者偏离票面一处，认可**：首答 200 而非票面写的 201——待识别今天就答 200，ADR-0199 决定五说未核对与已核对的待识别对设备是同一种处置，状态码不随核对态分叉。
- **Standards**：注释写为什么；新件另起文件，共享文件只追加。
- **评审修补（已做）**：UC-NO-002 `AT-NO-051` 一行末尾「翻转前提齐备之前，照 ADR-0186 不形成答案」随本票落地已过时，改为指 ADR-0199 决定五。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-nrs05`（作者开工时已 rebase 到 no-reception-identity/06 之上）加评审修补一笔，直接叠在远端 main 之上快进推送；机制清点重生成零差。
- 推送方全量一次：钉 `e2c6f836` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 147 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
