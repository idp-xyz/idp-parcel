# 02 同一来源两份异内容并发写入时，输家那一步单元更新照样提交

Category: bug
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R2）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-18 Claude Code 会话立，出自 [no-directional-basis/01](../../no-directional-basis/issues/01-consolidation-adds-members-without-any-directional-basis.md) 第二轮非作者评审（隔离子代理）的非阻断第 1 条。只读取证，未动代码。
Blocked by: 无
出处：`ConsolidateParcelsHandler.recorded` 的撞键分支；`ConsolidationUnits.Update`；票 01 的来源事实代数（AT-NO-043「冲突保留双方并停止自动派生」）。

## 现象（读码，钉 `claude-ndb01dev`）

- 两个导入进程（或日后两台设备）并发报同一来源标识、不同内容的移入或移出：两边都过了受理闸（那一刻册上还没有这条来源事实），各自在自己的事务里先 `Store.Update` 单元，再 `Facts.Save`。
- 后到的一方 `Facts.Save` 撞键答`已有记录`，`recorded` 读回赢家、比内容指纹，答`来源冲突`——答复是对的。
- 但 handler 交回的是 nil error，导入壳照常提交事务：输家那次 `Store.Update` 跟着提交了。`ConsolidationUnits.Update` 没有版本校验，按读到的单元整行改写，可能盖掉赢家刚写进去的成员。
- 结果：来源事实册上只有赢家那一条，单元的当前成员却可能是输家的样子；「冲突保留双方并停止自动派生」在这条路上不成立。

- 2026-10-25 追记（票 docs-consistency/code/19 进 main 后，协调方据该票第二轮隔离评审的真库探针）：改单元的各口读单元持单元行锁（`ports.ConsolidationStore.FindByIDForUpdate`），同一只单元上的两笔并发写按行锁先后串起，后到的一笔读到的是先到那笔提交后的单元，「按读到的单元整行改写，可能盖掉赢家刚写进去的成员」这一半不再成立；输家那一步仍叠在赢家之上随提交落册、答`来源冲突`，要裁的 1 与完成判据不变。要裁的 2（版本校验）要拦的只剩不经读来改读出的单元交给 `Update` 这一种（code/19 越权风险点 2）。
- 同一探针另测到一格：同一来源、同一成员、只有证据不同的两笔并发移入，输家答 `MEMBER_ALREADY_CONTAINED`，导入记成重放（「成员已由别的来源移入本单元」），而不是`来源冲突`——结构上「已在本单元」那一判排在来源冲突复查之前；移入的租户锁在读单元之前，两笔重叠时固定落这一格。裁要裁的 1 时一并定这一格该答什么。

## 要裁的问题

1. 撞键答来源冲突（或已有结果）时，本次单元更新要不要撤回：handler 交回错误让壳回滚，还是 `recorded` 之前先写事实、后写单元。
2. `ConsolidationUnits.Update` 要不要带版本（乐观锁），让并发两写的后一写在单元那一步就撞出来。

## 完成判据

- 真库用例：同一来源两份异内容并发写，事后单元成员与册上那条来源事实一致；输家答来源冲突，不改单元。
