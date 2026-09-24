# 目录读口分页、排序与筛选下推：ADR-0144 落地（首批）

Category: enhancement
Status: in-progress——2026-09-24 通道 3 按票 [admin-web-group-legal-entities/04](../admin-web-group-legal-entities/issues/04-catalogue-read-pagination-sort-filter-contract.md) 的产出要求拆票；子票全部 ready-for-agent
出处：票 admin-web-group-legal-entities/04（用户 2026-09-24 授权通道 3 自决）→ [ADR-0144](../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md)。

契约只在 ADR-0144 一处定义；本 spec 与子票只引它的决定号，不复述参数名、答复形状与校验规则——复述的那一份会在 ADR 修订时留在这里变旧。

## 为什么先拆这五张

ADR-0144 决定七：共用件一处定义、读口逐册迁、按规模先到的先改。首批只迁客户账户与网络目录两册，用它们把共用件的形状、索引写法与管理台的游标模式
一起跑通；其余各册等首批进 main 之后按同一形状逐册拆票，不在这里预拆——首批里要是改了共用件的接口，预拆的票就全是旧的。

## 子票

| 票 | 标题 | 状态 |
|---|---|---|
| [01](./issues/01-platform-catalogue-page-component.md) | `internal/platform` 共用件：游标编解码、查询参数校验、答复 `page` 拼装 | resolved · 通道 2 · main `62a04eac`（← 分支 `f23f16e6`） |
| [02](./issues/02-customer-accounts-read-adopts-adr-0144.md) | 客户账户目录读口（`/commercial-customer-accounts`）迁到 ADR-0144 | in-progress · 通道 2 · 分支 `mcp2-crp02`（基 `mcp2-crp03`） |
| [03](./issues/03-network-catalog-read-adopts-adr-0144.md) | 网络目录读口（`/network-catalog`）迁到 ADR-0144 | in-progress · 通道 2 · 分支 `mcp2-crp03`（基 `mcp2-crp01`） |
| [04](./issues/04-list-template-cursor-pagination-mode.md) | `ListPageTemplate` 的 `pagination` 槽加游标模式（向后兼容） | resolved · 通道 1 本地 `dc19e5f9`（已进 main `9a477af9`）；探针 11 ok；评审 ← 通道 3 可接受，其非阻断 1 已修 `6677f930` |
| [05](./issues/05-first-two-pages-push-down.md) | 管理台首批两页改为服务端翻页、筛选与检索下推，README 通则改写 | ready-for-agent · Blocked by 02、03、04 与 admin-web-workspace-form/06 |

走法：01–03 碰 Go / SQL，走[并行会话](../../docs/agents/parallel-sessions.md)那条路；04、05 的 diff 只在 `apps/admin-web/**` 与票面，走
[workflow.md「前端切片」](../../docs/agents/workflow.md#前端切片一人在-main-上直接做)。

## 红线

- 契约以 ADR-0144 为准；实施中发现它写不通，回 ADR 改（或 supersede），不在代码里另立口径。
- 查询参数一律封闭集，集外即拒；页大小不开调用方参数。
- 夹具与种子只出 `SYN-` 合成值。
