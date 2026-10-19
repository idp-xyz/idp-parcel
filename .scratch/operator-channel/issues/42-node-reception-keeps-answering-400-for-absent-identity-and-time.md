# 42 作业事实面上缺席的身份与时刻答哪一格：NO 收寄答 400，TF 交编排答未受理——NO 一侧与票 08「缺席交编排」不符

Category: task
Status: resolved——2026-10-19 进 main（与 tf-face-algebra/01、operator-channel/37、42 同批，代码验证钉 `003a1f36`），见「进 main 记录」。此前 in-progress——2026-10-19 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（分支 `claude-oc42`）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 序 5 研判）
Blocked by: operator-channel/29 已进 main（ADR-0186 在场）
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判簇 C4 第六张草案
地盘：
- internal/nodeoperations/adapters/http/reception_payload.go 与其测试（视裁定）
- NO 收寄编排（若裁定要它对缺席答未受理）
- docs/domain/node-operations/CONTEXT.md，或在 ADR-0022 后追加复核记录（若裁为有意差异；ADR 正文不改）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 裁定（2026-10-18，会话 idp-parcel-9b 代决）

取 (b)。维持 NO 收寄对缺席的事实身份与发生时刻答 400：事实身份是判重键，缺了连重放都认不出；这与 ADR-0186 定的「输入缺陷答 400」是同一格。理由在 ADR-0186 后追加复核记录（决定原文不动），并在票 08 Comments 记一句：其「缺席交编排」是 TF 作业事实口的分格，NO 收寄不适用。不立跨上下文的统一规则（ADR-0124 决定三是反向先例）。本票只动文档，外加一条钉住现状的用例。

## 现象（钉 `8d22729f`）

- NO `receptionCommand`：
  - `sourceId` 为空时答 400，错误信息是「the device-issued fact identity is not minted here (ADR-0023)」；
  - `occurredAt` 直接 `time.Parse`，缺席即答 400 `MALFORMED_REQUEST`；
  - `git log -S 'occurredAt is not an RFC 3339 instant'` 只命中 `f043ed03`（票 08 第 1 口），可见从那一笔起就是这样；
  - 研判探针实测：缺 `occurredAt` 答 `ErrMalformedRequest`。
- TF 在同一能力面上，事实时刻缺席时交编排，答 200 `SOURCE_NOT_ACCEPTED`（`parseOptionalInstant`；更正口由「TF 更正载荷缺时刻」票对齐到同一规则）。
- 影响：按 ADR-0022 的表，两种答法在离线端都是出队交人，影响低。但同一个一线作业端会在同一能力面上看到两种答法。

## 要裁的一问

NO 收寄缺席的事实身份与发生时刻，答哪一格：

- (a) 对齐「缺席交编排」：Intake 交零值，由 NO 编排答它的未受理格。要先看 NO 编排有没有这一格，以及 UC-NO-002 怎么说。
- (b) 维持 400，写明这是 NO 有意的分格及其理由（例如事实身份是判重键，缺了它连重放都认不出），并说明票 08 那句在 NO 一侧不适用。理由记进 NO 的 CONTEXT，或在 ADR-0022 后追加复核记录。

要不要跨上下文立一条「缺席字段答哪一格」的统一规则，也在本问之内。ADR-0124 决定三（计价回放缺任何一样即 400）是现成的反向先例。

## 不做

- 不改 TF：TF 包内的分格已由「TF 更正载荷缺时刻」票定。

## 完成判据

- [ ] 已裁，写进本票；若裁出跨上下文规则，或属难逆转的取舍，进 ADR 或复核记录。
- [ ] 若取 (a)：NO 收寄缺 `occurredAt`、缺 `sourceId` 两种情形，各有 Intake 用例、编排用例和真库用例。若取 (b)：NO 侧的注释或文档写明理由，并在票 08 Comments 追记。
- [ ] 若改了代码：过 Go 门禁，走 Go 那条路（parallel-sessions）。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的子代理，分支 claude-oc42）

- **落点**：`1f8d41d7`——[ADR-0186](../../../docs/adr/0186-node-reception-records-physical-facts-when-the-service-result-read-fails.md) 文末追加「复核记录（2026-10-19，决定原文不动）」三条（缺席答 400 与决定四同一格及理由；票 08「缺席交编排」只在 TF 作业事实口成立；不立跨上下文统一规则，ADR-0124 决定三为反向先例），Decision 与其余各节未动；票 [08](./08-isolated-release-of-main-chain-command-faces.md) Comments 末尾只加一行指向该复核记录。代码未改。
- **用例**：`c7357f34`——`work_fact_intake_test.go` 新增 `TestWorkFactReceptionAnswers400ForAbsentFactIdentityOrOccurrenceTime`：生产 `WorkFactReceptionIntake` 接端点，编排侧装 `failingHandler`（漏过 Intake 就成 500），四个子用例（缺事实身份、事实身份为空白、缺发生时刻、发生时刻为空串）各断言 400 `MALFORMED_REQUEST`，头注引上述复核记录。既有 `TestWorkFactReceptionIntakeRefusesMalformedReceptionPayloads` 只在 Intake 层断言缺事实身份，没有缺发生时刻一格，故补这一条。这是钉现状：首跑即绿，没有先红。
- **门**：`gofmt -l internal/nodeoperations` 无输出；`go vet ./internal/nodeoperations/...` 退 0；`go test -count=1 ./internal/nodeoperations/adapters/http/` ok。
- 评审：待会话读 diff 审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `10c2583c`（基线 `32a9d03f`）· 2026-10-19**：出自本会话起的子代理，会话读 diff 审，不算非作者评审。ADR-0186 文末追加复核记录三条、Decision 原文未动，与裁定 (b) 一致；票 08 Comments 只加一行；新用例 `TestWorkFactReceptionAnswers400ForAbsentFactIdentityOrOccurrenceTime` 钉现状（缺事实身份、空白事实身份、缺发生时刻、空发生时刻各答 400），首跑即绿、非先红后绿，完成记录如实写。阻断无，非阻断无。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 同批三张（tf-face-algebra/01、operator-channel/37、42）依次重放到远端 main `32a9d03f` 之上，机制清点在重放后的树上重生成；原分支 SHA 作封存出处。
- 推送方全量一次：钉 `003a1f36` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 143 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
