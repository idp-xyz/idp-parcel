# 03 全景「合作关系」块就地登记新关系

Category: enhancement
Status: resolved——2026-11-04 进 main `b62cc67b3` + `b2ccc39de`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

1. `PartyRelationshipRegistrationForm` 收 `preset`（本方 = 这家单位）与 `renderFooter`，能嵌在全景区「合作关系」块里（去掉自带的 Card 外壳由调用方决定）。
2. 全景区「合作关系」块标题条加「登记合作关系」；点开块体换成表单，块底「取消 / 保存」；落册后 toast、回读出、页面重取关系册。一次只编一块。
3. 有改动时取消先问；换行 / 关栏的守卫照公司主体的登记簿做法。

## 完成判据

- 三道门退 0。
- 浏览器核：块内出现表单、本方已预填且锁死、保存前答复显在块里、取消有改动先问、查看者无按钮。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `c5bf8c4c5`，提交 `b62cc67b3` 与修正 `b2ccc39de`，CI run 41780069386 全绿）

- 落点：`PartyRelationshipRegistrationForm.tsx` 收 `preset`（本方预填并锁死）与 `inline`（无卡片外壳与 JSON 导入，动作行多「取消」），整页形态逐字节不变；`PartyOverviewSection.tsx` 的块标题条留 `action` 位、`relationshipRegistration` 为真时块体换成表单；`BusinessPartyDetailPage.tsx` 与 `BusinessPartyDetailPanel.tsx` 各持有登记态（与编辑态互斥、钉住单位），落册后 toast 并重取关系册；`BusinessPartiesPage.tsx` 换行 / 关栏时按登记簿（`registration:party-relationship`）守住草稿。
- 票面第 1 项写的 `renderFooter` 没用上：嵌入形态的动作行就摆在块底，不需要 portal。
- ✅ 三道门退 0：tsc、node:test 1195/1195、vite build；CI run 41780069386 全部 job success。
- ✅ 浏览器在演示租户核（playwright 只读探针，零写请求、零页面错误）：对象页全景「合作关系」块有「登记合作关系」，点开块体换成表单、按钮收起，「哪一方」已选本单位且禁用，「取消 / 保存」在块底；未改动取消直接收起；填关系编号后取消弹「放弃修改？」；点「更正名称」时关系表单收起、编辑态打开；详情栏全景签同样可用；详情栏里有改动换行弹「放弃修改？」，「继续编辑」留在原行且输入还在，「放弃」后换行、表单收起。查看者账号无「登记合作关系」。
- 修正一笔（`b2ccc39de`）：换行守卫的确认框第一次插进了关系册工作台那条支路，守卫触发了却没框可显，探针抓到后移到参与方工作台支路。
- 评审：diff 全在 `pages/**`，按 workflow「前端切片」第 5 条自审。证据 S。
