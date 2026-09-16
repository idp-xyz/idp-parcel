# 10 参与方身份 / 参与方关系 / 身份停用三册逐字段表单（ADR-0101 决定八自裁），JSON 快照签降为折叠区

Category: enhancement
Status: resolved
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

## 完成记录（通道 6 · 分支 `mcp6-adminweb10` 基 main `f6010739` · 2026-09-16 22:5x → 23:1x）

**笔**（每笔 green 即推分支，`git push -u origin mcp6-adminweb10`）：`ee15cc65` 认领 → `a66df9b6` 参与方身份纯逻辑 + test →
`073f577c` 关系纯逻辑 + test → `cda11680` 停用纯逻辑 + test + `presentation.ts` 补 `identityKindLabels` → `ad59ae97` 三份表单组件 +
`party-registration-fields.tsx` 共用格 → `5938948c` 页面接线（列表状态上抬、登记签换表单）→ `14bb61ae` 作者自审去跨文件计数（只改注释）
→ 本笔完成记录。新件 `.ts` 6 件（三份 `*-form.ts` + 三份 `*-form.test.ts`）、`.tsx` 4 件（三份表单 + `party-registration-fields.tsx`），
改 `BusinessPartiesPage.tsx` 与 `presentation.ts`。

**三道门**（隔离树 `D:/tops/idp-parcel-mcp6-adminweb10`，全新 `pnpm install --frozen-lockfile`）：`tsc -b --noEmit` 0 /
`run-tests` **276 pass**（基线 254，本票 +22）/ `vite build` 0；`14bb61ae` 上复跑同数。全 TS，未跑全仓 Go、未占 55432。
**清点**：生成器不数 `apps/admin-web` 文件，预期零差，未另起清点笔。

**完成判据逐条**：
- 三道门绿 ✓（上）。
- 三份 `*-form.test.ts` ✓：修订号建议（新标识 1 / 已有取最新 + 1 / 列表 null 一律 1 / 标识空 1 / **按原串比、不裁空白**——
  `" SYN-PARTY-01"` 建议为 1，与载荷同判）；时刻编码含非法日期（`yesterday`、`2026-02-30T08:00`、`2026-13-01T00:00`）；可缺键缺席
  （关系的 `effectiveEndsAt` 留空、不勾「已批准」时 `approval` 整键缺席且批准两格残字不进；勾了但批准时刻留空则 `approval` 只带
  `reference`）；载荷顶层无 `tenantId`（三份各断言一次）。
- 演示形态 ✓（服务端半边 + 表单编码层）：本机 parcel-api `127.0.0.1:8090`（pid 45720，`dbe989cc` 构建，`SYN-TENANT-01`），用
  `run-tests` 编出的 `.tmp-test/pages/party/*-form.js` 组载荷直投，与表单送出的字节同源。① 登参与方 `SYN-PARTY-AWGLE10-20260916150119`
  （建议修订 1，`effectiveFrom` `2026-09-16T08:00` Asia/Shanghai → `2026-09-16T00:00:00Z`）→ **201 `REGISTERED`** → `GET
  /commercial-business-parties` 该行在（r1、`EFFECTIVE`）；② 登关系 `SYN-REL-AWGLE10-20260916150119`（持有方 = 刚登的、相对方 =
  既有 `SYN-PARTY-AGENT-01`、角色 `CUSTOMER`、不带批准、终点留空——载荷里 `effectiveEndsAt` / `approval` 两键都不在）→ **201
  `REGISTERED`** → 关系册该行 `CANDIDATE`、双方名称都转写到（`holderNameKnown` / `counterpartyNameKnown` 皆 true）；③ 停用刚登的
  （建议修订 2 = 最新 1 + 1，`at` `2026-09-16T20:00` → `12:00:00Z`）→ **201 `DEACTIVATED`** → 该行 `status: DEACTIVATED`、
  `deactivatedAt: 2026-09-16T12:00:00Z`、`deactivationBasis: SYN-BASIS/awgle10-deact`，即身份签「已停用」+ 时点 + 依据三样齐。
  第一次跑时 `at` 填了次日，行答 `EFFECTIVE` 而带停用两件——状态按时点导出，那是正确答案，不是缺陷；为让判据句逐字可核才用过去
  时点再跑一遍（另落一个参与方 `…150050`，同租户可留）。红线探针：同载荷加 `tenantId` 键 → **400 `MALFORMED_REQUEST`**（这只旧进程
  不含票 11 的 `detail`，页面「原因：」行看不到属正常，派单已预告）。**浏览器未验**（AuthGate 无会话），如实记。

**要做的逐条**：
1. ✓ 参与方身份五格；`revision` 建议同 02 的形状（只建议不裁）；`effectiveFrom` 墙钟 → RFC 3339；不带租户格。
2. ✓ 关系表单：双方 `ReferencePicker` 从参与方册选（名称 · 标识 · 状态，不按状态过滤，读面不可用退回手填）；角色下拉
   `partyRoleOptions()` 只从 `partyRoleLabels` 派生；`effectiveEndsAt` 留空即缺键；「已批准」勾选框决定 `approval` 键在不在，
   不勾即候选关系。**Picker 读一次只读一次**：页面把参与方列表的重取序号 `partiesVersion` 传进来作 key，参与方册重取后双方
   候选重读，刚登的参与方能立刻出现在候选里（判据②要的正是这一步）。
3. ✓ 停用表单：种类下拉 `identityKindOptions()` 从 `identityKindLabels`（CONTEXT 原词「业务参与方 / 责任法人 / 货主客户账户」）
   派生；标识按种类换 `ReferencePicker`（`listBusinessParties` / `listGroupLegalEntities` / `listCustomerAccounts`，换种类连标识一起
   清）；建议 = 该身份最新修订 + 1，`deactivationTargetsOf` 先按种类把对应册投成「标识 + 修订」再数，参与方册由页面持有传入、
   法人册与客户账户册在表单里各 `useLoaded` 一次（共享 Picker 只交候选不交行，改它归收口票）。
4. ✓ JSON 快照签降为折叠区 `SnapshotJsonDetails`，每册一个，提示句取 `registrationSnapshotHints`（票 08 改后那份）。
5. ✓ 三表单共用 `RegistrationAnswerNote`；参与方册答 `REGISTERED` / 停用答 `DEACTIVATED` → 重取参与方列表，关系册答
   `REGISTERED` → 重取关系列表；`useRegisterList` 各持一份答案 + 重取序号，两张读签只收 `answer` / `retry`，列 / 筛选 / 抽屉（09）
   一字未动；登记签从这份答案取修订建议（列表没取到传 null → 建议一律 1，不拿空数组冒充「册上没有」）。停用的 JSON 镜像路上分不出
   种类（快照是未译 JSON），一律重取参与方列表——多一个 GET，不为分种类去解一份 unknown。
6. **判断项，不做**：派单写明票 12（通道 4）的前端半边要往 `BusinessPartyDrawer` 的「修订历史」区接真数据、等本票进 main 后才碰
   该文件，并要求本票**不改抽屉组件的签名**；「登记下一修订」要给抽屉加一个回调（切签 + 预填），正是换签名。等 12 进 main 后再
   作为收口项做，那时抽屉已有整条修订链，预填「当前 + 1」也才有据。

**红线**：表单不算摘要、不裁任何门、不判领域规则——本地只拦修订号编不进正整数与时刻换不成 RFC 3339（`*LocalProblems`），角色未选、
双方为空、区间倒置、依据为空都照送让服务端答（测试钉着）；不带租户格（三份测试各断言）；载荷键名逐字镜像 `businessPartyDocument` /
`partyRelationshipDocument`（含 `relationshipApprovalDocument`）/ `deactivationDocument`，**身份串不裁首尾空白**——与票 02 法人表单
相反、与服务端及 CLI 一致（`businessPartyDocument` 注释），修订号例外（编成的是数不是身份串）。

**超票面 / 判断**：`revisionOf` 从 `business-party-form.ts` 导出供另两份同判；`party-registration-fields.tsx` 抬出修订号格、墙钟时刻格、
三册候选转写与折叠区（票 09 评审 N3 点过同形副本，本票不再各留一份）；登记签的选册 chip 与 `MultiRegistrationPanel` 同形但那份未
导出，页面里留一份并注明；换册即换表单（草稿是各表单内部状态，切走清空），与 `MultiRegistrationPanel` 换册清草稿同一条纪律。

**跟进（归收口票，本票不碰他人文件）**：票 02 `LegalEntityRegistrationForm.tsx` / `legal-entity-form.ts` 仍各留一份修订号格、时刻格、
`partyOptionsOf` 与裁空白的 `suggestedRevision`——切到 `party-registration-fields.tsx` 并统一「不裁空白」判据；`chipClass` 抬成共享件；
`ReferencePicker` 若交出行（或收一份已取回的答案）可免停用表单那两次重复读。

**不做四条**：未改服务端 / 端点；无「编辑」按钮；两签的列与筛选未动；未碰 spec.md / tasks.md / 清点。

## Comments
