# 82 操作者族、作业事实族与集成族各口仍写旧问题体，首方契约还没有描述符与生成物

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（线外，复核见 .scratch/launch-line-freeze-2026-10-27.md 第六节）：操作者族、作业事实族与集成族各口仍写旧问题体——首方族已定形，剩下两半都不改对外已发布的形状；解冻条件：首次对外发布前（ADR-0140 Consequences 的第三道门）。此前 needs-triage——R3：首方族已在首个兼容性基线之前定形（票 [code/22](./22-customer-face-http-contract-misses-accepted-adrs.md)），剩下的两半都不改对外已发布的形状。操作者族与产品自己的客户端同版本发布（ADR-0021），集成族今天没有外部调用方；生成器是首次对外发布前的一道门（ADR-0140 Consequences「首次对外发布前要过三道」）。2026-10-21 协调方据票 code/22 立（作者子代理代笔）。
Blocked by: 无
上下文：横切（`internal/platform/httpapi` 与各上下文 `adapters/http`、`tools/`、`apps/admin-web`）。
出处：票 code/22 代决与越权风险点；[ADR-0140](../../../docs/adr/0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 决定一、三、七。

## 现象

1. **问题体两种形状并存。** code/22 只把首方客户族各口与 PS 客户面四个命令口改成 RFC 9457 问题详情（`httpapi.WriteProblemWithDetail`）。操作者族、作业事实族与集成客户端族各口仍写 `{"error":{"code"}}`，媒体类型 `application/json`。ADR-0140 决定三写的是「4xx 与 5xx 一律 `application/problem+json`」「三族同用这条边界」。同包里两种形状由各处理器构造时选（PS、NO 的 `problemWriter`），各上下文的 `writeProblem` 仍各写一份。
2. **契约没有生成。** 端点描述符、`tools/api-contract`、`docs/api/generated/` 与 CI 比对一步都不存在（ADR-0140 决定一）；`parcel-api` 没有 `/docs`、`/openapi/v1.json`、`/openapi/integration-v1.json`（决定七）。首方契约今天只以处理器响应结构体与 [首方客户 API 契约](../../../docs/api/first-party-api-contract.md) 的横切规则存在。
3. **路由层的答复不是问题详情。** 路径不存在时 chi 答纯文本 `404`，处理器崩溃时 `middleware.Recoverer` 答空体 `500`；首方契约文档已把问题详情限在「各口处理器作答的 4xx/5xx」（code/22 评审 N4）。
4. **集成族没带 `/v1`。** ADR-0140 决定二把 URL 主版本延到集成客户端族，今天外部结果、监管凭证、外部资金事实各口仍无前缀。

## 三类

机制（问题体辅助、描述符与生成器、CI 比对）。产品策略：契约的发布形态（ADR-0140 已定）。租户取值：无。

## 最小的诚实修法

1. 各上下文 `adapters/http` 的 `writeProblem` 改经 `httpapi.WriteProblemWithDetail`，删掉旧形状；`apps/admin-web/src/pages/problem-body.ts` 与 `cmd/parcel-api` 各测试辅助里的旧形状分支随之删除。
2. 集成客户端族各口在装配点加 `/v1`；`cmd/parcel-api` 的前缀断言（`TestOnlyTheFirstPartyFamilyCarriesTheMajorVersion`）扩到集成族；演示种子的集成令牌脚本与演示动线随改。
3. 端点描述符（`httpapi.BusinessEndpoint` 增族一格）、`tools/api-contract` 生成三份 OpenAPI 3.1、生成物入库与 CI 重生成比对；ADR-0140 越权风险点 1 的工具链核实随锁版本那一笔。
4. `/docs` 与两份契约 JSON 入口。
5. 路由层的 `404` 与恢复中间件的 `500` 改写问题详情（`httpapi.NewWithEndpoints` 的 NotFound 与恢复处理），契约文档「问题详情」节的限定随之删去。

## 要裁的

- 1 与 2、3 是否拆票：1 是纯传输层迁移，可先做；3 改 `httpapi.BusinessEndpoint` 与 `endpoints.go`，按 ADR-0222 要避开并行改装配表的窗口。
