# PS 组 · ADR 停用链子审计（目标 3，兼目标 1 停用相关半边与目标 5）

只读 worktree `/home/tops/workspace/idp-parcel-audit`（detached @ `6b0074d0`）。口径照 `.scratch/docs-consistency/README.md`「审计口径」。引文均以 `grep -F` 核过；「正文」指 Status 行以下。ADR-0138 为 Proposed，不当现行。

共 14 条：高 0，中 6，低 8。

---

## 中

### PS-ADR-01　ADR-0081 决定三「任一步未决即整笔回滚等重投」已被四份后来记录收窄，0081 一头没有任何指针

- **类别**：3（停用链只有一头），兼 1
- **位置**：`docs/adr/0081-acceptance-judgment-is-envelope-driven.md` 决定三「**三、一个消费者按编排顺序推进，不拆三类信封接力。** 任一步未决即整笔回滚等重投」；Status 行与索引注记只记了 ADR-0158 一条（决定六），Links 节没有 0086 / 0094 / 0106 / 0132。
- **证据**：
  - ADR-0086 Context「ADR-0081 Decision 三把接受链的未决处置裁成一格：任一步未决即整笔回滚等重投。这一格对三个等待态里的两个成立，对第三个不成立」，决定一把`等待人工复核`改成提交入账；
  - ADR-0094 决定三把`等待运营登记`改成提交入账；ADR-0106 决定一把`等待受控补充`改成提交入账（「自此四格的处置是：`ResumeByInternalRetry` 一格回滚重投，其余三格提交入账」）；ADR-0132 再把`等待授权处置`列进提交入账；
  - 代码 `internal/parcelshipment/adapters/inbox/undecided_disposition.go` 的 `undecidedDisposition`：只有 `domain.ResumeByInternalRetry` 回滚，其余四格 `return nil`。
  - 四份记录都把这叫作「细分」，没有一份走 README 的部分停用机制，所以 0081 一头什么都没有。索引里 0081 一行只露出决定六那一格，读者会以为决定三原样有效。
- **归类**：A（代码与 0086 / 0094 / 0106 / 0132 一致，0081 的指针缺了）
- **严重度**：中。照 0081 新接一扇接受链消费门，会把非自愈的等待态写回回滚重投，`ABANDONED` 的老问题会回来。
- **建议修法**：0081 的 Status 行索引注记与 Links 各补一条：决定三「任一步未决即整笔回滚等重投」适用场景收窄为 `ResumeByInternalRetry` 一格，其余各格依次由 ADR-0086、0094、0106、0132 改为提交入账，其余各条不变。0086 / 0094 / 0106 / 0132 的 Status 行补「对 ADR-0081 决定三的作用」一句。

### PS-ADR-02　ADR-0081 决定六的四个「留 nil」格已全部填上，Status 行却说「其余各格仍有效」；所引原句也不是逐字

- **类别**：3（停用链缺头、引句不逐字），兼 4
- **位置**：`docs/adr/0081-…md` Status 行「部分停用：决定六里「结算账户目录留 nil」这一格由 ADR-0158 停用……决定六其余各格不由该记录停用」，索引注记「决定六其余各格仍有效」。正文决定六原文是「时点取值源、可达性闭包标识、结算账户目录、控制金额源全部留 nil，各按「显式未配置」停下」。
- **证据**：
  - 「结算账户目录留 nil」在 0081 正文里不存在（`grep -F` 只命中 Status 行与 Links），ADR-0158 后果节「本记录部分停用 ADR-0081 决定六里「结算账户目录留 nil」这一格」也是同一份改写句。
  - 可达性闭包标识：ADR-0156 后果节「生产装配不再传入 nil 身份映射」，代码 `cmd/parcel-dispatch/assemble.go` 的 `acceptanceReachability`。0156 只停用了 ADR-0064 的两句，没有停用 0081。
  - 时点取值源：ADR-0157（内置时点形态「提交接收」）；代码 `cmd/parcel-dispatch/assemble.go` 装 `pspartycommercial.NewSubmissionReceiptAsOf(requests)`。
  - 控制金额源：ADR-0179 决定一「parcel-shipment 发起控制时不再交金额」；代码里 `EstimationAmountSource` / `ControlAmountSource` 已不存在，`CONTROL_AMOUNT_NOT_CONFIGURED` 只在 `internal/settlementaccounting/application/apply_pre_acceptance_control.go` 里答。0179 只停用了 ADR-0048。
- **归类**：A
- **严重度**：中。「其余各格仍有效」会让读者以为时点、闭包标识、金额三格在生产装配里仍是 nil。
- **建议修法**：0081 的 Status 行与 Links 改成：决定六四格分别由 0158（账户目录）、0156（可达性闭包标识）、0157（时点取值源）、0179（控制金额源，改归结算）收窄或取代，「解析键登记面接真」那半句不变。被停条款引原句「时点取值源、可达性闭包标识、结算账户目录、控制金额源全部留 nil」，不要用改写句。0156 / 0157 / 0179 的 Status 行补对 0081 的作用；0158 后果节那句改写句是正文，不动，在它的 Status 行里改引原句。

### PS-ADR-03　财务控制链：0125 改了 0122 决定四的折叠位置，0132 又改了 0125 决定一的形状并解除其过渡态，被停方的 Status 行都没指针，0122 的索引注记还写着旧规则

- **类别**：3，兼 1
- **位置**：
  - `docs/adr/0122-…joint-pass-condition.md` 索引注记「CONTEXT 把「按共同通过条件形成接受判断」判给 PS，故 SA 逐项交回、折叠在 PS 消费适配器」；Status 与 Links 都没有 0125。
  - `docs/adr/0125-…folds-in-the-domain.md` 索引注记「`RESTRICTED` 一律拒绝是票 04 落地前的过渡状态，不是产品口径」，读起来像现在仍在过渡；Status 与 Links 都没有 0132。
- **证据**：
  - ADR-0125 决定一把折叠搬进领域；0122 只在「owner 复核记录」里写了一句「决定四写的折叠位置……自 ADR-0125 起搬进 PS 领域……以本条与 ADR-0125 为准」。
  - 代码：折叠在 `internal/parcelshipment/domain/financial_control_result.go` 的 `NewExecutedFinancialControlResult`；适配器 `adapters/settlementaccounting/pre_acceptance_control.go` 只做 `jointPassConditionFor` 翻译。
  - ADR-0132 越权风险点 3「受限控制项结果加两格采用引用，扩了 ADR-0125 决定一的形——ADR-0125 正文不改，以本记录为准」；0132 后果节「ADR-0125 的过渡状态解除」。0125 只在正文末的复核记录里写了「过渡态解除 2026-09-10」。
- **归类**：A
- **严重度**：中。0122 索引注记会把作者引去在消费适配器里折叠，这正是 ADR-0025 / 0125 要排除的写法。
- **建议修法**：0122 的 Status 行与 Links 补部分停用：决定四里折叠位置那半句由 0125 决定一停用，归属不变；索引注记里「折叠在 PS 消费适配器」改为「在 PS 领域」。0125 的 Status 行与 Links 补：决定一「受限项只带自己的原因」的形由 0132 决定三扩两格采用引用；过渡态「`RESTRICTED` 一律拒绝」2026-09-10 由 0132 解除。

### PS-ADR-04　ADR-0017 与 ADR-0026 在持久化实现能不能写上相抵，彼此没有停用关系

- **类别**：3（两份已接受 ADR 互相矛盾而无停用关系）
- **位置**：`docs/adr/0017-admission-gates-judged-by-blocking-cause.md` 决定「**Bento 持久化技术闸门原样保留。** 它卡在外部不可变工件不存在，与租户参数无关。PostgreSQL Repository 实现、迁移、事务与 Outbox 仍然阻断，仍不得建立本地框架替身。」；后果节「「无持久化」与「无事件机制」仍阻断于 Bento 闸门」。0017 的 Status 行只有「Accepted」。
- **证据**：ADR-0026 决定「为产出消费者证明而写的持久化实现，在闸门通过前即可写；把实现登记为发布基线仍在闸门后」，Status 也只有「Accepted」，没写对 0017 的作用。代码里 Postgres 仓储、迁移与 Outbox 全在生产装配中。AGENTS.md「当前默认切片」把 0017 列为闸门边界的权威，读者只读 0017 会判 Postgres 代码越闸。
- **归类**：B（要裁的是：0026 是否构成对 0017 那一句的部分停用，适用场景怎么写）
- **严重度**：中
- **建议修法**：修订票代决，二选一：0017 的 Status 行补部分停用指针，「PostgreSQL Repository 实现、迁移、事务与 Outbox 仍然阻断」一句的适用场景收窄为「登记为发布基线」，由 0026 决定停用，「仍不得建立本地框架替身」不变，0026 的 Status 行同补；或者另立一份短 ADR 写明这一停用。写越权风险点。

### PS-ADR-05　ADR-0055 决定三「不取 401」已在操作者族（0100）与首方族（0139）解除，0055 一头无指针

- **类别**：3（停用链只有一头）
- **位置**：`docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md` 决定三「**三、状态码取 `403`，不取 404、401 或任何 5xx。**」；Status 行只有一条「数目勘误」，Links 里没有 0100 / 0139。
- **证据**：
  - ADR-0139 决定四「ADR-0055 Decision 三否决 `401` 的前件是「此刻不存在任何凭证方案」，在首方族上这个前件自本记录接受起不成立，否决随前件解除，仅限此族」；同记录决定一又写「ADR-0055 **不动**」。ADR-0100 对操作者族用的是同一句。
  - 代码：`internal/parcelshipment/adapters/http/query_shipment_request_views.go` 对 `codeCustomerClientCredentialRejected` 答 `http.StatusUnauthorized`，`customer_submission_intake.go` 定义 `CUSTOMER_CLIENT_CREDENTIAL_REJECTED`。
  - 「随前件解除」实质上就是带适用场景的部分停用，只是没有走 README 的机制。
- **归类**：A
- **严重度**：中。只读 0055 的作者会给首方或操作者凭据失败答 `403`。0055 属横切组；本条因 0139 在 PS 范围内列出，修订时与横切组对齐。
- **建议修法**：0055 的 Status 行与 Links 补部分停用：决定三对 `401` 的否决，适用场景各限操作者族（ADR-0100 决定四）与首方客户族（ADR-0139 决定四），理由是前件「此刻不存在任何凭证方案」在该族不成立，其余各条不变。

### PS-ADR-06　0139 把 `PAR-INT-01` 收窄、停用 0106 决定四之后，UC-PS-001 与首个消费者简报仍以现行口吻把首方接入的请求标识、格式与回执挂在 `PAR-INT-01` 上

- **类别**：1（被停规则仍以现行口吻留在现行文档），兼 2
- **位置**：
  - `docs/application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md` 输入语义契约之后「具体接入渠道、请求标识范围、字段格式、枚举、必填条件和传输回执等待 `PAR-INT-01` 及真实产品、合同和线路参数确认。」
  - `docs/design/parcel-go-first-consumer-slice-decision-brief.md`「`SourceRequestKey` 的真实组成由各接入适配器依据 `PAR-INT-01` 绑定」。
- **证据**：
  - ADR-0139 决定一「`PAR-INT-01` 的适用面收窄为「某租户的客户实际启用哪条渠道」与「租户既有渠道的适配所需证据」」，并据此停用 ADR-0106 决定四「客户渠道的采信身份属 `PAR-INT-01` 接入契约，是实例半边」；
  - ADR-0139 决定五「「接入请求标识」……由契约定义为载荷里的一格并由首方族的 `RequestKeyDerivation` 推导为来源请求键」；ADR-0140 与 ADR-0223 定了首方契约的字段与答复（`parcels`）。
  - 首方族的请求标识、字段、回执现在都是产品契约，只有租户既有渠道的适配还等 `PAR-INT-01`。
- **归类**：A
- **严重度**：中。作者会把首方口的字段或回执当作待租户参数留成未配置，或另造一套。
- **建议修法**：UC-PS-001 那句收窄为「租户既有渠道（标准文件、门户、客户自有契约的 API）的……等待 `PAR-INT-01`；首方 API 的请求标识、字段与回执由首方契约定（ADR-0139 决定五、ADR-0140、ADR-0223）」。简报那句同此收窄。UC-PS-005「首发试点叠加条件」里的 `PAR-INT-01` 一条属试点叠加，可保留，但建议注明只指既有渠道。

---

## 低

### PS-ADR-07　ADR-0061 决定一的`已拒绝`一半已由 0188 推进，0061 的 Status 行不带指针

- **类别**：3
- **位置**：`docs/adr/0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md` 的 Status 行只有「Accepted」；决定一「`已拒绝`、`已撤回`仍由入口以 `ErrRehydrationStateNotSupported` 拒绝」。Links 那条写「Status 与其余各条不变」。
- **证据**：ADR-0188 索引注记「**推进** ADR-0061 决定一里`已拒绝`那一半」；代码 `internal/parcelshipment/domain/rehydration.go` 的 `admitRehydratedState` 已收 `ShipmentRequestRejected`。README 的部分停用机制要求 Status 行与 Links 两头都有指针，这里只有 Links。
- **归类**：A
- **严重度**：低
- **建议修法**：0061 的 Status 行补索引注记，写明决定一里「`已拒绝`」那一半由 0188 停用、`已撤回`照旧，并删掉 Links 里「Status 与其余各条不变」。

### PS-ADR-08　两份 ADR 的 Links 有死链，其中一条的说明也过时

- **类别**：5
- **位置**：
  - `docs/adr/0045-new-submission-version-keeps-history-and-reestablishes-the-task.md` Links：`./0028-aggregate-revision-moves-only-at-the-commit-boundary.md`、`./0030-rehydration-admits-only-the-submitted-state.md`（后一条说明「重建门只收`已提交`」）；
  - `docs/adr/0054-pre-acceptance-control-policy-view-has-an-unconfigured-grade.md` 决定二与 Links：`./0029-recovery-action-is-the-error-algebra.md`。
- **证据**：三个目标文件都不存在；现行文件名是 `0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md`、`0030-rehydration-admits-one-state-at-a-time-by-snapshot-expressiveness.md`、`0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md`。重建门现在收`已提交`、`已接受`、`已拒绝`（ADR-0061、0188）。
- **归类**：A
- **严重度**：低
- **建议修法**：改 Links 的链接目标，0045 那条说明改成「一个状态只有在快照能表达它可达的全部字段时才准进」。0054 决定二正文里那条链接，照 README「标注正文中已失效的引用坐标」的办法，在 Status 行注明坐标已移位，或者只改链接目标、不改文字，由修订票定。

### PS-ADR-09　同一次停用在同一份 ADR 里写了两到三遍（infra-speed/07）

- **类别**：3（同一次停用不写两遍）
- **位置与证据**（Status 行标记前段与 `<!-- 索引注记 -->` 之后各写一遍同一次被停用）：
  - PS：0081（0158）、0086（0106）、0094（0106）、0106（0139）、0151（0187、0230、0236，三次各两遍）、0183（0187）、0195（0211、0212）。
  - 邻接：0199（0210、0211、0212），0210（0211），0064（0156），0075（0215）。
  - 第三份：0048 与 0184 在 Status 行之外另有一行 `Partially suspended by:`，与 Status 索引注记、Links 是同一事实的三份。
  - 接受出处重说（索引注记「**已接受**（……）」与标记前段同义）：0139、0140、0179、0183、0184、0185、0187、0188、0195、0211、0212、0223。
- **归类**：A
- **严重度**：低
- **建议修法**：照 `.scratch/infra-speed/issues/07` 的做法逐份合并进索引注记，只有一边有的事实并入，不丢；标记前段写「本记录停用了谁」的不动。`Partially suspended by:` 行并进 Status 行后删掉。接受出处那一类按 07 二选一，写进完成记录。

### PS-ADR-10　可改写处的计数已过时：ResumePath 已是五格

- **类别**：5（计数）
- **位置**：
  - `docs/adr/0106-…md` 索引注记「**适用场景**：`ResumePath` 四格自此只剩`等待内部续办`一格回滚重投」；
  - `docs/adr/0086-…md` Links「CONTEXT.md：接受判断任务三个等待态「使用不同原因和续办路径」」。
- **证据**：代码 `internal/parcelshipment/domain/acceptance_task.go` 有五格，含 `ResumeByAuthorizedDisposition`（ADR-0132）；PS CONTEXT「接受判断任务」生命周期写了五个等待态。0094 / 0106 正文的「四格」是当时的历史，不动；可改的只有索引注记与 Links。0094 与 0106 的 Status 行都没有指向 0132 的补充指针。
- **归类**：A
- **严重度**：低
- **建议修法**：去掉计数，改成「`ResumePath` 自此只有`等待内部续办`一格回滚重投」与「接受判断任务的各等待态」。0094 的 Status 行仿 ADR-0140 对 0223 的写法加「**补充**：ADR-0132 增`ResumeByAuthorizedDisposition`一格，处置为提交入账」。

### PS-ADR-11　被停条款引文不逐字（0139 → 0072）

- **类别**：3（所引原句逐字存在）
- **位置**：ADR-0139 决定一与 `docs/adr/0072-…md` Status 行都引「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据到位前不立」。
- **证据**：0072 决定二原文是「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」，引文省了括注，也没打省略号。同类问题还有 PS-ADR-02 的「结算账户目录留 nil」。
- **归类**：A
- **严重度**：低
- **建议修法**：0072 的 Status 行改引原句，或用「……」标出省略。0139 的正文不动。

### PS-ADR-12　ADR-0118 索引注记的「今天」已不成立

- **类别**：4（文档与代码），兼 1
- **位置**：`docs/adr/0118-amendment-stage-facts-enter-parcel-shipment-through-consumer-side-read-ports.md` 索引注记「CC/NO 今天无按包裹键的读面，两只未接适配器答`不知道`，缺口归提供方立票（ps-port-remainder/05）」。
- **证据**：`cmd/parcel-api/assemble_customer_amendment.go` 的 `buildAmendmentStageFactViews` 已接 `ccpostgres.NewParcelDeclarationFactsView` → `pscustoms.NewCustomsStageView`，以及 `nopostgres.NewParcelContainmentView` → `psnode.NewConsolidationStageView`。
- **归类**：A
- **严重度**：低
- **建议修法**：索引注记那半句改成过去时，并补「ps-port-remainder/05 后两只适配器已接提供方真读面」。

### PS-ADR-13　CONTEXT-MAP `parcel-shipment → network-routing` 仍以无条件口吻写着 ADR-0075 决定一被部分停用的那句

- **类别**：1
- **位置**：`docs/domain/CONTEXT-MAP.md`「`parcel-shipment → network-routing`」一条：「网络与路由不建立读取小包托运地址的端口」。
- **证据**：ADR-0075 决定一「`network-routing` 不建立读取 `parcel-shipment` 地址的端口」已由 ADR-0215 决定二部分停用，适用场景是受理后、不由小包托运信封触发的复核，由 NR 的消费侧适配器读委托聚合、取基线地理投影。CONTEXT-MAP 同一条后文虽补了这一例外，前句仍是全称。
- **归类**：A
- **严重度**：低
- **建议修法**：前句加限定：「接受前与小包托运信封触发的判断里，网络与路由不建立读取小包托运地址的端口」，与后句的例外接上。

### PS-ADR-14　ADR-0185 停用了 0184 决定一，自己却没有 Links 节，也没有「对已接受记录的作用」一节

- **类别**：3
- **位置**：`docs/adr/0185-external-identifier-relation-identity-includes-its-formation-basis.md`：各节只有 Context、Decision、Consequences、越权风险点、复核记录，停用只写在 Status 行「部分停用 ADR-0184 决定一的键，其余各条不变」。
- **证据**：被停方 0184 的 Status、`Partially suspended by:` 与 Links 三处都有指针；停用方只有 Status 一处，不引被停原句。同组的 0195、0210、0211、0212 都有「与已接受决定的关系」一节，逐句引原句。
- **归类**：A
- **严重度**：低
- **建议修法**：0185 的 Status 行补被停原句「键（租户，标识类型，分配方，号码，被标识对象）」与适用场景。正文不加节；可加 Links 节，指向 0184、0195。

---

## 范围外顺带记下的（不计入）

- ADR-0152 决定八「不做：给结算、路由或任何下游消费试算结果的口」与 ADR-0179 让结算调用 PP 受理前试算之间有张力，0179 越权风险点 2 自己点过。属 PP / SA 组。
- ADR-0138（Proposed）未接受。PS CONTEXT 已写「源事实更正或事件有效性变化使终局失效后……重新派生」，代码对 TF 作废的收寄版本显式答 `ErrVoidedCarrierPickupRederivationUndecided`（`adapters/transportfulfillment/judge_on_carrier_first_effective_pickup.go`）。这是 CONTEXT 与代码之间一个已知、显式的未决，交主报告判要不要列进目标 4。
- 首个消费者简报「六处委托 `!found`」是计数引用（目标 5），交主报告。

## 已核无问题的要点

| 链 / 事项 | 结论 |
|---|---|
| 0086 ← 0106（Context 受控补充分侧） | 两头指针都在；原句除 `**` 粗体外逐字存在；0106 决定五写明作用 |
| 0094 ← 0106（决定三受控补充格） | 两头都在；原句（含「……」省略）存在 |
| 0106 ← 0139（决定四采信身份） | 两头都在；原句逐字存在 |
| 0022 ← 0140（4xx 行对 429） | 两头都在；表格行文字存在（引文用全角｜代表表格分隔） |
| 0140 ← 0223（补充，不停用） | 0140 的 Status 与 Links 都有补充指针；「不在本记录定任何端点的字段」逐字存在 |
| 0151 / 0183 ← 0187 | 两头都在；两句原句逐字存在；0187 决定七写明适用场景 |
| 0151 ← 0230、0236 | 两头都在；原句逐字存在 |
| 0184 ← 0185、0195、0212 | 0184 的 Status、Links 与复核记录都在；四句原句逐字存在 |
| 0195 ← 0211、0212 | 两头都在；六处原句逐字存在；0211、0212「与已接受决定的关系」逐句一致 |
| 0199 ← 0210、0211、0212；0210 ← 0211 | 两头都在；原句逐字存在 |
| 0048 ← 0179；0064 ← 0156；0075 ← 0215 | 两头都在（重复问题见 PS-ADR-09） |
| 0072、0085、0100、0101 ← 0139 | 四份的 Status 与 Links 都有指针（0072 的引文问题见 PS-ADR-11） |
| 0188 推进 0061 | 0188 一头完整（0061 的 Status 行见 PS-ADR-07） |
| 外部标识与歧义信号在 PS CONTEXT、CONTEXT-MAP `PS → NO` 里的口径 | 与 0184、0185、0195、0211、0212 现行合成一致；被停各句没有一句以现行口吻留在 PS CONTEXT、UC 或 GLOSSARY |
| 受控关闭请求证据与操作证据（CONTEXT、GLOSSARY） | 与 0187 一致 |
| 五个等待态（CONTEXT Rules 与「接受判断任务」生命周期） | 与 0094、0106、0132 及代码 `ResumePath` 五格一致 |
| 时点「提交接收」（CONTEXT Rules） | 与 0157 一致 |
| 0138（Proposed） | PS 文档与代码均未引作依据；代码按未决处理 |
| 0153 决定四的「初稿」修订 | 修订写在正文内并有说明，不构成链问题 |
| PS 现行文档（CONTEXT、7 份 UC、简报）的相对链接 | 全部可解析 |
| PS ADR 的 Status 行与 Links 里的行号引用 | 没有 |
