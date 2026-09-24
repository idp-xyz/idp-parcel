# 04 落地 ADR-0101 价卡导入：模板 → 草稿 → 批准 → 发布

Category: enhancement
Status: in-progress——跟踪容器：2026-09-25 通道 3 拆为 [`.scratch/price-card-import/`](../../price-card-import/spec.md) 下子票 01–06（用户授权自决），本票自身不再有可执行工作；子票全部 resolved 时本票随之 resolved。此前 draft
Blocked by: 无
出处：[spec](../spec.md) 缺口四。

## 为什么

价卡维护是租户定价人员的周常工作。今天管理台价卡页的「登记价卡」签是 `RegistrationPanel` 粘登记快照 JSON（`apps/admin-web/src/components/registration/RegistrationPanel.tsx`
的「粘贴登记快照 JSON」），这不是定价人员能用的形态。形态本身已经定了，不需要再裁：[ADR-0101](../../../docs/adr/0101-operator-facing-registration-payload-shape-is-product-defined.md)
决定二至六——产品发布版本化 Excel / CSV 导入模板；导入在 parcel-pricing 立持久化草稿；校验与发布共用一份摘要；批准是独立的操作者动作，发布交既有
`RegisterPriceCard`；审批职责规则是租户治理参数、缺省朝拦。

## 现状（取证钉 main `71e41e6d`）

- `internal/parcelpricing` 无草稿册、无导入 / 批准 / 发布用例；`migrations/` 无对应模块。
- `.scratch/` 无实施票（按「价卡草稿 / 草稿册 / PriceCardDraft」检索只命中计价参考序列那张不相干的 08）。
- 管理台价卡页仍是「价卡目录」「登记价卡」两签。

## 要拆的（照 ADR-0101 Consequences，不增不减）

1. 设计文档《价卡导入模板与校验规范》（parcel-pricing），模板版本号写在文档头部，与规范化版本号的对应关系写在那里；按 AGENTS「新增权威文档」在 `docs/README.md` 登入口。
2. 草稿册迁移模块 + 导入用例（解析模板 → 领域构造 → 摘要 → 存草稿，原始文件身份沿 `SourceFileIdentity`，文件本体按 ADR-0008 外置）。
3. 批准用例与发布用例（发布交既有 `RegisterPriceCard`，答案代数一格不改）+ 草稿查阅读口。
4. HTTP 端点挂 ADR-0100 的操作者 Intake，按 ADR-0085 两阶段进端点表。
5. 管理台：「登记价卡」签改「导入价卡」（上传 → 校验结果与摘要预览 → 存为草稿），新增「草稿」签（列表、批准、发布，录入者与批准者两列并排）；
   JSON 快照退为高级口。
6. 参数登记册增一行审批职责规则（实例半边，租户取值留空答`未配置`）。

## 形态

1–4 碰 Go / SQL / 领域设计文档，走并行会话那条路；5 在端点落地后另拆前端切片。拆票时按 issue-tracker 的「Draft and activate children」办。
