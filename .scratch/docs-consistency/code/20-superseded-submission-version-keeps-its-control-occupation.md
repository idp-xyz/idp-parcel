# 20 PS 新提交版本取代旧版本时，旧版本已成立的控制项占用无人释放

Category: bug
Status: ready-for-agent——R1（上线前必修）：碰钱、静默。可达性资料不足仍照常进入接受前财务控制并形成占用，任务停在`等待受控补充`；客户补件走新提交版本，旧版本被取代，它那一笔预付冻结或账期额度占用永远不释放——第一周补件就撞，客户可用额度被永久吃掉，没有任何释放入口能按旧版本认领。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ps.md](../audit/ps.md) PS-01，取证 [audit/ps-code.md](../audit/ps-code.md) 第 1 条（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，PS 代码在此期间未改）。

## 现象

`application` 包里 `ReleasePreAcceptanceControl` 只出现在拒绝、撤回、授权处置三条路，以及 `form_acceptance_decision.go` 的 `releaseIfRejected`。新提交版本形成、旧版本被取代的那一路没有释放调用。占用请求按提交版本认领，新版本的控制顶不掉旧的那一笔。

## 证据

- 文档：
  - PS CONTEXT 词条「控制项结果」：「任一项成立即表示提供方账本上形成了占用；接受确定未成立时必须按原业务关联请求释放，不论接受侧结论如何。」
  - 词条「授权处置」：「两者都按原业务关联释放本版本已成立各项的占用」。ADR-0132 给的理由：选交客户补充的那一刻，这一版的接受已确定不成立——被取代同理。
- 代码（读码推断，审计取证一致）：
  - `internal/parcelshipment/application/form_new_submission_version.go` 与 `domain/submission_supersession.go` 两处都没有释放调用（复核时 `grep Release` 两个文件均为零处）。
  - 可达性答资料不足时 `domain.ReachabilityCheckFor` 记 `CheckUndetermined`，`AdvanceAcceptanceJudgmentHandler` 仍交 `AcceptanceJudgmentAdvanced`；`AdvanceAcceptanceChainHandler` 因此照常进入财务控制并形成占用，`Decide` 再把任务停在`等待受控补充`。
  - `ControlReleaseRequest` 按提交版本认领。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：取代那一笔（或经续办信封）对被取代版本 `OccupationFormed()` 为真的各项按原关联发释放；失败按 `ControlReleasePending` 续办；补回归用例。

## 最小的诚实修法

新提交版本形成、旧版本被取代时，同一事务内为旧版本已成立占用的各项登记释放请求（与拒绝、撤回同一个出口），按原业务关联认领；释放未完成的照既有 `ControlReleasePending` 续办。补一条「资料不足 → 已占用 → 补件取代」的回归测试，断言旧版本的占用被释放、新版本重新判控制。

## 要裁的

无（CONTEXT 与 ADR-0132 的理由已覆盖）。存量：首发前只有演示租户，`--reset` 重灌即可。
