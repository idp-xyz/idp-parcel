# 11 演示网络作为参考配置，经 psb/03 的采用路径进入演示租户

Category: enhancement
Status: in-progress——2026-10-10 通道 2 认领（派单 `task-270b3557` ← 通道 1，重派 18:3x 未执行的 `task-728e2ecb`）；分支 `mcp2-rfc11`，基 `b57ff794`，隔离工作树 `/home/tops/workspace/idp-parcel-mcp2-rfc11`。此前：ready-for-agent
Blocked by: [psb/03](../../product-strategy-boundary/issues/03-reference-configuration-adoption-pattern.md)、08、10、12
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「演示网络作为参考配置」那一步
地盘：参考配置的存放处（psb/03 定）与演示种子；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)对应一步。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定三、四、五；psb/04 完成判据。

## 做什么

1. 一份演示网络（节点、连接、线路、服务区域、日历与截单、选首个内置排序形态的路由策略版本、线路的 BUY 价卡引用）作为参考配置随产品发布；演示租户经 psb/03 的采用路径显式采用，未采用的租户照旧`未配置`。
2. 演示租户上一票已接受的委托形成初始路由（`S`）。

## 不做

- 不进参数登记册；不代任何真实租户采用。

## 完成判据

- [ ] 演示租户上一票已接受的委托形成初始路由，不再停在路由证据未配置。
- [ ] 采用记录的依据指向参考配置版本；演示数据全为 `SYN-` 合成值，证据只记 `S`。

## 开工前取证（2026-10-10 通道 1，钉 `ed662238`；未开工，交下一个会话）

阻塞边四张（psb/03、08、10、12）此刻都已 resolved 进 main，本票不再被挡。没开工的原因是量：下面第一条把它从「换一种灌法」变成了跨七族的改动，本会话余量做不完。

- **NR 网络目录今天没有依据格。** `scripts/demo-seeds/data/network/` 十五份登记行的键只有租户、编码、版本、时区、生效期与各族自有字段（`segments`、`from_node`/`to_node`、`applicable_scope`、`target_kind`/`target_code` 等），没有一格能放 `REFCFG-1:<标识>@<版本>`；psb/03 完成记录「与样板不同的登记册」也把「NR 的版本化网络目录」列为待核。完成判据第二条要求采用记录的依据指向参考配置版本，所以要先给 NR 各族登记行补依据格（领域构造门、`network_routing` 迁移、适配器、`parcel-network-register` 载荷）。ADR-0147 决定四写「不开新端口、不开新表」，补的是既有表上的一格；格的形状与可空性归 NR owner。
- **采用入口照 ADR-0147 先例做。** 先例是 `cmd/parcel-commercial/register_registration_number_types.go` 的 `adopt` 项与 `registrationjson.RegistrationNumberTypeCommand`；`referenceconfig/` 包今天有 `Reference`、`ParseCitation`、`OpenCitation`、`Open`、`Released` 与摘要清单。`parcel-network-register` 今天只有 `-kind` 与 `-file` 两个参数。参考配置文件里不放租户与生效时点（租户取值，ADR-0146 决定三），由采用方给出。网络这一族按什么键切一份参考配置，ADR-0147 越权风险点 3 交 NR owner 定。
- **演示种子。** 网络在 `seed.sh` 第 4/7 段逐份 `-kind … -file` 灌入，本票改的就是这一段。线路要用的 BUY 成本卡 `pricing/price-card-cn-sg-cost.json`（CNY）已在 main 上登记。共享树上原先压着的第二客户 SGD 种子现场（`seed.sh` 另外四段与九份数据文件）已原样封存到本地分支 `salvage/demo-seeds-syn-account-02-wip`（`86eb081f`）并从共享树撤下，与本票的网络段不交；取回时在新的 main 上 cherry-pick 那一笔。
- **判据一的现状未量。** 演示动线文档「墙三」写的是 rfc/07 之前的成因；10、12 进 main 之后，演示租户的委托今天停在哪一格，开工时先按演示动线重走一遍取证再动手。

## 开工取证：判据一今天停在哪（2026-10-10 通道 2，钉 `090b92af`，即 main `b57ff794` 加认领笔；只记 `S`）

**怎么量的。** 55432 上一只一次性库 `idp_mcp2_rfc11_journey`，按本检出的 `scripts/demo-seeds/seed.sh` 原样灌，退 0。进程照[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「前置」起：`cmd/parcel-api` 读写两个隔离开关同取 `SYN-TENANT-01`，启动日志有 ADR-0078 与 ADR-0091 两行放行声明；`cmd/parcel-dispatch` 七个变量取脚本里的演示值。`scripts/demo-seeds/submit-one-shipment.sh` 提交一笔，答 `201 SUBMITTED`（`SHR-TSSDX7R4DSPBBWF64FKSNEGDNE`），列表与详情读回已提交。时段 11:07Z–11:08Z。

**停在哪。** 仍是 [psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 格 4，与该票 10-09 重走同一格（实测）：

- dispatch 投 `parcel-shipment.shipment-request.submitted` 三次，每次 `dispatch.consumer_undecided`，正文「acceptance chain is undecided: stage REACHABILITY_JUDGMENT, reason REACHABILITY_JUDGMENT_NOT_FORMED」；之后 outbox 里没有非终态行。
- `parcel_shipment` 的 `acceptance_processing_attempt`、`acceptance_reachability_judgment`、`acceptance_adopted_resolution` 都是零行（未决整笔回滚）；`network_routing` 的 `reachability_judgment`、`initial_route`、`line_cost_basis` 也都是零行。
- 目录库态：路由策略 `SYN-RS-CN-SG-01` v1 与线路 `SYN-LINE-CN-SG-01` v1 的适用范围都是 `SYN-SCOPE-01`，策略没声明排序形态；两个服务区域都没登覆盖；唯一一份日历挂在线路上，三格内容全空。NR 那一层的原因本次没加探针重取；库态与 10-09 探针那次相同，按 `catalogConfiguredFor` 的判法（判断时点要有适用范围等于服务目的 `NETWORK_SERVICE` 的策略版本）仍答 `NETWORK_EVIDENCE_NOT_CONFIGURED`（代码）。

**格 4 之后（代码，没走到）。** 两处挡判据一，都不在本票「做什么」里，11:2xZ 已报通道 1：

1. **格 5 · 受理前财务控制。** 种子规则包 `PRE_ACCEPTANCE_FINANCIAL_CONTROL` 那一格的时点语义仍是合成串 `SYN-ASOF-ACCEPT-TIME`，种子不登结算账户，控制金额源 `Amounts` 留空（估价方法，psb/06 第 2 项）。委托因此成不了`已接受`，接受决定那条线不会触发初始路由。
2. **初始路由的成本一格。** 成本单维排序即使只有一个合格候选也要它已计价（`RankRouteCandidates`）；ADR-0175 Consequences 原话「成本缺席时不能形成计划」。`cmd/parcel-dispatch` 的 `routeCosts` 把计价输入接成 `unconfiguredRoutePricingInput{}`，初始路由会停在 `COST_SOURCE_NOT_CONFIGURED`。[ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四第 7 条把路由时的计价输入定给 NR 侧的计价消费方适配器（预路由用客户声明），越权风险点 5「逐段怎样折成价卡区域」归 PP owner，尚未定。

所以本票做完，判据一的「不再停在路由证据未配置」可以取证，「形成初始路由」要等上面两处。本票的取证因此分两样：动线上可达性越过格 4；经生产装配的真库用例证初始路由越过`路由证据未配置`，并照实写下一个停点。

**另一处缺口，本票要补。** 「做什么」第 1 条要随参考配置发的日历与截单、线路的 BUY 价卡引用，今天只有端口行类型与表（迁移 `0015`、`0016`）。`registrationjson` 的登记行形状与登记用例都不带它们（`RegisterLineCostBases` 全仓只有测试调用），所以既有登记口登不进去。采用就是一次普通登记（ADR-0147 决定四），本票把这几格作为可选格补进既有登记口。
