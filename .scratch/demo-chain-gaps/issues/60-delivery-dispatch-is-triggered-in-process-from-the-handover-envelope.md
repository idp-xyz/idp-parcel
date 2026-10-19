# 60 派送发起随交接信封在进程内逐对象发起

Category: enhancement
Status: ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无
地盘：TF 新 `application/trigger_dispatch_on_handover.go`、新 `adapters/inbox/transport_handover_registered_consumer.go`（inbox 名与 VE 那一路分家）；`cmd/parcel-dispatch/assemble.go`（交接一行改 FanOut，`veHandoverUndecidedSentinels` 注释）+ 新 `assemble_delivery_dispatch_on_handover.go`（照 parcel-api `buildDeliveryDispatchTrigger` 另写一份，不跨 main 包复用）+ 新真库用例；`cmd/parcel-api/assemble_delivery_dispatch.go` 头注「实例半边」一句改指 ADR-0216；文档 ADR-0216、UC-TF-006 触发行与 AT、TF CONTEXT「揽收、派送与交付」一句、`docs/product/PILOT-PARAMETER-REGISTER.md` 的 `PAR-NET-09` 拍频格、演示脚本。
出处：[06](./06-delivery-dispatch-has-no-in-process-trigger-and-no-recipient-leg.md)；[psb/09](../../product-strategy-boundary/issues/09-tf-fulfillment-judgment-methods-and-connectors.md)「派送发起策略」；[psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「格 13 · 派送发起」；方案「代决」C10，「需要的 ADR」ADR-0216。

## 做什么

- TF 自消费交接信封，按对象此刻在场的段调既有触发执行器；各答复的入账与重投照方案 C10。
- 新消费者认 `DISPATCH_TASK_REFERENCE_TAKEN`（[tf-face-algebra/09](../../tf-face-algebra/issues/09-dispatch-trigger-task-reference-is-a-faithful-identity.md)）。
- 端点 `/transport-fulfillment-delivery-dispatch-triggers` 保留作补拍。
- 文档：起草 ADR-0216；`PAR-NET-09` 拍频格退役；psb/09 那一项与 psb/05 格 13 指向本票。
- 进 main 的令牌次序见方案「波次与令牌」。

## 完成判据

- 派送段且派送要求各缝齐：形成一项任务；重投不重建。
- 非派送段、已离场、引用被占：入账，无任务。
- 要求缺失：入账，不重投。
- 缝读不动：登哨兵重投。
- 真库测试。
- 演示：SIN-LM 场外设备登末端交接（`segmentServiceAction=FINAL_DELIVERY`，计划段照当前演示声明；[64](./64-route-plan-carries-a-delivery-leg.md) 落地后改声明交付段）→ 派发一拍 `PUBLISHED`；再手打触发口同（段、对象）答 200 `DISPATCH_TASK_ALREADY_FORMED`；派送尝试、有效交付、PS 终局照走。

## 规模

M
