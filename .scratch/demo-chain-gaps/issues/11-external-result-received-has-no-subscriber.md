# 11 外部结果已收信封没有订阅者，每封都撞 no_subscriber

Category: enhancement
Status: needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

外部结果落库后，`customs-compliance.external-result.received` 没有任何订阅者，每封都撞 `dispatch.no_subscriber`。这是最响的装配错误格，直到失败预算耗尽落 ABANDONED。结果本身已入库，但演示日志持续报红，放行事实也到不了 VE 或下游

## 证据（读码）

internal/customscompliance/adapters/postgres/external_result_handoff.go `externalResultEventType`；cmd/parcel-dispatch/assemble.go `wireDispatcher` 路由表没有这个类型；internal/platform/dispatch/dispatcher.go `failureCodeFor` 把 `ErrNoSubscriber` 定为最响一格。演示 VE 映射种子 visibility/01-milestone-mapping-v1.json 已登 CUSTOMS_COMPLIANCE/external-result → CUSTOMS_CLEARED，但没有消费者能产出这种事实

## 三类

机制。规模 M。

## 最小的诚实修法

接一个真接得住的消费者：VE 投影按 scope→单元→成员逐成员展开（同 `deriveDeclarationSubmissionConsumer` 的 ADR-0066 做法），只派生已形成的层；或者由 CC 的放行门禁核对来消费。接不住之前维持 no_subscriber，不登记空消费者

## 要裁的

谁消费外部结果：VE、CC 门禁核对，还是两者都要？演示映射把 external-result 一律映成 CUSTOMS_CLEARED，与 CC CONTEXT「不得用一个清关成功状态覆盖各层」相冲，应改成只映放行层，归 VE owner 定

## 既有票

none（.scratch/outbox-handoff-consumption-map/report.md 记为「应有但未开」，没有立票）
