# 10 参与方身份 / 参与方关系 / 身份停用三册逐字段表单（ADR-0101 决定八自裁），JSON 快照签降为折叠区

Category: enhancement
Status: ready-for-agent
Blocked by: 09（同一文件 `BusinessPartiesPage.tsx`——09 改两张读签与抽屉、本票改登记签并把列表状态抬到页面持有；两票同时写一个文件就是
parallel-sessions「共享接线文件」那一格，先后做）
地盘：`apps/admin-web/src/pages/party/BusinessPartiesPage.tsx`（登记签那段 + 列表状态抬到页面）、新 `BusinessPartyRegistrationForm.tsx` /
`PartyRelationshipRegistrationForm.tsx` / `IdentityDeactivationForm.tsx` 与各自 `*-form.ts`（纯逻辑 + node:test）、`presentation.ts`
补 `identityKindLabels` 三词。复用票 02 的 `Field` / `ReferencePicker`（`PublicationFormFields.tsx`）、`suggestedRevision` 的形状、
`RegistrationAnswerNote`、`RegistrationPanel` 作折叠区。
出处：通道 1 评估「业务参与方」页（钉 main `dbe989cc`，2026-09-16 20:2x），spec「第二轮」表第三档。

## 为什么

ADR-0101 决定八让各册自裁登记面形态；票 02 已把法人五格做成逐字段表单，理由是「低频、结构简单、无矩阵」——参与方身份五格同一判据，
停用五格外加一个封闭三词，关系十格里有封闭五词与两个从册上选的引用。三册今天全是粘 JSON：角色词、种类词打错只会得到一个说不清
的 400（票 11 之前尤甚），修订号要自己数，双方标识要从另一签抄。关系表单价值最大，因为它的格最多是「必须逐字对上的词」。

## 要做的

1. **参与方身份表单**五格：`partyId` / `name` / `revision`（建议 = 册上同 `partyId` 最新修订 + 1，新标识 = 1；同 02 的
   `suggestedRevision` 形状——只建议不裁）/ `basis` / `effectiveFrom`（本地墙钟 → RFC 3339，编码层，同 02）。不带租户格。
2. **关系表单**：`relationshipId` / `revision`（建议同上，按 `relationshipId` 数关系册）/ `holder`、`counterparty` 用 `ReferencePicker`
   从参与方册选（候选显名称 · 标识 · 状态，不按状态过滤，可手填）/ `role` 下拉五词从 `partyRoleLabels` 派生 / `scope` / `basis` /
   `effectiveStartsAt` / 可选 `effectiveEndsAt`（留空即开区间，**缺键不是零值**——`partyRelationshipDocument` 用指针表达缺席，
   编码层要把空格编成缺键）/ 可选 `approval{reference, approvedAt}`（勾「已批准」才展开两格，不勾即候选关系、缺键）。
3. **停用表单**：`kind` 下拉三词（`BUSINESS_PARTY` / `LEGAL_ENTITY` / `CUSTOMER_ACCOUNT`，`presentation.ts` 补 `identityKindLabels`，
   中文取 CONTEXT 原词「业务参与方 / 责任法人 / 货主客户账户」）/ `id` 按 `kind` 从对应册 `ReferencePicker`（参与方册 `listBusinessParties`、
   法人册 `listGroupLegalEntities`、客户账户册用客户与合同页已有的读函数）/ `revision`（建议 = 该身份最新修订 + 1）/ `basis` / `at`。
4. **JSON 快照签降为折叠区**「高级：粘贴登记快照 JSON」（同 02），每册一个，提示句用票 08 改后的那份。
5. **答案与重取**：三表单共用 `RegistrationAnswerNote`；登记册答 `REGISTERED`（停用答 `DEACTIVATED`）时触发对应读签重取——两张读签的
   列表状态因此要抬到 `BusinessPartiesPage` 持有再传下去（`GroupLegalEntitiesPage` 的做法），登记签也从这份答案取修订号建议。
6. **可选（判断题，做就单独一笔）**：09 的抽屉里加「登记下一修订」——切到登记签、预填该行各格、修订号 = 当前 + 1。这是「更正 =
   登下一修订」在页面上唯一直观的入口；不做也关票，票面写明。

## 红线

- 表单**不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：修订连续、参与方在册、角色与双方是否匹配、区间是否
  合法，一律送上去让服务端答。本地只拦编码层（整数编不出、时刻换不成 RFC 3339、可缺键缺席）。
- **不带租户格**（spec 红线；`refuseSelfReportedTenant` 键在场即拒）。
- 载荷逐字镜像 `isolated_write_intake.go` 各 `*Document` 的键名；不裁切首尾空白（服务端与 CLI 都不裁，表单那层裁了会让两口对同一
  输入译出不同身份——`businessPartyDocument` 注释）。

## 不做

- 不改服务端、不改端点。
- 不做「编辑」按钮或行内改：登记册不可覆盖，更正是登下一修订（第 6 条是它的预填入口，不是编辑）。
- 不动两张读签的列与筛选（09）。

## 完成判据

- 三道门绿：`tsc -b --noEmit`、`run-tests`、`vite build`。
- 三份 `*-form.test.ts`：修订号建议（新标识 1 / 已有取最新 + 1 / 列表为 null 一律 1）、时刻编码含非法日期、可缺键缺席（关系两格、
  批准两格）、载荷里没有 `tenantId` 键。
- 演示形态（parcel-api 为 06 之后的构建、`IDP_PARCEL_ISOLATED_WRITE_TENANT=SYN-TENANT-01`）：登一个参与方 → 201 `REGISTERED` → 身份签
  重取可见；登一段关系（双方用刚登的与既有 `SYN-PARTY-…`）→ 201；停用刚登的参与方 → 身份签该行显「已停用」+ 时点 + 依据。
  写进的是合成租户，可留、不必清。浏览器验收做不到就如实写「未验」。

## Comments
