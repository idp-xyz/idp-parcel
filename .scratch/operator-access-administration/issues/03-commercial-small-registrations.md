# 03 商业三小类的 HTTP 写面与登记面

Category: enhancement
Status: ready-for-human——2026-11-09 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)「二」R3）：解析键、注册号类型、审批职责规则三类登记今天都走受控 CLI，实施人员能登，不挡上线；首个租户的运营要在管理台自己登这三类时解冻，与 [price-card-import/07](../../price-card-import/issues/07-parcel-pricing-context-gains-approval-duty-rule-and-operator-subject-terms.md) 同批；用户答「按你的建议全部收掉吧，你是业务和系统专家，你来决定」，会话分诊。此前 draft
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

解析键（`register-resolution-key`）、注册号类型（`register-registration-number-types`）、审批职责规则（`register-approval-duty-rule`）三个只有 CLI 的登记，各加登记册配置写面的端点与管理台登记面（审批职责规则放审批与发布页头；注册号类型放公司主体页；解析键放客户与合同页）。形状与答复沿用各自 CLI 的翻译。
