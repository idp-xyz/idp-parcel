# 03 对象页与详情栏首屏重构、停用对话框留在本页、「被谁引用」全景

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无
出处：评估「一、概念模型与路径」第 2–4 条、「三、交互」后三条、「五、视觉细节」第 1 条。

## 要做的

1. **首屏指标带**（`LegalEntityDetailPage.tsx` 的 `buildSummary`、`LegalEntityDetailPanel.tsx` 的 `FactStrip`）：
   - 撤掉「变更 第 N 次更新」（身份版本只在「登记记录」签里看）和常驻的「停用时点 未停用」。
   - 改成三格：**生效日期**、**资料状态**（取当前资料解析：可开票 / 资料不全 / 未登记资料…，用 `StatusPill`，词由票 04 的 `legalEntityProfileStatusOf` 出）、**被引用**（第 4 条的计数，读不到时显「—」）。已停用时才多一格「停用日期」。
   - 指标卡字号降一级（主数据不是仪表盘）：对象页用模板的 `summary` 照旧，但 value 用 16px 而不是 24px——若模板不给这个选项，就把这三件放进头区 `meta` 行，不用指标带。
   - 概要卡里「停用的来源单据：未停用」只在停用时出现；「对应的往来单位」撤下，进技术详情「往来单位档案」。
   - 往来单位页有生命周期条（`LegalEntityLifecycle` 已在 `LegalEntityHeaderParts.tsx` 里写好却没摆），公司主体对象页与详情栏头区摆上，与往来单位页一致。
2. **停用留在本页**（`LegalEntityHeaderParts.tsx`、`IdentityDeactivationForm.tsx`、新 `LegalEntityDeactivationDialog.tsx`）：
   - 「更多 › 停用这个公司主体…」改为打开本页的对话框，不再跳往来单位页。对话框内嵌 `IdentityDeactivationForm`，加一个 `locked?: DeactivationPreset` 属性：种类与对象锁定、不显示「停用什么 / 停用哪一个」两格，只填版本号（建议值）、来源单据、停用时间；对话框标题「停用公司主体 合成运营方一号（演示）」，正文一句「从停用时间起，新的签约、开票不再用它；已有记录保留，停用后不能撤回」。
   - 落册答 `DEACTIVATED` 后关对话框、重取列表（对象页有 `retry`；详情栏经 `GroupLegalEntitiesPage.tsx` 的 `renderDetail` 传 `onChanged={retry}`——那文件归票 01，你只加这一个 prop，票面写明）。
   - 往来单位页的停用表单照旧可用（那是业务参与方与客户账户的停用入口）。
3. **「被谁引用」全景**（新 `LegalEntityReferencesSection.tsx` + `legal-entity-references.ts`）：
   - 对象页与详情栏各加一签 `related`（签名「关联」已在 `templates/workspace-tabs.ts` 里）：列出这家公司名下的**客户合同**（`listCustomerContracts`，按责任法人字段关联）、**供应商协议**（`listSupplierAgreements`，`legalEntity` 字段）、**结算政策**（`listCommercialPolicies('SETTLEMENT_POLICY')`，按责任法人关联——字段名看 `SettlementPolicyRecord`）、**客户账户**（`listCustomerAccounts`，若记录带责任法人）。每类一行「客户合同 3 · 在用 2」，展开列各版本名称、版本、状态、有效区间，点行按 `ref-links.ts` 既有机制跳到对应页并带检索词。
   - 判读纯函数照 `party-overview.ts` 的写法：只在已取回的行上关联，不下推查询参数，不改端点；取不到的那一册显「读不到」，不显 0。
   - 第 1 条指标「被引用」= 各册在用版本数之和；任一册读不到显「—」。
4. **用语**：本票落点里的词照 spec 表（「更新公司资料」按钮、「来源单据」、「登记记录」、技术详情行名不动）。对象页 `description` 「编码 SYN-LE-01」照旧。
5. 纯函数（引用关联、指标三格的取值、锁定预填的表单判读）钉 node:test。

## 不做

- 不改停用写口；不改往来单位页的停用表单形态（只加 `locked` 属性）。
- 不动票 01、02、04 的落点；`GroupLegalEntitiesPage.tsx` 只加一个 prop。
- 不做「停用前要先处理在用合同」之类的本地规则——那归服务端。

## 完成判据

- 三道门退 0；新增纯函数有 node:test。
- 浏览器在演示租户核（不提交停用）：对象页首屏三格 + 生命周期条；「关联」签列出 SYN-LE-01 名下的合同 / 协议 / 政策；「更多 › 停用」打开对话框、种类与对象锁定、标题带公司名；详情栏同样。
- 往来单位页 `#/business-parties?register=identity-deactivation` 照旧可用。
