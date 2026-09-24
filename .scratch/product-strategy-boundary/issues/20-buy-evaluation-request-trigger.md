# 20 BUY 评价请求的触发面

Category: enhancement
Status: needs-triage——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；分诊时先定触发策略再转 ready-for-agent
Blocked by: 无
地盘：`internal/settlementaccounting`（触发策略与入口）、`cmd/parcel-api` 或 `cmd/parcel-dispatch` 的装配。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 17。[票 12](./12-sa-amount-grammars-allocation-forms-and-accounting-connectors.md) 管金额文法与形态，不管这一格。

## 现象

`cmd/parcel-api/assemble_evaluation_request.go` 的 `buildEvaluationRequestOrchestration` 头注原话「今天没有运营端点、也没有进程内触发面调它」「谁在什么业务时点为哪些发生项发起请求是产品题，触发面另票」；`main` 装它只为 fail-fast，产物即丢。编排本身（`RequestBuyEvaluationHandler`）与 PP 侧的消费门都在。

## 要裁的

1. 内置触发策略：由运输收费发生项形成那一刻逐项发起（消费 TF 的发生项交接），还是按结算周期批量发起，或两者都给、由租户显式采用其一。
2. 与 [routing-first-cut/10](../../routing-first-cut/issues/10-candidate-cost-from-leg-buy-evaluations.md) 的边界：那是路由择优时的 BUY 评价，目的与请求方不同，两路不得共用请求身份。

## 完成判据

- 生产进程有一条路发起 BUY 评价请求，触发策略有内置执行器；票 05 格 17 越过（隔离形态实测只记 `S`）。
