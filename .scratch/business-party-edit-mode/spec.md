# 往来单位对象页：动作留在本页，照公司主体的显示态 / 编辑态落

Category: enhancement
Status: in-progress
出处：用户 2026-11-04 问「我们下一步的工作呢」；会话盘点管理台的编辑容器后建议先把往来单位对象页做成与公司主体一致（同一对模板，用户会并排比较），再统一各商业册的「表单签 + 抽屉」。用户此前裁定「以后都是 Yes」，按建议开工。

现状：往来单位对象页只有「返回列表」与「更多 › 停用」，停用把人送回列表页的登记视图（`?register=identity-deactivation&deactivate=…`）；名称更正要去列表页「新增往来单位」填同一个编码再保存；登记合作关系也在列表页的登记视图里。对象页本身改不了任何东西。

切片：PN-02，主责 party-commercial，非关务专项。权威：CONTEXT「参与方身份」「参与方关系」（登记版本化不可覆盖、停用携带依据与时点、关系变化形成新版本）、ADR-0315（写授予只管呈现）。只改 `apps/admin-web/**`，走 workflow「前端切片」。

## 不变的决定

- 写口不变：停用仍是 `/commercial-party-identity-deactivations` 一个命令带种类；名称更正仍是同编码再登记一笔；关系仍是关系登记口。
- 列表页的登记视图照旧（新增往来单位、别处深链来的停用）；本规格只给对象页与详情栏加本地入口。
- 容器照公司主体：停用用对话框（锁死种类与对象）；名称更正是对象页编辑态（地址 `?edit=identity`，页脚动作条）；登记关系在全景区「合作关系」块就地。

## 子票

| 票 | 内容 | 状态 |
|---|---|---|
| [01](./issues/01-deactivation-dialog-on-page.md) | 停用留在本页：对象页与详情栏「更多 › 停用」开本页对话框 | resolved（2026-11-04 进 main） |
| [02](./issues/02-identity-edit-mode.md) | 名称更正 = 对象页编辑态（`?edit=identity`），页脚保存 / 取消 | resolved（2026-11-04 进 main） |
| [03](./issues/03-relationship-inline-register.md) | 全景「合作关系」块就地登记新关系，本方预填 | in-progress |
