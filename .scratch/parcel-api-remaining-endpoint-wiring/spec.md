# parcel-api 剩余端点编排接线

Category: enhancement
Status: in-progress

## 决策

九个业务端点里六格编排仍是 `unwired*` 占位(NO 收寄、TF 交付登记与 POD 更正、VE 追踪视图、VE 索赔、CC 外部结果)。**决定:全部接真,立五张票逐笔做**(TF 两端点共用一个处理器故共一笔)。

依据:开发主线把机制半边的 HTTP 面缺口收敛为「逐格接线」;六格对应的应用编排与读适配器全部已存在且有测试(`NewReceiveDeliveredUnitHandler`、`NewRegisterEffectiveDeliveryHandler`、`NewHandleClaimHandler`、`NewReceiveExternalResultHandler`、VE postgres `NewCustomerViews`),缺的只是组合根里的装配。接线属机制半边,按 AGENTS.md「机制现在就做」,不等租户。

## 排序约束(先于一切票)

当前工作树有 MCP-3 已收工未提交的批次(撤回编排接线 + 委托查阅垂直切片 + 前端 + 文档),提交归属待用户裁定。五张票全部要改 `cmd/parcel-api/endpoints.go`、`endpoints_test.go`、`main.go`、`unwired_orchestration.go`,与该批次足迹重叠。**该批次落提交之前,任何一票不得开工**——先动会把两笔工作缠进一棵树,提交归属没法再拆。

五张票之间无逻辑阻塞,但共享上述四个装配文件,按 `docs/agents/parallel-sessions.md` 的占号纪律**串行认领**,不并行。

## 每笔的共同形状

参照已完成的 `assemble_submission.go` 与 `assemble_withdrawal.go`:

- 新建 `cmd/parcel-api/assemble_<slice>.go`,构造该编排的真库适配器与 Outbox handoff,由 `main` 交入装配点;
- 逐条依赖缝裁决:有真适配器的接真;规则/政策/权威视图类缝若登记册尚无(参照 `.scratch/port-inventory-r25/report.md` 的端口盘点),接「显式未配置」适配器,让编排如实停在未决/未配置(ADR-0063 的分界:恢复动作从写代码变成登记参数),**不得**为通链路造默认值;
- 装配测试对真库实跑(真库适配器不实跑不推),端点表测试与装配点互为对照;
- 每票各自成 commit,commit 信息点名本票。

## 子票

1. `issues/01-no-reception-wiring.md` — NO 收寄登记接线
2. `issues/02-tf-delivery-wiring.md` — TF 交付登记与 POD 更正接线(一笔两端点)
3. `issues/03-ve-tracking-view-wiring.md` — VE 客户追踪视图读口接线
4. `issues/04-ve-claims-wiring.md` — VE 索赔受理接线
5. `issues/05-cc-external-results-wiring.md` — CC 外部结果接收接线

顺序按主链优先:收寄与交付是主链环(PN-03/PN-04),追踪视图是客户读面(admin-web 后续阶段最可能消费),索赔与关务外部结果属例外流与关务旁路,殿后。
