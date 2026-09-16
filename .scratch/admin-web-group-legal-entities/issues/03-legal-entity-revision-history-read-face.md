# 03 责任法人修订历史读口 + 详情抽屉「修订历史」区

Category: enhancement
Status: in-progress
Blocked by: 无（前端那半要等本票 Go 侧落地，同一张票内先后做）
地盘：`internal/partycommercial/ports/`（读口接口）、`internal/partycommercial/adapters/postgres/`（读适配器）、
`internal/partycommercial/adapters/http/`（端点）、`cmd/parcel-api/`（装配表一行 + 隔离读面 Intake 一格）、
`apps/admin-web/src/pages/party/`（抽屉历史区 + `api.ts` 一函数）

## 为什么

身份登记按修订版本化、只增不覆盖（party-commercial CONTEXT「身份生命周期」；`legal_entity_registration` 表存全部修订），
这是本模型对企业客户的卖点——可它在页面上看不见：`GET /commercial-group-legal-entities` 答的是每个法人的**最新**修订，
`r1` 显出来了但点不进去，操作者看不到 r1→r2 改了什么、依据从哪份换到哪份、停用是哪一笔。

## 要做的

1. **读口**：`ports` 加 `LegalEntityRevisionHistory`（或按本包既有读口命名规矩），输入法人标识 + 授权查询作用域（租户维，
   同 `CommercialCatalogueIntake` 的形状），输出该法人全部修订按修订号升序，每行含修订、依据、生效自、停用时点/依据、
   登记时间。**不做 diff**——两笔修订之间改了什么由前端并排显，读口只交事实。
2. **适配器**：`pcpostgres` 在 `NewOperationsCatalogue` 同一只上补方法（同表同作用域纪律，判据同 PS 的复核队列读口），
   或另立——按行形状归谁裁；真库用例走 `pgtest`。
3. **端点**：`GET /commercial-group-legal-entities/{legalEntityId}/revisions`，Intake 同 `commercialCatalogue`（隔离读面
   由 `buildIsolatedReadIntakes` 的 `commercialCatalogue` 一格顺带覆盖，不另加开关——ADR-0078 决定四只此一维）。
   未配置答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`；法人不在册答 200 + 空数组还是 404，按 ADR-0022 裁：**能力在、册在、
   只是没有这一个身份 → 200 空数组**，判据同 `writePartyRegistryAnswer` 对`未找到`的处置。
4. **装配**：`assembleBusinessEndpoints` 加一行；机制清点重生成随笔。
5. **前端**：详情抽屉「修订历史」区改为取真数据，纵向时间线（`Timeline` 原语），每笔显修订号、依据、生效自、登记时间；
   停用那笔标出。读口墙前照旧显未配置。

## 完成判据

- Go：`go build ./...`、`go vet ./...`、动过的包及反向依赖 `go test -count=1`（`cmd/parcel-api` 带 DSN）。
- 端点用例：未配置 403、在册法人多笔按序、不在册 200 空数组、跨租户不可见。
- 前端三道门禁绿；抽屉在演示形态下对 `SYN-LE-01` 显 1 笔。

## Comments
