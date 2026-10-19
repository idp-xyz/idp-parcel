# 51 供应商协议目录读面不带收费发生条件

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（票 [29](./29-supplier-agreements-declare-charge-occurrence-conditions.md) 完成记录「偏离与待裁」，用户「你作为业务和系统专家自决……解决所有的问题」授权）。
Blocked by: 无
地盘（建议）：PC `adapters/postgres/operations_catalogue.go` 的供应商协议目录行与 `ports.SupplierAgreementCatalogueRow`；PC http 目录读口；管理台供应商协议页（`apps/admin-web`，按前端切片的落法另走）。
出处：[ADR-0209](../../../docs/adr/0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 决定一。

## 现象

供应商协议正文自票 29 起可带收费发生条件（逐原因计不计费、数量形），发生项登记按它答`未配置`或「协议不计此原因」。运营查阅的供应商协议目录（`ListSupplierAgreements`）只列供应商、法人、采购方案、范围与区间，看不到这一版声明了哪些条件；登记方收到`未配置`时，没有读面能看出是哪个原因没声明。

## 三类

机制（读面多带一节）。不涉租户取值。

## 完成判据（草拟）

- 目录行带出这一版的条件（原因、计不计费、数量形；整节没声明如实答没声明）。
- 真库测试：带条件与不带条件的两版各列一行。
