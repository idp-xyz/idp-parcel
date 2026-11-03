# 公司主体 UX 评估落地

Category: enhancement
Status: resolved——2026-11-03 子票 01–10 全部进 main
出处：用户 2026-11-02 要求以 UX 专家视角评估「公司主体」的 UX/UI 与内容表达，并裁定「按建议直接修改优化」。评估基于 main `97ca27b8b` 的预览（演示租户 5 家主体）；截图与页面文本在协调方会话的 scratchpad `ux-le/`。

## 评估结论（一句话）

领域严谨度与工作台形态都成熟；问题在「运营与财务的人看不看得懂、走不走得通」：一家公司拆在三处维护、领域内部语言漏到界面、跨境小包真正要印到单据上的主体信息没有引导。

## 不变的决定

- 领域不动：身份（责任法人）与法人资料分层、版本不可覆盖、按时点解析（ADR-0145、ADR-0350、ADR-0351）照旧。本轮只改管理台 `apps/admin-web/**`。
- 页名仍叫「公司主体」（`vocabulary/objects.ts` 的 `companyEntity`）。
- 写口不变：建档一体化是管理台把两次登记串起来，不加后端复合命令；停用仍调 `/commercial-party-identity-deactivations`。

## 统一用语（本轮唯一权威，各票照抄）

| 界面上 | 现在 | 改为 | 备注 |
|---|---|---|---|
| 资料的名字 | 法人资料 | **公司资料** | 按钮「更新公司资料」；「法人」只在解释责任法人概念时出现 |
| 身份登记的版本 | 身份修订 / 第 N 次更新 / 身份 r1 / 修订号 | **登记记录 · 第 N 版** | 签名「登记记录」；首笔不写「第 1 次更新」 |
| 资料的版本 | 资料修订 / 资料 r4 | **资料版本 · 第 N 版**（短写「第 4 版」） | 签名「资料版本」不变 |
| 表单里的版本格 | 更新序号 | **版本号** | 说明文字照旧「系统按已登记的记录自动算好，一般不用改」 |
| 依据 | 登记依据 / 资料依据 / 停用依据 / 身份更正依据 | **来源单据**（登记来源单据 / 资料来源单据 / 停用来源单据 / 更正来源单据） | |
| 对应的往来单位 | 对应的往来单位 | 首屏不显；技术详情里叫「往来单位档案」 | 公司名已在页头 |
| 注册号类型 | USCC / SYN-CN-REG 码 | 目录 `typeName`（如「统一社会信用代码」）；码进技术详情 | 目录 `GET /commercial-registration-number-types` 已有 `typeName` |
| 资料状态 | 开票资料可用 / 资料不全 / 未配置 / 主体尚未生效 / 主体已停用 | **可开票 / 资料不全 / 未登记资料 / 主体未生效 / 主体已停用** | |
| 解析结果句 | 已解析：这一刻有生效的资料修订，开票资料齐 | **可开票：当前有生效资料且开票资料齐全** | |
| | 资料不全（明确非成功，开立方据此拒开，不以默认值补齐） | **资料不全：缺开票抬头，开票会被拒** | 原因码另有一句 |
| | 这一刻没有生效的资料修订 | **这个日期没有生效的资料** | |
| | 生效的那一笔没有开票资料 | **生效的那一版没有开票抬头** | |
| 按时点查看 | 按时点查看法人资料 / 问的时刻 | **查看某个日期的资料 / 查询日期** | |
| 空值 | 未配置社交账号 / 用途未配置 / 部门未配置 | **—** | 只有「资料状态：未登记资料」这种结论句保留文字 |
| 地址 | CN · 合成市… | 去掉国家码前缀；国家经基础数据译名另起一格或括注 | |
| 时间 | 2026-01-02 08:00:00 UTC+8 | 主数据页生效 / 登记 / 停用显示到**日期**；完整时刻与时区放 `title` | 版本历史里的登记时刻保留到分钟 |
| 枚举码 | 责任法人 · LEGAL_ENTITY | 只显「公司主体」 | |
| 列表副标题 | 我方签约、开票与结算使用的法人；身份登记和开票资料分别留存修订 | **我们自己签约、开票、结算用的公司** | |
| 新增页副题 | 我们自己对外签约、开票的公司；先登记注册地与注册号，开票资料之后在详情里补 | **我们自己签约、开票用的公司；填注册地与注册号，开票资料和联系人保存后在详情里补** | |
| 使用说明 | 本页维护我方责任法人，并关联往来单位身份。客户、供应商是往来单位承担的业务角色，不是另一份法人身份。 | **这里登记的是我们自己开票、签约用的公司。客户、供应商在「往来单位」里登记。** | 其余段落同样改成运营的话 |
| 表单说明 | 法人是谁：稳定标识、所钉的业务参与方身份、注册国家 / 地区与终身注册号 | **这家公司是谁：编码、注册地、注册号** | |
| 注册号 label | 终身注册号 | **注册号**；说明「如统一社会信用代码、公司注册编号（CR No.）、EIN；一个公司可按类型登记多个」 | |
| 候选说明 | 候选是注册号类型目录里登过身份层类型的国家 / 地区 | **只能选已配置注册号类型的国家 / 地区；要加别的国家，先到「基础数据 › 注册号类型」配置** | |
| JSON 折叠 | 高级：粘贴登记快照 JSON（受控批量口 parcel-commercial register-parties 的在线镜像） | **高级：导入 JSON（批量迁移用）** | |
| 联系用途「通用联系」 | 通用联系 | **日常联系** | |

技术详情（`TechnicalDetails`）里的行名照旧用原词与原码，那是给技术人员对照的。

## 子票

| 票 | 内容 | 落点 | 阻塞 | 状态 |
|---|---|---|---|---|
| [01](./issues/01-wording-and-list-page.md) | 用语统一、列表页细节、签名、空值与时间精度 | `pages/party/{GroupLegalEntitiesPage,LegalEntityContactsTable,LegalEntityProfileContent,LegalEntityProfileReadout,LegalEntityProfileHistory,RevisionHistorySection}.tsx`、`presentation.ts`、`legal-entity-identity.ts`、`legal-entity-list.ts`、`legal-entity-revisions.ts`、`legal-entity-profile-history.ts`、`detail-primitives.tsx`、`navigation.ts`、`templates/workspace-tabs.ts` | — | resolved（2026-11-03 进 main） |
| [02](./issues/02-register-party-and-entity-together.md) | 建档一体化：新增公司主体时一并登记往来单位；往来单位列表标出「我方公司」 | `LegalEntityRegistrationForm.tsx`、`legal-entity-form.ts`、`BusinessPartiesPage.tsx`、`business-party-list.ts` | — | resolved（2026-11-03 进 main） |
| [03](./issues/03-object-page-deactivation-and-references.md) | 对象页与详情栏首屏重构、停用对话框留在本页、「被谁引用」全景 | `LegalEntityDetailPage.tsx`、`LegalEntityDetailPanel.tsx`、`LegalEntityHeaderParts.tsx`、`IdentityDeactivationForm.tsx`、新 `LegalEntityDeactivationDialog.tsx`、新 `LegalEntityReferencesSection.tsx` + `legal-entity-references.ts` | — | resolved（2026-11-03 进 main） |
| [04](./issues/04-profile-form.md) | 公司资料表单：默认基于当前版本、保存前差异预览、未保存保护、缺项提示、税务登记号类型从目录选、币种指引 | `LegalEntityProfileSection.tsx`、`LegalEntityProfileFields.tsx`、`legal-entity-profile.ts`、`CurrentLegalEntityProfile.tsx`、`LegalEntityProfileSummary.tsx` | — | resolved（2026-11-03 进 main） |
| [05](./issues/05-cross-border-profile-extensions.md) | 跨境主体资料扩展：监管登记号语言放宽、收款账户、多语言开票抬头（ADR-0359，协调方代决）——领域文档 + 后端 + 种子 | `docs/adr`、`docs/domain/party-commercial`、`internal/partycommercial`、`migrations/party_commercial`、`scripts/demo-seeds` | — | resolved（2026-11-03 进 main） |
| [06](./issues/06-cross-border-profile-admin-web.md) | 管理台跟上票 05：监管登记号、收款账户、多语言抬头 | `pages/party/LegalEntityProfile*`、`legal-entity-profile*.ts` | 04、05 | resolved（2026-11-03 进 main） |
| [07](./issues/07-polish-after-landing.md) | 收尾抛光：空的概要卡、「关联」签显名称、头部注册号胶囊截断 | `LegalEntityDetailPage.tsx`、`LegalEntityDetailPanel.tsx`、`LegalEntityReferencesSection.tsx`、`legal-entity-references.ts` | — | resolved（2026-11-03 进 main） |
| [08](./issues/08-visual-tidy-up.md) | 视觉收拾：详情栏与对象页一套版式、去重复、宽屏不散（用户 2026-11-03 反馈） | `LegalEntityDetailPage.tsx`、`LegalEntityDetailPanel.tsx`、`LegalEntityHeaderParts.tsx`、`LegalEntityProfileContent.tsx`、`LegalEntityProfileReadout.tsx`、`LegalEntityProfileSummary.tsx`、`GroupLegalEntitiesPage.tsx` | — | resolved（2026-11-03 进 main） |
| [09](./issues/09-card-groups-and-tab-counts.md) | 读出分组改成带图标标题的卡、签上带计数（照 WMS ASN 详情栏，用户 2026-11-03 裁定） | `LegalEntityProfileContent.tsx`、`LegalEntityProfileReadout.tsx`、`LegalEntityDetailPanel.tsx`、`LegalEntityDetailPage.tsx` | — | resolved（2026-11-03 进 main） |
| [10](./issues/10-workspace-tab-icons.md) | 对象工作区的签统一配小图标（全站；用户 2026-11-03 裁定） | `templates/workspace-tab-icons.tsx`（新）、`templates/DetailPageTemplate.tsx`、`pages/party/{LegalEntityDetailPanel,BusinessPartyDetailPanel,VersionDetailDrawer}.tsx` | — | resolved（2026-11-03 进 main） |

01–05 并行，各自只动表里的落点；落点之外的文件要改一行也先告诉协调方。进 main 顺序 01 → 02 → 03 → 04，每票进 main 前 `git merge origin/main` 再过三道门。

## 完成判据（父票）

- 01–04 resolved；三道门在 main 上退 0；CI `admin-web` job 绿。
- 浏览器在演示租户上核过：列表、详情栏四签、对象页四签、新增（含一并登记往来单位）、更新公司资料（预填当前版 + 差异预览）、停用对话框（不提交）。
- 05 走并行会话那条路（隔离评审、全量真库测试）进 main；06 在 04、05 之后做。

## 附带

- CI 上 `TestTheLargestLegalWorkbookIsReadWithinBoundedTimeAndMemory`（parcel-pricing 价卡模板读取）在本批几次只改文档的提交上红（带 `-race` 的共享执行器实测 15.6–17.8 s，上限 15 s）。耗时上限按该用例「防量级退化」的本意放宽到 45 s（分支 `claude-ci-workbook-bound`，非作者评审阻断为零，全量真库测试过），与本批一起进 main。
