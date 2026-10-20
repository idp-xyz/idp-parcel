# 16 管理台不认「授权交付方」这一参与方角色：关系列表显示原码，登记表单的角色下拉里没有它

Category: bug
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 的作者子代理认领，照前端切片在 `main` 上直接做。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立并代决
Blocked by: 无
父票：[10](./10-authorized-delivery-relationship-in-party-commercial.md)
出处：票 10 完成记录「没做」。
地盘：
- apps/admin-web/src/pages/party/**（`presentation` 里的 `partyRoleLabels` 与依赖它的列表、表单、测试）

前端票：照 docs/agents/workflow.md「前端切片」在 main 上直接做。

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「yes」。

## 现象（读码）

票 10 在 PC 参与方关系的封闭角色集里加了 `AUTHORIZED_DELIVERING_PARTY`（授权交付方）。管理台的 `partyRoleLabels` 没有这个键：关系列表里这一角色显示原码，角色筛选与登记表单的下拉里没有它，配置员在页面上登不了授权交付关系。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决）

- `partyRoleLabels` 加这一键，中文显示名取「授权交付方」（与 PC CONTEXT、ADR-0212 的称呼一致）。
- 登记表单选这一角色时，适用范围要按票 10 定的唯一拼法填写：`customerAccount=<客户账户>`，可选再加 `&node=<节点>`（键序固定、值转义）。表单为这一角色给出「客户账户」「节点（可选）」两个输入，由前端拼成那一种拼法，不让配置员手写查询串；拼法放纯函数模块配 node:test。
- 其余角色的表单与列表行为不变。

## 完成判据

- [ ] 三道门（Node 22）：`tsc -b --noEmit`、`node scripts/run-tests.mjs`、`vite build` 退 0。
- [ ] node:test：显示名、筛选项与表单下拉含新角色；范围拼法纯函数覆盖只给账户、账户加节点、需转义的值、空账户被拒四格，拼出的串与票 10 后端 `TestAuthorizedDeliveryScopeHasOneSpelling` 认的那一种逐字一致。
- [ ] 票面完成记录里判据逐条标 ✅ / ◑ / ❌。
