# 02 管理台：商业版本对象页与右栏入口

Category: enhancement
Status: resolved——2026-11-04 进 main（本笔提交，紧随 `1dabc6f45`）
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

照 spec 决定 3：地址判读（各页 address 模块加对象地址）、`readCommercialVersion` 客户端、共用的 `CommercialVersionObjectPage`（按类别套各册的列与技术详情）、四页在对象地址时渲染对象页、右栏头部「打开对象页」。

## 完成判据

- 三道门退 0。
- 浏览器核：右栏「打开对象页」落到对象地址、页面显示这一版；深链直落；不在册的身份显示未找到；返回目录带检索词。

## 完成记录（2026-11-04，直接在 `main` 上做）

- 落点：`version-object-address.ts`（对象地址 `#/<页>/<objectId>/<version>` 的判读与写回，带测试）；`commercial-version-api.ts`（`readCommercialVersion`，答复与目录行同形）；`VersionObjectPage.tsx`（`DetailPageTemplate` 上的对象页：壳、这一版的各格按目录列竖排、版本历史、技术详情摊开；动作由页面摆，「在目录中查看」带检索词回目录）；`VersionDetailPanel` 头部多一个「打开对象页」；服务产品、客户与合同、供应商协议、商业规则四页在对象地址时渲染各自的对象页，内容按身份读回（新鲜），历史与可收尾判断从目录来，目录翻不到这一版时把读回的那一版补进历史池。商业规则页的对象地址带 `?register=<册>`，`policyObjectKindOf` 把册名换成对象类别（PRICE_POLICY → PRICE_RULE），两本声明册没有对象页、右栏不摆按钮。客户与合同页的列组装抽成 `useContractColumns`，目录与对象页同一份。
- 「发新版本」不在对象页：预填状态住在目录页（`useReplaceDraftGuard` 与 `NewVersionRequest`），对象页只有收尾与回目录；要发新版本从目录右栏发。
- ✅ 三道门退 0：tsc、node:test 1201/1201、vite build。
- ✅ 浏览器核（playwright 只读探针，零写请求、零页面错误；用 land 构建另起的 19081 实例 + vite 5175，读口是真的）：四本册从右栏「打开对象页」各落到对象地址，页头是册名、标识与版本、状态；「这一版」各格、版本历史、技术详情三区在；「在目录中查看」回到目录并筛到那一条；工作区标签显「<页名> <objectId>」。接受前财务控制册的行右栏没有「打开对象页」。深链 `#/service-products/NOPE/v9` 显「没有这一版」；深链在册的版本直落。查看者只有「在目录中查看」。演示数据各对象只有一版，历史里换版未核。
- 评审：diff 全在 `pages/**`，按 workflow「前端切片」第 5 条自审。证据 S。
