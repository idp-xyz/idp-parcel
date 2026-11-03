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

## 完成记录（2026-11-03，分支 `claude-csux-03`，worktree 从 main `ae9a9a763` 起）

### 落点

- 票面落点：`LegalEntityDetailPage.tsx`、`LegalEntityDetailPanel.tsx`、`LegalEntityHeaderParts.tsx`（摆上 `LegalEntityLifecycle`，新增 `DateCell` / `ProfileStatusValue`，「更多」菜单自己记对话框开关）、`IdentityDeactivationForm.tsx`（`locked` 属性）、新 `LegalEntityDeactivationDialog.tsx`、新 `LegalEntityReferencesSection.tsx`（含 `useLegalEntityReferences`，两册一页只读一次、首屏与「关联」签共用）+ `legal-entity-references.ts`（+ `.test.ts`）。
- 落点之外：
  - `GroupLegalEntitiesPage.tsx` 只加 `onChanged={retry}` 一个 prop（票面写明）。
  - `identity-deactivation-form.ts`（+ `.test.ts`）：它是 `IdentityDeactivationForm.tsx` 的纯半边，别的票不碰。加了两个纯函数——`identityDeactivationDraftOf`（锁定 / 预填的草稿只带种类与标识）与 `deactivationLandedActions`（落册后谁来重取，提交路径在 node:test 里对替身钉住）；第 5 条要的「锁定预填的表单判读」只能落在 `.ts` 里，组件文件钉不了。
- 共享面（`templates/*`、`components/*`）一行没动：模板不给指标值字号选项，就在值节点上自己定 16px（`statValue`）。

### 判据

- ✅ 三道门退 0：`tsc -b --noEmit`、`run-tests.mjs`（1137 测试全绿）、`vite build`。
- ✅ 新增纯函数有 node:test：`legal-entity-references.test.ts`（关联、在用、段首措辞、被引用求和、去处、日期取日、资料状态格）、`identity-deactivation-form.test.ts` 加两例。
- ✅ 对象页首屏三格 + 生命周期条：`01-object-first-screen.png`——生效日期 2026-01-02、资料状态胶囊、被引用 4、生命周期条；「变更」与「停用时点 未停用」已撤。
- ✅ 「关联」签列出 SYN-LE-01 名下的协议 / 政策：`02-object-related.png`——「供应商协议 4 · 在用 2」四行、「结算政策 2」两行，行是去对应页带检索词的链接。
- ✅ 「更多 › 停用」开对话框、种类与对象锁定、标题带公司名：`03-object-deactivation-dialog.png`——标题「停用公司主体 合成运营方一号（演示）」，只有版本号（建议值 2）、来源单据、停用时间三格；Esc 关掉。
- ✅ 详情栏同样：`04-panel-first-screen.png`（头区生命周期条、三格）、`05-panel-related.png`、`06-panel-deactivation-dialog.png`。
- ✅ 往来单位页 `#/business-parties?register=identity-deactivation` 照旧可用：`07-parties-deactivation-form.png`——种类与对象两格仍在。
- ✅ 整个核验没有发任何写请求（脚本监听并断言：`NO WRITE REQUESTS`）。
- 截图与脚本：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/70733dd4-0571-48b5-8660-e52d102ae60f/scratchpad/csux03/`（`shots.js`，vite 自起 :5203 对 :19080，核完已停）。

### 与票面的出入（要协调方裁）

- **客户合同、客户账户不在「关联」签里**：`CustomerContractRecord` 没有责任法人一格（后端 `customerContractBody` 不透出法人，领域客户合同正文也没有这一格），`CustomerAccountRecord` 只钉客户参与方——在已取回的行上关联不出来。签底一句话如实说明，合同经结算政策的「合同」一格间接可见。读口上带 `legalEntity` 的另有授信政策与接受规则包，票面没列，没加；要加是两个 GET，裁了再做。
- **「被引用」= 供应商协议在用版本数 + 结算政策版本数**：结算政策读面没有状态（`version-groups.ts`「那几册只排不筛」），「在用」判不了，按版本数计；格的 `title` 写明口径。
- 「资料状态」格的词仍是旧词（开票资料可用），由票 04 的 `legalEntityProfileStatusOf` 改，本票只调用。

### 未验

- 停用的提交路径没在浏览器上走（不发写请求）；落册后「关对话框 + 重取列表」的链路以 `deactivationLandedActions` 的 node:test 与代码为证。
- 「停用日期」格与「停用来源单据」行只在已停用主体上出现，演示租户没有已停用主体，没截到图；是条件 JSX，判读在同一处。
