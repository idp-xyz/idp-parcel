# 24 订舱提交（引班次）与承运答复集成客户端面

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: [23](./23-transport-commission-submission-face.md)、[16](./16-transport-schedule-templates-and-concrete-schedules-are-registered.md)
地盘：`operation_decision.go`、`integration_client.go`；tfhttp 新 `submit_booking_request.go`、`answer_booking.go`、`integration_client_intake.go`；TF 新 `adapters/accessidentity/integration_client_authenticator.go`；domain、postgres 订舱（班次引用、`WAITLISTED`）；迁移 TF 0029；`assemble_transport_commission.go`、`assemble_integration_client.go`、`endpoints.go` 行；种子 access 02/03/05/06、governance。
出处：[02](./02-transport-commission-has-no-command-face.md)；方案「02 委托、订舱、承运答复」D02-2、D02-3、D02-8、D02-9，「需要的 ADR」ADR-0202。

## 做什么

- 运营决定种类 `BOOKING_REQUEST_SUBMISSION`（方案 D02-3）；订舱必须引用在册具体班次（方案 D02-9）。
- 承运答复走集成客户端族，新外部事实类型 `CARRIER_BOOKING_ANSWER`，客户端来源身份的核对照方案 D02-2；ADR-0202 补进 ADR-0149 的列举（ADR-0149 Links 加后继指针）。
- 答复封闭集加 `WAITLISTED`；`待确认`、`待补充` 只是读法；逐对象与委托级接受记为缺口（方案 D02-9）。
- 出站订舱连接器不在本票（方案 D02-8）；确认订舱的容量关系归 [37](./37-confirmed-booking-capacity-relationship.md)。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 引用不在册班次答 `SCHEDULE_NOT_FOUND`。
- 客户端来源身份不等于服务方答不受理。
- 答复五值。
- 幂等。
- 真库测试。
- 演示：对 SZX→SIN 物化班次订舱，合成承运客户端用令牌回 `ACCEPTED`。

## 规模

L
