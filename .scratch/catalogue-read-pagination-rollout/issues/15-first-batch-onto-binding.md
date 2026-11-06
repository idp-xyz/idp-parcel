# 15 首批两册改用 01 的补件、两份页类型并入统一结果类型

Category: enhancement
Status: in-progress——2026-11-06 作者会话在分支 `cursor/crpr-15-first-batch` 上做（worktree `idp-parcel-crpr-15`，基 `d6b3f64a5`）
Blocked by: 01
地盘：`internal/partycommercial/adapters/postgres/customer_account_catalogue.go`、`internal/networkrouting/adapters/postgres/network_catalog_list.go`，
`internal/partycommercial/ports/catalogue_page.go`、`internal/networkrouting/ports` 里的 `CatalogPage`，以及它们的调用点与测试替身。
出处：ADR-0144 决定七「一处定义」；spec「共用件的补件」。

## 做什么

1. 两册读面里手写的筛选、`q`、键集比较与次序改用 01 的落列声明，删掉两份逐字相同的转义器与取值换算。
2. `ports.CataloguePage`（party-commercial）与 `ports.CatalogPage`（network-routing）并入 01 的统一结果类型。
3. 两册端点改成先 Intake、后解码（spec 取舍 8）：渠道未配置时坏参数也答 403。这是本票唯一的行为变化。

## 不做

- 不改两册的声明（可排维、筛选维、缺省序）与答复形状。

## 完成判据

- 两册既有真库用例原样通过；端点用例里「未配置渠道时坏参数先答 400」那几格改成答 403，其余原样通过；`gofmt -l` 空，`go vet` 退 0。
