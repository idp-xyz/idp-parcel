# 11 写口 400 带 `detail`：Go `writeProblem` 加格、TS `callerProblem` 带 detail、`RegistrationAnswerNote` 显出

Category: enhancement
Status: ready-for-agent
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

## Comments
