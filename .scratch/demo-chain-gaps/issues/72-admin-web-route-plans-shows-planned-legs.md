# 72 管理台路由计划页列出计划各段，契约夹具补上 `legs`

Category: enhancement
Status: in-progress——2026-11-01 解冻：用户要求管理台改版「不要留下尾巴」，协调方派给线路与运力作者（分支 `claude-rd4-route`）。实现完成，待非作者评审；此前 ready-for-human（2026-10-27 冻结，分级 R3），再此前 needs-triage。
Blocked by: [64](./64-route-plan-carries-a-delivery-leg.md)（`legs` 一键随它进 main）
地盘：`apps/admin-web/src/pages/network/**`（`api.ts`、`route-plans.contract.test.ts` 与路由计划页）；另有后端一处：`internal/networkrouting/adapters/http/query_route_plans_test.go` 里钉契约夹具的那条用例与 `testdata/route_plans_initial_route.json`。diff 不全在 `apps/admin-web/**`，不走前端直做流程，照常开分支。
出处：[ADR-0217](../../../docs/adr/0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md) 越权风险点 7。

## 现象

票 64 进 main 之后，`GET /route-plans?register=initial-route` 的成计划行多一键 `legs`：逐段 `reference`、`kind`（`BETWEEN_NODES` / `DELIVERY`）、`from`、`to`（交付段是交付范围）、`responsible`、`earliest`、`latest`、`windowBasis`，全是字符串；无路可走行没有这一键。

管理台的 `InitialRouteRecord` 不认它，路由计划页不显示段。契约夹具 `route_plans_initial_route.json` 刻意没放 `legs`：放进去，前端契约测试会因「TS 类型不认识的键」当场红。在这一票落地之前，「改任一侧两侧都红」对 `legs` 不成立。

## 三类

机制（查阅页如实呈现计划内容）。

## 做什么

- `api.ts`：`InitialRouteRecord` 加可缺席的 `legs`，段行类型照端点 `plannedLegBody` 的键逐个列，值都是字符串；`kind` 按 NR `domain.PlannedLegKind` 的原词。
- 契约：Go 侧钉夹具那条用例的成计划行带两段（一段节点间、一段交付），重写夹具；前端 `KeyShape` 加 `legs` 的行形状。两侧同一笔。
- 路由计划页：成计划行可展开逐段显示引用、种类、起讫、窗口与依据。交付段的终点标作交付范围，不当节点显示。段引用原样可复制：末端交接声明计划段时照抄它。

## 完成判据

- 三道门退 0；前端契约测试与 Go 夹具用例都绿，任一侧改 `legs` 的键名两侧都红。
- 灌过演示种子、提交一单的库上，路由计划页看得到四段，末段种类是交付、终点 `SYN-AREA-SG@1`。

## 规模

S

## 完成记录

2026-11-01，作者：线路与运力作者会话（分支 `claude-rd4-route`，起点 `b2c1505c7`）。

- **后端（只动测试与夹具，生产代码不动）**：`internal/networkrouting/adapters/http/query_route_plans_test.go` 里钉契约夹具的那条用例，成计划行带上两段（一段节点间、一段交付），并断言段链转写出两段；带 `-update` 重写 `testdata/route_plans_initial_route.json`。用例里说「legs 例外不进夹具」的那段注释改成现行说法。
- **前端**：`apps/admin-web/src/pages/network/api.ts` 新增 `PlannedLegRecord`（键照端点 `plannedLegBody` 逐个列，值都是字符串），`InitialRouteRecord` 加可缺席的 `legs`；`route-plans.contract.test.ts` 的 `KeyShape` 加 `legs` 的行形状；路线规划页删掉页面里的本地类型，直接用 `InitialRouteRecord`。右栏逐段列起讫与窗口（交付段的终点说成「末端派送」，不当网点显示），技术详情逐段给段引用（单列一行、点按复制）与种类、起讫、责任、窗口依据。
- **自验**：`go build ./...`、`go vet ./internal/networkrouting/...`、`gofmt -l internal/networkrouting/` 都干净；`go test -count=1 -p 1 ./internal/networkrouting/...`（带 `IDP_PARCEL_POSTGRES_DSN`）全过；`apps/admin-web` 的 `tsc -b`、`pnpm test`、`vite build` 都过。
- **两侧都红的探针**：把夹具里段的 `windowBasis` 改名为 `windowRef`，Go 夹具用例红、前端契约测试红；还原后两侧都绿。
- **浏览器（只读）**：5181 接 19080 的演示库，路线规划页首行右栏列出四段，末段终点为「末端派送」（`SYN-AREA-SG@1`），技术详情里逐段有段引用。证据层级 `S`。
