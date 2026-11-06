# 15 首批两册改用 01 的补件、两份页类型并入统一结果类型

Category: enhancement
Status: ready-for-agent
Blocked by: 01
地盘：`internal/partycommercial/adapters/postgres/customer_account_catalogue.go`、`internal/networkrouting/adapters/postgres/network_catalog_list.go`，
`internal/partycommercial/ports/catalogue_page.go`、`internal/networkrouting/ports` 里的 `CatalogPage`，以及它们的调用点与测试替身。
出处：ADR-0144 决定七「一处定义」；spec「共用件的补件」。

## 做什么

1. 两册读面里手写的筛选、`q`、键集比较与次序改用 01 的落列声明，删掉两份逐字相同的转义器与取值换算。
2. `ports.CataloguePage`（party-commercial）与 `ports.CatalogPage`（network-routing）并入 01 的统一结果类型。

## 不做

- 不改两册的声明（可排维、筛选维、缺省序）与答复形状——零行为变化。

## 完成判据

- 两册既有真库用例与端点用例一字不改原样通过（行为不变的证据就是它们）；`gofmt -l` 空，`go vet` 退 0。
