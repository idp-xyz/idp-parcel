# 11 核对时判可验证交付范围：收寄来源带依据引用，PS 逐候选答范围相容、信号随范围收窄，NO 按 ADR-0212 的表认唯一，客户包裹件号缺范围时落待识别（有意翻转）

Category: enhancement
Status: in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（08、09 已进 main），分支 `claude-psxm11`，NO 迁移号 0015（0014 留给票 14）。此前 ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by:
- [08](./08-pre-advice-and-handover-note-model-and-window-register.md) 进 main：推导与册。
- [09](./09-pre-advice-submission-on-the-first-party-customer-channel.md) 进 main：翻转那一笔须同时把演示动线改成送站前先交预报（ADR-0212 决定十），没有预报口就改不了。
- 已进 main、本票在其上改的两张：[06](./06-numbers-enter-the-register-only-when-storable.md) 已进 main（`1734f2a0`），PS 按号核对读口已改收 `ExternalIdentifierLookupNumber`，本票给它加范围的客户账户；[no-reception-identity/07](../../no-reception-identity/issues/07-an-ambiguity-only-conflict-releases-when-the-declaration-is-gone.md) 已进 main（`a598db8d`，ADR-0211）：冲突来由、声明撤除门铃与只因歧义信号冲突的重核都已在 main 上，本票在它之上改身份判断。它的真库用例（`cmd/parcel-dispatch` 的 `TestTheWiredDispatchReleasesAnAmbiguityOnlyConflictOnceTheDeclarationIsGone`、`TestTheWiredDispatchKeepsConflictsTheRetiredDeclarationDidNotCause` 一类，以及 `internal/nodeoperations/application` 里冲突来由的编排用例）夹具里没有范围，翻转之后要补预报夹具才保持原意，归「有意翻转与夹具」一项里补预报夹具那一类。动手前照[并行会话](../../../docs/agents/parallel-sessions.md)看清在途分支。
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定四、五、六（前半）、七、八、十；PS CONTEXT「范围相容」「歧义信号」与「按外部标识核对包裹身份」；NO CONTEXT「实物身份与事实完整性」；UC-NO-002 步骤 4 与 `AT-NO-052`、`AT-NO-063` 至 `AT-NO-068`，UC-NO-006 `AT-NO-070` 的收寄半。
地盘：
- PS：`internal/parcelshipment/ports/external_identifier.go`、`adapters/postgres/external_identifier.go` 及测试
- NO 领域：`internal/nodeoperations/domain/payload_canonicalization.go`（来源文档新形状版本多一格；内容指纹不动）及测试
- NO 端口与适配：`internal/nodeoperations/ports/ports.go` 的 `ParcelIdentityView` 与 `ParcelIdentityAnswer` 一段，`adapters/parcelshipment/identity.go`，及测试
- NO 编排：`application/identity_judgment.go`、`receive_delivered_unit.go`、`recheck_pending_identity.go`，及测试
- NO 入口：`adapters/http/reception_payload.go`（在线口可选一格）；`cmd/parcel-frontline-import`（收寄模板可选一列）
- NO 存储与读面：`adapters/postgres/reception.go`、`review_catalogue.go`、`ports/catalogue_read.go`（核对态落册与复核目录）；`migrations/node_operations/`（核对态若单列，取当时下一个空号）
- 真库用例：`cmd/parcel-api`、`cmd/parcel-frontline-import` 下新文件，以及有意翻转或补预报夹具的既有用例
- 演示：`scripts/demo-seeds`（送站前先交预报、收寄带预报标识）与 `docs/design/synthetic-demo-journey-script.md` 相应一段
- `docs/product/MECHANISM-INVENTORY.md`（重生成）、`.scratch/at-coverage-inventory.md`

## 做什么

1. **来源一格**（ADR-0212 决定七）。
   - 收寄来源文档出新形状版本，多一格交付范围依据引用（种类与标识，可以没有）。
   - 在线收寄口与受控批量口的收寄模板各多一个可选格，原样存。
   - 内容指纹（NOC-1）不动：只在这一格上不同的重投照旧答已有结果。
   - 既有行读作没有依据。
2. **PS 读口**（决定四）。
   - 输入多一组客户账户（可空）。
   - 答复：候选照旧在租户内全部取，逐候选带范围相容；歧义信号在给了客户账户时只认这些账户的`已提交`委托，没给时照 ADR-0195。
   - 候选、范围相容与信号出自同一条语句、同一个快照。
   - 范围相容比的是关联的适用范围，不是分配方；渠道包裹件号的命中恒范围相容。
   - 不交分配方、客户账户或委托。
3. **NO 身份判断**（决定四、五）。
   - 收寄与重核先用票 08 的推导，由依据得出客户账户集合与核对态，再把集合交给 PS。
   - 认唯一只经端口答复值上的一个方法，那个方法照决定四的表；`ParcelIdentityAnswer.Unique` 换成它，编排不数候选。
   - 范围未证那一格：答`待识别实物已建立`，记候选，不记冲突，不问服务结果，不交意图，不进 ADR-0196 那一支。
   - 每一版做过核对的判断都记交付范围核对态，答复与复核目录都带它；方向依据的身份一格照待识别折。
4. **重核重算范围**（决定六前半）。
   - 按存下的依据与此刻的证据状态重算；预报的有效时窗按原收寄的业务时间判。
   - 重核对象不改（ADR-0199 决定三，经 ADR-0211 放宽）。「交付范围成立」门铃在票 12。
   - 只因歧义信号的冲突重核后，信号没了而只缺范围时，追加一版不记冲突的待识别，交付范围核对态记未证（ADR-0212 决定六新补的一格，`AT-NO-062` 末句）。
   - 有范围时，冲突来由按范围相容的候选与范围内信号分（ADR-0212 决定八）。
5. **有意翻转与夹具**。先 grep 经客户包裹件号唯一识别、而夹具里没有范围的既有用例，按意图分两类：
   - 意图是「这件能被识别」的（例如 `TestTheWiredReceptionIdentifiesAnAcceptedParcelByItsCustomerNumber`，以及一码两物、同一实物、重核、身份冲突处置各组的真库用例）：补预报夹具，保持原意。
   - 意图正是「没有范围也关联」的（例如 `TestTheWiredReceptionAssociatesOnceTheOtherDeclarationIsRejectedOrWithdrawn`）：按决定四翻转，改名、改头注。
   - 两类清单写进完成记录。修生产代码之前先红。
6. **演示动线**（决定十）。送站前经票 09 的脚本交一份预报，收寄带预报标识，与翻转同一笔落地；不这样做，演示租户的客户送站会全部落待识别。

## 不做

- 交接单的开立、归入、关闭、确认与「交付范围成立」门铃（票 12）。
- 管理台展示核对态（票 13）。
- 按范围收窄候选（ADR-0184 决定六不废）。

## 完成判据

- [x] PS 真库用例：带范围时候选仍全部交回、逐候选范围相容；渠道包裹件号恒范围相容；范围外客户账户的`已提交`声明不进信号、范围内的进；没带范围时信号照 ADR-0195；同一快照（接受与读并发不两头漏）。
- [x] NO 编排用例：决定四的表逐格，`AT-NO-063` 至 `AT-NO-068`；`AT-NO-052` 两半（没有范围答冲突，范围内声明撤回后唯一识别）。
- [x] 重核用例：范围未证的待识别，依据此后变为成立（夹具里直接改证据状态）再经人工重核，追加关联版，业务时间取原收寄时刻；预报时窗按原业务时间判，不按重核时刻。
- [x] 与 ADR-0211 的接缝：只因歧义信号的冲突在声明撤除后重核，没有范围、只经客户包裹件号命中时追加不记冲突的待识别（`AT-NO-062` 末句）；有范围时照常解开并关联；有范围时冲突来由按范围相容的候选分（范围外另有候选、范围内只差信号的，记只因歧义信号）。
- [x] 来源用例：依据引用原样保全；只在这一格上不同的重投答已有结果；既有行读作没有依据；在线口与收寄模板各一条。
- [x] 真库用例（`cmd/parcel-api`，经生产装配）：带预报的送站唯一识别并消歧多候选；没带依据的同号送站落待识别、不记冲突；`AT-NO-070` 的收寄半（撤回后依据不适用）。
- [x] 翻转与补夹具两类清单写进完成记录；先红后绿的日志路径写进完成记录。
- [x] 演示实走（`S`）：交预报、受理、送站带预报标识，答`节点收寄已形成`；不带依据再送一件同号，答待识别。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/parcelshipment/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`cmd/parcel-dispatch`、`migrations/...`、`scripts/demo-seeds/...`、`internal/architecture/...` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

L

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-psxm11`）

起点 main `0c56e842`，收口前 rebase 到 `origin/main` `2c6311f1`。授权：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」；下面「代决」各条照此。权威：ADR-0212 决定四至八、十，PS / NO CONTEXT 与 CONTEXT-MAP 相应句，UC-NO-002 `AT-NO-052`、`AT-NO-062` 至 `AT-NO-068`、UC-NO-006 `AT-NO-070`。没有立新 ADR：没遇到 ADR-0212 没裁的难逆转取舍，实施细节的取舍记在下面「代决」。证据层级 **S**：夹具、种子与演示全为 `SYN-` 合成值，隔离合成库上跑。

### 提交

| 提交 | 内容 |
|---|---|
| `dcdea565` | 实现与用例：PS 读口带范围、NO 判断表、核对态落册（迁移 `node_operations/0015`）、重核重算范围、来源文档 NRS-2、在线口与收寄模板 INTAKE-2；翻转一条、补夹具若干；演示动线同一笔改成送站前先交预报（新脚本 `receive-one-delivery.sh`、README 第 7 项、动线文档「送站预报」一段）；两份棘轮基线剪掉随本票接上的各条 |
| `e45f622b` | 演示实走记录；`submit-one-pre-advice.sh` 与 `receive-one-delivery.sh` 在仓里补执行位 |
| `a70f8285` | 机制清点重生成 |
| 本笔 | 本完成记录、AT 盘点追记十六、几处新用例的 Covers 注释补 AT 号 |

### 落点

- PS：`ports/external_identifier.go`（读口多 `deliveryScope []CustomerAccountID`；答复 `Candidates []ExternalIdentifierCandidate{Parcel, ScopeCompatible}` 加信号）；`adapters/postgres/external_identifier.go`（同一条语句）；领域 `ExternalIdentifierKind.CountsInCustomerAccountSpace` 与 `CustomerAccountSpaceKinds`（哪些类型的适用范围是客户账户编号空间，分类只在领域一处）。
- NO 领域：`payload_canonicalization.go` 的来源文档出 NRS-2，`DeliverySource.ScopeBasis`；内容指纹 `CanonicalizeDeliveryPayload` 一字未动。
- NO 端口：`ports.go` 的 `ParcelIdentityView` 多一格 `scope []CustomerAccountReference`；`ParcelIdentityAnswer` 换成 `Candidates []ParcelIdentityCandidate`、`Ambiguous`、`Scoped`，`Unique()` 换成 `Verdict()`（判断格 `ParcelIdentityVerdict` 五值）与 `Parcels()`；`ReceptionRecord.DeliveryScopeCheck`；冲突来由两格的注释照决定八改。新端口 `ports/delivery_scope_evidence.go`（`DeliveryScopeEvidence` 四问：预报、交接单、最后归入的交接单、时窗）。复核目录两行 `ReceptionCatalogueRow`、`UnidentifiedItemCatalogueRow` 各多 `DeliveryScopeCheck`。
- NO 编排：`identity_judgment.go`（`identitySubject`、`deriveScope`、判断格驱动的冲突与来由）；`receive_delivered_unit.go`（命令多 `ScopeBasis`，依赖多 `Scope`）；`recheck_pending_identity.go`（依赖多 `Scope`，重核按原收寄业务时间重算，新结果 `RecheckAmbiguityGoneScopeUnverified` / `AMBIGUITY_GONE_SCOPE_UNVERIFIED`）。
- NO 适配：`adapters/parcelshipment/identity.go`（客户账户逐字交 PS，范围相容照转，记下带没带范围）；`adapters/postgres/identity_savepoint.go`（透传）；新文件 `adapters/postgres/delivery_scope_evidence.go`（三本册读口并成一个口）；`reception.go` 与 `review_catalogue.go`（三列读写，经领域门还原）；`adapters/http/reception_payload.go`（可选 `deliveryScopeBasis`）、`receive_delivered_unit.go` 与 `recheck_pending_identity.go`（答复三键，新结果答 201）。
- 迁移 `migrations/node_operations/0015_reception_delivery_scope_check.sql`（`0014` 留给票 14，没占）。
- 装配：`cmd/parcel-api/assemble_reception.go`、`assemble_identity_recheck.go`，`cmd/parcel-dispatch/assemble_node_identity_recheck.go`，`cmd/parcel-frontline-import/intake_import.go` 各装 `nopostgres.NewDeliveryScopeEvidence`；`intake_template.go` 加 INTAKE-2。

### 范围相容怎样问 PS

- NO 先由依据推导范围（`domain.DeriveVerifiableDeliveryScope`，票 08），已证时把范围给出的客户账户逐字交给 PS；未证与未配置时一个也不交，PS 答复记「没带范围」。
- PS 一条语句：CTE 按号取此刻适用、标识包裹的关联，按包裹聚合，`bool_or(类型不在客户账户编号空间 OR 适用范围 ∈ 所给客户账户)` 即那件候选范围相容；同一件包裹经几条关联被这个号标识，任一条相容即相容。比的是 `scope` 列，不是分配方。两个数组（候选、相容）取自同一个 CTE、按同一个键定序；信号在同一条语句里收到「没带范围认全租户，带了只认 `customer_account_id ∈ 所给客户账户`」。范围为空时两个数组参数都传空数组、不传 NULL（NULL 会让「认全租户」那一支落空）。
- PS 不交任何 NO 没给过的客户账户：答复只有候选包裹、逐件的是否值与信号的是否值。
- NO 认唯一只经 `ParcelIdentityAnswer.Verdict()`：带了范围只数范围相容的候选；没带范围数全部候选，只有「恰一个、无信号」那一格再看相容（相容即唯一命中，不相容即 `SCOPE_UNVERIFIED`）。冲突来由由判断格给：多候选、只因歧义信号两格有范围时都只数范围相容的、只看范围内的信号；一码两物仍只从唯一命中那一支来。

### 核对态的存储

- 收寄册三列 `delivery_scope_check`、`delivery_scope_unverified_reason`、`delivery_scope_inapplicability`（迁移 0015），取领域词表的线上名；CHECK 守：三列各自封闭、原因只随未证且未证必带、细分只随「依据不适用」且它必带、只在收寄已形成与待识别两格上、未核对的待识别不带。不要求「必带」。
- 记在哪几版：做成了身份核对的每一版——收寄的形成与核对过的待识别、重核追加的各版。不记：未核对的待识别（没取得 PS 答复）、未收寄、待确认、并入行、ADR-0196 给先到那件追加的冲突版、身份冲突处置版，与这一格落地之前的行（不回填）。
- 读回经 `domain.NewDeliveryScopeCheck` 那道门还原（`deliveryScopeCheckFrom`），集外取值与搭配不对让整行读不出。
- 收寄答复与人工重核答复多三键 `deliveryScopeCheck`、`deliveryScopeUnverifiedReason`、`deliveryScopeInapplicability`（没记时三键都不出现）；复核目录端口两行带它。

### 在线口与前线导入的新格

- 在线收寄口载荷可选 `deliveryScopeBasis: {"kind": "DELIVERY_PRE_ADVICE" | "DELIVERY_HANDOVER_NOTE", "id": "…"}`；缺席或 `null` 即没带；给了就要给全，种类认不出、标识空、缺种类、带未知键、不是对象都答 400。标识原样进来源文档，不去空白。
- 收寄模板出 INTAKE-2：多一列 `deliveryScopeBasis`，一格写「种类:标识」（冒号之后全部是标识，交接单标识可能自带冒号），空格即没带；写了就要写全，否则整批拒。表头有这一列按 INTAKE-2 核，没有按 INTAKE-1 核；INTAKE-1 照旧认、读作没有依据，两处来源标记照用的那个版本号拼（`FTI/INTAKE-2/…`）。报告里的模板版本取这份文件的版本。`.scratch/frontline-transition-import/template.md` 补了 INTAKE-2 一节。
- 来源文档 NRS-2：多 `scope_basis_kind`、`scope_basis_id` 两键，缺席写空串；NRS-1 照旧读得回、读作没有依据；半格或种类认不出拒读。内容指纹不动：只在依据上不同的重投答已有结果，首次存下的依据不被覆盖。

### 重核怎样按当时证据重算

- 依据取「最后归入的交接单，没有归入过的取来源里的引用」（`UnitDeliveryScopeBasis.Effective`）；重核才读最后归入的交接单，收寄不读（见代决 2）。
- 证据状态（预报在不在册、撤没撤回，交接单绑定确认到哪一步）读此刻的；预报有效时窗按原收寄的业务时间（判断版本上的 `Intake.ReceivedAt`）去时窗册取生效行，不按重核时刻。用例里时窗宽限十分钟、重核时钟晚一小时照样唯一识别，时窗册每一次都按原业务时间被问；时窗行收寄之后才生效的，那一刻没有生效行，重核答未配置、不追加。
- 重核对象不改（ADR-0199 决定三经 ADR-0211 放宽）。只因歧义信号的冲突这一回判成 `SCOPE_UNVERIFIED`（信号没了、只经客户包裹件号命中、没有范围），追加一版不记冲突的待识别：候选照记、核对态未证、业务时间与控制照前版、不交意图；再响门铃时它已是不记冲突的待识别，判断不变。零候选照旧留冲突。

### 有意翻转与补夹具（两类清单）

**按决定四翻转**（意图正是「没有范围也关联」）：

- `cmd/parcel-api/assemble_reception_identity_test.go` 的 `TestTheWiredReceptionAssociatesOnceTheOtherDeclarationIsRejectedOrWithdrawn` → 改名 `TestTheWiredReceptionNeedsAScopeOnceTheOtherDeclarationIsRejectedOrWithdrawn`、改头注：声明撤回之后不带依据送站落范围未证的待识别（`AT-NO-063`），带客户账户一的预报送站才唯一识别（`AT-NO-052` 后半）。

**补预报（或交接单）夹具保持原意**（意图是「这件能被识别」或「这一格冲突在信号消失后解开」）：

- `cmd/parcel-api`（夹具 `delivery_scope_fixture_test.go` 的 `preAdviseOnRealDatabase` / `preAdviseAtNodeOnRealDatabase` / `confirmedHandoverNoteOnRealDatabase`）：`TestTheWiredReceptionIdentifiesAnAcceptedParcelByItsCustomerNumber`；`TestTheWiredReceptionFormsTheIntakeWhenServiceResultsAreUnreadable` 与 `TestTheWiredReceptionHandsOffTheIntentForTheLongestSourceIDTheIntakeAccepts`（经 `wiredIdentifiedReception`）；`TestTheWiredReceptionRollsBackWhenItsIntentIsNotHandedOff`（经 `receptionWithFailingHandoff`）；`TestTheWiredDispositionDisposesOneMarkOnTwoUnits` 与 `TestADispositionAndAnotherUnitsReceptionSerializeOnTheParcel`（经 `oneMarkOnRealAssembly`，第三件另带一份预报）；`TestTheWiredManualRecheckLandsTheSameVersionAsTheDoorbell`、`TestTheWiredManualRecheckLeavesConflictsAndSourcelessRowsAlone`、`TestARecheckAndAnotherUnitsReceptionSerializeOnTheParcel`（`receivePendingOnRealAssembly` 多一个依据参数）；`TestTheWiredReplayAfterAnIdentityConflictAnswersTheCurrentVersion`；`TestConcurrentDeliveriesOfOneMarkSerializeOnTheParcel`；`TestTheWiredReceptionTurnsOneMarkOnTwoUnitsIntoAnIdentityConflict`；`TestConcurrentDeliveriesOfOneUnitSerializeOnTheUnit` 与 `TestConcurrentDeliveriesOnTheUnitAndTheParcelLocksDoNotDeadlock`（经 `sameUnitConcurrency`）；`TestTheWiredReceptionMergesTheSameUnitAtTheSameNode`（到另一个节点那一次带那个节点的预报）。`TestTheWiredReceptionAssociatesAnOverlongNumberRegisteredBeforeTheGate` 补的是**交接单**夹具：预报列的号与入册门同一界（决定二），门前的超长号进不了预报，只能凭账户级的交接单。
- `cmd/parcel-dispatch`（夹具 `delivery_scope_fixture_test.go` 的 `preAdviseForDispatch` / `confirmedHandoverNoteForDispatch`）：`TestTheWiredDispatchRechecksAPendingUnitOnceItsMarkIsRelated`、`TestTheWiredDispatchRechecksAnUncheckedPendingUnitByItsOwnDoorbell`（预报）；`TestTheWiredDispatchReleasesAnAmbiguityOnlyConflictOnceTheDeclarationIsGone` 与 `TestTheWiredDispatchKeepsConflictsTheRetiredDeclarationDidNotCause` 的「第三份声明仍在」（**交接单**，声称持号与声明各方的客户账户、都已确认：有范围时信号只认范围内的声明，单账户预报会让另一账户的声明出范围、这一格就不再冲突，原意落空）；同一用例的「一码两物」（预报）。
- 替身（替身此后照 PS 的答法答范围相容，用例正文不动）：`internal/nodeoperations/application` 的 `identityViewDouble`——没登编号空间的候选照渠道包裹件号答、恒相容，于是 `receive_delivered_unit_test.go`、`identity_conflict_cause_test.go`（冲突来由的编排用例）、`recheck_pending_identity_test.go`、`receive_unchecked_identity_test.go`、`dispose_identity_conflict_test.go` 各用例原文不动、原意不变；`adapters/http` 的 `identityViewDouble` 与 `recheckIdentity`、`adapters/postgres` 的 `sqlIdentityView`、`adapters/parcelshipment` 的 `externalIdentifiersFake`、`cmd/parcel-frontline-import` 的 `resolvingParcelIdentityView` 同样答相容。

不属两类、只随签名或形状改的既有用例：`payload_canonicalization_test.go` 的 `TestDeliverySourceRoundTripsAsNRS1` 改名 `…AsNRS2`（新落的写 NRS-2，NRS-1 字面量照样读回）、「认不出的形状」改用 NRS-3；PS 各用例的读口多传 `nil` 范围、`Parcels` 改 `Candidates`；`identity_savepoint_test.go` 的 `Unique()` 改 `Verdict()` 并多钉一格「客户账户原样透给内层」；几张缺依赖表各多「交付范围证据」一行；`intake_handoff_continuation_test.go` 手装的依赖多一格。

### 演示动线改了什么

送站前先交预报（决定十，与翻转同一笔 `dcdea565`）：新脚本 `scripts/demo-seeds/receive-one-delivery.sh <外部标识>`（配置员令牌、上海枢纽那台合成设备，`IDP_PARCEL_PRE_ADVICE_ID` 设了就带上它）；README「首方客户渠道」第 7 项写顺序：`submit-one-shipment.sh` → `submit-one-pre-advice.sh <客户包裹参考>` → 委托接受之后带预报收寄。`docs/design/synthetic-demo-journey-script.md`「送站预报」一段把「归票 11」那句换成这段动线与答复表。种子没动：时窗（`data/node-operations/01`）票 08 已登。

实走（`S`，一次性库 `psxm11_demo`，记录在动线文档同段与 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm11-walk.log`）：`seed.sh` 退 0；共享的客户端发行方仍答 `invalid_client`，另起私有 Dex（发布在 5563，issuer 串仍是 5558）；委托经 dispatch 到`已接受`；交预报签出 `PREADV-Y56LSYUAF4N7BQWXKDMI4RKQ5Y`；带它送站答 `201 INTAKE_FORMED`、核对态 `VERIFIED`、意图一封投出；不带依据再送一件同号答 `200 PENDING_IDENTIFICATION`、不记冲突、`UNVERIFIED/NO_BASIS`；依据种类拼错答 `400 MALFORMED_REQUEST`；撤回预报后带它送站答 `UNVERIFIED/BASIS_NOT_APPLICABLE/WITHDRAWN`。实走时 `submit-one-pre-advice.sh` 没有执行位（`core.fileMode=false`，票 09 落仓时就是 100644），第一枪答 `Permission denied`、那一次不带依据收寄落了待识别，补上执行位重走。走完已停进程、删容器、删库，`psb26_walk` 未碰。

### 完成判据格 → 用例

| 格 | 用例 |
|---|---|
| PS 真库：候选全部交回、逐候选相容；渠道恒相容；信号随范围收；没带照 ADR-0195；同一快照 | `internal/parcelshipment/adapters/postgres/external_identifier_scope_test.go`：`TestAScopedCheckReturnsEveryCandidateAndAnswersCompatibilityPerCandidate`、`TestAChannelNumberHitIsAlwaysScopeCompatible`、`TestTheSignalNarrowsToTheScopeAccounts`、`TestCandidatesCompatibilityAndSignalComeFromOneSnapshot`（接受与带范围的核对逐号赛跑，任何一次答零候选又不带信号、或既有候选又带信号即红）；领域 `internal/parcelshipment/domain/external_identifier_scope_test.go::TestOnlyCustomerParcelNumbersCountInACustomerAccountNumberSpace` |
| NO 编排：决定四的表逐格，`AT-NO-063` 至 `AT-NO-068`，`AT-NO-052` 两半 | 表：`internal/nodeoperations/ports/parcel_identity_test.go::TestTheIdentityVerdictFollowsTheScopeTable`、`TestEveryCandidateIsKeptRegardlessOfScope`、`TestTheIdentityVerdictIsAClosedVocabulary`。编排 `internal/nodeoperations/application/delivery_scope_judgment_test.go`：063 `TestACustomerNumberHitWithoutAVerifiableScopeIsPendingAndUnverified`（没带依据、不在册、没列这个号、节点不符、时窗外、已撤回六格）；064 `TestAPreAdviceScopePicksItsAccountsCandidateAmongOthers`；065 `TestAScopeStillContestsWithinItsOwnAccounts`；066 `TestAScopeWithNoCompatibleCandidateIsPendingAndARecheckLaterPicksTheScopedOne`；067 `TestAChannelNumberHitIsIdentifiedWithoutAScope`；068 `TestAnUnregisteredWindowIsUnconfiguredAndJudgedWithoutAScope`；052 `TestADeclarationContestsUntilItIsGoneAndThenTheScopedHitIsUnique`；证据册读不动与未核对不记核对态 `TestScopeEvidenceFailuresFormNoAnswerAndAnUncheckedUnitCarriesNoScopeCheck`。适配 `adapters/parcelshipment/identity_scope_test.go::TestTheScopeAccountsAreAskedVerbatimAndCompatibilityIsTranslated` |
| 重核：依据后来成立再人工重核追加关联版、业务时间取原收寄时刻；时窗按原业务时间 | `TestARecheckRederivesTheScopeFromTheStoredBasisAndTheEvidenceNow`（来源带的交接单后来确认；此后归入一张已成立的交接单）、`TestARecheckJudgesThePreAdviceWindowAtTheOriginalBusinessTime` |
| 与 ADR-0211 的接缝 | 编排 `TestAnAmbiguityOnlyConflictWithoutAScopeIsReleasedIntoAnUnverifiedPendingUnit`、`TestAnAmbiguityOnlyConflictWithAScopeIsReleasedIntoAnIntake`、`TestAScopeStillContestsWithinItsOwnAccounts` 的「范围外另有候选、范围内只差信号」；命令口 `adapters/http/reception_scope_basis_test.go::TestAManualRecheckThatReleasesAnAmbiguityIntoAnUnverifiedUnitAnswersCreated`；派发真库 `cmd/parcel-dispatch/node_identity_ambiguity_release_test.go::TestTheWiredDispatchReleasesAnUnscopedAmbiguityIntoAnUnverifiedPendingUnit`（新）与补夹具后的 `TestTheWiredDispatchReleasesAnAmbiguityOnlyConflictOnceTheDeclarationIsGone`（有范围照常解开） |
| 来源：依据原样保全；只在这一格上不同的重投答已有结果；既有行读作没有依据；在线口与收寄模板各一条 | 领域 `internal/nodeoperations/domain/delivery_source_scope_basis_test.go`：`TestDeliverySourceKeepsTheScopeBasisAsNRS2`、`TestAnNRS1SourceReadsAsHavingNoScopeBasis`、`TestAHalfOrUnknownScopeBasisInTheSourceIsRefused`；编排 `TestTheScopeBasisIsPreservedAndAReplayDifferingOnlyThereAnswersTheFirstResult`；存储 `adapters/postgres/reception_delivery_scope_check_test.go::TestTheDeliveryScopeCheckAndBasisAreReadBackUnchanged`；在线口 `adapters/http/reception_scope_basis_test.go::TestWorkFactReceptionIntakeTakesAnOptionalDeliveryScopeBasis`、`TestWorkFactReceptionIntakeRefusesAMalformedDeliveryScopeBasis`；收寄模板 `cmd/parcel-frontline-import/intake_delivery_scope_test.go::TestTheIntake2TemplateCarriesAnOptionalDeliveryScopeBasis`、`TestTheIntake2TemplateRefusesAMalformedBasisOrAMismatchedVersion`、真库 `TestTheIntakeImportJudgesTheScopeFromTheTemplateBasis` |
| 真库（`cmd/parcel-api`，生产装配） | `cmd/parcel-api/reception_delivery_scope_test.go`：`TestTheWiredReceptionIdentifiesAPreAdvisedDeliveryAmongCandidates`、`TestTheWiredReceptionLeavesAnUnadvisedCustomerNumberPending`、`TestTheWiredReceptionTreatsAWithdrawnPreAdviceAsNotApplicable`（`AT-NO-070` 收寄半，撤回之前的关联不回滚） |
| 核对态存储与复核目录（「做什么」第 3 项） | `TestTheDeliveryScopeCheckAndBasisAreReadBackUnchanged`、`TestTheReceptionTableRefusesAMisshapenDeliveryScopeCheck`、`TestTheDeliveryScopeCheckMigrationLeavesExistingRowsWithoutACheck`、`TestCatalogueRowsCarryTheDeliveryScopeCheck`；答复 `adapters/http/reception_scope_basis_test.go::TestTheDeliveryScopeCheckTravelsWithTheReceptionAnswer` |
| 方向依据的身份一格照待识别折 | 不需改码（范围未证就是不记冲突的待识别）；真库面 `TestTheWiredDispatchReleasesAnUnscopedAmbiguityIntoAnUnverifiedPendingUnit` 末尾断言身份一格缺少·待识别 |

### 先红后绿

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm11-red.log`。用例先写、生产代码一行未改（钉 `0c56e842`），八个包 `[build failed]`，现象原话是「undefined: ports.ParcelIdentityCandidate」「too many arguments in call to fixture.identifiers.FindParcelsByExternalIdentifier」「command.ScopeBasis undefined (type application.ReceiveDeliveredUnitCommand has no field or method ScopeBasis)」「verified.DeliveryScopeCheck undefined」一类，日志里这类行 77 条，各包报到「too many errors」即止。第一次跑出两处用例写法错（新文件里的 `parcelsOf` 与既有用例撞名、拿二维切片比 `slices.Equal`），改写法、不改断言之后重抓这份红。
- 实现之后，既有真库用例 17 条在 `cmd/parcel-api`、4 条在 `cmd/parcel-dispatch`、3 条在 `cmd/parcel-frontline-import` 先红（前者答 `PENDING_IDENTIFICATION, want INTAKE_FORMED` 一类，后两者是手装依赖缺交付范围证据被构造期拒），按上面两类逐一处理；两份棘轮基线报这几条已不在名单上，照成因剪掉。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm11-green.log`，钉 rebase 后的 `a70f8285`。

### 门

钉 `a70f8285`（rebase 到 `2c6311f1` 之后）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 55432 DSN 以 `-p 1 -count=1` 跑 `./internal/parcelshipment/... ./internal/nodeoperations/... ./cmd/... ./migrations/... ./internal/architecture/... ./tests/... ./scripts/demo-seeds/...`，有测试的包 49 个 ok、0 FAIL。没跑全量 `./...`，留给推送方。本笔只动文档与几份新测试文件的 Covers 注释（补 AT 号），提交前重跑过 `gofmt -l .` 与相关包的 `go vet`。机制清点在同一 tip 上重生成（`a70f8285`）：node_operations 迁移 13→14，端口声明 525→526。

### 代决

1. **核对态三列分立、不回填，只记在做成身份核对的那几版**（见「核对态的存储」）。未核对的待识别不记：那一版没取得 PS 答复，记一个范围核对态会让人以为它查过；ADR-0196 追加给先到那件的冲突版与处置版不记：那几版不是对那件实物本身做的范围推导。
2. **收寄不读「最后归入的交接单」，只有重核读。** ADR-0212 决定六写「此后归入的交接单」；新的一次送站还没有「此后」，那件实物在别的送站里归入过的交接单不是这一次的证据。
3. **时窗只在预报在册时才读**，按原收寄业务时间读；预报不在册答依据不在册，与时窗无关。
4. **证据册读不动不形成答案**，不落成未核对：没推出范围就去问 PS，范围内本可唯一的一件会落成范围未证。
5. **新重核结果 `AMBIGUITY_GONE_SCOPE_UNVERIFIED`**（追加了一版，命令口答 201），不复用 `IDENTITY_NOW_CHECKED`：那一格说的是「未核对变已核对」，这一格是「冲突解开成待识别」，复核页要分得开。
6. **判断格放在端口答复值上**（`Verdict()` 五值），答复带 `Scoped` 记这次带没带范围；适配器据入参填它。
7. **PS 的范围相容按类型分类比 `scope` 列**，类型分类（哪几类的适用范围是客户账户编号空间）由 PS 领域给，SQL 只比较；同一件包裹任一条关联相容即相容。
8. **在线口用对象、模板用「种类:标识」一格**；INTAKE-1 照旧认（已填好的纸单汇总不必重抄），INTAKE-2 的来源标记用 INTAKE-2。
9. **答复平铺三键**，与既有 `identityCheck`、`identityConflictCause` 同一种写法。
10. **派发的冲突解开真库用例用交接单夹具**（声称持号与声明各方的客户账户）：理由见「补夹具」那一类的说明；**超长号用例用交接单夹具**：预报号码受入册门同一界。

### 越权风险点（待 owner 复核）

1. INTAKE-2 与 INTAKE-1 并存时，同一张纸单分别按两个版本录，来源标识不同（`FTI/INTAKE-1/…` 与 `FTI/INTAKE-2/…`），不再是重放；同一件实物在同一节点会并入已有收寄（ADR-0199 决定四），不另起控制起点，但册上多一行并入。owner 若要两版同号，改的是来源标记的拼法（去掉版本号），那要另立决定（标记里的版本号本来就是为了让列形状不说谎）。
2. 代决 2：收寄不读最后归入的交接单。owner 若认为一件实物此前归入的交接单对新的一次送站也算数，改在 `identitySubject.afterReception` 一处。
3. 代决 1：ADR-0196 追加给先到那件的冲突版不记核对态，复核页上这几版的核对态一格空着。owner 若要照抄后到那件的核对态，改在 `appendIdentityConflicts` 一处，但那会把后到那件的范围说成先到那件的。
4. 收寄与重核答复多了三键（信息格），设备照 `outcome` 出队不受影响；契约票（ADR-0140）若改名，`receive-one-delivery.sh` 一起改。

### 越出地盘的改动

- `internal/parcelshipment/domain/external_identifier.go`：类型分类两个导出（读口要它，分类归 PS 领域）。
- `internal/nodeoperations/ports/delivery_scope_evidence.go`（新端口）、`adapters/postgres/delivery_scope_evidence.go`（新适配）、`adapters/postgres/identity_savepoint.go`（签名透传）。
- `internal/nodeoperations/adapters/http/receive_delivered_unit.go`、`recheck_pending_identity.go`：答复三键与新结果的 201。
- 装配三处：`cmd/parcel-api/assemble_reception.go`、`assemble_identity_recheck.go`，`cmd/parcel-dispatch/assemble_node_identity_recheck.go`。
- `internal/architecture/production_wiring_baseline.txt`、`production_type_reachability_baseline.txt`：剪掉随本票接上的五条与九条，剪前剪后的数锚在 `0c56e842` 的检出上，见各段。
- `.scratch/frontline-transition-import/template.md`：INTAKE-2 一节。
- `scripts/demo-seeds/submit-one-pre-advice.sh`：只补执行位。
- 既有测试文件若干（见两类清单）。

### AT 盘点

`.scratch/at-coverage-inventory.md` 追记十六：`AT-NO-064` 至 `AT-NO-067` 由②入①；`AT-NO-063`、`AT-NO-068` 补齐收寄编排面，`AT-NO-070` 补收寄半边，`AT-NO-052`、`AT-NO-062` 补范围那几格。①334→338、②232→228（NO 39→43、29→25），合计不变。几条新用例的 Covers 注释补上 AT 号，扫描口径才认得出。

### 未做

- 完成判据末格「走 Go 那条路」：分支与作者自验已做；非作者评审与推送方在干净检出上带 DSN 跑全量不归作者，该格未勾。
- 运营查阅 HTTP 读面（`query_node_operations_records.go`）与管理台没带核对态：复核目录端口已带，HTTP 与页面归票 13。
- 交接单的开立、归入、关闭、确认与「交付范围成立」门铃：票 12；本票的重核已认最后归入的交接单与此刻的绑定状态，门铃一到就能接上。
- `AT-NO-070` 的重核半边没单立用例：重核走同一份推导，撤回只会让范围消失（决定六「预报没有触发」）。
- UC / CONTEXT 文档没改：ADR-0212 落地时票 07 已改好对应句，实施没有新的领域语言。
