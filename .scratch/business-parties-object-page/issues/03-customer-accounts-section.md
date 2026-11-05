# 03 客户账户分区：结构化新增表单（两处共用）、停用对话框

Category: enhancement
Status: resolved——2026-11-05 进 main（本笔提交）
父票：[spec](../spec.md)

## 要做的

见 spec「差距」与「决定」中对应条目。

## 完成判据

- 三道门退 0。
- 浏览器在演示租户核对应行为，截图核版式。

## 完成记录（2026-11-05，直接在 `main` 上做）

- 落点：新 `customer-account-form.ts`（纯函数：草稿 → 载荷 `customerAccounts[0]`、编码层本地问题、修订号建议；带 node:test）与 `CustomerAccountRegistrationForm.tsx`（账户编码、所属客户〔往来单位候选，可由 `preset` 锁定〕、来源单据、生效时间，修订号收在「高级」；整页与 `inline` 两种形态）。客户与合同页「新增客户账户」视图由粘 JSON 的 `RegistrationPanel` 换成这张表单，离开守卫一并认它的登记簿。`PartyOverviewSection.tsx` 的客户账户块抽成 `CustomerAccountsBlock`：有写授予时块头「新增客户账户」就地换表单（所属客户锁成本单位），已生效与待生效的账户一行「停用…」（对话框里是 `IdentityDeactivationForm` 锁成 `CUSTOMER_ACCOUNT` + 账户编码），有没保存的输入时「取消」先问；行上加停用时间。`Section` 标题条改成动作与「去某页」并排（原来二选一，客户账户块会把动作吃掉）。对象页与右栏都接上（`accountWrites`）。
- ✅ 三道门退 0（node:test 1206/1206）。
- ✅ 浏览器真写（5175 指演示 API）：「合成货主二号」对象页就地新增 `SYN-ACCOUNT-DEMO-03`（所属客户格锁死），答 `REGISTERED`，行立刻出现「已生效」；同一行「停用…」→ 确认框 → 答 `DEACTIVATED`，行变 r2「已停用」并显停用时间。未填生效 / 停用时间时服务端答「未受理」并原样显示（表单不代填时间，与新增往来单位同一做法）。客户与合同页 `?tab=register` 显结构化表单、无 JSON 文本框。查看者无「新增客户账户」与「停用…」。
- 演示库因此多了一个已停用的合成账户 `SYN-ACCOUNT-DEMO-03`（真实业务写入，留着）。
- 评审：diff 全在 `pages/**`，自审；整页非作者评审归票 05。证据 S。
