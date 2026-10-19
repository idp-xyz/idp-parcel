# 04 票 02 评审遗留收口：ADR-0184 与 ADR-0185 复核记录、UC-NO-002 同步，入口去空白与同号核对的现状钉成用例

Category: chore
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 恢复（分支 `claude-psxm04` 上已有半成品，先 rebase）。此前 ready-for-agent——2026-10-18 暂缓：用户因 token 花费选「精简续做」，本票没做完；分支 `claude-psxm04`（基 `b58b10ef`，工作树 `/home/tops/workspace/idp-parcel-claude-psxm04`）上留有1 笔（入口去两端空白与逐字核对的钉住用例），未经评审，接手前先 rebase 到最新 main 并核一遍。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 序 0 评审遗留；用户授权代决）；分支 `claude-psxm04`
Blocked by: 无
父票：[01](./01-external-mark-relations-have-no-model-in-parcel-shipment.md)（已 resolved）。被评的是 [02](./02-register-and-customer-parcel-numbers-feed-node-identity.md)，代码 `cc52e9b6`，建模 `5714a137`。
地盘：
- docs/adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md
- docs/adr/0185-external-identifier-relation-identity-includes-its-formation-basis.md
- docs/adr/README.md
- docs/application/node-operations/UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md
- internal/parcelshipment/adapters/http/isolated_write_intake.go
- internal/parcelshipment/adapters/http/isolated_write_intake_test.go
- internal/parcelshipment/adapters/postgres/external_identifier_test.go
- cmd/parcel-api/assemble_reception_identity_test.go
- docs/product/MECHANISM-INVENTORY.md
- docs/development-artifacts/ps-external-mark-relations/02-register-and-customer-parcel-numbers-feed-node-identity.md
- .scratch/ps-external-mark-relations/issues/02-register-and-customer-parcel-numbers-feed-node-identity.md
- .scratch/ps-external-mark-relations/issues/03-channel-parcel-numbers-from-label-results-void-and-replacement.md
- .scratch/operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

ADR-0184、ADR-0185 越权风险点的 owner 复核行不在本票（归 operator-channel/51）；本票只写事实更正与补记条目。

## 出处

票 02 Comments 里「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `cc52e9b6`」那一条，结论列了「仍待 PS owner 裁」的三格：

- Spec (1) / Standards (1)：去两端空白进了册键。
- Spec (3) / Standards (4)：决定六与 UC-NO-002「权限、隔离与审计」相抵。
- Spec (4) / Standards (2)：已接受的 ADR-0184 后果节被原地改写。

汇总在 [operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)「评审结果」序 0。研判钉 main `8d22729f`；其后 main 前进到 `627dde00`，没碰本票任何文件。

## 为什么

这三格评审两轴都判非阻断，交 PS owner。研判与两路对抗核验各起了探针（放在会话草稿区，经 `go test -overlay` 挂入，不入库，证据只记 `S`），量出三件事：

- 入口去空白是既有纪律；册与核对一处都不折算。
- 决定六之下，除了「两边都在册 → 冲突」，还有「只一边在册 → 唯一误关联」。
- ADR-0185 有同日同形的后果节改写。

本票把裁定落成复核记录与 UC 同步，并把用到的事实钉成入库用例。复核记录只引入库用例，不引草稿区探针。

## 裁定（2026-10-16，会话 idp-parcel-9b 代决）

- **去两端空白进册键：维持代码，按事实更正记。** 入口去两端空白，定的是委托记下的声明值；领域、册、核对读口与 NO 适配器照旧不折算。越权风险点 2 漏看了这个入口行为，照同节「越权风险点 4 的事实更正」的写法更正前提，风险点 2 原文不动。边界：入口只去单元格两端的空白，号码本身的字符一个不动。现场观察一侧交 NO owner（no-reception-identity/02）。
- **决定六与 UC-NO-002：维持决定六，不立新 ADR。** 收寄当下没有可核验的客户账户；租户隔离始终成立。UC-NO-002 同步两处，只引用权威。两格代价如实记下，风险点 1 一并更正。唯一误关联另立 ps-external-mark-relations/05，一码两物另立 no-reception-identity/01。
- **后果节原地改写：保留现行文字、不回滚，补记；同形的 ADR-0185 一并补记。**

规则全文就是下面的「条目草案」。落进 ADR 之后以 ADR 为准，本票不另立口径。

## 做什么

先写用例（条目要引它们的符号名），再改文档。

1. **用例**（下面是建议名；改了名，就同步改条目里的引用）
   - `internal/parcelshipment/adapters/http/isolated_write_intake_test.go` · `TestIsolatedSubmissionIntakeTrimsOnlyTheEndsOfCustomerReferences`：
     - 客户包裹参考两端带空格与 Tab 的草案：命令里的客户包裹件号是去了两端空白的值，载荷摘要与不带空白的同一草案相同。
     - 参考是小写、带内部空格、两端也带空白的草案：号码只去两端，大小写与内部空格原样。
     - 去掉 `customerDeclaredReferences` 里那处 `strings.TrimSpace` 时，这条用例必须红。研判时的变异实测：去掉之后 `shipmenthttp` 现有用例全绿，没有一条钉住它。
   - `internal/parcelshipment/adapters/postgres/external_identifier_test.go` · `TestTheRegisterKeepsNumbersVerbatimAcrossWhitespace`（真库）：
     - 两端带空白交来的号码原样落册：按原样查得到，按去了空白的号查不到。
     - 不带空白落册的号：按带尾随空格、前导空格或尾随回车换行的号去查，都是零件。
   - 新文件 `cmd/parcel-api/assemble_reception_identity_test.go`（真库，经生产装配 `buildReceptionOrchestration`）。不写进 `assemble_reception_test.go`，免得与同期改那份文件的票相撞。
     - `TestTheWiredReceptionAnswersAConflictWhenTwoAcceptedRequestsShareANumber`：两个客户账户的已接受委托用了同一个客户包裹件号。`deliveredBy` 写客户一、客户二还是一个代理交付方，都答待识别、身份冲突、两个候选、控制建立。
     - `TestTheWiredReceptionAssociatesTheOnlyRegisteredHolderOfANumber`：这是已知代价的刻画，不是要守的规则。(a) 客户账户一的委托已接受、号在册，客户账户二的委托用同号只提交：客户二的实物送站，答`节点收寄已形成`，关联到客户一的包裹。(b) 同一客户账户的第二份委托用同号只提交：它的实物唯一关联到第一份的包裹，第二份也接受之后才答身份冲突。头注写明：它刻画的是条目「决定六与 UC-NO-002「权限、隔离与审计」」记下的代价，票 ps-external-mark-relations/05 裁定之后按裁定有意翻转。
2. **ADR-0184**：在「复核记录（2026-10-16，接受当日；决定原文不动）」里接现有编号，追加下面三条。Decision、Consequences、越权风险点原文一字不动。
3. **ADR-0185**：文末新起「## 复核记录（2026-10-16，接受当日；决定原文不动）」，记下面一条。
4. **UC-NO-002**：改两处，见「UC 改动」。
5. **ADR README**：ADR-0184 那一行的「**越权风险点**六条待 PS owner 复核。」改为「**越权风险点**与复核记录里留给 owner 的问待复核。」。复核记录多了留给 owner 的问，按计数写的摘要会无声变旧。
6. **注释**：`internal/parcelshipment/adapters/http/isolated_write_intake.go` · `customerParcelNumbers` 的头注，去掉「号码照客户给的原样」，改成三层意思：号码取 `customerDeclaredReferences` 定下的声明值；两端空白已去，号码本身的字符一个不动；为什么这不算条码折算，见 ADR-0184 复核记录「越权风险点 2 的事实更正」（按标题引，不写条目序号）。生产代码只改这段注释。
7. **票面**：
   - 票 02 Comments 记本裁定一条（立票时已记就核对）。
   - 票 03 Comments 记一句「ADR-0185 后果节的原地改写已在其复核记录补记」。
   - operator-channel/26「评审结果」表下追加一句，不改表格原格：序 0 待裁的三格已于 2026-10-16 由会话 `idp-parcel-9b` 代裁，见 ADR-0184 复核记录三条标题与本票；另立 ps-external-mark-relations/05、no-reception-identity/01 与 02。

### 条目草案 · ADR-0184（接现有编号）

3. **越权风险点 2 的事实更正**（2026-10-16；出处：非作者会话 `idp-parcel-9b` 对票 02 的评审，Spec 与 Standards 两轴都提「去两端空白进了登记册的键」。同一会话据用户原话「你作为业务和系统专家你来自决」代 PS owner 裁定：维持，不改生产代码；仍请 PS owner 过目）：风险点 2 说「现场扫描与客户录入常差在大小写、前导零或两端空白」，要去两端空白也「不进登记册的键」。它漏看了一个既有的入口行为：客户录入一侧的两端空白，在进册之前已经去掉了。管理台 `buildDraft` 送出前逐格去一次，`IsolatedSubmissionIntake` 的 `customerDeclaredReferences` 对委托参考与逐件客户包裹参考再去一次，`customerParcelNumbers` 取的就是去过的值。所以经今天唯一的提交入口进册的客户包裹件号，不带两端空白。这一行随 `2b4186d2` 封存入库（那笔自称「未代为评审」），此后改这份文件的各笔都留着它，不是本记录加的。这里更正前提，风险点 2 原文不动；这条入口纪律自本条起以本条为准：
   - **它定的是声明值。** 去两端空白是 UC-PS-001「参与者与上下文协作」派给接入适配器的「转换为统一输入」中的一步。载荷摘要的成员键、委托快照里的配对与册里的号码同出这一个值，委托没有第二个声明值。Intake 去两端空白的是这类标识格，地址等内容条目的值原样进摘要。
   - **边界。** 入口只去单元格两端的空白，号码本身的字符一个不动：大小写、前导零、校验位、号码内部的空格都不改。改号码字符的规则，就是 PS CONTEXT「按外部标识核对包裹身份」那一条说的条码规则，属租户取值，今天`未配置`，在入口同样不许做。
   - **比对一侧照旧逐字。** 决定五与 PS CONTEXT 那一条管的是核对，不受本条影响。领域 `NewExternalIdentifierNumber`、册的写入、核对读口 `FindParcelsByExternalIdentifier` 与 NO 的 `ParcelIdentityView` 都不折算：两端带空白交来的号码原样落册；扫到的号多一个空格、带回车换行或大小写不同，核对答零个候选，NO 走待识别，不会关联错件（`TestIsolatedSubmissionIntakeTrimsOnlyTheEndsOfCustomerReferences`、`TestTheRegisterKeepsNumbersVerbatimAcrossWhitespace`、`TestTheSameNumberFromTwoCustomersAnswersBothParcelsVerbatimWithinTheTenant`）。入口去空白也不制造新的错配：去空白后撞号的，同一草案在规范化那一步答 4xx（本节「越权风险点 4 的事实更正」）；跨委托的，与本来就同号没有区别，落到本节「决定六与 UC-NO-002「权限、隔离与审计」」那两格。
   - **不在本条。** 现场观察一侧归 NO：受控批量口的收寄模板（`decodeTemplate`）逐格去两端空白，在线收寄口（`receptionCommand`）把 `mark` 原样交进命令与收寄内容摘要。扫描附带的回车换行由谁去、两口要不要一致，交 NO owner（票 no-reception-identity/02）。风险点 2 留给 owner 的那一问——核对读口做「去两端空白」之类的折算，算不算产品策略——不因本条关闭；要做，先改 PS CONTEXT 那一句（CONTEXT 高于本记录）。
   - **留给 PS owner**，与本节「越权风险点 4 的事实更正」末句那一问一起复核：本册按号码跨来源查，写册的入口口径不一时，不去两端空白的那个入口进来的号，正常扫描就配不上。渠道包裹件号今天原样取自渠道结果，只是面单渠道写链还没有触发面。要不要把「进册号码不带两端空白」定为所有写入方的约定（交不出就拒收，不在册里折算），须在那条写链有触发面、真渠道或首方 API 的接单入口上线之前有答案。
4. **决定六与 UC-NO-002「权限、隔离与审计」**（2026-10-16；出处同上，两轴都提，本记录接受时没读那一节。同一会话据同一授权代 PS owner 维持决定六，并代 NO owner 同步 UC-NO-002 那一节首条与「输入语义契约」里「包裹身份依据」一行；两处仍请各自 owner 过目）：
   - **读法。** 那一节首条「收寄和身份核对必须在明确集团租户、客户账户、节点和作业责任范围内执行」，要与同节「跨客户批量作业只允许最小必要信息」连读；它又是 UC-PS-003、UC-NR-001、UC-NR-003 同名节的同一句式。两相对照，它说的是授权与隔离。候选怎么取，CONTEXT-MAP `parcel-shipment → node-operations` 与 PS CONTEXT「按外部标识核对包裹身份」那一条已经定了：在租户内按号码取，不按分配方挑一件；权威次序上二者高于用例。同一份用例的「参与者与协作」写交付方「提供可验证的交付范围和身份线索」，可见用例设想过按交付范围识别，只是那份范围今天没有模型：作业事实信封交给收寄编排的只有租户与节点，载荷的 `deliveredBy` 只验非空，受控批量口同样只有 `deliveredBy`。按这一格的自报收窄，自报一错，身份冲突就变成唯一关联；按册上的 `scope` 过滤，还会把渠道包裹件号排出去。所以今天不收窄，决定六维持。租户隔离始终成立：读口按 `tenant_id` 过滤，租户取自认证结果或运维参数，他租户同号答零个候选。
   - **不收窄的代价有两格，都记下**（`TestTheWiredReceptionAnswersAConflictWhenTwoAcceptedRequestsShareANumber`、`TestTheWiredReceptionAssociatesTheOnlyRegisteredHolderOfANumber`）。
     - 其一，两份已接受委托撞号：答身份冲突、两个候选，包裹停在待识别，直到「身份后来被确认」（`AT-NO-026`）有执行器。
     - 其二，号只在一份已接受委托里在册：别的实物贴着同号送站，会唯一命中那一件，形成`节点收寄已形成`并建立控制。跨客户账户会这样（另一方的委托还没接受，或它根本没报这个号）；同一客户账户在两份委托里重复用号也会这样。提交入口只在同一草案内查重，所以后一种今天的演示租户就走得到。PS 采用这份收寄时，按关联到的包裹反查目标委托（`AdoptOnNodeIntakeAdapter.HandleFormedNodeIntake` 经 `FindCurrentAcceptedByParcel`），客户账户取自那份委托。所以 UC-PS-003「适用资格判断」里「来源所属租户、客户账户、责任法人和包裹与接受基线一致」那一项，对节点收寄来源拦不住它。这一格的修法另立票 ps-external-mark-relations/05，须在生产形态第一条接单入口上线之前裁定并修好。
   - **风险点 1 的更正。** 「接受之前送站答待识别」只在别处没有同号在册时成立；有，就是上面「其二」那一格。
   - **留给 PS owner 与 NO owner**（随票 05 裁）：PS CONTEXT「外部标识关联」说，客户账户给的号在该客户账户内算数。没有可验证的交付范围时，这类号的唯一命中，算不算 UC-NO-002 应用流程「唯一且证据充分时关联」里的「证据充分」？NO 端口 `ParcelIdentityView` 的头注写的是「恰一个→关联」，仓里没有记录论证过。
   - **日后。** 要让交付范围参与候选，不论收窄还是消歧，前提依次是：NO 给可验证的交付范围建模；先改 PS CONTEXT「按外部标识核对包裹身份」那一条与 CONTEXT-MAP 同句；再另立记录停用本条决定。怎么用、范围外的唯一命中怎么答，留给那篇记录定，本条不预设。哪个授权交付方可以替哪个客户账户送货，属租户取值。
5. **后果节在接受后被原地改写的补记**（2026-10-16；出处同上，两轴都列为非阻断。同一会话据同一授权补记，决定原文不动）：本记录在 `5714a137` 以 Accepted 立下。同日，票 02 的代码提交 `cc52e9b6` 在同一分支、进 main 之前，把 Consequences 以「PS 迁移」起头那一条的开头原地改了。接受时原句是「PS 迁移一份（关联表、适用变化表、按号码查的索引）」；现行文字是「PS 迁移两份：关联表与按号码查的索引随客户包裹件号进来，适用变化表随渠道来源的作废与替代进来（客户包裹件号没有作废或替代的来源事实）」。同一条其余各句、Decision 与越权风险点都没动。理由见票 02 完成记录「评审中改的」：票 02 没有任何来源事实会产生作废或替代，适用变化表挪到票 03，与渠道来源同笔进来。这是实施拆分的如实更正，但改的是已接受记录的正文。照 AGENTS.md「改文档」里「不改写已接受 ADR 历史」，它本该像本节「越权风险点 4 的事实更正」那样追加在这里。现行文字与落地相符（迁移 `0024`、`0025`；`0025` 同笔另含 ADR-0185 的改键，见该记录），所以保留、不回滚：回滚是对正文的又一次改写，还会让后果节与迁移对不上。接受时的原句，以本条引文与 `5714a137` 为准。

### 条目草案 · ADR-0185（文末新起一节）

## 复核记录（2026-10-16，接受当日；决定原文不动）

1. **后果节在接受后被原地改写的补记**（出处：会话 `idp-parcel-9b` 裁票 02 评审遗留里 ADR-0184 后果节原地改写那一格时，一并核到本记录同日同形的一处。同一会话据用户原话「你作为业务和系统专家你来自决」补记，决定原文不动）：本记录在 `58c4c496` 以 Accepted 立下。同日，票 03 第二轮评审修补 `d8030e69` 在同一分支、进 main 之前，把 Consequences 以「不在本记录」起头那一条整句改写了。接受时原句是「不在本记录：同一条关联同时收到作废与替代时按写入先后立住终态，而不是按业务时间比先后——读口的答案不受影响（两者都让它不再算数），只是记下的终态种类可能与业务先后不符。票 03 的完成记录如实记为已知局限。」现行文字补进了「同一笔交易上后补一条业务时间更早的作废」那一格，理由见票 03 完成记录第二轮评审「ADR-0185 漏记「后补更早作废」一格」。Decision 与越权风险点都没动。同一笔把 `Status` 行的「修订」改称「部分停用」，属 ADR README「已接受决策」节头许可的标注，不在此列。现行文字与落地相符，保留、不回滚；接受时的原句，以本条引文与 `58c4c496` 为准。

### UC 改动（UC-NO-002）

- 「权限、隔离与审计」首条，原句之后接：其中客户账户一维今天不作身份候选的检索条件。候选怎么取，以 [CONTEXT-MAP](../../domain/CONTEXT-MAP.md#关系约束) `parcel-shipment → node-operations` 与 [小包托运 CONTEXT](../../domain/parcel-shipment/CONTEXT.md#rules-and-invariants)「按外部标识核对包裹身份」那一条为准；「参与者与协作」里交付方要提供的可验证交付范围，今天还没有模型。由此留下的局限，以及日后让交付范围参与候选的前提，见 [ADR-0184](../../adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 复核记录「决定六与 UC-NO-002「权限、隔离与审计」」。
- 「输入语义契约」里「包裹身份依据」一行：最低业务语义改为「租户、正式包裹、接受基线、当前身份与外部标识关联」（去掉「客户账户」）；使用约束改为「节点只读引用，不能自行创建或合并正式包裹；身份核对交回什么，以 [CONTEXT-MAP](../../domain/CONTEXT-MAP.md#关系约束) `parcel-shipment → node-operations` 为准」。

## 不做

- 生产行为一处不改：Intake、册的写入、核对读口、NO 防腐适配器、两条收寄口都不动。
- 不定现场观察一侧的口径（no-reception-identity/02），不修唯一误关联（ps-external-mark-relations/05），不修一码两物（no-reception-identity/01），不补「身份后来被确认」（`AT-NO-026`）。
- 风险点 2 留给 owner 的核对折算一问，本票不关闭；「进册号码不带两端空白」是否成为写入方统一约定，本票也不定。两问都写进条目，交 PS owner。
- 隔离形态的来源保全只存载荷摘要、不存原文，与 UC-PS-001 输入语义契约「来源信封」一行要的「原始内容或其不可变引用」有差。这是既有缺口，另案处理；条目不拿它当论据。
- 票 02 评审 Standards (3)（`service_results.go` 包头注计数）不在本票，照票 02 Comments 的结论，随下一笔碰到该文件时改。

## 完成判据

- [ ] 「做什么」第 1 项的用例入库：Intake 那条在去掉 `customerDeclaredReferences` 的 `TrimSpace` 时红；真库用例带 DSN 绿（缺 DSN 时照 `pgtest` 惯例：本地跳过，CI 上报错）。
- [ ] ADR-0184 追加的三条、ADR-0185 新起的复核记录，与本票「条目草案」一致；条目引的用例符号名与入库的一致。`git diff` 显示两份 ADR 只在复核记录处增行，Decision、Consequences、越权风险点原文无改动。
- [ ] UC-NO-002 按「UC 改动」改好；README 那一行不再计数。
- [ ] `customerParcelNumbers` 头注不再说「照客户给的原样」，按条目标题引复核记录。
- [ ] 票 02、票 03 的 Comments 与票 26「评审结果」表下各有一句。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/parcelshipment/...`、`cmd/parcel-api`、`internal/architecture` 全绿；机制清点在本笔的干净检出上重生成，有差异就随笔提。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审（空闲通道或 `/code-review` 的隔离子代理）、推送方在干净检出上带 DSN 跑全量。
