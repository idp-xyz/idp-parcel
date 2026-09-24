# 责任法人业务属性：ADR-0145 落地

Category: enhancement
Status: in-progress——2026-09-24 通道 3 按票 [admin-web-group-legal-entities/05](../admin-web-group-legal-entities/issues/05-legal-entity-business-attributes.md) 的产出要求拆票；子票全部 ready-for-agent
出处：票 05（用户 2026-09-24 授权通道 3 自决）→ [ADR-0145](../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md)；领域语言在
party-commercial [`CONTEXT.md`](../../docs/domain/party-commercial/CONTEXT.md) 的「责任法人」「法人资料」词条、Rules 与 Lifecycles「法人资料」一节。

规则只在 CONTEXT（领域语言）与 ADR-0145（取舍记录）两处；本 spec 与子票只引，不复述。

## 子票

| 票 | 标题 | 状态 |
|---|---|---|
| [01](./issues/01-registration-number-type-catalogue.md) | 注册号类型目录：按注册国家 / 地区登记注册号类型、格式与所属层 | resolved · 分支 `mcp4-lep01`（含清点 tip `f0216e34`）· 待评审与重放 |
| [02](./issues/02-legal-entity-identity-carries-registration.md) | 责任法人身份登记加注册国家 / 地区与终身注册号 | ready-for-agent · Blocked by 01 |
| [03](./issues/03-legal-entity-profile-revisions-and-as-of-resolution.md) | 法人资料修订链与按时点解析（含「资料不全」答复） | ready-for-agent · Blocked by 02 |
| [04](./issues/04-admin-web-identity-fields-and-profile-face.md) | 管理台：法人登记表单加身份两格、法人资料页 | ready-for-agent · Blocked by 02、03 |

走法：01–03 碰 Go / SQL，走[并行会话](../../docs/agents/parallel-sessions.md)那条路；04 只在 `apps/admin-web/**` 与票面，走
[workflow.md「前端切片」](../../docs/agents/workflow.md#前端切片一人在-main-上直接做)。

**不在本 spec**：settlement-accounting 开立对账单与发票时固定法人资料引用、见「资料不全」拒开——归 SA owner，随 SA 开立单据的实施票；
customs-compliance 申报主体资料取不取法人资料——归 CC owner。两件都只消费 03 的按时点解析。

## 红线

- 种子、夹具、文档只出 `SYN-` 值；注册号类型目录不带生产默认（ADR-0145 决定一、七）。
- 登记责任法人不要求法人资料（决定六）；币种不上法人（决定四）。
