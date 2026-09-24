# 02 试算入口：面向运营的计价试算端点与管理台页

Category: enhancement
Status: draft
Blocked by: 无
出处：[spec](../spec.md) 缺口二。

## 为什么

`parcel-pricing` CONTEXT 写明试算能力进入首发（「评价对象为试算对象时，评价不得进入结算，也不得形成客户承诺……因此试算能力进入首发」），
而运营今天没有任何入口：客户问「1 公斤、这个尺寸、寄到这个邮编多少钱」，产品答不了。这是销售演示的第一个问题。

## 已有的（取证钉 main `71e41e6d`）

- 领域：`SubjectEstimate` 评价对象与 `NewEstimateSubject`（`internal/parcelpricing/domain/input.go`）；试算对象只在该次试算内有身份。
- 进程内先例：小包托运的接受前估价已经用试算对象向计价要评价（`internal/parcelshipment/adapters/parcelpricing/estimation_amount.go`）。
- 价卡解析口径已定（CONTEXT-MAP `settlement-accounting → parcel-pricing` 那条）：按主要范围、方向、计算目的与业务时点解析在用价卡，唯一命中才形成，
  零命中是`未配置`，多于一版是`适用冲突`，计价不替租户挑卡。
- 缺的：`internal/*/adapters/http` 与 `cmd/parcel-api` 无试算端点；管理台无试算页；`docs/application/` 下没有计价用例目录。

## 动手前要定的（先定行为，再编码）

1. **用例形状**：立 `UC-*`（计价尚无用例目录）写清输入（重量、尺寸、目的地、服务产品 / 客户范围、方向、计算目的、业务时点）、结果与失败边界。
   `SELL` 评价只能引用一次冻结的 `BUY` 评价（CONTEXT），试算要不要一次给两个方向、怎么给，在用例里定。
2. **留不留痕**：试算评价是只算不存，还是登进评价册（带回放与解释）但标明不进结算。影响可解释性与存储，可能要 ADR。
3. **多候选**：一票多份适用价卡时每个候选各形成评价、评价之间没有优劣；择优归 `network-routing`。试算页并排展示各候选及其解释，不替路由选。
4. **接入与准入**：操作者面端点挂哪个 Intake、隔离形态下怎么放行（与 ADR-0078 / ADR-0091 的读写面放行对照）。

## 形态

碰 Go 与 `cmd/parcel-api` 端点表，走[并行会话](../../../docs/agents/parallel-sessions.md)那条路：隔离工作树、占号 `cmd/parcel-api/endpoints.go`、
非作者评审、推送方重放。管理台页可在端点落地后另拆一张前端切片。页面的解释排法可参考 spec 里记的原型「同址多注入」比较卡（计费起点 → 目的地、
命中分区、依据、逐项费用），规则一律以本仓 CONTEXT 为准。
