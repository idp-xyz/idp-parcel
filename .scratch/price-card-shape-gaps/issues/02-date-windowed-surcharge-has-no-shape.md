# 只在日期窗内生效、金额逐周变的附加费（PSS / 高峰附加费）没有形状

Category: enhancement
Status: resolved——MCP-3 实施完成（2026-09-04，隔离分支 `mcp3-pp-shapegaps`，基线 main `ae7b4c8a`，task-de0159af 换号批续作四票之一；分支 SHA 见文末「完成记录」，main 上的 SHA 待 MCP-1 重放后对照）；此前 MCP-6 领了未开工（2026-09-04，隔离分支 `mcp6-pp-renumber`，基线 main `4cc1bc34`，task-2b9edfe4 换号批五票之一；交接点见文末 Comments）；已裁改法 2，落文 [ADR-0110](../../../docs/adr/0110-date-windowed-and-periodically-published-surcharge-amounts-are-a-third-reference-series-kind.md)（2026-09-04，通道 6，owner 授权）；CONTEXT「计价参考序列」词条已改口；本票自定的 resolved 判据「ADR 编号落进某一条并被引用」已满足，按派单口径转为实施票承接，范围见「裁决」节末段
Blocked by: 无

## 为什么立

[E1 形状核对](../report.md) 第 16 项。末端渠道的高峰附加费（Peak / Demand Surcharge）有两个特征：**只在某个日期窗内生效**（如每年十月至次年一月），以及**金额按周或按段公布、逐段不同**。今天的模型（核于 `a17bfac`）三处都装不下：

| 形状 | 为什么装不下 |
|---|---|
| `TriggerCondition` / `FeatureSource` | 十项可判定量全是包裹的几何量、重量与三个类别量，**没有日期或计价基准时点**；条件读不到「今天在窗内」 |
| `ReferenceSeriesKind` | 只有 `FUEL_RATE` 与 `EXCHANGE_RATE`，且序列取值只经 `NewSeriesRateSurcharge` 作**百分比费率**用；PSS 是**定额**（每件若干美元）或**分区分档定额**，不是基数百分比 |
| `EffectivePeriod` | 是整张方案与价表的有效期，不是某一条规则的 |

**今天唯一能表达的**是把方案切成三个版本（窗前 / 窗内 / 窗后），窗内那版多一条定额附加费；金额逐周变则每周再发一版。可行但代价与 ADR-0099 解决燃油率时否掉的那条路一模一样：「序列每出一版，引用它的每张卡都要重登」。

## 三条改法（互斥，要 ADR 裁一条）

1. **版本切分，不改模型**：把「按日期窗生效」读成方案版本的有效期问题，运营按窗口发版。代价：逐周金额 = 逐周发版；六家渠道 × 若干周 = 大量方案版本，且每版内容摘要不同，重放与对账要跨版本看。优点：零形状改动。
2. **序列种类扩充 + 序列定额计算**：新增一种参考序列种类（按期次公布的**金额**而不是费率），附加费计算多一种「取序列定额」（或让 `FIXED_AMOUNT` 的金额可来自序列）。窗口本身由序列期次表达——窗外无期次即 `REFERENCE_SERIES_UNRESOLVED`，评价照既有原因码挂起。代价：`ReferenceSeriesKind` 闭合集合与 `SurchargeCalculation` 的规范化形状改动（`canonicalization` 换号）；CONTEXT「计价参考序列」词条要扩到金额。优点：与燃油率同一套登记/复核/在用机制，逐周金额不重登卡。
3. **判定谓词加时点特征**：`FeatureSource` 加「计价基准时点」一项，配区间比较，PSS 写成「时点 ∈ [起, 止] → 定额」。代价：CONTEXT「特征」闭合集合改动进内容摘要；逐周金额仍要多条规则或多版。优点：不引入新种类。

倾向（不是裁决）：2。理由是 PSS 的本质是「承运商按期公布的外部数值」，与燃油率同族，只差取值是金额；1 把运营负担推回逐周重登，正是 ADR-0099 已经否过的形状；3 解决了窗口却解决不了逐周金额。

## 红线

- 不写任何真实 PSS 窗口与金额进仓；SYN 夹具只记 `S`。
- 裁定前 E2 转换工具**不把 PSS 折进基础运费或写成常年附加费**，如实列「未转换：等本票」。
- 若取 2 或 3，规范化版本按 ADR-0014 换号，旧评价按原版本重放。

## 裁决（2026-09-04，通道 6，task-f530ad56 裁决批口径：owner 授权自决，写明能力边界）

**取改法 2，落文 [ADR-0110](../../../docs/adr/0110-date-windowed-and-periodically-published-surcharge-amounts-are-a-third-reference-series-kind.md)。** PSS 的本质是承运商按期公布的外部数值，与燃油率同族只差取值是金额；1 把运营负担推回逐周重登（ADR-0099 已否过的形状），3 解决了窗口解决不了逐周金额、还把时点混进「包裹的可判定量」集合。ADR 五条：`ReferenceSeriesKind` 加「按期公布金额」，取值带币种、币种须与方案一致；附加费计算加「取当期序列定额」，定额不带基数依赖；窗口由期次表达、不加时点特征，**但绑金额序列的规则必须在卡上声明窗外行为（不计收 / 待判断）**——PSS 窗外是正当的零而不是数据缺口，零只在登记出来时才是零（label-channel/13 立场）；分区分档不新造形状，每分区一条规则各绑一条序列 + 既有 `FeatureZone`；规范化换号一次，与 ADR-0107 / 0108 / 0109 同期实施合并为一次。本裁决在票面三条改法之上**多加了一格**（窗外行为声明），理由写在 ADR 决定三与 Alternatives 末条。

**能力边界**：读了本票与 `report.md` 第 16 项的取证、ADR-0099 全文、PP CONTEXT「计价参考序列」「特征」「判定条件」词条、`NewSeriesRateSurcharge` 与 `ReferenceSeriesBinding` 的符号面；**未读** `SurchargeCalculation` 各种类在 `fingerprint.go` 里的规范化写法与 `reference_series.go` 的期次解析路径——「窗外无期次」在解析口上今天怎么答，实施开工时先核；未打开任何客户 PSS 表。

### 本票转实施票，范围

1. 领域：`ReferenceSeriesKind` 加金额种类（期次取值改为「费率或带币种金额」的判别形状，构造门按种类拒错类型）；`SurchargeCalculation` 加「取当期序列定额」并带「窗外行为」两格封闭声明；规范化换号（同期合并）；评价解释项记「取自序列 X 第 N 期」；解析无期次时按窗外行为分流——不计收即该规则不形成费用行并在解释项留痕，待判断即 `REFERENCE_SERIES_UNRESOLVED`。
2. 登记面：序列登记 spec / 逐字段表单 / JSON 口支持金额期次；卡的附加费规则表单加该计算种类与窗外行为。
3. E2：客户 PSS 表转成金额序列登记快照 + 每分区一条附加费规则；裁定落地前如实列「未转换：等本票」。
4. 领域封闭集与快照改动走三步法或单独 worktree，频道占号。

## 验证

ADR 接受后按所选改法另拆实施票；本票 `resolved` 的判据是 ADR 编号落进上面某一条并被引用。

## 完成记录（2026-09-04，MCP-3，分支 `mcp3-pp-shapegaps`，基线 main `ae7b4c8a`）

分支上两笔（main 上的 SHA 由 MCP-1 重放后在 Comments 补对照）：

- `4c5d8938`（领域）：`ReferenceSeriesKind` 加 `PUBLISHED_AMOUNT`，取值带币种（`NewPublishedAmountSeriesValue`），登记 spec 加 `Currency`（金额序列必备、费率序列拒），PRS-2 内 omitempty；`ChargeMethodSeriesAmount` + `NewSeriesAmountSurcharge(seriesID, outOfWindow)`，`OutOfWindowBehaviour` 封闭两格（NOT_CHARGED / PENDING）、未声明不立；规则指名的金额序列必须已绑定；金额序列按标识引用、同种可绑多条（费率序列仍每种一条）；「窗外无期次」以缺席读数（`NewAbsentSeriesReading`）冻结进输入——不计收即规则不成行、解释留痕、不进互斥竞争，待判断即 `REFERENCE_SERIES_UNRESOLVED`，根本没有读数不论声明都待判断；金额币种与方案不一致 → `REFERENCE_SERIES_CURRENCY_MISMATCH` 冲突；解释记「取自序列 X@V」（期次以「在用版本 + 基准时点」定位，没有另编期号）；规范化 `series_id` / `out_of_window` / `currency` / `absent` 均 omitempty，PPC-5 不换号。
- `d7f33d9b`（登记面与编排）：迁移 **0006** 把 `reference_series_version.kind` 的库层封闭集扩到 PUBLISHED_AMOUNT；序列登记载荷 `currency`；编排按（种类，标识）对绑定与读数，在用版本无期次时对金额序列冻结缺席读数；管理台序列表单多一格「按期公布金额」与币种输入（tsc 退 0、run-tests 67/67）。

**范围第 2 项里「卡的附加费规则表单加该计算种类与窗外行为」**：管理台今天没有价卡逐字段表单（八价卡按 ADR-0101 决定八是模板导入 / JSON 快照口），两格随价卡快照的 `surchargeRules[].calculation.seriesId` / `outOfWindow` 进来，没有表单可加。**第 3 项（E2）**：E2 未开工，落地后客户 PSS 表应转成 PUBLISHED_AMOUNT 序列登记（每分区一条）+ 每分区一条 SERIES_AMOUNT 规则并声明窗外行为；本票不替它写。

**给 MCP-1 / owner 的一格**：CONTEXT「计算方法」词条仍写「只有定额、查表、按基数百分比，以及取较大值」四种，ADR-0110 Decision 二加了第五种「取当期序列定额」——`docs/domain` 不在本批地盘，词条那句请随 spec 对齐一起改（AGENTS「改文档」）。

验证：分支 tip 上 `gofmt -l` 空、build / vet 退 0、`go test -count=1 ./...` 绿；真库 `TestPublishedAmountSeriesRegistersAndResolvesWithItsCurrency` 带 DSN PASS（0006 的 CHECK 放行）。

## Comments

- 2026-09-04 · MCP-1：立票。起因是 E1 核对第 16 项。**只写票面，未动代码。**
- 2026-09-04 · MCP-3：实施完成，见「完成记录」。裁决能力边界点名的那一问——「窗外无期次」在解析口今天怎么答——核过：`ReferenceSeriesRegistration.ResolveAt` 对不在任何期次内的时点答「未解析」，编排此前只留一条说明、不给读数，纯函数据以落待判断；本票没有改那个答法，而是在编排层对金额序列多加一格「查过这一版、无期次」的缺席读数，让窗外行为在纯函数里按卡的声明分流且可重放。
- 2026-09-04 · 通道 6：换号批里领了本票，**一行未写**。交接点：PPC-5 已在 `b8dfc9a8` 换好、本票的规范化改动落同一号不再换（`fingerprint.go` 注释已预告）；序列登记快照族已是 PRS-2（同笔），期次取值若加「带币种金额」判别形状，新字段 `omitempty` 可留在 PRS-2 内，改既有字段含义才需 PRS-3；「窗外无期次」在解析口今天怎么答先核 `reference_series.go` 的期次解析路径（裁决能力边界点名未读）；`SurchargeCalculation` 的规范化写法在 `fingerprint.go` `canonicalSurchargeCalculationDocument`——加「取当期序列定额」方法与「窗外行为」两格时照 `series_kind` / `series_factor` 那样 `omitempty`。
