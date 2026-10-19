# 12 装载分配：在变化版上撤回会照抄前版的修订时刻，存库撞 revision_xor_withdrawal

Category: bug
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-tfa121310`（与 13、10 同一分支）。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立（tf-face-algebra/02 实施中发现，票外未改）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：tf-face-algebra/02 实施子代理的「本票外的发现」，经其非作者评审确认。
地盘：
- internal/transportfulfillment/domain/load_assignment.go 及其测试
- internal/transportfulfillment/adapters/postgres/load_assignment_registry.go 的真库用例

## 现象

领域 `LoadAssignment.Withdraw` 撤回一个变化版时，把前版的 `revisedAt` 一并照抄进撤回版；迁移 0009 的 `revision_xor_withdrawal` 要求修订与撤回二者只占其一，这一版存库会被 CHECK 拒，整笔事务作废。今天撤回入口没接（tf-unwired-seven/05），所以潜伏。

## 应当

撤回版只带撤回那一组格，不带修订时刻；链上的分配时刻照迁移 0009 头注沿用原分配时刻。

## 完成判据

- [ ] 领域用例：在变化版上撤回，撤回版不带修订时刻。
- [ ] 真库用例：同一条撤回版经 `LoadAssignments.Save` 落库成功、读回一致。
- [ ] 排在 tf-unwired-seven/05 接撤回入口之前修好。
- [ ] Go 门禁照常。
