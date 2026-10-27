# 01 客户合同目录行带出货主客户账户

Category: enhancement
Status: resolved——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-pcub1（见文末「完成记录」）
Blocked by: 无
地盘：`internal/partycommercial` 的客户合同目录读（`ports.CustomerContractCatalogueRow`、`adapters/postgres` 的 `ListCustomerContracts`、`adapters/http` 的合同目录答复）；管理台 `pages/party` 的客户合同页（前端跟进，同票）。
出处：[party-commercial-uiux spec](../../party-commercial-uiux/spec.md)「后续」第三段首条「客户合同读口带出货主客户账户与责任法人，列表才能有『客户』一列」；评估时管理台合同页的说明「客户与责任法人暂不在本列表显示」。

## 现象（读码，钉 main `cbaa256d7`）

- 合同正文自 0037 迁移起落库一格 `customer_account_id`（[ADR-0177](../../../docs/adr/0177-a-customer-contract-version-names-its-customer-account-and-the-closure-narrows-contract-candidates-by-it.md) 决定一：合同版本必须明确关联货主客户账户，闭包按解析键上的账户收窄合同候选）。
- 合同目录读（`ListCustomerContracts`）从不取这一列，答复体也没有这一键，于是管理台合同页没法给「这份合同是哪个客户账户的」。
- 「责任法人」不属合同自己：合同正文没有这一格，它随接单规则包（规则包行上有 `legalEntity`）。本票不加。

## 要做的

- 目录行与 HTTP 答复体带 `customerAccountId`：正文已登记时是正文里的账户；正文未登记的壳行、或 0037 之前写下没法补的旧行没有这一格，如实交回空、键缺席，不猜一个账户。
- 管理台合同页加「货主客户账户」一列（账户标识为主，客户名称取自客户账户册第一页，取不到不显名称），详情抽屉随目录列自动带出；页面说明里去掉「客户暂不显示」。
- 不在本票：责任法人；账户册自己的目录（已有）。

## 完成判据

- [x] 真库用例：三份合同（正文未登记、账户 A、账户 B），目录各读各的账户，壳行为空。
- [x] HTTP 用例：带正文的行有 `customerAccountId`，壳行无此键。
- [x] 管理台：合同表多一列，演示库里两份合同分别显示账户标识与客户名称。
- [ ] 全量 `scripts/test-full.sh`、管理台三道门与 `internal/architecture`。

## 三类

回显是机制；哪份合同属哪个账户是租户取值。

## 规模

S

## 完成记录

- 起点 `cbaa256d7`，分支 `claude-pcub1`。
- `ports.CustomerContractCatalogueRow` 补 `CustomerAccountID`；`ListCustomerContracts` 的 SELECT 与 GROUP BY 补 `content.customer_account_id`，扫成可空串（旧行 NULL → 空串）；`customerContractBody` 补 `customerAccountId`（omitempty，只在正文已登记时赋值）。无迁移、无新 ADR、无新端点。
- AT：`AT-PC-066`（UC-PC-001，紧接 065；全仓与各 claude- 分支核过无占用），`.scratch/at-coverage-inventory.md` ①分桶表已补行。
- 用例：真库 `TestCustomerContractCatalogueEchoesTheCustomerAccount`；HTTP `TestCustomerContractsEndpointEchoesTheCustomerAccountOnlyWhenContentIsRegistered`。
- 管理台：`api.ts` 的 `CustomerContractRecord` 补 `customerAccountId`；`PartyContractsPage.tsx` 加「货主客户账户」列，客户名称取 `loadCustomerAccountSuggestions` 读回的账户册第一页，取不到不显名称；检索也认账户标识。演示库里 `SYN-CONTRACT-01`、`02` 分别显示 `SYN-ACCOUNT-01 合成货主一号（演示）`、`SYN-ACCOUNT-02 合成货主二号（演示）`。
- 代决与越权风险点：无新决定。
