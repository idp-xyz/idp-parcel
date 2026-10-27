# 54 CR 运营企业真实到账的证据引用不指向结算已采用的外部资金事实，同一笔回款可以在两处各铸一次

Category: bug
Status: resolved——2026-10-27 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code54。R3（COD 进入试点之前必修）：COD 首发不在试点范围（`PAR-COM-11`），今天只有演示租户经受控 CLI 登记；但真实到账一旦按今天的形接真渠道，同一笔银行回款会在 settlement-accounting 的采用与 collection-remittance 的真实到账各存一份，两边各自守恒、合起来重记，只能靠线下对账发现。2026-10-21 文档自洽 10（10a 半边）立（用户「你作为业务和系统专家自决」授权），分支 claude-dc10a。
Blocked by: 无（结算一侧被引用部分怎样映射，见「要裁的」，不挡 CR 一侧的修法）
出处：[audit/cross.md](../audit/cross.md) CM·M-2；裁定 [ADR-0274](../../../docs/adr/0274-collected-principal-bank-arrival-enters-through-the-settlement-accounting-funds-adoption.md)；用例 [UC-CR-001](../../../docs/application/collection-remittance/UC-CR-001-REGISTER-COLLECTION-AND-POST-THE-SUBLEDGER.md) `AT-CR-020`。

## 现象

- collection-remittance CONTEXT「代收义务与实收事实」（ADR-0274 之后）：运营企业真实到账只经 `settlement-accounting` 的采用进入本产品，登记这一层代收事实时证据引用必须指向一条已采用的外部资金事实，按引用读回那一版在册、币种与指令相同才接受；引用同一条已采用事实的各条真实到账，金额之和不超过该事实金额。
- 代码对真实到账的证据引用不作任何核对：任意字符串都能登成`运营企业真实到账`。

## 证据

- `internal/collectionremittance/domain/collection_fact.go`：`AcceptCollectionFact` 对 `Evidence` 只核非空（`EvidenceReference.valid()`），四个来源层级同一待遇。
- `internal/collectionremittance/application/register_collection.go`：`AcceptFact` 只核指令在册，`Deps` 里没有 settlement-accounting 的读口。
- `cmd/parcel-collection-register/translate.go`：事实输入的 `evidenceRef` 是一格自由字符串。
- settlement-accounting 已有可读的口：`internal/settlementaccounting/ports/ports.go` 的 `AdoptedFundsFactView`（customs-compliance 的 `adapters/settlementaccounting/adopted_funds_fact_source.go` 就是按引用读它）。

## 三类

机制：真实到账按引用读已采用事实并核在册、币种与金额之和。产品策略：无。租户取值：哪条回款对应哪几条指令由代收运营登记，本产品不推导。

## 规模

S–M：CR 加一个消费侧适配器读 `AdoptedFundsFactView`；`AcceptFact` 对`运营企业真实到账`层按引用读回、核币种、核同一采用事实下各条到账金额之和（读回与落册同一事务）；CLI 输入把这一层的证据写作采用引用；演示种子的真实到账先在结算采用一条合成资金事实再引用它；补用例（不在册答依据不在册、币种不同答未受理、超额答内容冲突、多条按指令拆分合计不超额照登）。

## 最小的诚实修法

照 ADR-0274 决定一、二做 CR 一侧；结算一侧在 SA owner 定之前不动：被引用的那部分在 `UC-SA-005` 里停在未分配，看得见、不进运营结算余额。

## 要裁的

- 结算一侧怎样处置被代收与清分引用为真实到账的那部分（ADR-0274 越权风险点 2）：给 `UC-SA-005` 加一种「代收本金」映射目标（金额与引用它的各条真实到账之和对得上），还是保持未分配并另立对账口径。归 SA owner。

## 裁向（2026-10-27，作者代决，[ADR-0330](../../../docs/adr/0330-a-collected-principal-bank-arrival-is-registered-against-an-adopted-funds-fact-that-settlement-has-set-aside-as-collected-principal.md)）

「要裁的」那一条（结算一侧怎样处置被引用的那部分）裁为第一种的加强版：给 `UC-SA-005` 加映射目标`代收本金`，**整条**认作、与这条事实上任何别的映射互斥、不核销、不进入账余额、不可撤；不取「保持未分配并另立对账口径」，也不取按金额部分核销。理由：保持未分配挡不住同一条事实之后被映射到对账单或账户（只落一个作用域的收款，没核销的部分整条记入入账余额，ADR-0283 决定二），同一笔 1000.00 的回款在运营余额记一次、在`待清分`再记一次；按金额部分核销则结算撤销核销时代收与清分已登记的到账失去依据，而结算看不见那边。代收与清分一侧：到账的证据引用即这条事实的引用，登记与移位都读事实此刻的链头，重放、重登、迟到、被结算更正各有答案（ADR-0330 决定四与手算三例）。

## 完成记录（2026-10-27，分支 claude-code54，基于 main `d497a2fa1`，收尾 rebase 到当时的 origin/main）

### 做了什么

- 结算一侧（`internal/settlementaccounting`）：领域 `TargetCollectedPrincipal`（目标等于事实自己、只认确认到账、`ApplySettlement` 拒分配到它）；映射编排 `Map` 对每种目标都先取事实锁，再经新端口方法 `FundsMappingStore.ListByFact` 读回该事实上已有映射，`collectedPrincipalExcludes` 判互斥，冲突答 `MAPPING_CONFLICT`（结果代数不增格）；新只读口 `ports.CollectedPrincipalFundsView` 与 Postgres 实现 `CollectedPrincipalFundsView`（对事实身份行 `FOR SHARE`，再读链头与认作状态）；Postgres 映射读回词表、受控 CLI 译装词表、CLI 对映射冲突的文案、管理台目标种类词表各补一格。迁移 `settlement_accounting/0048`：词表、认作的目标须等于事实自己、一事实至多一条认作（部分唯一索引）。
- 代收与清分一侧（`internal/collectionremittance`）：领域 `AdoptedFundsReference`、`AdoptedFundsVersion`、`AdoptedFunds`，`CollectionFact` 的采用版本（只属真实到账，值语义，`PinnedToAdoptedFunds`），`AdmitAdoptedFundsReference`（币种、每指令至多一条、合计，含溢出保护）、`AdoptedFundsCover`（移位前核名下全部到账）；端口 `AdoptedFundsView`、`CollectionRegistry.LockAdoptedFundsReferences`、`CollectionView.LoadAdoptedFundsReferences`；消费侧适配器 `adapters/settlementaccounting`；编排 `acceptArrival`（重放 → 币种门 → 事实锁 → 锁内再判重放 → 读结算 → 读回名下到账判三条 → 钉版本落册）与 `PostSubledger` 的采用事实门（`admitArrivalMove`，在重放判定与取分户账锁之后、按指令上限之前）；Postgres 写口（没钉版本的真实到账写口拒）、读口、按引用的事务级咨询锁。迁移 `collection_remittance/0004`：版本列、`NOT VALID` 的新登记必有版本、版本只属真实到账、（租户、证据引用、指令）唯一索引（建之前预检存量重复并拒绝施加）。受控 CLI 装配结算只读口，`evidenceRef` 即采用事实引用，不另给版本字段。
- 演示种子：`data/settlement/09`（采用 1400.00 CNY 银行回款）、`10`（整条认作代收本金）、`steps/340-settlement-collected-principal-funds.sh`，`data/collection/06` 的证据引用改引它；README 与 `350-collection.sh` 头注同步。
- 文档：ADR-0330（部分停用/扩成 ADR-0274 决定一、二的三句原句，回答其越权风险点 2、4；另部分停用（改为）ADR-0283 决定四「映到别的目标的映射不取任何一把锁。」一句；0274、0283 只在 Status 注记之后与 Links 加前向指针）；CR CONTEXT、SA CONTEXT、GLOSSARY、CONTEXT-MAP、UC-CR-001（`AT-CR-020` 改写，新立 `AT-CR-026`、`AT-CR-027`，交接表更新）、UC-SA-005（新立 `AT-SA-216`）；AT 盘点把 `AT-CR-020` 由④移入①并补三行新号。

### 代决与越权风险点

下列为本票代决，待 CR owner、SA owner 复核（完整版在 ADR-0330「越权风险点」）：

1. **结算一侧整条认作代收本金，互斥、不核销、不可撤**（SA owner、CR owner）。代决理由见「裁向」。越权点：UC-SA-005 的映射目标集多一格；SA CONTEXT「COD 代收本金不进」从一句话变成结构。认错的事实没有出口（不可撤）；反方向同样——事实被映到了错的目标之后就再也不能认作代收本金，认作了也不能再映别处，两个方向都不可撤；一条回款混着服务费与本金时要在来源侧拆成两条事实。
2. **映射到任何目标都先取事实锁**（SA owner）。此前映到审核应付、贷项、金额本体的映射不取；互斥的读回与插入必须在同一把锁下。锁序未变（事实在全序首位），代价是这几类映射与同一条事实上的核销、更正排队；既有锁序用例随之改写（`TestMappingLocksTheFactThenOnlyTheScopesItMovesInKeyOrder`）。这一条同时部分停用 ADR-0283 决定四的末句，ADR-0330 与 ADR-0283 各加了前向、入向指针。绕过事实锁的第二条认作被库上唯一索引拒，但映射写口 `FundsMappings.Save` 的无目标 `ON CONFLICT DO NOTHING` 会把它吞成「已有记录」，编排读不回赢家，答「映射库不可用」未决：停是对的，文案与原因不符，只在直接写库绕锁时出现，不改行为。
3. **证据引用就是事实引用，登记读此刻链头并钉版本，不让运营给版本**（CR owner）。少一个会填错的输入；判金额永远读链头，版本只是留痕，不参与重放比对。
4. **一条已采用事实对一条指令至多一条真实到账**（CR owner）。比 ADR-0274 的「金额之和不超额」多一条收紧：换事实标识重登同一指令，即使合计没超也答内容冲突。真正的第二笔钱来自另一次回款，引用另一条事实。
5. **登记与移位都读结算，读时对事实身份行取共享锁**（CR、SA owner）。这是跨上下文的锁：代收与清分持自己的锁之后向结算要共享锁，结算从不回头等代收与清分的锁，接不成环；代价是读方事务提交之前结算一侧对这条事实的更正、映射、核销要等。
6. **移位前再核一次：名下全部到账之和不超过此刻链头，容不下答内容冲突、账面不动；冲正不读结算**（CR owner）。回答 ADR-0274 越权风险点 4。被结算更正得容不下的状态没有出口（要结算再更正回去，或后继记录给到账的更正形），已落的移位不自动冲正。
7. **重放先于一切按此刻状态作答的门，含取锁与读结算**（CR owner）。重放一条早已登记的到账，即使事实已被更正小、结算读口没装配、取锁失败，也答已在册。
8. **按旧口径登记的存量真实到账不回填，撑不起移位**（CR owner）。迁移 `NOT VALID` 只管新写入；要为同一笔钱按引用另登一条。演示种子重灌即可。
9. **认作代收本金的事实若被 customs-compliance 的税费付款核对误引**（CC owner）：CC 读到金额、币种、付款人，不知道它是受托资金。评审核实：CC 的适配器 `adopted_funds_fact_source` 按（引用，版本）读事实、不看结算的映射状态，认作代收本金的事实仍可能被 CC 当税费付款证据；这是既有盲区（映到客户账户的事实今天同样可被引用），本票没有制造新的双记口子，也不更易误引。结论：不立票，本票不动 CC。
10. **同一笔银行回款被不同引用各采用一次，代收与清分防不住**（SA owner）。来源身份的去重是采用口的事；唯一的网是按指令的移位上限，而它只拦同一条指令：两条事实的到账指向同一条指令时，第二条能登记、移位撞 `UNDERFUNDED`；指向两条各有渠道报告的不同指令时，两条都能登记、都能移位，`待清分`合计是真实回款的两倍。原始缺陷「代收与清分自铸、结算也采用」已实质关闭，残留的是结算采用口的来源身份去重，不是本票新增。

### AT

`AT-CR-020` 由④移入①并改写（币种、拆分、依据不在册）；新立 `AT-CR-026`（重复到达与换标识重登、取锁后读合计、并发）、`AT-CR-027`（被结算更正与迟到：下调后登记与移位、没有版本的存量、读不动答未决）、`AT-SA-216`（整条认作代收本金）。总览计数未动，随进 main 重算（`go run ./tools/at-coverage-counts -check` 退 1，只是总览过期；退 2 的缺行已补）。
### 验证

- 先写失败用例：结算与代收两侧的领域、编排、真库、受控 CLI 新用例都先于实现写成，实现前编译不过（无 `TargetCollectedPrincipal`、`AdoptedFunds`、`Deps.AdoptedFunds` 等符号），实现后全绿；变异暴露出的两处空白（锁前重放判定、锁内重放判定）和真库并发用例的不确定（见下）补用例后红绿各一次。
- `go build ./... && go vet ./...`、`gofmt -l`：干净。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh <结算与代收两侧全部包及其反向依赖：cmd/parcel-api、cmd/parcel-dispatch、cmd/parcel-collection-register、cmd/parcel-settlement-register、customs-compliance / parcel-pricing / parcel-shipment 三个 settlementaccounting 适配器> ./internal/architecture/... ./migrations/... ./internal/platform/... ./tools/...`：全 ok（rebase 到当时的 origin/main 之后跑）。`./internal/architecture/...` 覆盖事务外拒跑门禁（新写口 `LockAdoptedFundsReferences`、`LockCollectedPrincipalFact` 各补负向证据）与生产接线棘轮（新构造器都在 `parcel-collection-register` 装配）。
- 新用例 `-count=3` ok；并发用例（事实上唯一去处、读口共享锁与更正互斥、事实锁排队、六路与两路并发登记、同一份并发到达）`-count=5` ok。
- 管理台：`tsc -b --noEmit`、`node scripts/run-tests.mjs`（803 项）、`pnpm exec vite build` 通过，软链与产物已删。
- 一次性库灌演示种子（含迁移 SA 0048、CR 0004）：全程零报错；库里 `SYN-COD-FACT-02` 的证据引用是 `SYN-COD-FUNDS-BANK-01`、钉的版本 `v1`，结算一侧同引用的映射目标是 `COLLECTED_PRINCIPAL`；账内各位置与原来相同。原样重放 `external-funds-fact`、`funds-mapping`、`fact`、`posting` 全答已存在 / 已在册（退出码 0）；到账证据引用改成任意字符串答 `BASIS_MISSING`（2），同一指令换事实标识再登答 `CONTENT_CONFLICT`（2），认作之后再映到客户账户答 `MAPPING_CONFLICT`（2）；用后已删库。
- 变异（每次改一处、跑用例、从备份还原并 `cmp` 一致），全部被杀：(1) 去掉每指令至多一条；(2) 合计上限放宽一分（账平衡：多收的一分没有来源）；(3) 去掉移位前的采用事实门（`待清分` 多出银行没交付的本金，账面红）；(4) 去掉锁前重放判定——**先存活**（锁后那次足够答已在册），补 `TestAReplayIsAnsweredFromTheBookEvenWhenTheDependenciesAreDown`（取锁失败、读口未装配时重放仍答已在册）后被杀；(5) 登记不取事实锁——内存用例红，但真库六路并发用例**没红**（连接池只有两条，竞争窗口太窄），补屏障用例 `TestTwoArrivalsThatCannotBothFitAreQueuedOnTheFundsLock`（先占住咨询锁、看两路都等在上面再放）后被杀；(6) 去掉锁内重放判定——**先存活**，补 `TestASameArrivalCommittedWhileWaitingForTheLockIsAnsweredExisting`（等锁期间同一份已登记、事实又被更正小了）后被杀；(7) 结算互斥只拦第二条认作；(8) 结算映射只有映到对账单或账户才取事实锁（真库并发 ×5 红）；(9) 读口不取共享锁（更正落进读方事务之内）；(10) CR 迁移去掉（证据，指令）唯一索引；(11) 登记不钉版本；(12) 去掉到账币种对账币种的门；(13) 结算允许核销到代收本金；(14) 结算资金退回也能认作。
- 过程记录：读口共享锁的用例一开始因用例池只有两条连接、旁观查询要不到连接而挂住；旁观改用 `pgtest.AdminDSN` 单开的连接，并在断言失败提前退出时释放持锁方（否则收尾关池也挂）。

### 未决

- 认作代收本金没有撤销形；认错的事实在结算一侧没有出口（ADR-0330 越权风险点 1）。
- 被结算更正得容不下的状态没有出口，等结算再更正或到账的更正形（code/15，ADR-0330 越权风险点 4）。
- ADR-0298 越权风险点 1（认可抵扣登记时不核结算）不在本票：认可抵扣不是资金事实，没有认作可言，SA 给出认可的只读口之后另立记录。

### 评审记录与修订轮

独立评审判**不可合入**：一条阻断 B，另有六条 N。评审核实无误、本轮不重做的部分：四个攻击序列、锁序、手算、迁移、CC 盲区判定。

- **B（阻断）ADR-0283 决定四原句被实质改写，缺部分停用指针。**ADR-0330 决定一与越权风险点 9 都承认映射到任何目标现在先取事实锁，代码 `MapExternalFundsHandler.Map` 也确实一律先取，而 ADR-0283 决定四写的是「映到别的目标的映射不取任何一把锁。」。补三处，写法照 ADR-0298 停 ADR-0082：ADR-0330 Status 前段加「部分停用（改为）」及被停原句；ADR-0330「对已接受记录的作用」同样列这一句并写适用场景，去掉与之矛盾的「ADR-0283 照采」，改成「除决定四那一句外的各条照采」；ADR-0283 的 Status 注记（`<!-- 索引注记 -->` 之后）加「部分停用（2026-10-27）」与适用场景，Links 加 ADR-0330，正文不动。核对：被停原句与 ADR-0283 原文 `grep -F` 逐字相同、决定号属实；ADR-0274 的三句原句仍与其原文逐字相同；ADR-0330 里其余指针都在；`git diff origin/main...HEAD -- docs/adr/0283-*` 只有 Status 注记与 Links。
- **N1 `CollectedPrincipalFundsView` 把 SetAside 判成「该事实有任何映射」无用例红。**加 SA 真库用例 `TestTheCollectedPrincipalViewDoesNotCallAMappedFactSetAside`（事实映到对账单、账户、审核应付后 SetAside 为假）；CR 受控 CLI 纵向用例 `TestAnArrivalIsRegisteredAndMovedOnlyWhileTheAdoptedPrincipalFactCoversIt` 的结算一侧事实改为经真 SA 的采用、更正与映射编排种下（`settlementFunds`，装配同 `parcel-settlement-register` 的 `buildRegistrar`），并加「到账引用一条已映到账户的事实 → `BASIS_MISSING`、之后它也认作不了代收本金 → 映射冲突」。变异：SA 查认作状态的 SQL 里 `target_kind = $3` 换成恒真——SA 用例三格全红，CR 纵向用例红（该到账答 REGISTERED，要 BASIS_MISSING）。
- **N2 读口先读链头、后取共享锁的变异存活。**评审写的用例放进 SA `adapters/postgres` 包，改名 `TestTheCollectedPrincipalViewReadsTheNewHeadOfACorrectionItWaitedFor` 并补 `Covers:` 与失败时放更正方的 `defer`：更正握着未提交、读方在共享锁上等、更正提交之后读方必须读到新链头。变异：读口在取共享锁之前先读链头并用这次的结果——该用例红（读到 100000，要 60000）；`-count=5` 稳定。
- **N3 SA 迁移 0048 的两道库面兜底都没有用例。**加真库用例 `TestTheMappingTableItselfRefusesASecondRecognitionAndATargetThatIsNotTheFact`：绕过编排直接 INSERT，第二条认作被唯一索引 `funds_mapping_one_collected_principal_per_fact` 拒（23505），目标不是事实自己被 CHECK `funds_mapping_collected_principal_names_the_fact` 拒（23514），各断言约束名；唯一索引不罩别的目标种类。变异：去掉唯一索引——红；去掉 CHECK——红。0048 头注原写「库上只封住词表」与后面的 CHECK、唯一索引矛盾，改为库上封三件。`FundsMappings.Save` 的无目标 `ON CONFLICT DO NOTHING` 吞掉绕锁的第二条认作、答「映射库不可用」未决，已在 ADR-0330 越权风险点 9 与本票越权风险点 2 各补一句，不改行为。
- **N4 移位门里「没有钉版本」那一分支没被单独证明。**加 `TestMovingAnUnpinnedArrivalWhoseEvidenceIsAnAdoptedPrincipalFactIsStillBasisMissing`：存量未钉版本的到账，证据引用恰好是一条已采用、已认作、容得下它的事实，移位答依据不在册、账面不动、不碰结算。变异：`!pinned` 恒假——新用例红（答 REGISTERED），原有用例仍绿，证明这一格先前确实无人守。
- **N5 操作员看到的文案不实。**事实被更正得容不下既有到账后，移位和登记答的 `CONTENT_CONFLICT` 并没有同键记录。受控 CLI 对内容冲突的文案补一句：真实到账还可能是其引用的已采用事实被更正得容不下名下到账之和，或同一指令已有一条引用同一事实的到账，此时没有同键记录，核对事实此刻的链头金额与已引用它的各条到账；纵向用例对移位与登记各断言这句文案。变异：去掉这句——纵向用例红。
- **N6 规范小项。**改掉本分支新增行里的变更叙述与位置词：UC-SA-005「下面的外部更正」、`funds_application_test.go` 的「依据照旧必填」「词表外的名字照旧拒」、`domain/adopted_funds_test.go` 的「链头不低于合计则照旧」「不再被事实撑住」、迁移 0004 的「照旧存引用本身」「下面的「新登记必有版本」」（改为 CHECK 名 `collection_fact_arrival_pins_funds_version`）、迁移 0048 的「照旧满足」、`register_arrival.go`「不再能移位」、`domain/adopted_funds.go`「不再自铸」、几处「不再接受」、演示种子两步注释里的「下一节」「前一节」、`Deps` 头注的「前三对」；代码、测试、SQL、脚本与 UC、CONTEXT 的新增行再 grep 无剩余；ADR-0330 里讲决策来由的「此前」「今天」属决策记录的叙述，保留。
- **信息项。**ADR-0330 越权风险点 1 补另一个方向（事实被映到错的目标之后再也不能认作代收本金，认作了也不能再映别处，两个方向都不可撤）；风险点 3 补明具体后果（同一指令：第二条到账能登记、移位撞 `UNDERFUNDED`；两条各有渠道报告的不同指令：两条都能移位，待清分合计 2×；原始缺陷已实质关闭，残留是结算采用口的来源身份去重）；风险点 10 写明 CC 结论（`adopted_funds_fact_source` 不看映射状态，既有盲区，本记录没制造新口子，不立票）。

## 进 main 记录（2026-10-27，协调方）

- 分支 `claude-code54` 经一轮评审与一轮修订，rebase 到 origin/main 后快进，作者 tip `5a3dd4263`。迁移 `settlement_accounting/0048_funds_mapping_collected_principal_target`（外部资金映射新增目标 `代收本金`，唯一索引与 CHECK 各一道库面兜底）、`collection_remittance/0004_arrival_references_an_adopted_funds_fact`（到账钉版本，`NOT VALID` 的新登记必有版本 CHECK，唯一索引前预检存量重复）；新立 ADR-0330（部分停用（扩成）ADR-0274 决定一、二的三句，**另部分停用（改为）ADR-0283 决定四「映到别的目标的映射不取任何一把锁。」**——评审阻断项：原稿只写「照采」，实际映射到任何目标都先取事实锁；入向指针只在被停 ADR 的 Status 注记与 Links，被停原句逐字相同）。AT-CR-020 改写并由④移入①，新立 AT-CR-026、027、AT-SA-216。演示种子 `340-settlement-collected-principal-funds.sh`、管理台目标词表一格。
- 做法：SA 一侧新增映射目标 `代收本金`——整条认作，目标就是这条事实自己，不按金额分、不核销、不进任何客户应收作用域的入账余额、不可撤；与这条事实上任何别的映射互斥（含第二条认作），冲突答既有 `MAPPING_CONFLICT`，互斥在事实锁下读回再判，库上唯一索引兜底；映射到任何目标都先取事实锁。CR 一侧：真实到账的证据引用就是这条事实的引用，没采用、采用了没认作、别租户同引用一律答「依据不在册」；登记时经 SA 只读口读此刻链头并对事实身份行取共享锁，读到的版本钉在到账上；三条判据缺一不登（币种同时等于账币种与事实币种、引用同一条事实的各条到账之和不超过链头金额、一条事实对一条指令至多一条到账）；重放先于一切按此刻状态作答的门；移位前再核名下全部到账之和仍不超链头；冲正不读结算；按旧口径登记的真实到账不回填。
- 评审：不可合入——阻断为 ADR-0283 决定四缺停用指针；评审设计的四个攻击序列（先核销再认作、先认作再核销或映射、同一笔钱用不同引用采用两次、SA 更正把链头改小到容不下既有到账）都按设计，锁序独立画出两边取锁序列不成环、并发压测 4 轮无死锁，原始缺陷「CR 自铸、SA 也采用」实质关闭。六条建议（`SetAside` 判成「有任何映射」的变异存活，会让已映到客户账户的事实复活双记缺陷；读口先读链头后取共享锁的变异存活；迁移 0048 两道库面兜底没有用例；移位门里未钉版本分支没单独证明；操作员看到的冲突文案不实；规范小项）由 Sonnet 修订代理补齐，四处变异均红。
- 协调方单独提交重算的 AT 计数、重生成的 ADR 索引与机制清点；检查均退 0。全量 `scripts/test-full.sh` 退 0、160 个包 ok；`go build ./...`、`go vet ./...` 干净；种子灌一次性库退 0；管理台 `tsc -b --noEmit`、`node scripts/run-tests.mjs`（803 过）、`vite build` 与 `./internal/architecture/...` 全过。
- 去向：无新票。评审的两条不挡上线的发现记入冻结文件「记录」：CC 的税费付款核对不看事实类型与映射状态（既有盲区）；SA 把同一笔回款用不同引用采用两次并各认作时代收与清分防不住（待清分合计 2× 的具体后果，残留是 SA 采用口的来源身份去重，不是本票新增）。越权风险点见 ADR-0330：认作后没有出口，映错目标与认错都不可撤，两个方向；整条认作，一条回款混着服务费与本金时要在来源侧拆成两条事实；登记与移位跨上下文取共享锁，读方事务提交前结算对这条事实的更正、映射、核销要等；被结算更正得容不下既有到账的状态没有读面能看见，要结算再更正回去，到账没有更正形。
