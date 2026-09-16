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

## 第二轮：业务参与方页评估（钉 main `dbe989cc`，通道 1，2026-09-16 20:2x）

出处：06 / 07 落 main 并重建 parcel-api 后，用户经 IDP 队列问「集团与法人不能手动添加吗 → 如何登参与方身份 → 能修改的吗 → 请你评估这个页面 →
按你的建议开始吧」。评估对象换成同模块的「业务参与方」页（`apps/admin-web/src/pages/party/BusinessPartiesPage.tsx`），判据来自代码 + 对本机
parcel-api 的实探（GET 两读口 200、五个 POST 空载荷 400），浏览器未验。

**做对的**：三签分法（身份本体册 / 关系册 / 登记签）有道理——两册状态代数不同，分签不并表；停用摆本页的理由成立；领域模型忠实（最新登记修订、
停用两件与状态同格、关系撤销 / 到期 / 替代带时点依据后继、悬空引用如实标出、「方向」列撤下由次序表达）；四态如实、计数只在业务答案后显示；
换册即清草稿；答案三态不折成「提交失败」。

**缺口**，四档：

| 档 | 缺口 | 归属 |
|---|---|---|
| 今天最伤（06 落地后才暴露） | 登记签五句提示仍写「外加整批的 tenantId」，而在线隔离口 `refuseSelfReportedTenant` 键在场即拒；400 到页面只剩 code，`problemNote('MALFORMED_REQUEST')` 显的是读口 `?kind=` 的说明——照提示填 → 被拒 → 读到的原因与真相无关 | 票 08（文案）、票 11（根治） |
| 页面机制（与票 01 同类，01 只修了法人页） | 状态纯文字无徽章；时间无悬停原串；无筛选排序；行点不进去、无复制；描述是工程师口吻、空态提 CLI | 票 09 |
| 登记面形态（ADR-0101 决定八） | 三册全是粘 JSON；参与方身份五格与法人同一判据该逐字段；停用有封闭三词与从册上选的目标；关系十格里有封闭五词与两个引用，打错词只得到说不清的 400；JSON 签不给修订号建议 | 票 10 |
| 契约与建模 | 参与方身份无修订历史读口（法人有 03）；写口 400 无 detail；目录读口无分页排序筛选（票 04）；参与方无业务属性（票 05 同族） | 票 12、票 11；04 / 05 照旧归 owner |

阻塞边：10 与 12（前端半边）都落在 09 改过的 `BusinessPartiesPage.tsx` 上，先后做；08 / 09 / 11 互不阻塞、地盘不交。

## 范围

- **做**：票 01、02 由通道 6 本会话在隔离 worktree `D:/tops/idp-parcel-mcp6-adminweb`（分支 `mcp6-admin-web-legal-entities`，基 `a608536d`）上做，纯 `.ts/.tsx/.md`。
- **第二轮（08–12）**：由通道 1 点名后派给应答的通道，各在自己的隔离 worktree 上做；08 / 09 / 11 可并行，10 与 12 等 09 进 main。**五票已全部进 main**（2026-09-16 23:35，远端 main `29e5117b`）；各票评审的判断项归收口票 13。
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
| [03](./issues/03-legal-entity-revision-history-read-face.md) | 责任法人修订历史读口 + 详情抽屉「修订历史」区 | resolved（通道 5 → 1 封存 → 1 重放，`ef7086f0` 进 main，见票面完成记录） |
| [04](./issues/04-catalogue-read-pagination-sort-filter-contract.md) | 目录读口分页 / 排序 / 筛选下推的契约决策 | needs-info（归 owner） |
| [05](./issues/05-legal-entity-business-attributes.md) | 责任法人业务属性建模（税号、注册国家、开票主体、结算币种、联系人） | needs-info（归 owner） |
| [06](./issues/06-isolated-write-admission-for-commercial-identity-family.md) | ADR-0091 逐口放行：`/commercial-*` 身份族在隔离形态下放行 | resolved（通道 4 → 4 新会话收尾，rebase 后以原 SHA ff 进 main，清点 `f6569f51`；见票面完成记录） |
| [07](./issues/07-isolated-write-intake-decode-strict-and-comment-counts.md) | A 类尾巴：隔离身份 Intake 外壳解码改调 `decodeStrict`（尾随内容拒）+ 注释去计数（实做四处；06 评审 N1 / N3，可选 N2 未做） | resolved（通道 4，三笔原 SHA ff 进 main，远端 main = `19047d51`，评审 ← 通道 2 无阻断；见票面完成记录） |
| [08](./issues/08-registration-hints-drop-tenant-id-and-malformed-note.md) | 登记签提示句去「外加整批的 tenantId」+ `problemNote` 的 `MALFORMED_REQUEST` 措辞改成读口 / 写口都成立（纯 .ts 文案） | resolved（通道 5 → 5 新会话收尾；与 09 同批重放进 main，远端 main = `1f569998`，评审 ← 通道 6 无阻断；见票面完成记录） |
| [09](./issues/09-business-parties-read-face-polish.md) | 业务参与方页读面打磨：徽章、时间悬停原串、筛选排序、行详情抽屉、复制、操作者文案（与票 01 同形） | resolved（通道 6；三笔重放进 main，远端 main = `1f569998`，评审 ← 通道 2 无阻断；`pages/party` 三处收口项记在票面处置；见票面完成记录） |
| [10](./issues/10-business-party-relationship-deactivation-field-forms.md) | 参与方身份 / 关系 / 身份停用三册逐字段表单（ADR-0101 决定八），JSON 签降为折叠区 | resolved（通道 6；八笔重放进 main，远端 main = `b1efbfdf`，评审 ← 通道 5 无阻断——七条判断项并入 `pages/party` 收口票清单，见票面处置） |
| [11](./issues/11-write-refusal-carries-detail.md) | 写口 400 带 `detail`：Go `writeProblem` 加格、TS `callerProblem` 带 detail、`RegistrationAnswerNote` 显出 | resolved（通道 4 → 4 新会话收尾；四笔重放 + 推送方 N1 文案笔进 main，远端 main = `958e6c57`，评审 ← 通道 2 无阻断；**与 ADR-0140 草案 Decision 三相反，归 owner**，见票面判断项 1） |
| [12](./issues/12-business-party-revision-history-read-face.md) | 业务参与方修订历史读口 + 抽屉「修订历史」区（按票 03 形态） | resolved（通道 4；(a)(b) 七笔进 main `44c4ebe2`、(c) 两笔进 main `29e5117b`，两段评审 ← 通道 2 皆无阻断；见票面） |
| [13](./issues/13-pages-party-consolidation.md) | `pages/party` 收口：09 / 10 / 12 评审判断项里的同形副本、重复 switch 与陈旧读面归一（九条，可分人） | ready-for-agent |
