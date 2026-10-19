# 72 管理台路由计划页列出计划各段，契约夹具补上 `legs`

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [64](./64-route-plan-carries-a-delivery-leg.md) 的实现。
Blocked by: [64](./64-route-plan-carries-a-delivery-leg.md)（`legs` 一键随它进 main）
地盘：`apps/admin-web/src/pages/network/**`（`api.ts`、`route-plans.contract.test.ts` 与路由计划页）；另有后端一处：`internal/networkrouting/adapters/http/query_route_plans_test.go` 里钉契约夹具的那条用例与 `testdata/route_plans_initial_route.json`。diff 不全在 `apps/admin-web/**`，不走前端直做流程，照常开分支。
出处：[ADR-0217](../../../docs/adr/0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md) 越权风险点 7。

## 现象

票 64 进 main 之后，`GET /route-plans?register=initial-route` 的成计划行多一键 `legs`：逐段 `reference`、`kind`（`BETWEEN_NODES` / `DELIVERY`）、`from`、`to`（交付段是交付范围）、`responsible`、`earliest`、`latest`、`windowBasis`，全是字符串；无路可走行没有这一键。

管理台的 `InitialRouteRecord` 不认它，路由计划页不显示段。契约夹具 `route_plans_initial_route.json` 刻意没放 `legs`：放进去，前端契约测试会因「TS 类型不认识的键」当场红。在这一票落地之前，「改任一侧两侧都红」对 `legs` 不成立。

## 三类

机制（查阅页如实呈现计划内容）。

## 做什么

- `api.ts`：`InitialRouteRecord` 加可缺席的 `legs`，段行类型八个字符串键；`kind` 按封闭两格原词。
- 契约：Go 侧钉夹具那条用例的成计划行带两段（一段节点间、一段交付），重写夹具；前端 `KeyShape` 加 `legs` 的行形状。两侧同一笔。
- 路由计划页：成计划行可展开逐段显示引用、种类、起讫、窗口与依据。交付段的终点标作交付范围，不当节点显示。段引用原样可复制：末端交接声明计划段时照抄它。

## 完成判据

- 三道门退 0；前端契约测试与 Go 夹具用例都绿，任一侧改 `legs` 的键名两侧都红。
- 灌过演示种子、提交一单的库上，路由计划页看得到四段，末段种类是交付、终点 `SYN-AREA-SG@1`。

## 规模

S
