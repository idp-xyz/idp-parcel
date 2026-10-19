# 02 同一来源两份异内容并发写入时，输家那一步单元更新照样提交

Category: bug
Status: needs-triage——2026-10-18 Claude Code 会话立，出自 [no-directional-basis/01](../../no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md) 第二轮非作者评审（隔离子代理）的非阻断第 1 条。只读取证，未动代码。
Blocked by: 无
出处：`ConsolidateParcelsHandler.recorded` 的撞键分支；`ConsolidationUnits.Update`；票 01 的来源事实代数（AT-NO-043「冲突保留双方并停止自动派生」）。

## 现象（读码，钉 `claude-ndb01dev`）

- 两个导入进程（或日后两台设备）并发报同一来源标识、不同内容的移入或移出：两边都过了受理闸（那一刻册上还没有这条来源事实），各自在自己的事务里先 `Store.Update` 单元，再 `Facts.Save`。
- 后到的一方 `Facts.Save` 撞键答`已有记录`，`recorded` 读回赢家、比内容指纹，答`来源冲突`——答复是对的。
- 但 handler 交回的是 nil error，导入壳照常提交事务：输家那次 `Store.Update` 跟着提交了。`ConsolidationUnits.Update` 没有版本校验，按读到的单元整行改写，可能盖掉赢家刚写进去的成员。
- 结果：来源事实册上只有赢家那一条，单元的当前成员却可能是输家的样子；「冲突保留双方并停止自动派生」在这条路上不成立。

## 要裁的问题

1. 撞键答来源冲突（或已有结果）时，本次单元更新要不要撤回：handler 交回错误让壳回滚，还是 `recorded` 之前先写事实、后写单元。
2. `ConsolidationUnits.Update` 要不要带版本（乐观锁），让并发两写的后一写在单元那一步就撞出来。

## 完成判据

- 真库用例：同一来源两份异内容并发写，事后单元成员与册上那条来源事实一致；输家答来源冲突，不改单元。
