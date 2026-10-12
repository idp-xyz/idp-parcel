# 05 管理台：客户服务规则表单的合同版——空表即继承、只收紧不删减

Category: enhancement
Status: resolved——2026-10-12 12:2x 通道 2 在 main 上收口（经 IDP 队列接手，用户令继续）：完成判据二按其后半句如实记下——五步停在第 2 步「预览」，答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`，因为生产装配里发布四口与词表口都挂字面量 `UnconfiguredIntake{}`；走通要等 operator-channel/07 与 06，不在本切片。取证见文末「浏览器五步取证」。此前 in-progress——2026-10-12 11:0x 通道 1 在 main 上做（前端切片，用户令独立完成）：文案与提示进 main，三道门退 0；完成判据二「浏览器五步」本轮未走，票不收口，见文末完成记录。此前：ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: [04](04-ve-claim-eligibility-checks-contract-and-inherits-rows-from-product-base.md)——文案不赶在行为前面
父票：[spec](../spec.md)
地盘：`apps/admin-web` 的客户服务规则表单；前端切片，在 `main` 上做（[workflow.md「前端切片」](../../../docs/agents/workflow.md#前端切片一人在-main-上直接做)）。
出处：[ADR-0176](../../../docs/adr/0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定三、六；admin-write-faces/18（现表单已有合同格与壳引用 `CUSTOMER_CONTRACT`）。

## 做什么

1. 空表语义改写：挂产品时无行即未登记；挂合同时无行即继承同范围产品版该行。
2. 加「只收紧」提示。
3. 浏览器发一版挂合同规则，走通五步。

## 判断项（拆票时裁定）

- **只收紧怎么守（Q3）**：本拆法只落 ADR-0176 决定三「不提供删减表达」那一半，不设「合同版行比底座更严」的比对门——底座是读时按锚点选的，起算事件或日历不同的两条期限行也比不出严宽。spec 用户故事 3 据此读作「不能借空行放宽成删减」；收紧是商务纪律，由本票的提示承担。**越权风险点 · 待 PC owner 复核。** **owner 复核 2026-10-12 认可**（用户 2026-10-12 12:3x 经 IDP 队列通道 2 授权「请你帮我自决」，通道 2 代裁）：底座按锚点读时才选，发布时不知道会配上哪一版；起算事件或日历不同的两行没有可比的严宽；更要紧的是合同版是客户签了的条款，系统拒发一条比产品宽的合同条款，等于拒绝如实表达已签的合同，与 spec 用户故事 6「合同承诺什么系统就判什么」相反。删减那一半已由结构守住（没有显式空行），收紧由表单提示交给发布人。
- **预览键（Q6）**：只做前端；预览口不改成答继承后的整行。实现时发现非碰 Go 不可，就停下来报，这张票不再是前端切片。

## 完成判据

- [x] 三道门退 0。
- [x] 浏览器五步走通；做不到就如实写哪一步、为什么。——◑ 按后半句收：停在第 2 步「预览」（403，发布四口未配置），见「浏览器五步取证」。
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

## 浏览器五步取证（2026-10-12 12:2x，通道 2，main 上）

**结论**：五步走不通，停在第 2 步「预览」。第 1 步表单的文案与 04 一致；从第 2 步起发布四口一律答 403
`ACCESS_CHANNEL_NOT_CONFIGURED`，页面如实显示，「存为待批准」钮不出现。原因不在本票：生产装配
`cmd/parcel-api/endpoints.go` 里 `/commercial-publication-previews`、`/commercial-publication-drafts`、
`/commercial-publication-draft-approvals`、`/commercial-publication-draft-publications` 与词表口
`/commercial-publication-vocabularies` 各行都挂字面量 `commercialhttp.UnconfiguredIntake{}`，没有任何环境开关能换；
两道隔离开关都开时，启动日志的 `admittedCommandLines` 里也没有任何 `/commercial-publication*`。批准门因此根本没被
触到——上一轮预计的「批准那一步可能因审批职责规则没登记而答未配置」不成立，停得更早。

**走通要等**（都不在前端切片内）：[operator-channel/07](../../operator-channel/issues/07-admin-web-login-gate.md)
（管理台请求带 Bearer、演示环境接发行方）与
[operator-channel/06](../../operator-channel/issues/06-commercial-publication-and-replay-take-operator-identity.md)
（发布四口与词表口换操作者 Intake）。两张落地之后再走，批准那一步才轮到审批职责规则答话。

**环境**（证据层级 S）：钉 main `a0804038`。门禁库容器上另开临时库 `csr05_probe`，`scripts/demo-seeds/seed.sh`
灌入零报错；`parcel-api` 由同一 SHA 编出，监听 `127.0.0.1:19181`，`IDP_PARCEL_ISOLATED_READ_TENANT` 与
`IDP_PARCEL_ISOLATED_WRITE_TENANT` 都设 `SYN-TENANT-01`（与 `parcel.sh` 同值），操作者渠道未配置（启动日志如实报出）。
取证完毕后进程已停、临时库已删；`parcel.sh` 用的那只演示库没碰。

**为什么不是真浏览器**：`AuthGate` 包在应用最外层，要 gk.idp.xyz 的会话，本会话没有凭据。改用
`apps/admin-web/scripts/dom-probe.mjs` 挂**真的** `CommercialPoliciesPage`，探针里把 `fetch` 按 vite 代理同一规则
（剥 `/api` 前缀）转给上面那只 `parcel-api`——绕开的只有登录门，请求与答复都是真的。探针源在仓外，未入库。
happy-dom 20.14.5，Node v22.23.3。

**逐步**（探针断言全部 ok、零 FAIL，退 0）

1. 填表 ✅：词表口答 403，期限种类显占位、不自造码；适用对象未选时两张空表都说两种读法、不显「只收紧」；选「随某个
   客户合同」两张空表都说继承、显「只收紧」；改选「随某个服务产品」两张都说未登记、「只收紧」消失；加一行期限之后
   只剩材料表说继承。
2. 预览摘要 ❌：`POST /commercial-publication-previews` 答 403 `ACCESS_CHANNEL_NOT_CONFIGURED`；页面显「预览：接入渠道
   未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）」与既有的未配置说明；动作钮仍是「预览」，「存为待批准」钮没有出现。
3. 存为待批准、批准、发布：页面走不到。同一份载荷经页面自己的客户端函数 `submitPublicationDraft`、
   `approvePublicationDraft`、`publishPublicationDraft` 直打三口，都答 403，归 `unconfigured` 一格。

**读面顺带取证** ✅：`GET /commercial-policies?kind=CUSTOMER_SERVICE_RULE` 经隔离读放行答 200。政策册客户服务规则
一栏里，合同版 `SYN-SERVICE-RULE-CONTRACT-02` 的最低材料格显「无行·继承产品版」；产品版 `SYN-SERVICE-RULE-01` 的
材料格显 `SYN-CLAIM-LOSS:SYN-DOC-POD-01`，不出现「无行」。

**本笔顺手一处**：`a0aae6a1` 把两个新函数插进了 `emptyServiceRuleDraft` 的文档注释与函数之间，「空草稿」那段注释
挂到了 `serviceRuleEmptyTableNote` 头上；另一笔把它挪回 `emptyServiceRuleDraft` 上方，行为零变化。三道门（`a0804038`
加该笔改动的工作树，WSL，Node v22.23.3）：`tsc -b --noEmit` 退 0；`run-tests` 487 tests / 487 pass / 0 fail；
`vite build` 退 0（只有既有的 chunk 体积提示）。

**评审**：只动 `pages/**` 一处注释位置与票面，按 workflow.md 第 5 步自查即可。判断项 Q3 那条越权风险点仍待 PC owner
复核，不因本票收口而消。
