# 02 价卡内容加「金额取整策略」并在评价里按声明点取整：ADR-0107 的实施票

Category: enhancement
Status: in-progress——MCP-6（2026-09-04，隔离分支 `mcp6-pp-renumber`，基线 main `4cc1bc34`，task-2b9edfe4 换号批五票之一）：领域半边已落 `66a90dc4`，余项与交接点见文末「交接」；裁决已落 [ADR-0107](../../../docs/adr/0107-evaluation-amount-rounding-is-declared-by-the-price-card-like-weight-rounding.md)，CONTEXT 词条「金额取整策略」已在；本票只做机制半边，不填任何模式取值与进位单位（通道 6 2026-09-04 立票，只写票面未动代码）
Blocked by: 无

## 缺口

见 [`01`](./01-evaluation-amount-scale-has-no-declared-source.md) 的取证与裁决：CONTEXT 要求金额精度由版本化规则声明，代码只落了重量那半；
ADR-0107 裁定槽落价卡内容、与重量取整同形、进内容摘要、未声明即不取整并记问题项、消费方不得在评价外取整。本票把这五条做成代码。

## 做什么

1. **领域**（`internal/parcelpricing/domain`）：
   - `RoundingMode` 封闭集扩到商业取整至少需要的模式（`HALF_UP` 起；扩几个由首份真实价卡的条款定，不预填），扩集合是领域封闭集
     改动，会打到重量取整的守卫与快照——走三步法或单独 worktree（[parallel-sessions](../../../docs/agents/parallel-sessions.md) 判据）。
   - 新值对象「金额取整策略」：模式 + 进位单位（该卡币种的 `Decimal` 金额）+ 应用点集合（封闭：合计必声明，逐行与换算后可选）；
     构造门拒空模式、非正进位单位、缺合计点。
   - 进 `PricingPlanVersion`：构造门、`plan_snapshot.go` 快照文档、`fingerprint.go` 规范化文档；`canonicalization` 按 ADR-0014 换号；
     旧规范化版本的卡照既有 `CANONICALIZATION_VERSION_UNSUPPORTED` 语义处置，既有评价按原版本重放不改写。
   - 评价：在固定顺序（逐行 → 换算后 → 合计）的**已声明**点上调 `Decimal.RoundToIncrement`；解释项记下每一次取整（点、模式、进位单位、
     取整前后值），让「可复算」对金额成立；未声明策略时评价完成并加一条结构化问题项「金额精度未声明」（形照 ADR-0105，主体是卡而不是序列，
     问题项的主体形状是否要为此扩一格，实施时对 ADR-0105 的问题项形状定，不新造第二套）。
2. **登记面**：JSON 登记口与逐字段表单（ADR-0101 决定八）各加该槽；解码器对集合外的模式词拒（`INPUT_NOT_ACCEPTED` 由领域门答，
   不在传输层 400——与 label-channel/19 登记口同一分法）。
3. **消费侧**：`settlement-accounting` SA-c 缝（`mechanism-executor-triage/06` 留下的那条 BUY 评价 → SA 入向缝）改为直接采用 `Total()`
   的 scale，不再自己取整；未声明卡的评价按问题项如实交给 SA 自己的下游。`label-channel/13` 的成本分值桥若对 scale 有任何假设一并去掉。
4. **E2 转换工具**（`tenant-implementation-01` 的 E2）：客户价卡的取整条款转进该槽，转不出的如实列「未声明：等运营确认」，不折默认。

## 红线

- **不编币种小数位表**；不给未声明的卡任何默认模式或进位单位；SYN 夹具只记 `S`。
- 既有评价不追溯改写；规范化版本换号、旧版本按原版本重放。
- 裁决落地前后消费方都不得在评价之外取整。
- 领域包不依赖 HTTP / `pgx`。

## 完成判据

- 一张声明了合计 `HALF_UP` / `0.01` 的 SYN 卡，USD 两位 × 汇率四位换算后合计落在两位小数，解释项里有那次取整；同卡去掉声明后合计仍是六位小数且评价带
  「金额精度未声明」问题项——两格各有用例，且同一评价重放语义摘要不变。
- `12.5` 与 `12.50` 写法的价表在声明合计取整后得到同一个合计。
- 规范化版本换号后，旧版本快照读回按 `CANONICALIZATION_VERSION_UNSUPPORTED` 处置的既有用例仍绿。
- SA-c 缝的用例证明它不再自己取整。
- `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库；机制清点在干净检出上重生成随笔提。

## 参照

ADR-0107；ADR-0014（规范化版本）；ADR-0105（问题项形状）；票 `01`；label-channel/13 的「不编币种小数位表」裁决；
`internal/parcelpricing/domain/{decimal,evaluation,reference_series,weight_rounding,plan,plan_snapshot,fingerprint}.go`。

## 交接（2026-09-04，通道 6，task-2b9edfe4 换号批；本会话余量将满，下一人接）

**已落（分支 `mcp6-pp-renumber`，代码笔 `66a90dc4`，其上是 ADR-0108 那笔 `b8dfc9a8`）**——「做什么」第 1 项整项：

- `RoundingMode` 加 `HALF_UP`（`roundsUp` 是两个取整内核共用的进位判据）；`AmountRoundingPolicy`（`amount_rounding.go`：模式 + 卡币种进位单位 + 应用点封闭集 `PER_LINE` / `AFTER_CONVERSION` / `TOTAL`，合计必声明，NONE 拒）；经 `PricingPlanStructures.WithAmountRounding` 进卡，`NewPricingPlanVersion` 判进位单位币种；快照 `amountRounding`、规范化 `amount_rounding`（`omitempty`，PPC-5 之内不再换号——换号已在 `b8dfc9a8` 一次做掉，理由在 `canonicalizationVersion` 注释）。
- 评价：`roundLine` 是逐行那一点的唯一入口（基础运费、固定规则、附加费三处，先于百分比依据登记）；换算后与合计各一处；`AmountRoundingStep` 留痕进解释项、快照与语义摘要；`validCompletedCharges` 沿留痕重走费用行之和到合计；未声明 → `AMOUNT_PRECISION_UNDECLARED`（合计被收回时撤下）。
- 用例在 `amount_rounding_test.go`：完成判据里「USD × 四位汇率 → 两位合计 + 留痕」「去掉声明 → 精确 + 问题项」「两格重放语义摘要不变」「`12.5` / `12.50` 同合计」都有；另有三点先后、摘要与快照往返、构造门、HALF_UP 内核。**Decimal 算术结果是规范形（去尾零）**，`12.50` 写成 `12.5`——消费方要的 scale 从取整留痕的进位单位取，不从合计的字面取。

**未做（第 2–4 项），交接点**：

- 第 2 项登记面：JSON 登记口走价卡快照，`amountRounding` 槽已随快照进来（`plan_snapshot.go` 的 `amountRoundingSnapshot`），解码器对集合外模式由领域门拒；**逐字段表单**（ADR-0101 决定八，`apps/admin-web/src/pages/pricing/` 的价卡表单若有）尚未加该槽——先核有没有价卡逐字段表单，有就照序列表单的做法加三格（模式下拉只列封闭集、进位单位一格、应用点多选且合计锁定）。
- 第 3 项消费侧：`internal/settlementaccounting/adapters/parcelpricing/` **今天不存在**（`ports.BuyEvaluationView` 注释此前说「等取整槽落地」，本批已把那句改成「槽已到、适配器仍留空」）。写它时：`Total()` 的精度依据是 `AmountRounding()` 里 `TOTAL` 那步的进位单位；评价带 `AMOUNT_PRECISION_UNDECLARED` 时拒或原样保全十进制，不得补取整。`internal/parcelshipment/adapters/parcelpricing/estimation_amount.go` 核过：它按调用方声明的 `MinorDigits` 折最小币单位，**超位即拒不取整**，没有要去掉的假设。
- 第 4 项 E2 转换工具：未动；转不出的取整条款如实列「未声明：等运营确认」。

## Comments

- 2026-09-04 · 通道 6：立票。票 `01` 自定的 resolved 判据是「裁决引用在 CONTEXT」，故裁决与实施分票；本票承接实施。**只写票面，未动代码。**
- 2026-09-04 · 通道 6：领域半边落 `66a90dc4`；余项见「交接」。
