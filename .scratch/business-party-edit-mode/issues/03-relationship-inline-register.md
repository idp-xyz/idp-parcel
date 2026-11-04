# 03 全景「合作关系」块就地登记新关系

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

1. `PartyRelationshipRegistrationForm` 收 `preset`（本方 = 这家单位）与 `renderFooter`，能嵌在全景区「合作关系」块里（去掉自带的 Card 外壳由调用方决定）。
2. 全景区「合作关系」块标题条加「登记合作关系」；点开块体换成表单，块底「取消 / 保存」；落册后 toast、回读出、页面重取关系册。一次只编一块。
3. 有改动时取消先问；换行 / 关栏的守卫照公司主体的登记簿做法。

## 完成判据

- 三道门退 0。
- 浏览器核：块内出现表单、本方已预填且锁死、保存前答复显在块里、取消有改动先问、查看者无按钮。
