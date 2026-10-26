# 12 SA 资金映射没有「客户赔付义务」「追偿应收」两种目标

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code12（见文末「完成记录」，与 [code/46](./46-settlement-application-and-reversal-have-no-process-entry.md) 同批）。原 ready-for-agent——R2（上线后尽快）：资金映射与索赔金额结算都在生产装配上（`cmd/parcel-api`）；首批赔付付款或追偿到账一来，就映射不到它该核销的义务上，只能停在未分配。看得见的卡住，不答错金额；绕法是先留未分配、线下对账。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/sa-cr.md](../audit/sa-cr.md) M9（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

CONTEXT、UC-SA-005、UC-SA-007 要求客户赔付的真实付款、供应商或保险追偿的真实到账都由 UC-SA-005 形成收付映射与核销；代码的映射目标只有对账单、应付、贷项通知、应收结算账户四种，没有赔付义务与追偿应收。代码注释还把 `AT-SA-116` 读反了。

## 证据

- 文档：
  - SA CONTEXT Lifecycles「客户赔付与外部追偿」：「财务、银行或支付系统确认客户赔付付款或追偿收款后，只有 `UC-SA-005` 可以形成真实收付映射以及未分配、部分或完整核销」。
  - UC-SA-005 目标与边界：「将客户赔付的真实付款与运营企业对客户的应付/贷记/退款义务关联，将供应商或保险追偿的真实到账与运营企业应收关联」；`AT-SA-116`、`AT-SA-117`；UC-SA-007 `AT-SA-154`、`AT-SA-155`。
- 代码：
  - `internal/settlementaccounting/domain/external_funds.go` 的 `SettlementTargetKind`：`STATEMENT` / `PAYABLE` / `CREDIT_NOTE` / `ACCOUNT`。
  - 同处注释「费用、赔付金额与追偿认可本体不在其中——映射与核销改不了那些金额（AT-SA-116/121）」。`AT-SA-116` 要的是「分别形成对应应付的付款映射和核销；不改写……赔付金额」：映射到它，不改它的金额；注释把「不改金额」读成了「不映射」。
  - ADR-0182 决定一把 `ACCOUNT` 限在应收结算账户上，赔付与追偿借不了这一格。
  - 赔付与追偿金额本体在 `domain/claim_recovery_amount.go`，由 `application/settle_claim_amounts.go` 形成。

## 三类

机制。无产品策略；哪个账户收付由租户的结算账户登记给出，未登记答未配置。

## 规模

M：映射目标加两格（赔付义务、追偿应收），核销余额按目标各自算；迁移 CHECK 放开；修正那段注释。

## 最小的诚实修法

`SettlementTargetKind` 加 `CUSTOMER_COMPENSATION`（指向已认可的客户赔付金额）与 `RECOVERY_RECEIVABLE`（指向已认可的追偿金额）；映射与核销只改核销状态与未分配余额，不改被指向的金额本体（这才是 `AT-SA-116` 那句的意思）。

## 要裁的

- 两种目标指向的是金额本体的哪一个版本（认可后又更正时，按映射时的那一版还是当前版）。推荐按映射时的版本并回指，作者可代决。

## 完成记录（2026-10-25，作者会话，分支 claude-code12，变基到 main `061156bcc`）

与 [code/46](./46-settlement-application-and-reversal-have-no-process-entry.md) 同批（同一只编排 `MapExternalFundsHandler`）。证据只记 `S`（隔离合成）。

### 落了什么

1. **两格目标**：`domain.SettlementTargetKind` 加 `TargetCustomerCompensation`（`CUSTOMER_COMPENSATION`）与 `TargetRecoveryAcknowledgement`（`RECOVERY_ACKNOWLEDGEMENT`；修订轮由 `RECOVERY_RECEIVABLE` 改名，见文末「评审记录与修订轮」N1），序号接在 `TargetAccount` 之后，既有映射与核销的载荷摘要不变；`PointsAtClaimAmount` 认这两格。头注把 `AT-SA-116` 读回原意：映射到金额本体、核销它，不改它；`AT-SA-121` 禁的是形成或修改金额。
2. **映射**：`Map` 在落行之前经新口 `ports.ClaimSettlementTargets.FindClaimTarget` 核那一条在本租户的金额册上；不在答 `SOURCE_NOT_ACCEPTED`，册读不回答未决 `CLAIM_TARGET_STORE_UNAVAILABLE`（`FundsUndecidedReason` 新增一格，封闭集用例改为四格）。
3. **核销**：`Apply` 过了领域门、落行之前，按 `SettlementApplication.ClaimTargetDemands`（按格、引用排序）逐条 `LockClaimTarget`——先 `FOR UPDATE` 锁金额本体那一行，再另起一句求指向它、未撤销的核销分配净额，交回 `domain.ClaimTargetStanding`；`Admit` 判币种（不同答 `CROSS_CURRENCY`）与未核销（超出答 `APPLICATION_IMBALANCE`），什么都不落。锁与落行同一事务。指向两格的分配只收借向（`claimAllocationDirected`，形成门与读回门都守）。
4. **金额本体只读**：适配器 `sapostgres.ClaimSettlementTargets` 只有两个读口，不写 `customer_claim_amount`、`recovery_acknowledgement`；映射与核销改的只是 `funds_mapping`、`settlement_application` 两表与事实的累计未分配（修订轮改准：`RemainderMinor` 只扣本笔，事实的累计未分配是当前有效那一版金额减去该事实全部未撤销核销，见「评审记录与修订轮」B1）。
5. **库**：迁移 `migrations/settlement_accounting/0046_funds_mapping_claim_amount_targets.sql` 照 0036 同形重建 `funds_mapping_target_kind_closed`，六值；存量行照旧满足（演示种子那一行 `ACCOUNT` 在一次性库上实测共存）。postgres 读回与批文两张词表各补两格。
6. **装配**：`cmd/parcel-settlement-register/main.go` 的 `buildRegistrar` 与 `cmd/parcel-api/assemble_settlement_registration.go` 各接一只真适配器（`NewMapExternalFundsHandler` 构造门拒 nil，不接就起不来）。未动 `cmd/parcel-api` 的 endpoints.go、main.go、endpoints_test.go、routes.go。
7. **管理台**：`apps/admin-web` 收付款核销页的目标词表补 `ACCOUNT` 与两格新值，改正同一句「赔付金额与追偿认可本体不在其中」的误读（三道门退 0，601 例全过）。
8. **文档**：SA CONTEXT Lifecycles「客户赔付与外部追偿」加一条规则句（两格目标、映射时那一版、按目标求未核销）；UC-SA-005 状态行与交接表删本票一行；UC-SA-007 交接表只剩本票一行，整节删去（照无缺口 UC 不设该节的写法）；`.scratch/at-coverage-inventory.md` 只移行、加名字（见「测试」）。

### 代决

1. **版本：按映射时那一版并回指。** 两本金额册只增不改，映射指名的那一条（赔付 `amount_id`、认可 `acknowledgement_id`）就是映射时那一版，目标引用本身即回指，映射行不另加版本列。责任、金额规则或认可范围更正时追加的调整（`claim_amount_adjustment`）或另立的认可各有身份，不改已有映射，也不并进它的未核销。
2. **核销余额按目标各自算**：未核销 = 那一条自己的金额（赔付取 `amount_minor`，认可取 `acknowledged_minor`）− 指向它、未撤销的核销分配净额（借加贷减）。不跨目标、不跨同一索赔项或同一应追偿下的别的条目合并，追加调整不算进来。超出即分配失衡、整笔不落，超额保持未分配（`AT-SA-109`）。
3. **`CUSTOMER_COMPENSATION` 收客户方向索赔金额的两种**（赔付义务、索赔费用退款）：UC-SA-005 目标「运营企业对客户的应付/贷记/退款义务」与输入表「赔付应付/费用贷记或退款义务」都把退款算在内。
4. **`RECOVERY_ACKNOWLEDGEMENT` 只指追偿认可金额，不指应追偿金额**（票面「指向已认可的追偿金额」）：对方认可之前到账的追偿收款停在未分配，直到 UC-SA-007 凭 VE 判定有效的对方响应形成认可。
5. **存在性与租户一致性**：按（租户、引用）在那一格对应的册上取；他租户的与不存在的同答 `SOURCE_NOT_ACCEPTED`（ADR-0029：两者恢复动作同是改目标，分开答就是存在性预言机）；格与册对不上（拿认可号当赔付映射）同不存在。这是结果格不是 error，不进 CLI 的未决哨兵表；只有册读不回才是未决。
6. **结算账户一致性**：资金事实不带责任法人与结算账户维（只有来源、付款人、币种、金额），两格目标的法人与账户一致性由映射依据（付款指示）承担，产品不核——与审核应付、贷项两格同一处境。能核的币种在核销时以那一条金额本体的币种为准，不以命令自报的 `targetCurrency` 为准。
7. **只收借向**：贷向是拿赔付或认可去抵客户应收、供应商应付，UC-SA-005「默认不做应收应付净额抵销」；本票不做抵销依据的规则，贷向一律未受理。
8. **不进入账余额**：赔付付款与追偿收款不是客户应收作用域上的收付，`CustomerDirectionNets` 的 P 只加总映射到对账单与账户两格上的收付、W 只加总指向对账单的分配，映射与核销这两格不重算运营余额与信用状况（修订轮改准）。
9. **锁**：行锁落在金额本体那一行，锁序按（格、引用），在事实之后、运营余额作用域之前（全序见「评审记录与修订轮」）；锁与求和分两句——并成一句时等锁的那一路读的是锁前快照（变异实测读到已核销 0，见「测试」）。
10. **入账余额按事实去重是既有缺陷，本批顺修**（第二轮评审 N4，协调方裁本批修、不立票）。
    - 缺陷：`CustomerReceiptPostings.CustomerDirectionNets` 的 P 原按 `funds_mapping` 行求和。一条事实映到同一作用域的两张对账单、或兼映到账户，P 记两次；10000 映到同账户下 stmt-1、stmt-2 得入账余额 20000，可用余额多出 10000。碰钱且静默：预付冻结读可用余额，比例额度读入账余额。
    - 为什么本批修：缺陷早于本批，但只有一对多核销才会把一笔收款映到同一作用域的几个目标（`AT-SA-106`）。核销入口是本批开的，不修就是本批让它走得到。
    - 修法：对账单段与账户段先 `UNION` 成一个事实集，再取链头金额求和。

### 越权风险点（待 SA owner 复核）

1. **追偿到账先于认可**：保险方不回应直接赔款的那一格，钱停在未分配，要 VE 先登对方响应、UC-SA-007 形成认可才映射得上。若 owner 认为应追偿金额也该是核销目标，加一格或放宽这一格即可。
2. **同一应追偿下先后两条认可各算各的未核销**：旧一条已核销的部分不并进新一条，读单格会把新一条看成整额未结；产品今天没有认可之间的替代关系可读。
3. **追加调整不进未核销**：赔付减额调整之后按原额付款，那一条显示部分核销、留下已不欠的未结；增额调整那一部分的付款没有目标可映射，停在未分配。
4. **净额抵销一律不收**（代决 7）：租户合同明确允许以赔付抵应收时，这一格今天答未受理。
5. **法人与结算账户一致性不核**（代决 6）：把甲法人的赔付付款映到乙法人的赔付上，产品不拦，只认付款指示。
6. **索赔费用退款也收进 `CUSTOMER_COMPENSATION`**（代决 3）：若它已经对账单贷记给客户，再映射一笔现金付款就付了两遍；映射要显式依据，产品不交叉核对账单纳入。
7. **未核销面的求和扫该租户全部未撤销核销的 jsonb 分配**，没有索引；与 `CustomerDirectionNets` 同形，量大时要加。
8. **外部更正把事实金额改到已核销合计以下**（第二轮评审 N3）：更正照采，不读合计、不标超核、不重算它映射到的作用域；只有读面上为负的累计未分配与 CLI 的「累计超核」可见，对同一事实再核一律答分配失衡。核销与更正在事实身份行上排队只保证核销读到更正后的金额。去向 [code/92](./92-a-funds-fact-correction-below-its-applied-total-is-adopted-silently.md)。
9. **同一条资金事实映射到多个作用域时，每个作用域都按全额记入入账余额**（第三轮轻评审 N-a，既有缺陷，与代决 10 同类）。代决 10 的去重只在一个作用域之内。一笔 5000 的收款映到 acct-1 下的 stmt-1，又映到账户 acct-2，全部核到 stmt-1 之后，acct-2 那个作用域的可用余额仍凭空多 5000，没有提示。碰钱且静默，去向 [code/94](./94-a-funds-fact-mapped-into-several-scopes-is-posted-in-full-in-each.md)（R1）。

### 新票

无（核销事务里的入账余额重算缺口曾立 [code/88](./88-settlement-application-and-reversal-do-not-recompute-the-posted-balance.md)，修订轮并入本票修，见 code/46 修订轮）。

### 测试（含 PG，`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`，变基后的 tip 上跑）

- 改动包的反向依赖（`go list -test` 以 `internal/settlementaccounting/domain` 与 `migrations` 为根算出 66 个包，含各 postgres 适配器与 `cmd/*`）加 `./cmd/parcel-settlement-register/...`、`./internal/architecture/...`、`./migrations/...`：退 0（65 个包 ok、2 个无测试文件；含 PBC-08 门禁——新写口 `LockClaimTarget` 的无事务负面证据是 `TestLockingAClaimTargetRefusesToRunOutsideATransaction`）。
- 新用例 `-count=3`：退 0。`go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出。
- 种子灌一次性库 `c12seed`：退 0；库上 `funds_mapping_target_kind_closed` 已是六值、种子那一行 `ACCOUNT` 共存；用后 `DROP DATABASE … WITH (FORCE)`。
- 管理台三道门（Node 22）：`tsc -b --noEmit`、`scripts/run-tests.mjs`（601 过）、`vite build` 退 0。
- 先红：领域、应用、postgres、批文、CLI 五层的新用例都先于实现写成，先红是编译不过（新符号未定义）；行为上的红由 CLI 竖切用例撞出一处——postgres 读回词表漏了两格，映射读回失败答 `FUNDS_MAPPING_STORE_UNAVAILABLE`，补上并加了 postgres 往返用例。
- 变异（改完都跑红、复原后跑绿；复原是把备份拷回，没有用 `git checkout`）：
  1. 核销时改动赔付本体那一行（`LockClaimTarget` 里 `UPDATE customer_claim_amount SET recorded_at`）→ CLI 竖切与 postgres 未核销面两条用例红；
  2. 撤销删掉原分配（领域 `Reverse` 清空分配 / 适配器 `Replace` 改写 `allocations`）→ 应用层与 CLI 竖切各红；
  3. 新目标漏租户一致性（`FindClaimTarget` 去掉 `tenant_id = $1`）→ postgres 存在性用例与 CLI 竖切红；
  4. 去掉 `FOR UPDATE` → 并发用例红（后到的事务十秒内没在锁上等）；锁与求和并成一句 → 并发用例红（后到的锁读到已核销 0）；
  5. 去掉按目标的未核销上限 → 应用层两条用例红；
  6. postgres 读回词表去掉追偿认可那一格（时名 `RECOVERY_RECEIVABLE`）→ 往返用例红。
- AT 盘点：`AT-SA-116`、`AT-SA-117` 从③、`AT-SA-108`、`AT-SA-109`、`AT-SA-110`、`AT-SA-154` 从②移进①（各注明半边）；`AT-SA-112`、`AT-SA-113`、`AT-SA-155` 加名字。总览计数没手算，留给推送方重算。

## 评审记录与修订轮（2026-10-26，协调方隔离评审：不可合入，一条阻断；作者修订轮，分支 claude-code12）

评审结论与各条照协调方回给作者的原文摘要；修订轮在同一分支上加新提交，不改写旧提交（变基照规矩重放）；收尾时变基到本地 main `fb8ee7096`，修订轮期间 main 前移的那几笔不碰 settlement-accounting，变基无冲突，变基后重跑了测试（见 code/46 修订轮「测试」）。

- **B1 阻断：同一笔资金事实能被多次核销，累计超过事实金额**（评审真库实测：5000 的收款换三个核销引用各交 5000，三笔都退 0）。病因：`Apply` 只靠 `domain.ApplySettlement` 看本笔，库上没有按事实的上限；CLI 答复的「未分配」取 `RemainderMinor()`，不跨核销累计。**修**：`Apply` 落行前先以 `FOR UPDATE` 锁事实身份行（`SettlementApplications.LockFundsFactApplications`，新口 `ports.FundsFactApplicationLock`），锁后另起一句求该事实未撤销核销的 `applied_minor` 合计；`domain.SettlementApplication.AdmitWithinFact` 判本笔加已有是否超过当前有效那一版的金额，超过答 `APPLICATION_IMBALANCE`、什么都不落。CLI 答复改为「本笔 X；事实 F 累计未分配 Y」，Y 是这一步之后该事实的累计未分配（`FundsResult.FactUnappliedMinor`）；撤销答复只说「已撤销」与累计未分配，不再报原核销金额。补真库竖切两条：`TestAFundsFactIsNotAppliedBeyondItsAmountAcrossApplications`（顺序超额）、`TestConcurrentApplicationsOfOneFactStayWithinItsAmount`（并发超额，一条自己的事务先占住身份行，等两路都在锁上排队——由 `pg_stat_activity` 认——再放开）。
  - **锁选身份行不选链头、不选咨询锁的理由**：身份行一条事实一行、终身不换，链头随每次更正换一行；更正落版本行时外键对身份行取 KEY SHARE，与 `FOR UPDATE` 互斥（一次性库 `c12fk` 上实测：父行被 `FOR UPDATE` 占着时，子表插入等了约 2 秒，直到占锁事务提交），所以核销与更正在同一行上排队、核销读到的一定是更正之后的金额；咨询锁挡不住不取它的更正。（第三轮改准：原写「更正编排不用另取一把锁」不对——排队保不住已核销合计不超过事实金额，见文末第三轮 N3。）
- **N1 改名**：`RECOVERY_RECEIVABLE` 全链路改为 `RECOVERY_ACKNOWLEDGEMENT`（领域常量 `TargetRecoveryAcknowledgement`、两张词表、迁移 0046 的 CHECK、管理台标签）；`RECOVERY_RECEIVABLE` 改作词表外值的反例——同上下文 `AdjustedAmountKind` 里它指应追偿金额。
- **N2 并入 code/88**：见 [code/88](./88-settlement-application-and-reversal-do-not-recompute-the-posted-balance.md) 与 code/46 修订轮。`ErrInvalidClaimTargetStanding`（金额本体账不平）不再落进 `CLAIM_TARGET_STORE_UNAVAILABLE` 未决，改答分配失衡（确定性，ADR-0029）。
- **N3**：UC-SA-005 目标与边界、输入表「未结项」、`AT-SA-117` 对齐到「核销到追偿认可金额，认可之前到账的追偿收款保持未分配」；越权风险点 1 保留。
- **N4**：`docs/application/README.md` 的 UC-SA-005 一行按新状态行同步。
- **N7**：`TestAClaimTargetStandingCountsOnlyLiveApplicationsOfItsOwn` 在 tenant-b 的同名 `compensation-1` 上真落一笔核销；去掉求和里 `app.tenant_id = $1` 的变异现在被杀。
- **协调方另裁**：资金退回事实映射或核销到两格金额本体一律未受理（`returnedFundsKeptOffClaimAmounts`，形成门两处守；既有四格不动），SA CONTEXT 规则句补一句；管理台 `CUSTOMER_COMPENSATION` 标签改「客户赔付或索赔费用退款」；`MapExternalFundsDeps.ClaimTargets` 注释点名 `Books`、`Balances`、`Standings`；代决 8 改准。

### 锁序全序论证

资金侧取的锁只有三类（取证于修订轮那一笔代码提交，标题「fix(settlement-accounting): 文档自洽 code/12、code/46 修订轮——事实一级核销上限……」；按标题锚而不按 SHA，变基与推送方重放都会换 SHA；第三轮没有加第四类锁），全仓只按一个次序取：**事实身份行（`FOR UPDATE`）→ 金额本体行（`FOR UPDATE`，按格、引用）→ 运营余额作用域（事务级咨询锁，按法人、账户、币种）**；次序写在 `ports.FundsFactApplicationLock` 头注，`ClaimSettlementTargets.LockClaimTarget`、`OperationalBalanceAdvance.LockOperationalBalanceScope` 与 `Apply`、`Reverse` 头注都回指它。

- `Apply` 取全序：事实 → `ClaimTargetDemands` 排好序的金额本体 → `statementScopes` 排好序的作用域。
- `Reverse` 取子序列：事实 → 作用域。
- `Map` 只取一个作用域。
- `CorrectFact` 只在落版本行时经外键对事实身份行取 KEY SHARE——它与 `FOR UPDATE` 互斥，算作只取事实这一类。
- 接受前控制取的是另一个锁空间（`pre_acceptance_control.scope`），那一路不重算、不碰这三类。

每条路径取的都是这条全序的一个前缀或子序列、同类之内按同一键序，没有一条在持有后一类时再去取前一类，所以任意两笔事务不会各握一半互等。同一事务里作用域锁再取一次是重入。

### 修订轮测试（含 PG，变基后的 tip）

见 code/46 修订轮「测试」，两票一并记。

## 第二轮评审与第三轮修订（2026-10-26，协调方隔离评审：B1 已消、无阻断；作者第三轮修订，分支 claude-code12）

先变基到 origin/main `0f21df208`，收尾时再变基到 `24ba2c85c`（评审时说的 `744a424b7` 之后又前移了几笔，都不碰 settlement-accounting），两次都无冲突；AT 盘点两边的追记都留着。各条：

- **N1 锁后取时**：`recomputeScope` 先取作用域锁再取 `Clock.Now()`，`advanceCustomerReceipt` 的信用状况那一步用它交回的同一时刻。锁下推进运营余额或信用状况答未推进就返 `ErrPostingNotAdvanced`、整笔回滚，CLI 落未决 3；只可能是跨进程时钟回拨，不该静默答成功。
- **N2 作用域锁的守护**：补两条真库并发用例（code/46 第三轮）。
- **N3 外部更正低于已核销合计**：本批不改代码，立 [code/92](./92-a-funds-fact-correction-below-its-applied-total-is-adopted-silently.md)。越权风险点补一条；UC-SA-005 恢复「给开发的交接」一节，放 `AT-SA-114` 一行；AT 盘点 `AT-SA-114` 注明只覆盖「事实是引用」那半边。锁选理由改准，见下一条。读面与 CLI 都不把负的累计未分配截成 0，CLI 说成「累计超核 X」。
  - **锁选理由改准**：`SettlementApplications.LockFundsFactApplications` 头注与上一轮「锁选身份行」那段原说「更正编排不必另取一把锁」，不对。排队只保证核销读到更正后的金额，保不住「已核销合计不超过事实金额」。更正侧要核合计，得自己先对身份行 `FOR UPDATE` 再求和——插版本行时那把 KEY SHARE 来在读之后，晚了。
- **N4 入账余额按事实去重**（代决 10）。
- **N5**：金额本体账不平与作用域账不平续办相同，并成一格。编排对前者返包着 `ErrInvalidClaimTargetStanding` 的 error，`execute` 与 `ErrNegativeConfirmedUnsettled` 同一行归冲突格 2。
- **N6、N7**：见 code/46 第三轮。
- **N8**：UC-SA-005 `AT-SA-112` 补半句——原收款核销在两格金额本体上时，退回事实不映射、不核销到这两格，停在未分配，由本用例撤销那一笔原核销。

## 第三轮轻评审（2026-10-26，协调方：可合入，无阻断）

合入前两条记录，只改票面与注释、不改逻辑：
- **N-a**：跨作用域重复计入 P 立 [code/94](./94-a-funds-fact-mapped-into-several-scopes-is-posted-in-full-in-each.md)（R1），越权风险点补第 9 条。`CustomerDirectionNets` 在端口与适配器上的头注改准：去重只在本作用域之内。
- **N-b**：[product-strategy-boundary/22](../../product-strategy-boundary/issues/22-settlement-orchestrations-assembly-and-triggers.md)「要裁的」加一行，回指 code/46 越权风险点「潜伏卡死：确认后的贷记调整让作用域账不平之后，撤销也撤不动」。
- **可选两条也做了**：`admitClaimTargets` 头注改为按返回值指；`ErrPostingNotAdvanced` 的文案补一句与跨进程时钟有关。
