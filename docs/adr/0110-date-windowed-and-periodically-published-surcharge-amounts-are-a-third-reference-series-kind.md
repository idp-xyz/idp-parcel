# ADR-0110: 只在日期窗内生效、金额逐期公布的附加费（PSS / 高峰附加费）作为第三种计价参考序列——按期公布的**金额**序列；附加费计算新增「取当期序列定额」，窗口由期次表达，窗外即序列未解析

Status: Accepted
Date: 2026-09-04

## Context

末端渠道的高峰附加费（Peak / Demand Surcharge）有两个特征：只在某个日期窗内生效（如每年十月至次年一月），以及金额按周或按段公布、逐段不同。今天的模型（票 [price-card-shape-gaps/02](../../.scratch/price-card-shape-gaps/issues/02-date-windowed-surcharge-has-no-shape.md)，核于 `a17bfac`，本记录复核于 `512b419` 各条仍成立）三处都装不下：

- `TriggerCondition` / `FeatureSource` 的可判定量全是包裹的几何量、重量与三个类别量，没有日期或计价基准时点，条件读不到「今天在窗内」；
- `ReferenceSeriesKind` 只有 `FUEL_RATE` 与 `EXCHANGE_RATE`，序列取值只经 `NewSeriesRateSurcharge` 作**百分比费率**用；PSS 是**定额**（每件若干）或分区分档定额，不是基数百分比；
- `EffectivePeriod` 是整张方案与价表的有效期，不是某一条规则的。

今天唯一能表达的是把方案切成窗前 / 窗内 / 窗后三个版本，金额逐周变则每周再发一版。可行，但代价与 [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md) 解决燃油率时否掉的那条路一模一样：「序列每出一版，引用它的每张卡都要重登」。

## Decision

**一、PSS 的本质是承运商按期公布的外部数值，与燃油率同族，只差取值是金额；因此它是第三种计价参考序列，不是价卡上的第三类条款。** `ReferenceSeriesKind` 新增一种「按期公布金额」（符号名由实施定），一期取值是**带币种的金额**；同一序列的期次连续或留缺口，登记、复核、在用版本派生全部照 ADR-0099 给两种既有序列立的那一套，连接器（[ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md) 决定六、票 pricing/06）同样适用。

**二、附加费计算新增一种「取当期序列定额」。** 与 `NewSeriesRateSurcharge`（当期费率 × 系数 × 基数）并列：按评价基准时点解析该序列的在用版本与所在期次，取那一期的金额作附加费金额；金额币种须与方案币种一致，不一致在构造期拒——与附加费币种须与价表一致那条既有门同形。它是定额不是百分比，因此不带基数依赖。

**三、日期窗由序列期次表达，不加时点特征。** 窗内有期次，评价取到定额；窗外没有期次，序列解析答「无期次」，评价按既有原因 `REFERENCE_SERIES_UNRESOLVED` 挂起——**但这一格要读窄**：对 PSS 序列，「窗外无期次」是**正当的零**（本期不收），不是数据缺口。所以本记录要求：绑了金额序列的附加费规则**在卡上声明「窗外行为」**，封闭两格——「不计收」或「待判断」；未声明的卡不合法。前者是 PSS 的常态（窗外就是不收），后者留给「金额尚未公布、不能当零」的场合。这一格是实例声明的，机制不猜。

**四、分区分档的 PSS 不新造形状。** 每分区一条附加费规则各绑一条金额序列，触发条件用既有 `FeatureZone` + `ComparisonEquals`；「一张卡 N 个分区 N 条序列」是登记量的事，不是模型的事。

**五、规范化版本换号一次（ADR-0014）。** `ReferenceSeriesKind` 闭合集合与 `SurchargeCalculation` 的规范化形状都变；旧评价按原版本重放。与 ADR-0107 / 0108 / 0109 同期实施时合并为一次换号。

## Consequences

- **逐周金额不再逐周重登卡**：序列出一期，所有绑它的卡在下一次评价时自动取到；与燃油率同一套治理、复核与在用派生，运营只学一次。
- **「窗外不收」有了显式的家**：不是评价挂起，不是零金额顶替，是卡上声明过的那一格——与 label-channel/13「出局不得以零金额顶替」的立场一致：零是登记出来的答案时才是零。
- **CONTEXT「计价参考序列」词条改口**：燃油费率、汇率与按期公布的定额附加金额是三个实例；序列取值可以是费率也可以是带币种的金额。
- 代价：`ReferenceSeriesKind` 与 `SurchargeCalculation` 两个封闭集扩格，登记 spec、快照、规范化文档、逐字段表单与 JSON 登记口都要跟；三步法或单独 worktree。
- 代价：评价解释项多一格「取自序列 X 第 N 期」的来源记录，读面（评价清单、成本分值桥）要能透出它。
- **本记录不填任何取值**：没有任何真实 PSS 窗口、金额或渠道进仓；SYN 夹具只记 `S`。E2 转换工具在本记录落地前不把 PSS 折进基础运费、不写成常年附加费，如实列「未转换：等本票」。

## Alternatives considered

- **改法 1：版本切分，不改模型。** 否决：把运营负担推回逐周重登，正是 ADR-0099 已经否过的形状；六家渠道 × 若干周的方案版本让重放与对账跨版本看，且每版内容摘要不同。
- **改法 3：`FeatureSource` 加「计价基准时点」配区间比较。** 否决：它解决了窗口却解决不了逐周金额——每周金额仍要多条规则或多版；且把时点混进「包裹的可判定量」这个集合，让判定条件从此有两类输入。
- **让 `FIXED_AMOUNT` 的金额可来自序列，不加新计算种类。** 否决：那让同一种计算的金额有两种来源，规范化形状里要多一个判别字段；并列一种新计算比在旧计算里开分支清楚。
- **窗外一律挂起（不加「窗外行为」声明）。** 否决：PSS 的窗外是正当的零，挂起会让全年大半时间的评价停在未决；而一律当零又把「金额尚未公布」的场合误判为不收。两格都真实存在，只能由卡声明。

## Links

- [票 price-card-shape-gaps/02](../../.scratch/price-card-shape-gaps/issues/02-date-windowed-surcharge-has-no-shape.md)：三条改法与取证
- [E1 形状核对报告](../../.scratch/price-card-shape-gaps/report.md)：第 16 项
- [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)：序列的登记、复核、在用派生与连接器——第三种序列全部沿用
- [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md)：计价拥有外部数值序列
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：规范化版本换号
- [票 label-channel/13](../../.scratch/label-channel-service-first-release/issues/13-buy-evaluation-to-cost-score-bridge.md)：「不得以零金额顶替」——窗外行为要显式声明的同一立场
- [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)：「计价参考序列」词条改口
