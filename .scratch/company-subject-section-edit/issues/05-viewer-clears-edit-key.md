# 05 查看者落到编辑地址时清掉编辑键

Category: enhancement
Status: in-progress
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
