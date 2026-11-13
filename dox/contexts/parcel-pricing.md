# 小包计价（`parcel-pricing`）

代码在 `internal/parcelpricing`。对照 main `3dd72c42a`（2026-11-12）写成。

**这份文件核到什么程度**：取值表、评价状态、问题码、入口三节是从代码里读出来的。「规则」一节的句子是从旧 `docs/domain/parcel-pricing/CONTEXT.md` 提炼的，每条只确认了代码里有对应的机制，没有逐条跑测试验证行为；旧文档写了而代码里没有的，单列在最后一节。

## 管什么，不管什么

管：价卡（定价方案、价表、规则）及其版本发布，计价参考序列（燃油费率、汇率等），纯价格评价及其解释，回放。

不管：

- 客户合同、供应商协议、谁能用哪个价格方向——`party-commercial`。
- 费用、应收应付、对账、核销，以及把评价变成钱——`settlement-accounting`。评价不是费用，消费方不得回写评价。
- 包裹、测量、地址、收费发生项——各自的上下文。价卡只引用，不创建也不修改。
- 多份候选评价里选哪一份——`network-routing`。本上下文每个候选各出一份评价，不比优劣。
- 授权、租户隔离、审计留痕。

## 词

| 词 | 意思 |
|---|---|
| 定价方案 | 一个价格方向、一个用途、一个适用范围下的一套价表与计价政策。不是合同，不是费用明细 |
| 定价方案版本 | 方案一次发布形成的不可覆盖版本，带适用期与依赖版本 |
| 价表版本 | 在明确的分区、重量段、单位、币种下给出金额的不可覆盖版本 |
| 价表族 | 价表把计量映射为金额的结构形状，见取值表 |
| 价格方向 | 钱归哪一侧：采购、销售、内部 |
| 计算目的 | 这次计算用来做什么。首发与方向一一配对 |
| 计价范围 | 选卡键上的一格，随价卡登记，由请求方给出 |
| 计价输入快照 | 一次评价用到的输入事实的冻结副本 |
| 评价对象 | 评价针对的东西：已接受包裹、试算对象、票、主单 |
| 价格评价 | 一次纯计算的结果：状态、计价重量、费用行、解释、版本清单 |
| 计价重量 | 用于查基础价和后续费用基数的重量，由卡的重量策略定 |
| 体积系数 | 由尺寸算体积重的系数，是价卡内容，不是常量 |
| 判定条件 | 一条规则是否触发的可解释条件，只能读已声明的特征 |
| 附加费规则 | 在基础运费之外按条件计收的规则 |
| 互斥组 | 组内至多一条规则命中 |
| 费用依赖 | 一条费用以哪些别的费用为基数，显式声明 |
| 总额上下限 | 价卡声明的最低、最高收费 |
| 计价参考序列 | 随时间变化、按期登记的外部取值（燃油费率、汇率、金额） |
| 版本清单 | 一次评价实际用到的全部版本引用 |
| 规范化版本 | 价卡内容写成规范文档所用的格式版本，摘要按它算 |
| 不可计价 | 规则明确排除了这个对象。不是零金额，也不是缺资料 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 价格方向 | `BUY` `SELL` `INTERNAL` |
| 计算目的 | `CUSTOMER_CHARGE` `SUPPLIER_COST` `INTERNAL_PRICE` |
| 评价对象 | `ACCEPTED_PACKAGE` `ESTIMATE` `SHIPMENT` `MASTER_DOCUMENT` |
| 聚合方式 | `PER_PACKAGE` `PER_SHIPMENT` `PER_MASTER_DOCUMENT` |
| 价表族 | `WEIGHT_ZONE`（重量段阶梯） `FIRST_CONTINUE`（首重加续重） `UNIT_PRICE`（计费重乘单价） |
| 计价重量方法 | `ACTUAL_ONLY` `MAX`（实重与体积重取大） |
| 取整 | `NONE` `CEILING` `HALF_UP` |
| 重量单位 | `KG` `LB` `OZ` |
| 费用行种类 | `BASE` `FIXED` `SURCHARGE` `TOTAL_BOUND` |
| 费用方向 | `ADD` `DEDUCT` |
| 计算方法 | `FIXED_AMOUNT` `TABLE_LOOKUP` `PERCENT_OF_BASIS` `GREATER_OF` `SERIES_AMOUNT` |
| 地址性质 | `COMMERCIAL` `RESIDENTIAL` |
| 序列复核结论 | `APPROVED` `RETURNED` |

## 评价的五种结果

| 状态 | 什么时候 | 之后 |
|---|---|---|
| `COMPLETED` 已完成 | 输入、规则、计算都齐，有金额 | 不可变 |
| `PENDING` 待判断 | 事实、规则或依赖不足 | 补齐后另行评价 |
| `CONFLICT` 冲突 | 互斥的规则、区间或版本候选选不出唯一一个 | 由治理责任方裁决 |
| `UNRATABLE` 不可计价 | 规则明确排除 | 补资料、重试都不改变；只有新规则版本能改，且算另一次评价 |
| `FAILED` 计算失败 | 请求不合法或算不下去 | 技术或结构问题，不是业务结论 |

五者不得互相替代。**不得用金额为零的已完成评价表达不可计价或无适用价格。**

问题码（解释里点名停在哪）：`ADDRESS_NATURE_REQUIRED` `AGGREGATION_SUBJECT_MISMATCH` `CALCULATION_FAILED` `CANONICALIZATION_VERSION_UNSUPPORTED` `DIMENSIONS_REQUIRED` `EXCLUDED_BY_RATE_CARD` `INVALID_REQUEST` `LENGTH_UNIT_MISMATCH` `NEGATIVE_TOTAL` `PACKAGE_FEATURES_UNAVAILABLE` `PLAN_CONTENT_MISMATCH` `PLAN_STRUCTURES_NOT_EXECUTABLE` `PRICING_SCOPE_MISMATCH` `RATE_NOT_FOUND` `RATE_TABLE_CONFLICT` `REFERENCE_CATALOGUE_MISMATCH` `REFERENCE_SERIES_CURRENCY_MISMATCH` `REFERENCE_SERIES_UNRESOLVED` `REFERENCE_SERIES_VERSION_MISMATCH` `REMOTE_TIER_UNRESOLVED` `REPLAY_RESULT_MISMATCH` `VERSION_MANIFEST_MISMATCH` `VERSION_NOT_APPLICABLE` `WEIGHT_UNIT_MISMATCH` `ZONE_UNRESOLVED`。

## 规则

**版本与发布**

- 发布后的方案、价表、规则与版本清单不可原地覆盖；变化形成新版本。
- 发布要过依赖闭合、方向隔离与区间、单位、币种检查。发布失败只影响之后的选卡，不动已有评价。
- 同一范围、方向、目的上同一方案两版同时适用时，旧版只经指名后继的显式收尾离开：后继晚于旧版起点的，旧版截止在后继起点；不晚于的，旧版整段退役。
- 规则只表达确定的组合与查找；任意脚本、隐式平台默认不是价卡规则。

**选卡**

- 每次评价必须明确租户、计价范围、价格方向、计算目的、计价基准时点。
- 方向与目的必须成对；`BUY`、`SELL`、`INTERNAL` 的评价各自形成，一侧不可计价不推出另一侧不可计价。
- 选销售卡时：指名了委托所报服务产品的在用卡恰一张，用它；一张没有，看不声明产品的卡，恰一张用它；同一层多于一张是冲突，不挑。
- 没有在用价卡、多版同时适用、造不出输入快照时不形成评价，把停在哪一格交回结算。

**重量、单位、取整**

- 体积重由本上下文按卡上的体积系数从尺寸算，不接收外部算好的值。
- 输入重量或尺寸的单位与卡不一致时，按产品内置单位目录换算，并在评价里留下原值、原单位、目标单位与系数；回放只认记下的系数。
- 条件最低计价重量抬高的是方案级计价重量；多条同时触发取最高。
- 取整顺序与精度由卡声明；页面显示精度不改变结果。

**费用行**

- 每条费用行有计价费用代码、方向、范围、基数和方法。
- 判定条件只读已声明的特征，可读体积重、自身计价重量与地址性质；恰等于阈值不触发。读地址性质而输入没声明时评价待判断，不套默认。
- 未命中的规则也要在解释里留痕。
- 互斥组内多条满足时取声明优先级最高的，同级取金额最高的；仍选不出唯一一条是冲突。
- 附加费按偏远档位查表时，档位解不出、或解出的档位表里没有行，都是待判断，不给零金额。
- 费用依赖显式声明基数构成；有环是冲突。
- 总额上下限：低于下限补一条补差行，高于上限补一条封顶行。

**参考序列**

- 方案只绑序列标识，不绑序列版本。评价形成时取在用序列版本，再按计价基准时点取期次，一并写进版本清单。
- 一版序列经复核通过才在用；来源连接器转录的同样要复核，除非租户显式声明免复核（租户取值，产品不设默认）。
- 取值有错就出更正版本；已有评价保留原值。
- 燃油费是公布费率乘卡上系数，两个数都留在解释里。币种换算保留原币金额与所用汇率序列版本。

**回放**

- 同样的输入、方向、目的、时点、版本清单与内容摘要必须得出同样的结果。
- 回放用新的评价引用，不动原评价；结果不同是冲突，不算回放成功。原评价不可计价的，回放也必须重现不可计价。

**试算**

- 试算对象的评价不进结算，不形成客户承诺。受理前估价是唯一例外，只作冻结与信用暴露的控制金额。

## 生命周期

- **价卡**：草稿 → 提交 → 批准并发布 → 在用 →（后继截断，或退役）。也可以经受控登记口直接登记一版。已发布的要改，另起草稿。
- **序列版本**：已登记 → 复核通过即在用，或退回；更新的版本复核通过后，旧版被替代。登记与复核记录只增不改。
- **评价**：形成即定为上面五种结果之一，此后不变。

## 入口

在线口（路径在 `adapters/http`）：

| 做什么 | 路径 |
|---|---|
| 评价、试算、回放 | `/pricing-evaluations` `/pricing-estimates` `/pricing-evaluation-replays` `/pricing-pending-series-evaluations` |
| 价卡草稿到发布 | `/pricing-price-card-previews` `/pricing-price-card-drafts` `/pricing-price-card-draft-views` `/pricing-price-card-draft-approvals` `/pricing-price-card-draft-publications` |
| 价卡登记、读、收尾 | `/pricing-price-card-registrations` `/pricing-price-cards` `/pricing-price-cards/content` `/pricing-price-card-successor-cuts` `/pricing-price-card-retirements` `/pricing-price-card-template/spec` |
| 参考序列 | `/pricing-reference-series` `/pricing-reference-series-previews` `/pricing-reference-series-registrations` `/pricing-reference-series-reviews` `/pricing-reference-series-coverage` `/pricing-reference-catalogue-reviews` |

离线口（运营在数据库网络内手跑，不监听端口）：`cmd/parcel-pricing-register`（配置登记）、`cmd/parcel-pricing-feed`（序列批量喂价）、`cmd/parcel-pricing-template`（写出价卡导入空白模板）。

## 旧文档写了、代码里还没有的

- **仓储计费方案**（计量费用规则、声明值采用、仓储作业收费发生项的评价）：旧 `CONTEXT.md` 写成现行规则，`internal/parcelpricing` 里没有对应的类型。它属可选的仓配履约，已搁置。
- **销售卡按有序范围列表选卡**：评价目前按单个计价范围选卡。实现是客户要求清单的票 18，未进 main。

## 证据限制

外部参考设计带来的那张第三方价卡只能当**结构来源**（有哪些费用、怎么触发、怎么算），不能当金额来源：卡内有人工录入痕迹，源文件哈希也对不上。相关案例只算 `S` 证据。
