# 24 各册封闭集的 `*Named` 反查合一到 awf/12 的泛型 `closedCodeNamed`

Category: chore
Status: ready-for-agent——2026-09-09 通道 1 代裁立票（用户授权自决）：票 13 两份评审（通道 4、通道 3）与票 15 评审都点名同一条——`domain` 包里
每个封闭集各写一份「从 `String()` 原词反查码」的循环，一字同形；awf/12 已写了泛型 `closedCodeNamed[Code ~uint8]`（`publication_canonicalization_acceptance_rule_package.go`
一带），十个 `*Named` 共用它。12 / 13 / 15 都进 main 了，可以合一
Blocked by: 无

## 缺口（锚 `3d90130c`）

`internal/partycommercial/domain/` 里按 `String()` 反查码的函数至少有：`PreAcceptanceControlKindNamed` / `ControlFailureDispositionNamed` /
`JointPassConditionNamed`（13）、`SettlementMethodNamed`（15）、`PlanBindingConversionNamed` 等（14）、`CommercialObjectKindNamed`（更早），以及 12 那十个已经走
`closedCodeNamed` 的。各自写的循环形状一样：从第一个合法码迭代到 `valid()` 为止、`String()` 相等即命中、空串与集外答 false。差别只在两点：
有的集有「不可声明」的格（`ManualReviewDirectiveNamed` 走 `.Declared`、`AmendmentAllowanceNamed` 走 `.declarable`，`NOT_DECLARED` 写不进去），有的集
零值不是合法码（迭代起点不同）。

## 完成判据

1. 所有 `*Named` 反查改为调 `closedCodeNamed`（或它的一个带「可声明谓词」参数的变体，作者定形、写理由）；每个集**保留自己的导出函数名**（调用方零改动），
   函数体只剩一行委托。
2. **语义零变化**，逐集用既有测试证：空串答 false、集外答 false（含票 13 钉的 `NO_CONTROL`、票 15 钉的第三取值「客户级默认」、12 的 `NOT_DECLARED`）、
   每个合法码往返（`Named(String(c)) == c`）——缺往返测试的集补一条表驱动测试，一张表盖全部集。
3. `go test ./internal/partycommercial/...` 全 ok；`go vet` 0；无 HTTP / pgx 进领域包。
4. 完成记录列出改了哪几个函数、删了多少行（`git diff --stat` 数字），不写「N 处」到注释里。

## 边界

只动 `internal/partycommercial/domain/`；不改任何 `String()` 取值、不改任何封闭集成员；不动 http / application / postgres。
