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

1. **两格目标**：`domain.SettlementTargetKind` 加 `TargetCustomerCompensation`（`CUSTOMER_COMPENSATION`）与 `TargetRecoveryReceivable`（`RECOVERY_RECEIVABLE`），序号接在 `TargetAccount` 之后，既有映射与核销的载荷摘要不变；`PointsAtClaimAmount` 认这两格。头注把 `AT-SA-116` 读回原意：映射到金额本体、核销它，不改它；`AT-SA-121` 禁的是形成或修改金额。
2. **映射**：`Map` 在落行之前经新口 `ports.ClaimSettlementTargets.FindClaimTarget` 核那一条在本租户的金额册上；不在答 `SOURCE_NOT_ACCEPTED`，册读不回答未决 `CLAIM_TARGET_STORE_UNAVAILABLE`（`FundsUndecidedReason` 新增一格，封闭集用例改为四格）。
3. **核销**：`Apply` 过了领域门、落行之前，按 `SettlementApplication.ClaimTargetDemands`（按格、引用排序）逐条 `LockClaimTarget`——先 `FOR UPDATE` 锁金额本体那一行，再另起一句求指向它、未撤销的核销分配净额，交回 `domain.ClaimTargetStanding`；`Admit` 判币种（不同答 `CROSS_CURRENCY`）与未核销（超出答 `APPLICATION_IMBALANCE`），什么都不落。锁与落行同一事务。指向两格的分配只收借向（`claimAllocationDirected`，形成门与读回门都守）。
4. **金额本体只读**：适配器 `sapostgres.ClaimSettlementTargets` 只有两个读口，不写 `customer_claim_amount`、`recovery_acknowledgement`；映射与核销改的只是 `funds_mapping`、`settlement_application` 两表与事实的未分配余额（`RemainderMinor`）。
5. **库**：迁移 `migrations/settlement_accounting/0046_funds_mapping_claim_amount_targets.sql` 照 0036 同形重建 `funds_mapping_target_kind_closed`，六值；存量行照旧满足（演示种子那一行 `ACCOUNT` 在一次性库上实测共存）。postgres 读回与批文两张词表各补两格。
6. **装配**：`cmd/parcel-settlement-register/main.go` 的 `buildRegistrar` 与 `cmd/parcel-api/assemble_settlement_registration.go` 各接一只真适配器（`NewMapExternalFundsHandler` 构造门拒 nil，不接就起不来）。未动 `cmd/parcel-api` 的 endpoints.go、main.go、endpoints_test.go、routes.go。
7. **管理台**：`apps/admin-web` 收付款核销页的目标词表补 `ACCOUNT` 与两格新值，改正同一句「赔付金额与追偿认可本体不在其中」的误读（三道门退 0，601 例全过）。
8. **文档**：SA CONTEXT Lifecycles「客户赔付与外部追偿」加一条规则句（两格目标、映射时那一版、按目标求未核销）；UC-SA-005 状态行与交接表删本票一行；UC-SA-007 交接表只剩本票一行，整节删去（照无缺口 UC 不设该节的写法）；`.scratch/at-coverage-inventory.md` 只移行、加名字（见「测试」）。

### 代决

1. **版本：按映射时那一版并回指。** 两本金额册只增不改，映射指名的那一条（赔付 `amount_id`、认可 `acknowledgement_id`）就是映射时那一版，目标引用本身即回指，映射行不另加版本列。责任、金额规则或认可范围更正时追加的调整（`claim_amount_adjustment`）或另立的认可各有身份，不改已有映射，也不并进它的未核销。
2. **核销余额按目标各自算**：未核销 = 那一条自己的金额（赔付取 `amount_minor`，认可取 `acknowledged_minor`）− 指向它、未撤销的核销分配净额（借加贷减）。不跨目标、不跨同一索赔项或同一应追偿下的别的条目合并，追加调整不算进来。超出即分配失衡、整笔不落，超额保持未分配（`AT-SA-109`）。
3. **`CUSTOMER_COMPENSATION` 收客户方向索赔金额的两种**（赔付义务、索赔费用退款）：UC-SA-005 目标「运营企业对客户的应付/贷记/退款义务」与输入表「赔付应付/费用贷记或退款义务」都把退款算在内。
4. **`RECOVERY_RECEIVABLE` 只指追偿认可金额，不指应追偿金额**（票面「指向已认可的追偿金额」）：对方认可之前到账的追偿收款停在未分配，直到 UC-SA-007 凭 VE 判定有效的对方响应形成认可。
5. **存在性与租户一致性**：按（租户、引用）在那一格对应的册上取；他租户的与不存在的同答 `SOURCE_NOT_ACCEPTED`（ADR-0029：两者恢复动作同是改目标，分开答就是存在性预言机）；格与册对不上（拿认可号当赔付映射）同不存在。这是结果格不是 error，不进 CLI 的未决哨兵表；只有册读不回才是未决。
6. **结算账户一致性**：资金事实不带责任法人与结算账户维（只有来源、付款人、币种、金额），两格目标的法人与账户一致性由映射依据（付款指示）承担，产品不核——与审核应付、贷项两格同一处境。能核的币种在核销时以那一条金额本体的币种为准，不以命令自报的 `targetCurrency` 为准。
7. **只收借向**：贷向是拿赔付或认可去抵客户应收、供应商应付，UC-SA-005「默认不做应收应付净额抵销」；本票不做抵销依据的规则，贷向一律未受理。
8. **不进入账余额**：赔付付款与追偿收款不是客户应收作用域上的收付，`CustomerDirectionNets` 的 P、W 只加总对账单与账户两格，映射与核销这两格不重算运营余额与信用状况。
9. **锁**：行锁落在金额本体那一行，锁序按（格、引用）；锁与求和分两句——并成一句时等锁的那一路读的是锁前快照（变异实测读到已核销 0，见「测试」）。

### 越权风险点（待 SA owner 复核）

1. **追偿到账先于认可**：保险方不回应直接赔款的那一格，钱停在未分配，要 VE 先登对方响应、UC-SA-007 形成认可才映射得上。若 owner 认为应追偿金额也该是核销目标，加一格或放宽这一格即可。
2. **同一应追偿下先后两条认可各算各的未核销**：旧一条已核销的部分不并进新一条，读单格会把新一条看成整额未结；产品今天没有认可之间的替代关系可读。
3. **追加调整不进未核销**：赔付减额调整之后按原额付款，那一条显示部分核销、留下已不欠的未结；增额调整那一部分的付款没有目标可映射，停在未分配。
4. **净额抵销一律不收**（代决 7）：租户合同明确允许以赔付抵应收时，这一格今天答未受理。
5. **法人与结算账户一致性不核**（代决 6）：把甲法人的赔付付款映到乙法人的赔付上，产品不拦，只认付款指示。
6. **索赔费用退款也收进 `CUSTOMER_COMPENSATION`**（代决 3）：若它已经对账单贷记给客户，再映射一笔现金付款就付了两遍；映射要显式依据，产品不交叉核对账单纳入。
7. **未核销面的求和扫该租户全部未撤销核销的 jsonb 分配**，没有索引；与 `CustomerDirectionNets` 同形，量大时要加。

### 新票

无（核销事务里的入账余额重算缺口立在 [code/88](./88-settlement-application-and-reversal-do-not-recompute-the-posted-balance.md)，见 code/46 完成记录）。

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
  6. postgres 读回词表去掉 `RECOVERY_RECEIVABLE` → 往返用例红。
- AT 盘点：`AT-SA-116`、`AT-SA-117` 从③、`AT-SA-108`、`AT-SA-109`、`AT-SA-110`、`AT-SA-154` 从②移进①（各注明半边）；`AT-SA-112`、`AT-SA-113`、`AT-SA-155` 加名字。总览计数没手算，留给推送方重算。

