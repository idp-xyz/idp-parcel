# 05 管理台：客户服务规则表单的合同版——空表即继承、只收紧不删减

Category: enhancement
Status: in-progress——2026-10-12 11:0x 通道 1 在 main 上做（前端切片，用户令独立完成）：文案与提示进 main，三道门退 0；完成判据二「浏览器五步」本轮未走，票不收口，见文末完成记录。此前：ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: [04](04-ve-claim-eligibility-checks-contract-and-inherits-rows-from-product-base.md)——文案不赶在行为前面
父票：[spec](../spec.md)
地盘：`apps/admin-web` 的客户服务规则表单；前端切片，在 `main` 上做（[workflow.md「前端切片」](../../../docs/agents/workflow.md#前端切片一人在-main-上直接做)）。
出处：[ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定三、六；admin-write-faces/18（现表单已有合同格与壳引用 `CUSTOMER_CONTRACT`）。

## 做什么

1. 空表语义改写：挂产品时无行即未登记；挂合同时无行即继承同范围产品版该行。
2. 加「只收紧」提示。
3. 浏览器发一版挂合同规则，走通五步。

## 判断项（拆票时裁定）

- **只收紧怎么守（Q3）**：本拆法只落 ADR-0176 决定三「不提供删减表达」那一半，不设「合同版行比底座更严」的比对门——底座是读时按锚点选的，起算事件或日历不同的两条期限行也比不出严宽。spec 用户故事 3 据此读作「不能借空行放宽成删减」；收紧是商务纪律，由本票的提示承担。**越权风险点 · 待 PC owner 复核。**
- **预览键（Q6）**：只做前端；预览口不改成答继承后的整行。实现时发现非碰 Go 不可，就停下来报，这张票不再是前端切片。

## 完成判据

- [x] 三道门退 0。
- [ ] 浏览器五步走通；做不到就如实写哪一步、为什么。
- [x] 文案与 04 的行为一致。

## 完成记录（2026-10-12，通道 1，main 上）

**落点**（全在 `apps/admin-web/src/pages/party/`，未碰 Go）

- `customer-service-rule-form.ts`：新 `serviceRuleEmptyTableNote(appliesTo, table)`——挂服务产品无行答「索赔时这一维答未登记」，挂客户合同无行答「继承同一范围挂服务产品那一版的同一行，产品版也没有才答未登记」，未选两种都说；新 `serviceRuleTightenOnlyNote(appliesTo)`——只对挂合同显，说明只收紧、没有删减写法、系统不比对两版孰严（判断项 Q3）。
- `CustomerServiceRulePublicationForm.tsx`：两张子表的零行提示改用上一条；挂合同时在两张表上方显「只收紧」提示。
- `policy-rows.ts`：政策目录两张子表空数组的格由「无客户差异」改为按适用对象分两句——挂产品「无行·索赔答未登记」、挂合同「无行·继承产品版」。旧句把两种读法折成一句，挂产品的那一版读起来像「这一维没有差异、照常」，而 04 之后它答的是未登记。
- 测试：`customer-service-rule-form.test.ts` 加两条（零行提示三格、只收紧提示只对合同显）；`policy-rows.test.ts` 改挂产品那一格的期望、加挂合同空表一条。

**完成判据**

- ✅ 三道门（钉 main `832a24e1` 加本笔改动的工作树，WSL，Node v22.23.3）：`node node_modules/typescript/bin/tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 退 0，487 tests / 487 pass / 0 fail；`pnpm exec vite build` 退 0（只有既有的 chunk 体积提示）。
- ❌ 浏览器五步本轮未走。走它要一只灌好种子的演示库、按操作者渠道配好的 `parcel-api`、`admin-web` 开发服务器与浏览器，再以发布人与批准人两种身份各走一段；本轮没起这一套，不是走了某一步不通。下一轮照 workflow.md「前端切片」的门补走，结论写在这里。
- ✅ 文案与 04 一致：挂产品无行 → 04 `translateRule` 那一维 `Registered=false`（未登记）；挂合同无行 → 04 取底座那一行、底座也无才未登记；「系统不比对孰严」与 04 / 判断项 Q3 一致（VE 与发布面都没有比对门）。
- 判断项 Q6（预览键）：没碰预览口，仍是前端切片。
- 评审：只动 `pages/**`，按 workflow.md 第 5 步业务页自查判据即可，不另评。
