# 47 CR 协议抵扣没有记账形：认可的抵扣额只能留在`渠道在途`

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code47。R2（上线后尽快）：COD 渠道按有效协议从回款里扣下的金额（例如服务费抵扣），经 settlement-accounting 认可后，在代收分户账上没有任何记账能把它了结，只能留在`渠道在途`。2026-10-21 文档自洽 09 评审轮立（用户「你作为业务和系统专家自决」授权），分支 claude-dc09。记账形由作者代决，立 [ADR-0298](../../../docs/adr/0298-an-agreed-offset-approved-by-settlement-accounting-leaves-in-transit-for-an-offset-applied-position.md)。
Blocked by: 无——[14](./14-collection-principal-posts-more-than-once-and-shortfall-moves-received-funds.md) 已 resolved（2026-10-21 进 main）；抵扣记账形用户已授权作者代决，裁在 ADR-0298。
出处：文档自洽 09 第 2 轮评审（协调方裁定：短款公式扣除经 settlement-accounting 认可的协议抵扣，抵扣的记账形未裁、另立本票）；与 [SCENARIOS](../../../docs/domain/SCENARIOS.md) 场景十七（报告 100、到账 96、认可抵扣 3、短款 1）对照。

## 现象

- collection-remittance CONTEXT「清分、汇付与差异」写：代收本金不得与 COD 服务费、运费或其他运营应收在本上下文内相互净额，抵扣只能依据 `settlement-accounting` 拥有的、有效协议允许的抵扣金额，通过明确记账依据表达。
- 代码没有这一种依据：`domain.PostingBasisKind` 只有 `COLLECTION_FACT`、`ALLOCATION`、`REMITTANCE_BATCH`、`DISCREPANCY`、`CORRECTION`，`domain.FundPosition` 也没有抵扣那一格；认可的抵扣额除了留在`渠道在途`，没有地方可去。
- 照 ADR-0271 的短款公式，认可抵扣从短款里扣除、不落`短款`；场景十七里那 3 因此留在`渠道在途`。

## 证据

- `internal/collectionremittance/domain/subledger.go`：`PostingBasisKind` 只有 `COLLECTION_FACT`、`ALLOCATION`、`REMITTANCE_BATCH`、`DISCREPANCY`、`CORRECTION`，没有抵扣依据；`FundPosition` 只有 `EXTERNAL_SOURCE` 与账内的渠道在途、待清分、应付客户、已汇付、短款、溢款，没有抵扣位置。
- `migrations/collection_remittance/0001_collection_subledger_and_remittance.sql`：`subledger_posting` 的 `from_position`、`to_position`、`basis_kind` 三列各有 CHECK 封住同一份词表，再加依据门的 CHECK；新依据或新位置要另取迁移号放开。
- 文档：collection-remittance CONTEXT「清分、汇付与差异」里抵扣那一条与「给开发的交接」；ADR-0271 决定四与越权风险点 6。

## 三类

机制：抵扣的记账依据与去向（待 CR owner 定形）。产品策略：无。租户取值：哪些抵扣有效、金额多少，由 settlement-accounting 的协议与认可给出，CR 不预填。

## 规模

M（定形之后）：新依据种类（指向 settlement-accounting 的认可引用），新去向或出账形，迁移 CHECK、领域构造门与编排；CLI 与种子各补一份。

## 最小的诚实修法

形定下来之前代码不动：认可的抵扣额留在`渠道在途`。汇付与对账时这部分余额要看得出是未了结的认可抵扣，不能当作渠道还欠的本金去追收——读面与对账口径按 settlement-accounting 认可的抵扣额把它从「渠道欠款」里分出来列示；形定之后照定下的依据与去向补记账，存量按认可记录补记。

## 要裁的

- 抵扣落到哪里：另立一个账内位置（例如「已抵扣」，账内终局，守恒照旧），还是允许一种出账记账（与 ADR-0082 决定四「去向侧没有账外位置」相抵，要另立记录）。
- 依据是什么：settlement-accounting 认可的抵扣引用经什么口交到 CR，CR 是否核金额上限。

## 裁向（2026-10-26，作者代决，[ADR-0298](../../../docs/adr/0298-an-agreed-offset-approved-by-settlement-accounting-leaves-in-transit-for-an-offset-applied-position.md)）

- 抵扣落到账内新的终局位置`已抵扣`（`OFFSET_APPLIED`），不开出账记账，ADR-0082 决定四「去向侧没有账外位置」不变，守恒照旧。
- 依据是新种类`AGREED_OFFSET`，指向 CR 单独登记的「认可的协议抵扣」记录（CR 侧标识、指令、金额、settlement-accounting 的认可引用、认可时刻）；登记只钉引用、不动账，落账是另一笔凭它的记账。CR 登记时不读 settlement-accounting（它没有只读口），核账上有没有这么多可扣，不核抵扣额合不合协议。
- 短款公式的「− 认可抵扣」并入「移出」：抵扣落账之后已在移出里，短款上限由「不超过」变成落账后的在途余额。
- 同一抵扣重复到达、被更正、被撤回：登记按（认可引用，指令）唯一、落账一条认可至多一笔、更正走新认可新记录、撤回冲正已落的那一笔；一笔记账至多被冲正一次（全部冲正适用）。

## 完成记录（2026-10-26，分支 claude-code47，基于 main `a5544faff`，收尾 rebase 到当时的 origin/main）

### 做了什么

- 领域（`internal/collectionremittance/domain`）：新增 `AgreedOffset`（`RegisterAgreedOffset`）、`AgreedOffsetID`、`ApprovalReference`；`FundPosition` 加 `PositionOffsetApplied`，`PostingBasisKind` 加 `BasisAgreedOffset`；`RecordSubledgerPosting` 加三道门（进入`已抵扣`必须凭认可抵扣且只自`渠道在途`、认可抵扣依据只支撑这一种形、离开`已抵扣`只有冲正）；`InstructionPrincipal` 带出已抵扣与「认可抵扣已支撑的记账」，抵扣等到账先移位，`ShortfallCeiling` 头注改为准确值。
- 编排：`RegisterAgreedOffset`（指令在册、币种同账、`(认可引用, 指令)` 撞键折成内容冲突）；`PostSubledger` 加认可抵扣依据（形状、一条认可至多一笔）、冲正判重（`LoadCorrectionOf`，余额守卫之前）、取锁之后再判一次重放、`instructionBehind` 认得抵扣依据。结果代数不增格。
- 端口与适配器：`CollectionRegistry.RegisterAgreedOffset`、`CollectionView.LoadAgreedOffset`、`SubledgerView.LoadCorrectionOf`；Postgres 写口、读口，`LoadInstructionPostings` 的递归追溯加认可抵扣一支。
- 迁移 `migrations/collection_remittance/0003_agreed_offset_has_its_own_position_and_basis.sql`：`agreed_offset` 表（外键到指令，`(tenant, approval_ref, instruction_ref)` 唯一）；放开位置与依据词表；三条 CHECK；部分唯一索引「一条认可至多一笔记账」「一笔记账至多被冲正一次」。只放开、加约束，不改已落行。
- 读面：目录上列、HTTP `/collection-subledgers`、管理台分户账页各带出`已抵扣`。受控 CLI 新增 `agreed-offset` 命令。演示种子补一条认可抵扣（14、15 号文件，6000），短款改为余下的 4000，账内合计仍是入账 150000。
- 文档：ADR-0298；ADR-0271、ADR-0082 只在 Status 注记与 Links 加前向指针；CR CONTEXT（资金位置、认可的协议抵扣词条、规则、生命周期）、GLOSSARY、CONTEXT-MAP、结算 CONTEXT 的位置清单、UC-CR-001（输入、流程、结果、验收 `AT-CR-024`、`AT-CR-025`、交接）、种子 README 与 `350-collection.sh` 头注；AT 盘点补行。

### 代决与越权风险点

下列为本票代决，待 CR owner、SA owner 复核（完整版在 ADR-0298「越权风险点」）：

1. **落到账内终局位置`已抵扣`，不开出账记账**（CR owner）。理由：ADR-0082 决定四的守恒在一条求和上核得完；出账要第二个口径。越权点：词表多一格、依据门多一条，这是 ADR-0082 决定四、五的扩成。
2. **认可抵扣单独成册，既不是代收事实也不是差异事项，登记与落账分两步**（CR owner）。理由：键和按指令的上限要由依据推出；差异事项是待处置异常，抵扣是已了结的正常扣减。
3. **登记时 CR 不读 settlement-accounting，认可引用与金额是操作员录入的**（SA、CR owner）。SA 今天没有认可抵扣的只读口。录错金额会让渠道欠款少记；SA 有只读口后照 ADR-0274 办法核，另立记录。
4. **到账先于抵扣，抵扣先于短款，靠余额不足挡，不给短款加闸**（CR owner）。短款先落会吃掉在途款，抵扣停在余额不足，要先冲正短款。没落账的认可没有撤回形（记录一直在册，引用不再可登给同一指令）。
5. **抵扣全额一次，不部分落账；大于在途时整笔停住等补报告**（CR owner）。
6. **一笔记账至多被冲正一次，收紧了全部冲正**（CR owner）。此前同一笔可被重复冲正，同账别的指令在同位置有钱时会被多拿；这是缺口，不是规则，改后重复冲正答内容冲突。迁移 0003 的唯一索引在存量里有重复冲正时施加失败、整笔回滚。
7. **取锁之后再判一次重放，对全部记账生效**（CR owner）。同一命令并发到达时后到的一路答已在册，不再答来源被移空的余额不足；这是改了一处已有的答案。
8. **SA 一侧怎样消费`已抵扣`（核销 COD 服务费应收？）没有裁**（SA owner）。在 SA 定之前它只是 CR 一侧的事实，对得上靠认可引用逐笔对。
9. **已抵扣的钱客户拿不到**（CR owner）。协议归 settlement-accounting，CR 不核协议是否与该客户签过。

### AT

`AT-CR-024`（登记认可的协议抵扣与落账形状）、`AT-CR-025`（认可抵扣落账的上限、次序、判重、撤回与并发）新立入①；`AT-CR-017` 抵扣那一半补点名整条场景用例；`AT-CR-018` 自②移入①。总览计数未动，随进 main 重算（`go run ./tools/at-coverage-counts -check` 退 1，只是总览过期）。

### 验证

- 先写失败用例：领域、编排、真库、受控 CLI 的新用例先于实现写成，实现前编译不过（无 `AgreedOffset` 等符号），实现后全绿。
- `go build ./...`、`go vet ./...`、`gofmt -l`：干净。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/collectionremittance/... ./cmd/parcel-collection-register/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./internal/architecture/... ./migrations/... ./internal/platform/... ./tools/...`：全 ok（反向依赖只有 `cmd/parcel-api` 与 `cmd/parcel-collection-register`）。
- 管理台：`tsc -b --noEmit`、`node scripts/run-tests.mjs`（795 项）、`pnpm exec vite build` 通过，软链与产物已删；`./internal/architecture/...`（含管理台请求路径与端点表的核对）在上一条里通过。
- 新用例 `-count=3` ok；并发用例 `-count=5` ok。
- 变异（每次改一处、跑用例、从备份还原并 `cmp` 一致），全部被杀：(1) 指令名下`渠道在途`不随抵扣落账减少；(2) 分户账余额派生漏记`已抵扣`的去向（账平衡）；(3) 去掉一条认可至多一笔的判重；(4) 去掉一笔至多冲正一次的判重；(5) 去掉分户账事务锁（三条并发用例红）；(6) 去掉取锁后再判重放（同一命令并发退出码 [4 0]）；(7) 去掉到账先于抵扣的闸；(8) 迁移里去掉冲正唯一索引；(9) 迁移里去掉认可抵扣唯一索引；(10) 去掉离开`已抵扣`只有冲正的域门；(11) 去掉币种核；(12) 抵扣金额不再要求等于登记额；(13) 目录上列 SQL 漏`已抵扣`；(14) HTTP 读面漏字段。
- 一次性库 `code47seed` 灌演示种子（含迁移 0003）：全程零报错，CR 一节含 `agreed-offset: REGISTERED`；原样重放 14、15、08、09 全答 `EXISTING`（退出码 0），换记账标识重落抵扣答 `CONTENT_CONFLICT`（2）；账内合计 150000（已抵扣 6000、短款 4000、应付客户 140000）；用完已删库。
- 过程记录：变异里去掉迁移索引那几次，真库用例曾因 `t.Fatalf` 落在事务闭包内而挂住到测试超时；已把新真库用例的事务闭包内断言改成返回错误。既有同类写法（`instruction_principal_test.go`）未动。

### 未决

- 登记时核 settlement-accounting 的认可（引用在册、币种、同一认可下各指令之和不超认可金额）：等 SA 的只读口，另立记录。
- SA 一侧对`已抵扣`的消费：SA owner 定。
