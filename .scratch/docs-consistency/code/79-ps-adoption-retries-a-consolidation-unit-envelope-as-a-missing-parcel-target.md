# 79 小包托运采用与面单终局各路把集运单元的信封当「目标委托还没到」一路重投

Category: bug
Status: needs-triage——R2（上线后尽快）：整袋交接、整袋交付是常态，每一封对象是单元的揽收、交付、首次有效收寄信都会在 PS 那几路记未决、重投到失败预算耗尽，再落成发布失败，运维面上全是假的依赖故障。2026-10-21 协调方据票 [code/75](./75-tracking-projects-control-facts-on-a-consolidation-unit-as-one-parcel.md) 隔离评审立（作者子代理代笔）。
Blocked by: 无（成员是否随单元采用那一半随票 [demo-chain-gaps/80](../../demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md) 再定）
上下文：PS（小包托运），面单渠道终局判断。
出处：票 code/75 完成记录越权风险点 6。

## 现象（读码，只记 `S`）

- PS 采用场外揽收（`internal/parcelshipment/adapters/transportfulfillment/adopt_on_offsite_pickup.go`）、采用有效交付（`adopt_on_effective_delivery.go`）、面单终局判断经首次有效收寄（`judge_on_carrier_first_effective_pickup.go`）各路的头注写明「不替它猜」：载运对象串原样进包裹反查，对象是集运单元时反查不中，落各自的 `ErrParcelTargetNotFound`。
- 这一格在各路的未决名单里（`offsitePickupUndecidedSentinels`、`effectiveDeliveryUndecidedSentinels`、`labelTransactionJudgmentUndecidedSentinels`，后者经 `carrierFirstEffectivePickupJudgmentUndecidedSentinels` 引用），理由是「委托可能还没落到已接受，重投会改变结果」。对象是单元时重投不会改变结果：单元永远没有已接受委托。于是每封这样的信一路重投到失败预算耗尽。
- 全程追踪那一侧（票 code/75）已按节点作业单元册先认单元、是单元即入账不派生；两侧口径不同。

## 三类

机制。规模 S–M。租户取值：无。

## 修法

- 各路在反查包裹之前，照全程追踪同一判据先问节点作业单元册「这一件那一刻是不是集运单元」（同串的包裹号按单元认，ADR-0214 决定二、ADR-0225 决定四）；是单元就确定性入账、不采用、不判终局，不进未决名单。PS 自己立消费侧端口（ADR-0025），翻译留在 PS，判据仍是节点作业那一本册，不另立串形或前缀规则。
- 单元册读不动是续办格，进各路未决名单；答了却译不回是确定性的，不进。
- 成员是否随单元被采用（整袋交付是不是袋里每件包裹的有效交付）随票 80 的单元控制规则再定，本票不投到成员上。

## 验收

- 三路各一条：对象是节点作业册上的一袋，信封入账、不采用、不判终局，不落未决；对象串长得像袋号却不在单元册上，照旧反查委托（反查不中仍是未决）。
- 派发分区：单元册读不动落 `dispatch.consumer_undecided`，答复不一致保持 `dispatch.publish_failed`。

## 既有票

- [code/75](./75-tracking-projects-control-facts-on-a-consolidation-unit-as-one-parcel.md)、[demo-chain-gaps/80](../../demo-chain-gaps/issues/80-a-consolidation-unit-handover-does-not-move-member-control.md)
