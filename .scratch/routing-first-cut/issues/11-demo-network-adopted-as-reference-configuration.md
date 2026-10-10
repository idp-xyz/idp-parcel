# 11 演示网络作为参考配置，经 psb/03 的采用路径进入演示租户

Category: enhancement
Status: ready-for-agent
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
