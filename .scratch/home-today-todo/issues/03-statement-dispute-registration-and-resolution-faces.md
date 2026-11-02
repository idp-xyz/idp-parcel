# 03 客户对账单争议的管理台入口：客服代登记、财务核实后处理

Category: enhancement
Status: in-progress——2026-11-01 作者开工，分支 claude-disputes（从 origin/main `4aade8e49` 起）
父票：无。由票 [02](./02-statement-confirmation-and-disputes-have-no-production-face.md)（在 `claude-hometodo` 上）转来：02 列的两条缺口里，「争议的提出与处理没有生产口」由本票做；「相对方确认不记」用户裁定不做（ADR-0353 决定五），02 收口时指向本票。
Blocked by: 无
地盘：`internal/accessidentity` 两种决定种类与迁移 `access_identity/0037`；`internal/settlementaccounting/{domain,application,ports,adapters/postgres,adapters/registrationjson,adapters/http}` 的争议开立与处理、迁移 `settlement_accounting/0052`；`cmd/parcel-api` 结算决定特性文件、编排包事务与未接线占位；`apps/admin-web` 对账单页与授予常量；演示种子步骤 `510`、`data/access/34`、`35`、`data/governance/75`、`76`、种子 README；ADR-0353；SA CONTEXT 一句；UC-SA-003 一句规则、两条 AT、交接一行；AT 盘点两行；本票。
出处：用户 2026-11-01 经协调方裁定——「做管理台入口：客户电话或邮件提出异议，客服代登记，财务核实后处理；按授予收起。不做客户自助口。待确认应收（客户确认）不做」。

## 要做的

1. 照 UC-SA-003 接出登记争议、处理争议两个写口：HTTP 端点、parcel-api 装配、两种运营决定授予；幂等、并发、只对已发布单、终局不再改等失败边界照 UC；真库用例。
2. 对账单页右栏两张任务式表单，说人话，提交后显示结果，按授予收起（ADR-0315）。
3. UC「给开发的交接」、AT 与 AT 盘点。
