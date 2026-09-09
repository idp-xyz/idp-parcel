# 24 各册封闭集的 `*Named` 反查合一到 awf/12 的泛型 `closedCodeNamed`

Category: chore
Status: resolved——2026-09-09 14:4x 通道 6 交付（接续单 task-bb38aeda，原单 task-057b6dc3；分支 `mcp6-awf24`，代码 tip `60cdca52`，基 `74ef0da8`，逐笔 SHA 与验证强度见文末「完成记录」；main 上的 SHA 与非作者评审结论待通道 1 重放后补「进 main 记录」）。完成判据四条：九个 `*Named` 换成一行委托、导出名不变；一张表驱动测试盖二十个集的往返与集外拒收（重构前先绿）；PC 包组 + 反向依赖 + architecture 全 ok、vet 0；改了什么见完成记录。**一处按地盘留下**：`CommercialObjectKindNamed` 在 `publication_canonicalization.go`（通道 3 awf/18 在途地盘），函数体本票不换，行为已进表。此前 in-progress——2026-09-09 通道 6 认领（分支 `mcp6-awf24`，基 `74ef0da8`）。原 ready-for-agent——2026-09-09 通道 1 代裁立票（用户授权自决）：票 13 两份评审（通道 4、通道 3）与票 15 评审都点名同一条——`domain` 包里
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

## 完成记录（2026-09-09，通道 6；分支 `mcp6-awf24`，基 `74ef0da8`；接续单 task-bb38aeda，原单 task-057b6dc3 因 13:0x 全通道会话中断结 failed，从 `cc22b9b0` 接着做）

**逐笔（分支 SHA；main 上的 SHA 由进 main 记录补）**：

| SHA | 内容 |
|---|---|
| `cc22b9b0` | 票面转 in-progress（前一任会话的认领笔，代码一行未动） |
| `363544ef` | `closed_set_named_test.go`：一张表驱动测试盖二十个集——成员逐个往返 `Named(String(c)) == c`、空串与集外答 `(零值, false)`。重构前先跑：全绿 |
| `60cdca52` | 九个 `*Named` 函数体换成一行 `closedCodeNamed(...)` 委托；`TaxDisposition` 补 `valid()`；授权规则那份去掉 `math` 导入 |
| 本笔 | 票面完成记录 + Status resolved |

**改了哪几个函数（`git diff --stat 363544ef 60cdca52`：6 files, +26 / −66）**：

| 文件 | 函数 | 接受判据 |
|---|---|---|
| `publication_canonicalization_settlement_policy.go` | `SettlementMethodNamed` | `SettlementMethod.valid` |
| `publication_canonicalization_pre_acceptance_financial_control_policy.go` | `PreAcceptanceControlKindNamed` / `ControlFailureDispositionNamed` / `JointPassConditionNamed` | 各自 `valid` |
| `publication_canonicalization_price_policy.go` | `PriceDirectionNamed` / `PlanBindingConversionNamed` / `TaxDispositionNamed` | 各自 `valid`（`TaxDisposition.valid` 本笔新加） |
| `publication_canonicalization_authorization_rule.go` | `DeclaredCancellationPartyNamed` | `DeclaredCancellationParty.valid`（原本已自己扫值域，只是没走共用的那份） |
| `pre_acceptance_control.go` | `PreAcceptanceControlRequirementNamed` | `PreAcceptanceControlRequirement.Declared`（「未声明」零值写不进去） |
| `pricing_caliber.go` | 新加 `TaxDisposition.valid()` | 边界 `TaxInclusive..TaxNotApplicable`，与原反查里手写的区间一字不差；`NewTaxCaliber` 的 default 分支不改（那里连着分类一起判） |

**为什么不需要「可声明谓词」变体**：`closedCodeNamed[Code ~uint8](accepts, name, raw)` 的第一个参数就是接受判据。12 那十个已经分别传 `valid` / `Declared` / `declarable`，本票只是让其余各集也传自己的那一个——「可声明」不是第二种函数，是第一个参数的一种取值。

**语义零变化的证法**：所有集落空时返回值原本就是各自的零值常量（`*Invalid` / `PlanBindingConversionNone` / `PreAcceptanceControlUndeclared`），与 `closedCodeNamed` 的 `Code(0)` 同一个值；表驱动测试连这一格也钉了（`refuses` 断言 `got == 0`）。旧循环从首个合法码扫到 `valid()` 为假为止，新的扫整个 uint8 值域按接受判据认——各集的接受判据都是连续区间或显式等值列表，且非成员的 `String()` 一律为空串而空串在入口就拒，所以两种扫法接受的集合相同。集外词钉的是各票裁过的：13 的 `NO_CONTROL`、15 的 `CUSTOMER_DEFAULT`、12 的 `UNDECLARED` / `NOT_DECLARED`、14 的空转换（不代填 `NONE`）与空税务口径（不落进不适用）。

**按地盘留下的一处**：`CommercialObjectKindNamed`（`publication_canonicalization.go`）。派单明写不碰该文件——通道 3 在 `mcp3-awf21` 做 awf/21 → awf/18，到 18 会在同一文件加客户服务规则册一格。它的行为已进表驱动测试（十个成员往返、集外拒收），换函数体只是把循环换成 `closedCodeNamed(CommercialObjectKind.valid, CommercialObjectKind.String, name)` 一行，谁下次碰那份文件顺手带上即可；通道 3 为 18 新加的 `*Named` 由它自己直接用 `closedCodeNamed`。

**验证强度（钉 `60cdca52`，在分支树 `D:/tops/idp-parcel-mcp6-awf24` 上跑；树干净、无未提交改动，等价于干净检出）**：`gofmt -l internal/partycommercial/` 空；`go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./internal/partycommercial/... ./internal/architecture/...` 全 ok；反向依赖（`go list` 反查 `internal/partycommercial/domain`：`cmd/parcel-api` / `parcel-commercial` / `parcel-dispatch`、PS postgres、PC postgres、NR / PS / SA / TF / VE 各自的 partycommercial 适配器）**带 DSN `-p 1 -count=1` 全 ok**（PC postgres 61.6s、PS postgres 45.2s——真跑不是跳过；`cmd/parcel-commercial` 的 `TestAPublishedPreAcceptanceFinancialControlPolicyIsReadBackByTheContentView` 单跑 `-v` 是 PASS 不是 SKIP）。通道 5 的量数窗口 14:16–14:21 已关（14:18 广播）后才跑带 DSN 的包。领域包无 HTTP / pgx 导入进入（`go vet` 与 architecture 门禁未报）。证据层级 **S**（纯重构，无实例参数）。

**未做（不在本票）**：`CommercialObjectKindNamed` 的一行委托（理由见上）；`NewTaxCaliber` 的 default 分支改成调 `valid()`（那里的判连着分类，改它是另一件事）。
