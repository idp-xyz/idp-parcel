# 上线线与冻结清单（2026-10-27）

Status: Accepted——2026-10-27 用户同意（会话 tops-20 提议：「定一条硬线，线以外的票冻结；新发现不挡上线的只记清单，不立票」，用户答「同意」）。

本文件是冻结范围的单一权威。起因：开工三个月、819 张票，2026-10-19 至 10-25 一周新开 213 张、关掉约 140 张，开着的待分诊票大多来自评审、复核与审计；首发八个切片的机制半边 2026-08-28 已达标，之后的票多是「再查一遍还缺什么」，票池不会自己清空。

取证基线 main `c482b3aee`：开着的票 111 张 = 上线前必须 2 + 冻结 101 + 本次关掉 6 + 记录件 2。分级沿用 [needs-triage 分诊（2026-10-26）](./needs-triage-sweep-2026-10-26.md)。

## 上线线

可以给第一家客户看，指三条同时成立：

1. 合成演示动线能走通（下面「上线前必须」第 2 张在演练里收口）。
2. 首方五个客户口能用：补充、撤回、索赔、修订、取消（first-party-api/06–12）已进 main，演示脚本 `scripts/demo-seeds/walk-customer-command-faces.sh` 实走过。
3. 挡上线的票清零：2026-10-26 分诊的两张 R1（first-party-api/04、docs-consistency/code/97）已 resolved，加上下面「上线前必须」第 1 张。

挡上线的口径（照 [demo-chain-gaps 分诊](./demo-chain-gaps/triage-2026-10-19.md) A 组）：真实租户上线当天就撞且无绕法，或会丢数据、答错而没人看得见。

## 一、上线前必须（2 张）

| 票 | 为什么挡 |
|---|---|
| [envelope-id-bounds/issues/01-remaining-envelope-ids-still-concatenate-instance-references.md](./envelope-id-bounds/issues/01-remaining-envelope-ids-still-concatenate-instance-references.md) | 运输履约各口的信封 ID 仍原样串接租户、委托、包裹等引用；真实租户的标识一长就入不了队，交接只留一条没人重发的续办引用，主链静默断开。其余各口已改指纹，门禁 `TestEveryEnvelopeIDIsMintedAsAFingerprint` 的例外清单登着去处，余下是照做的活。 |
| [product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md](./product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) | 收口那一步：首个租户上线前的演练里把合成演示动线重走一遍，逐格确认不停在「未配置」。不是写代码，是演练。 |

## 二、冻结（101 张）

- 票面 Status 改为 `ready-for-human——2026-10-27 冻结到首个租户上线之后……`，保留「此前…」链；代理不认领，分诊扫描不再重读。`ready-for-human` 不进 `scripts/owner-review-queue.ps1` 的「等 owner 拍板」一节，冻结不给你添待裁项。
- 草稿（`draft`）与已是 `ready-for-human` 的几张票面不动，以本表为准。
- 解冻：首个租户的真实使用撞上了，或用户点名。R2 是首批候选，按租户实际用到的流程挑，例如租户用集运袋，就把 80、81、92、122 同批解冻。
- 解冻时按 2026-10-26 分诊「同一个毛病可合并的」A–G 成族做，不逐张做。

### R2：上线后尽快（30 张，首批解冻候选）

| 票 | 一句话（2026-10-26 分诊） |
|---|---|
| [demo-chain-gaps/issues/104-unbound-buy-cost-cards-get-no-zone-on-the-formal-evaluation.md](./demo-chain-gaps/issues/104-unbound-buy-cost-cards-get-no-zone-on-the-formal-evaluation.md) | 未绑目录的卡正式评价ZONE_UNRESOLVED，绕法为绑目录 |
| [demo-chain-gaps/issues/109-capacity-quantities-are-whole-numbers-while-preparation-demand-is-decimal.md](./demo-chain-gaps/issues/109-capacity-quantities-are-whole-numbers-while-preparation-demand-is-decimal.md) | 容量量只收整数，小数重量往上取整，写1.5答400 |
| [demo-chain-gaps/issues/114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md](./demo-chain-gaps/issues/114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md) | 确定性失败答成可重试未决，响亮但占分区 |
| [demo-chain-gaps/issues/120-movement-facts-before-a-pickup-attempt-or-carrier-segment-are-not-backfilled.md](./demo-chain-gaps/issues/120-movement-facts-before-a-pickup-attempt-or-carrier-segment-are-not-backfilled.md) | 揽收执行立的段上先到的移动事实不回补 |
| [demo-chain-gaps/issues/122-handover-gate-reading-does-not-expand-consolidation-unit-members.md](./demo-chain-gaps/issues/122-handover-gate-reading-does-not-expand-consolidation-unit-members.md) | 交出门禁读数不展开集运成员，被扣包裹读成不受门禁 |
| [demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md](./demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md) | 关务同层冲突说不出对方，VE不立待确认信号 |
| [demo-chain-gaps/issues/152-delivery-dispatch-reask-has-no-in-process-driver.md](./demo-chain-gaps/issues/152-delivery-dispatch-reask-has-no-in-process-driver.md) | 重问无驱动、无门铃，补齐后要人去按 |
| [demo-chain-gaps/issues/153-delivery-requirement-owners-ring-no-doorbell-when-they-fill-in.md](./demo-chain-gaps/issues/153-delivery-requirement-owners-ring-no-doorbell-when-they-fill-in.md) | 重问无驱动、无门铃，补齐后要人去按 |
| [demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md](./demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md) | NR复核入账未判后，补登记不续办 |
| [demo-chain-gaps/issues/162-commission-start-does-not-rederive-when-the-first-participation-is-corrected.md](./demo-chain-gaps/issues/162-commission-start-does-not-rederive-when-the-first-participation-is-corrected.md) | 首个参与被更正后委托开始时点不重派生 |
| [demo-chain-gaps/issues/168-price-card-has-no-retirement-without-a-same-plan-successor.md](./demo-chain-gaps/issues/168-price-card-has-no-retirement-without-a-same-plan-successor.md) | 价卡错登到另一方案/无后继，旧卡退不了场 |
| [demo-chain-gaps/issues/173-route-cost-basis-reads-price-cards-by-version-without-folding-closures.md](./demo-chain-gaps/issues/173-route-cost-basis-reads-price-cards-by-version-without-folding-closures.md) | 线路成本依据按版本点读，不折价卡收尾 |
| [demo-chain-gaps/issues/34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md](./demo-chain-gaps/issues/34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md) | 改路新计划无运输准备且无再准备口 |
| [demo-chain-gaps/issues/36-tf-authorized-re-preparation-request-face.md](./demo-chain-gaps/issues/36-tf-authorized-re-preparation-request-face.md) | 改路新计划无运输准备且无再准备口 |
| [demo-chain-gaps/issues/39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md](./demo-chain-gaps/issues/39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md) | 确定性失败答成可重试未决，响亮但占分区 |
| [demo-chain-gaps/issues/40-capacity-pool-capacity-correction-has-no-entry.md](./demo-chain-gaps/issues/40-capacity-pool-capacity-correction-has-no-entry.md) | 容量池更正无入口，缩容形不成缺口 |
| [demo-chain-gaps/issues/41-transport-commission-handoff-does-not-separate-deterministic-rejection.md](./demo-chain-gaps/issues/41-transport-commission-handoff-does-not-separate-deterministic-rejection.md) | 确定性失败答成可重试未决，响亮但占分区 |
| [demo-chain-gaps/issues/54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md](./demo-chain-gaps/issues/54-charge-occurrence-registration-does-not-separate-deterministic-agreement-check-failures.md) | 确定性失败答成可重试未决，响亮但占分区 |
| [demo-chain-gaps/issues/55-load-assignment-revision-and-withdrawal-entries-are-not-wired.md](./demo-chain-gaps/issues/55-load-assignment-revision-and-withdrawal-entries-are-not-wired.md) | 装载分配撤回/改版无入口，池占着不放 |
| [demo-chain-gaps/issues/56-allocation-guarantee-lapses-when-the-reservation-expires-before-departure.md](./demo-chain-gaps/issues/56-allocation-guarantee-lapses-when-the-reservation-expires-before-departure.md) | 预占先于出发到期，分配保障静默失效 |
| [demo-chain-gaps/issues/77-charge-occurrence-fact-reads-do-not-separate-unrehydratable-rows.md](./demo-chain-gaps/issues/77-charge-occurrence-fact-reads-do-not-separate-unrehydratable-rows.md) | 确定性失败答成可重试未决，响亮但占分区 |
| [demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md](./demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md) | 集运袋交接成员控制失真，在身控制重复累积 |
| [demo-chain-gaps/issues/81-control-source-chain-recompute-has-no-disposition-face.md](./demo-chain-gaps/issues/81-control-source-chain-recompute-has-no-disposition-face.md) | 集运袋交接成员控制失真，在身控制重复累积 |
| [demo-chain-gaps/issues/89-actual-fulfillment-formation-blocks-later-envelopes-in-the-handover-and-delivery-partitions.md](./demo-chain-gaps/issues/89-actual-fulfillment-formation-blocks-later-envelopes-in-the-handover-and-delivery-partitions.md) | 计费形成停住时挡同对象后续交接与追踪信封 |
| [demo-chain-gaps/issues/90-handover-consumers-retry-deterministic-rebuild-failures-as-undecided.md](./demo-chain-gaps/issues/90-handover-consumers-retry-deterministic-rebuild-failures-as-undecided.md) | 确定性失败答成可重试未决，响亮但占分区 |
| [demo-chain-gaps/issues/92-a-node-intake-does-not-see-a-control-established-by-handover-in.md](./demo-chain-gaps/issues/92-a-node-intake-does-not-see-a-control-established-by-handover-in.md) | 集运袋交接成员控制失真，在身控制重复累积 |
| [demo-chain-gaps/issues/99-handover-scope-summary-counts-one-object-twice-under-the-same-scope.md](./demo-chain-gaps/issues/99-handover-scope-summary-counts-one-object-twice-under-the-same-scope.md) | 交接范围汇总同对象计两次（读面答错） |
| [first-party-api/issues/02-per-client-rate-limit-and-quota.md](./first-party-api/issues/02-per-client-rate-limit-and-quota.md) | 首方API无按client限流，一个重试环拖慢全租户 |
| [no-consolidation-fact-provenance/issues/02-concurrent-same-source-writes-can-commit-the-losers-unit-update.md](./no-consolidation-fact-provenance/issues/02-concurrent-same-source-writes-can-commit-the-losers-unit-update.md) | 同源并发异内容，输家的单元更新照样提交 |
| [product-strategy-boundary/issues/35-built-in-customs-case-trigger-from-acceptance.md](./product-strategy-boundary/issues/35-built-in-customs-case-trigger-from-acceptance.md) | 关务立案只有运营手工口，无内置派生 |

### R3：不急（60 张）

| 票 | 一句话（2026-10-26 分诊） |
|---|---|
| [auto-reroute-demo-reachability/issues/02-syn-vertical-run-reaches-reroute-after-lapse.md](./auto-reroute-demo-reachability/issues/02-syn-vertical-run-reaches-reroute-after-lapse.md) | 演示用例与runbook，替身测试已覆盖分派 |
| [demo-chain-gaps/issues/107-demo-notification-policy-names-the-view-disclosure-policy.md](./demo-chain-gaps/issues/107-demo-notification-policy-names-the-view-disclosure-policy.md) | 种子键错，但通知编排无cmd装配 |
| [demo-chain-gaps/issues/110-demo-transport-seeds-have-no-guard-for-three-leg-preparation-coverage.md](./demo-chain-gaps/issues/110-demo-transport-seeds-have-no-guard-for-three-leg-preparation-coverage.md) | 演示班期覆盖缺守卫用例，整洁性 |
| [demo-chain-gaps/issues/112-node-control-transfer-out-reference-names-only-the-handover-version.md](./demo-chain-gaps/issues/112-node-control-transfer-out-reference-names-only-the-handover-version.md) | 转出引用指不回具体交接，控制正确性不受影响 |
| [demo-chain-gaps/issues/117-projection-entries-are-stored-without-their-supersession.md](./demo-chain-gaps/issues/117-projection-entries-are-stored-without-their-supersession.md) | 投影条目不带前身，查阅少说，不答错 |
| [demo-chain-gaps/issues/118-final-voidance-and-cross-kind-rejudgment-into-the-projection.md](./demo-chain-gaps/issues/118-final-voidance-and-cross-kind-rejudgment-into-the-projection.md) | 终局失效异种改判，罕见且ADR待接受 |
| [demo-chain-gaps/issues/119-customer-tracking-view-locates-by-the-customers-own-parcel-reference.md](./demo-chain-gaps/issues/119-customer-tracking-view-locates-by-the-customers-own-parcel-reference.md) | 按客户参考定位视图，增强 |
| [demo-chain-gaps/issues/125-concurrent-same-source-results-with-different-content-answer-existing-result.md](./demo-chain-gaps/issues/125-concurrent-same-source-results-with-different-content-answer-existing-result.md) | 同源并发异内容后到者答已有，极罕见 |
| [demo-chain-gaps/issues/148-commercial-version-supersession-write-face.md](./demo-chain-gaps/issues/148-commercial-version-supersession-write-face.md) | 收尾替代/管理台表单，增强 |
| [demo-chain-gaps/issues/149-admin-web-commercial-version-closure-forms.md](./demo-chain-gaps/issues/149-admin-web-commercial-version-closure-forms.md) | 收尾替代/管理台表单，增强 |
| [demo-chain-gaps/issues/151-stalled-evaluation-queue-has-no-operator-view.md](./demo-chain-gaps/issues/151-stalled-evaluation-queue-has-no-operator-view.md) | 停着的评价请求无查阅面，读面 |
| [demo-chain-gaps/issues/156-admin-web-manual-external-fact-forms-replace-json-entry.md](./demo-chain-gaps/issues/156-admin-web-manual-external-fact-forms-replace-json-entry.md) | 管理台逐字段表单，展示 |
| [demo-chain-gaps/issues/157-integration-client-adoptions-record-the-submitting-client.md](./demo-chain-gaps/issues/157-integration-client-adoptions-record-the-submitting-client.md) | 集成客户端族不记提交端，审计增强 |
| [demo-chain-gaps/issues/159-integration-client-registration-faces-cap-the-request-body.md](./demo-chain-gaps/issues/159-integration-client-registration-faces-cap-the-request-body.md) | 集成登记口不限请求体，仅已登记客户端可达 |
| [demo-chain-gaps/issues/160-unadoption-leaves-actual-fulfillment-unformed-entries-and-revision-catch-up-lists-unadopted-segments.md](./demo-chain-gaps/issues/160-unadoption-leaves-actual-fulfillment-unformed-entries-and-revision-catch-up-lists-unadopted-segments.md) | 解除采用/取消的边角，运营可见可绕 |
| [demo-chain-gaps/issues/161-closing-an-agreement-version-named-by-in-flight-commissions-raises-no-prompt.md](./demo-chain-gaps/issues/161-closing-an-agreement-version-named-by-in-flight-commissions-raises-no-prompt.md) | 解除采用/取消的边角，运营可见可绕 |
| [demo-chain-gaps/issues/163-a-cancellation-dated-after-the-start-has-no-correction-face.md](./demo-chain-gaps/issues/163-a-cancellation-dated-after-the-start-has-no-correction-face.md) | 解除采用/取消的边角，运营可见可绕 |
| [demo-chain-gaps/issues/164-whether-a-cancellation-reached-the-provider-is-not-recorded.md](./demo-chain-gaps/issues/164-whether-a-cancellation-reached-the-provider-is-not-recorded.md) | 解除采用/取消的边角，运营可见可绕 |
| [demo-chain-gaps/issues/165-customs-docs-name-the-delivery-gate-preview-as-a-request-timing.md](./demo-chain-gaps/issues/165-customs-docs-name-the-delivery-gate-preview-as-a-request-timing.md) | 关务文档补写派送前问询 |
| [demo-chain-gaps/issues/169-admin-web-price-card-closure-forms.md](./demo-chain-gaps/issues/169-admin-web-price-card-closure-forms.md) | 收尾替代/管理台表单，增强 |
| [demo-chain-gaps/issues/170-booking-answer-bounce-register-has-no-retention-or-aggregation.md](./demo-chain-gaps/issues/170-booking-answer-bounce-register-has-no-retention-or-aggregation.md) | 门禁问询/挡回册无保留期或限流，容量增长 |
| [demo-chain-gaps/issues/171-gate-previews-are-scoped-to-the-asking-devices-work-and-leave-a-trace.md](./demo-chain-gaps/issues/171-gate-previews-are-scoped-to-the-asking-devices-work-and-leave-a-trace.md) | 门禁问询/挡回册无保留期或限流，容量增长 |
| [demo-chain-gaps/issues/172-customs-judgement-register-grows-with-every-gate-preview.md](./demo-chain-gaps/issues/172-customs-judgement-register-grows-with-every-gate-preview.md) | 门禁问询/挡回册无保留期或限流，容量增长 |
| [demo-chain-gaps/issues/35-network-routing-route-deviation-consumes-movement-facts.md](./demo-chain-gaps/issues/35-network-routing-route-deviation-consumes-movement-facts.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/37-confirmed-booking-capacity-relationship.md](./demo-chain-gaps/issues/37-confirmed-booking-capacity-relationship.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/38-admin-web-transport-schedule-section-reads-connection-and-nodes.md](./demo-chain-gaps/issues/38-admin-web-transport-schedule-section-reads-connection-and-nodes.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/44-tf-re-prepares-on-node-actual-measurement.md](./demo-chain-gaps/issues/44-tf-re-prepares-on-node-actual-measurement.md) | 实测登记册未落，准备按申报判，无实测可读 |
| [demo-chain-gaps/issues/45-admin-web-transport-preparation-section.md](./demo-chain-gaps/issues/45-admin-web-transport-preparation-section.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/46-capacity-reservations-have-no-per-reservation-review-registry.md](./demo-chain-gaps/issues/46-capacity-reservations-have-no-per-reservation-review-registry.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/47-capacity-demand-from-a-preparation-snapshot-is-not-read-back.md](./demo-chain-gaps/issues/47-capacity-demand-from-a-preparation-snapshot-is-not-read-back.md) | 预占声明快照不回读核，运营声明可绕 |
| [demo-chain-gaps/issues/49-booking-does-not-check-schedule-against-commission-scope.md](./demo-chain-gaps/issues/49-booking-does-not-check-schedule-against-commission-scope.md) | 票面等用户决定，本身是范围增强 |
| [demo-chain-gaps/issues/51-supplier-agreement-catalogue-does-not-show-charge-conditions.md](./demo-chain-gaps/issues/51-supplier-agreement-catalogue-does-not-show-charge-conditions.md) | 目录读面/规则在线登记，展示与便利 |
| [demo-chain-gaps/issues/52-departure-gate-signal-rule-has-no-online-registration-or-catalogue-read.md](./demo-chain-gaps/issues/52-departure-gate-signal-rule-has-no-online-registration-or-catalogue-read.md) | 目录读面/规则在线登记，展示与便利 |
| [demo-chain-gaps/issues/58-adoption-does-not-check-commission-members-against-segment-objects.md](./demo-chain-gaps/issues/58-adoption-does-not-check-commission-members-against-segment-objects.md) | 票面等用户决定，本身是范围增强 |
| [demo-chain-gaps/issues/72-admin-web-route-plans-shows-planned-legs.md](./demo-chain-gaps/issues/72-admin-web-route-plans-shows-planned-legs.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/73-route-reassessments-register-lists-the-new-plan-legs.md](./demo-chain-gaps/issues/73-route-reassessments-register-lists-the-new-plan-legs.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/76-cancellation-charge-only-registers-at-booking-level.md](./demo-chain-gaps/issues/76-cancellation-charge-only-registers-at-booking-level.md) | 票面等用户决定，本身是范围增强 |
| [demo-chain-gaps/issues/79-admin-web-handover-register-shows-declared-nodes.md](./demo-chain-gaps/issues/79-admin-web-handover-register-shows-declared-nodes.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/84-route-reassessment-register-shows-the-trigger-control-and-location.md](./demo-chain-gaps/issues/84-route-reassessment-register-shows-the-trigger-control-and-location.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/86-blocked-handover-gate-readings-raise-exception-signals.md](./demo-chain-gaps/issues/86-blocked-handover-gate-readings-raise-exception-signals.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/87-handover-register-shows-the-gate-reading-bases.md](./demo-chain-gaps/issues/87-handover-register-shows-the-gate-reading-bases.md) | 读面、展示或下游信号增强 |
| [demo-chain-gaps/issues/91-a-handover-in-that-names-no-unit-has-no-node-reconciliation.md](./demo-chain-gaps/issues/91-a-handover-in-that-names-no-unit-has-no-node-reconciliation.md) | 交进节点对不上实物，无对账面，低频 |
| [demo-chain-gaps/issues/93-blocked-delivery-gate-readings-raise-exception-signals.md](./demo-chain-gaps/issues/93-blocked-delivery-gate-readings-raise-exception-signals.md) | 读面、展示或下游信号增强 |
| [first-party-api/issues/01-customer-api-scopes-and-account-binding-cardinality.md](./first-party-api/issues/01-customer-api-scopes-and-account-binding-cardinality.md) | scope粒度/可观测/管理台提交页，增强 |
| [first-party-api/issues/03-per-client-observability.md](./first-party-api/issues/03-per-client-observability.md) | scope粒度/可观测/管理台提交页，增强 |
| [first-party-api/issues/05-admin-web-submission-page-has-no-customer-credential.md](./first-party-api/issues/05-admin-web-submission-page-has-no-customer-credential.md) | scope粒度/可观测/管理台提交页，增强 |
| [operator-channel/issues/53-shipment-request-views-tenant-wide-operator-read-and-isolated-read-switch-retires.md](./operator-channel/issues/53-shipment-request-views-tenant-wide-operator-read-and-isolated-read-switch-retires.md) | 读面、文档词条、运维重放口 |
| [price-card-import/issues/07-parcel-pricing-context-gains-approval-duty-rule-and-operator-subject-terms.md](./price-card-import/issues/07-parcel-pricing-context-gains-approval-duty-rule-and-operator-subject-terms.md) | 读面、文档词条、运维重放口 |
| [product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md](./product-strategy-boundary/issues/06-ps-acceptance-and-label-selection-judgment-methods.md) | 产品策略伞票，含未装配路径与参考配置 |
| [product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md](./product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md) | 产品策略伞票，含未装配路径与参考配置 |
| [product-strategy-boundary/issues/08-no-current-valid-measurement-derivation.md](./product-strategy-boundary/issues/08-no-current-valid-measurement-derivation.md) | 产品策略伞票，含未装配路径与参考配置 |
| [product-strategy-boundary/issues/09-tf-fulfillment-judgment-methods-and-connectors.md](./product-strategy-boundary/issues/09-tf-fulfillment-judgment-methods-and-connectors.md) | 产品策略伞票，含未装配路径与参考配置 |
| [product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md](./product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) | 产品策略伞票，含未装配路径与参考配置 |
| [product-strategy-boundary/issues/11-ve-eta-triage-disclosure-methods-and-notification-connector.md](./product-strategy-boundary/issues/11-ve-eta-triage-disclosure-methods-and-notification-connector.md) | 产品策略伞票，含未装配路径与参考配置 |
| [product-strategy-boundary/issues/13-pg-hard-risk-detection-and-evidence-storage-connector.md](./product-strategy-boundary/issues/13-pg-hard-risk-detection-and-evidence-storage-connector.md) | 产品策略伞票，含未装配路径与参考配置 |
| [product-strategy-boundary/issues/14-pp-postal-prefix-granularity-and-public-unit-reference-configuration.md](./product-strategy-boundary/issues/14-pp-postal-prefix-granularity-and-public-unit-reference-configuration.md) | 产品策略伞票，含未装配路径与参考配置 |
| [product-strategy-boundary/issues/22-settlement-orchestrations-assembly-and-triggers.md](./product-strategy-boundary/issues/22-settlement-orchestrations-assembly-and-triggers.md) | 结算余项触发/节点作证/关务授权其余格 |
| [product-strategy-boundary/issues/33-intake-qualification-node-authority-has-no-registration-path.md](./product-strategy-boundary/issues/33-intake-qualification-node-authority-has-no-registration-path.md) | 结算余项触发/节点作证/关务授权其余格 |
| [product-strategy-boundary/issues/36-submission-authorization-business-role-face-remainder.md](./product-strategy-boundary/issues/36-submission-authorization-business-role-face-remainder.md) | 结算余项触发/节点作证/关务授权其余格 |
| [ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md](./ve-008-late-account-rederive/issues/04-ops-replay-endpoint-blocked-on-par-int-01.md) | 读面、文档词条、运维重放口 |

### 未进那次分诊的（11 张）

| 票 | 现状 |
|---|---|
| [demo-chain-gaps/issues/179-a-control-fact-segment-half-left-owing-has-no-continuation-entry.md](./demo-chain-gaps/issues/179-a-control-fact-segment-half-left-owing-has-no-continuation-entry.md) | 控制事实进段欠着的那一半无续办入口；票面自评 R3，形状已代决；票面已改冻结 |
| [demo-chain-gaps/issues/181-planned-leg-window-basis-spelling-and-reading-are-written-twice-across-nr-and-tf.md](./demo-chain-gaps/issues/181-planned-leg-window-basis-spelling-and-reading-are-written-twice-across-nr-and-tf.md) | 计划段窗口依据 NR、TF 各写一份；两侧今天一致，票面自评 R3；票面已改冻结 |
| [demo-chain-gaps/issues/158-settlement-policy-not-adopted-has-no-resumption-path.md](./demo-chain-gaps/issues/158-settlement-policy-not-adopted-has-no-resumption-path.md) | 结算政策没采用的那一拍无续办路；原等 SA、PC owner 裁，冻结后不必现在裁；票面已改冻结 |
| [no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md](./no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md) | 偏离标记半边已进 main；余下读面半边随一线作业端；票面已改冻结 |
| [no-directional-basis/issues/02-network-routing-has-no-routing-instruction-or-consolidation-compatibility-judgment.md](./no-directional-basis/issues/02-network-routing-has-no-routing-instruction-or-consolidation-compatibility-judgment.md) | NR 路由指令与集运兼容判断；随一线作业端，原排在演示动线走通之后；票面已改冻结 |
| [label-channel-service-first-release/issues/20-tracking-pull-beat.md](./label-channel-service-first-release/issues/20-tracking-pull-beat.md) | draft；随 product-strategy-boundary/09 外部轨迹连接器 |
| [label-channel-service-first-release/issues/22-rejudge-pending-facts-by-registered-rule.md](./label-channel-service-first-release/issues/22-rejudge-pending-facts-by-registered-rule.md) | draft；随票 20 |
| [operator-workspace-gaps/issues/03-customer-account-onboarding-readiness.md](./operator-workspace-gaps/issues/03-customer-account-onboarding-readiness.md) | draft；开户就绪清单——首个客户开户时若人工核对太费事，第一个解冻 |
| [pp-pricing-input-seams/issues/04-no-actual-measurement-registry-and-valid-measurements-read-port.md](./pp-pricing-input-seams/issues/04-no-actual-measurement-registry-and-valid-measurements-read-port.md) | draft；NO 实际测量登记册 |
| [admin-web-bundle-size/issues/02-icon-picker-off-main-entry-and-chunk-gate.md](./admin-web-bundle-size/issues/02-icon-picker-off-main-entry-and-chunk-gate.md) | 已是 ready-for-human（上线后再做），不改 |
| [seed-completeness/issues/03-pricing-network-and-gaps.md](./seed-completeness/issues/03-pricing-network-and-gaps.md) | block 1、2 已了；接管写侧另立 takeover-write-side，不改 |

## 三、本次关掉（6 张）

| 票 | 理由 |
|---|---|
| [product-strategy-boundary/issues/26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md](./product-strategy-boundary/issues/26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md) | 完成记录里的三笔行为代码 `45247ce0`、`21b102fd`、`15a7e77b` 都已在 main（2026-10-27 逐笔 `git merge-base --is-ancestor` 核过），只是票面一直没改 |
| [syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md](./syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md) | 本票自己写的关闭条件已满足：客户一半由 first-party-api/00 承接，该票 2026-10-19 已 resolved；操作者一半归 operator-channel |
| [seed-completeness/issues/04-runtime-demo-data.md](./seed-completeness/issues/04-runtime-demo-data.md) | 步 0 已进 main；步 1、2 用户 2026-09-29 裁决不做 |
| [operator-workspace-gaps/issues/04-price-card-template-import-per-adr-0101.md](./operator-workspace-gaps/issues/04-price-card-template-import-per-adr-0101.md) | 跟踪容器，子票 price-card-import/01–06 全部 resolved（按 issue-tracker「Complete a parent」） |
| [product-strategy-boundary/issues/16-mechanism-gaps-without-a-ticket.md](./product-strategy-boundary/issues/16-mechanism-gaps-without-a-ticket.md) | 2026-10-26 分诊定为已失效：四项均已进 main，票面逐项记了 commit |
| [routing-first-cut/issues/14-reassessment-carries-no-customs-projection-so-improvement-reroute-is-unreachable.md](./routing-first-cut/issues/14-reassessment-carries-no-customs-projection-so-improvement-reroute-is-unreachable.md) | 2026-10-26 分诊定为已失效：复核命令已把关务投影带到证据视图，ADR-0250 改为入账未判 |

## 四、记录件，不动（2 份）

| 文件 | 说明 |
|---|---|
| [docs-consistency/issues/01-adr-suspension-chains-merge-record.md](./docs-consistency/issues/01-adr-suspension-chains-merge-record.md) | 附表（Status: Accepted），不是工作票 |
| [takeover-write-side/issues/01-proposed-adr.md](./takeover-write-side/issues/01-proposed-adr.md) | 已裁（decided，ADR-0154），不是工作票 |

## 五、新发现怎么记

- 挡上线的（口径见「上线线」），照常立票，并在「一」节加一行。
- 不挡上线的，**不立票、不派代理**，在下面「记录」追加一行：日期、现象一句、出处（票或文件）、建议分级。评审里「不挡」的意见同样记在这里，不另立后续票。
- 已在做、已认领的工作做完为止，不因本文件中断。

### 记录

- 2026-10-27｜五步发布流程里批准遇到「已变更」后，再点批准仍带旧摘要、继续得「已变更」，只能「重新开始」或去「待批准发布」页处理｜`apps/admin-web/src/pages/party/PublicationDraftFlow.tsx`；[owner 复核清单](./party-commercial-uiux-backend/owner-review.md)｜R3
- 2026-10-27｜查阅员的登记与发布页签被收起后，空态与说明文字仍写「在「登记」签登记第一个」之类指向被收起页签的话｜[party-commercial-uiux-backend/04](./party-commercial-uiux-backend/issues/04-operator-grants-self-read-and-write-entries-hidden.md)「已知」｜R3
- 2026-10-27｜授予自查读口只被参与方与商业区用；计价、网络、关务、结算等区的写入口仍对查阅员摆出｜ADR-0315 越权风险点 5｜R3
- 2026-10-27｜`boundTenantOf`（`apps/admin-web/src/pages/operator-binding.ts`）对 JSON `null` 响应体会抛错，同款的 `heldGrantsOf` 已防｜party-commercial-uiux-backend/04 评审意见｜R3
- 2026-10-27｜发布载荷的逐格问题多数领域错误仍答兜底码 `INVALID`，受控批文口与各 JSON 镜像登记口的问题不带码｜ADR-0316 越权风险点 2｜R3
