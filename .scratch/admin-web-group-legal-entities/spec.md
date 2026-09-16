# 集团与法人页：从「诚实的骨架」到运营配置员能用的册页

Category: enhancement
Status: in-progress
出处：用户 2026-09-16 13:1x 经 IDP 队列通道 6 问「这个页面功能完整、UI/UX 专业吗，满足企业级小包业务的优秀产品吗」，
通道 6 给出评估后用户令「你是系统和国际小包业务超级专家，你自决」。本规格是那次评估的落笔，票按评估条目拆。

## 评估结论（钉在 `a608536d` 的 `apps/admin-web/src/pages/party/GroupLegalEntitiesPage.tsx`）

**做对的**：领域模型正确（身份登记按修订版本化只增不覆盖、停用是新修订、名称从参与方册转写、悬空引用如实标出）；
四态如实（加载/空/错误/未配置），计数带 outcome 守卫；列面收敛到端点真实字段。

**缺口**，分三档：

| 档 | 缺口 | 归属 |
|---|---|---|
| 页面机制（前端就能做，不碰契约） | 状态是纯文字没用 `StatusBadgeFor`；时间显原始 UTC ISO 带毫秒；「对象类型」整列同值（撤「运营集团租户」列用的正是这条理由）；无状态筛选、无排序；行点不进去，看不到 `deactivationBasis`、`tenantId`、`kind`；标识与依据不可复制；描述与空态文案是给工程师看的（限界上下文名、「行对象」、「转写」） | 票 01 |
| 登记面形态（ADR-0101 决定八已放行） | 「登记法人」签是粘 JSON——ADR-0101 决定一已把「不逐字段建」的理由收窄到客户渠道载荷，决定八让各册自裁；法人身份登记五格、低频、结构简单，是决定八点名「可以直接逐字段表单」的那一类。JSON 快照签保留为受控批量口的在线镜像，不再是主路径 | 票 02 |
| 契约与建模（要后端或 owner 裁） | 修订历史没有读口（行对象是最新修订，`r1` 点不进去）；目录读口一次拉全量（`isolatedReadLimit = 200`）、无分页排序筛选参数；责任法人没有业务属性（税号/注册国家/开票主体/结算币种/联系人）；`/commercial-*` 身份族在隔离形态下仍答 403（ADR-0091 逐口放行首批只有 `/shipment-requests`） | 票 03、04、05、06 |

## 范围

- **做**：票 01、02 由通道 6 本会话在隔离 worktree `D:/tops/idp-parcel-mcp6-adminweb`（分支 `mcp6-admin-web-legal-entities`，基 `a608536d`）上做，纯 `.ts/.tsx/.md`。
- **只出票不动手**：票 03（Go 读口 + 前端历史区）、04（契约决策）、05（CONTEXT 建模）、06（ADR-0091 放口）。
- **不做**：不改两签结构为「列表 + 主按钮 + 抽屉」——二十余张册页同用两签，一页独改只添不一致；若要换形态另立票全站一起换。不加导出。

## 红线

- 状态徽章只用 CONTEXT 原词（`已登记`/`已生效`/`已停用`，出处 party-commercial CONTEXT Lifecycles 下「参与方身份（业务参与方、责任法人、货主客户账户）」），不自造译法。
- 表单**不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：修订连续、参与方在册且届时已生效、时刻格式，一律送上去让服务端答。表单只做编码层的事（修订号编成整数、可缺键缺席、本地时刻换成 RFC 3339）。
- 表单**不带租户格**：在线 Intake 要把认证结果填进租户格、只从载荷取行内容（`register_party_identity.go` 包注释；ADR-0100 决定二后端不采信自报身份）。载荷镜像受控 CLI `register-parties` 的 `legalEntities` 一项，去掉整批的 `tenantId`。
- 过滤与排序只在已取回的数据上做，不下推成查询参数（README 列表页上列通则）；下推归票 04。
- 不为让页面变绿种任何行；页面在未配置态照旧如实呈现。

## 子票

| 票 | 标题 | 状态 |
|---|---|---|
| [01](./issues/01-list-face-polish.md) | 集团与法人读面打磨：徽章、时间本地化、撤同值列、筛选排序、行详情抽屉、复制、操作者文案 | resolved（通道 6 → 4 → 1，分支 tip 见票面完成记录） |
| [02](./issues/02-legal-entity-field-form.md) | 登记法人改逐字段表单（ADR-0101 决定八自裁），JSON 快照签降为受控批量口镜像 | resolved（同上） |
| [03](./issues/03-legal-entity-revision-history-read-face.md) | 责任法人修订历史读口 + 详情抽屉「修订历史」区 | ready-for-agent |
| [04](./issues/04-catalogue-read-pagination-sort-filter-contract.md) | 目录读口分页 / 排序 / 筛选下推的契约决策 | needs-info（归 owner） |
| [05](./issues/05-legal-entity-business-attributes.md) | 责任法人业务属性建模（税号、注册国家、开票主体、结算币种、联系人） | needs-info（归 owner） |
| [06](./issues/06-isolated-write-admission-for-commercial-identity-family.md) | ADR-0091 逐口放行：`/commercial-*` 身份族在隔离形态下放行 | ready-for-agent |
