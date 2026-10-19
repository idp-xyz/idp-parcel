# 25 履约段采用运输委托；取消委托与订舱

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [23](./23-transport-commission-submission-face.md)、[24](./24-booking-references-a-schedule-and-carrier-answers-via-integration-client.md)
地盘：`operation_decision.go`；tfhttp 新 `adopt_segment_commission.go`、`cancel_transport_commission.go`、`cancel_booking_request.go`；`application` 新 `adopt_segment_commission.go` + `CancelBooking`；`domain` 新 `segment_commission_adoption.go`；ports 新文件（含 `BookingStore.Replace`）；postgres 新表，迁移 TF 0030；装配与 `endpoints.go` 行；种子；ADR-0204（ADR-0098 加后继指针）、TF CONTEXT。
出处：[02](./02-transport-commission-has-no-command-face.md)；方案「02 委托、订舱、承运答复」D02-3、D02-6，「需要的 ADR」ADR-0204。

## 做什么

- 新运营决定 `FULFILLMENT_SEGMENT_COMMISSION_ADOPTION`：声明（段、委托）这一对，前置条件照方案 D02-6 与「需要的 ADR」ADR-0204。
- 采用即调 `MarkTransportStarted`，开始依据照方案 D02-6。
- 取消委托 `TRANSPORT_COMMISSION_CANCELLATION`、取消订舱 `BOOKING_REQUEST_CANCELLATION` 随本票开放，已开始的答法照方案 D02-6（方案 D02-3）。
- 起草 ADR-0204：部分停用 ADR-0098 决定四，范围照方案「需要的 ADR」；ADR-0098 Links 加后继指针，README 状态格注明「部分停用」。TF CONTEXT Rules 补段采用委托。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 段不在册答 `SEGMENT_NOT_FOUND`。
- 服务方不一致答冲突。
- 采用即标开始，随后取消答 `TRANSPORT_ALREADY_STARTED`（AT-TF-092）。
- 未开始的可以取消。
- 真库测试。
- 演示：用现有交接口立干线段后采用委托答 201，再取消委托答已开始。

## 规模

M
