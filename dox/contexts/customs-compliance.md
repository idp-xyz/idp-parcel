# 关务与贸易合规（`customs-compliance`）

代码在 `internal/customscompliance`。对照 main `99fc0037d`（2026-11-12）写成。

**这份文件核到什么程度**：取值表、动作准入判断的答复与成因码、入口三节是从代码里读出来的。「规则」一节的句子是从旧 `docs/domain/customs-compliance/CONTEXT.md` 提炼的，每条只确认了代码里有对应的机制（类型、函数、测试名或码），没有逐条跑测试验证行为。有几块规则只有用例代码、没有接进任何进程，列在「入口」末段；旧文档写了而代码里没有的，单列在最后一节。

## 管什么，不管什么

管：关务案件、申报单元、提交版本与提交尝试、申报发送、外部监管结果、监管凭证、监管限制与内部合规限制、税费付款协作与核对、放行门禁核对、动作准入判断、关务适用性判断、案件关闭与重开，以及这些判断所读的登记册（口岸、申报路径、节点关务区域、解释规则、门禁目录等）。

不管：

- 客户委托、包裹身份、货物明细与客户原始申报资料——`parcel-shipment`。本上下文只读、只引用，不覆盖。
- 节点收寄、实测、隔离、开封、销毁等执行事实——`node-operations`。
- 班次、运输总单、运输舱单、装载与实际移动——`transport-fulfillment`。
- 线路、路由计划与候选选择——`network-routing`。本上下文只答候选能不能过关务边界。
- 实际付款、资金退回、代垫成立与客户代垫回收——银行或支付系统与 `settlement-accounting`。
- 参与方身份、角色权限与委派——`party-commercial`。
- 外部监管舱单的形成与提交——承运商。本上下文只保存引用。
- 最终监管决定——监管机构。内部判断不替代它。

## 词

| 词 | 意思 |
|---|---|
| 关务案件 | 围绕固定的监管辖区、进出口方向、监管程序和法定义务范围建立的案件。出口、进口各自建案 |
| 申报单元 | 作为一次申报、查验、放行对象的业务集合。不是客户委托、总单、舱单或班次 |
| 申报就绪判断 | 明确申报范围已具备申请提交的条件。不等于已授权、已提交、已受理或已放行 |
| 提交授权 | 允许把明确范围和资料版本正式提交的独立授权 |
| 提交版本 | 首次对外发送前固定的不可覆盖快照。它存在不证明发送或受理成功 |
| 提交尝试 | 针对一个提交版本的一次对外发送。越过发送边界之前是`已登记待发送` |
| 申报发送通道 | 一个发送目标的出向连接：端点与凭据引用，都是租户取值 |
| 技术回执 | 对连接、报文接收或技术校验的反馈，归属提交尝试。不是外部结果的一层 |
| 放行结果 | 监管机构对明确范围的全部、部分或附条件放行。不能由技术成功、受理、付款或内部解除推导 |
| 监管处置决定 | 监管机构作出的隔离、移交、销毁、退运等决定。不等于已执行 |
| 外部监管舱单引用 | 对承运商在产品之外形成并提交的监管舱单保存的身份、版本、范围与关联 |
| 后续申报动作目标 | 对已提交申报形成的补充、更正、撤销或重报目标。只是拟执行意图 |
| 监管凭证 | 许可证、证书等监管资格凭据。有独立身份和不可变版本；附件不代替它 |
| 监管限制 | 依据监管过程决定对明确对象形成的阻断，列明所约束的动作 |
| 内部合规限制 | 运营企业依据贸易合规规则形成的内部阻断决定。与监管限制互不改名、互不解除 |
| 限制责任来源／限制覆盖项 | 对一项内部合规限制及其解除负责的来源；该限制对明确对象、动作和时间的一条独立覆盖 |
| 税费付款核对 | 把核定税费与外部资金事实按范围比较的版本化判断。不形成付款或放行 |
| 放行门禁核对 | 对明确范围、拟执行动作，判断放行前置条件是否满足。不代替放行结果 |
| 动作准入判断 | 对明确对象和一次动作意图，把限制、放行门禁核对、外部放行结果折成的封闭答复 |
| 节点关务区域 | 租户为网络节点登记的关务区域。起讫区域不同即跨关务区域 |
| 关务适用性判断 | 对一条路由候选能否用在册的口岸与申报路径过关务边界的即时作答 |
| 关务案件关闭核对 | 在明确截点对案件全部适用义务逐项形成关闭依据的评估 |
| 受控重开决定 | 已关闭案件收到仍属原案件的迟到事实后，允许继续处理的追加式决定 |

## 取值表（代码里的封闭集）

| 轴 | 取值 |
|---|---|
| 进出口方向 | `IMPORT` `EXPORT` |
| 提交尝试结果 | `REGISTERED_PENDING_SEND` `ACKNOWLEDGED` `FAILED` `PENDING_CONFIRMATION` |
| 外部结果层 | `REGULATORY_RECEIPT` `BUSINESS_ACCEPTANCE` `PROCESS_DECISION` `ASSESSED_DUTY` `RELEASE_RESULT` `DISPOSITION_DECISION` |
| 放行种类 | `FULL` `PARTIAL` `CONDITIONAL` |
| 后续申报动作 | `IN_CASE_SUPPLEMENT` `IN_CASE_CORRECTION` `WITHDRAWAL` `RESUBMISSION_REPLACEMENT` |
| 凭证门禁结论 | `APPLICABLE` `NOT_APPLICABLE` `CREDENTIAL_NOT_REGISTERED` `UNDECIDED` |
| 受门禁动作 | `OUTBOUND_RELEASE`（出库） `LOADING_DEPARTURE`（装载出发） `CROSS_CUSTOMS_MOVEMENT`（跨关务区域移动） `FINAL_DELIVERY`（交付） |
| 动作意图 | `DEPARTURE` `HANDOVER` `DELIVERY` |
| 内部限制决定种类 | `CONFIRM` `RELEASE` |
| 限制适用性 | `NONE_RECORDED` `IN_FORCE` `UNCONFIRMED` `CONFLICTING` |
| 处置执行核对结论（执行事实与处置决定逐范围比较） | `COVERED` `PARTIALLY_COVERED` `DEVIATION` `FACT_CONFLICT` `EVIDENCE_INSUFFICIENT` |
| 税费义务依据 | `ASSESSED_DUTY` `EXPLICITLY_NOT_REQUIRED` |
| 付款覆盖状态 | `NONE` `PARTIAL` `COVERED` |
| 付款差额状态 | `NO_DELTA` `SHORT` `EXCESS` `PENDING` |
| 资金事实有效性 | `VALID` `INVALIDATED` `CONFLICTING` `PENDING` |
| 放行门禁结论 | `UNMET` `PARTIALLY_MET` `MET` `CONFLICTING` `NOT_APPLICABLE` |
| 关闭依据项 | `CONCLUDED` `HANDED_OVER` `UNRESOLVED` `CONFLICT` |
| 关务适用性答案 | `AVAILABLE` `UNAVAILABLE`（理由 `PORT_NOT_REGISTERED` `PORT_NOT_EFFECTIVE` `PATH_DIRECTION_NOT_COVERED`） `STATUS_UNKNOWN`（成因 `ENDPOINT_COUNTRY_MISSING` `CATALOG_EMPTY` `CATALOG_UNREADABLE`） |

## 动作准入判断的答复

| 答复 | 什么时候 |
|---|---|
| `ADMITTED` 放行 | 没有有效限制约束本动作，门禁满足，且动作时点前已有全部放行 |
| `BLOCKED` 阻断 | 监管限制或内部合规限制有效且约束本动作；或门禁未满足、部分满足、冲突；或未收到放行 |
| `NOT_GUARDED` 不受门禁 | 对象不在任何案件与单元里（跨关务区域移动除外），或该道门禁不适用 |
| `UNDECIDED` 未决 | 在案而未成单元；只有部分或附条件放行；门禁与放行对不上；内部限制适用性待确认或冲突；税费付款核对缺或待确认 |
| `NOT_CONFIGURED` 未配置 | 门禁目录、税费付款门禁规则未登记，或跨区所需的节点关务区域未登记 |

五格不得互相替代。任一来源读不动时判断不形成，不折进任何一格。

成因码：`GATE_NOT_APPLICABLE` `OUTSIDE_CUSTOMS_BOOKS` `RELEASED_IN_FULL` `CUSTOMS_SCOPE_UNRESOLVED` `DECLARATION_UNIT_NOT_FORMED` `DUTY_VERIFICATION_ABSENT` `DUTY_VERIFICATION_PENDING` `RELEASE_PARTIAL_ONLY` `RELEASE_CONDITIONAL_ONLY` `GATE_RELEASE_DISCREPANCY` `INTERNAL_RESTRICTION_UNCONFIRMED` `INTERNAL_RESTRICTION_CONFLICTING` `NODE_ZONE_NOT_REGISTERED` `GATE_CATALOG_NOT_REGISTERED` `DUTY_PAYMENT_RULE_NOT_REGISTERED` `DUTY_PAYMENT_FINDING_BESIDE_RULE` `RESTRICTION_IN_FORCE` `INTERNAL_RESTRICTION_IN_FORCE` `GATE_UNMET` `GATE_PARTIALLY_MET` `GATE_CONFLICTING` `RELEASE_NOT_RECEIVED`。

## 规则

**案件与申报单元**

- 建案时固定监管辖区、进出口方向、监管程序和法定义务范围；这些要素不得原地改，变了就另建案件。
- 是否要求建案由登记的建案要求规则定；「不要求建案」也要带依据登记，未登记是未决。
- 案件与提交版本都保存参与方角色快照；后续资格变化不覆盖历史快照。
- 建案时在同一事务内取案件单据号（类型 `CUSTOMS_CASE`）；租户没登单据编号时区时照常建案、不带号，记下原因。

**就绪、授权与提交**

- 申报就绪判断与提交授权是两本独立的册，各自登记、各自撤销；撤销不删原判断。
- 两者在固定提交版本时都有效，才形成不可覆盖的提交版本和`已登记待发送`的首次尝试；之后任何变化不回写已提交快照。
- 越过发送边界另记一笔发送事实，一次尝试至多一笔；技术回执逐份追加，按尝试归属。
- 目标没登申报发送通道就不发送；登记之后就绪或授权失效了也不发。尝试保持`已登记待发送`。
- 超时或结果缺失保持待确认，不读作失败。只有安全再次发送判断成立，同一提交版本才可以有新的受控尝试。
- 更正或补充在原案件内形成新提交版本，原提交及其结果保留。
- 重报替代先形成拟替代关系；只有外部结果成立才生效，原对象不回退为从未提交。

**外部结果**

- 监管接收、业务受理、监管过程决定、监管核定税费、放行结果、监管处置决定分层保存，各带来源、业务发生时间与所用解释规则版本。前一层成功不生成后一层结果；当前判断不按到达先后覆盖。
- 放行按业务发生时间只进不退：部分或附条件之后到全部是后续事件，不是冲突。监管过程决定按时间成序列，各自成立。
- 同一时刻两种结论、放行回退成更窄种类、其余各层对同一提交版本语义不同，是同层冲突：冲突标在后到的那一份上，当前判断不变。
- 来源指名更正某一份同层结果时，形成新结果与更正关系，原结果不改写。所指那份还没在册就停在未决；来源没指明的不推断为更正。
- 解释规则按辖区与法定生效起点登记不可覆盖的版本；换版是登记起点更晚的新版。

**监管凭证（部分无生产入口，见末节）**

- 凭证版本不可变；换期限、持有人或额度是另一张凭证。同身份同内容重登只答已存在。
- 凭证可以经集成客户端、租户人员人工登记、受控登记 CLI 三路进来。人工登记记下登记人，只作操作证据，不是凭证内容。

**限制**

- 监管限制与内部合规限制是两种限制，各按自己的依据解除，互不解除；外部放行不自动解除任一种。
- 监管限制由关务人员依据一条已接收的外部监管结果建立与解除，两种决定分开授予。建立只认过程决定或处置决定；范围与生效时刻取自所引结果，人只声明约束哪些动作。解除只认同范围、生效之后的全部放行或决定（部分与附条件放行不算；一条决定够不够解除由持授予的人读原文判断），解除只落一次。结果不在册、归属不上、已被来源更正掉、或是一对同层冲突里的任一方，答依据不成立。
- 内部合规限制必须写明责任来源、依据、受限动作、开始时间和解除条件，覆盖项逐项保存。
- 只有原限制责任来源可以形成解除决定。解除是追加式的，只覆盖指名的覆盖项，不删原限制历史；任一限制仍有效就继续阻断。
- 适用性待确认或冲突时如实答待确认或冲突，不读成无内部限制。

**税费与放行门禁**

- 核定税费、付款事实、税费付款核对、放行各自保存，不互相推导。缺少税费结果不解释为无需付款。
- 税费付款核对分别给出覆盖、差额、有效性三个状态，不压成一个状态。迟到事实按新版本追加；结算采纳外部资金事实后重新派生。金额相等不证明覆盖，没有关联依据就是待关联。
- 程序没登记是否要求付款人维度时，核对不进行，保持未决。
- 「税费付款」这道门禁怎么读核对，是按监管程序登记的规则：对三个状态各自的接受集合，或声明不构成前置条件。待确认与冲突不可登记为接受；未登记答规则未配置。
- 放行门禁核对绑定明确范围和拟执行动作，不能复用于其他动作。门禁满足不生成放行结果。

**动作准入判断**

- 适用哪些受门禁动作由本上下文定：出发对应装载出发，交出对应出库，交付对应交付；起讫节点关务区域不同时另加跨关务区域移动。消费方只交意图。
- 起讫有一端没登区域时：对象带着跨区的门，或租户已有节点登了区域，跨区那一道答未配置并指名节点；租户一条区域都没登，这一道不列。
- 限制、门禁、放行三源逐动作折叠，都按动作时点读。门禁满足不等于放行，须有全部放行；门禁未满足而已全部放行答未决。
- 判断按内容派生标识记下，不覆盖；答复或依据变了才另记一份，来源变化不后台重算。

**关务适用性判断**

- 候选两端同国，答可用，不要求口岸与申报路径在场。
- 两端异国时按口岸与申报路径两级判：口岸已登记且在判断时点生效，申报路径生效且方向对得上。寄件国一侧为出口，收件国一侧为进口。
- 任一端缺国家码、两本目录皆空、依赖读不到，都答状态未知并带成因。状态未知不折成可用，也不折成不可用。
- 同一租户、时点、候选重复作答结果一致；判断标识可重算，随目录版本引用交回。本判断不落判断史库。

**关闭与重开**

- 关闭前在明确截点逐义务形成关闭依据项。每项都是已终结或已移交才可关闭；任一未解决或冲突都阻止关闭，没有部分关闭。义务清单来自登记的关闭义务目录，未登记答未配置。
- 重开是追加式决定，不撤销原关闭决定；再次关闭要重新核对。

**外部监管舱单与处置核对（处置核对无生产入口，见末节）**

- 舱单引用由责任承运商的集成客户端交来，承运商责任取客户端在册上绑定的来源身份，不收载荷自报。引用按（承运商责任，舱单身份）成行：舱单号只在承运商自己的编号空间里唯一，别家用同一个号读不到也占不住它。
- 更正或替代走版本推进口，形成新版本并指向前一版；基于旧前版的推进答版本冲突，不盖掉先落的。同舱单异版本走首登口是版本冲突，同版本而程序、方向或范围不同是内容冲突。能唯一匹配申报单元才关联，否则待关联，不猜。
- 处置执行核对逐范围比较执行事实与处置决定；执行方标记完成不自动终结监管义务。

## 生命周期

- **关务案件**：建立 → 进行中 → 已关闭 →（受控重开 → 再次关闭）。身份要素全程不变。
- **就绪判断、提交授权**：登记 → 有效 → 撤销。各走各的。
- **提交**：固定提交版本 → 首次尝试`已登记待发送` → 发送事实 → 技术回执（确认、失败或待确认）。之后各层外部结果各自到达，不拼成一条线性状态。
- **内部合规限制**：建立并带覆盖项 → 逐项确认或解除 → 适用性按时点重新派生。
- **税费**：核定税费或无需付款依据 → 税费付款协作事项 → 资金事实进入 → 税费付款核对逐版追加 → 放行门禁核对。

## 入口

在线口（路径登记在 `cmd/parcel-api`，处理在 `adapters/http`）：

| 做什么 | 路径 |
|---|---|
| 运营决定 | `/customs-cases` `/customs-declaration-submissions` `/customs-submission-authorizations` `/customs-case-closures` `/customs-release-gate-verifications` `/customs-internal-restrictions` `/customs-internal-restriction-decisions` `/customs-regulatory-restrictions` `/customs-regulatory-restriction-releases` `/customs-regulatory-credential-manual-registrations` |
| 线下递交标记 `/customs-declaration-manual-lodgements`：没有对接报关行时，一版申报在线下递交后记一笔；落的是与系统发送同一种发送事实、守同一组门（尝试仍待发送、单元仍就绪且授权仍有效），结果恒为待确认，必带递交凭据，递交人与凭据另记留痕。外部舱单引用的手工首登与版本推进 `/customs-external-manifest-reference-manual-registrations` `/customs-external-manifest-reference-revision-manual-registrations`：同一只编排、同一条引用（一份舱单一条，不因谁录的分成两条），承运商由录入的人指名，必带原件，每版另记留痕。集成客户端送入（外部结果另有手工录入口 `/customs-external-result-manual-registrations`：关务人员照回执录入，进同一只接收编排，必带所凭原件，录入人与原件记在结果上、不进内容指纹；来源标识由服务端加 `MANUAL/` 前缀，与接口来的各自成行） | `/customs/external-results` `/customs/technical-receipts` `/customs-regulatory-credential-registrations` `/customs/external-manifest-references` `/customs/external-manifest-reference-revisions` |
| 动作准入 | `/customs-action-admission-judgements` `/customs-unadmitted-objects` |
| 登记 | `/customs-case-requirement-registrations` `/customs-interpretation-rule-registrations` `/customs-gate-catalog-registrations` `/customs-candidate-port-registrations` `/customs-declaration-path-registrations` `/customs-node-zone-registrations` `/customs-duty-collaboration-registrations` `/customs-duty-payment-verification-registrations` |
| 查阅 | `/customs-case-records` `/customs-submission-records` `/customs-external-result-records` `/customs-external-manifest-reference-records` `/customs-case-registers` `/customs-compliance-rules` `/customs-gate-conditions` `/customs-ports-paths` `/customs-credentials` `/customs-duty-collaborations` `/customs-duty-verifications` |

没有启用接入渠道时，这些口答 `ACCESS_CHANNEL_NOT_CONFIGURED`。

后台（`cmd/parcel-dispatch`）：申报发送消费者经登记的通道发出`已登记待发送`的尝试；结算采纳资金事实后重新派生税费付款核对；出发前门禁重问；为 `network-routing` 提供关务适用性判断。

离线口：`cmd/parcel-customs-register`，运营在数据库网络内手跑，不监听端口，输入是 JSON 文件。登记就绪判断与提交授权（含撤销）、解释规则、关闭义务目录与明细、门禁目录与明细、税费付款门禁规则、建案要求规则、口岸、申报路径、监管凭证、税费付款协作事项与核对、节点关务区域、申报发送通道。

**有用例代码、但没有接进任何进程的**（`cmd/` 下非测试代码里找不到构造点）：后续申报动作与替代（`ManageFollowUpHandler`）、原案内更正（`CorrectDeclarationHandler`）、处置执行核对（`VerifyDispositionHandler`）、凭证门禁判断（`RecordCredentialGateHandler`、`JudgeCredentialApplicabilityHandler`）。合规判断只有领域类型 `ComplianceJudgment`，没有用例。上面「规则」里属于这几块的句子，眼下没有生产入口能触发。

## 旧文档写了、代码里还没有的

- **申报货物项的形成**（把货物明细行按税号、原产地并项，补算、折算，选品名语言与编码口径）：读货物明细的口 `CustomerGoodsDetailsView` 及其适配器在，但 `application` 里没有消费它的代码；搜 `GoodsItem`、`申报货物项` 无结果。
- **正式申报资料快照本体与字段级溯源**：代码只有 `DossierSnapshotReference` 这一个引用；搜 `Provenance`、`FieldSource`、`溯源` 只中一处注释。
- **由本上下文评出申报就绪**（逐项确认资料、角色、合报资格、凭证、口岸、渠道与限制）：代码里就绪判断是登记进来的，依据只是一个引用（`ReadinessBasisReference`）。
- **受控跨客户合报的兼容性判定**：搜 `合报`、`CrossCustomer`、`Consolidat` 只中注释。
- **版本化自动提交政策**：搜 `AutoSubmi`、`AutomaticSubmi`、`自动提交` 无结果。提交只走运营决定口。
- **合规判断进入人工复核的分流**（资料缺失、多个合理归类、受管制商品、高风险）：搜 `HighRisk`、`ManualReview`、`人工复核` 无结果。
- **凭证额度或次数的占用、释放与核销**：凭证只记可选的次数额度；搜 `Occup`、`WriteOff`、`Reservation` 无类型，注释写明这段生命周期另有。
- **监管补缴要求、监管税费退回决定**：搜 `Supplementary`、`补缴`、`RefundDecision`、`退回决定` 无结果。
- **关务执行协作事项**（查验、扣留、处置各自的协作对象与交接）：搜 `ExecutionCollaboration`、`执行协作` 只中包注释；代码只有处置执行核对。
- **后续关务案件及其与原案的关系**、同一迟到来源按范围分别重开与另建案件：搜 `FollowUpCase`、`PriorCase`、`RelatedCase`、`后续关务案件` 无类型。
- **查询响应区分监管事件与状态快照**：搜 `StatusSnapshot`、`状态快照`、`QueryAttempt` 只中一处注释。
