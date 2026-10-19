# 48 订舱与承运答复没有查阅面：`待确认`、`候补`只能从两口的答复里看

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [24](./24-booking-references-a-schedule-and-carrier-answers-via-integration-client.md) 完成记录与 [ADR-0202](../../../docs/adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 文末补记风险点 15。
Blocked by: 无（文件归 tf-chain-plan「并行车道」车道 A，落地顺序随车道 A 的令牌）
地盘：TF `ports/catalogue_read.go`、`adapters/postgres/review_catalogue.go`、tfhttp `query_transport_fulfillment_records.go`（+ 各自测试）；不碰订舱与答复的写口。

## 现象

票 24 让订舱册（`transport_fulfillment.booking_request`）与承运答复册（`transport_fulfillment.booking_answer`）有了写入方，但 `GET /transport-fulfillment-records` 没有这两本册的 `registry`，管理台也没有页。运营人员提交订舱之后，只能从提交口的答复里知道它在册；承运方答没答、答的是什么，只有承运系统自己看得到答复口的回显。ADR-0202 补记写的读法（订舱名下没有任何答复读作`待确认`、当前一份是候补读作`候补`、接受量小于申请量读作部分确认）今天没有任何一处按它读。

## 做什么

- 查阅口加 `registry=booking-request`：每次订舱一行，带委托、班次、申请量与单位、申请时刻、取消时刻（若有），以及按 ADR-0202 补记读出的当前状态词与当前那一份答复的摘要（答复值、接受量、答复时刻）。
- 查阅口加 `registry=booking-answer`（或并进上一条的明细，二选一由作者定）：同一订舱名下的全部答复按答复时刻先后，候补与它的后继都在。
- 读法只在读口一处派生（ADR-0202 补记的三条），不在页面或适配器各算一遍。

## 完成判据

- 真库：一次订舱无答复读作`待确认`；候补之后读作`候补`；候补后接受读作已确认（接受量等于申请量）或部分确认（小于）；他租户读不到。
- 读口不触发判断、不写任何行（查阅的性质，ADR-0077）。

## 三类

机制。规模 S–M。
