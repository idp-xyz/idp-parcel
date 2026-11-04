# 05 查看者落到编辑地址时清掉编辑键

Category: enhancement
Status: resolved——2026-11-04 进 main `7e511744a`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 03

## 出处

票 03 评审 Nit 6，当时因写授予钩子在加载期返回 false 而不做；用户 2026-11-04 裁定「先做 2，3」。

## 要做的

1. `use-operator-grants.ts` 加 `useRegistryWriteGrant()`：答出与否（`settled`）与有没有写授予分开说；`useMayWriteRegistry` 语义不变。
2. 公司主体与往来单位两张对象页：授予已答出且没有写授予、地址却带编辑键时，退出编辑态（替换地址，不压历史）。加载期不动。

## 完成判据

- 三道门退 0。
- 浏览器核：查看者账号打开 `?edit=profile` / `?edit=identity` 深链，地址被清成对象地址；配置操作者刷新编辑态深链仍在编辑态。

## 完成记录（2026-11-04，提交 `7e511744a`，CI run 41783394552 全绿）

- 落点：`pages/use-operator-grants.ts` 加 `useRegistryWriteGrant()`（`settled` / `mayWrite`），`useMayWriteRegistry` 改为由它派生；`LegalEntityDetailPage.tsx`、`BusinessPartyDetailPage.tsx` 在授予答出且无写授予、地址带编辑键时退出编辑态（替换地址）。
- ✅ 三道门退 0；CI 全绿。
- ✅ 浏览器核：查看者打开 `…?edit=profile` 与 `…?edit=identity`，地址分别被清成 `#/group-legal-entities/SYN-LE-CN-SZ-01`、`#/business-parties/SYN-PARTY-CN-SZ-01`；配置操作者的两种编辑深链仍在编辑态。
- 评审：自审。证据 S。
