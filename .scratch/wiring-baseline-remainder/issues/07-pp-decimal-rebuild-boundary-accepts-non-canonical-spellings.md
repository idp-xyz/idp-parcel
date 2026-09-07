# Decimal 重建门收非规范写法，`percentShare` 真会产出它：同一个数两种写法进语义摘要是两个串

Category: bug
Status: resolved——取甲，MCP-4 四笔在分支 `mcp4-wbr07`（基 main `0fcbed6e`）落地：`1b1a2980` ADR-0123 / `f1b21c47` 修复 / `223d6145` 棘轮基线 PP 段注释 / `305402f5` 清点；MCP-4 会话随后 crash（用户 16:4x 于通道 6 报），MCP-6 接管收口（分支 `mcp6-wbr07` 自 `305402f5` 切出，只加本收口笔，不动 `.go`；已知会 MCP-1），干净检出验证强度在文末「完成记录」；main 上的 SHA 待 MCP-1 重放后对照。此前：MCP-6 2026-09-07 立票，随 `ParseCanonical` 删除（分支 `mcp6-pp-ratchet` 的 `1d13d510`，main 上为 `7cef122f`）；同日接管会话只读复核判为真缺陷，MCP-1 12:2x 裁「转 ready、不并入 task-f31a5650、作下一单」；MCP-1 15:4x 派给 MCP-4（task-d3c3a7ac），16:0x 认领
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

## 裁决（MCP-4 2026-09-07，task-d3c3a7ac；按 MCP-1 派单「owner 授权自决口径」）

按 `/domain-modeling` 走过：边界是 `parcel-pricing` 一个上下文内的表示层，不跨上下文；术语用 CONTEXT 原词——「语义摘要」「版本内容摘要」「规范化版本」「重放」「冲突」；没有新词要进 CONTEXT。

**裁甲，且把「规范写法」收进 `Decimal.valid()` 本身，而不是只在 `decimalFrom` 加一道门。** 三条理由：

1. **它守的是值对象的不变量，不是某一道边界的纪律。** 「同一个数只有一种字段写法」要对 `Decimal` 的每个持有者都成立才有用：`NewMoney` 是 `percentShare` 产出物进费用行的那道门，`ChargeLine.valid()`、`Weight.valid()`、`ConversionStep.valid()` 是整图重验时逐个决定的门，评价快照、价卡登记快照、序列登记快照三条重建门最后都落在整图 `valid()` 上。规范性写进 `valid()`，这些门一个都不必改就全部继承拒绝；只改 `decimalFrom`，`NewMoney` 那道门对非规范写法仍然开着，下一个按字段构造的生产路径照样能进费用行——上面第四格讲的正是这种路径已经存在过一次。
2. **代价此刻为零，且只有此刻为零。** ADR-0014 Consequences「代价在此刻支付最低……晚于形成第一条真实评价再引入，就要同时处理存量摘要的归属版本」在这里逐字成立：仓内无租户、无生产评价；`internal/parcelpricing` 下没有持久化的快照夹具（无 `testdata`）；本机门禁库是一次性容器。乙那层版本分支要长期维护，为一个今天不存在的存量付这个价不值。
3. **丙留着的那道口子正是缺陷本体。** 丙只改 `percentShare`，重建门继续原样收回非规范写法——等于承认快照里可以躺着一个 `valid()` 通过、`String()` 却不是规范写法的数；而语义摘要按 `String()` 取值，这格一开摘要的可比性就没有保证。

**不换规范化版本（`PPC-5` 不动）。** ADR-0014 说换号「必须来自规范化结构的变化」：本裁决不动 `fingerprint.go` 里任何文档形状，每一个规范写法的 `Decimal` 在改前改后映射成逐字节相同的规范化文档；唯一会变的是「快照里带着非规范写法的评价」——它们在新门下**重建被拒**（与 `ErrEvaluationSnapshotInvalid` 同格），不是被按另一种形状重算出另一个摘要。这属票面说的「一次性重算窗口」，不是形状换号。

**`decimalFrom` 不规范化。** 「落地」第 2 条给了两种写法：校验拒绝，或先规范化再靠摘要自校兜底。取前者（经 `valid()`）：重建门的职责是「坏写入在重建处暴露」（`evaluation_snapshot.go` 头注原句）；一个在读回时悄悄把 `1.00` 改成 `1` 的门，会把生产路径上新出现的非规范产出者藏到第一次重放才露头，而且露头时报的是摘要不符，与真正的内容冲突长同一张脸。

**CONTEXT 不改。** 「版本内容摘要」词条已写「按稳定结构生成」；规范写法是让那句在数字这一格成立的实现纪律，不是新的领域语言，本裁决没有引入或改写任何术语、不变量或生命周期。

**ADR 取 0123**（MCP-1 预留号）：记「规范写法是 `Decimal` 值不变量；语义摘要与内容摘要按规范写法比；重建门拒绝而不规范化；不换 `PPC` 号；一次性窗口在无生产评价的今天支付」这五件的取舍，与被否的乙、丙。

### 越权风险点（单列，供 MCP-1 / owner 复核）

- **收紧的是 `valid()` 而不是只收 `decimalFrom`。** 派单与票面写的都是「`decimalFrom` 校验」，我把门往里推到了值对象——影响面是整个 `internal/parcelpricing/domain` 的 `Decimal` 持有者，不只重建边界。理由在上面第 1 条；若 owner 认为值对象不变量的变更该另走一轮，退回「只在 `decimalFrom` 拒绝」是一处改动，ADR-0123 Decision 一相应改一句。
- **「不换 `PPC` 号」是对 ADR-0014「规范化结构的变化」的一次解释。** 我读作「文档形状」，不含「值域收窄」。若 owner 读法不同，换号是 `canonicalizationVersion` 那一行加版本门的几处测试，ADR-0123 Decision 四要改。
- **「今天无存量非规范快照」是前提不是取证。** 仓内查得的是无 `testdata`、无租户；本机门禁库是一次性容器；任何部署环境的库我没有、也没法查。ADR-0123 里把它写成「本记录成立的前提」而不是事实陈述。

## 完成记录（2026-09-07，MCP-6 接管收口；分支 `mcp6-wbr07` = `305402f5` + 本收口笔，基线 `0fcbed6e`，不推——MCP-1 重放进 main）

MCP-4 在 `mcp4-wbr07` 上落完四笔后会话 crash，没留完成记录、没发完工报、没记验证强度；用户 16:4x 于通道 6 指令 MCP-6 继续并知会 MCP-1。取证：`mcp4-wbr07` 工作副本与其验证树 `idp-parcel-mcp4-verify`（detached `305402f5`）均干净、无未提交现场，所以本次接管**没有封存笔**——四笔都已入库，本会话只在干净检出上补验证、写本记录。四笔与本收口笔每笔 pathspec 提交：

| 笔 | SHA | 内容 |
|---|---|---|
| A | `1b1a2980` | [ADR-0123](../../../docs/adr/0123-decimal-canonical-spelling-is-a-value-invariant-and-digests-compare-canonical-spellings.md)：规范写法是 `Decimal` 值不变量、语义摘要与内容摘要按规范写法比、重建门拒绝而不规范化、不换 `PPC` 号、一次性窗口今天支付；乙、丙与「只收 `decimalFrom`」「读回先规范化」「顺手换号」五条被否；`docs/adr/README.md` 一行；本票「裁决」节与越权风险点 |
| B | `f1b21c47` | `Decimal.valid()` 加两条（标度大于零时系数不以零结尾；零不带负号）；`percentShare` 改 `decimalFromBig(product.bigCoefficient(), product.scale+2)`；`decimalFrom` 加注释说明为何按字段原样构造；`decimal.go` 里 `ParseCanonical` 那段留言改指本裁决。探针 `decimal_canonical_spelling_test.go` 两条一正一反（`TestPercentOfBasisChargeLineIsSpelledCanonically`、`TestSnapshotSpellingTheSameNumberNonCanonicallyIsRejectedAtRebuild`）；`decimal_canonical_rebuild_test.go` 原钉缺口的四格改钉收紧后的形状，逐条对照写在该笔提交信 |
| C | `223d6145` | `internal/architecture/production_wiring_baseline.txt` PP 段 `ParseCanonical` 留言追一句「票 07 已裁甲落地（ADR-0123）」，名单行与条目数不动 |
| D | `305402f5` | 机制清点在 `223d6145` 干净检出上重生成：parcelpricing 测试 83→84，合计 766→767；生产面、端口声明、端点数不变 |
| E | 本笔 | 本票转 resolved + 本记录 |

**验收对照**（票面「落地（裁定后）」逐条）：① `percentShare` 改走 `decimalFromBig` ✓（B）；② 「`decimalFrom` 加规范写法校验」——裁决取「经 `valid()`」那一格：`decimalFrom` 不动、不规范化，规范性收进值对象，三条重建门整图重验时继承拒绝 ✓（B；ADR-0123 Decision 一、三）；③ 四格改钉收紧后的形状 ✓（B），另加两条探针；④ 「若取乙另立 ADR」——取甲，ADR 仍写（五件取舍与被否路径）✓（A）。边界两条：`ParseDecimal` 宽收未动、金额取整（ADR-0107）未动 ✓——B 的 diff 只落在 `percentShare`、`valid()` 与一段注释。**越权风险点三条原样留在「裁决」节供 owner 复核，本会话未复裁**——接管的是收口，不是重裁。

**验证强度**（本会话在干净 detached 检出 `idp-parcel-mcp4-verify` @ `305402f5` 上首次记录；MCP-4 提交信 B 里记的是未设 DSN 的包级绿）：`gofmt -l .` 零输出；`go build ./...`、`go vet ./...` 退 0；**含 DSN**（门禁容器 `127.0.0.1:55432`，healthy）`go test -p 1 -count=1 ./...` 99 ok / 0 FAIL / 退 0。探针一正一反 `go test ./internal/parcelpricing/adapters/postgres/ -count=1 -v`：无 DSN `--- SKIP` 58 / `--- PASS` 0；有 DSN `--- SKIP` 0 / `--- PASS` 58。机制清点在同一检出重生成 `git status --porcelain -- docs/product/MECHANISM-INVENTORY.md` 为空（D 已是 tip 上的数）。未跑 `-race`（本机走不了，见 workflow.md 本机环境）。验证日志写在仓外 `%TEMP%\wbr07-verify\`，验证树未留任何未跟踪文件。

**parallel-sessions「接手别人在途产出先写自己第一片 red」不适用于本次**：四笔已全部入库、内容完整，本会话没有写任何实现或测试，只验不写——镜像风险在这里没有载体。

**要 MCP-1 落的装配行**：无。纯 `internal/parcelpricing/domain` 改动 + 文档。

**父 spec**：`wiring-baseline-remainder/spec.md` 状态行不由本票改，完工对齐归 MCP-1。

## Comments

- 2026-09-07 · MCP-6（接管收口）：收口。四笔是 MCP-4 会话的产出，本会话对 ADR-0123 五条 Decision 与 B 笔 diff（`percentShare` / `valid()` / `decimalFrom` 注释）逐条对照过票面「落地」四条再写完成记录；验证强度是本会话在干净检出上补测的，不是转述。分支指针 `mcp4-wbr07` 保留作出处，`mcp6-wbr07` 只比它多本笔。
