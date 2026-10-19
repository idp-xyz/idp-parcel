# 11 核对时判可验证交付范围：收寄来源带依据引用，PS 逐候选答范围相容、信号随范围收窄，NO 按 ADR-0212 的表认唯一，客户包裹件号缺范围时落待识别（有意翻转）

Category: enhancement
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（08、09 已进 main），分支 `claude-psxm11`，NO 迁移号 0015（0014 留给票 14）。此前 ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
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

- [ ] PS 真库用例：带范围时候选仍全部交回、逐候选范围相容；渠道包裹件号恒范围相容；范围外客户账户的`已提交`声明不进信号、范围内的进；没带范围时信号照 ADR-0195；同一快照（接受与读并发不两头漏）。
- [ ] NO 编排用例：决定四的表逐格，`AT-NO-063` 至 `AT-NO-068`；`AT-NO-052` 两半（没有范围答冲突，范围内声明撤回后唯一识别）。
- [ ] 重核用例：范围未证的待识别，依据此后变为成立（夹具里直接改证据状态）再经人工重核，追加关联版，业务时间取原收寄时刻；预报时窗按原业务时间判，不按重核时刻。
- [ ] 与 ADR-0211 的接缝：只因歧义信号的冲突在声明撤除后重核，没有范围、只经客户包裹件号命中时追加不记冲突的待识别（`AT-NO-062` 末句）；有范围时照常解开并关联；有范围时冲突来由按范围相容的候选分（范围外另有候选、范围内只差信号的，记只因歧义信号）。
- [ ] 来源用例：依据引用原样保全；只在这一格上不同的重投答已有结果；既有行读作没有依据；在线口与收寄模板各一条。
- [ ] 真库用例（`cmd/parcel-api`，经生产装配）：带预报的送站唯一识别并消歧多候选；没带依据的同号送站落待识别、不记冲突；`AT-NO-070` 的收寄半（撤回后依据不适用）。
- [ ] 翻转与补夹具两类清单写进完成记录；先红后绿的日志路径写进完成记录。
- [ ] 演示实走（`S`）：交预报、受理、送站带预报标识，答`节点收寄已形成`；不带依据再送一件同号，答待识别。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/parcelshipment/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`cmd/parcel-dispatch`、`migrations/...`、`scripts/demo-seeds/...`、`internal/architecture/...` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

L
