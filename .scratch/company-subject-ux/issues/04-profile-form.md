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
