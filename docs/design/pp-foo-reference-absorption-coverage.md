# IDP Parcel `foo` 参考设计吸收覆盖对照

状态：对照已建立；`已吸收`与`已确认不采纳`各有现存决策依据，`待决`项尚无决策，不得当作已决使用

`foo/` 是外部提供给本仓的参考设计输入，不是本仓权威来源，也不是待实现规格。它的定性决策见 [ADR-0011](../adr/0011-parcel-pricing-context-within-idp-parcel.md)。

本文是**台账**，只回答「foo 的某一块内容在本仓落到了哪里、或为什么没落」，并给出权威位置的链接。它不复述任何规则、不定义任何术语、不产生第二套口径。要知道某条规则本身，去它的权威位置读。

本文也不是评审记录，不替 `foo` 的正确性背书。`foo` 内部结论与本仓权威文档冲突时，以本仓权威文档为准。

## 三态定义

| 状态 | 含义 | 必须满足 |
|---|---|---|
| `已吸收` | 内容已进入本仓某个权威位置或代码 | 给出具体落点链接 |
| `已确认不采纳` | 存在现存决策说明本仓不取这一块 | 给出该决策的链接；无决策不得用此状态 |
| `待决` | 既未吸收也无不采纳决策 | 给出建议处置与决定方；建议不是决定 |

决策依据可以是既有权威文档，也可以是本台账本身——若某项在此处定案且不值得单独立档，就在该行写明定案范围，不再另建第二处记录。

`待决`不等于遗漏。它可能是刻意留到真实参数出现之后，也可能是从未核对过。两者在下方逐项区分。

## `foo` 的权威版本

`foo` 的三版《最终解决方案》并存，但 `foo/international-parcel-rating-golden-cases-v1.0.1.json` 的 `normative_baselines` 已经指明该套交付物自己认定的基线只有三份：

| 文档 | 版本 |
|---|---|
| 国际小包计费与结算平台最终解决方案 | `V1.2 审定版` |
| 国际小包计费领域模型 | `V1.0.1 终审版` |
| Rating Runtime 计算语义规范 | `V1.0.1 终审版` |

因此 `V1.0 EER 集成版` 和 `V1.1 独立版` 不是 `foo` 自己的基线，读参考设计时以 `V1.2 审定版` 为准。本文各表按该结论对照。

## 逐交付物对照

### 领域模型 `V1.0.1`（§1 至 §64）

| foo 章节 | 状态 | 落点或依据 |
|---|---|---|
| §1-5 愿景、范围、子域、统一语言、建模原则 | 已吸收 | [ADR-0011 Context](../adr/0011-parcel-pricing-context-within-idp-parcel.md)、[parcel-pricing/CONTEXT.md Language](../domain/parcel-pricing/CONTEXT.md) |
| §6-10 限界上下文总览、内部关系、Context Map、外部集成、数据权属 | 已吸收（改判） | [ADR-0011 Decision](../adr/0011-parcel-pricing-context-within-idp-parcel.md)、[CONTEXT-MAP.md](../domain/CONTEXT-MAP.md)。foo 的十二个上下文并非全部保留：见下方「上下文改判」 |
| §11-20 共享内核、标识值对象、Money、Quantity、DimensionSet、TimeRange/Bitemporal、VersionRef、PartyRef、SourceReference、DomainError | 部分已吸收 | `internal/parcelpricing/domain/` 的 `decimal.go`、`value_objects.go`、`errors.go` 已覆盖精度、币种、重量、版本引用与有效期。`DimensionSet`、双时态区间和 `PartyRef` 尚无落点 → 待决 |
| §21 Product & Eligibility | 已吸收（改判 `party-commercial`） | [CONTEXT-MAP.md](../domain/CONTEXT-MAP.md)、[party-commercial/CONTEXT.md](../domain/party-commercial/CONTEXT.md) |
| §22 Pricing Catalog | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) |
| §23 Contract Policy | 已吸收（改判 `party-commercial`） | [party-commercial/CONTEXT.md](../domain/party-commercial/CONTEXT.md) |
| §24 Facts & Classification | 已吸收（改判各来源上下文） | [CONTEXT-MAP.md](../domain/CONTEXT-MAP.md)：测量归 `node-operations`、包裹归 `parcel-shipment`、履约与收费发生项归 `transport-fulfillment` |
| §25 Rating | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md)、`internal/parcelpricing/domain/evaluation.go` |
| §26 Quote Management | 已确认不采纳 | [ADR-0011 Consequences](../adr/0011-parcel-pricing-context-within-idp-parcel.md) 延后正式 Quote 生命周期；[`PP-S03` 明确不做](./pp-s03-par-set-02-03-evidence-and-synthetic-contract.md) |
| §27-30 Charge Assessment、Financial Charging、Accrual & Settlement、Carrier Reconciliation | 已吸收（改判 `settlement-accounting`） | [settlement-accounting/CONTEXT.md](../domain/settlement-accounting/CONTEXT.md)、[`PN-07` 交接](./pn-07-operational-settlement-and-accounting-development-handoff.md) |
| §31 Pricing Governance | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 的价卡治理案例与发布生命周期 |
| §32 Security & Administration | 待决 | 本仓无对应落点。建议：不作为计价范围处理，租户与权限属跨切面，由 PN-08 的 `PAR-GOV-*` 承接。决定方：产品与治理 |
| §33-40 八条业务流程 | 已逐条核对 | 见下方[八条业务流程逐条核对](#八条业务流程逐条核对)；本台账 `FOO-OPEN-03` 已定案 |
| §41 价格发布流程 | 已吸收 | [parcel-pricing/CONTEXT.md 生命周期](../domain/parcel-pricing/CONTEXT.md) |
| §42-47 聚合事务边界、一致性模型、幂等、乐观并发、领域事件、Process Manager | 待决 | 建议不采纳：[ADR-0009](../adr/0009-go-modular-monolith-and-versioned-bento-contracts.md) 已定模块化单体，这些边界应在实现时收敛，现在决定即提前决策。决定方：技术 |
| §48 确定性约束 | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 的同输入同结果与回放；`internal/parcelpricing/domain/fingerprint.go` |
| §49 CQRS 边界 | 待决 | 同 §42-47 |
| §50 RatingExplanation | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 的评价解释 |
| §51 Margin 投影 | 已吸收（改判 `settlement-accounting`） | [settlement-accounting/CONTEXT.md](../domain/settlement-accounting/CONTEXT.md) 的经营指标 |
| §52-57 审计模型、租户隔离、组织与数据范围、敏感数据分类、职责分离、数据保留与删除 | 待决 | 建议：与 §32 合并处理，属跨切面而非计价范围。决定方：产品与治理 |
| §58-62 Repository、应用层、基础设施层、错误处理、性能约束 | 已确认不采纳 | [ADR-0009](../adr/0009-go-modular-monolith-and-versioned-bento-contracts.md) 与 [Go 首个消费者切片决策简报](./parcel-go-first-consumer-slice-decision-brief.md) 已自定本仓技术边界 |
| §63-64 领域模型验收标准、后续专项文档输入 | 不适用 | 属 `foo` 自身交付治理 |

#### 八条业务流程逐条核对

只标覆盖状态，不为对齐 `foo` 而补用例。`foo` 把计价、收费认定与财务处理放在同一条流水线上；本仓按 [CONTEXT-MAP](../domain/CONTEXT-MAP.md) 把纯评价留给 `parcel-pricing`、金额责任留给 `settlement-accounting`，因此一条 `foo` 流程常落到多个 `UC-*`，这不是缺口。

| `foo` 流程 | 覆盖状态 | 本仓落点 |
|---|---|---|
| §33 即时报价 | 不适用 | 依赖 Quote 生命周期，§26 已确认不采纳。价格解析本身由 [`UC-PC-002`](../application/party-commercial/UC-PC-002-RESOLVE-COMMERCIAL-BASIS.md) 承担，但不形成报价单 |
| §34 采购成本与销售价格并行 | 语义已吸收，无应用层用例 | `SELL` 只能引用一次冻结的 `BUY` 评价、两方向权限独立，已进 [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md)。`docs/application/` 下尚无计价用例目录 |
| §35 报价接受与履约重定价 | 不适用 | 依赖 Quote 与 `PriceLockPolicy`，同 §26 |
| §36 实际客户收费认定 | 已覆盖 | [`UC-SA-002`](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)。`foo` 的 Charge Assessment 对应本仓的费用确认与调整 |
| §37 承运商实际成本认定 | 已覆盖 | [`UC-SA-004`](../application/settlement-accounting/UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md)。账单主张、匹配与审核应付分立，与 `foo` 的「账单金额与重算金额都保留」一致 |
| §38 承运商对账与客户补扣 | 已覆盖但分立 | 匹配与争议归 `UC-SA-004`，客户补扣按经济原因归 [`UC-SA-002`](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)，纳入账期归 [`UC-SA-003`](../application/settlement-accounting/UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md)。`foo` 的五分支责任判断在本仓由「调整类型与唯一所有权」表裁定，不合并为一个流程 |
| §39 周期结算 | 主体已覆盖 | 截单与对账单归 [`UC-SA-003`](../application/settlement-accounting/UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md)；最低消费、保底量、阶梯返利已在 [settlement-accounting/CONTEXT.md](../domain/settlement-accounting/CONTEXT.md) 定为以合同或结算账户周期为主要范围。**佣金未见对应落点** |
| §40 追溯调价 | 部分覆盖 | 版本不可覆盖、历史评价不可改写已进 [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md)；补充或贷项调整归 [`UC-SA-002`](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)。**「识别受影响区间内既有评价与费用」这一影响分析无对应 `UC-*`** |

核对只暴露两处「未识别」：§39 的佣金和 §40 的追溯影响分析。两者都不在首发范围（首发不做 Quote、不做批量、价卡治理只到发布与回放），因此**此处只登记，不新增用例**；真到需要时再按各自所属上下文立用例。

#### 上下文改判

`foo` 的十二个限界上下文没有一对一复制。改判结果的权威在 [ADR-0011](../adr/0011-parcel-pricing-context-within-idp-parcel.md) 与 [CONTEXT-MAP.md](../domain/CONTEXT-MAP.md)，此处只作索引：

| foo 上下文 | 本仓落点 |
|---|---|
| Pricing Catalog、Rating、Pricing Governance | 合并为 `parcel-pricing` |
| Product & Eligibility、Contract Policy | `party-commercial` |
| Facts & Classification | 拆回 `node-operations`、`parcel-shipment`、`transport-fulfillment` |
| Charge Assessment、Financial Charging、Accrual & Settlement、Carrier Reconciliation | 合并为 `settlement-accounting` |
| Quote Management | 已确认不采纳 |
| Security & Administration | 待决 |

四合一到 `settlement-accounting` 是本次吸收中压缩幅度最大的一处。它在首发范围内成立，因为 [`PN-07`](./pn-07-operational-settlement-and-accounting-development-handoff.md) 的生产候选锁定在一个客户、一个币种、一个账期。范围扩大后是否需要重新拆分，见下方「触发条件」。

### Rating Runtime 计算语义规范 `V1.0.1`（§0 至 §41）

这是吸收比例最高的一份。

| foo 章节 | 状态 | 落点或依据 |
|---|---|---|
| §0-2 文档控制、Runtime 边界、核心不变量 | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 的边界与不变量 |
| §3-4 规范性统一语言、基础数据类型 | 已吸收 | `internal/parcelpricing/domain/decimal.go`、`value_objects.go` |
| §5 CalculationPurpose | 已吸收（收窄） | [parcel-pricing/CONTEXT.md 的「计算目的」](../domain/parcel-pricing/CONTEXT.md) 与 [GLOSSARY 词条](../domain/GLOSSARY.md)。本仓保留目的轴但首发只取三值并与价格方向一一配对，`foo` 的其余取值不引入；[`PP-S03`](./pp-s03-par-set-02-03-evidence-and-synthetic-contract.md) 当初挂起的语言决策至此结清 |
| §6-8 RatingInputSnapshot、业务时间与版本解析、Fact 选择 | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 的计价输入快照与版本清单；`internal/parcelpricing/domain/input.go` |
| §9 单位换算 | 部分 | 代码仅有重量单位。多单位换算待真实参数 |
| §10-12 Geometry 语义、地址与地理分类、包裹特征判定 | 待决 | 首期未实现。建议：由真实 `PAR-SET-02/03` 决定是否进入，不预先建模。决定方：计价与结算业务责任方 |
| §13-17 Actual Weight、Volumetric Weight、Conditional Minimum Weight、Billable Weight、Weight Rounding | 部分已吸收 | 代码已有计价重量方法与取整；体积重与条件最低重量按 [`PN07-S01` 首期计价实现边界](./pn-07-operational-settlement-and-accounting-development-handoff.md)待真实证据支持 |
| §18 Rating Aggregation | 已确认收窄 | [`PN07-S01`](./pn-07-operational-settlement-and-accounting-development-handoff.md)：首期只 `PER_PACKAGE`，票级与周期聚合不进首期 |
| §19 价表家族 | 已确认收窄 | [`PN07-S01`](./pn-07-operational-settlement-and-accounting-development-handoff.md)：首期只用 `WEIGHT_ZONE` 合成骨架，且不得成为生产默认 |
| §20 Published Tariff 与折扣 | 部分已吸收 | 费用方向已覆盖；完整折扣模型待真实价卡 |
| §21-26 Charge 候选生成、Scope/Basis/Method、Basis 解析、Method 算法、Charge Composition、费用依赖图 | 部分已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 的费用行语义与组合顺序；费用依赖图无落点 → 待决 |
| §27 Fuel Policy | 已确认不采纳 | [`PN07-S01`](./pn-07-operational-settlement-and-accounting-development-handoff.md)：最低/封顶/多层燃油不进首期 |
| §28 BUY 与 SELL 计算 | 已吸收 | [ADR-0011](../adr/0011-parcel-pricing-context-within-idp-parcel.md) 的方向隔离；`internal/parcelpricing/domain/evaluation_test.go` |
| §29 多币种与汇率 | 已确认不采纳 | [`PN07-S01`](./pn-07-operational-settlement-and-accounting-development-handoff.md)：复杂多币种换算不进首期 |
| §30 多段费用 | 待决 | 无落点。建议：与 §10-12 一并由真实参数决定 |
| §31 宏观执行阶段 | 待决 | 属实现形态。建议不采纳，理由同领域模型 §42-47 |
| §32 Rating Evaluation | 已吸收 | `internal/parcelpricing/domain/evaluation.go` |
| §33 解释、证据与执行轨迹 | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 的评价解释与版本清单 |
| §34 错误模型 | 部分已吸收 | `internal/parcelpricing/domain/errors.go` |
| §35 幂等与重放 | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 的回放规则；`fingerprint.go` 的内容摘要冲突检测 |
| §36 Compiled Pricing Plan | 已确认不采纳 | 本台账 `FOO-OPEN-04` 定案：本仓以内容指纹（`fingerprint.go`）解决同一正确性问题，编译式方案多买的是预计算与缓存，属无度量支撑的性能优化 |
| §37 Custom Function | 已确认不采纳 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md)：任意脚本与无法审计的动态函数不是首发价卡规则 |
| §38 性能与确定性 | 部分已吸收 | 确定性已吸收（见 §48）；性能约束待决 |
| §39 Golden Cases | 已吸收但证据隔离 | [parcel-pricing/CONTEXT.md 源完整性门禁](../domain/parcel-pricing/CONTEXT.md)；闭合路径见 [`PP-S03-W01`](./pp-s03-w01-golden-case-source-evidence-request.md) |
| §40 上游一致性追踪 | 待决 | 无落点。建议：由本文承担该职责，不另建机制 |
| §41 自洽审查 | 不适用 | 属 `foo` 自身交付治理 |

### Rating API Contract `V1.0.1`（§0 至 §30，含 OpenAPI、examples、validator）

| foo 内容 | 状态 | 落点或依据 |
|---|---|---|
| §1-27、§29-30 端点族（Eligibility、Rating、Replay、Comparison、Batch、Quote）、鉴权与租户、幂等键、`If-Match`、状态码、同步/异步、分页、版本策略、事件边界、可观测性、客户端与服务端要求 | 已确认不采纳 | [ADR-0011 Decision](../adr/0011-parcel-pricing-context-within-idp-parcel.md)：`parcel-pricing` 不是独立部署单元或微服务；[ADR-0011 Alternatives](../adr/0011-parcel-pricing-context-within-idp-parcel.md) 否决把 `foo` 复制为独立计费平台。对外契约以 [ADR-0009](../adr/0009-go-modular-monolith-and-versioned-bento-contracts.md) 的版本化 Bento 合同为准 |
| §28 Golden Cases 映射 | 待决 | 与 [`PP-S03-W01`](./pp-s03-w01-golden-case-source-evidence-request.md) 的闭合结果一并处理 |
| `rating-api-openapi-v1.0.1.yaml`、`rating-api-examples-v1.0.1.json`、`validate_rating_api_contract_v1_0_1.py` | 已确认不采纳 | 同上 |

ADR-0011 否决的是「独立计费平台」这一形态，未点名 API 契约本身。本台账 `FOO-OPEN-05` 已定案：不另写 ADR，由本表承担该记录。ADR-0011 状态为 Accepted，按[改文档规则](../../AGENTS.md)也不得改写其正文。

### Golden Cases `V1.0.1`（§1 至 §12，含 JSON、schema）

| foo 内容 | 状态 | 落点或依据 |
|---|---|---|
| 治理案例概念、案例分层、通过标准 | 已吸收 | [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 的价卡治理案例 |
| 136 个案例作为验收证据 | 已吸收但隔离 | [parcel-pricing/CONTEXT.md 源完整性门禁](../domain/parcel-pricing/CONTEXT.md)：83 例 `NORMATIVE_SYNTHETIC` 与 12 例 `UPSTREAM_CONSISTENCY` 只作隔离 `S`，41 例 `SOURCE_RATE_CARD` 必须隔离 |
| `source_discrepancies` 声明的四条源价卡差异 | 已记录，门禁未列，裁决未决 | `foo` 声明了 `SRC-DISC-001` 至 `004`，其中 `001` 为 `BLOCKING`。四条已逐条记入 [`PP-S03-W01`](./pp-s03-w01-golden-case-source-evidence-request.md)；[源完整性门禁](../domain/parcel-pricing/CONTEXT.md)仍只列 Schema 指针、源文件身份和哈希三项，未把这四条列为门禁条件。`SRC-DISC-001` 裁决前，41 例即使那三项闭合也不能成为金额证据 |

### Rating Runtime 技术设计 `V1.0.1`（`price tech/`，§0 至 §42）

| foo 内容 | 状态 | 落点或依据 |
|---|---|---|
| §1-10、§14-35、§37-42 分层、Go 工程结构、Port/Adapter、事务边界、26 阶段执行管线、各 Engine、缓存、持久化、并发、安全、可观测性、容量、高可用、保留归档、测试策略、发布回滚、Runbook、实施分期、技术验收 | 已确认不采纳 | [ADR-0009](../adr/0009-go-modular-monolith-and-versioned-bento-contracts.md) 与 [Go 首个消费者切片决策简报](./parcel-go-first-consumer-slice-decision-brief.md) 已自定本仓技术边界 |
| §11 Compiled Pricing Plan、§13 Artifact 格式与内容寻址 | 已确认不采纳 | 与计算语义 §36 同一取舍，见本台账 `FOO-OPEN-04` |
| §12 Plan Compiler 与发布边界 | 部分已吸收 | 发布治理语义已进 [parcel-pricing/CONTEXT.md 生命周期](../domain/parcel-pricing/CONTEXT.md)；编译器形态属实现 |
| §36 发布与回滚 | 部分已吸收 | 业务侧「发布失败或回滚只影响新的评价选择」已进 [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) |
| `compiled-pricing-plan-manifest-*.json`、`rating-runtime-architecture-manifest-v1.0.1.yaml`、`validate_rating_runtime_tech_design_v1_0_1.py` | 已确认不采纳 | 同上 |

### MVP 研发实施设计与任务拆解 `V1.0.1`

| foo 内容 | 状态 | 落点或依据 |
|---|---|---|
| §1-17 MVP 范围、团队组织、Epic、Sprint 路线、DoR/DoD、分支与发布、测试、数据初始化、UPS Ground 首个验证切片、风险、任务工件 | 已确认不采纳 | 本仓自有工作组织：[首发开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md) 的 `PN-01` 至 `PN-08`，以及 [issue tracker 约定](../agents/issue-tracker.md) 的 `.scratch/` |
| `mvp-work-breakdown`、`mvp-sprint-plan`、`mvp-task-import`、`mvp-traceability-matrix`、`validate_mvp_implementation_plan_v1_0_1.py` | 已确认不采纳 | 同上。按独立平台组织，与本仓 PN 切片逻辑正交 |

## 待决清单

按建议处置的紧要程度排列。建议不是决定。已定案的项保留在表内并注明结论，便于对照 `foo` 时知道结论从何而来。

| 编号 | 事项 | 结论或建议处置 | 决定方 |
|---|---|---|---|
| `FOO-OPEN-01` | 计算语义 §5 `CalculationPurpose` 的取值范围 | **已定案。** 核实后发现真问题不是取值多少：`PricingPurpose` 的三个常量与 `PricingDirection` 一一对应，目的轴未携带方向之外的信息，而校验只过正则不查成员，也不强制配对。结论是保留该轴并在 [parcel-pricing/CONTEXT.md](../domain/parcel-pricing/CONTEXT.md) 写明首发一一对应及其解除条件，同时闭合枚举、强制配对，术语统一为「计算目的」并立 [GLOSSARY 词条](../domain/GLOSSARY.md) | 计价业务责任方 |
| `FOO-OPEN-02` | Golden Cases 的 `source_discrepancies` 四条差异 | 记录已完成：四条已进 [`PP-S03-W01`](./pp-s03-w01-golden-case-source-evidence-request.md) 的取证要求。仍缺两件事——把这四条列入 [源完整性门禁](../domain/parcel-pricing/CONTEXT.md)，以及对 `BLOCKING` 的 `SRC-DISC-001` 作出合同裁决。文件身份与哈希闭合是必要不充分条件 | 计价与结算业务责任方 |
| `FOO-OPEN-03` | 领域模型 §33-40 八条业务流程从未与 `UC-*` 逐条核对 | **已定案。** 核对结果见[八条业务流程逐条核对](#八条业务流程逐条核对)：§33、§35 因 Quote 不采纳而不适用，§36、§37 分别由 `UC-SA-002`、`UC-SA-004` 覆盖，§38、§39 覆盖但按经济原因分立于多个用例，§34 语义已进 CONTEXT 而无应用层用例。仅两处「未识别」——§39 的佣金与 §40 的追溯影响分析，均不在首发范围，只登记不新增用例 | 结算与计价业务责任方 |
| `FOO-OPEN-04` | 计算语义 §36 与技术设计 §11/§13 的 Compiled Pricing Plan | **已定案：不采纳。** `fingerprint.go` 已用内容指纹解决「版本引用相同但内容不同判为冲突」这一正确性问题，编译式方案在此之上多买的是预计算与缓存，属性能诉求，而本仓无任何性能度量。将来出现度量支撑时可重新评估 | 技术 |
| `FOO-OPEN-05` | ADR-0011 未点名 API 契约与技术设计本身 | **已定案：不另写 ADR，由本台账承担该记录。** 「不采纳 `foo` 的对外 API 契约与运行时技术设计」是 [ADR-0009](../adr/0009-go-modular-monolith-and-versioned-bento-contracts.md) 与 [ADR-0011](../adr/0011-parcel-pricing-context-within-idp-parcel.md) 的推论而非独立决策，为推论单写 ADR 会制造第二套口径 | 技术与产品 |
| `FOO-OPEN-06` | 领域模型 §32、§52-57 的安全、租户隔离、审计、数据保留 | 建议按跨切面处理，由 PN-08 的 `PAR-GOV-*` 承接，不作为计价范围 | 产品与治理 |

各表中标为`待决`但未进本清单的条目（如计算语义 §10-12、§30、§31、费用依赖图、性能约束、上游一致性追踪），共同处置原则是等真实参数决定，不预先建模。

## 触发条件

`settlement-accounting` 承接了 `foo` 四个上下文的职责。该压缩在首发范围内成立，但下列任一情形出现时应重新评估是否需要拆分：

- 出现第二个结算币种；
- 出现第二个结算责任法人；
- 承运商对账进入常态化运营而非逐票处理。

按红线「只实现已确认规则」，现在不拆。此处只登记触发条件，不构成拆分决定。

## 收口条件

本对照达到收口的条件是：待决清单各项形成决策或明确延后依据，并回填本表状态。`FOO-OPEN-01`、`03`、`04`、`05` 已定案，余 `02`、`06` 两项。收口后 `foo/` 即为只读参考，任何人再读它都能从本表知道本仓的取舍及其依据。

本表随权威文档变化而更新；它不是快照，也不承担解释规则的职责。

## 相关文档

- [ADR-0011：在 idp-parcel 内建立小包计价上下文](../adr/0011-parcel-pricing-context-within-idp-parcel.md)
- [小包计价上下文](../domain/parcel-pricing/CONTEXT.md)
- [领域上下文地图](../domain/CONTEXT-MAP.md)
- [`PP-S03` 证据与合成契约开发交接](./pp-s03-par-set-02-03-evidence-and-synthetic-contract.md)
- [`PP-S03-W01` Golden Case 源证据闭合工作单](./pp-s03-w01-golden-case-source-evidence-request.md)
- [`PN-07` 运营结算与经营核算开发交接](./pn-07-operational-settlement-and-accounting-development-handoff.md)
