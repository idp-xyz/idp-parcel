# PS 组子审计：UC-PS-002 至 006

审计基线：只读 worktree `/home/tops/workspace/idp-parcel-audit`（detached @ `6b0074d0`）。范围：`docs/application/parcel-shipment/` 下 UC-PS-002、003、004、005、006。覆盖目标 2（与 CONTEXT/ADR 一致、同一场景只有一套结果）、目标 5（引用规范、断链）、目标 6（术语）。只在 UC 内部或 UC 与 CONTEXT 互相矛盾时才顺带比对代码。口径按 `.scratch/docs-consistency/README.md`「审计口径」。

共 17 条：高 2、中 6、低 9。

---

## 高

### PS-UC-01 收寄早于取消、取消先提交、收寄消息后到：UC-PS-003 判「保留责任起点」，代码与 UC-PS-006 判「未决」

- **类别**：2（同一场景两套结果）
- **位置**：
  - `UC-PS-003-ESTABLISH-NETWORK-INTAKE-AND-FORMAL-COMMITMENT.md`「验收示例」中 `AT-PS-045`：「收寄事实先发生，取消消息后到 | 按业务时间保留责任起点；取消不能追溯抹去收寄」
  - 同文件「一致性、幂等与并发」：「取消与收寄并发按权威业务时间和生效边界裁决，不按消息到达顺序。」
- **证据**：
  - 代码 `internal/parcelshipment/application/adopt_network_intake.go` `AdoptNetworkIntakeHandler.Handle` 的取消边界一段：取消已成立且 `cancellation.RequestedAt()` 不早于 `source.OccurredAt()` 时交回 `IntakeEligibilityUndecided` / `IntakeCancellationOrderConflict`。注释写的是「收寄发生早于取消决定……事实冲突保持未决，不按消息到达顺序选边」。测试 `TestTheCancellationBoundaryJudgesByBusinessTime` 的子例「an intake that happened before the cancellation stalls as a conflict」断言的就是未决。
  - `UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md` 决定矩阵：「收寄事实存在但有效性或业务顺序待确认 | 取消/处置未决，续办事实核对」；`AT-PS-081`：「取消与处置均保持未决」。
  - CONTEXT Rules 一句可作两种读法：「形成取消决定前必须在同一决定提交边界重新核验当前有效收寄及终局事实；并发收寄已经先行成立时不得提交取消，消息晚到不能改变这一裁决」。「先行成立」既可指业务时间在先，也可指提交时已采用。按前一种读，取消不该提交；按后一种读，取消有效、收寄不采用。
  - `.scratch/at-coverage-inventory.md` 把 `AT-PS-045` 列为无测试覆盖；`AT-PS-080`/`081` 指向的正是上面那条判「未决」的测试。
- **归类**：B。CONTEXT 两读皆可，没有 ADR 裁过。一边是 UC-PS-003 的 `AT-PS-045` 和「按权威业务时间」那句（判采用），另一边是 UC-PS-006 加代码（判未决），三处各说一套。附带一点：代码拿取消的**请求时间**（`RequestedAt`）跟收寄发生时间比，注释写的却是「取消决定」。裁决时应一并定下比哪个时刻。
- **严重度**：高。照 `AT-PS-045` 实现，一个包裹会同时带着取消终局和责任起点/正式承诺。
- **建议修法**：立 ADR 代决。倾向认可代码现状：判事实冲突、未决，交明确处置；同时定下比较时刻是取消决定提交时刻还是请求时刻。随后改写 `AT-PS-045`、UC-PS-003 那句和 CONTEXT「消息晚到不能改变这一裁决」，三处说同一套。

### PS-UC-02 撤回只释放「资金冻结」：与 ADR-0125「按占用释放」、代码 `OccupationFormed` 相反

- **类别**：2（AT 行与现行规则相反）
- **位置**：`UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md`
  - 「本用例包含」：「对已形成的接受前资金冻结发出使用原业务关联的显式释放请求。」
  - 步骤 6：「按原业务关联幂等形成适用冻结释放」
  - `AT-PS-074`：「未形成过资金冻结 | 撤回成立并保存财务补偿不适用依据」
  - 「首发试点叠加条件」：「`PAR-COM-15` 必须明确撤回成立时适用冻结的释放条件……以及无财务控制或未形成冻结时的不适用依据」
- **证据**：
  - ADR-0125 Context 点名过这个缺陷：「`WithdrawShipmentRequestHandler.releaseFreeze` 都按 `Outcome() == HELD` 发。于是 `CREDIT_EXPOSED` 从不释放——账期额度占用在拒绝 / 撤回后成孤儿」。它的 Decision 四改成「`OccupationFormed()` 在任一项`成立`时为真；三处释放改按它发」。ADR-0122 决定三：「释放两本账各认领一次」。ADR-0047：账期控制形成的是信用暴露，不是冻结。
  - CONTEXT 词条「控制项结果」：「任一项成立即表示提供方账本上形成了占用；接受确定未成立时必须按原业务关联请求释放，不论接受侧结论如何。」
  - 代码 `internal/parcelshipment/application/withdraw_shipment_request.go` `releaseOccupation` 按 `recorded.FinancialControl.OccupationFormed()` 判断要不要发释放，不看冻结。
  - `UC-SA-002` 目标里写的也是「估价、资金冻结、信用暴露……或显式释放结果」。
- **归类**：A。代码与 ADR-0125 一致，UC 没跟着改。同一过时措辞还留在两处，供 CONTEXT 与代码那边参考：CONTEXT Rules「已经形成的接受前资金冻结必须通过原业务关联请求 `settlement-accounting` 显式释放」一句，以及 `releaseOccupation` 头注「从未形成过冻结时既不发释放」。
- **严重度**：高。照 `AT-PS-074` 写，只做了信用校验的委托撤回后不释放信用暴露，正是 ADR-0125 修掉的那个孤儿占用。
- **建议修法**：把 UC-PS-005 里的「资金冻结」统一改成「已成立控制项的占用（冻结或信用暴露）」，引 CONTEXT「控制项结果」。`AT-PS-074` 改为「未形成任何占用（无成立项）| 撤回成立并保存财务补偿不适用依据」，另加一行「仅信用校验成立 | 按原关联释放信用暴露」。

---

## 中

### PS-UC-03 UC-PS-002 把依赖或授权规则未定判成「待复核」，结果表也没有「未决」一行；代码与 ADR-0116 判的是未决

- **类别**：2、4
- **位置**：`UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md`
  - 步骤 4：「授权不足形成业务拒绝；依赖无法确定形成待复核」
  - 「结果语义契约」表共八行（已记录并采用、已记录但待下游判断、待补充/待复核、业务拒绝、输入未受理、已有结果、请求冲突、技术未形成），没有「未决」；元数据里却有「业务未决结束」一行。
- **证据**：
  - ADR-0116 决定三：「grant 缺席照旧 `ErrAuthorityRulesNotConfigured`」；决定五：「缺省落在既有的 `ErrAuthorityRulesNotConfigured` → PS 未决」。
  - 代码 `internal/parcelshipment/application/amend_customer_source_data.go`：授权服务答不出走 `undecided(…, SourceDataAmendmentAuthorityUnavailable)`，授权规则未配置走 `undecided(…, SourceDataAmendmentAuthorityRulesNotConfigured)`；阶段读不回或判不出、矩阵读不回同样进 `AmendmentUndecided`，并带 `PendingReason` 和续办引用。只有矩阵答 `NotDeclared` 才进 `AmendmentAwaitingReview`（ADR-0120 决定三：「客户看到的是『等复核』」）。
  - CONTEXT Rules 要求五类等待各有原因和续办路径，依赖抖动不得与复核合并（「内部重试推进不了一次人工复核」）。
- **归类**：A
- **严重度**：中。照步骤 4 写，依赖抖动会被交给人工复核，续办对象弄错。
- **建议修法**：步骤 4 改为「授权未获准形成业务拒绝；授权规则未配置或授权依赖答不出形成未决（带原因与续办引用）」；结果表补一行「业务未决」，写明原因与续办引用必须保存；`待复核`只留给「矩阵未声明」和「重叠需人工合并」。

### PS-UC-04 越权/跨客户请求的结果各处写法不一，与 CONTEXT「统一不可见结果」相抵

- **类别**：2、6
- **位置**：
  - `UC-PS-002` 元数据「输入未受理结束」：「无法识别目标委托/包裹/资料范围或请求授权」；步骤 3：「目标不明、跨客户越权或范围不可解释时输入未受理」；`AT-PS-019`：「请求方无客户、委托或字段范围授权 | 业务拒绝」。同一份 UC 里，跨客户越权一处判输入未受理，一处判业务拒绝。
  - `UC-PS-003` `AT-PS-052`：「拒绝越权且不泄露……」
  - `UC-PS-005` `AT-PS-075`：「不泄露对象，拒绝越权操作」
  - `UC-PS-006` 步骤 2：「越权或对象冲突逐件返回」
- **证据**：
  - CONTEXT Rules：「未授权、越权和不存在对外返回统一不可见结果」；Language「统一不可见结果」：对外使用同一不可见语义，不暴露对象存在。
  - 代码：`amend_customer_source_data.go` 与 `withdraw_shipment_request.go` 在按来源身份（含客户账户）查不到委托时上抛 `domain.ErrInvalidShipmentRequest`。`adapters/http/amend_customer_source_data.go` 头注写明它「与其余没形成答案的失败一并回 5xx `NO_ANSWER_FORMED`——统一不可见结果不拆开」。`withdraw_shipment_request.go` 注释写「`AT-PS-075` 要求越权撤回不泄露对象存在与否，走的正是这条出口」。只有本客户范围内、请求方没有该动作授权时，才交 `NOT_AUTHORIZED`（业务拒绝）。
- **归类**：A
- **严重度**：中。照 `AT-PS-019` 对跨客户请求答「业务拒绝」，就泄露了对象存在。
- **建议修法**：各 UC 统一分两格写：「跨客户、跨租户或对象不存在 → 统一不可见结果」，「本客户范围内请求方无此动作授权 → 业务拒绝（未获授权）」。`AT-PS-019` 拆成两行；UC-PS-002 步骤 3 与元数据里的「跨客户越权」「请求授权」改归统一不可见结果。

### PS-UC-05 UC-PS-006 用「授权处置决定」指收寄后服务处置，撞上 CONTEXT 词条「授权处置」

- **类别**：6
- **位置**：`UC-PS-006` `AT-PS-082`：「明确不能回退取消；没有授权处置决定时保持待处置」
- **证据**：CONTEXT 词条「授权处置」专指「接受前财务控制按共同通过条件不成立……由适用授权规则授权的角色在封闭去向中选一个——拒绝，或交客户补充」，即接受前的去向决定（ADR-0132；代码 `dispose_shipment_request.go` 的 `AuthorizedDispositionOutcome`、迁移 `0021_authorized_disposition.sql`）。收寄后的那个词在 CONTEXT 和 GLOSSARY 里叫「收寄后服务处置决定」。代码注释 `cancel_parcel.go`「没有授权处置决定时保持待处置（AT-PS-082）」把这个混用照抄了过去。
- **归类**：A
- **严重度**：中。读者可能把收寄后的待处置接到接受前`等待授权处置`的续办路径上。
- **建议修法**：`AT-PS-082` 改为「没有收寄后服务处置决定时保持待处置」；`cancel_parcel.go` 那句注释同改，属代码注释，可随票附带。

### PS-UC-06 UC-PS-003「给开发的交接」与代码现状、ADR-0017/0026/0146 的闸门口径都相反

- **类别**：4（兼 1）
- **位置**：`UC-PS-003`「给开发的交接」：「当前代码只存在建单前身份、来源重放/冲突和最小委托候选；尚无已接受聚合、包裹服务、收寄采用、正式承诺、持久化事务或 Outbox。可以先建立纯业务端口、结果类型和命名夹具，但真实写入必须等待 PN-02 前置、`PAR-COM-16`、`PAR-NET-13` 和技术门槛。」
- **证据**：
  - 代码早已存在：`internal/parcelshipment/application/adopt_network_intake.go`（`AdoptNetworkIntakeHandler`），`domain/network_intake.go`（`FormalCommitment`），`migrations/parcel_shipment/0004_adoption_cancellation_final.sql` 与 `0017_intake_adoption_supersession_chain.sql`，`ports.OutboxNetworkIntakeHandoff` 及 `adapters/postgres/network_intake_handoff.go`。
  - ADR-0017：「机制半边即刻放行……实例半边仍然阻断：任何真实客户、合同……取值进入代码，以及任何租户流量进入生产接单」。ADR-0026：「持久化实现属机制半边，其阻断理由是外部工件，与租户参数无关」。`PAR-COM-16`、`PAR-NET-13` 是租户取值，挡不住机制写入。
- **归类**：A
- **严重度**：中。读者会误判现状，或把机制工作当成被租户参数阻断。
- **建议修法**：删掉「当前代码只存在……」那句状态描述（代码现状不该写进 UC）；闸门句改为「机制与持久化不等租户取值；真实资格、承诺规则与来源优先级（`PAR-COM-16`、`PAR-NET-13`）未登记时如实答未配置或未决」，并引 ADR-0017、ADR-0146。

### PS-UC-07 UC-PS-003「按上一条作竞争处理」是位置引用，而且已经指错

- **类别**：5
- **位置**：`UC-PS-003`「一致性、幂等与并发」末条：「来源种类相同而无更正声明的新版本按上一条作竞争处理（ADR-0117）」
- **证据**：该条正上方是「取消与收寄并发按权威业务时间和生效边界裁决……」，不是竞争规则。真正要指的是同节「客户送站与场外揽收都指向同一包裹时不能形成两个责任起点。先合法形成者保留……」那条（`AT-PS-049`）。ADR-0117 决定一写作「走 `AT-PS-049` 那一格」。
- **归类**：A
- **严重度**：中。位置引用已经指到另一条规则上。
- **建议修法**：改成按引文指：「……按『先合法形成者保留』那一条（`AT-PS-049`）作竞争处理」。另：UC-PS-004「判断结果作为责任结果进入上面的采用流程」是同文件内的位置引用，风险低，可顺手改为「进入『应用流程』节」。

### PS-UC-08 当前资料版本采用判断的取值集合四处说法不同；UC-PS-002 的「待补充/待复核」把两个续办方并成一格

- **类别**：6（兼 2）
- **位置**：`UC-PS-002`「结果语义契约」行「待补充/待复核 | 缺少必要内容、基础版本过期、字段冲突或规则需要人工判断」，以及 `AT-PS-021`、`AT-PS-022` 中的「待复核」。
- **证据**：
  - CONTEXT Language「当前客户资料版本采用判断」写四值：「已采用、待补充、冲突或未决」。
  - CONTEXT Lifecycles「客户原始资料版本」与 GLOSSARY 同名词条写五值：「已采用、待下游判断、待补充、冲突或未决」。
  - CONTEXT Rules「收件地点引用」那条又直接用了 `待复核`：「`待复核`不给引用而答收件地点未定」。这个值不在上述任一集合里。
  - 代码 `domain/customer_source_data.go` 的 `SourceDataAdoptionOutcome` 只有 `ADOPTED`/`AWAITING_REVIEW`，头注称「`待复核`是 UC-PS-002 对 CONTEXT`待补充`那一格的点名叫法……客户补不出东西来，要人来并」。这等于把客户续办（待补充）和人工续办（待复核）当成同一格，与 CONTEXT Rules「不同原因和续办路径……合并其中任意两者都会让等待对象弄错」的精神相抵。
- **归类**：A。以代码已产出的 `待复核` 为准补进 CONTEXT 与 GLOSSARY。CONTEXT 与 GLOSSARY 本身的分歧归 PS 主审与横切组。
- **严重度**：中
- **建议修法**：CONTEXT 与 GLOSSARY 统一成一套取值，把`待复核`列为独立一格（续办方：授权复核角色），与`待补充`（续办方：客户）分开。UC-PS-002 把「待补充/待复核」拆成两行，各写触发与续办方。

---

## 低

### PS-UC-09 UC-PS-002 步骤 1 把「无法保全」判成「技术未形成」，与自己对技术未形成的定义相抵

- **类别**：2
- **位置**：`UC-PS-002` 步骤 1：「无法保全时形成技术未形成，不进入资料版本处理」。结果表与元数据对「技术未形成」的定义是「原始请求已保全，但版本提交或发布意图未完成」。
- **证据**：同一文件两处。代码 `adapters/http/amend_customer_source_data.go` 在 intake 失败时回 5xx `INTAKE_FAILED`，不形成任何应用结果。
- **归类**：A
- **严重度**：低
- **建议修法**：步骤 1 改为「无法保全时不形成任何结果（没有形成答案）」，或把技术未形成的定义扩到「保全未完成」，两处取同一句。

### PS-UC-10 UC-PS-002 仍把 BD-PS-009/010 整格列为「待确认」，没有交代 ADR-0116/0118/0120 已裁定的机制半边

- **类别**：2（兼 1）
- **位置**：`UC-PS-002`「待确认业务决策」表 `BD-PS-009`（「当前建议：优先复用唯一生产接入渠道……」）、`BD-PS-010`（「当前建议：采用版本化阶段矩阵」）；「权威依据」表没有引 ADR-0116、0118、0120。
- **证据**：ADR-0116 已裁定资料修订是授权动作、合同委派决定实际决定方、委派缺席时答业务拒绝；ADR-0120 已裁定允许矩阵挂在规则包版本下，三维两值加 `closed`；ADR-0118 裁定阶段事实经消费侧读口进入。ADR-0120 Links 写明「`PAR-COM-13` / `BD-PS-010`：实例半边的出处」。按 ADR-0146 的三类，这两项的机制与产品策略已做完，剩下的只是租户取值。
- **归类**：A
- **严重度**：低
- **建议修法**：BD-PS-009/010 两行注明「机制已由 ADR-0116 / ADR-0120 裁定，余下租户取值见 `PAR-COM-13`」；「权威依据」表补 ADR-0116、0118、0120 三行。

### PS-UC-11 UC-PS-004/005/006 状态行「稳定业务骨架待实现」已过时

- **类别**：4
- **位置**：`UC-PS-004` 状态行「稳定业务骨架待实现；……」；`UC-PS-005`、`UC-PS-006` 状态行同句。
- **证据**：`internal/parcelshipment/application/form_parcel_final.go`、`judge_label_service_final.go`（UC-PS-004），`withdraw_shipment_request.go`（UC-PS-005），`cancel_parcel.go`（UC-PS-006）均已在，`.scratch/at-coverage-inventory.md` 也列出了这些 AT 的测试覆盖。
- **归类**：A
- **严重度**：低
- **建议修法**：状态行只写业务边界的成熟度与待提供的租户取值，去掉「待实现」这类会过时的实现进度描述。

### PS-UC-12 UC-PS-004 `AT-PS-099`「保持未形成」撞上本 UC 的「未形成结束」（技术义）

- **类别**：6
- **位置**：`UC-PS-004` `AT-PS-099`：「关闭路径不再成立，保持未形成」。元数据「未形成结束 | 并发或技术提交未完成；不得伪装为未决或业务拒绝」。
- **证据**：同文件两处。代码 `domain/label_service_final.go` 对这一格答 `NOT_FINAL`，原因 `CONTINUED_ATTEMPT_STILL_OPEN`，是业务判断，不是技术未形成。
- **归类**：A
- **严重度**：低
- **建议修法**：`AT-PS-099` 改为「关闭路径不再成立，不形成终局（继续尝试仍开放）」。

### PS-UC-13 UC-PS-005「人工接受」与 CONTEXT「复核完成本身不形成决定」不合

- **类别**：6
- **位置**：`UC-PS-005`「规则、补偿与并发」：「撤回、自动接受、人工接受和主动拒绝竞争同一不可覆盖决定边界。」
- **证据**：CONTEXT Lifecycles「接受判断任务」：「复核完成本身不形成决定，决定仍由判断任务按适用规则形成」；Rules：「人工处理不得绕过硬规则」。接受只有一种来路，即判断任务。同一说法也出现在 `docs/product/PILOT-SCOPE.md`、`docs/design/pn-02-*.md`，属横切。
- **归类**：A
- **严重度**：低
- **建议修法**：改为「撤回、（含人工复核完成后由任务形成的）接受和主动拒绝竞争同一……」，或干脆只写「接受」。

### PS-UC-14 UC-PS-005 漏写 CONTEXT 要求的「同一笔状态变化里交出声明撤除」

- **类别**：2
- **位置**：`UC-PS-005` 步骤 5：「同一提交保存撤回、任务停止、审计和发布意图」
- **证据**：CONTEXT Rules：「一份带客户包裹件号的委托由`已提交`转为`已拒绝`或`已撤回`时……本上下文在同一笔状态变化里按它声明的每个号向节点作业交出一次声明撤除」（ADR-0211 决定三）。代码在 `adapters/postgres/shipment_request.go` 与 `customer_parcel_number_declaration_retired_handoff.go` 里实现了这一点。
- **归类**：A
- **严重度**：低
- **建议修法**：步骤 5 补上「及（委托声明了客户包裹件号时）声明撤除意图」，引 CONTEXT 那条。

### PS-UC-15 UC-PS-005 的结果代数漏了「未获授权」

- **类别**：4
- **位置**：`UC-PS-005`「本用例包含」：「区分撤回已成立、已有决定、未决、输入未受理和技术未形成」；「输入与结果语义」表与元数据都没有业务拒绝或未获授权这一格。
- **证据**：代码 `withdraw_shipment_request.go` `WithdrawalOutcome` 有 `NOT_AUTHORIZED`，即请求方在本客户范围内但 PC 答 `AuthorizationRefused`；授权规则未配置则进 `UNDECIDED`。UC 自己也写「不得默认任何角色有撤回权」，可见确有未获授权这一格。
- **归类**：A
- **严重度**：低
- **建议修法**：结果表补「未获授权 | 请求方在本客户范围内但无撤回授权 | 不形成撤回、不释放」，并注明它与跨客户的统一不可见结果是两格（见 PS-UC-04）。

### PS-UC-16 UC-PS-003 资格检查第 1 项读来像 PS 侧的防线，对节点来源其实自己比自己

- **类别**：2
- **位置**：`UC-PS-003`「适用资格判断」第 1 项：「来源所属租户、客户账户、责任法人和包裹与接受基线一致。」
- **证据**：ADR-0184 复核记录「决定六与 UC-NO-002」写道：「PS 采用这份收寄时，按关联到的包裹反查目标委托……客户账户取自那份委托。所以 UC-PS-003『适用资格判断』里……那一项，对节点收寄来源拦不住它」。ADR-0195 Context 也说「对节点来源就成了自己比自己」。真正挡错关联的地方是 NO 侧身份判断（ADR-0195 唯一命中要无歧义信号，ADR-0212 要可验证交付范围）。
- **归类**：A
- **严重度**：低
- **建议修法**：在第 1 项后加一句：「节点收寄来源的客户账户随身份关联而来，本项对它不独立成立；错关联由 `node-operations` 身份判断按 ADR-0195 / ADR-0212 挡住。」

### PS-UC-17 断链与锚点抽检（无发现，记录口径）

- **类别**：5
- **位置**：五份 UC 的全部相对链接。
- **证据**：脚本逐个解析了相对链接的目标文件，并按标题生成 slug 核对锚点，包括 `CONTEXT.md#language`、`#rules-and-invariants`、`#lifecycles`、`CONTEXT-MAP.md#关系约束`、`party-commercial/CONTEXT.md#rules-and-invariants`，以及 UC-CC/NO/TF/SA 各文件。全部能解析，没有行号引用，也没有跨文件计数引用。位置引用只有 PS-UC-07 记的两处。
- **归类**：无需修
- **严重度**：低（仅记录）
- **建议修法**：无

---

## 已核无问题的要点

| 要点 | 核对对象 | 结论 |
|---|---|---|
| UC-PS-002「资料范围与阶段边界」六格 | CONTEXT「资料修订阶段」六格原词、ADR-0118/0120 | 一致 |
| UC-PS-002 重放/冲突与 `occurredAt`/`receivedAt`/`requestEffectiveAt` 口径 | CONTEXT Language 三个时间词条与 Rules「同一完整来源请求身份……」 | 一致 |
| UC-PS-002 `AT-PS-023`/`024` 增删成员、改共享约束即拒绝 | CONTEXT Rules「接受后不得以资料更正方式新增或删除委托成员」；代码 `AmendmentDisallowed` 先于矩阵判 | 一致 |
| UC-PS-002 矩阵未声明转「待复核」、已声明不允许即拒绝 | ADR-0120 决定三；代码 `AmendmentAwaitingReview`/`AmendmentDisallowed` | 一致 |
| UC-PS-003 同来源更正与另一来源竞争分格（正文，除 PS-UC-07 的指代） | ADR-0117 决定一、五 | 一致 |
| UC-PS-003 结果六格 | 代码 `IntakeAdoptionOutcome`（另多一格 `REQUEST_NOT_ACCEPTED`，归 PS 主审的代码比对） | 一致 |
| UC-PS-004 面单渠道服务终局形成各条 | CONTEXT Rules「首个面单渠道服务产品默认……」、Lifecycles「包裹身份与服务」三条面单终局转换 | 一致，引文逐字存在 |
| UC-PS-004 引「ADR-0135；外部承运轨迹事实本身不构成收寄」 | ADR-0135 决定一（收寄是 TF 在合格来源之上的独立判断事实） | 一致 |
| UC-PS-004 有效期规则未配置时不按时钟推算 | ADR-0119、CONTEXT Rules「自然失效必须来自渠道确认或接受时固定的有效期规则」 | 一致 |
| UC-PS-004 未引 ADR-0138 | ADR-0138 为 Proposed；docs/domain、docs/application 均未当现行引用 | 无问题 |
| UC-PS-005 撤回与接受/拒绝竞争同一决定边界、后到读既有结果、`AT-PS-071` 接受先成立不释放 | CONTEXT Rules 撤回三条；代码 `WithdrawalDecisionAlreadyFormed` 与撞上接受不释放 | 一致 |
| UC-PS-006 取消边界、批量逐件、委托派生`已取消` | CONTEXT Rules 取消四条、Lifecycles「已接受 → 已取消」 | 一致 |
| UC-PS-006 `AT-PS-079`/`080`/`082` | 代码 `cancel_parcel.go` 提交边界重读收寄、`DispositionPending`；`adopt_network_intake.go` 的 `CANCELLED_BEFORE_INTAKE` | 一致（术语问题见 PS-UC-05） |
| AT 编号 | 七份 PS UC 的 AT-PS 编号 | 无重号 |
| ADR 正文引 UC-PS-002/003 的原句（0116「登录操作人不能替代实际决定方」、0117 步骤 7、0184/0195「适用资格判断」） | UC 原文 | 逐字存在 |
