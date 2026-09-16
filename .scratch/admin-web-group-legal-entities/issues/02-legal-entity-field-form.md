# 02 登记法人改逐字段表单（ADR-0101 决定八自裁），JSON 快照签降为受控批量口镜像

Category: enhancement
Status: in-progress
Blocked by: 无
地盘：`apps/admin-web/src/pages/party/legal-entity-form.ts`（新，纯逻辑）、`apps/admin-web/src/pages/party/LegalEntityRegistrationForm.tsx`（新）、
`apps/admin-web/src/pages/party/GroupLegalEntitiesPage.tsx`（登记签换组件）、`apps/admin-web/src/components/registration/RegistrationPanel.tsx`
（把答案呈现 `AnswerNote` 抬成导出件 `RegistrationAnswerNote`，行为不变）

## 决定八的自裁记录（ADR-0101 要求每册在自己的实施票里写明选了哪一形与理由）

**选逐字段表单。** 责任法人身份登记一笔五格（`legalEntityId` / `partyId` / `revision` / `basis` / `effectiveFrom`），无矩阵、
无正文、低频（一个租户的法人以个计不以百计），不需要双人治理——正是决定八「结构简单、低频的册可以直接逐字段表单」那一类。
不走「模板导入 → 草稿 → 批准 → 发布」：那条路是为价卡这种上百格矩阵与审批职责规则设计的，套在五格上是把一次登记变成五步。

**JSON 快照签保留**为「高级：粘贴登记快照 JSON」折叠区（ADR-0101 决定一原句：JSON 快照签保留为受控批量口的在线镜像，
不是运营配置员的主路径）。

## 载荷形状

镜像受控 CLI `parcel-commercial register-parties` 输入里 `legalEntities` 数组的一项：

```json
{ "legalEntities": [ { "legalEntityId": "…", "partyId": "…", "revision": 1, "basis": "…", "effectiveFrom": "2026-01-02T00:00:00Z" } ] }
```

**不带 `tenantId`**：在线 Intake 要把认证结果填进租户格、只从载荷取行内容（`internal/partycommercial/adapters/http/register_party_identity.go`
包注释），带上就是在请求里自报租户。今天端点挂 `UnconfiguredIntake{}` 必答 403，这份形状没有服务端消费者；真 Intake 落地时以它为准
重谈（`api.ts` 那段注释的原话），本票不把它当已发布 Schema。

## 表单只做编码层的事（伞票 admin-write-faces/07 硬句：不算摘要、不裁任何门、不判领域规则）

- `revision` 编成 JSON 整数：填的不是正整数就在本地报「修订号要是正整数」——那是编不进类型，不是领域门。
- `effectiveFrom` 用 `datetime-local` 收操作者本地时刻，换成 RFC 3339 UTC 送出；留空缺席，让服务端点名。
- 其余各格原样送；空串照送，由服务端逐格答。
- **修订号建议值**：法人标识在已取回的列表里有则建议 `最新修订 + 1`，没有则建议 `1`，操作者可改。建议不是判定——服务端
  仍按册面判连续性；列表只有最新修订、且可能已过期，所以它只是省一次翻册。
- **参与方身份**用 `ReferencePicker` 从 `GET /commercial-business-parties` 读面取候选（显名称 + 标识 + 状态，不按状态过滤——
  表单不裁，届时是否已生效由服务端判）；读面在墙前退回手填。

## 三态照旧由 `RegistrationAnswerNote` 显

登记册答案（`已登记` / `已在册` / `未受理` + 散文原因）、403 未配置整段说明、调用方问题、未形成答案、未到达——与 JSON 签同一份呈现。
登记成功（`PARTY_IDENTITY_REGISTERED`）后触发列表重取，操作者切回「集团与法人」签看得到新行。

## 完成判据

- 三道门禁绿。
- `legal-entity-form.test.ts`：载荷组装（正常 / 缺可缺键 / 修订号非整数）、建议修订号（在册 / 不在册）、本地时刻 → RFC 3339。
- 本机演示形态下填表提交答 403 未配置整段说明（那是今天的诚实答案）；JSON 折叠区仍能粘快照提交。

## Comments
