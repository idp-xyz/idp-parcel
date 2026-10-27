# 149 管理台没有收尾旧版的表单：截止与同起点退役两口只能经受控 CLI 或直接调 API

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 票 [50](./50-commercial-versions-have-no-closing-entry-so-agreement-versions-pile-up.md) 的作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权；50 的地盘不含 `apps/admin-web`）。是否认领、归哪一位 owner 待分诊。
Blocked by: 无
地盘（建议）：`apps/admin-web/**`（前端切片，一人在 `main` 上直接做，见 `docs/agents/workflow.md`「前端切片」）。
出处：[ADR-0233](../../../docs/adr/0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md) 决定一。

## 现象

票 50 在 `parcel-api` 上开了 `POST /commercial-version-successor-cuts` 与 `POST /commercial-version-retirements`（操作者渠道的登记册配置写），受控 CLI 有 `register-version-closures`。管理台的商业发布页没有对应的表单：运营在管理台上发了新版之后，看得到同范围解析答`适用冲突`，却没有地方把旧版收尾，要回去跑 CLI 或手写请求。

## 要做的

- 商业版本列表（或发布页）上对一版旧版给出「按后继截止」「同起点退役」两个动作：选同一对象的后继版本、填依据引用（截止另填更正时刻），提交到对应端点。
- 答复照端点回显：截止显示旧版截止后的选用区间，退役显示后继与退役时刻；`未受理`显示原因。
- 两个动作只对同一对象的另一版可选，起点相同的只给退役、后继更晚的只给截止——判据仍以服务端为准，前端只是不把注定被拒的选项摆出来。

## 目录查阅折入收尾的余项（票 50 评审 3 记下）

票 50 只把收尾折进了一部分管理台目录（`internal/partycommercial/adapters/postgres/operations_catalogue.go`）：服务产品、客户合同、授权规则、供应商协议四张的状态格认退役；供应商协议另回显 `selectionEndsAt`、`successorVersion`、`retiredAt`。余下：

- 管理台页面还不渲染供应商协议那三格（`apps/admin-web` 的供应商协议页）。
- 服务产品、客户合同、授权规则三张不回显截止点。连同接受前财务控制策略、客户服务规则，另立票 [184](./184-five-commercial-catalogues-do-not-echo-the-cut-of-a-closed-version.md)。
- 按正文册驱动的目录（接单规则包、价格政策、结算政策、接受前财务控制）既不认退役也不回显截止点。

## 三类

表单与动作是机制；哪一版何时收尾是租户取值。

## 规模

S
