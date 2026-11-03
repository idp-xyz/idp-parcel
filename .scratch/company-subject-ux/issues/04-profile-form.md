# 04 公司资料表单：默认基于当前版本、保存前差异预览、未保存保护、缺项提示、税务登记号类型从目录选、币种指引

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无
出处：评估「三、交互与任务流」前五条、「四、国际小包视角」税务与币种两条。

## 现状

`LegalEntityProfileSection.tsx` 的 `ProfileRegistrationForm`：空白起步，「从已登记版本带入」要先选版本再点「带入此版本」；表单藏在「资料版本」签底部；没有未保存保护；必填缺项要点保存才由服务端告知；税务登记号类型是自由文本「税务登记号类型码」；表单不说币种在哪定。

## 要做的

1. **默认基于当前版本**：打开表单时自动带入当前生效的资料版本（`useCurrentLegalEntityProfile` 已有答案就直接用，没有就等它）；顶部一行「基于 第 4 版（2026-10-30 起生效）修改 · [改为从空白开始]」。没有任何已登记版本时从空白开始，说「这是第一版资料」。「从已登记版本带入」下拉保留为次选（带入别的版本）。
2. **保存前差异预览**：点「保存」先展示「这次改了什么」——复用 `legal-entity-profile-history.ts` 的 `compareLegalEntityProfiles`（把草稿折成 `LegalEntityProfileContent` 形状与基线比），列新增 / 修改 / 移除的字段；预览里再点「确认保存」才发。没有基线（第一版）时跳过预览直接发。空白项会移除旧值这件事由预览的「移除」行直接显示，不再靠说明文字。
3. **未保存保护**：表单有没提交的输入时，关闭详情栏、换签、换行、离开对象页先确认（照 `GroupLegalEntitiesPage.tsx` 登记视图的 `ConfirmDialog` 写法；`useRegistrationForm` 的 `guardUnsaved` / `unsaved` 看能不能直接用）。
4. **缺项提示**：本地只标「必填未填」（注册地址国家 / 地区、注册地址、来源单据、生效时间、联系人姓名），点保存时把缺项列在按钮旁并把第一个缺项滚进视野；不判任何领域规则（国家一致、税号合规、版本连续仍由服务端答）。
5. **位置**：表单改成从页头「更新公司资料」打开的**抽屉**（右侧，宽度与详情栏同级；`@idpxyz/ui-primitives` 有 Drawer / Sheet 就用，没有就用 Dialog `size="lg"`），不再藏在「资料版本」签底部；「资料版本」签底部留一个「更新公司资料」按钮也打开同一抽屉。
6. **税务登记号类型从目录选**（`LegalEntityProfileFields.tsx`）：类型格改为 `ReferencePickerFor`，候选来自 `listRegistrationNumberTypes` 里 `layer` 为资料层（后端 `RegistrationNumberProfileLayer` 的线上名，看目录答复里的值）且国家 = 注册地址国家的类型，显示 `typeName`；目录里没有时允许手填码并提示「要加类型，先到「基础数据 › 注册号类型」配置」。说明一句：「跨境开票常用：中国税号、香港 BR、美国 EIN、欧盟 VAT / IOSS / EORI——按注册地配置后在这里选」。
7. **币种指引**：「税务与开票」组说明加一句「开票币种不在这里定：由「商业规则 › 结算政策」按公司主体、相对方与合同解析」，带链接到结算政策页（`commercial-policies-address.ts` 的 `policiesHash`）。
8. **用语**（本票落点）：`legalEntityProfileStatusOf` 的 label / description 改成 spec 表的「可开票 / 资料不全 / 未登记资料 / 主体未生效 / 主体已停用」与对应说明；「法人资料」→「公司资料」；「更新序号」→「版本号」；「来源单据」统一；「按时点查看法人资料」→「查看某个日期的资料」；「通用联系」→「日常联系」（`contactPurposeLabels`）；`LegalEntityProfileSummary.tsx` 技术详情「资料来源」行名照旧。
9. 纯函数（草稿 ↔ 内容折算、缺项判读、预览 diff 的输入折算）钉 node:test。

## 不做

- 不改写口载荷形状；不在本地判领域规则。
- 不动票 01、02、03 的落点。
- 海关备案、银行账户、多语言抬头归票 05。

## 完成判据

- 三道门退 0；新增纯函数有 node:test。
- 浏览器在演示租户核（不真提交）：打开抽屉即预填第 4 版；改一格点保存出现差异预览且只列那一格；清空开票抬头后预览出现「移除 开票抬头」；关抽屉有确认；税务登记号类型下拉显目录译名；币种一句带链接。

## 完成记录（2026-11-02，分支 `claude-csux-04`，自 main `ae9a9a763`，已合入 origin/main `5bc6ff32e`）

### 落点

| 文件 | 做了什么 | 提交 |
|---|---|---|
| `legal-entity-profile.ts` | 状态词五格改「可开票 / 资料不全 / 未登记资料 / 主体未生效 / 主体已停用」与说明句；`contactPurposeLabels.GENERAL`「日常联系」；`addressText` 空值「—」；新增 `legalEntityProfileDraftBasedOn`（预填内容格、来源单据 / 生效时间 / 版本号空着）、`profileComparisonContentOf`（草稿经载荷折成资料内容给预览比）、`legalEntityProfileMissingFields` + `problemsWithMissing`（必填未填，按表单次序）、`profileTaxTypeOptions`（目录 `layer === 'PROFILE'` 且国家 = 注册地址国家，显 `typeName`）；删掉没有消费者的 `profileContentLines` / `profileRevisionTimeline` / `profileRevisionHistoryNote` | `35021148d`、`e384dfe1b` |
| `legal-entity-profile.test.ts` | 上述纯函数各一条（预填、不动无差异 / 改一格列一格 / 清空抬头是「移除」/ 半空联系人行丢掉 / 译名比、缺项次序与并进问题表、资料层候选、空地址「—」）；状态词字面随改 | 同上 |
| `LegalEntityProfileFields.tsx` | 税务登记号类型格 `TaxTypeField`：目录有候选时 `SearchableSelect`（显 `typeName`，已停用括注，手填值保留为一项），没有 / 读不到时手填码并指路「基础数据 › 注册号类型」；「税务与开票」组一句币种由「商业规则 › 结算政策」解析（`settlementPolicyHash`，点击交 `onLeave` 走未保存确认）；跨境常用税号一句；`FieldAnchor`（`data-field-path`）包住必填格给滚动用；代表人身份类型空项「未选」 | `38d1b1a2f` |
| `LegalEntityProfileSection.tsx` | 导出受控抽屉 `LegalEntityProfileDrawer`（`open` / `onOpenChange` / `legalEntityId` / `onLanded`）；本区沿用 `formOpenRequest` 打开它，底部留「更新公司资料」按钮；抽屉体读当前资料 + 版本清单 + 注册号类型目录后挂表单；顶行「基于 第 4 版（2026-10-30 起生效）修改 · 改为从空白开始」/「从空白开始 · 改回基于第 4 版」/「这是第一版资料」，「从已登记版本带入」次选；点保存：缺项 → 按钮旁清单 + 格下「必填未填」+ 第一个缺项滚进视野并聚焦；齐了且有当前生效版 → `ChangePreview`（与当前版比，只列变化行、`修改 / 新增 / 移除` 标在项目前）→「确认保存」才 `form.send()`；第一版直接发；关抽屉（叉 / Esc / 遮罩）与站内链接离开走 `ConfirmDialog`；落册后脚部换「关闭」；「查看某个日期的资料」答复用 `legalEntityProfileStatusOf` + `StatusPill`，「生效版本 · 来源单据」；「版本号」 | `38d1b1a2f`、`e384dfe1b` |
| `CurrentLegalEntityProfile.tsx` | 各句「法人资料」→「公司资料」、「法人」→「公司主体」 | `e2e8460b7` |
| `LegalEntityProfileSummary.tsx` | 生效时刻 `Instant precision="day"`；读一次注册号类型目录把税务登记号类型码译成目录译名交给读出；技术详情「资料来源」行名照旧 | `e2e8460b7`、`e384dfe1b` |
| `LegalEntityProfileContent.tsx`（落点外，票 01 已进 main） | 可选 `typeNameOf`：税务登记号显目录译名、码在 `title`；不传照旧显码 | `e384dfe1b` |
| `legal-entity-profile-history.ts`（落点外，票 01 已进 main） | `compareLegalEntityProfiles` 可选第三参 `typeNameOf`，税务登记号行按译名比；不传照旧 | `e384dfe1b` |

抽屉用的件：`@idpxyz/ui-primitives` 有 `Drawer`，但定宽 `w-96`（24rem）且不收 `className`，装不下两列的资料表单；改用同库 `Dialog` + `DialogContent`（收 `className`，`cn` 走 tailwind-merge）覆盖成贴右整高、`w-[min(60rem,94vw)]` 的面板（实测 1440 宽下 960px，与工作台详情栏同级），`DialogHeader / Title / Description / Close / Body / Footer` 照用。`LegalEntityDetailPage.tsx` / `LegalEntityDetailPanel.tsx` 一行未动：页头按钮经既有 `formOpenRequest` 打开抽屉（实测两处都通）；票 03 若要直接挂抽屉，用导出的 `LegalEntityProfileDrawer`。

### 判据

- 三道门退 0（`git merge origin/main` 后，main `5bc6ff32e`）：`tsc -b --noEmit` 0、`run-tests` 1148 / 1148、`vite build` 0 ✅
- 新增纯函数有 node:test（草稿 ↔ 内容折算、缺项判读、预览 diff 的输入折算、资料层候选）✅
- 浏览器（演示租户 `SYN-LE-01`，自起 vite :5204 + `PARCEL_API_TARGET=http://127.0.0.1:19080` + `VITE_DISPLAY_TIME_ZONE=Asia/Shanghai`，playwright 脚本监听并断言**零写请求**、零 pageerror；脚本与截图在作者 scratchpad `csux04/`：`verify.js`、`shots2.js`、`shots3.js`、`verify.log`）：
  - 打开抽屉即预填第 4 版：顶行「基于 第 4 版（2026-10-30 起生效）修改 · 改为从空白开始」，4 位联系人、开票抬头「合成运营方一号（演示）」都带入；来源单据与生效时间空着 ✅ `02-drawer-prefilled.png`
  - 改一格（代表人姓名）点保存出现差异预览且只列那一格 ✅ `03-preview-one-change.png`（预览前先填来源单据与生效时间，否则按第 4 项先报缺项）
  - 清空开票抬头后预览出现「移除 开票抬头」（另一行「修改 开票资料 已登记 → 未登记」）✅ `04-preview-remove-title.png`
  - 缺项：「还有 2 项必填未填：注册地址、来源单据」列在按钮旁，注册地址格下「必填未填」，焦点落在注册地址 ✅ `06-missing-fields.png`
  - 关抽屉有确认：Esc → 「关闭表单？有内容还没有保存…」→「继续填写」留下、叉 →「放弃并关闭」才关；没改过东西时 Esc 直接关 ✅ `07-close-confirm.png`、`08-after-close.png`
  - 税务登记号类型下拉显目录译名：选项「未选 / 合成税务登记号（演示用，非真实格式）」，值 `SYN-CN-TAX` ✅ `12-drawer-tax-and-invoicing.png`
  - 币种一句带链接：`#/commercial-policies?register=SETTLEMENT_POLICY` ✅ 同上
  - 「改为从空白开始」清空内容格、「改回基于第 4 版」带回 ✅ `14-drawer-blank-base.png`
  - 工作台详情栏页头按钮打开同一抽屉、同样预填 ✅ `10-panel-drawer.png`
  - 资料版本签「查看某个日期的资料」：「资料状态：可开票 · 查询日期 2026-11-03 · 生效版本 第 4 版 · 2026-10-30 起生效 · 来源单据 …」✅ `11-date-lookup-answer.png`
  - 概要签税务登记号显「合成税务登记号（演示用，非真实格式） SYN-CN-TAX-0001」、`title` 为码 ✅ `15-summary-tax-typename.png`
  - 目录答复里资料层的线上名是 `PROFILE`（身份层 `IDENTITY`）；演示库里资料层类型有 CN / SG / HK / US 各一（SG 另一已停用）✅ `verify.log`
- 「关闭详情栏、换签、换行、离开对象页先确认」◑：抽屉是模态的（Radix Dialog 遮罩 + 焦点圈），开着时这几处点不到，工作台的键盘动作在 `[role="dialog"]` 内也让位（`workbenchKeyAction` 的 overlay 分支），所以确认只守关抽屉与站内链接两处；浏览器后退 / 刷新仍由 `useRegistrationForm` 的 `beforeunload` 守，hash 后退没守。

### 未验 / 留给别的票

- 真发送（「确认保存」→ `POST`）未在浏览器核，按票面只到差异预览；发送路径与原表单同一个 `useRegistrationForm.send`，落册判据未动。
- 版本历史（`LegalEntityProfileHistory.tsx`）的对比表与快照仍显类型码：它没带目录，`compareLegalEntityProfiles` 的 `typeNameOf` 已备好，接上要读一次目录——归票 01 的落点，留给协调方裁。
- 同一国同一码两层都登时 `registrationNumberTypeNamesOf` 取身份层的名（它的既定规则），税务登记号会跟着显身份层的名；演示库没有这种码。
- 页头按钮仍叫「更新法人资料」（`LegalEntityDetailPage.tsx` / `LegalEntityDetailPanel.tsx`，票 03）。
- `profileMaintenanceNotes` 的提示句（「法定代表人 / 登记负责人未配置」等）未改措辞：是结论句，不在空值规则内。
