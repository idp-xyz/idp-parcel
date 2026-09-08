# 生产接线棘轮基线余项：九条零调用点导出工厂的逐条处置

Category: chore
Status: in-progress——PP 四条已在本目录第一笔处置（三删一留，分支 `mcp6-pp-ratchet` 的 `1d13d510`，进 main 为 `7cef122f`，2026-09-07 16:00）；**PC 三条 03/04/05 全部 resolved 并进 main（2026-09-08：05 剪 `1ebd7e4b`；04 并入 Authorize `d41862bd`；03 接上 + ADR-0127，进 main 的 SHA 见票 03「进 main 记录」）——`ValidateBeforeDecision`、`ManualReviewRequirementFor`、`ResolveCreditPolicy` 三条目都已出名单，成因与取证在基线头注**；PS 二条 01/02 仍是只读取证 draft，PS 地盘待派；PP 两张后继票：07 已 resolved（ADR-0123，MCP-4 四笔 + MCP-6 收口，2026-09-07），06 记的是留待留下的那件事

## 来源

- [`completion-assessment-2026-09-04`](../completion-assessment-2026-09-04.md) 剩余机制缺口第 2 项：棘轮基线九条零生产调用点的导出工厂（PS 2、PC 3、PP 4）。
- [`mechanism-executor-triage/spec.md`](../mechanism-executor-triage/spec.md)「处置裁决」第 2 条余项：PP 平行第二写法删、`ParseCanonical` 二选一——该目录 resolved 时自注「应立而未立」。
- 2026-09-02 那次裁决把「零生产调用点的导出领域工厂」计入机制差量；`internal/architecture/production_wiring_baseline.txt` 头注是纪律（剪一条前三分成因、记数钉 SHA 两法同得、理由行写「谁是调用方、那一层何时落」）。
- MCP-1 2026-09-07 派单（task `2fd68c18`），owner 授权自决口径。

## 取证锚

`2efef58e`（远端 main tip；与派单写的 `ffa6bd0e` 只差一笔 `tasks.md`）。基线在该检出上两法同得 **9** 条。下表的「今天」都指这一刻；名单是活的，读表前先看条目还在不在基线里。

## 逐条

| 条目 | 上下文 | 三分 | 落点 | 票 |
|---|---|---|---|---|
| `AssessSafeHandoff` | PS | 支路未接（缺出向缝 + 编排步） | 只读取证，PS 地盘 MCP-2 | [01](./issues/01-ps-safe-handoff-is-assessed-nowhere-because-nothing-hands-over.md) |
| `CurrentPayloadCanonicalizationVersion` | PS | 二选一：删，或理由行改指 PSC-2 | 只读取证，PS 地盘 MCP-2 | [02](./issues/02-ps-canonicalization-version-exit-has-no-consumer-the-prefix-already-carries-it.md) |
| `ResolveCreditPolicy` | PC | 支路未接（缺 PC→SA 授信额度缝） | 只读取证，PC 地盘 MCP-3 | [03](./issues/03-pc-credit-basis-is-never-asked-for-the-pc-to-sa-seam-does-not-exist.md) |
| `ManualReviewRequirementFor` | PC | 支路未接，先裁「谁有权」命名 | 只读取证，PC 地盘 MCP-3 | [04](./issues/04-pc-manual-review-predicate-answers-who-may-not-whether-and-nobody-asks-either.md) |
| `ValidateBeforeDecision` | PC | 死码（被闭包形态取代） | 只读取证，PC 地盘 MCP-3 | [05](./issues/05-pc-single-basis-revalidation-was-superseded-by-the-closure-form.md) |
| `ReplayPricingEvaluation` | PP | 有意留待（调用方 PN-08 W02，三件缺口） | 理由行已改写；执行器另立 | [06](./issues/06-pp-replay-pricing-evaluation-has-no-executor.md) |
| `MarshalPricingPlanSnapshot` / `RehydratePricingPlanSnapshot` | PP | 死码（平行第二写法），已删 | `1d13d510` | — |
| `ParseCanonical` | PP | 死码（守卫接不到它自称的边界），已删 | `1d13d510`；它自称要守的事已在 07 落地（规范写法进 `valid()`，ADR-0123） | [07](./issues/07-pp-decimal-rebuild-boundary-accepts-non-canonical-spellings.md)（resolved） |

## 边界

- 本目录对 PS/PC 五条**只读取证、不改代码、不改基线**：PS 地盘此刻是 MCP-2（ftr/10），PC 地盘是 MCP-3（pc-gaps/07）。票面写清应该的调用方、UC 步、缺哪一层、能否归已认可留待；先接哪张归 MCP-1 派。
- PP 四条的代码与基线改动在隔离分支上，不碰共享树；重放进 main 时基线文件按「共享文件：占号、逐块核」纪律。
- 「已认可留待」指 r27 交用户认可的那份清单（SA 三口目录读口 + PS `PAR-COM-13`、`BD-PS-009`、VE 真实渠道凭证、外部标识关系子域、NR `PAR-NET-14`）。五条 PS/PC 里没有一条能整条归进去——每一条缺的都是缝或执行器，不是实例值。

## Comments

**2026-09-07 接管复核与验证记录（MCP-6 新会话，task-f31a5650；原会话在 `28ea268b` 之后中断，未留完工报）。**

分支 `mcp6-pp-ratchet`（基于 main `0f84c0ec`；`0f84c0ec..08f54867` 纯 `.md`，未 rebase）：`1d13d510` PP 三删一留 → `28ea268b` 本目录立票 → `2837718d` 接管复核（票 02 补 UC 步、票 07 补判定、基线头注 PP 段补 `08f54867` 重量）→ 本笔（只此评论）。

- 基线计数两法同得（UTF-8 逐行滤非空非注释 / 字节层数行首非 `#`）：`0f84c0ec` 9、`08f54867` 9（该文件 blob 与 `0f84c0ec` 逐字节同）、`1d13d510` 6、`28ea268b` 6、`2837718d` 6。
- 门禁：`TestWiringBaselineHasNoStaleEntry` 在分支上 PASS。
- 全仓验证（detached 干净检出 `2837718d`，含 DSN）：`gofmt -l` 无输出、`go build` / `go vet` 退 0、`go test -p 1 -count=1 ./...` 退 0，`FAIL` 裸子串零命中。探针一正一反：`internal/parcelpricing/adapters/postgres` DSN 已设 `--- PASS` 58 / `--- SKIP` 0（17.8s），DSN 未设 `--- PASS` 0 / `--- SKIP` 58（0.016s），两次退出码都是 0、包行都是 `ok`；`internal/architecture` 两种设置下 `--- PASS` 157 / `--- SKIP` 0。
- 机制清点：在 `2837718d` 干净检出上重跑生成器，`docs/product/MECHANISM-INVENTORY.md` 零差异（生成器数的是端点、消费适配器与路由表，本分支没动那些面），**不另成笔**。
- 1d13d510 九个测试文件的复核结论见 `2837718d` 提交信；票 07 的「真缺陷非留待」判定见票面。

**同日 MCP-1 对现场报告的三句裁定及落法：** ① PS/PC 五行理由行不合规但不在 MCP-6 地盘——「该写成什么」逐行写进 01–05 各票的「完成判据」，落地那笔连理由行一起剪或改；② 票 07 转 `ready-for-agent`、不并入 task-f31a5650，作 MCP-6 下一单；③ 1d13d510 两处测试偏离逐条点名（用例、原断言、现断言、为何不丢覆盖）写进完工报前那笔的提交信，不只留在测试注释里。
