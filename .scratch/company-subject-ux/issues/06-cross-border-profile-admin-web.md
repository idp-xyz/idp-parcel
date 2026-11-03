# 06 管理台跟上跨境主体资料扩展：监管登记号、收款账户、多语言抬头

Category: enhancement
Status: in-progress（分支 `claude-csux-06`，票 04 已进 main；票 05 分支 `claude-csux-05` tip `a34565442` 评审中，本票对着它的契约做）
父票：[spec](../spec.md)
Blocked by: [04](./04-profile-form.md)、[05](./05-cross-border-profile-extensions.md)
出处：票 05 决定；评估「四、国际小包视角」。

## 要做的

1. **监管登记号**：票 04 做好的「税务登记号类型从目录选」改名「监管登记号（税务 / 海关）」，候选仍取目录资料层；说明一句按票 05 决定 1。读出、差异预览、技术详情同改词。
2. **收款账户**：公司资料表单加「收款账户」组（多条：用途标签、户名、开户行、账号 / IBAN、SWIFT / BIC、开户行地址、币种、备注；币种从基础数据币种目录选，没有目录就手填 ISO 4217 并提示）；读出页「税务与开票」下加「收款账户」表（按币种分行）；差异预览与版本对比（`legal-entity-profile-history.ts` 的 `compareLegalEntityProfiles`）识别账户的新增 / 修改 / 移除；资料维护提示：有生效开票抬头却没有收款账户时提示「对账单与发票上印不出收款账户」。
3. **多语言抬头**：「开票抬头」格旁加「其他语言的抬头」多行（语言标签从一小组常用值选：zh-Hans、zh-Hant、en、ja；可手填 BCP 47）；读出页显示主抬头 + 其他语言；差异预览识别。
4. **译名收尾**（票 01 / 04 留下的）：版本历史对比表与快照（`LegalEntityProfileHistory.tsx`）把类型码经 `typeNameOf` 译名；`legal-entity-identity.ts` 的 `registrationNumberTypeNamesOf` 查法按层分键（同国同码两层都登时不再取身份层名顶资料层）。
5. 用语照 spec「统一用语」表；纯函数钉 node:test；三道门；浏览器核验（只读）。

## 不做

- 不在管理台判领域规则（币种形状、重复之类由服务端答）。
- 不做对账单 / 发票页的账户与抬头选择。

## 完成判据

- 三道门退 0；浏览器在演示租户核：SYN-LE-01 资料读出显示收款账户与双语抬头；表单预填它们；清掉一个账户后差异预览出现「移除」。

## 完成记录（2026-11-03，分支 `claude-csux-06`，起点 main `f17868989`，收尾合入 origin/main `456dd64d8`）

**落点**（一条一笔，都在 `apps/admin-web/src/pages/party/`）

- 第 1 条 `98074b373`：「税务登记号」→「监管登记号（税务 / 海关）」。`LegalEntityProfileFields.tsx` 组名、号码 / 类型格的 aria-label、添加按钮与说明句（哪些号登在这里、类型按注册地在「基础数据 › 注册号类型」配置后选）；`LegalEntityProfileContent.tsx` 读出格；`legal-entity-profile-history.ts` 对比行名；`LegalEntityProfileHistory.tsx` 快照行名；`api.ts` 的 `NOT_ACCEPTED` 一句「税号」→「登记号」。线上字段 `taxRegistrationNumbers` 不动。
- 第 2 条 `8024e3b3a`：收款账户。`api.ts` 加 `LegalEntityBankAccountRecord`，`LegalEntityProfileContent.bankAccounts?`；`legal-entity-profile.ts` 草稿 `BankAccountDraft` 逐行、载荷各格去空白 / 可缺格缺席 / 整行空着不送（`bankAccountPayload`）、必填四格（户名、开户行、账号 / IBAN、币种）空着进 `legalEntityProfileMissingFields` 按格点名、（币种，账号）两行相同本地先提「此币种与账号已经填写」、`profileComparisonContentOf` 与 `profileDraftContentOf` 两个方向都带、`profileMaintenanceNotes` 有开票抬头没账户提「未登记收款账户，对账单与发票上印不出收款账户」、`bankAccountCurrencyOptions` 把基础数据币种目录折成候选；`legal-entity-profile-history.ts` 账户以（币种，账号）认同一个（`bankAccountKey`），其余格变了是修改、换了币种或账号是一移除一新增，项目名 `bankAccountSubject` 带币种译名与账号；`LegalEntityProfileFields.tsx` 「收款账户」组（`BankAccountCurrencyField`：目录有就 `SearchableSelect`，读不到退回手填并提示 ISO 4217）；`LegalEntityProfileContent.tsx` 「税务与开票」下 `LegalEntityBankAccountsTable`（币种打头、没有是「未登记收款账户」）；`LegalEntityProfileHistory.tsx` 快照多一节。
- 第 3 条 `028e56bf9`：多语言抬头。`api.ts` 加 `LocalizedInvoiceTitleRecord`，`invoiceTitleLanguage?` / `invoiceTitles?`；`legal-entity-profile.ts` 两格草稿、载荷留空缺席、半填行与重复语言（含主抬头语言、不分大小写）本地先提、主抬头空着而这两格有值在开票抬头格下说「要挂在开票抬头上」、`invoiceTitleLanguageOptions`（zh-Hans / zh-Hant / en / ja）与 `languageTagLabel`；`legal-entity-profile-history.ts` 「主抬头语言」一行、「其他语言的抬头 · 语言」一种语言一行；`LegalEntityProfileFields.tsx` 「主抬头语言」格与「其他语言的抬头」多行（语言用 datalist，常用四种一点即选、别的 BCP 47 可手填）；`LegalEntityProfileContent.tsx` 抬头后弱化显语言、另一格列其他语言；快照多两格。
- 第 4 条 `56bd52e35`：译名收尾。`legal-entity-identity.ts` 的 `registrationNumberTypeNamesOf` 键带层，`RegistrationNumberTypeNameOf` 多一个可选 `layer`——同国同码两层都登时各取各的名，不交层按身份层优先（`identityLayerCellsOf` 与 `legal-entity-revisions.ts` 的老调用方不动），只登一层的码不论问哪层都译；`LegalEntityProfileContent.tsx`、`compareLegalEntityProfiles` 与快照问资料层；`LegalEntityProfileHistory.tsx` 收一格 `typeNameOf`，对比表、版本摘要与快照里的类型码都译名，`LegalEntityProfileSection.tsx` 把已读好的目录查法交进去。
- 收尾 `1d35f5066`：`git merge origin/main`（票 05 已进 main，契约与 `a34565442` 相同），无冲突。
- 用语照 spec 表与票面新词：「监管登记号（税务 / 海关）」「收款账户」「其他语言的抬头」「主抬头语言」；空值「—」，结论句「未登记收款账户」照 ADR-0359 对旧修订的读法。
- 落点之外只动 `api.ts` 一句答复译名（「税号」→「登记号」）与 `legal-entity-identity.ts` / `.test.ts`（票面第 4 条点名）。`GroupLegalEntitiesPage.tsx` 使用说明里「开票抬头、税号、地址和联系人叫公司资料」一句的「税号」没改（不在落点），留给协调方裁。

**判据**

- ✅ 三道门退 0（`tsc -b --noEmit`、`run-tests.mjs` 1163 过 0 败、`vite build`），每笔各跑一次，合入 origin/main 后再跑一次。
- ✅ 纯函数钉 node:test：`legal-entity-profile.test.ts`（草稿 ↔ 载荷含三组新格、缺项按格点名、重复账户 / 重复语言 / 半填行、没有主抬头时的提示、维护提示「有抬头没账户」、币种候选、语言译名）、`legal-entity-profile-history.test.ts`（账户新增 / 修改 / 移除与次序、抬头按语言对应）、`legal-entity-identity.test.ts`（按层分键）。
- ✅ 浏览器（一次性后端：票 05 `a34565442` 的 worktree 起 `up.sh demo_csux06 --reset`、parcel-api `:19106`；vite `:5206` + `VITE_DISPLAY_TIME_ZONE=Asia/Shanghai`；playwright 脚本与截图在作者 scratchpad `csux06/`：`verify.js`、`verify-ro2.log`、`verify-submit.log`）：
  - `SYN-LE-CN-SH-01` 概要签读出：「监管登记号（税务 / 海关）」显「海关注册编码 0000000101；合成税务登记号（演示用，非真实格式） SYN-CN-TAX-0101」；开票抬头后弱化显「简体中文（zh-Hans）」；「其他语言的抬头」显「英语（en） IDP Logistics Technology (Shanghai) Co., Ltd. (Demo)」；收款账户表两行（人民币 / 美元）各带用途、户名、开户行与地址、账号与 SWIFT、备注 ✅ `01-readout-SH-01.png`
  - 资料版本签第 3 版 vs 第 2 版对比表：「监管登记号 已修改」（类型码已译名）、「主抬头语言 新增」、「其他语言的抬头 · 英语（en） 新增」、两行「收款账户 · 人民币（CNY） SYN-CNY-… 新增」「收款账户 · 美元（USD） … 新增」；快照多「主抬头语言」「其他语言的抬头」「收款账户 · 2」✅ `02-history-diff-SH-01.png`、`03-history-snapshot-SH-01.png`
  - 抽屉预填：开票抬头、主抬头语言 `zh-Hans`、其他语言 `en` 行、两个账户的八格都带入；币种格是目录选单（显「人民币（CNY）」）✅ `04-drawer-titles-SH-01.png`、`05-drawer-accounts-SH-01.png`
  - 加一行空账户点保存：「还有 4 项必填未填：收款账户 3 户名、开户行、账号 / IBAN、币种」，焦点落在户名 ✅ `06-missing-account-fields-SH-01.png`
  - 清掉美元账户后差异预览一行「移除 收款账户 · 美元（USD） SYN-USD-0000000101」✅ `07-preview-remove-account-SH-01.png`
  - 改备注 + 换主抬头语言 + 加 `ja` 抬头：预览「修改 主抬头语言」「新增 其他语言的抬头 · 日语（ja）」「修改 收款账户 · 人民币（CNY） …」✅ `08-preview-modify-SH-01.png`；语言填 `EN` 与已有 `en` 重复时格下「此语言已经填写」✅ `09-dup-language-SH-01.png`
  - `SYN-LE-01` 第 5 版读出：一个人民币账户、`en` 抬头、三个监管登记号（含纳税人识别号）；抽屉预填同样 ✅ `10-readout-LE-01.png`
  - **真提交一次**（只对一次性库）：加 `zh-Hant` 抬头与账户备注 → 预览两行 → 「确认保存」→ 登记册答「已登记」，正好一次 `POST /api/commercial-legal-entity-profile-registrations`；版本历史出现「第 6 版 未来生效 2 个字段发生变化」；按日期查 2026-12-02 读出第 6 版，其他语言的抬头两行、账户备注带上 ✅ `11-preview-before-submit-LE-01.png`、`12-submit-answer-LE-01.png`、`13-history-after-submit-LE-01.png`、`14-lookup-v6-LE-01.png`
  - 只读跑零写请求、零 pageerror；提交跑只有那一次 POST。验完 `down.sh --drop-db`、停 vite、`git worktree remove --force` 后端树。

**未验**

- 币种目录读不回时的手填格（`BankAccountCurrencyField` 的退路）只过 tsc 与 build，演示库目录读得回。
- 服务端对（币种，账号）重复、语言重复、主抬头空着带语言版本的拒绝答复在格下怎么显，未在浏览器触发（本地先提了，要绕过本地提示才会到服务端）。
- 工作台详情栏（`LegalEntityDetailPanel`）的概要签走同一个 `LegalEntityProfileContent`，未另截图。
