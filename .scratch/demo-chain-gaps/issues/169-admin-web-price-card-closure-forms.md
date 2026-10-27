# 169 管理台没有价卡收尾的表单：截止与退役两口只能经受控登记口或直接调 API，价卡目录也不渲染收尾三格

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 票 [150](./150-price-card-has-no-controlled-cut-off-for-a-superseded-version.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权；150 的地盘不含 `apps/admin-web`）。是否认领、归哪一位 owner 待分诊。
Blocked by: 无
地盘（建议）：`apps/admin-web/**`（前端切片，一人在 `main` 上直接做，见 `docs/agents/workflow.md`「前端切片」）。
出处：[ADR-0234](../../../docs/adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md) 决定一、五与越权风险点 9。

## 现象

票 150 在 `parcel-api` 上开了 `POST /pricing-price-card-successor-cuts` 与 `POST /pricing-price-card-retirements`（操作者渠道的登记册配置写），受控登记口有 `parcel-pricing-register -kind price-card-cut|price-card-retirement`。管理台的价卡目录页（`PriceCardCatalogPage`）与草稿发布那一签都没有对应的动作：运营导入、批准发布了同一方案的新一版之后，结算那一侧的请求停在适用冲突，管理台上却没有地方把旧版收尾。`/pricing-price-cards` 的响应多了 `selectionEndsAt`、`retiredAt`、`successorVersion` 三个可缺席的键，页面也还不渲染。

## 要做的

- 价卡目录上对一版旧版给出「截止在后继起点」「整段让位退役」两个动作：选同一方案的后继版本、填依据，提交到对应端点；载荷只有 `planId`、`version`、`successorVersion`、`reference` 四格，截止点与退役时刻都由服务端定。
- 答复照端点回显：截止显示截止点与后继，退役显示后继与退役时刻；`未受理`显示原因。
- 目录行渲染收尾三格，发布时的适用期照旧显示。
- 两个动作只对同一方案的另一版可选；后继起点不晚于旧版的只给退役、更晚的只给截止——判据仍以服务端为准，前端只是不把注定被拒的选项摆出来。

## 三类

表单与动作是机制；哪一版何时让位给哪一版是租户取值。

## 规模

S
