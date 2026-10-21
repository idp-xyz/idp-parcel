# 54 CR 运营企业真实到账的证据引用不指向结算已采用的外部资金事实，同一笔回款可以在两处各铸一次

Category: bug
Status: ready-for-agent——R3（COD 进入试点之前必修）：COD 首发不在试点范围（`PAR-COM-11`），今天只有演示租户经受控 CLI 登记；但真实到账一旦按今天的形接真渠道，同一笔银行回款会在 settlement-accounting 的采用与 collection-remittance 的真实到账各存一份，两边各自守恒、合起来重记，只能靠线下对账发现。2026-10-21 文档自洽 10（10a 半边）立（用户「你作为业务和系统专家自决」授权），分支 claude-dc10a。
Blocked by: 无（结算一侧被引用部分怎样映射，见「要裁的」，不挡 CR 一侧的修法）
出处：[audit/cross.md](../audit/cross.md) CM·M-2；裁定 [ADR-0274](../../../docs/adr/0274-collected-principal-bank-arrival-enters-through-the-settlement-accounting-funds-adoption.md)；用例 [UC-CR-001](../../../docs/application/collection-remittance/UC-CR-001-REGISTER-COLLECTION-AND-POST-THE-SUBLEDGER.md) `AT-CR-020`。

## 现象

- collection-remittance CONTEXT「代收义务与实收事实」（ADR-0274 之后）：运营企业真实到账只经 `settlement-accounting` 的采用进入本产品，登记这一层代收事实时证据引用必须指向一条已采用的外部资金事实，按引用读回那一版在册、币种与指令相同才接受；引用同一条已采用事实的各条真实到账，金额之和不超过该事实金额。
- 代码对真实到账的证据引用不作任何核对：任意字符串都能登成`运营企业真实到账`。

## 证据

- `internal/collectionremittance/domain/collection_fact.go`：`AcceptCollectionFact` 对 `Evidence` 只核非空（`EvidenceReference.valid()`），四个来源层级同一待遇。
- `internal/collectionremittance/application/register_collection.go`：`AcceptFact` 只核指令在册，`Deps` 里没有 settlement-accounting 的读口。
- `cmd/parcel-collection-register/translate.go`：事实输入的 `evidenceRef` 是一格自由字符串。
- settlement-accounting 已有可读的口：`internal/settlementaccounting/ports/ports.go` 的 `AdoptedFundsFactView`（customs-compliance 的 `adapters/settlementaccounting/adopted_funds_fact_source.go` 就是按引用读它）。

## 三类

机制：真实到账按引用读已采用事实并核在册、币种与金额之和。产品策略：无。租户取值：哪条回款对应哪几条指令由代收运营登记，本产品不推导。

## 规模

S–M：CR 加一个消费侧适配器读 `AdoptedFundsFactView`；`AcceptFact` 对`运营企业真实到账`层按引用读回、核币种、核同一采用事实下各条到账金额之和（读回与落册同一事务）；CLI 输入把这一层的证据写作采用引用；演示种子的真实到账先在结算采用一条合成资金事实再引用它；补用例（不在册答依据不在册、币种不同答未受理、超额答内容冲突、多条按指令拆分合计不超额照登）。

## 最小的诚实修法

照 ADR-0274 决定一、二做 CR 一侧；结算一侧在 SA owner 定之前不动：被引用的那部分在 `UC-SA-005` 里停在未分配，看得见、不进运营结算余额。

## 要裁的

- 结算一侧怎样处置被代收与清分引用为真实到账的那部分（ADR-0274 越权风险点 2）：给 `UC-SA-005` 加一种「代收本金」映射目标（金额与引用它的各条真实到账之和对得上），还是保持未分配并另立对账口径。归 SA owner。
