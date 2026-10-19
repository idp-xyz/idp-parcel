# 54 发生项登记不分协议核对的确定性失败：词表走岔与坏数据都答成可重试的未决

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [29](./29-supplier-agreements-declare-charge-occurrence-conditions.md) 的隔离评审（非阻断项，评审未要求进 main 前修）。号接分支所见最大号 53 之后；同波别的票若也取 54，合批时改号。
Blocked by: 无
出处：[ADR-0029](../../../docs/adr/0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)（失败按恢复动作分格）；[ADR-0207](../../../docs/adr/0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 与 [ADR-0209](../../../docs/adr/0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 的协议核对一步。

## 现象

运输收费发生项登记（`POST` 发生项登记口）核协议时，`CheckSupplierAgreement` 交回任何错误，用例一律答 `UNDECIDED` / `SUPPLIER_AGREEMENT_UNAVAILABLE` 并给续办引用，叫登记方原样重试。其中有两类重试永远不会变：

- 消费侧适配器把 PC 的答复译成 TF 的词时失败（`ErrUntranslatableAnswer`）：法人、快照、供应商立不起来，或票 29 新加的收费发生条件里原因词、数量形不在 TF 的封闭集里。适配器注释自己写着「两边词表走岔了，确定性的」。
- PC 库里那一版的条件行坏了，折回失败。

两者都被说成「依赖读不动、重试」。登记方照续办引用重交，每次得到同一个未决，看不出是产品缺陷。眼下两边的封闭集逐字相同，实际碰不到；哪一边单独加了一个原因或数量形，就会出现。

## 证据（读码）

- `internal/transportfulfillment/application/register_charge_occurrence.go` 的 `Register`：`CheckSupplierAgreement` 返回 err 即 `chargeUndecided(ChargeSupplierAgreementUnavailable, …)`。同一函数里数量形 `Measure` 失败、核对口交回集外一格都响亮上抛，两种处理并存。
- `internal/transportfulfillment/adapters/partycommercial/supplier_agreement_source.go`：`chargeConditionsOf` 与外壳翻译都包 `ErrUntranslatableAnswer`；PC 读口的错误原样包一层交回。
- 这个哨兵在适配器包里，应用层不能 import，所以今天用例想分也分不出来。

## 三类

机制（失败按恢复动作分格）。规模 S。

## 最小的诚实修法

在 TF `ports` 立一枚「协议核对答复不可译」的确定性哨兵，适配器的 `ErrUntranslatableAnswer` 包住它；`Register` 见到它整笔交回 error（装配点回滚，线上答 500），与 `Measure` 失败同样处理，不给续办引用。PC 库面坏数据要分出来，得 PC 读口先有自己的确定性哨兵，可以并在本票，也可以另拆。

## 要裁的

- TF 其余经 `SupplierAgreementSource` 核协议的路径（今天是 `commission_transport.go` 的委托提交；在途票若让订舱也接这一口，同）要不要同票一起分。归 TF owner。
- PC 读口坏数据的哨兵归 PC owner。

## 既有票

- [39](./39-buy-evaluation-request-handoff-does-not-separate-deterministic-rejection.md)、[41](./41-transport-commission-handoff-does-not-separate-deterministic-rejection.md)：交接口的同类问题。
