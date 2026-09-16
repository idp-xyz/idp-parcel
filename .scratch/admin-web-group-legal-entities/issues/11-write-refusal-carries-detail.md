# 11 写口 400 带 `detail`：Go `writeProblem` 加格、TS `callerProblem` 带 detail、`RegistrationAnswerNote` 显出

Category: enhancement
Status: resolved
Blocked by: 无
地盘：`internal/partycommercial/adapters/http/catalogue_intake.go`（`problemDetail` / `writeProblem`）、`registration_transport.go`
（`ErrMalformedRequest` → 400 那一支）与同包 `_test.go`；`apps/admin-web/src/pages/catalogue-api.ts`（`ApiResult.callerProblem`）、
`apps/admin-web/src/components/registration/RegistrationPanel.tsx`（`RegistrationAnswerNote` 的 callerProblem 分支）。
出处：通道 1 评估「业务参与方」页（钉 main `dbe989cc`，2026-09-16 20:2x），spec「第二轮」表第四档。

## 为什么

身份族在线口对形状问题的拒绝理由**已经写在错误里**：`isolated_write_intake.go` 用 `ErrMalformedRequest` 包装的散文逐条说了是
tenantId 在场、多出未知键、不恰一项、别的口的项、修订号 / 时刻解不出还是尾随第二个 JSON 值——可 `registration_transport.go` 到
`writeProblem` 只交 code，`{"error":{"code":"MALFORMED_REQUEST"}}` 一行，六种错在线上长一张脸。页面因此只能显一句通用说明
（票 08 修过措辞后仍是通用），操作者要靶着六种可能挨个试。这不是文案问题，是服务端把它知道的事丢在了线的这一头。

对照本包既有的纪律：`writePartyRegistryAnswer` 对`未受理`交 `cause` 散文（`RegistrationResponseBody.cause` 注释：「登记方拿一个没有
指名的 NOT_ACCEPTED 什么也补不了」）。同一个理由在 400 上同样成立，只是那一格今天没有。

## 要做的

1. **Go**：`problemDetail` 加 `Detail string `json:"detail,omitempty"``；给 `writeProblem` 一个带 detail 的兄弟（或变参），
   **只在** `registration_transport.go` 的 `ErrMalformedRequest` 分支交 detail——取包装散文里 `ErrMalformedRequest` 之后那半
   （`errors.Unwrap` 或按哨兵文本剪，别把哨兵自己的英文原句重复一遍）。`catalogue_intake.go` 读口的 400 是否一并交，作者裁：
   读口的 400 只有「kind 缺席或不在封闭集」一种，通用说明够用，不交也成立，写明即可。
2. **5xx 不带 detail**：`INTAKE_FAILED` / `NO_ANSWER_FORMED` 照旧只 code——那是依赖故障的内部原文，不外泄（本包 `writeProblem`
   注释若有此句就引，没有就在新函数头注写下这条边界）。
3. **detail 是散文不是代数**：前端原样示出、不查表、不据此分支——纪律同 `cause`。要可判别的理由代数得在用例侧立封闭枚举
   （先例 `CatalogRefusalReason`），不在传输层按字符串拼。这句写进 `problemDetail.Detail` 的注释。
4. **TS**：`catalogue-api.ts` 的 `callerProblem` 加 `detail?: string`，从 `error.detail` 取、缺席即 undefined（`visibility/api.ts`
   那份同形的 `ApiResult` 不动——各上下文自己的票）；`RegistrationAnswerNote` 的 callerProblem 分支在 `problemNote(code)` 后追加
   「原因：{detail}」一行，缺席不显。逐字段表单与 JSON 签共用这一处呈现，两边同时得到。
5. **契约登记**：`docs/` 里若有 problem 响应形状的权威描述（`"error":{"code"…}`），在那里补一行「`detail` 可选、散文、只随 4xx」；
   没有就在票面写明「本仓无此权威文档，形状以 `problemDetail` 为准」，不为一格新建 ADR——它是可加字段、不难逆转。

## 不做

- 不改 code 集合、不把 detail 做成枚举。
- 不动其他上下文的 http 适配器（各自另票，形状可照抄）。
- 不改 `isolated_write_intake.go` 的错误文本本身。

## 完成判据

- `go build ./...`、`go vet ./...`；`internal/partycommercial/adapters/http` 与 `cmd/parcel-api`（带 DSN）`go test -count=1` ok。
- http 包用例：带 tenantId / 未知键 / 两项 / 尾随内容四种拒绝各钉 `detail` 非空且含关键字（`tenantId` / `unknown field` / `exactly one` /
  尾随那句的关键词——不钉全文，全文属 `encoding/json` 与本包的措辞自由）；`INTAKE_FAILED` 用例钉无 `detail` 键。
- 三道门绿；演示形态：登记签粘带 tenantId 的项 → 页面显「原因：tenantId must not be carried in the payload…」。
- 机制清点预报零差（不增删文件）；不动 `cmd/`。

## 完成记录（2026-09-16，分支 `mcp4-adminweb11`，基 main `cadd2d46`）

作者：通道 4。`7fec23ce`（要做的 1–3，Go）与 `564f813d`（要做的 4，TS）为前一会话所作；该会话断后由同通道新会话接手收尾——要做的 5、完成判据全跑、本记录。隔离树 `D:/tops/idp-parcel-mcp4-adminweb11`，每笔推 origin。接手方式：先读票面与两笔 diff、再跑判据；两笔已推 origin、非未提交在途现场，故未另写 red，只对判据核：四子测钉的正是票面点名的四个关键字，5xx 钉「键不在场」而非「值为空」。

对完成判据（钉 `564f813d`，21:1x）：

- `gofmt -l internal/partycommercial/adapters/http` 空 / `go build ./...` 0 / `go vet ./...` 0；`go test -count=1 -v ./internal/partycommercial/adapters/http/` ok（187 PASS / 0 FAIL / 0 SKIP）；带 DSN `go test -p 1 -count=1 -v ./cmd/parcel-api/` ok（88 PASS / 0 FAIL / 0 SKIP；探针 `TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase` PASS 非 SKIP）。
- http 包用例 `TestMalformedRegistrationRefusalCarriesTheReasonAsDetail`：带 tenantId / 未知键 / 两项 / 尾随内容各钉 400、`MALFORMED_REQUEST`、`detail` 在场非空、含关键字（`tenantId` / `unknown field` / `exactly one` / `trailing`）、不含哨兵原句、无 outcome、`registrar.called` 为假。`TestCommercialRegistrationMapsOtherIntakeFailureToIntakeFailed`（INTAKE_FAILED）与 `TestCommercialRegistrationAnswersServerErrorWhenTheOrchestrationFails`（NO_ANSWER_FORMED）加钉 `detail` 键不在场——`problemDetailOf` 分开报「在场」与「值」，map 取值会把缺席折成空串。
- 三道门（隔离树 `apps/admin-web`，与 CI `admin-web` job 同口径）：`node node_modules/typescript/bin/tsc -b --noEmit` 0；`node scripts/run-tests.mjs` 242 pass / 0 fail；`pnpm exec vite build` 0。跑完 `git status --untracked-files=all` 空。
- 演示形态，服务端半边实跑：从 `564f813d` 构建 `parcel-api`，`IDP_PARCEL_ISOLATED_WRITE_TENANT=SYN-TENANT-AW11`（读开关同值）、同一只 55432 库，起在 `127.0.0.1:8095`；`POST /commercial-legal-entity-registrations` 带 tenantId 的项 → `400 {"error":{"code":"MALFORMED_REQUEST","detail":"tenantId must not be carried in the payload; the tenant grid is filled by the access channel, not by the request"}}`；未知键 → `detail` 为 `json: unknown field "name"`；两项 → `legalEntities must carry exactly one item, got 2`；尾随 → `trailing content after the payload`。进程已停、临时目录已清。页面半边（`RegistrationAnswerNote` 显「原因：…」）是 JSX，本仓测试口径（`tsconfig.test.json` 只收 `src/**/*.test.ts`）不覆盖组件渲染，浏览器未验；判读半边由 `catalogue-api.test.ts` 两条钉住（带 detail 原样交回；detail 非字符串当缺席）。
- 清点预报零差：在 `564f813d` 的 detached 干净检出（`%TEMP%\idp-inv11`）跑 `tools/mechanism-inventory -out` 到临时路径，与已提交文件 `git diff --no-index` 退 0；检出已拆（`worktree remove` 不加 `--force`，退 0，目录已不在）。不增删文件、不动 `cmd/`。

要做的逐条：

① Go：`problemDetail.Detail`（`json:"detail,omitempty"`）+ `writeProblemWithDetail`——单立函数不给 `writeProblem` 加变参，交不交 detail 在调用点一眼读得出；只在 `writeRegistrationIntakeProblem` 的 `ErrMalformedRequest` 分支交。`malformedRequestDetail` 按哨兵文本剪出之后那半：`errors.Unwrap` 解到的是哨兵本身，理由散文在外层 `Error()` 里；哨兵不在文本里时整句就是散文，剪完为空时那一格缺席。**读口 400 不交 detail（作者裁）**：`writeCatalogueIntakeProblem` 那一支只有「kind 缺席或不在封闭集」一种，通用说明够用。
② 5xx 不带 detail：INTAKE_FAILED / NO_ANSWER_FORMED 照旧走 `writeProblem`；边界写在 `problemDetail.Detail` 与 `writeProblemWithDetail` 头注。
③ 散文不是代数：写进 `problemDetail.Detail` 注释，纪律同 `registrationAnswer.Cause`；要可判别的代数在用例侧立封闭枚举（先例 `CatalogRefusalReason`），不在传输层按字符串拼。
④ TS：`ApiResult.callerProblem` 加 `detail?: string`，`classifyMasterDataResponse` 只在 `error.detail` 是字符串时带上这一格（缺席不多出值为 undefined 的键，别的形状不冒充散文）；`RegistrationAnswerNote` callerProblem 分支在 `problemNote(code)` 后追加「原因：{detail}」，缺席不显；`visibility/api.ts` 未动。
⑤ 契约登记：**本仓无已接受的 problem 响应形状权威文档，形状以 `problemDetail` 为准。** 取证（钉 main `e5ff0f20`，对 `docs/` 查 `"error":{"code"` 与 `problemDetail|writeProblem`）：命中的只有 ADR-0140（**Proposed 草案**，Status 自述「接受与否归用户，接受前不是依据」）与 `docs/design/synthetic-demo-journey-script.md`（一个 403 例子，不是形状描述）；ADR-0055 决定二只定码集，不定体形状。不新建 ADR。

判断项 / 上报：

1. **与 ADR-0140 草案 Decision 三相反，归 owner 裁**。该稿写 RFC 9457 问题体「**不带 `detail`**——自由文本正是 UC-PS-001 那条泄露约束要挡的东西，今天的问题体不带它，换媒体类型不把它加回来」，且「4xx 与 5xx 一律」、写问题体的辅助函数将收进 `internal/platform/httpapi`。本票照票面做、不改草案：(a) 草案接受前不是依据；(b) 本票的 detail 只随操作者族（ADR-0100）写口的 4xx，内容是请求自身的形状错，不含任何他租户的存在、业务量或内容——UC-PS-001 那条约束落在首方客户面；(c) 同包 `cause` 对`未受理`交散文已是先例；(d) 可加字段、不难逆转。但 ADR-0140 落地时两者必须裁一个：要么 Decision 三收窄为「首方族不带 `detail`；操作者族 4xx 可带、内容限于请求自身形状」（RFC 9457 本有 `detail` 成员，映射零翻译），要么 `writeProblem` 收进 `httpapi` 那一笔把本票这一格删去。作者无权定，随完工报已告知通道 1。
2. 红写在 HTTP 缝（`NewRegisterLegalEntityEndpoint` + `httptest`）而不是 `malformedRequestDetail` 函数缝：票面判据是登记方看得见的三样（400 / code / detail），只有过了端点才证得到；剪字的分支（哨兵不在文本里、剪完为空）由四子测的形状覆盖不到，属函数级细节，头注写明即可，不另立用例。

## Comments
