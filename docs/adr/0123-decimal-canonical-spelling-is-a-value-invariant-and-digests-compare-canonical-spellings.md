# ADR-0123: `Decimal` 的规范写法是值对象不变量——同一个数只有一种字段写法；语义摘要与内容摘要按规范写法比；重建门拒绝非规范写法而不规范化；不换规范化版本，一次性窗口在无生产评价的今天支付

Status: Accepted
Date: 2026-09-07

## Context

`internal/parcelpricing/domain` 的 `Decimal` 是「系数文本 + 标度」两个字段，文本入口 `ParseDecimal` 宽收各种外部写法并规范化（去前导零、去尾随零、零无标度），所以从文本进来的数只有一种写法。但字段层不是：`Decimal.valid()` 拒前导零、拒「系数为零而标度非零」，**不拒标度大于零时的尾随零**，于是 `{coefficient:"100", scale:2}` 与 `{"1", 0}` 都通过 `valid()`、`Cmp` 判同一个数、`String()` 却分别是 `1.00` 与 `1`。

这件事之所以是缺陷而不是趣闻，在于三条线在票 [wiring-baseline-remainder/07](../../.scratch/wiring-baseline-remainder/issues/07-pp-decimal-rebuild-boundary-accepts-non-canonical-spellings.md) 里被钉到了一起（取证锚 `0fcbed6e`）：

- **摘要按 `String()` 取值。** `fingerprint.go` 的 `canonicalMoneyValue` 与各规范化文档里的数值字段都是 `String()`——同一个数的两种写法进摘要就是两个串。`evaluation.valid()` 的摘要自校比的是「摘要与本图自洽」，不是「本图是规范写法」，一份非规范但自洽的快照整套通过。
- **重建边界按字段原样构造。** `evaluation_snapshot.go` 的 `decimalFrom` 不解析也不规范化；评价快照、价卡登记快照、序列登记快照三条重建路径都经它。非规范写法进了快照就原样回到内存。
- **生产路径真会产出它。** `charge_dependency_execution.go` 的 `percentShare` 把「基数乘百分比」除以一百的做法是系数照抄、标度加二——系数末位为零时产出的正是尾随零那种写法；`NewMoney` 只查 `valid()` 与非负，不规范化，它就这样进了费用行。唯一会顺手规范掉它的是卡声明了逐行金额取整（`applyAmountRounding` → `RoundToIncrement` → `decimalFromBig` 去尾随零），于是**一条费用行的写法取决于卡有没有声明取整**。

今天没有一条可达的假冲突：同一条路径每次产出同一种写法，不同卡本就是不同摘要。但两条不同路径算出同一个数、写法不同，语义摘要就不同，回放判 `REPLAY_RESULT_MISMATCH`；而且 **`percentShare` 那一行不能单独改**——改它等于改存量评价的语义摘要，一批假冲突。所以要先裁，裁的是语义摘要的可比性，与 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 同族。

三条路（票面甲、乙、丙）：收紧并接受一次性重算窗口；给语义摘要加形状版本，旧摘要按旧形状比；只修产出者、重建门照旧收回旧写法。

## Decision

**一、规范写法是 `Decimal` 的值不变量，写在 `valid()` 里。** 一个 `Decimal` 立得住，当且仅当系数无前导零、系数为零时标度为零且不带负号、**标度大于零时系数无尾随零**。这样同一个数在字段层只有一种写法，`String()` 与字段一一对应。它是值对象的不变量而不是某道边界的纪律：`NewMoney`、`ChargeLine.valid()`、`Weight.valid()`、`ConversionStep.valid()` 以及三条重建门的整图重验都经 `valid()`，一处收紧、处处继承，不必在每道门上各写一遍。

**二、产出 `Decimal` 的算术一律经 `decimalFromBig` 或 `ParseDecimal`，不按字段构造。** `percentShare` 改走 `decimalFromBig`（它去尾随零）；除以十的幂仍只是小数点移位，结果仍精确、仍无需声明精度——变的只是写法，不是数。本包内再出现「系数照抄、标度改一下」的写法，`valid()` 会在它进 `NewMoney` 时就拒掉，不必等到重放。

**三、重建门拒绝非规范写法，不规范化。** `decimalFrom` 继续按字段原样构造；写法不规范的数在随后的整图 `valid()` 上使整份快照重建失败，与 `ErrEvaluationSnapshotInvalid` / `ErrPriceCardRegistrationSnapshotInvalid` 同格。不取「读回时先规范化再靠摘要自校兜底」：重建门的职责是「坏写入在重建处暴露」，一个悄悄把 `1.00` 改成 `1` 的门会把新出现的非规范产出者藏到第一次重放才露头，而那时报的是摘要不符，与真正的内容冲突长同一张脸。

**四、不换规范化版本。** ADR-0014 要求换号「必须来自规范化结构的变化」。本记录不动 `fingerprint.go` 里任何文档形状：每一个规范写法的 `Decimal` 在改前改后映射成逐字节相同的规范化文档，既有规范写法快照的摘要一个字都不变。唯一会变的是「快照里带着非规范写法的评价」——它们在新门下重建被拒，不是被按另一种形状重算出另一个摘要。这是一次性重算窗口，不是形状换号；`PPC-5` 不动。

**五、一次性窗口在今天支付，且只在今天为零。** 本记录成立的前提：仓内无租户、无生产评价、无持久化的快照夹具，本机门禁库是一次性容器——所以没有任何存量快照会在新门下被拒。ADR-0014 Consequences 那句「代价在此刻支付最低……晚于形成第一条真实评价再引入，就要同时处理存量摘要的归属版本」在这里逐字成立。若日后哪个环境的库里真有带非规范写法的存量快照，它们按本记录**被拒绝重建**，不得被就地改写成规范写法——改写等于替它们重算一个从未产生过的摘要。

**本记录不改任何计价语义。** `ParseDecimal` 的宽收不变；金额取整策略（[ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md)）不变——取整是业务声明，规范写法是表示层纪律；`Cmp`、`Add`、`Mul` 对规范写法的数逐字同答。

## Consequences

- **「同一个数两种写法进摘要是两个串」这一格关上了**：语义摘要与内容摘要从此只比规范写法，不同路径算出同一个数就是同一个串，回放不会因写法而假冲突。
- **`percentShare` 产出的费用行写法不再取决于卡有没有声明逐行取整**：`5%` 乘 `100` 无论卡声明什么，费用行金额都是 `5`。
- **重建门多拒一种坏写入**：带非规范写法的快照与内容被改的快照同格，都答「快照不合法」。它不是新的错误种类，也不为它另立错误值。
- 代价：`Decimal.valid()` 多一个条件，本包每次算术前的操作数检查都多跑它；`decimal_canonical_rebuild_test.go` 原先钉「缺口」的四格改钉「收紧后的形状」——它们自己写明过「若此处变红说明 `valid()` 已收紧，本测试的前提要重写」。
- 代价：**错过窗口的代价随第一条真实评价出现而变为非零**。若本记录晚于那一刻落地，Decision 四就不成立——那时要么换号、要么走票面乙。所以本记录与票 07 同批落地，不留待。
- 不改 CONTEXT：「版本内容摘要」词条已写「按稳定结构生成」，本记录是让那句在数字这一格成立的实现纪律，没有引入或改写任何领域语言。

## Alternatives considered

- **乙：语义摘要也带形状版本，旧摘要按旧形状比、新摘要按新形状比。** 否决：它是为「已有存量摘要」设计的处方（ADR-0014 的场合），今天没有存量；为一个不存在的存量长期维护一层按版本分支的比对路径，代价实、收益零。它仍是将来窗口错过之后的正确处方，本记录不排除它。
- **丙：只修 `percentShare`，`decimalFrom` 维持原样收回旧写法。** 否决：留着的那道口子正是缺陷本体——快照里仍可躺着一个 `valid()` 通过、`String()` 却不是规范写法的数，摘要可比性仍无保证；今天与甲等价，只是少了那道门。
- **只在 `decimalFrom` 加校验，`valid()` 不动。** 票面甲的字面写法。否决：`NewMoney` 那道门对非规范写法仍开着，下一个按字段构造的生产路径照样进费用行；而写法纪律要对值的每个持有者成立才有用。退回这条是一处改动，若 owner 认为值对象不变量的变更该另走一轮，Decision 一改一句即可。
- **读回时先规范化，再靠摘要自校兜底。** 票面「落地」第 2 条的另一格。否决理由在 Decision 三：它藏产出者、混淆两种失败。
- **顺手换号到 `PPC-6`。** 否决：文档形状没变，规范写法快照的摘要一个字不变；换号会让所有既有 `PPC-5` 快照在版本门被拒，那才是真正的存量代价，而它买到的只是一个不存在的区分。

## Links

- [票 wiring-baseline-remainder/07](../../.scratch/wiring-baseline-remainder/issues/07-pp-decimal-rebuild-boundary-accepts-non-canonical-spellings.md)：四格取证、甲乙丙三条路、裁决与越权风险点
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：换号的判据（「规范化结构的变化」）与「代价在此刻支付最低」那句
- [ADR-0107](./0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md)：金额取整策略不受本记录影响；`RoundToIncrement` 经 `decimalFromBig` 的去尾随零是本记录之前唯一顺手规范掉写法的路径
- [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)：「版本内容摘要」「规范化版本」词条与重放不变量——本记录让它们在数字写法这一格成立，不改写它们

## owner 复核记录

- owner 复核 2026-09-09 认可（用户 2026-09-09 12:3x 经 IDP 队列通道 1 授权「你自决，目标是全部解决」，通道 1 代裁，票 wiring-baseline-remainder/07「裁决」节三条逐条）：1. 收紧的是 `Decimal.valid()` 而不是只收 `decimalFrom`——规范写法是值不变量，放在值对象里是「单一权威」，放在某一道边界上是让别的持有者各守一份；2. 不换 `PPC` 号——ADR-0014「规范化结构的变化」读作文档形状，值域收窄不改任何已成立摘要的字节；3. 「今天无存量非规范快照」是前提——无租户、门禁库一次性，记录里写成前提而非事实陈述是对的。