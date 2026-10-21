# 22 PS 客户面 HTTP 契约没按已接受的 ADR 落：`!found` 答 500 而非 404，`/v1` 与问题详情没落地

Category: bug
Status: ready-for-agent——R1（上线前必修）：理由不是答错，而是不可逆窗口。首方客户提交口今天已在生产上（`/shipment-requests`），上线即对客户系统发布首个兼容性基线；ADR-0140 后果节写首方族就位后提交口变 `/v1/shipment-requests`、问题体用 `application/problem+json`，基线之后再改就是破坏性变更，只能长期双挂。`!found` 答 5xx 会让离线客户端无限重投、堵住自己的队列（ADR-0022 给的理由）。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ps.md](../audit/ps.md) PS-07、PS-24，取证 [audit/ps-code.md](../audit/ps-code.md) 第 3、12 条。两条同一上下文、同一根因——客户面 HTTP 适配器的契约形状没跟上已接受的契约 ADR——合成本票（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

1. **`!found` 不是 404**（PS-07）：客户面撤回、资料修订、受控补充等命令口把「查无委托」与其余没形成答案的失败一并答 5xx `NO_ANSWER_FORMED`；逐包裹取消口在 `!found` 时交 `CancellationNotAccepted`，即 `200` + `REQUEST_NOT_ACCEPTED`，把「查无」与「字段缺」并成一格。
2. **`/v1`、问题详情、端点描述符都没落地**（PS-24）：提交口仍是 `/shipment-requests`，以 `application/json` 写出自造的 `problemResponse`；仓内没有契约生成器，也没有 `problem+json`；ADR-0140 接受后没有立实施票。

## 证据

- 决定：
  - ADR-0022 Decision：「**`!found` 一律映射为对应上下文的`统一不可见结果`，用 `404`。**」，Consequences：「接 HTTP 时由适配器映射为 `404`，不得原样 500。」这一条不在 ADR-0022 已部分停用的范围内。
  - ADR-0140 决定一（端点描述符与生成器）、决定二（`/v1`）、决定三（`application/problem+json`）；后果节：「首方族就位后，客户面提交口的路径变为 `/v1/shipment-requests`」。
- 代码：
  - `internal/parcelshipment/adapters/http/withdraw_shipment_request.go` 头注：「其余没形成答案的失败一并回 5xx `NO_ANSWER_FORMED`——不细分」；`amend_customer_source_data.go`、`form_new_submission_version.go`、`complete_manual_review.go` 同款。头注把 `404` 读成了「拆开统一不可见结果」——统一不可见要的是「不存在」与「他人的」同答，`404` 本身就是那个同答。
  - `application/cancel_parcel.go`：`!found` 时 `CancelParcelResult{outcome: CancellationNotAccepted}`。
  - `cmd/parcel-api/endpoints_submission.go`：`"/shipment-requests"`；`adapters/http/submit_shipment_request.go` 写 `problemResponse`。
  - 撤回、资料修订等口今天在生产装配下仍以 `UnconfiguredIntake{}` 起步，首方 Intake 逐口换上时即生效。

## 三类

机制（HTTP 适配器契约）。产品策略：契约版本与问题详情的形状（ADR-0140 已定）。租户取值：无。

## 规模

M–L：路径加 `/v1`（旧路径是否在过渡期双挂见「要裁的」）；问题详情改 `problem+json` 并带 `requestId`；客户面命令口 `!found` 统一 `404` 加上下文统一不可见码；ADR-0140 决定一的描述符与生成器可拆成后续一笔，但路径与问题体须在基线前落。

## 最小的诚实修法

1. 客户面命令口（撤回、资料修订、受控补充、逐包裹取消）`!found` 一律 `404` + 上下文统一不可见码；取消口把「查无」与「字段缺」分开。改掉那几处头注。
2. 客户面提交口及同族口迁到 `/v1/...`，问题体改 `application/problem+json`。
3. 登记进 [first-party-api/spec.md](../../first-party-api/spec.md) 的子票表（ADR-0140 的实施票），已在那份规格补指针。
4. 操作者族（`403 OPERATOR_NOT_GRANTED` 的探针同答，ADR-0100）不在本票改；若那是有意的，修订票 docs-consistency/06 在 ADR-0022 的 Status 行补注适用范围。

## 要裁的

- 旧路径 `/shipment-requests` 在基线前直接改名，还是双挂一段过渡期。首发前没有真实集成方，推荐直接改名（管理台提交页随改），作者可代决。
- 描述符与生成器（ADR-0140 决定一）是否与路径、问题体同票：推荐拆为后续票，本票只做基线前必须定形的两件。

## 文档自洽 06 复核补记（2026-10-21）

- 「最小的诚实修法」第 4 条操作者族一问：不需要在 ADR-0022 的 Status 行补注。`403 OPERATOR_NOT_GRANTED` 是接入身份层在指名任何对象之前的答复（ADR-0100 决定四：操作者不在册、不绑这个租户或无此能力面的授予），ADR-0022 的 `!found` 一条管的是通过接入之后、按对象指名查不到的那一格，两者不在同一层。
- 资料修订口同族的另几格：委托未接受、修订意图未声明、内容与范围不配，今天同样以错误上抛、答 5xx `NO_ANSWER_FORMED`；它们是调用方的错，照 ADR-0022 属 4xx。随本票第 1 条一起改，UC-PS-002「给开发的交接」已指本票。
- 首方契约文档写明不收客户声明的来源发生时间、信封以接收时刻记（[ADR-0258](../../../docs/adr/0258-the-first-party-family-declares-no-source-occurrence-time-and-records-the-receipt-instant.md)），随本票第 2 条的契约形状一起落；代码取法今天已是如此，不改。
- 取消口「查无与字段缺分开」一条：UC-PS-006 新立的结果语义契约把 `REQUEST_NOT_ACCEPTED` 限到字段缺，查无、跨租户、跨客户与成员出界归统一不可见，「给开发的交接」已指本票。
