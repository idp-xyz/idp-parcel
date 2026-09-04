# 资格编排把「规则已登记但截止算不出」报成「规则未登记」/「补充窗口已关」——两格未决的名字对新状态不准

Category: enhancement
Status: ready-for-agent
Blocked by: [03](./03-claim-deadline-and-materials-read-party-commercial-rule-content.md)（把两维接上 PC 之后这一状态才可达）

## 事实（钉在票 03 分支 `mcp4-ve03`，`internal/visibilityexception/application/handle_claim.go` 与 main 同）

- 票 03 落地后，PC 登了客户服务规则正文的租户会拿到 `FilingDeadline.Registered=true` 而 `Deadline`
  为零值（起算事实源与业务日历今天没有，截止时刻是留格，见票 03「裁决」），以及
  `Materials.Registered=true` 而 `SupplementDeadline`、`Notice` 为零值。
- `judgeFilingDeadline` 把 `Registered` 为真但 `Deadline` 为零判成核不了，reason 用的是
  `EligibilityFilingDeadlineNotRegistered`、basis 用的是 `FILING_DEADLINE_RULE_NOT_REGISTERED`——
  这一格原本只为「规则没登」而设，现在同时承载「规则登了、起算事实缺」。两种的恢复动作不同：
  前者去 PC 登一版规则，后者要等 VE 有起算事实源与日历能力（票 03「裁决」两条留格）。看着
  `NOT_REGISTERED` 去 PC 补规则的人会发现规则早就在。
- `applyScreen` 在差材料时先看 `!rules.Materials.SupplementDeadline.After(now)`，零值截止落进
  `EligibilitySupplementWindowClosed`。那一支的注释写的是「补充期限已经不在未来……规则未定或延期待
  确认时保持待决定」——停在未决、不记第三态、不拒赔，这些都对；但名字说的是「窗口已关」而实情是
  「截止算不出」。装配测试 `TestTheWiredClaimsReadCustomerServiceRulesFromPartyCommercial` 钉着这一格，
  本票落地时那一行随之换名。
- 两格都不是票 03 能改的：`application/` 不在那一票地盘（派单原文），而它们是编排对端口答案的
  解读，归编排。

## 要做什么

- `HandleClaimUndecidedReason` 加两格并进 `String()`：`EligibilityFilingDeadlineUnderivable`
  （`ELIGIBILITY_FILING_DEADLINE_UNDERIVABLE`）与 `EligibilitySupplementDeadlineUnderivable`
  （`ELIGIBILITY_SUPPLEMENT_DEADLINE_UNDERIVABLE`）；basis 常量对应加 `FILING_DEADLINE_UNDERIVABLE`。
- `judgeFilingDeadline`：`Registered` 为假照旧答未登记；`Registered` 为真而 `Deadline` 为零（或
  五样里任一缺）答 `Underivable`，basis 带上 RuleVersion 与 StartEvent——那两样这时候是有的，
  记下来人才知道缺的是事实不是规则。
- `applyScreen` 差材料那一支：`SupplementDeadline` 为零值先答 `SupplementDeadlineUnderivable`，
  再谈「不在未来」；零值不是一个过去的时刻。
- 既有钉 `ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED` 的用例（`handle_claim_test.go`、
  `cmd/parcel-api/assemble_claims_test.go` 两处）逐条核：规则确实没登的保持原名，登了而算不出的换名。
- `internal/architecture/enum_exhaustiveness_test.go` 若钉着 reason 集合，随之更新。

## 不做的

- 不在这两格里派生任何截止：起算事实源与日历能力是另一票的事，本票只把名字说准。
- 不把「未登记」与「算不出」压回一格：恢复动作不同，票 01 Comments 的纪律（「整份声明还没登记」与
  「只差材料清单」不折成同一格）在这里同样成立。

## 完成标准

- 装配测试对真库钉：PC 登了正文、材料齐 → `ELIGIBILITY_FILING_DEADLINE_UNDERIVABLE`；PC 登了正文、
  差材料 → `ELIGIBILITY_SUPPLEMENT_DEADLINE_UNDERIVABLE`；PC 没登 → 照旧 `ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED`。
- VE 全包与 `cmd/parcel-api` 真库套件绿。

## Comments

- 2026-09-04 MCP-4：随票 03 立（ready-for-agent）。两格的判据在类型上已经分得开（`Registered && Deadline.IsZero()`），
  不需要新裁决。
