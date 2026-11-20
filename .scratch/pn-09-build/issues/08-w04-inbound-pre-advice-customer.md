# 08 `PN09-W04` 第 3 段：入库预报的货主口

Category: enhancement
Status: resolved（2026-11-20 进 main，见「进 main 记录」）
Blocked by: 07

依据：[UC-WF-003](../../../docs/application/warehouse-fulfillment/UC-WF-003-ACCEPT-INBOUND-PRE-ADVICE.md)；[ADR-0384](../../../docs/adr/0384-warehouse-access-words-customer-faces-and-the-switched-off-answer.md) 决定一、决定六、决定八；[首方契约](../../../docs/api/first-party-api-contract.md)「仓配口」。

运营代录已经接过。本段只接货主口。

## 词

一口一词，只加本段要用的：

- `INBOUND_PRE_ADVICE_SUBMISSION`
- `INBOUND_PRE_ADVICE_ARRIVAL_NOTICE`
- `INBOUND_PRE_ADVICE_WITHDRAWAL`
- `WAREHOUSE_READ`（读口共用，本段只挂入库预报的读）

准入：三个写口的能力词与事实类型词都是该口自己的字面量，不用 `CUSTOMER_COMMAND`。这是 ADR-0384 决定八，与既有客户命令口不同。`WAREHOUSE_READ` 不判准入。

来源请求键 = 前缀 + 去两端空白的接入请求标识。前缀 `inbound-pre-advice:`、`inbound-arrival-notice:`、`inbound-withdrawal:` 并入首方族保留前缀表。委托提交与送站预报的标识都不得以保留前缀开头。

## 路径

一路径一行。写口用契约里的路径。读口按契约「`/v1/warehouse/reads/` 下按对象拆开」：

- `POST /v1/warehouse/inbound-pre-advices`
- `POST /v1/warehouse/inbound-pre-advices/arrival-notices`
- `POST /v1/warehouse/inbound-pre-advices/withdrawals`
- `GET /v1/warehouse/reads/inbound-pre-advices`
- `GET /v1/warehouse/reads/inbound-pre-advice?preAdviceId=`
- `GET /v1/warehouse/reads/inbound-pre-advice-by-number?documentNumber=`

租户与货主只取自信封。载荷或查询里再带租户、货主客户账户即坏载荷。别的货主的预报与查无此预报同答。

货主读口带出途径，代录的另带依据引用，不带登记责任方。没有单据号就不出现 `documentNumber`，改带 `documentNumberAbsence`。累计实收与「另有到货待认领」本段没有来源，不要编 0 或 false。

问题详情用首方族的 `application/problem+json`，不用运营口的 `{"error":{"code"}}`。开关关着仍是 `403 WAREHOUSE_FULFILLMENT_NOT_ENABLED`，在认证之前。

答复：新建 `201`，其余业务结果 `200`。`outcome` 用预报核心已经有的字面量。

## 进展

六个货主口与四个客户面能力落地。三个写口的准入两词都是该口自己的字面量，不用 `CUSTOMER_COMMAND`。`WAREHOUSE_READ` 不判准入。前缀 `inbound-pre-advice:`、`inbound-arrival-notice:`、`inbound-withdrawal:` 进了保留前缀表。委托提交与送站预报两口都拒以保留前缀开头的标识。

出入：票面写累计实收与「另有到货待认领」本段没有来源，不要编 0 或 false。后段在同一读口上带出累计实收，没收过是把已入账的行加出来的 0；有待认领到货时多一格 `unclaimedArrival`，没有就不出现这一格。本段落地时不编这两格。

代码与测试由 Grok 4.7 子代理写，主会话拆包与验收。

验证：全仓 `go test ./...` 连真库跑过并通过。仓配各包与 `cmd/parcel-api` 整包另带 `-race` 跑过。证据层级是隔离合成 `S`。演示种子没有另造。没有非作者评审。代码写完，没有进 main。

## 进 main 记录

- 出处：分支 `cursor/wf-w03-goods` 的 `0c9fb7ba5`（PR #5，连同票 04 第 1–2 段、票 05–07 与票 09–11 一起落成）。三笔 cherry-pick 重放到 `b5e6d9ed6` 之上，无冲突，main 上内容在 `81e190099`（对应三笔 `895c401f3` / `7d96ccf52` / `81e190099`）。收口是随后这一笔。
- 评审：非作者 `/code-review` 双轴钉 `5d652d903`（相对 `8a97e4d9`）：Standards 0；Spec 3 处阻断均已于 `0c9fb7ba5` 清零。
- 全量真库测试：`scripts/test-full.sh` 对 `81e190099` 跑，`TEST_FULL_EXIT=0`；受影响包另带 `-race` 跑，`RACE_EXIT=0`。
- 种子灌一次性库：`scripts/demo-seeds/seed.sh` 对一次性 PG 16.14 容器跑，`SEED_EXIT=0`，用后删容器。
- 生成物不重算。
