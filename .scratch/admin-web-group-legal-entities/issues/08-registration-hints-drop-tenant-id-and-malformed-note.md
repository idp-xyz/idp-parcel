# 08 登记签提示句去「外加整批的 tenantId」+ party `problemNote` 的 `MALFORMED_REQUEST` 措辞改成读口 / 写口都成立

Category: bug
Status: resolved
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

## 完成记录（2026-09-16，分支 `mcp5-adminweb08`，基 main `cadd2d46`）

作者：代码笔 `f6f1597c`（要做的 1–2，Status → in-progress 随笔）为通道 5 前任会话所作，20:58 推 origin 后会话断；本会话（通道 1 21:0x 接管广播后接续，`task-934ef20e`）在同一隔离树 `D:/tops/idp-parcel-mcp5-adminweb08` 上复跑三道门与演示、写本记录。代码未再动。纯 `.ts` 文案，无红可写（/tdd 不适用）；`presentation.test.ts` 不存在，同目录只有 `policy-rows.test.ts` 钉着 `registrationSnapshotHints.publication`（本票未动），身份族五句与 `MALFORMED_REQUEST` 一格没有测试钉着。

对完成判据：

- 三道门（隔离树，代码 tip `f6f1597c`，本会话 21:1x 复跑）：`node node_modules/typescript/bin/tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 240 pass / 0 fail / 0 skipped；`pnpm exec vite build` 退 0（chunk > 500 kB 的警告为既有）。前任 20:5x 同样三绿，随代码笔记录。
- `git grep -c '外加整批的 tenantId' -- apps/admin-web/src/pages/party/presentation.ts` = 2，即 `service-product-form` 与 `product-channel-mapping` 两处（git 自己匹配，不经 PowerShell 解码）。
- 本机演示（parcel-api `127.0.0.1:8090`，隔离写开关 `SYN-TENANT-01`，07 之后的构建）：`POST /commercial-business-party-registrations` 不带 tenantId 的 `businessParties` 单项 → **201** `{"outcome":"REGISTERED"}`（本会话 `SYN-PARTY-AWGLE08-20260916-211817`，revision 1；前任 `SYN-PARTY-AWGLE08-20260916-205730`；两条都在合成租户，可留）；同一项加 `"tenantId":"SYN-TENANT-01"` → **400** `{"error":{"code":"MALFORMED_REQUEST"}}`。两次都是 HTTP 直投，不经页面粘贴；页面那句提示与 `problemNote` 文案由 tsc / 测试守，**浏览器未起、未验**。
- 清点预报零差：在干净树 `f6f1597c` 上 `tools/mechanism-inventory -out` 到临时路径，与已提交 `docs/product/MECHANISM-INVENTORY.md` `git diff --no-index` 退 0（不增删文件）。

要做的逐条：

① 五句（business-party / legal-entity / customer-account / party-relationship / identity-deactivation）拿掉「外加整批的 tenantId」半句，只剩各自键名与规则；否定句 `tenantGridFilledByChannel`（「载荷里**不带 tenantId**——在线口的租户格由接入渠道填入，带了（含 null）即 400」）只定义一处，经 `snapshotHint` 的 `options` 拼进前半。
② `problemCodeNotes.MALFORMED_REQUEST` 改为一句覆盖两侧：目录读口（kind 缺席或不在封闭集）与登记口（载荷带 tenantId 含 null、多出未知键、不恰一项、修订号非整数、时刻非 RFC 3339、封闭集词不在集内、标识或依据为空、尾随第二个 JSON 值）。成因清单对照 `isolated_write_intake.go` 里包 `ErrMalformedRequest` 的那几道门，比票面措辞多列了封闭集词、空标识、空依据三种——它们同样以 `MALFORMED_REQUEST` 答回，不列则那三种被拒的人读到的清单仍缺自己那条。
③ 产品渠道族两句**不动**：本会话对 `/commercial-service-product-form-registrations`、`/commercial-product-channel-mapping-registrations`（连同 `/commercial-publications`）各投一次 `{}`，三口都答 **403** `ACCESS_CHANNEL_NOT_CONFIGURED`——墙没降，改了验不了；三句照批文形状留着，等各口放行时随其票改。

判断项：

1. 否定句不是无条件拼进 `snapshotHint` 前半，而是经 `options?.tenantGridFilledByChannel` 只拼给身份族五签。票面「一处定义、不在五句各抄」守住了；但发布签（`publication`）与产品渠道族两签今天仍 403，其提示句照批文形状写着「一项的键为 tenantId / …」「外加整批的 tenantId 与 scope」——若无条件拼入，同一段里会同时出现「不带 tenantId」与「外加整批的 tenantId」。一句验不了的否定与一句验不了的肯定同样不可信，所以只给验得到的那五签。
2. `MALFORMED_REQUEST` 成因清单比票面多三种，理由见 ②；服务端带 detail 归票 11，落地后这一句只需收短。
3. 头注引 `register_party_identity.go` 包注释、ADR-0100 决定二与本票编号，不引行号、不计数（AGENTS「改文档」）。

## Comments
