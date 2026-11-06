# 08 settlement-accounting 各册迁到 ADR-0144

Category: enhancement
Status: ready-for-agent——迁移编号预留 settlement-accounting `0056`（进 main 时按 tip 重核）
Blocked by: 01
地盘：`internal/settlementaccounting/{ports,adapters/http,adapters/postgres}` 里下表各册的读端口、端点与读面及其测试；`cmd/parcel-api` 里这些读口的占位；
`migrations/settlement_accounting` 的一条新迁移（索引）。
出处：ADR-0144 决定三至七；spec「横贯各子票的取舍」。

## 本票的册（今天的次序钉 `a1274a6b8`）

| 路由（选择器） | 读端口方法 | 今天的 `ORDER BY` |
|---|---|---|
| `/settlement-charges`（两种） | `ChargeCatalogueRead.ListCustomerCharges` / `ListSupplierExpectedCosts` | `c.charge_id` / `occurrence_id, fee_item, version` |
| `/settlement-statements` | `StatementCatalogueRead.ListCustomerStatements` | `s.statement_number` |
| `/settlement-supplier-bill-receptions` | `StatementCatalogueRead.ListSupplierBillReceptions` | `(claim ->> 'received_at')::timestamptz DESC, claim_id, claim_version` |
| `/settlement-disputed-statements` | `DisputedStatementRead.ListDisputedCustomerStatements` | `s.statement_number`，带计数与 `truncated` |
| `/settlement-funds-applications` | `FundsApplicationCatalogueRead.ListExternalFundsFacts` | `f.fact_id` |
| `/settlement-operating-results`（两种） | `OperatingCatalogueRead.ListOperatingResults` / `ListCostAllocations` | `scope_ref, period_ref, basis` / `allocation_id` |
| `/settlement-audited-payables` | `AuditedPayableCatalogueRead.ListAuditedPayables` | `p.audited_at DESC, p.payable_id` |

开工先核 `/settlement-statement-lines` 与 `/settlement-supplier-bill-lines`：按一张对账单、一张账单取行的属单条读取的从属面，按租户上列的则一并迁。
供应商账单接收的排序维今天取自 JSON 文档里的时刻，迁移时导出成非空列再作可排维（取舍 6）。

## 做法与判据

见 spec「每张迁移子票（02–13）的做法与判据」。本票特有：带计数的两册照取舍 5。
