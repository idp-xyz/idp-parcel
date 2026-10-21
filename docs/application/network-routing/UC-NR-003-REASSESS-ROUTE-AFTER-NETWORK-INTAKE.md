# UC-NR-003 按网络收寄与节点实测复核路由

状态：稳定业务骨架已形成；真实路由策略、收寄位置、节点实测与非功能参数待提供

## 目标与边界

本用例从包裹形成有效网络收寄，或者节点形成新的当前有效实测、控制节点及其他必须重评的事实开始，到 `network-routing` 对剩余旅程形成“原路由仍适用”、新路由版本、无当前有效路由、改路建议、判断未决或触发未判结果为止；不构成复核触发的来源答触发未受理。

委托接受后的 [`UC-NR-001`](./UC-NR-001-CREATE-INITIAL-ROUTE.md) 使用客户声明和接受基线形成初始路由。本用例必须重新使用已经发生的接货位置、控制依据和节点实测，不能把接受前可达性或初始计划视为永久有效。

本用例包含两个强制复核点：

1. 有效网络收寄形成后，按实际接货位置、控制依据、当前限制、截单和网络可用性复核；场外揽收不等待尚未发生的节点到站，只按硬约束判原计划适用性，揽收地点不与计划节点比，不建立当前可控节点（[ADR-0249](../../adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)）。
2. 包裹进入节点并形成节点控制和当前有效实测后，按实际节点、实测、截单、限制和网络可用性再次复核。

本用例不执行：

- 节点收寄、场外揽收、测量、位置、控制或身份事实。
- 委托接受、有效网络收寄、正式承诺、服务变更或包裹终局。
- 具体班次选择、容量预占、订舱、运输委托、装载分配或实际运输。
- 关务决定、限制解除、异常案件、客户通知或费用。
- 以新计划覆盖旧计划、承诺或已经发生的事实。

## 元数据

| 项目 | 内容 |
|---|---|
| 用例 ID | `UC-NR-003` |
| 主责上下文 | `network-routing` |
| 主要参与者 | 路由能力、节点/运输来源、网络规划、必要时授权运营角色 |
| 业务决定所有者 | `network-routing` |
| 触发 | 有效网络收寄、节点控制、当前有效实测、权威运输交接把包裹交进声明的接收节点（[ADR-0215](../../adr/0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)），或其他明确重评事实形成 |
| 起始边界 | 触发能够关联包裹、当前路由结果、来源事实和业务时间 |
| 成功结束 | 保留原路由适用性，或形成一个新当前有效路由版本 |
| 其他结束 | 形成无当前有效路由、改路建议、判断未决、触发未判、触发未受理或不适用 |

## 权威依据

| 引用键 | 权威内容 | 来源 |
|---|---|---|
| `NR-LANG` | 路由计划、当前可控节点、冻结边界、改路和无当前有效路由 | [网络与路由：Language](../../domain/network-routing/CONTEXT.md#language) |
| `NR-RULE` | 收寄/实测复核、路由稳定性、硬限制和资源边界 | [网络与路由：Rules and invariants](../../domain/network-routing/CONTEXT.md#rules-and-invariants) |
| `NR-LIFE` | 路由计划适用性和受控改路生命周期 | [网络与路由：Lifecycles](../../domain/network-routing/CONTEXT.md#lifecycles) |
| `PS-INTAKE` | 有效网络收寄与正式承诺 | [`UC-PS-003`](../parcel-shipment/UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md) |
| `NO-FACT` | 节点收寄、控制、位置和当前有效实测 | [节点作业](../../domain/node-operations/CONTEXT.md) |
| `TF-FACT` | 场外揽收、运输控制和实际接货位置 | [运输履约](../../domain/transport-fulfillment/CONTEXT.md) |
| `CTX-REL` | 路由与小包、节点、运输和关务的所有权 | [领域上下文地图](../../domain/CONTEXT-MAP.md#关系约束) |
| `ADR-LAYERS` | 承诺、计划和实际事实分离 | [ADR-0004](../../adr/0004-separate-commitment-plan-actual.md) |
| `ADR-FACT` | 来源事实、有效事件和派生状态分离 | [ADR-0005](../../adr/0005-source-facts-effective-events-derived-state.md) |
| `E2E` | 正常服务、无路由、实测差异和场外揽收场景 | [核心领域场景](../../domain/SCENARIOS.md) |
| `ACCEPTANCE` | `OPS-01/02/03/04`、`DAT-02/03` 和 `NFR-02` | [首发试点验收矩阵](../../product/PILOT-ACCEPTANCE-MATRIX.md) |

## 参与者与协作

| 参与者或上下文 | 本用例职责 | 明确不负责 |
|---|---|---|
| `parcel-shipment` | 提供包裹身份、接受基线、有效网络收寄、正式承诺及当前服务范围 | 不选择路线或修改路由 |
| `node-operations` | 提供节点收寄、节点控制、当前位置和当前有效实测 | 不决定路由或计费重量 |
| `transport-fulfillment` | 提供场外揽收、运输控制、实际接货位置和能力信号 | 不因重评请求预占容量或创建装载 |
| `customs-compliance` | 提供候选关务路径资格、当前限制和解除结果 | 不选择最终路径 |
| `network-routing` | 验证触发、评估当前计划与候选、形成适用性或改路结果 | 不修改来源事实、客户承诺或资源 |
| 授权运营角色 | 在策略不允许自动改路时决定是否采用建议 | 不能绕过硬限制、已发生事实或其他上下文责任 |

## 启动条件

- 包裹已经接受且属于适用网络服务；当前包裹身份、接受基线和服务责任范围可以验证。
- 触发来自已经接受的有效网络收寄、节点控制、当前有效实测或明确需要重评的权威事实。
- 当前路由计划或无当前有效路由结果可以查询；没有历史结果时不得凭空假定初始计划。
- 路由策略、网络、日历、截单、限制和资格版本可以按实际业务时点解析。依赖读不动时答未决、回滚重投；判断时点证据未配置时入账为`触发未判`、留下原因（[ADR-0235](../../adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)）。

来源到达不等于来源已经被业务接受。普通扫描、位置消息、装卸或车辆到场只能作为来源线索，不能在没有节点收寄或权威运输控制依据时建立当前可控节点。

## 输入语义契约

| 输入组 | 权威来源 | 最低业务语义 | 使用约束 |
|---|---|---|---|
| 服务与承诺 | `parcel-shipment` | 包裹、接受基线、服务范围、有效网络收寄、预计/正式承诺和业务时间 | 路由只读引用，不修改承诺 |
| 当前路由结果 | `network-routing` | 当前计划或无路由判断、版本、计划节点/段、时间窗口、冻结边界和已执行前缀 | 旧版本永久保留 |
| 客户地理投影 | `parcel-shipment`，随有效网络收寄的发布意图携带 | 采用所对那一版提交的寄件、收件两端国家与邮编 | 路由不回读、不留存地址（[ADR-0075](../../adr/0075-customer-address-is-carried-with-the-routing-request.md)）；没带时关务适用性只能答两端未知。交接触发的复核没有 PS 信封可随带，由本上下文的消费侧适配器读受理基线那一版提交的同一份投影，只到地理维（[ADR-0215](../../adr/0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)） |
| 实际接货来源 | `node-operations` / `transport-fulfillment` | 节点收寄或场外揽收、实际地点、控制依据、发生时间和版本；权威运输交接以`已交接`进入声明的接收节点时，地点取接收节点、版本与发生时间取交接（[ADR-0215](../../adr/0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)） | 两类来源不得互相伪装；拒收、待确认与没声明接收节点的交接不是复核触发。场外揽收的地点是客户场所一类的接货位置，原样记入复核依据，不与计划节点比（[ADR-0249](../../adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)） |
| 节点实测 | `node-operations` | 当前有效重量、尺寸、数量及来源事实版本 | 不使用客户或供应商计费重量替代 |
| 网络判断基线 | `network-routing` | 服务区域、节点、连接、线路、时区、日历、截单、可用性和策略版本 | 每次判断保存实际采用版本 |
| 合规与限制 | `customs-compliance` / 各限制源 | 关务路径资格、当前限制和解除结果 | 只能在合格候选中选择 |
| 履约能力信号 | `transport-fulfillment` | 已知班次机会和容量风险及判断时间 | 只参与策略，不绑定或占用资源 |

## 结果语义契约

| 结果 | 业务含义 | 必须保存 | 禁止行为 |
|---|---|---|---|
| 原路由仍适用 | 使用新实际依据复核后，当前计划仍满足硬约束、承诺和策略稳定性 | 触发、输入版本、候选复核、当前计划引用和适用性判断 | 创建语义重复的新计划或覆盖旧判断 |
| 新路由已形成 | 原计划不再适用，或原先无路由且现在有合格候选，新版本已生效 | 原计划/无路由、触发、候选、决定方式、新计划、未执行范围和生效边界 | 修改已执行前缀、承诺或实际事实 |
| 无当前有效路由 | 原计划已经失效或原先无计划，且尚无新的当前有效计划；替代候选可以仍在评估 | 原计划历史、计划失效依据、候选评估状态、判断时点和阻断范围 | 为等待候选继续使用失效计划、占位路线、自动取消、终局失败或默认交出 |
| 改路建议已形成 | 存在候选，但不满足自动改路条件，或自动条件都满足而最低成本并列、选不出唯一一条，需要授权决定 | 建议、原因、候选、影响、当前计划状态和授权入口；并列时原因逐家点名并列候选 | 建议直接改计划或绕过硬限制；并列时按候选标识或任何无业务依据的次序自动改路 |
| 替代候选评估未决 | 当前计划适用性已经独立判断，但候选来源、资格、网络或限制尚不足；或候选在而排序选不出唯一一条（原先无路由的包裹遇最低成本并列时不形成首个计划，记在这一格） | 当前计划适用性结果、已评估候选、缺口和续办引用 | 覆盖已确定的计划失效，或默认选择候选 |
| 已有结果 | 同一触发和输入版本已经处理 | 原适用性、计划、无路由或建议引用 | 重复计划、决定或发布意图 |
| 触发冲突 | 同一触发身份携带不同事实或版本 | 原触发、冲突内容和纠正入口 | 覆盖原判断 |
| 触发未判 | 触发已保全，这一次不形成判断：判断的前提缺了且重投推不动——没有路由历史可复核、判断时点的证据未配置、原计划仍可执行而冻结形态未配置（[ADR-0235](../../adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)）、原计划适用性看不清（选中候选的限制状态未知，[ADR-0250](../../adr/0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)）；或触发的业务时间早于当前控制位置（触发早于当前控制位置，[ADR-0235](../../adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)） | 触发关联、判断键、原因、触发业务时间；适用性看不清时另记取数侧交回的缺口与再判条件；早于当前控制位置时另记比较所用的当前控制位置时刻 | 占着投递回滚重投、空耗派发预算；默认保留、默认失效或选默认候选；据较早的位置失效或改路 |
| 触发未受理 | 来源不构成复核触发：只有普通扫描、位置一类线索，没有节点收寄或权威运输控制依据 | 不落复核、不入账未判；答`TRIGGER_NOT_ACCEPTED` | 建立当前可控节点、据线索失效或改路、回滚重投 |

如果原计划已被硬限制证明失效，即使替代候选暂时无法完成评估，也不能把它继续标记为适用；应提交“原计划已失效 + 无当前有效路由 + 替代候选评估未决”。原计划适用性本身看不清（选中候选的限制状态未知）时入账为`触发未判`、计划与适用性都不动（[ADR-0250](../../adr/0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)）；只有依赖读不动或结果提交技术上不确定时，才答未决、回滚重投。

## 复核顺序

| 层次 | 判断 | 关键约束 |
|---|---|---|
| 1. 触发有效性 | 收寄、控制、实测及业务时间是否权威、当前且可关联 | 普通扫描不建立控制节点 |
| 2. 当前计划适用性 | 实际起点/节点、服务范围、已执行前缀和冻结边界是否与计划相容；场外揽收不比节点、已执行前缀为空、不判冻结（[ADR-0249](../../adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)） | 不修改已经执行部分 |
| 3. 硬约束 | 限制、关务资格、节点/线路/法人资格、实测和安全边界 | 自动和人工都不能绕过 |
| 4. 时间可行性 | 正式承诺、当前截单、日历、处理时间和衔接窗口 | 计划窗口不等于客户承诺 |
| 5. 稳定性策略 | 自动改路条件、冻结边界和改善阈值 | 避免轻微改善导致反复切换 |
| 6. 候选排序 | 路由策略版本声明的内置排序形态；首发只有成本单维，比的是合格候选的候选成本事实 | 不发明固定权重；缺成本的候选出局、不以零顶替；最低成本并列不按候选标识收尾 |

原计划仍可执行时，成本或时效略有改善不能自动切换。只有策略允许、处于当前可控节点、只改变未执行部分，并且没有未处理的订舱、装载、申报或其他既有责任时，才可自动形成改路决定。

## 应用流程

| 步骤 | 责任方 | 应用动作 | 结果与下一步 |
|---|---|---|---|
| 1 | 路由接入 | 保全触发、来源版本、业务时间和包裹关联 | 后续失败不删除触发 |
| 2 | `network-routing` | 识别首次、重复、冲突或来源新版本 | 重复返回原结果；冲突不覆盖 |
| 3 | `network-routing` | 校验有效网络收寄、实际地点和控制依据；节点阶段再校验当前有效实测 | 没有控制依据的线索不受理为复核触发（`触发未受理`），不建立当前可控节点 |
| 4 | `network-routing` | 载入当前计划或无路由结果，固定已执行前缀和本次剩余旅程范围 | 不回写历史路线 |
| 5 | `network-routing` / 资格来源 | 解析当时适用网络、日历、截单、限制、关务资格和策略 | 依赖读不动时未决、回滚重投；判断时点证据未配置时入账`触发未判` |
| 6A | `network-routing` | 独立评估当前计划是否仍可执行 | 形成适用或失效，不等待候选排序；适用性看不清时入账`触发未判`（[ADR-0250](../../adr/0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)） |
| 6B | `network-routing` | 独立评估替代候选 | 形成有候选、无候选、评估未决或冲突 |
| 7A | `network-routing` | 当前计划仍适用时形成适用性判断 | 保留原计划，不制造新版本 |
| 7B | `network-routing` | 满足自动条件时形成改路决定、新计划和新路由指令（路由指令未实现，见「给开发的交接」） | 从明确可控边界和未执行段生效 |
| 7C | `network-routing` / 授权角色 | 不满足自动条件、或自动条件都满足而最低成本并列时形成建议并等待决定 | 建议不改变当前计划 |
| 7D | `network-routing` | 原计划失效且尚无新计划时形成无当前有效路由，同时保存候选评估状态 | 阻止需要下一跳的动作；候选评估可以继续续办 |
| 8 | `network-routing` | 原子提交适用性/决定/计划/无路由、审计和对应发布意图 | 投递失败只重试原意图 |

## 节点安全作业与方向性作业

- 无当前有效路由、复核未决或入账未判、替代候选评估未决时，节点仍可接收、隔离、记录状况、测量和在场核对。
- 分拣到下一方向、建立面向下一段的集运成员、集包出运、装载和交出必须等待当前有效路由指令。
- 新路由生效后形成新的路由指令。旧标签、分拣码或指令不能继续授权未来作业，但原扫描和执行事实必须保留。
- 集运前由本上下文按每个成员当前计划形成集运兼容判断；兼容判断不是实际成员关系。
- 这几条由节点作业端按随对象下发、带有效区间的方向依据执行（[ADR-0024](../../adr/0024-directional-work-basis-carries-a-validity-interval.md)），节点写口对已经发生的作业照记（[ADR-0251](../../adr/0251-directional-node-work-is-paused-by-a-reading-before-the-action-and-recorded-moves-are-never-refused.md)）。路由指令与集运兼容判断尚未实现，见「给开发的交接」。

## 一致性、幂等与并发

- 同一包裹、触发事实版本和复核目的只形成一个业务结果。
- 同一业务时点只能有一个当前有效路由计划；旧计划、候选和建议可以同时保留但不并行生效。
- 收寄、实测、限制、装载和改路并发时按业务时间、生效边界和因果关系裁决，不按消息到达顺序。
- 收寄采用与交接两路触发分属不同分区，复核前比较触发的业务时间与当前控制位置，规则见 [network-routing CONTEXT](../../domain/network-routing/CONTEXT.md)「路由稳定性与受控改路」；比较与随后的落库按包裹的判断键串行，已判过的触发由另一封信封再带来时读回原结果（[ADR-0235](../../adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)）。
- 实际装载先发生时，新计划从下一可控节点生效；改路先有效时，随后按旧计划发生的装载保留为事实并形成偏离。同一时刻与因果不明都不并进这两格。
- 来源更正或当前实测变化形成新复核，不覆盖原判断。更正的那次触发业务时间早于当前控制位置时例外：入账为`触发未判`、不判（`AT-NR-079`、`AT-NR-081`）。
- 重投推不动的前提缺口入账为`触发未判`、留下原因，依赖读不回之类仍回滚重投；携带同一触发的另一封信封再来时重新判一遍，前提补齐了就判；同一封信封入账后由消费门跳过（`AT-NR-068`），今天没有续办手段（[ADR-0235](../../adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)）。
- 结果提交不确定时查询原判断、计划和发布意图，不盲目重算并提交第二计划。

## 权限、隔离与审计

- 判断在明确租户、客户账户、责任法人、包裹和服务范围内执行。
- 自动改路只使用已发布且适用的 `PAR-NET-14` 策略；人工决定必须保存实际决定方、授权、原因和影响范围。策略版本未声明自动改路时只形成建议，不自动改路，答未配置。
- 操作人员不能解除其他来源限制、修改客户承诺或覆盖实际事实。
- 跨客户批量路由、候选和错误信息不得泄露地址、货物、线路选择或业务量。
- 审计至少关联触发来源、包裹、接受基线、正式承诺、当前/原计划、控制和实测、判断 `asOf`、全部输入版本、候选、限制、决定方式、生效边界和发布状态。

## 首发试点叠加条件

- `PAR-NET-01..06` 提供锚点线路、区域、节点、日历、截单和真实作业边界。
- 新增 `PAR-NET-14` 提供初始路由及收寄/实测复核策略、自动与人工边界、冻结边界、改善阈值和重算规则。
- `PAR-CUS-01..04` 只提供候选关务路径资格和当前限制；本用例不建立关务案件或申报。
- `PAR-NET-08/12` 只在策略明确消费班次或容量风险信号时使用，不能因此预占资源。
- `PAR-NFR-01/02/06` 提供真实观察期、规模和路由复核目标。
- 真实参数未确认时可以实现触发/版本、原路由保留、结果类型、显式未配置和命名场景；不得配置默认线路、节点、截单、权重、阈值或自动改路。

## 验收示例

| ID | 场景 | 预期结果 |
|---|---|---|
| `AT-NR-033` | 客户送站形成有效网络收寄，实际节点与初始计划一致；策略版本已声明冻结形态，没有超过改善阈值的候选 | 记录复核依据并保留原路由，不创建重复计划 |
| `AT-NR-034` | 场外揽收交接已采用，尚未到节点；选中候选的硬约束仍满足 | 原计划继续适用；揽收地点原样记入复核依据，不与计划节点比；不建立当前可控节点，不判冻结、不试改善切换，不落「冻结形态未配置」；不虚构节点实测或到站（[ADR-0249](../../adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)；未实现，今天按地点比节点判失效，见 [docs-consistency/code/16](../../../.scratch/docs-consistency/code/16-pickup-reassessment-compares-the-pickup-place-with-plan-nodes.md)） |
| `AT-NR-035` | 有效网络收寄发生在非计划但已确认控制的节点 | 原计划按实际节点与计划不符失效、评估替代候选，形成偏离，不把节点回填进旧计划（偏离未实现，见 [demo-chain-gaps/35](../../../.scratch/demo-chain-gaps/issues/35-network-routing-route-deviation-consumes-movement-facts.md)） |
| `AT-NR-036` | 当前有效实测仍满足原计划硬限制；策略版本已声明冻结形态 | 原计划继续适用，测量事实不被路由复制或修改（实测触发未实现，见「给开发的交接」） |
| `AT-NR-037` | 原线路不再合格（有效硬限制或当前有效实测），包裹在当前可控节点上，存在替代候选且允许自动改路 | 替代候选比失效计划便宜、差额严格大于阈值时形成改路决定与新计划，保留旧计划及已执行前缀；差额不超过阈值时只形成建议，阻塞原因 `IMPROVEMENT_BELOW_THRESHOLD`（[ADR-0173](../../adr/0173-auto-reroute-is-folded-from-the-strategy-version.md) 决定三）。新路由指令与实测触发未实现，见「给开发的交接」 |
| `AT-NR-038` | 原计划失效且没有替代候选 | 形成无当前有效路由，阻止方向性作业但允许安全作业；阻止由节点作业端按下发的方向依据执行（[ADR-0251](../../adr/0251-directional-node-work-is-paused-by-a-reading-before-the-action-and-recorded-moves-are-never-refused.md)，依据下发未实现） |
| `AT-NR-039` | 先前无路由，收寄后出现合格候选 | 形成首个当前有效路由计划，不恢复旧占位（生产接法上复核不取候选成本、走不通，见 [demo-chain-gaps/115](../../../.scratch/demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md)） |
| `AT-NR-040` | 只有普通扫描或卸载，没有控制依据 | `触发未受理`（`TRIGGER_NOT_ACCEPTED`）：不落复核、不入账未判、不回滚重投，不建立当前可控节点 |
| `AT-NR-041` | 存在更优候选但已过冻结边界且原计划仍可执行 | 保留原计划，不因轻微改善自动切换 |
| `AT-NR-042` | 候选可行但不满足自动改路条件 | 只形成建议；授权决定前不改变计划（今天只有失效那条线形成建议，仍适用那条线见 [docs-consistency/code/17](../../../.scratch/docs-consistency/code/17-still-applicable-plan-never-forms-a-reroute-suggestion.md)；授权决定面未实现，见 [docs-consistency/code/29](../../../.scratch/docs-consistency/code/29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md)） |
| `AT-NR-043` | 改路会改变关务区域或报关服务方 | 等待新的关务适用性判断，不沿用原资格 |
| `AT-NR-044` | 单次班次或装载分配失败但线路仍有可行机会 | 保留逻辑路由，交运输履约重新分配 |
| `AT-NR-045` | 同一触发重复处理 | 返回原适用性或改路结果，不重复计划和事件 |
| `AT-NR-046` | 同一触发身份携带不同实测版本 | 形成冲突或关联新复核，不覆盖原判断 |
| `AT-NR-047` | 路由复核与实际装载并发 | 按业务边界保留实际事实，新计划从合法可控边界生效 |
| `AT-NR-048` | 结果提交成功但发布失败 | 不回退计划或重复决定，只重试原发布意图 |
| `AT-NR-049` | 跨租户查询路由复核 | 不泄露包裹、路线、候选或限制信息 |
| `AT-NR-050` | 硬限制已证明原计划失效，但替代候选来源暂时不可用 | 原计划退出当前有效范围，形成无当前有效路由并保持候选评估未决；不继续下发旧指令 |
| `AT-NR-051` | 选中候选的限制状态未知：关务目录为空、受理基线缺端点国家或关务来源未接 | `触发未判`，原因 `PLAN_APPLICABILITY_UNCLEAR`，未判登记记下缺口与再判条件；不回滚重投，既不默认失效也不形成新计划（[ADR-0250](../../adr/0250-unclear-plan-applicability-in-reassessment-is-settled-as-not-judged-and-resume-in-the-same-change-stays-in-the-acceptance-chain.md)；未实现，今天答未决、回滚重投，见 [docs-consistency/code/28](../../../.scratch/docs-consistency/code/28-unclear-plan-applicability-in-reassessment-is-retried-instead-of-settled.md)） |
| `AT-NR-052` | 原计划失效，候选评估确认没有合格候选 | 无当前有效路由继续有效，候选评估形成无候选结果 |
| `AT-NR-053` | 原计划失效，候选评估未决后补齐并找到合格候选 | 保留无路由期间，形成新计划版本并从明确边界生效（生产接法上复核不取候选成本、走不通，见 [demo-chain-gaps/115](../../../.scratch/demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md)） |
| `AT-NR-059` | 当前计划末尾带交付段，包裹在某个计划节点上复核 | 已执行前缀与冻结边界的剩余段数只数节点间段，交付段不算剩余段；改路形成的新计划照被选候选带上它的交付段（[ADR-0217](../../adr/0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md)） |
| `AT-NR-064` | 权威运输交接以`已交接`把当前已接受包裹交进声明的接收节点 | 形成一条复核：控制依据为权威运输交接，位置为接收节点，来源版本与业务时间取交接；所携地理投影取受理基线那一版提交的，摘要与那一版相同；接收节点不在计划上时原计划按实际节点与计划不符失效，失效依据点名该节点（[ADR-0215](../../adr/0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)） |
| `AT-NR-065` | 交接是拒收或待确认，或`已交接`却没声明接收节点（交给运输方） | 入账，不形成复核，也不读小包托运 |
| `AT-NR-066` | 交接对象不是当前已接受包裹：没有已接受委托声明它、受理基线里没有它，或是集运单元号 | 入账，不形成复核，不停在未决等它出现 |
| `AT-NR-067` | 交接还读不回、小包托运此刻读不动，或复核停在依赖类未决（登记册或路由历史读不回、证据读不动、取号依赖故障） | 回滚重投（本路未决哨兵），不入账、不落复核、不落未判；交接与键不符、已接受委托自相矛盾或反查歧义响亮失败，不进未决 |
| `AT-NR-068` | 同一版交接重复投递，或同一版交接另一封信封 | 消费门跳过重投；另一封答已有结果，仍只一条复核 |
| `AT-NR-069` | 交接被更正：仍`已交接`，或改成拒收 / 待确认 | 仍`已交接`的新版本承继接收节点、另成一次复核，原复核不被覆盖；改成拒收或待确认入账不复核，也不回退已有复核。更正的那次交接业务时间早于当前控制位置时入账不判，见 `AT-NR-079`、`AT-NR-081` |
| `AT-NR-074` | 收寄采用或交接触发复核，这件已接受包裹没有路由历史 | `触发未判`：入账，原因 `NO_ROUTING_HISTORY` 与判断键、触发业务时间落未判登记，不落复核，不登本路未决哨兵（[ADR-0235](../../adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)） |
| `AT-NR-075` | 判断时点没有适用的网络证据（证据未配置） | `触发未判`，原因 `REASSESS_EVIDENCE_NOT_CONFIGURED`；证据读不动是依赖故障，仍按 `AT-NR-067` 回滚重投 |
| `AT-NR-076` | 原计划仍可执行，策略版本没声明冻结形态 | `触发未判`，原因 `FREEZE_FORM_UNCONFIGURED`；不当成未冻结，原计划与适用性都不动 |
| `AT-NR-077` | 生产路由表上，没有路由历史的已接受包裹交进节点，随后同一对象再交接一次 | 两封交接信封都定稿 `PUBLISHED`，复核这一路处理完毕、各落一行未判；不再占着交接分区重投到 `ABANDONED` |
| `AT-NR-078` | 入账未判之后，携带同一触发的另一封信封再来 | 重新判一遍：前提仍缺则再入账、未判登记只留先到那一行；前提补齐则形成一条复核；原因落不了库时不入账，答未决回滚重投 |
| `AT-NR-079` | 包裹已凭较晚的交接在计划节点上判过，一次更早的交接（进一个不在计划上的节点）后到，或更早那次交接被更正（仍`已交接`，承继原判断时刻） | `触发未判`，原因 `TRIGGER_PRECEDES_CURRENT_POSITION`，未判登记记下比较所用的当前控制位置时刻；原计划不因那个已经过去的位置失效，复核册只有较晚那一次；另一封信封带来同一触发同答（[ADR-0235](../../adr/0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)） |
| `AT-NR-080` | 包裹已凭交接判过，一份更早的有效网络收寄采用后到 | 收寄采用一路同一条判据：`触发未判`、早于当前控制位置，不判；收寄采用信封照常入账 |
| `AT-NR-081` | 触发的业务时间晚于当前控制位置，或与当前控制位置同一时刻（含仍`已交接`的交接更正，它承继原交接的判断时刻） | 照判；已判过的触发在被更晚的复核超过之后由另一封信封再带来，读回原结果，不改答早于当前控制位置 |
| `AT-NR-082` | 同一包裹的收寄采用（较早）与交接（较晚）按两种顺序到达 | 按业务时间到达时两次都判；较早的后到时它入账不判。两种顺序下业务时间最晚的已判复核都是交接那一次，当前控制位置不倒退；后到的那一次由另一封信封再带来，答案不变 |
| `AT-NR-083` | 同一判断键的两次复核在两笔并发事务里读当前控制位置 | 按判断键串行：后一笔等前一笔提交，读到的是它刚落的那次复核；读当前控制位置不在事务里时拒 |
| `AT-NR-084` | 场外揽收复核时原计划被有效硬限制淘汰，存在替代候选，策略版本允许自动改路 | 原计划失效、形成无当前有效路由；只形成改路建议，阻塞原因含 `NOT_AT_CONTROLLED_NODE`、不含 `EXECUTED_PREFIX_AFFECTED`（已执行前缀为空）；授权决定从包裹交进的首个节点生效（[ADR-0249](../../adr/0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)；未实现，见 [docs-consistency/code/16](../../../.scratch/docs-consistency/code/16-pickup-reassessment-compares-the-pickup-place-with-plan-nodes.md)） |

## 给开发的交接

实现必须把“触发事实、当前计划适用性判断、候选、建议、改路决定和新计划”分别建模，不能使用一个可覆盖的 `route_status`。

没有 `PAR-NET-14` 时不得启用自动改路或默认阈值。

已实现的是收寄采用与交接两路触发、计划适用性复核、替代候选评估、自动改路与失效线上的改路建议、入账未判与按业务时间比当前控制位置。本用例下列各块尚未实现，验收行里以现行口吻写到它们的，以本表为准：

| 能力 | 现状 | 承接 |
|---|---|---|
| 场外揽收复核只按硬约束判、不比节点、不建可控节点（`AT-NR-034`、`AT-NR-084`） | 今天拿揽收地点比计划节点，原计划被误判失效 | [docs-consistency/code/16](../../../.scratch/docs-consistency/code/16-pickup-reassessment-compares-the-pickup-place-with-plan-nodes.md) |
| 适用性看不清入账未判（`AT-NR-051`） | 今天答未决、回滚重投 | [docs-consistency/code/28](../../../.scratch/docs-consistency/code/28-unclear-plan-applicability-in-reassessment-is-retried-instead-of-settled.md) |
| 复核取候选成本（`AT-NR-039`、`AT-NR-053`、步骤 6B/7B/7C） | 复核没有成本取数口，有合格候选时排序报错或候选评估一律记未决 | [demo-chain-gaps/115](../../../.scratch/demo-chain-gaps/issues/115-route-reassessment-never-loads-candidate-costs.md) |
| 仍适用线形成改路建议（步骤 7C、`AT-NR-042`） | 只有失效线形成建议 | [docs-consistency/code/17](../../../.scratch/docs-consistency/code/17-still-applicable-plan-never-forms-a-reroute-suggestion.md) |
| 授权角色据建议或并列作出改路决定（步骤 7C、CONTEXT「授权角色接受建议 → 形成改路决定」） | 没有决定面，建议只能看 | [docs-consistency/code/29](../../../.scratch/docs-consistency/code/29-reroute-suggestions-and-ties-have-no-authorized-decision-face.md) |
| 路由指令（步骤 7B、`AT-NR-037`、「节点安全作业与方向性作业」） | 没有路由指令的模型 | [no-directional-basis/02](../../../.scratch/no-directional-basis/issues/02-network-routing-has-no-routing-instruction-or-consolidation-compatibility-judgment.md) |
| 集运兼容判断（「节点安全作业与方向性作业」） | 没有成员级集运兼容判断的模型 | [no-directional-basis/02](../../../.scratch/no-directional-basis/issues/02-network-routing-has-no-routing-instruction-or-consolidation-compatibility-judgment.md) |
| 路由偏离（`AT-NR-035`） | 没有偏离的模型 | [demo-chain-gaps/35](../../../.scratch/demo-chain-gaps/issues/35-network-routing-route-deviation-consumes-movement-facts.md) |
| 节点实测触发复核（元数据「触发」的当前有效实测、`AT-NR-036`、`AT-NR-037`） | `node-operations` 没有实际测量登记册，复核的触发今天只有收寄采用与交接；NR 侧消费票随实测登记册另立 | [pp-pricing-input-seams/04](../../../.scratch/pp-pricing-input-seams/issues/04-no-actual-measurement-registry-and-valid-measurements-read-port.md) |
| 复评新计划的发布意图与运输准备消费 | 新计划落库，不另发信封 | [demo-chain-gaps/34](../../../.scratch/demo-chain-gaps/issues/34-network-routing-replanned-route-envelope-and-tf-preparation-consumer.md) |
| 入账未判之后的续办 | 没有续办手段 | [demo-chain-gaps/154](../../../.scratch/demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md) |

完成本用例只证明实际收寄和节点实测能够驱动受控路由复核。它不证明班次、容量、装载、运输交接或实际移动已经形成。
