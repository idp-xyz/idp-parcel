# 01 抽 usePushedAddress 钩子，七处登记视图 / 编辑态的「返回」换用

Category: bug
Status: in-progress
父票：[spec](../spec.md)

## 要做的

1. `templates/use-pushed-address.ts`：`push` / `back` / `retarget` / `forget`，听 `hashchange` 判压完之后地址有没有被别处动过。
2. 七张页删掉各自的 `useRef(false)` 标志，换钩子；往来单位登记视图内换册 `retarget`。

## 完成判据

- 三道门退 0。
- 浏览器核：进登记视图 → 切到别的工作区标签 → 切回来 → 「返回列表」落在本页目录，不落在别的标签；不切标签时仍用后退（历史不多一条）。
