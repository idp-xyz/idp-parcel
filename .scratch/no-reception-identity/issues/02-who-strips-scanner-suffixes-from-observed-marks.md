# 02 收寄现场观察的号码：在线收寄口原样交核对，受控批量口逐格去两端空白——扫描后缀与两口口径归 NO 定

Category: task
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（分支 `claude-nri02`）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 序 0 研判）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判簇 C2 第四张草案
地盘：
- internal/nodeoperations/adapters/http/reception_payload.go
- internal/nodeoperations/adapters/http/work_fact_intake_test.go
- cmd/parcel-frontline-import/template.go
- cmd/parcel-frontline-import/intake_template_test.go
- docs/application/node-operations/UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md（或 docs/domain/node-operations/CONTEXT.md，按裁定）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 出处

票 ps-external-mark-relations/02 评审遗留（operator-channel/26 序 0）的 C2-Q1 裁定，经研判与两路对抗核验，会话 `idp-parcel-9b`，钉 main `8d22729f`。两路核验都要求：不要在 PS 的 ADR 里替 NO 与一线客户端定这条规矩。

## 现象

- 在线收寄口：`receptionCommand` 把载荷 `mark` 原样交进命令（`ports.ExternalMarkObservation`），这个值也进收寄内容摘要（`ReceiveDeliveredUnitHandler.Handle` 调 `CanonicalizeDeliveryPayload`）。探针：前导空格加尾随回车换行的 `mark`，原样进了命令。
- 受控批量口：收寄模板 `decodeTemplate` 对每一格做 `strings.TrimSpace`，`mark` 也在内。探针：两端带空格与 Tab 的 `mark` 格，去成了干净的号。
- 端到端：PS 核对逐字（PS CONTEXT「按外部标识核对包裹身份」）。在线口带尾随空格或回车换行的号，答零个候选、走待识别。这是安全失败（实物事实照样保全，不关联错件），但识别丢了，要等「身份后来被确认」（`AT-NO-026`）。
- `receptionDocument` 头注说两口「同一件事在两处不换词」，指的是键名与词表；值里的空白怎么处理，两口没有共同说法。

## 要裁的一问

扫描附带的终止符（回车、换行、Tab）与两端空白，由谁去掉？

- **(a)** 一线客户端送出前去掉，服务端两口维持现状。在线口原样保全设备交来的事实（ADR-0023：作业事实由设备签发）；批量口去空白，算 CSV 模板的形状纪律。
- **(b)** 在线口在服务端也去两端空白。两口一致了，但改写了设备交来的事实与收寄内容摘要：带不带后缀的同一来源重投，会被判成同一内容。
- **(c)** 在线口把带两端空白或控制字符的 `mark` 当畸形请求拒掉（400）。客户端缺陷当场暴露，但这一次的实物事实没保全，与 UC-NO-002「先保全来源事实」相抵。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

取 (a)。服务端两口维持现状：在线收寄口原样保全设备交来的观察（ADR-0023 作业事实由设备签发），批量模板逐格去两端空白算 CSV 模板的形状纪律；扫描附带的回车、换行、Tab 与两端空白由一线客户端送出前去掉，写进一线客户端形状的要求。在线口带空白的号答零候选、走待识别，是安全失败。本票只动文档（UC-NO-002 或 NO CONTEXT 记一句、`receptionDocument` 头注写明值的空白两口各自怎么办）与一条钉住在线口原样的用例。

## 做什么（裁后）

- 把裁定写进权威处：UC-NO-002「输入语义契约」里「作业实物」一行，或 NO CONTEXT，由 NO owner 定。在线口的载荷注释与之保持一致。
- 两口各钉一条用例：裁定维持现状，就是刻画用例；改了行为，就是回归用例。

## 不做

- 不在 PS 核对读口做折算：PS CONTEXT 规定条码规则属租户取值；ADR-0184 风险点 2 那一问归 PS owner。
- 一线客户端本身的实现。

## 完成判据

- [ ] 这一问已裁，写进本票与权威文档。
- [ ] `internal/nodeoperations/adapters/http/work_fact_intake_test.go`：在线口 `mark` 带两端空白或回车换行时，按裁定处理；`cmd/parcel-frontline-import/intake_template_test.go`：模板 `mark` 格按裁定处理。两条都要有。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-frontline-import` 全绿。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
