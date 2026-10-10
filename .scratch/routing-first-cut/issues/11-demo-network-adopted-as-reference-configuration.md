# 11 演示网络作为参考配置，经 psb/03 的采用路径进入演示租户

Category: enhancement
Status: 完工，待评审与重放——2026-10-10 通道 2 收口（派单 `task-270b3557` ← 通道 1）：分支 `mcp2-rfc11` 基 `b57ff794`，代码 tip `bdee59cd`，清点 `ee2143ad`，完成记录见文末；判据二 ✅，判据一 ◑（初始路由越过了「路由证据未配置」；要形成初始路由，还差两处本票之外的格）；非作者评审预定通道 3，作者不自评；重放进 main 由通道 1 做。此前：in-progress——2026-10-10 通道 2 认领（派单 `task-270b3557` ← 通道 1，重派 18:3x 未执行的 `task-728e2ecb`）；分支 `mcp2-rfc11`，基 `b57ff794`，隔离工作树 `/home/tops/workspace/idp-parcel-mcp2-rfc11`。更早：ready-for-agent
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

- [ ] 演示租户上一票已接受的委托形成初始路由，不再停在路由证据未配置。（◑：后半成立，前半卡在本票之外的两格，见完成记录）
- [x] 采用记录的依据指向参考配置版本；演示数据全为 `SYN-` 合成值，证据只记 `S`。

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

**格 4 之后（代码，没走到）。** 两处挡判据一，都不在本票「做什么」里，11:1xZ 已报通道 1：

1. **格 5 · 受理前财务控制。** 种子规则包 `PRE_ACCEPTANCE_FINANCIAL_CONTROL` 那一格的时点语义仍是合成串 `SYN-ASOF-ACCEPT-TIME`，种子不登结算账户，控制金额源 `Amounts` 留空（估价方法，psb/06 第 2 项）。委托因此成不了`已接受`，接受决定那条线不会触发初始路由。
2. **初始路由的成本一格。** 成本单维排序即使只有一个合格候选也要它已计价（`RankRouteCandidates`）；ADR-0175 Consequences 原话「成本缺席时不能形成计划」。`cmd/parcel-dispatch` 的 `routeCosts` 把计价输入接成 `unconfiguredRoutePricingInput{}`，初始路由会停在 `COST_SOURCE_NOT_CONFIGURED`。[ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四第 7 条把路由时的计价输入定给 NR 侧的计价消费方适配器（预路由用客户声明），越权风险点 5「逐段怎样折成价卡区域」归 PP owner，尚未定。

所以本票做完，判据一的「不再停在路由证据未配置」可以取证，「形成初始路由」要等上面两处。本票的取证因此分两样：动线上可达性越过格 4；经生产装配的真库用例证初始路由越过`路由证据未配置`，并照实写下一个停点。

**另一处缺口，本票要补。** 「做什么」第 1 条要随参考配置发的日历与截单、线路的 BUY 价卡引用，今天只有端口行类型与表（迁移 `0015`、`0016`）。`registrationjson` 的登记行形状与登记用例都不带它们（`RegisterLineCostBases` 全仓只有测试调用），所以既有登记口登不进去。采用就是一次普通登记（ADR-0147 决定四），本票把这几格作为可选格补进既有登记口。

## 完成记录（2026-10-10，通道 2，派单 `task-270b3557`；分支 `mcp2-rfc11` 基 `b57ff794`；待非作者评审与重放）

**落点**

| 笔 | 内容 |
|---|---|
| `090b92af` | 票面：认领 |
| `80284575` | 票面：开工取证（判据一今天停在 psb/05 格 4） |
| `201a3c17` | network-routing：依据格（领域 `CatalogBasisReference`、迁移 `network_routing/0018`、端口行类型、目录适配器写入与运营查阅上列读回、`registrationjson` 直接登记行可选 `basis` 且不许自写引用串）；日历三格与线路逐段成本依据接进既有登记口，受理门新增 `CALENDAR_CONTENT_OUT_OF_RANGE`、`COST_BASIS_MALFORMED`；network-routing CONTEXT「网络定义与临时可用性」补一条登记依据与采用的规则 |
| `aca995b5` | 参考配置 `network-routing/network-catalog/SYN-CN-SG@1`（嵌入行与发布清单同笔）；`registrationjson` 的采用路径（`adopt.go`），各族内容格由直接登记与采用共用一段翻译；发布门两条用例（逐行过登记用例的受理门、网络自洽） |
| `1bbd0f09` | `parcel-network-register`：头注改写本口登得进的格与采用行写法；采用行用例 |
| `6857e599` | 演示种子：网络段改为采用行，日历改为按节点与连接登，上海枢纽修订 2 改为采用后的租户修订；`submit-one-shipment.sh` 带寄 / 收国家码；种子 README 与[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)第 3 步、「墙三」各补一段 |
| `eb57f999` | `cmd/parcel-dispatch`：判据一取证用例（照种子逐行采用之后经生产装配取证据与成本） |
| `ee2143ad` | 清点：在 `eb57f999` 的干净检出上重生成 |
| `bdee59cd` | network-routing：初始路由编排在成本来源没配置时形成未决 `COST_SOURCE_NOT_CONFIGURED` 的用例（此前只有代码） |
| 本笔 | 票面：判据、完成记录、Status |

**对完成判据**

- ◑ **判据一**「演示租户上一票已接受的委托形成初始路由，不再停在路由证据未配置」：后半成立，前半没到。
  - 动线（实测，只记 `S`）：代码钉 `1bbd0f09`、种子是 `6857e599` 那一份，一次性库 `idp_mcp2_rfc11_journey2`，11:42Z。`seed.sh` 干净灌退 0，按动线脚本起进程，提交一笔（`SHR-7RLWIKJX5HU6R3JREKJ4UYJH54`）。受理链第 1 次投递就 `PUBLISHED`。`network_routing.reachability_judgment` 有一行，PS 的 `acceptance_reachability_judgment` 记 `REACHABLE`，格 4 越过。下一个停点是 `acceptance_processing_attempt` 那一行：`FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` / `OPERATOR_REGISTRATION`，即 psb/05 格 5。委托停在`已提交`，接受决定那条线不会触发初始路由。
  - 生产装配（真库用例）：`TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring` 证了三件事。初始路由证据视图答已配置，候选 `SYN-LINE-CN-SG-01@1` 三段，策略成本单维，带时间投影；按处理器的层次逐层评估，它是唯一合格候选；生产装配的候选成本取数侧答 `ErrRouteCostSourceNotConfigured`。编排把这一格折成未决 `COST_SOURCE_NOT_CONFIGURED`，由 `TestAnUnconfiguredCostSourceLeavesTheParcelUndecided` 钉着（`bdee59cd`）。没采用的租户两个证据视图照旧答`未配置`，见 `TestATenantThatDidNotAdoptTheDemoNetworkStaysUnconfigured`。
  - 前半没到，卡在两处，都不在本票「做什么」里，11:1xZ 已报通道 1：① psb/05 格 5（财务控制的时点语义、演示租户的结算账户、估价方法 psb/06 第 2 项）；② 路由时的计价输入（ADR-0148 决定四第 7 条；越权风险点 5「逐段折成价卡区域」归 PP owner）。
- ✅ **判据二**「采用记录的依据指向参考配置版本；演示数据全为 `SYN-` 合成值，证据只记 `S`」：
  - 依据：上面那只一次性库里，节点、连接、线路、服务区域、日历与路由策略各行的 `basis_ref` 都是 `REFCFG-1:network-routing/network-catalog/SYN-CN-SG@1`，只有上海枢纽修订 2 是 `SYN-NET-OPS/CHANGE-SHA-HUB-2606`（采用后的租户修订）。用例：`TestEveryReleasedNetworkCatalogReferencePassesTheRegistrationGates` 逐行断依据格等于该版引用串；`TestCatalogVersionsKeepTheirRegistrationBasis` 是真库往返；`TestExecuteAdoptsALineFromTheReferenceAndRegistersItsCostBases` 走登记口。
  - `SYN-`：参考配置里的身份全是 `SYN-` 值，`TestEveryReleasedNetworkCatalogReferenceHangsTogether` 对键以 `SYN-` 起头的网络逐个核；种子各行同。`docs/product/PILOT-PARAMETER-REGISTER.md` 自 `b57ff794` 起未动。全部取证只记 `S`。
  - 没采用的租户照旧`未配置`：见判据一生产装配那条的反事实用例。

**判断项**（前两条是卡面授权照 ADR-0147 先例的代裁）

1. **依据格的形状与可空性 · 越权风险点 · 待 NR owner 复核。** 稳定定义各族的版本表各加一列 `basis_ref text`，可空，带非空白 CHECK。领域 `CatalogBasisReference` 的零值就是没给依据。带 `REFCFG-1` 前缀的串必须打得开已发布版本（ADR-0157 同一条）。理由：
   - 一列串，不拆标识与版本两列，照 ADR-0147 候选丁。
   - 可空，因为存量行没有依据，直接登记也从来不要求依据。做成必填，就得给存量行补默认值（碰红线），还要改每个登记口的契约。这是与 PC 先例不同的地方：注册号类型目录的依据格是必填。
   - 直接登记行可以带租户自己的依据（ADR-0147 决定四「改过的……依据换成租户自己的」），但不许自写引用串：引用串只由采用路径写，否则内容是登记方自己给的、却声称是采用。
   - 临时可用性调整不加这一格：它已有来源，是陈述，不经采用。线路的逐段成本依据随线路版本走，依据记在版本行上。
   - 选版读口 `LoadDefinitionsAt` 不取依据，折叠证据用不到它。
2. **网络这一族按什么键切一份参考配置 · ADR-0147 越权风险点 3 · 待 NR owner 复核。** 按「一份网络」切：标识写 `network-routing/network-catalog/<网络键>`，目录名取登记册「版本化网络目录」，键是这份网络的自然键。本份键为 `SYN-CN-SG`，`SYN-` 标明它是合成演示网络。一份装齐稳定定义各族互相引用的行；采用照 PC 先例批文里「逐项 adopt」的写法逐行登记，每行依据都指这一份。不按族或按身份切，是因为那样一张网要按每族每个身份各拆一份参考配置，拆开采用拼不成路。参考配置原文的形状归 NR 登记口（`registrationjson` 的 `networkCatalogReferenceDocument`），`referenceconfig` 包只管版本与摘要，同 ADR-0147 决定一的分工。
3. **日历三格与线路成本依据补进登记口，本票做了。** 理由见「开工取证」末段。路由策略的比较币种与所引价格政策两列，本票用不到，没补，`parcel-network-register` 头注里如实写着。
4. **线路三段都引同一张 BUY 卡 `SYN-PLAN-CN-SG-COST-01/v1`。** 演示种子只有这一张成本卡，种子 README 里两份供应商协议也都采购它。合成金额只为让成本单维有得比，参考配置的 `note` 里写明了。
5. **上海枢纽修订 2 改为采用后的租户修订**（直接登记，带演示租户自己的 `SYN-` 依据）。原种子 v2 与 v1 内容相同，只为演示版本轴；现在它同时演示 ADR-0147 决定四「改过的就是租户取值」。
6. **日历按节点与连接登，删掉了挂在线路上的那一份。** 时间投影只读节点日历的处理时长与截单、连接日历的缓冲（ADR-0175）。原先那份三格全空，什么都不供，参考配置不带它。
7. **服务区域按整个国家覆盖。** `SYN-AREA-CN-EAST` 名字里是华东，覆盖却是整个 CN：演示提交只带国家码、不带邮编，按邮编前缀覆盖会让服务区域解析答资料不足。合成网络这样简化，判断方法不变。
8. **`submit-one-shipment.sh` 加两格国家码。** 这在演示种子地盘内；不加，服务区域解析在两侧都答资料不足。

**未做 / 边界**

- 判据一的前半（形成初始路由）：见判据一「前半没到」那一条，两处都在本票之外。
- **一处能编过、语义不对的缝**（探针，未入库；所涉两份文件自 `b57ff794` 起本票都没动）。目录折叠铸的候选标识是 `线路@版本`（`catalog_network_evidence.go` 的 `versionReference`），候选成本适配器 `RouteCandidateCostAdapter.resolveLegs` 却按 `/` 切线路引用；rfc/10 的用例用 `pathOf` 手造 `线路/1`，没碰过折叠出来的标识。探针把计价输入换成一个答「已配置」的替身，喂进生产折叠出来的证据，实得 `untranslatable pricing evaluation: candidate "SYN-LINE-CN-SG-01@1" carries no line reference`。所以计价输入一接上，初始路由会落 `COST_SOURCE_UNAVAILABLE`，而不是出价。今天它被计价输入未配置挡在前面，看不出来。卡面范围外，本票不改，记给通道 1。
- 管理台读面没有显示依据格（ADR-0147 越权风险点 1，归各读面的票）；`/network-catalog` 答复体字段不变。
- psb/05 格 4、格 6 的票面没有更新，那是 psb/05 的地盘；本票的取证在这里。
- 评审：非作者评审预定由通道 3 做，作者不自评。

**验证**（钉 `ee2143ad`，11:48Z–11:51Z，`IDP_PARCEL_POSTGRES_DSN` 指 55432 门禁库）

- 本树干净，本地与远端同 SHA。自 `b57ff794` 以来改过的 `.go` 跑 `gofmt -l` 无输出；`go build ./...`、`go vet ./...` 全仓退 0。
- `go test -count=1 -p 1`：范围由 `go list` 反查，包括动过的包（NR 的 domain、ports、application、registrationjson、postgres，`referenceconfig`，`migrations`，两个 `cmd`）、它们的反向依赖（含只在测试里导入的）、全部 `pgtest` 使用方（迁移 `0018` 进了每个真库用例的建库）、`./internal/architecture/...` 与全部 `cmd/*`。钉在此 SHA 上共 68 个包，其中 `cmd/*` 14 个。结果 5022 个用例 PASS，FAIL 0；SKIP 1，是 `pgtest` 的 `TestHelperTemplateOwnerProcess`（只由子进程驱动，与 DSN 无关）；另 2 个包没有测试文件。
- 开跑后 `-v` 单跑 `TestCatalogVersionsKeepTheirRegistrationBasis`、`TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring`、`TestATenantThatDidNotAdoptTheDemoNetworkStaysUnconfigured`，都是 PASS 不是 SKIP。
- `bdee59cd` 只在 `create_initial_route_test.go` 里加一条用例：`./internal/networkrouting/application/...` 与 `./internal/architecture/...` 在它上面重跑全过，全仓 build / vet 退 0，清点在它的干净检出上重生成零差。
- 判别力：下面每次变异都只改一处，跑完还原、未提交。采用路径不写依据：`TestAnAdoptRowTakesItsContentFromTheReferenceAndCitesIt` 与 `TestEveryReleasedNetworkCatalogReferencePassesTheRegistrationGates` 红。参考配置拿掉末端节点的处理时长并改钉摘要：`TestEveryReleasedNetworkCatalogReferenceHangsTogether` 红。编排把成本来源没配置折成 `RouteCostSourceUnavailable`：`TestAnUnconfiguredCostSourceLeavesTheParcelUndecided` 红。候选标识那次探针不是变异，见上一节。
- 两次动线取证的一次性库（`090b92af` 一次、`1bbd0f09` 加本票种子一次）都已删。
