# needs-triage 分诊（2026-10-26，基于 main ee92309c6，只读）

计数：R1 2 张，R2 36 张，R3 77 张，D 2 张，U 0 张，合计 117。

## R1 挡上线（逐张）

### 1. `first-party-api/issues/04-ps-production-ownership-directory-before-the-write-switch-retires.md`
- 现象：生产形态下 PS 的生产归属目录（Directory）留空，适配器对缺席一律答`权威未确定`。
- 会撞的场景：真实租户的客户系统经首方口 `POST /v1/shipment-requests` 提交第一单，不设隔离写开关就停在 `OWNERSHIP_UNRESOLVED`；只有设合成的写开关（演示形态）才答 `SUBMITTED`。即真实租户一单也提交不进来，上线当天就撞，无绕法（写开关把合成坐标注入给任何租户，ADR-0150 要求退场）。
- 修法规模：M-L（目录的生产实现 + PS 委托受理的能力词与事实类型词要一份 ADR + 演示种子改登 + 写开关退场）。
- 证据：`cmd/parcel-api/assemble_submission.go` 约 357 行注释「Directory 生产形态留空」，仅 `isolated != nil` 才赋 `deps.Directory`；`internal/parcelshipment/adapters/pilotgovernance/production_ownership.go` 的 `governanceScope` 在 `Directory == nil` 时早退。

### 2. `docs-consistency/code/issues/97-initial-route-tie-has-no-authorized-decision-face.md`（票面自评 R2，我升 R1，拿不准）
- 现象：初始路由最低成本并列时落`路由未形成`（`CANDIDATES_TIED`），没有任何口读回并裁定。
- 会撞的场景：租户在同一线路上给两家承运商登了等价（含取整后相等）的成本，该包裹没有初始路由；之后每次复核都落「没有路由历史」入账未判，运输准备、订舱、装载全无从起；票面自己写「绕法：没有」，且 `InitialRouteNotFormedRegister` 只有 `RecordNotFormed` 写法，生产与管理台都没有读这一册的地方，运营甚至看不到卡在哪里。
- 修法规模：M（读法 + 授权决定口随 code/29 的 ADR-0284 形状 + 重判所择候选形成首个计划 + 管理台读面）。
- 证据：`internal/networkrouting/application/create_initial_route.go` 约 466 行 `case domain.RankingTied` 走 `NotFormedCandidatesTied`；`internal/networkrouting/adapters/postgres/initial_route_not_formed.go` 仅写；全仓无读该册处。code/29（改路建议的决定口）已 resolved，但它不覆盖初始路由并列。
- 降级条件：若租户上线时用确定性的差异化成本或规则包给了唯一最优，可降 R2。

## 分诊中发现的、票清单之外的同类事实（供协调方留意，未算进计数）
`cmd/parcel-api` 里撤回、取消、资料修订、补充、`/claims` 的客户 Intake 都是字面量 `UnconfiguredIntake{}`（`endpoints_withdrawal.go`、`endpoints_cancellation.go`、`endpoints_customer_amendment.go`、`endpoints_customer_supplement.go`、`endpoints_claims.go`），真实客户今天只能提交和查追踪视图，撤回、取消、更正、索赔走不通。这是上线范围问题，不在 117 张票内；相关的 dc/36、dc/44、dc/76、dc/98、dcg/102 因此都归 R3，但**客户口一接上就同时升 R2/R1**（dc/36、dc/44 票面自己写了这一条）。

## 同一个毛病可合并的
- **A. 确定性失败答成可重试未决**：dcg/114（已是合并票）已含 39、41、54、77、90；dcg/89（计费形成停住挡同分区后续信封）是同一机制的后果，宜同批。
- **B. 改路后运输准备不跟**：dcg/34（新计划已形成信封 + TF 准备消费者）与 dcg/36（再准备请求面）同一缺口；dcg/44（实测后再准备）复用同一用例，可后接。
- **C. 派送要求补齐后无自动重问**：dcg/152（无驱动）与 dcg/153（无门铃），都是票 70 的余项。
- **D. 集运袋交接与成员**：dcg/80、81、92、91 与 dc/74、dc/79 同族（单元与成员的控制、追踪、采用口径）。
- **E. 收尾旧版的替代与管理台**：dcg/148 + dcg/168（无后继退役 / 异方案让位）一个机制，dcg/149 + dcg/169 是各自的管理台表单。
- **F. 投影前身**：dcg/117 + dcg/118（118 等 ADR-0138 接受）。
- **G. 撤回/取消客户口未配置的一组**：dc/36、44、76、98，加 dc/99（部分可达）。口一接上同批做。
- **H. 门禁问询的容量增长**：dcg/170、171、172。
- **I. 复核/并列续办**：dc/97 依赖 dcg/154 第二条（地理投影取法）；做 97 时一并定 154。

## 全表（117 张）

| 票（相对 .scratch/） | 格 | 一句话理由 |
|---|---|---|
| auto-reroute-demo-reachability/issues/02-syn-vertical-run-reaches-reroute-after-lapse.md | R3 | 演示用例与runbook，替身测试已覆盖分派 |
| demo-chain-gaps/issues/102-claims-line-has-no-first-party-customer-intake.md | R3 | /claims入口字面量未配置，路径今天走不到 |
| demo-chain-gaps/issues/104-unbound-buy-cost-cards-get-no-zone-on-the-formal-evaluation.md | R2 | 未绑目录的卡正式评价ZONE_UNRESOLVED，绕法为绑目录 |
| demo-chain-gaps/issues/107-demo-notification-policy-names-the-view-disclosure-policy.md | R3 | 种子键错，但通知编排无cmd装配 |
| demo-chain-gaps/issues/109-capacity-quantities-are-whole-numbers-while-preparation-demand-is-decimal.md | R2 | 容量量只收整数，小数重量往上取整，写1.5答400 |
| demo-chain-gaps/issues/110-demo-transport-seeds-have-no-guard-for-three-leg-preparation-coverage.md | R3 | 演示班期覆盖缺守卫用例，整洁性 |
| demo-chain-gaps/issues/112-node-control-transfer-out-reference-names-only-the-handover-version.md | R3 | 转出引用指不回具体交接，控制正确性不受影响 |
| demo-chain-gaps/issues/114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md | R2 | 确定性失败答成可重试未决，响亮但占分区 |
| demo-chain-gaps/issues/117-projection-entries-are-stored-without-their-supersession.md | R3 | 投影条目不带前身，查阅少说，不答错 |
| demo-chain-gaps/issues/118-final-voidance-and-cross-kind-rejudgment-into-the-projection.md | R3 | 终局失效异种改判，罕见且ADR待接受 |
| demo-chain-gaps/issues/119-customer-tracking-view-locates-by-the-customers-own-parcel-reference.md | R3 | 按客户参考定位视图，增强 |
| demo-chain-gaps/issues/120-movement-facts-before-a-pickup-attempt-or-carrier-segment-are-not-backfilled.md | R2 | 揽收执行立的段上先到的移动事实不回补 |
| demo-chain-gaps/issues/122-handover-gate-reading-does-not-expand-consolidation-unit-members.md | R2 | 交出门禁读数不展开集运成员，被扣包裹读成不受门禁 |
| demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md | R2 | 关务同层冲突说不出对方，VE不立待确认信号 |
| demo-chain-gaps/issues/125-concurrent-same-source-results-with-different-content-answer-existing-result.md | R3 | 同源并发异内容后到者答已有，极罕见 |
| demo-chain-gaps/issues/148-commercial-version-supersession-write-face.md | R3 | 收尾替代/管理台表单，增强 |
| demo-chain-gaps/issues/149-admin-web-commercial-version-closure-forms.md | R3 | 收尾替代/管理台表单，增强 |
| demo-chain-gaps/issues/151-stalled-evaluation-queue-has-no-operator-view.md | R3 | 停着的评价请求无查阅面，读面 |
| demo-chain-gaps/issues/152-delivery-dispatch-reask-has-no-in-process-driver.md | R2 | 重问无驱动、无门铃，补齐后要人去按 |
| demo-chain-gaps/issues/153-delivery-requirement-owners-ring-no-doorbell-when-they-fill-in.md | R2 | 重问无驱动、无门铃，补齐后要人去按 |
| demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md | R2 | NR复核入账未判后，补登记不续办 |
| demo-chain-gaps/issues/156-admin-web-manual-external-fact-forms-replace-json-entry.md | R3 | 管理台逐字段表单，展示 |
| demo-chain-gaps/issues/157-integration-client-adoptions-record-the-submitting-client.md | R3 | 集成客户端族不记提交端，审计增强 |
| demo-chain-gaps/issues/159-integration-client-registration-faces-cap-the-request-body.md | R3 | 集成登记口不限请求体，仅已登记客户端可达 |
| demo-chain-gaps/issues/160-unadoption-leaves-actual-fulfillment-unformed-entries-and-revision-catch-up-lists-unadopted-segments.md | R3 | 解除采用/取消的边角，运营可见可绕 |
| demo-chain-gaps/issues/161-closing-an-agreement-version-named-by-in-flight-commissions-raises-no-prompt.md | R3 | 解除采用/取消的边角，运营可见可绕 |
| demo-chain-gaps/issues/162-commission-start-does-not-rederive-when-the-first-participation-is-corrected.md | R2 | 首个参与被更正后委托开始时点不重派生 |
| demo-chain-gaps/issues/163-a-cancellation-dated-after-the-start-has-no-correction-face.md | R3 | 解除采用/取消的边角，运营可见可绕 |
| demo-chain-gaps/issues/164-whether-a-cancellation-reached-the-provider-is-not-recorded.md | R3 | 解除采用/取消的边角，运营可见可绕 |
| demo-chain-gaps/issues/165-customs-docs-name-the-delivery-gate-preview-as-a-request-timing.md | R3 | 关务文档补写派送前问询 |
| demo-chain-gaps/issues/168-price-card-has-no-retirement-without-a-same-plan-successor.md | R2 | 价卡错登到另一方案/无后继，旧卡退不了场 |
| demo-chain-gaps/issues/169-admin-web-price-card-closure-forms.md | R3 | 收尾替代/管理台表单，增强 |
| demo-chain-gaps/issues/170-booking-answer-bounce-register-has-no-retention-or-aggregation.md | R3 | 门禁问询/挡回册无保留期或限流，容量增长 |
| demo-chain-gaps/issues/171-gate-previews-are-scoped-to-the-asking-devices-work-and-leave-a-trace.md | R3 | 门禁问询/挡回册无保留期或限流，容量增长 |
| demo-chain-gaps/issues/172-customs-judgement-register-grows-with-every-gate-preview.md | R3 | 门禁问询/挡回册无保留期或限流，容量增长 |
| demo-chain-gaps/issues/173-route-cost-basis-reads-price-cards-by-version-without-folding-closures.md | R2 | 线路成本依据按版本点读，不折价卡收尾 |
| demo-chain-gaps/issues/34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md | R2 | 改路新计划无运输准备且无再准备口 |
| demo-chain-gaps/issues/35-network-routing-route-deviation-consumes-movement-facts.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/36-tf-authorized-re-preparation-request-face.md | R2 | 改路新计划无运输准备且无再准备口 |
| demo-chain-gaps/issues/37-confirmed-booking-capacity-relationship.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/38-admin-web-transport-schedule-section-reads-connection-and-nodes.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md | R2 | 确定性失败答成可重试未决，响亮但占分区 |
| demo-chain-gaps/issues/40-capacity-pool-capacity-correction-has-no-entry.md | R2 | 容量池更正无入口，缩容形不成缺口 |
| demo-chain-gaps/issues/41-transport-commission-handoff-does-not-separate-deterministic-rejection.md | R2 | 确定性失败答成可重试未决，响亮但占分区 |
| demo-chain-gaps/issues/44-tf-re-prepares-on-node-actual-measurement.md | R3 | 实测登记册未落，准备按申报判，无实测可读 |
| demo-chain-gaps/issues/45-admin-web-transport-preparation-section.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/46-capacity-reservations-have-no-per-reservation-review-registry.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/47-capacity-demand-from-a-preparation-snapshot-is-not-read-back.md | R3 | 预占声明快照不回读核，运营声明可绕 |
| demo-chain-gaps/issues/49-booking-does-not-check-schedule-against-commission-scope.md | R3 | 票面等用户决定，本身是范围增强 |
| demo-chain-gaps/issues/51-supplier-agreement-catalogue-does-not-show-charge-conditions.md | R3 | 目录读面/规则在线登记，展示与便利 |
| demo-chain-gaps/issues/52-departure-gate-signal-rule-has-no-online-registration-or-catalogue-read.md | R3 | 目录读面/规则在线登记，展示与便利 |
| demo-chain-gaps/issues/54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md | R2 | 确定性失败答成可重试未决，响亮但占分区 |
| demo-chain-gaps/issues/55-load-assignment-revision-and-withdrawal-entries-are-not-wired.md | R2 | 装载分配撤回/改版无入口，池占着不放 |
| demo-chain-gaps/issues/56-allocation-guarantee-lapses-when-the-reservation-expires-before-departure.md | R2 | 预占先于出发到期，分配保障静默失效 |
| demo-chain-gaps/issues/58-adoption-does-not-check-commission-members-against-segment-objects.md | R3 | 票面等用户决定，本身是范围增强 |
| demo-chain-gaps/issues/72-admin-web-route-plans-shows-planned-legs.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/73-route-reassessments-register-lists-the-new-plan-legs.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/76-cancellation-charge-only-registers-at-booking-level.md | R3 | 票面等用户决定，本身是范围增强 |
| demo-chain-gaps/issues/77-charge-occurrence-fact-reads-do-not-separate-unrehydratable-rows.md | R2 | 确定性失败答成可重试未决，响亮但占分区 |
| demo-chain-gaps/issues/79-admin-web-handover-register-shows-declared-nodes.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md | R2 | 集运袋交接成员控制失真，在身控制重复累积 |
| demo-chain-gaps/issues/81-control-source-chain-recompute-has-no-disposition-face.md | R2 | 集运袋交接成员控制失真，在身控制重复累积 |
| demo-chain-gaps/issues/84-route-reassessment-register-shows-the-trigger-control-and-location.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/86-blocked-handover-gate-readings-raise-exception-signals.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/87-handover-register-shows-the-gate-reading-bases.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/89-actual-fulfillment-formation-blocks-later-envelopes-in-the-handover-and-delivery-partitions.md | R2 | 计费形成停住时挡同对象后续交接与追踪信封 |
| demo-chain-gaps/issues/90-handover-consumers-retry-deterministic-rebuild-failures-as-undecided.md | R2 | 确定性失败答成可重试未决，响亮但占分区 |
| demo-chain-gaps/issues/91-a-handover-in-that-names-no-unit-has-no-node-reconciliation.md | R3 | 交进节点对不上实物，无对账面，低频 |
| demo-chain-gaps/issues/92-a-node-intake-does-not-see-a-control-established-by-handover-in.md | R2 | 集运袋交接成员控制失真，在身控制重复累积 |
| demo-chain-gaps/issues/93-blocked-delivery-gate-readings-raise-exception-signals.md | R3 | 读面、展示或下游信号增强 |
| demo-chain-gaps/issues/99-handover-scope-summary-counts-one-object-twice-under-the-same-scope.md | R2 | 交接范围汇总同对象计两次（读面答错） |
| docs-consistency/code/100-a-control-trigger-between-a-suggestion-and-its-decision-reviews-a-plan-not-yet-effective.md | R2 | 建议与决定间控制触发晚到，整封重投至弃置 |
| docs-consistency/code/33-node-operations-has-no-work-task-batch-scan-location-condition-or-presence-check-model.md | R3 | 模型缺口、注释、非首发主链 |
| docs-consistency/code/36-withdrawn-shipment-requests-cannot-be-read-back-from-the-store.md | R3 | 撤回/取消客户口字面量未配置，走不到；接口接上即升 |
| docs-consistency/code/37-post-intake-service-disposition-decision-has-no-model.md | R3 | 模型缺口、注释、非首发主链 |
| docs-consistency/code/38-parcel-shipment-comments-follow-docs-consistency-06.md | R3 | 模型缺口、注释、非首发主链 |
| docs-consistency/code/43-regulatory-disposition-acceptance-and-alternate-journey-have-no-production-face.md | R3 | 模型缺口、注释、非首发主链 |
| docs-consistency/code/44-withdrawal-and-amendment-replays-short-circuit-after-an-undecided-round.md | R3 | 撤回/取消客户口字面量未配置，走不到；接口接上即升 |
| docs-consistency/code/51-eta-customer-visibility-judgement-is-not-modelled-or-read-by-the-customer-view.md | R2 | 客户视图ETA一维一律待确认，无判断对象 |
| docs-consistency/code/74-a-member-identified-after-the-movement-never-gets-that-movement-fact.md | R2 | 事后识别的集运成员永缺随袋出发事实 |
| docs-consistency/code/76-a-cancellation-order-conflict-has-no-resolution-decision-face.md | R3 | 撤回/取消客户口字面量未配置，走不到；接口接上即升 |
| docs-consistency/code/78-initial-route-dependency-undecided-drags-formed-siblings.md | R3 | 整洁性、可观测性或同序差异 |
| docs-consistency/code/79-ps-adoption-retries-a-consolidation-unit-envelope-as-a-missing-parcel-target.md | R2 | 集运单元信封被PS当缺包裹重投至弃置 |
| docs-consistency/code/81-savepoint-decorators-are-three-copies-of-one-helper.md | R3 | 整洁性、可观测性或同序差异 |
| docs-consistency/code/82-operator-and-integration-families-still-answer-the-legacy-problem-shape-and-the-contract-is-not-generated.md | R3 | 整洁性、可观测性或同序差异 |
| docs-consistency/code/83-multi-parcel-projection-derivation-and-acceptance-rederive-lock-markers-in-opposite-orders.md | R3 | 整洁性、可观测性或同序差异 |
| docs-consistency/code/85-a-prior-resolution-that-cannot-be-read-back-is-swallowed-without-a-provider-log.md | R3 | 整洁性、可观测性或同序差异 |
| docs-consistency/code/87-a-publication-draft-this-build-cannot-rehydrate-answers-5xx-instead-of-a-business-grade.md | R3 | 整洁性、可观测性或同序差异 |
| docs-consistency/code/90-a-pending-attempt-object-result-has-no-correction-entry.md | R2 | 待确认尝试结果无更正口，只能再上门 |
| docs-consistency/code/91-an-executor-refusal-records-no-responsibility-attribution.md | R3 | 整洁性、可观测性或同序差异 |
| docs-consistency/code/93-price-card-and-commercial-approval-gates-compare-subjects-before-the-required-grant.md | R3 | 整洁性、可观测性或同序差异 |
| docs-consistency/code/97-initial-route-tie-has-no-authorized-decision-face.md | R1 | 最低成本并列的包裹永无首个路由，无口可裁、无读面 |
| docs-consistency/code/98-a-network-delivery-final-is-formed-over-a-standing-cancellation.md | R3 | 取消口字面量未配置，走不到；接上即升 |
| docs-consistency/code/99-withdrawal-review-and-disposition-take-the-request-row-before-the-control-scope-lock.md | R2 | 拒绝/处置与接受链锁序相反，并发死锁自愈 |
| first-party-api/issues/01-customer-api-scopes-and-account-binding-cardinality.md | R3 | scope粒度/可观测/管理台提交页，增强 |
| first-party-api/issues/02-per-client-rate-limit-and-quota.md | R2 | 首方API无按client限流，一个重试环拖慢全租户 |
| first-party-api/issues/03-per-client-observability.md | R3 | scope粒度/可观测/管理台提交页，增强 |
| first-party-api/issues/04-ps-production-ownership-directory-before-the-write-switch-retires.md | R1 | 生产目录留空，任何租户首方提交停在OWNERSHIP_UNRESOLVED |
| first-party-api/issues/05-admin-web-submission-page-has-no-customer-credential.md | R3 | scope粒度/可观测/管理台提交页，增强 |
| no-consolidation-fact-provenance/issues/02-concurrent-same-source-writes-can-commit-the-losers-unit-update.md | R2 | 同源并发异内容，输家的单元更新照样提交 |
| operator-channel/issues/53-shipment-request-views-tenant-wide-operator-read-and-isolated-read-switch-retires.md | R3 | 读面、文档词条、运维重放口 |
| price-card-import/issues/07-parcel-pricing-context-gains-approval-duty-rule-and-operator-subject-terms.md | R3 | 读面、文档词条、运维重放口 |
| product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md | R3 | 产品策略伞票，含未装配路径与参考配置 |
| product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md | R3 | 产品策略伞票，含未装配路径与参考配置 |
| product-strategy-boundary/issues/08-no-current-valid-measurement-derivation.md | R3 | 产品策略伞票，含未装配路径与参考配置 |
| product-strategy-boundary/issues/09-tf-fulfillment-judgment-methods-and-connectors.md | R3 | 产品策略伞票，含未装配路径与参考配置 |
| product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md | R3 | 产品策略伞票，含未装配路径与参考配置 |
| product-strategy-boundary/issues/11-ve-eta-triage-disclosure-methods-and-notification-connector.md | R3 | 产品策略伞票，含未装配路径与参考配置 |
| product-strategy-boundary/issues/13-pg-hard-risk-detection-and-evidence-storage-connector.md | R3 | 产品策略伞票，含未装配路径与参考配置 |
| product-strategy-boundary/issues/14-pp-postal-prefix-granularity-and-public-unit-reference-configuration.md | R3 | 产品策略伞票，含未装配路径与参考配置 |
| product-strategy-boundary/issues/16-mechanism-gaps-without-a-ticket.md | D | 四项均已进main（票面Status逐项记commit） |
| product-strategy-boundary/issues/22-settlement-orchestrations-assembly-and-triggers.md | R3 | 结算余项触发/节点作证/关务授权其余格 |
| product-strategy-boundary/issues/33-intake-qualification-node-authority-has-no-registration-path.md | R3 | 结算余项触发/节点作证/关务授权其余格 |
| product-strategy-boundary/issues/35-built-in-customs-case-trigger-from-acceptance.md | R2 | 关务立案只有运营手工口，无内置派生 |
| product-strategy-boundary/issues/36-submission-authorization-business-role-face-remainder.md | R3 | 结算余项触发/节点作证/关务授权其余格 |
| routing-first-cut/issues/14-reassessment-carries-no-customs-projection-so-improvement-reroute-is-unreachable.md | D | 复核命令已携Carried到证据视图，ADR-0250改入账未判 |
| ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md | R3 | 读面、文档词条、运维重放口 |

说明：D 两张核得较粗。rfc/14：`ReassessRouteCommand.Carried` 现已随证据读取（`internal/networkrouting/application/reassess_route.go` 约 261 行），且存在 `cmd/parcel-dispatch/reassess_improvement_reroute_test.go`，票面所说「不携投影」已不成立（拿不准，未跑测试）。psb/16：票面 Status 记四项均进 main。
R2 里 dcg/34+36 与 dcg/89、dc/51 是「拿不准」的（可能 R1 或 R3）。
