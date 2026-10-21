# 23 PS「POD 错误致有效交付失效」（`AT-PS-063`）在代码里表达不出来：重派生只能产出另一个终局

Category: bug
Status: ready-for-agent——R3（不急）：TF 今天只有「POD 证据更正」（`EffectiveDelivery.CorrectProof`），没有「有效交付失效」的生产者，这一格走不到；TF 立出失效入口时须同时修本票，否则失效之后包裹仍挂着终局、委托完成摘要跟着错。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ps.md](../audit/ps.md) PS-23，取证 [audit/ps-code.md](../audit/ps-code.md) 第 11 条（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象与证据

- 文档：UC-PS-004 `AT-PS-063`：「原有效交付因 POD 错误被失效 | 保留原终局历史，形成新判断版本并重新派生当前结果」；结果表「当前终局重派生」一行。
- 代码：
  - `domain.ResponsibilityOutcomeSpec` 没有有效性格。
  - `FormParcelFinalHandler` 遇同包裹、同种来源时一律 `rederive` 并写 `Finalized: true`。
  - `domain/final_outcome.go` 的 `ParcelFinalOutcome.Rederive` 沿用原 `kind` 与 `ruleVersion`，不重新调用终局规则；头注却声称它就是 `AT-PS-063`。
  - 相关的另一半——面单服务「承运商收寄被作废」后的重派生——代码显式答 `ErrVoidedCarrierPickupRederivationUndecided`，等 ADR-0138（Proposed），已由票 label-channel-service-first-release/39 承接，不在本票。

## 三类

机制。无产品策略、无租户取值。

## 规模

M：责任结果加有效性格（或「失效」种类）；重派生时按规则重新判，可以得出「无当前终局」；完成摘要随之；改正头注。

## 最小的诚实修法

来源失效时形成新的终局判断版本，按终局规则重判当前结果：没有其他有效来源就是「当前无终局」，原终局留作历史；不得沿用原种类。头注改成如实描述。TF 的失效入口另立（不在本票），本票先把 PS 半边的表达能力补齐并用单元用例覆盖。

## 要裁的

- TF「有效交付失效」入口归哪张票：TF 今天只有证据更正。作者在本票落地时若发现无票承接，按 TF CONTEXT「POD 被更正或失效时保留原证据和判断，形成新版本」另立。
