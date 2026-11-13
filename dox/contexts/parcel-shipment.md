# 小包托运（`parcel-shipment`）

代码在 `internal/parcelshipment`。对照 main `99fc0037d`（2026-11-12）写成。

**这份文件核到什么程度**：取值表、委托与接受判断任务的状态、答复码、入口四节是从代码里读出来的。「管什么」「词」「规则」「生命周期」的句子是从旧 `docs/domain/parcel-shipment/CONTEXT.md` 提炼的，每条只用搜索确认了代码里有对应的机制（类型、函数或码），没有逐条跑测试验证行为；旧文档写了而代码里没有的，单列在最后一节。

## 管什么，不管什么

管：委托及其提交版本、接受判断任务、接受或拒绝决定、撤回、委托接受基线、包裹身份、客户原始资料及其版本、面单交易、渠道择优决定、面单继续尝试决定、外部标识关联、包裹取消、收寄后服务处置决定、包裹终局服务结果与委托完成汇总、预计承诺与正式承诺。

不管：

- 客户、合同、服务产品、接单规则包、授权规则、渠道产品与映射——`party-commercial`。本上下文只保存采用的依据，不临时发明规则或授权。
- 生产归属——试点准入控制。它不是委托接受或拒绝。
- 节点收寄、测量、装袋、集运单元——`node-operations`。
- 场外揽收、交接、实际承运商收寄、运输、交付——`transport-fulfillment`。
- 路由计划与计划履约段的外包伙伴择优——`network-routing`。
- 正式申报、监管结果、关务限制——`customs-compliance`。
- 全程追踪投影、异常案件——`visibility-exception`。本上下文不维护「运输中、清关中、派送中」这类主状态。
- 估价、冻结、信用暴露、费用与金额——`settlement-accounting`；价格评价——`parcel-pricing`。
- 凭据验证、通用授权策略。本上下文只消费传入的授权查询作用域。

## 词

| 词 | 意思 |
|---|---|
| 委托 | 客户要求为一个或多个包裹提供同一项服务的业务请求。不是提交批次（一次提交多份委托的处理归组）、集运单元或结算批次 |
| 委托提交版本 | 决定形成前对请求内容形成的不可覆盖版本；同一时刻只有一个待判断的当前版本 |
| 委托接受 | 运营企业明确同意承担该请求的决定。数据到达、导入、实物收寄都不等于接受 |
| 接受判断任务 | 针对当前提交版本的可续办判断工作。不是委托状态；任务未完成时委托仍为`已提交` |
| 控制项结果 | 接受前财务控制里一项控制（预付冻结或信用校验）的结论。成立即表示提供方账本上有占用 |
| 授权处置 | 财务控制不通过且策略交给授权角色时，对去向的决定：拒绝，或交客户补充。从不形成接受 |
| 委托撤回 | 客户在决定形成前终止整份待决请求。不是运营企业拒绝 |
| 委托接受基线 | 接受时形成的不可覆盖快照：成员、依据、资料 |
| 客户原始资料 | 客户声明的寄收件、货物、申报与服务要求。不是节点实测、正式申报或计费资料 |
| 客户原始资料版本 | 接受后经补充/更正请求形成的不可覆盖来源版本 |
| 当前客户资料版本采用判断 | 某资料范围上当前可供下游消费的引用，派生而来，不可手工覆盖 |
| 地址要素、声明测量 | 国家/地区码与邮编；客户给的毛重、可选尺寸与单位。都原样保存，不校验、不换算、不算体积重 |
| 货物明细 | 客户对一份委托装了什么的声明：内件性质、货物明细行、装箱分配 |
| 收件地点引用 | 对外交出的复合引用（租户、委托、收件资料范围、资料版本锚）。地址本体留在本上下文 |
| 来源请求身份 | 租户、客户账户、来源、来源请求键四段。只用于重放与冲突分类，不授予权限 |
| `PayloadDigest` | 规范化业务内容摘要。覆盖 `requestEffectiveAt` 的值与有无，不覆盖 `occurredAt`、`receivedAt`，也不由它们补齐 |
| 包裹 | 有独立身份、可被分别收寄、跟踪并形成服务结果的物理服务件。不是委托行、面单交易或号码 |
| 面单交易 | 向渠道服务方发起的一次渠道业务请求及其结果，可覆盖多个包裹；每个包裹的结果分别保存。结果都已明确叫定案 |
| 渠道择优决定 | 一次面单渠道择优的只追加记录：候选、规则、结论、逐候选结果 |
| 面单继续尝试决定 | 针对包裹的追加式决定（受控关闭或重开），控制今后能否发起新的面单尝试。关闭带权威业务截断边界，裁决并发的新尝试是否合法 |
| 外部标识关联 | 外部号码与被标识对象之间的版本化关系，只追加 |
| 终局服务结果 | 运营企业对某包裹的本次服务责任已形成最终业务结果 |
| 有效取消 | 已成立、且未被取消先后冲突决定认定依据不成立的包裹取消 |
| 取消先后冲突决定 | 取消已提交之后才到、发生却不晚于取消的边界事实，由运营主体作出的解开决定 |
| 收寄后服务处置决定 | 包裹越过取消边界后对去向的决定。不等于执行已发生，也不直接形成终局 |
| 待人工处理项 | 处置决定里系统不替对象所有者执行的请求项，由运营人员登记处理结果 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 生产归属 | `IDP_PARCEL` `OTHER` `UNRESOLVED` |
| 接受校验组 | `CUSTOMER_RELATIONSHIP` `LEGAL_ENTITY_AND_CONTRACT` `PRODUCT_AND_SERVICE` `MEMBER_BASELINE` `REQUIRED_DOCUMENT` `PRE_ACCEPTANCE_FINANCIAL_CONTROL` `NETWORK_REACHABILITY`；结论 `PASSED` `FAILED` `UNDETERMINED` |
| 提交条件 | `PLATFORM_SHIPMENT_DECLARED` `EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL` `PACKING_DECLARED` |
| 控制项 | `PREPAID_FREEZE` `CREDIT_CHECK`；结论 `SATISFIED` `RESTRICTED`；共同通过条件 `ALL_CONTROLS_PASS`；失败处置 `REJECT` `AUTHORIZED_DISPOSITION` |
| 财务控制接受侧结论 | `HELD` `CREDIT_EXPOSED` `RESTRICTED` `NOT_APPLICABLE`；授权处置去向 `REJECT` `CUSTOMER_SUPPLEMENT` |
| 委托关联方向 | `REJECTED_CORRECTION` `ACCEPTED_RESHAPING` `WITHDRAWN_RESUBMISSION` |
| 资料修订阶段 | `ACCEPTED_NOT_YET_RECEIVED` `RECEIVED_OR_MEASURED` `LABELLED_OR_BAGGED` `CUSTOMS_DATA_FORMING_NOT_SUBMITTED` `CUSTOMS_SUBMITTED` `CASE_CLOSED_OR_SERVICE_COMPLETED` |
| 修订意图 | `SUPPLEMENT` `CORRECTION` `EXPLICIT_CLEAR`；资料采用判断 `ADOPTED` `AWAITING_REVIEW` |
| 地址要素 | `COUNTRY_CODE` `POSTAL_CODE` |
| 内件性质 | `GIFT` `DOCUMENTS` `COMMERCIAL_SAMPLE` `RETURNED_GOODS` `SALE_OF_GOODS` `OTHER`；申报格来源标记 `THIS_DECLARATION` `BOX_LIST` `COMMODITY_DECLARATION` |
| 外部标识类型 | `CUSTOMER_PARCEL_NUMBER` `CHANNEL_PARCEL_NUMBER` `PLATFORM_BOX_LABEL`（三种都标识包裹）；关联终态 `VOIDED` `REPLACED` |
| 面单交易状态 | `ESTABLISHED` `SUBMITTED_TO_CHANNEL` `RESULT_UNCERTAIN` `SUCCEEDED` `PARTIALLY_SUCCEEDED` `FAILED` |
| 面单交易关系与后续动作 | 关系 `RETRY` `REPLACEMENT`；动作 `CHANNEL_VOID` `CHANNEL_REFUND` `REPLACEMENT` |
| 渠道择优 | 规则 `COST_ONLY`；结论 `SELECTED` `TIED` `NONE_QUALIFIED`；逐候选 `SELECTED` `NOT_SELECTED` `EXCLUDED` `TIED` |
| 继续尝试决定 | `CONTROLLED_CLOSURE` `REOPENING`；派生判断 `OPEN` `CONTROLLED_CLOSED` |
| 面单服务终局判断 | `FINAL_BY_FIRST_PICKUP` `FINAL_OUTCOME_BY_CLOSURE` `FINAL_FAILURE_BY_CLOSURE` `NOT_FINAL` `CANCELLATION_STANDS` |
| 收寄来源与取消边界 | 来源 `NODE_INTAKE` `OFFSITE_PICKUP`；边界 `NETWORK_INTAKE` `CARRIER_FIRST_EFFECTIVE_PICKUP`；取消先后冲突决定 `CANCELLATION_NOT_UPHELD` `BOUNDARY_FACT_REFERRED` |
| 处置去向 | `CONTINUE` `INTERCEPT` `REDIRECT` `RETURN` `TERMINATE_SERVICE`；来源 `CUSTOMER_REQUEST` `OPERATOR_INITIATED`；请求的对象所有者 `NETWORK_ROUTING` `NODE_OPERATIONS` `TRANSPORT_FULFILLMENT`；待人工处理项结果 `HANDLED` `UNABLE` |
| 终局来源结果 | `EFFECTIVE_DELIVERY` `RETURN_COMPLETED` `SERVICE_TERMINATED` `REGULATORY_DISPOSITION_EXECUTED` `LABEL_SERVICE_OUTCOME` `LABEL_SERVICE_FAILURE`；委托完成度 `NONE` `PARTIAL` `COMPLETE` |

## 委托状态与接受判断的等待

委托只有四个存下来的状态（`ShipmentRequestState`）：

| 状态 | 什么时候 | 之后 |
|---|---|---|
| `SUBMITTED` 已提交 | 取得生产归属并建立委托 | 可形成新提交版本；等接受、拒绝或撤回 |
| `ACCEPTED` 已接受 | 当前提交版本全部校验通过，规则要求的人工复核已完成 | 不回退。`已取消`、`已完成`由包裹结果派生，不另存状态 |
| `REJECTED` 已拒绝 | 规则确定接受条件不成立，或授权角色主动拒绝、授权处置选拒绝 | 终态；修正走关联新委托 |
| `WITHDRAWN` 已撤回 | 客户在决定形成前撤回整份请求 | 终态；再要服务走关联新委托 |

接受判断任务（`AcceptanceTaskState`）是 `RUNNING`、`COMPLETE`（决定已越过提交边界）或 `STOPPED`（委托已撤回）。任务在跑而停下时，按续办方分五条路（`ResumePath`），不得合并。技术或依赖未决不得伪装成业务拒绝；任一所需结果无法确定时委托保持`已提交`，不按默认放行。

| 续办路 | 等谁 | 什么时候 |
|---|---|---|
| `CUSTOMER_SUPPLEMENT` 等待受控补充 | 客户 | 客户可补的资料缺口；或授权处置选交客户补充 |
| `INTERNAL_RETRY` 等待内部续办 | 系统 | 依赖失败、并发冲突、权威一时答不出 |
| `MANUAL_REVIEW` 等待人工复核 | 授权复核角色 | 结果都已通过，规则要求人工复核。复核完成本身不形成决定 |
| `OPERATOR_REGISTRATION` 等待运营登记 | 运营企业的登记动作 | 适用参数没登记，或登了却答不出，重试永远产不出 |
| `AUTHORIZED_DISPOSITION` 等待授权处置 | 授权处置角色 | 财务控制不通过，且全部受限项的失败处置都是进入授权处置 |

常见答复码：提交 `SUBMITTED` `EXISTING_RESULT` `SOURCE_CONFLICT` `OWNERSHIP_UNRESOLVED` `OTHER_PRODUCTION_AUTHORITY` `ADMISSION_PAUSED` `LINK_INELIGIBLE` `PRIOR_REQUEST_NOT_FOUND` `NUMBERING_RULE_COLLISION` `INPUT_NOT_ACCEPTED`；撤回与决定竞争 `DECISION_ALREADY_FORMED`；包裹取消 `PARCEL_CANCELLED` `CANCELLATION_REFUSED` `CANCELLATION_UNDECIDED` `REQUEST_ENDED`（委托已终结） `REQUEST_AWAITING_DECISION`（委托待决定） `DISPOSITION_PENDING` `FINAL_OUTCOME_STANDS`；通用 `NOT_AUTHORIZED` `REQUEST_CONFLICT` `REQUEST_UNAVAILABLE`。

## 规则

**委托与提交**

- 委托只在本产品对完整拟受理范围取得生产归属之后建立；其他权威承接、归属未决、准入暂停都不建立委托。
- 来源请求身份四段整体比较。同身份同摘要是重放，答原结果；同身份不同内容是冲突。`occurredAt`、`receivedAt` 变了仍是重放。
- 提交时在同一事务里取单据号（类型 `SHIPMENT`）；重放答原号，新提交版本不换号。租户没登单据编号时区时照常提交、不带号；撞号这一次不建立委托。
- 查询只认传入的授权查询作用域。不存在、越界、不覆盖答统一不可见结果；可见但无动作授权答未获授权。

**接受、拒绝、撤回**

- 提交与接受是两个事件。全部硬规则与权威判断通过时自动接受；只有规则显式要求时才进人工复核。人工处理不得把缺的结果改成通过。
- 一份当前提交版本只形成一个委托级决定，不支持成员级部分接受：任一成员确定性不满足，整份不接受。接受时至少有一个声明包裹，并固定委托接受基线、预计承诺与采用的依据；之后不得静默覆盖。
- 接受、拒绝、撤回竞争同一个决定边界，先提交的获胜，后到的只能读既有结果。
- 接受或拒绝决定无论经哪条路形成，都在同一事务里交出「接受决定已形成」信封。撤回不形成决定，不交这封信封。
- 已提交期间的纠错或补充形成同一委托的新提交版本并重新判断。成员重组或共享约束变化不是新版本：先撤回原委托，再提交带关联的新委托。

**接受前财务控制与占用**

- 本上下文只记逐项控制项结果与按共同通过条件形成的结论，不拥有冻结、信用暴露或金额。
- 接受确定不成立时（拒绝、撤回、授权处置、停进等待受控补充、版本被取代），已成立各项的占用按原业务关联请求释放；被取代版本的占用放掉之前新版本不判。已接受的委托绝不放。
- 释放的答复分三种：已收口、可重试未形成（续办）、确定未受理（不续办，记入释放未受理册交人处理）。册上的行由运营在结算侧释放或标记已处理，处理后接受链再驱一拍。

**客户原始资料**

- 接受后的补充或更正经受控请求形成新的资料版本，不覆盖接受基线与旧版本，不增删成员，也不自动解除限制或重开案件；下游各自消费新版本重新判断。
- 资料修订阶段多格同时成立取靠后的一格；所需事实不可知时请求停在未决，不默认为最早阶段。允许什么由接单规则包的资料修订允许声明说，本上下文不自带矩阵。基础版本过期且与当前版本在同一范围重叠时停在`待复核`，不按最后到达覆盖。
- 平台货件（如专线型 FBA 头程）是普通委托：一个平台货件一份委托、一箱一个包裹，差异由服务产品的接单规则承载，不设标志。场景见 [scenarios.md](../scenarios.md)。
- 货物明细按委托整组替换，至多 999 行。本上下文只核形状，不核单价乘数量、净重或价值；装箱分配要么不报，要么报全。
- 收件地点引用按（租户，包裹）答：范围上无修订答接受基线锚，`已采用`答该版本锚，其余答收件地点未定。引用一经交出含义不变。

**包裹取消、收寄与终局**

- 包裹取消与终局形成只对`已接受`的委托开放；`已撤回`、`已拒绝`答「委托已终结」，`已提交`答「委托待决定」。
- 网络服务包裹只能在有效网络收寄形成前取消；面单渠道服务包裹只能在实际承运商首次有效收寄前取消。越界后不得回退为已取消，只能走收寄后服务处置。
- 取消与边界事实的先后按两者的业务时间比，消息到达顺序不参与。取消已提交之后到达、发生不晚于取消的边界事实是取消先后冲突：不形成责任起点或非取消终局，也不自动改写取消。包裹已有有效取消时，之后到达的履约结果不采用为终局。
- 冲突只由持授予的运营主体的一份决定解开，去向封闭为两种；一份冲突至多一个决定。谁持有授予是租户取值。
- 有效网络收寄来自节点收寄或场外揽收，由本上下文统一解释并形成包裹级正式承诺，每个包裹独立判断；正式承诺引用而不覆盖预计承诺。
- 载运对象在来源发生那一刻是节点作业册上的集运单元时，不采用为收寄，也不判终局。是不是单元只由那本册说，不按串形猜。

**收寄后服务处置**

- 处置按包裹形成明确决定，去向封闭五种。客户消息、异常信号、拒收都不能自动生成退运或服务终止。
- 决定只向对象所有者提出范围化请求。系统有执行器的由所有者承接；其余同笔落成待人工处理项，每项至多登记一份结果，不改不删。

**面单交易与继续尝试（写链无生产入口，见「入口」）**

- 交易结果与逐包裹结果分别保存，互不覆盖。渠道结果不确定不等于失败；交易级失败不推导包裹失败，面单成功也不自动构成包裹终局。
- 面单服务终局跨该包裹全部相关交易判断：实际承运商首次有效收寄即终局；否则须受控关闭已生效且未重开、全部交易已定案、没有有效或待确认的面单结果。
- 受控关闭与重开由授权的运营角色明确形成，系统不自动形成，决定只追加。关闭只拒绝截断边界之后的新尝试，不动既有交易。当前存在有效终局时不得重开。原关闭因货主指令形成的，重开要同一货主账户新的授权证据。
- 一次择优形成一条渠道择优决定，重跑形成新记录。只按成本比，不拷金额；候选成本币种不齐时不形成记录。

**外部标识**

- 关联只在被标识对象成为正式对象时形成：客户包裹件号与平台箱标号随委托接受，渠道包裹件号随面单交易包裹结果受理。
- 按号码核对包裹身份逐字比对。命中多件如实交回多个候选，不挑；逐候选答范围相容与歧义信号。
- 委托由`已提交`转为`已拒绝`或`已撤回`时，按它声明的每个号向节点作业交一次声明撤除。包裹取消或终局不改变关联的适用状态。

## 生命周期

- **委托**：已提交 →（新提交版本，可多次）→ 已接受、已拒绝或已撤回，三者单向、互不转换。已接受之后只按包裹结果派生完成度，不能人工设置；全部成员都是取消终局时派生为已取消。
- **客户原始资料版本**：收到请求先保全 → 受理后形成版本 → 派生采用判断 → 下游各自重新判断。
- **包裹**：接受时进入成员基线 → 取消边界前可取消 → 有效网络收寄形成正式承诺 → 越界后只能处置 → 终局服务结果。
- **面单交易**：建立 → 已提交渠道 →（结果不确定）→ 成功、部分成功或失败；之后可追加渠道作废、渠道退款、替代。
- **面单继续尝试**：开放 ⇄ 受控关闭；有效终局存在时禁止重开。 **外部标识关联**：适用 → 已作废或已被替代，两者都是终态。

## 入口

在线口（路径在 `cmd/parcel-api` 的 `endpoints_*.go`，处理器在 `adapters/http`）：

| 做什么 | 路径 |
|---|---|
| 客户：提交、补充、撤回 | `/v1/shipment-requests` `/v1/shipment-requests/supplements` `/v1/shipment-requests/withdrawals` |
| 客户：资料修订、包裹取消 | `/v1/shipment-requests/source-data-amendments` `/v1/shipment-requests/parcel-cancellations` |
| 运营：复核、拒绝、授权处置 | `/shipment-requests/manual-review-completions` `/shipment-requests/rejections` `/shipment-requests/authorized-dispositions` |
| 运营：受控关闭与重开 | `/shipment-requests/continued-attempt-closures` `/shipment-requests/continued-attempt-reopenings` |
| 运营：取消先后冲突 | `/cancellation-order-conflicts` `/cancellation-order-conflict-decisions` |
| 运营：释放未受理册 | `/control-release-refusals` `/control-release-refusal-handlings` |
| 运营：收寄后处置 | `/post-intake-disposition-decisions` `/post-intake-disposition-requests` `/post-intake-disposition-manual-items` `/post-intake-disposition-manual-item-results` |
| 运营：只读 | `/shipment-request-views` `/acceptance-review-queue` `/authorized-disposition-queue` `/label-transactions` `/channel-selection-decisions` |

后台（`cmd/parcel-dispatch`，按节拍消费信封，不监听端口）：推进接受链，采用节点收寄与场外揽收，形成包裹终局，判面单服务终局，承接收寄后处置，续办取消。

面单渠道写链（择优 → 建立 → 提交渠道 → 记录结果）在 `cmd/parcel-api` 启动时装配，但今天没有端点也没有进程内触发面调它；上线的只有面单交易与择优决定的只读口。

## 旧文档写了、代码里还没有的

- **包裹身份谱系**（真实拆分、合并形成新包裹身份）：领域里没有模型，代码注释自己写明「谱系今天在领域里还没有模型」；当前有效包裹恒等于接受基线成员。
- **当前资料版本采用判断的`待补充`、`冲突`、`未决`三格**：`SourceDataAdoptionOutcome` 只有 `ADOPTED` 与 `AWAITING_REVIEW`。修订意图「来源更正与撤销关系」同样没有，`AmendmentIntent` 只有三格。
- **标识委托或面单交易的外部标识关联**（客户参考号、平台货件号、平台参考号、渠道订单号或主号）：`ExternalIdentifierKind` 只有三种包裹级类型。平台货件声明随委托保存，但不形成关联。
- **来源「仓配出库」与 `warehouse-fulfillment` 代办**：搜 `Warehouse`、`仓配`，只有申报格来源标记的两个取值，没有这一来源或代办授权。属可选的仓配履约，已搁置。
- **保价要求**、**代收货款要求进接受基线**：搜 `保价`、`Insur`、`代收货款`、`CashOnDelivery`，没有对应类型。
- **补充期限届满复核形成确定性拒绝**：搜 `Deadline`、`届满`、`补充期限`，只有一处注释提到。
- **客户指定渠道产品时锁定该次选择**：搜 `锁定`、`Specified`、`Pin`，择优代码里没有。
- **面单服务正式承诺**：`FormalCommitment` 只在有效网络收寄上形成，没有面单渠道受理这一路。
- **重开前硬限制须由来源责任方解除**：重开编排注释写明这一格的「解除」今天没有来源可读。
- **批量关闭、重开、取消的归组**：端点一次只受理一件包裹，批量由调用方逐件分发；提交批次有模型（`SubmissionBatchCandidate`）。委托的「草稿」也不是存下来的状态，`ShipmentRequestState` 从已提交开始。
