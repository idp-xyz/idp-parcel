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

**进 main 记录**（推送方 = 通道 1 新会话，22:0x 接手；前任 21:3x 派完三份评审、在 `%TEMP%\idp-land0911` 把 08 + 11 + 09 叠好之后会话断，那棵树未推、已弃用）：与 09 同批重放——隔离树 `%TEMP%\idp-land0809` detached 于 `e5ff0f20`（= 当时远端 main），cherry-pick 本票两笔零冲突 `f6f1597c→db8a3ee4` / `62a1aea5→86edd4da`（patch-id 逐笔同；本票两件对作者 tip `62a1aea5` 零 diff）；tip `1f569998` 上 `gofmt -l` 空、`go build ./...` / `go vet ./...` 0、清点重生成 porcelain 空（零差，无单独清点笔）；admin-web 全新 `pnpm install --frozen-lockfile` 后 `tsc -b --noEmit` 0 / `run-tests` 252 pass / `vite build` 0；带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（22:12:36→22:14:51，135 s；本票不动 Go，这一跑验的是 tip 整体）。评审 ← 通道 6 21:38 无阻断（下）→ `ls-remote` 核 `e5ff0f20` 未动 → 22:15 `push 1f569998:main` 成，**远端 main = `1f569998`**；共享树 ff 同 SHA。非阻断代落：N1 → 纯注释笔 `b185c0f2`（`tenantGridFilledByChannel` 头注「两口」→「那几口」，自审）；S2 随 N1 消解（判断项 3 那句自 `b185c0f2` 起属实）；N2 / S1 只记。

## Comments

**评审 ← 通道 6 · 钉代码 tip `f6f1597c`（票面 tip `62a1aea5`）· 基线 `cadd2d46` · 21:38**（`task-4cda8c0d`；隔离检出 `%TEMP%\idp-review-08` detached `62a1aea5`，只读、未改分支；两轴由该会话串行跑，Spec 轴指票面 `62a1aea5` 版；diff 只触 `presentation.ts` +39/−13 与票面 .md；三道门与演示按评审纪律未复跑，以票面记录为据。）

- **Standards（阻断 0 / 非阻断 2）**：阻断无。**N1** `tenantGridFilledByChannel` 头注「发布口与产品渠道族两口今天仍挂 UnconfiguredIntake 答 403」——「两口」数的是 `cmd/parcel-api` 放行名单里别处的东西，AGENTS「写代码注释 → 引另一个文件照改文档办 → 不计数」；那一侧多放一口这句就静默变错。改「产品渠道族那几口」即可。**N2（判断项）** `problemCodeNotes.MALFORMED_REQUEST` 上方新头注用 ASCII 标点，同文件相邻注释与本笔另一段头注用全角；无成文标准、工具不管，仅一致性。核过无发现：注释全中文、只写理由不叙述；引用用符号名（`refuseSelfReportedTenant`、`register_party_identity.go` 包注释、ADR-0100 决定二、ADR-0101、票 08/11）无行号；字符串内 ASCII 标点与 `**…**` 强调沿本文件既有写法。Smell 基线无命中：`options?: { tenantGridFilledByChannel: true }` 单键字面量 true 的 options 在五个调用点自述其意，非 Speculative Generality；否定句一处常量、无重复。
- **Spec（阻断 0 / 非阻断 2）**：阻断无。**S1** 完成判据第 3 条「页面那句说得出「tenantId」」——作者如实记浏览器未起、未验，只以 HTTP 直投证 201/400、以 tsc 守文案含「tenantId」；与票 03 先例同形，随票记不挡合入。**S2** 判断项 3 声称头注「不计数」，而 `tenantGridFilledByChannel` 头注有「两口」（见 Standards N1）；票面这半句要改如实，或注释去计数——两者取一。逐条核对：要做的 1 ✓（五句各去「外加整批的 tenantId」半句，只剩键名与规则；否定句一处定义、经 `snapshotHint` 的 options 拼进前半、五签各传 `{ tenantGridFilledByChannel: true }`；文案与票面逐字对；`f6f1597c` 上 grep 只剩 `service-product-form` / `product-channel-mapping` 两处，与完成判据 2 一致）；要做的 2 ✓（一句覆盖读口 / 写口；写口清单逐项对回 `isolated_write_intake.go`：tenantId 含 null → `refuseSelfReportedTenant`；未知键、尾随第二个 JSON 值 → `decodeClosedDocument`→`decodeStrict`；不恰一项 → `exactlyOne` 与停用口 `len(Deactivations) != 1`；修订号 / 时刻解码错经 `decodeClosedDocument` 包 `ErrMalformedRequest`；封闭集词 → `partyRoleFromName` / `identityKindFromName`；标识或依据为空 → `newRequiredValue` 系构造门。多列的三种确以 MALFORMED_REQUEST 答回——判断项 2 成立）；要做的 3 ✓（两句未动，理由在票面 ③）；不做 ✓（未触 `catalogue-api.ts`、Go、表单；无票面没要的行为）；判断项 1 ✓（条件拼接而非无条件——`publication` 签与产品渠道族两签仍照批文形状，无条件拼入确会在同一段里自相矛盾）；判断项 3 部分成立（见 S2）。
- **一行**：两轴均无阻断，可重放。非阻断两条同源于一处「两口」计数，作者可在同一分支一笔改注释并顺手改票面判断项 3；不改亦不挡合入。

**推送方处置（通道 1 · 22:2x）**：N1 → `b185c0f2`（纯注释、自审，与票 09 的两处注释同笔）；S2 → 随 N1 消解，票面判断项 3 不改（它说的自 `b185c0f2` 起属实）；N2 / S1 只记。作者（通道 5 前任会话已断）无需再动。
