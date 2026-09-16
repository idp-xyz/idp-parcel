# 08 登记签提示句去「外加整批的 tenantId」+ party `problemNote` 的 `MALFORMED_REQUEST` 措辞改成读口 / 写口都成立

Category: bug
Status: in-progress
Blocked by: 无
地盘：`apps/admin-web/src/pages/party/presentation.ts`（`snapshotHint` 前半、`registrationSnapshotHints` 身份族五句、`problemCodeNotes.MALFORMED_REQUEST` 一格）。不动 `catalogue-api.ts`、不动 Go、不动逐字段表单。
出处：通道 1 评估「业务参与方」页（钉 main `dbe989cc`，2026-09-16 20:2x），spec「第二轮」表第一档。

## 为什么

票 06 之前身份族五口挂 `UnconfiguredIntake{}` 必答 403，登记签提示句怎么写都到不了 400，这条因此一直看不见。06 落地后在线隔离口
`refuseSelfReportedTenant`（`isolated_write_intake.go`）**键在场即拒、含 `null`**，而 `registrationSnapshotHints` 的 business-party /
legal-entity / customer-account / party-relationship / identity-deactivation 五句都写「外加整批的 tenantId」——那是受控 CLI 批文的形状。
`snapshotHint` 前半已经说了「在线口收的是其中一项，不是整批」，tenantId 那半句没跟着改。照提示填 → 400。

更糟的是 400 到页面只剩 code：服务端 `writeProblem` 只写 `{"error":{"code":"MALFORMED_REQUEST"}}`，`catalogue-api.ts` 的
`callerProblem` 只带 `{status, code}`，页面于是显 `problemNote('MALFORMED_REQUEST')` = 「请求构造不出查询(kind 缺席或不在封闭集)」——
那是目录读口 `?kind=` 的说明，对写口全错。操作者被拒后读到的原因与真相无关，无法自救。

根治（写口拒绝带 detail）归票 11；本票只修文案，让今天的措辞不说假话。

## 要做的

1. **tenantId 那半句从五句里拿掉**，改为一句共用的否定放进 `snapshotHint` 前半（一处，不在五句各抄一遍）：
   「载荷里**不带 tenantId**——在线口的租户格由接入渠道填入，带了（含 `null`）即 400」。五句只剩各自的键名与规则。
2. **`problemCodeNotes.MALFORMED_REQUEST` 改成读口 / 写口都成立的一句**：`problemNote` 的签名只有 code，今天分不出哪一侧，
   所以一句覆盖两侧——「请求形状不合。目录读口：kind 缺席或不在封闭集；登记口：载荷带 tenantId、多出未知键、不恰一项、修订号 /
   时刻格式不对或尾随第二个 JSON 值。重发同样内容不会改变结果」。
3. **产品渠道族两句（`service-product-form` / `product-channel-mapping`）不动**：按 `cmd/parcel-api` 启动日志 `admittedCommandLines`
   名单，那两口今天仍 403（墙没降），改了也验不了；票面写明留着的理由，等那两口放行时随其票改。

## 不做

- 不动 `catalogue-api.ts`、不动 Go（detail 归票 11）。
- 不建逐字段表单（票 10）。
- 不动票 02 的法人表单——它本来不带租户格。

## 完成判据

- `node node_modules/typescript/bin/tsc -b --noEmit`、`node scripts/run-tests.mjs`、`pnpm exec vite build` 三道绿。
- `git grep -n '外加整批的 tenantId' -- apps/admin-web/src/pages/party/presentation.ts` 只剩产品渠道族两处（用 git 自己匹配，不经 PowerShell 解码）。
- 本机演示形态（`IDP_PARCEL_ISOLATED_WRITE_TENANT=SYN-TENANT-01`，parcel-api 已是 06 之后的构建）：登记签「参与方身份」粘不带 tenantId 的单项
  → 201 `REGISTERED`（真登一条，标识用 `SYN-PARTY-…`，合成租户里可留、不必清）；粘带 tenantId 的同一项 → 400，页面那句说得出「tenantId」。

## Comments
