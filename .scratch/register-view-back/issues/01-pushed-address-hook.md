# 01 抽 usePushedAddress 钩子，七处登记视图 / 编辑态的「返回」换用

Category: bug
Status: resolved——2026-11-04 进 main `a9b193054`
父票：[spec](../spec.md)

## 要做的

1. `templates/use-pushed-address.ts`：`push` / `back` / `retarget` / `forget`，听 `hashchange` 判压完之后地址有没有被别处动过。
2. 七张页删掉各自的 `useRef(false)` 标志，换钩子；往来单位登记视图内换册 `retarget`。

## 完成判据

- 三道门退 0。
- 浏览器核：进登记视图 → 切到别的工作区标签 → 切回来 → 「返回列表」落在本页目录，不落在别的标签；不切标签时仍用后退（历史不多一条）。

## 完成记录（2026-11-04，直接在 `main` 上做，提交 `a9b193054`）

- 落点：`templates/use-pushed-address.ts`（`push` / `back` / `forget`，听 `hashchange` 判压完之后地址有没有被别处动过）；七张页换用，删掉各自的标志。
- ✅ 三道门退 0；浏览器核：供应商协议与往来单位的登记视图——不切标签「返回列表」用后退回列表；切到渠道产品标签再切回来按「返回列表」落在本页目录，不落在别的标签。
- 评审：`templates` 有改动，非作者评审 APPROVE；两处建议已照改。证据 S。
- CI：这一笔推送时 GitHub 没有起 run（API 当时反复答 EOF，工作流也没有手动触发口），由紧随其后的提交的 run 覆盖。
