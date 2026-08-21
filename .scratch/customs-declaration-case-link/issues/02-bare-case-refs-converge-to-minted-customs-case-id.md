# CC 存量案件引用从裸 string 收敛到铸造 CustomsCaseID

Category: enhancement
Status: ready-for-agent

[ADR-0073](../../../docs/adr/0073-declaration-unit-is-a-persisted-aggregate-holding-its-case.md)
决定六的后半：新增处一律铸造，存量收敛在本票显式跟踪。**本票存在期间，仓里两种案件引用
表达并存是已声明状态，不是无声漂移**——钉住的正是 01 票红线「不许无声地留成两种」。

## 收敛面（录于 `9e5c5c0` 取证，开工前对当时 main 复核）

01 票 Comments 第〇节列举的存量裸 `string` 案件引用：

- `FollowUpTarget.caseRef`
- `ClosureVerification.caseRef`、`CustomsCaseClosure.caseRef`、`CloseCustomsCaseCommand.CaseRef`
- `ObligationInventoryView.LoadObligationItems` 的 `caseRef` 形参
- `CaseClosureStore.FindByCase`
- 案件配置登记册五本的 `CaseRef`

## 做法约束

- 依据 [ADR-0069](../../../docs/adr/0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md)
  决定三：案件维一律用铸造 `CustomsCaseID`，范围键职责收敛为建案幂等。
- 纯类型收敛，不改任何业务判断；逐处替换后各自用例应零语义变化。
- 与 01 票的实现（新增侧）可并行，不互相阻塞；两票都完成后，仓里案件引用只剩一种表达。

## Comments

- 2026-08-21 MCP-2：随 ADR-0073 决定六开出，防「另票」落空（与
  supplier-expected-cost-correction/05 的开票动机同款）。
