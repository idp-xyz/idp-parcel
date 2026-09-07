# Decimal 重建门收非规范写法，`percentShare` 真会产出它：同一个数两种写法进语义摘要是两个串

Category: bug
Status: ready-for-agent——MCP-6 2026-09-07 立票，随 `ParseCanonical` 删除（分支 `mcp6-pp-ratchet` 的 `1d13d510`）；同日接管会话只读复核判为真缺陷，MCP-1 12:2x 裁「转 ready、不并入 task-f31a5650、作 MCP-6 下一单（PP 地盘不换人）」
Blocked by: 无外部票。「要先裁的一格」在本票内先裁（`/domain-modeling`，收紧与语义摘要可比性一起；落 ADR 时预留号已尽，向 MCP-1 取号）

## 现状（四格钉在 `internal/parcelpricing/domain/decimal_canonical_rebuild_test.go`，锚 `2efef58e`）

1. `Decimal.valid()` 不是规范性检查：它拒前导零、拒「系数为零而标度非零」，**不拒标度大于零时的尾随零**。于是 `{coefficient:"100", scale:2}` 与 `{"1", 0}` 都过 `valid()`，`Cmp` 判同一个数。
2. 两者的 `String()` 不同（`1.00` 与 `1`）。语义摘要按 `String()` 取值，**同一个数的两种写法进摘要是两个串**；`evaluation.valid()` 的摘要自校比的是「摘要与本图自洽」，不是「本图是规范写法」——一份非规范但自洽的快照整套通过。
3. 重建边界 `decimalFrom(decimalSnapshot)` 按字段原样构造，不解析也不规范化：非规范写法进了快照就原样回到内存。
4. **生产路径真会产出它**：`charge_dependency_execution.go` 的 `percentShare` 按字段移位（系数照抄、标度加二）不走任何规范化，系数末位为零时产出的正是尾随零那种写法。

文本入口不在此列：`ParseDecimal` 宽收各种外部写法并规范化，这一半是对的；坏的是**字段层**的构造与重建。

## 判定：真缺陷，不是留待（接管会话只读复核，锚 `28ea268b`）

它不等任何实例值，缺的是一条裁决（下面「要先裁的一格」）与两处小改，所以归 `bug` 而不归留待。上面四格之外，复核时又核了三件，写下来免得下一个人再读一遍源码：

- `NewMoney` 只查 `valid()` 与非负、不规范化，`percentShare` 产出的写法原样进 `ChargeLine.amount`。
- 语义摘要 `canonicalMoneyValue` 按 `amount.String()` 取值，写法差异**必然**进摘要，中间没有任何一层把它归一。
- 唯一会顺手规范掉它的是卡声明了逐行金额取整：`applyAmountRounding` → `RoundToIncrement` → `decimalFromBig`（去尾随零），`RoundingNone` 除外。**一条费用行的写法因此取决于卡有没有声明取整**——同一张卡内路径确定，不同卡本就是不同摘要，所以今天没有一条可达的假冲突。

今日爆炸半径为零（无生产评价、单路径确定），也正因如此甲那道一次性重算窗口的成本此刻为零——与 ADR-0014 Consequences 那句是同一笔算术。`Blocked by` 那格裁决仍是前置；本票不在 task-f31a5650 内修。

## 今天的后果

确定性今天成立——同一条路径每次产出同一种写法。但：

- 两条不同路径算出同一个数、写法不同 → 语义摘要不同 → 回放判 `REPLAY_RESULT_MISMATCH`，是假冲突。
- 若日后把 `percentShare` 改成规范化输出（一行改动），存量评价的重放摘要就变了 → 同样一批假冲突。**所以这一行不能单独改**，改它等于改语义摘要的形状。

## 要先裁的一格

收紧的代价是**语义摘要的可比性**，与 ADR-0014 对方案内容摘要的处理同族。三条路：

- (甲) 裁定「语义摘要按规范写法计算」：`percentShare` 改经 `decimalFromBig`（它会去尾随零），`decimalFrom` 重建时校验规范写法（非规范即重建拒绝，与 `ErrEvaluationSnapshotInvalid` 同格），接受一次性重算窗口——**今天无租户、无生产评价，窗口成本为零；晚做就不再是零**。
- (乙) 语义摘要也带形状版本（评价侧的 ADR-0014），旧摘要按旧形状比、新摘要按新形状比；代价是多一层要长期维护的版本分支。
- (丙) 不收紧，只把 `percentShare` 那一处规范化、`decimalFrom` 维持原样收回旧写法，接受「存量非规范快照重放会假冲突」——在无生产评价的今天等价于甲，但少了那道重建门。

倾向甲：ADR-0014 的 Consequences 自己写过「代价在此刻支付最低……晚于形成第一条真实评价再引入，就要同时处理存量摘要的归属版本」，这里是同一句话。

## 落地（裁定后）

1. `percentShare` 改走 `decimalFromBig`；
2. `decimalFrom` 加规范写法校验（或先规范化再交 `valid()` 的摘要自校兜底——两种写法各有一格，按裁定取）；
3. `decimal_canonical_rebuild_test.go` 四格改成钉「收紧后的形状」——它们现在钉的是缺口，缺口补上那天前提失效，正文里已写明「若此处变红说明 valid() 已收紧，本测试的前提要重写」；
4. 若取乙，另立 ADR。

## 边界

不动 `ParseDecimal` 的宽收语义；不动金额取整策略（ADR-0107）——取整是业务声明，规范写法是表示层纪律，两件事。
