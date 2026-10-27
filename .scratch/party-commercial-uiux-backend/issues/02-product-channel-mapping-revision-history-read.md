# 02 产品—渠道映射的历史修订读口

Category: enhancement
Status: resolved——2026-10-26 进 main `51e154be0`（见文末「进 main 记录」）。此前：作者完工，分支 claude-pcub1
Blocked by: 无
地盘：`internal/partycommercial` 的映射目录读（`ports.ProductChannelMappingCatalogueRead` 及新读口、`adapters/postgres/product_channel_mapping_catalogue.go`、`adapters/http` 新端点）、`cmd/parcel-api/endpoints_commercial_catalogue.go` 的端点表与装配；管理台渠道产品目录页（前端跟进，同票）。
出处：[party-commercial-uiux 票 21](../../party-commercial-uiux/issues/21-row-detail-and-version-history.md)「已知」：渠道映射的历史修订要后端补读口；spec 第三段新增一条。先例：业务参与方与责任法人的修订历史读口（`/commercial-business-parties/{partyId}/revisions`）。

## 现象（读码，钉 main `cbaa256d7`）

- 映射按修订推进，调整绑定或区间是翻旧插新、不覆盖历史（CONTEXT 生命周期节；表 `product_channel_mapping_registration` 键带修订号），历史本来就在库里。
- 目录读（`ListProductChannelMappings`）只交每个映射的最新一笔（`DISTINCT ON (mapping_id) … ORDER BY revision DESC`），之前各笔没有读口，管理台答不了「这条映射改过什么」。

## 要做的

- 新读口 `ListProductChannelMappingRevisions(tenant, mappingID)`：按映射标识交出整条修订链，按修订号升序，不带 LIMIT，不在册交空切片；行形状与目录行同一个 `ProductChannelMappingRow`（每一笔是一份完整登记信封），不导出状态。读口并进 `ProductChannelMappingCatalogueRead`（判据同身份目录对两册修订历史的安排）。
- 新端点 `GET /commercial-product-channel-mappings/{mappingId}/revisions`，答复体 `{outcome, mappingId, revisions}`，每一笔与目录行同一个 body；不在册与跨租户答 200 加空数组，不答 404；同 Intake、同操作者查阅读授予面；挂端点表并带探针。
- 管理台渠道产品目录页：行点开的抽屉「历史」栏按修订列各笔（渠道、依据、登记时刻），点某一笔把抽屉切到那一笔的详情。
- 不在本票：版本册（产品、合同、协议、政策）的历史——它们已能从目录行各版直接列出。

## 完成判据

- [x] 真库用例：一个映射三笔修订按序交出，旧笔不被顶掉；显式未配置那一笔为空切片；别的映射、别的租户读不到；不在册答空切片。
- [x] HTTP 用例：整条链逐笔转写、顶层回显映射标识、不在册答 200 加空数组、方法门、路径参数缺失、读失败答 5xx。
- [x] 端点表与隔离读用例（`cmd/parcel-api`）通过；`internal/architecture` 通过。
- [x] 管理台：演示库里有三笔修订的映射（种子只有第 1 修订，另用 `parcel-commercial register-products` 登记了第 2、3 修订），抽屉历史栏按新到旧列出，点第 1 笔详情切到它。
- [x] 全量 `scripts/test-full.sh`、管理台三道门。

## 三类

读面是机制；映射怎么调整是租户取值。

## 规模

S–M

## 完成记录

- 起点 `cbaa256d7`，分支 `claude-pcub1`。
- 新增：`ports.ProductChannelMappingRevisionHistoryRead`（并进 `ProductChannelMappingCatalogueRead`）；`(*OperationsCatalogue).ListProductChannelMappingRevisions`；`commercialhttp.NewQueryProductChannelMappingRevisionsEndpoint` 与读口接口 `ProductChannelMappingRevisionHistoryReader`（并进 `ProductChannelCatalogueReader`）；`cmd/parcel-api` 端点表一行（带探针 `/commercial-product-channel-mappings/SYN-MAP-01/revisions`）与挂载一行；占位读口 `unwiredCommercialCatalogue.ListProductChannelMappingRevisions`。无迁移、无新 ADR。
- AT：`AT-PC-067`（UC-PC-001，紧接 066），`.scratch/at-coverage-inventory.md` ①分桶表已补行。
- 用例：真库 `TestProductChannelMappingRevisionsExpandOneMappingAscending`；HTTP 三条（整条链转写、不在册答空链、方法门与路径参数与读失败）。
- 管理台：`api.ts` 补 `listProductChannelMappingRevisions` 与答复类型；纯函数 `mapping-history.ts`（新到旧，每笔写渠道或「未配置（显式登记）」、依据、登记时刻）带 2 条用例；`VersionDetailDrawer` 的历史条目多可选的 `label` 与 `note`；`ChannelProductCatalogPage.tsx` 点开时读整条链，答复核对映射标识防错位，点历史某一笔切抽屉。演示库种子只有 `SYN-MAP-CN-SG-EXPRESS-01` 的第 1 修订，验证时另用 `parcel-commercial register-products` 登记了第 2、3 修订（渠道与依据各换一次），抽屉历史栏列出第 3、2、1 修订，点第 1 修订详情切到 r1 的渠道与依据。
- 代决与越权风险点：无新决定。判据全按参与方修订历史读口的先例（路径取标识、不在册答空、同 Intake）。

## 评审记录

评审 ← 隔离子代理(sonnet) · 钉 `2e13d629d`（两票同审）：阻断无；总评可进 main；逐点核对参与方修订历史先例（方法门、Intake 之后读路径、400/500 取舍、顶层标识回显、租户条件落在 SQL、非 nil 空切片、无 LIMIT）相符，`lines` 与 `mount` 同模式字、探针合法并自动入隔离读，新路径过 `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable`。非阻断已处理：真库用例改为先落第 2 修订再落第 1 再落第 3，钉住「序按修订号不按落库次序」，并让第 3 笔带区间终点、逐笔读回；HTTP 用例补真正的跨租户（读口登 tenant-2 的映射、请求租户 tenant-1，同名与不存在同形答空链）；`mapping-history` 用例补钉终点；`ports.go` 与端点表两处注释的变更叙述、位置词改写；种子只有第 1 修订的复现说明写进判据与完成记录。

## 进 main 记录（2026-10-26，推送方会话 tops-54）

- **main**：`51e154be0`（快进，main 此前未动，起点 `cbaa256d7`）。出处：分支 `claude-pcub1` 自 `cbaa256d7` 起的代码笔 `f0a2cf881`、票面笔 `2e13d629d`、评审修订笔 `f79a0eab6`；推送方在 tip 上另补 AT 盘点总览计数（`b856eeccd`）与机制清点（`51e154be0`）两笔生成物。
- **验**：全量 `scripts/test-full.sh` 退 0（158 ok / 0 FAIL）；`go run ./tools/adr-index -check` 与 `at-coverage-counts -check` 退 0；管理台三道门：tsc 退 0、node:test 780/780、vite build 退 0；一次性库灌种子退 0、用后删库。
- **号**：`AT-PC-066`、`AT-PC-067`（UC-PC-001）。
- **评审**：见上「评审记录」。
