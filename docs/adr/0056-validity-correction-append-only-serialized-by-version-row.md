# ADR-0056: 区间更正只增多条，同版本写入以版本行锁串行化

Status: Accepted  
Date: 2026-08-18

## Context

[ADR-0038](./0038-validity-correction-keeps-original-and-relationship.md) 把有效性更正定为登记册上的独立事实：不改写原版本键下的正文、批准与原区间，接纳更正必须使该范围的 `ViewRevision` 变化。它没有回答同一版本上第二次更正怎么落。

内存登记册原先撞键覆盖。D-5 裁定「更正一条更正」合法，正解是两侧同为只增：一个版本可有多条，选用区间取登记顺序上的最后一条。覆盖按缺陷修掉。

只增需要一条登记序轴。`corrected_at` 是更正事实自己的时间，调用方可以填得比前一条更早，不能拿来当「最后」。`registered_at` 在同一微秒内可撞。于是落库用 `registration_id`（`bigserial`）作内部序轴。

序轴本身不够。PostgreSQL 在 `INSERT` 时分配 identity，分配序不是事务提交序：T1 先取到较小的 id 后迟提交，T2 取到较大的 id 先提交，T1 后来成功接纳却永远不是生效的「最后一条」。这与「接纳新更正」的直觉和 D-5 的最后一条语义不够稳，不能以「管理命令不是高并发路径」代替不变量。

## Decision

**一、同一商业版本可有多条区间更正，只增不改写。** 同内容重放不追加；不同内容是合法的下一条。没有「内容冲突」格——异更正不是冲突。选用区间取登记顺序上的最后一条；`ViewRevision` 覆盖全部更正历史。

**二、`registration_id` 只是内部序轴，不是领域标识。** 它不出现在 `ValidityCorrection` 上，不参与跨上下文协议。装载按它保序；调用方看见的是更正事实本身。

**三、同一版本的更正写入先锁 `commercial_version` 对应行，再分配 `registration_id`。** 锁与 `INSERT` 在同一条语句、同一事务语义内（`SELECT … FOR UPDATE` 的 CTE 喂给 `INSERT … SELECT`），兼容调用方已有事务。成功接纳的序列因此与串行化顺序一致：后接纳者的 `registration_id` 更大，装载后生效。版本不在册则明确拒绝，不把空锁误译成重放。

**四、`corrected_at` 不作排序键。** 它是外部源那次更正的事实时间，保全在行上供重建，不决定谁是最后一条。

**五、并发顺序不得靠提交时钟或「不是高并发」来猜。** 不变量由版本行锁交付，不由注释里的负载假设交付。

## Consequences

- 同版本两条不同更正并发写入时，后写者停在版本行锁上，直到先写者提交或回滚；后接纳者成为选用更正。
- 先写者回滚：其行不在册，不占「最后一条」；序列可能有空洞，空洞不是登记顺序的一部分。
- 重放撞内容唯一键，答`已登记`，不追加，不改变最后一条。
- [ADR-0038](./0038-validity-correction-keeps-original-and-relationship.md) 各条不变；本记录补的是条数与写入串行化，不是更正与改正文的分界。

## Alternatives considered

- **只按 `registration_id` 分配序取最后，不锁版本行。** 否决：见 Context 的 T1/T2 反例。分配序与成功接纳序可以分家。
- **按 `corrected_at` 取最后。** 否决：那是外部事实时间，不是登记顺序；D-5 要的是后者。
- **按 `registered_at` 取最后。** 否决：时钟可并列，并列时「最后」无定义。
- **用咨询锁或应用层队列代替版本行锁。** 否决：更正外键已经指向那一行，锁它就把「版本必须在册」和「同版本串行化」收在同一处；另立一把锁是第二处口径。

## Links

- [ADR-0038](./0038-validity-correction-keeps-original-and-relationship.md)：更正保留原版本与更正关系；本记录补充条数与写入串行化
- [UC-PC-001](../application/party-commercial/UC-PC-001-MAINTAIN-AND-PUBLISH-COMMERCIAL-AUTHORITY.md)：`AT-PC-013`
