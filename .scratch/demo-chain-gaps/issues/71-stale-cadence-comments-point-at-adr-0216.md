# 71 两处派送发起的注释仍说「谁按拍调、拍频多大」待定，改指 ADR-0216

Category: enhancement
Status: ready-for-agent——2026-10-19 票 [60](./60-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。票 60 地盘里本有第 1 处，但该文件此刻在会话 idp-parcel-9b 的在途地盘内，票 60 没碰；第 2 处同在其地盘。
Blocked by: 无（地盘协调：两份文件都在会话 idp-parcel-9b 的在途地盘内，等它释放再动）

## 做什么

只改注释，不改行为。两句都过期：[ADR-0216](../../../docs/adr/0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md) 已定派送发起由派发进程随交接信封逐对象发起，任务成立时间取派发时钟，作业事实口留作补拍，`PAR-NET-09` 拍频格退役。

1. `cmd/parcel-api/assemble_delivery_dispatch.go` 的 `buildDeliveryDispatchTrigger` 头注，原句「谁按拍调这一口、拍频多大，是租户运营节拍的实例半边，仓内不写默认。」改为：进程内的那一拍由派发进程随交接登记信封发起（ADR-0216），这一口是补拍。
2. `internal/transportfulfillment/application/trigger_delivery_dispatch.go` 的 `TriggerDeliveryDispatchCommand` 注释，原句「谁按拍调本编排、拍频多大，随第一条派送要求缝的实施票立（ADR-0114 决定二末句），这里不预设。」改为指 ADR-0216：调用方是派发进程的派送发起那一路，以及作业事实口的补拍。

## 完成判据

- 两句改完；`go build ./...`、`gofmt -l` 干净。
- 全仓再搜「拍频」，生产代码注释里不再有说它待定的句子。

## 规模

S
