# 10 授权交付关系：参与方关系多一种「授权交付方」角色，PC 答授权交付判断，NO 侧立防腐适配器

Category: enhancement
Status: resolved——2026-10-19 进 main（代码验证钉 `61543aa0`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领，分支 `claude-psxm10`；与票 08 并行。此前 ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by: 无（ADR-0212 已接受，PC CONTEXT「参与方关系」已随票 07 补上授权交付关系一句）
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定一、三、九；PC CONTEXT「参与方关系」；CONTEXT-MAP `party-commercial → node-operations`；参数登记册 `PAR-NET-17`。
地盘：
- `internal/partycommercial/domain/party_relationship.go`（`PartyRole` 多一种；授权交付的适用范围怎样写）及测试
- `internal/partycommercial/ports`、`adapters/postgres`：授权交付判断读口（新文件）及测试
- 既有参与方关系的登记写面与受控批量口：只需认新角色，先 grep 既有登记口与 CLI 再动
- `migrations/party_commercial/`：只在角色或范围落库带 CHECK 时才要，取当时下一个空号
- `internal/nodeoperations/ports`（授权交付判断端口，新文件）、`internal/nodeoperations/adapters/partycommercial`（新防腐适配器）及测试；装配在票 12
- `scripts/demo-seeds/data/commercial`：演示租户一个合成交付方参与方、一条授权交付关系
- `docs/product/MECHANISM-INVENTORY.md`（重生成）

## 做什么

1. **角色**。`PartyRole` 多「授权交付方」：持有方是交付方参与方，相对方是客户账户所属的参与方；适用范围收到客户账户，可再收到节点。范围怎样写进 `CommercialScopeReference` 由本票定，写进完成记录；若要改 PC CONTEXT 的措辞，先改 CONTEXT。生命周期照参与方关系：候选、生效、到期、撤销、替代。
2. **授权交付判断读口**：输入（租户，交付方参与方，客户账户，时刻），答复是、否、未配置之一，读不动交回错误。
   - **是**：交付方就是该客户账户所属参与方本身；或该时刻有一条生效的授权交付方关系，适用范围覆盖这个客户账户（有节点范围的，也覆盖开立节点）。
   - **未配置**：本租户一条授权交付方关系都没登记过，交付方又不是该客户账户所属参与方本身。
   - **否**：其余情形。
3. **NO 防腐适配器**。把 PC 的答复译成 NO 端口值，不把参与方关系的形状带进 NO。装配在票 12 接，本票只立适配器与它的用例。
4. **演示**。种子登一个合成交付方参与方，再登一条它替 `SYN-ACCOUNT-01` 送站的授权交付关系，证据记 `S`。

## 不做

- 送站交接确认的运营决定口与范围成立（票 12）。
- 按设备上报的交付方推定授权（ADR-0212 决定一）。
- 「操作者代客提交」等别的授权（ADR-0139 决定三）。

## 完成判据

- [x] 领域与真库用例：本人答是；生效关系覆盖答是；关系未生效、已到期、已撤销、范围不覆盖这个客户账户或节点，都答否；本租户空册答未配置；别的租户的关系不算。
- [x] 登记写面与受控批量口能登记新角色，重放与冲突照既有参与方关系口的代数。
- [x] NO 防腐适配器用例：每种答复（含读不动）各译一格。
- [x] 演示种子 `seed.sh` 退 0。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/partycommercial/...`、`internal/nodeoperations/...`、`cmd/parcel-commercial`、`cmd/parcel-api`、`migrations/...`、`internal/architecture/...` 全绿；机制清点重生成。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

S

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-psxm10`）

起点 main `a2ff42f6`。证据层级 **S**：夹具与演示种子全为 `SYN-` 合成值，隔离合成库上跑。

**提交**

| 提交 | 内容 |
|---|---|
| `b2e0d59f` | 先改 CONTEXT：PC CONTEXT「参与方关系」补方向与范围一句；CONTEXT-MAP `party-commercial → node-operations` 与 UC-NO-006 参与者表的问法多一格开立节点 |
| `9ba6e15e` | 领域、登记用例、三处角色名镜像、PC 读口与 postgres 实现、迁移 0039、NO 端口与防腐适配器、演示种子，各层用例 |
| `70b0b7ed` | 机制清点在 `9ba6e15e` 上重生成 |
| 本笔 | 本完成记录 |

**角色与范围的写法**

- 角色：`domain.PartyRole` 多 `AuthorizedDeliveringPartyRole`，外部名 `AUTHORIZED_DELIVERING_PARTY`。持有方是交付方，相对方是客户账户所属参与方。生命周期照参与方关系，没另立。
- 范围写进 `CommercialScopeReference`，写成 URL 查询串：只收到客户账户时 `customerAccount=<账户>`，再收到节点时 `customerAccount=<账户>&node=<节点>`。键按字典序，值按查询串转义。拼由 `AuthorizedDeliveryScope.Reference` 做，拆由 `ParseAuthorizedDeliveryScope` 做，拆只认 `Reference` 拼出的那一种：换序、重复键、多余键、空值、多余转义都拒（`ErrInvalidAuthorizedDeliveryScope`）。
  - 为什么只认一种拼法：登记册按快照摘要答重放与冲突，同一范围两种拼法会把重放答成内容冲突。
  - 为什么用查询串：标识里自带 `&`、`=`、`/` 也拆得回，不必另立禁用字符。
  - 为什么不另开两列：范围是参与方关系各角色共用的一格（PC CONTEXT「参与方关系必须保存双方、角色、方向、适用范围、依据和有效区间」），别的角色的范围照旧是不透明引用。
- 构造门：这种角色的范围拆不出时，`NewCandidateRelationship` 拒，`ErrInvalidPartyRelationship` 包住拆的拒因。读取用 `PartyRelationship.AuthorizedDeliveryScope`，别的角色答 false。
- 节点在 PC 领域新立 `LogisticsNodeReference`，只拿来收窄范围，不核它在不在网络目录里。
- PC CONTEXT 改了措辞（`b2e0d59f`）：「它的持有方是交付方，相对方是该客户账户所属参与方；适用范围收到这个客户账户，可再收到节点。」

**判断读口**

- PC 端口 `ports.AuthorizedDeliveryJudgmentView.JudgeAuthorizedDelivery(ctx, domain.AuthorizedDeliveryQuery)`，新文件 `internal/partycommercial/ports/authorized_delivery.go`。问法是（租户，交付方参与方，客户账户，开立节点，开立时刻），缺格答 `ErrInvalidAuthorizedDeliveryQuery`。
- 判法在领域 `domain.JudgeAuthorizedDelivery(query, evidence)`，postgres 实现 `AuthorizedDeliveryJudgments` 只取册面。它用一条语句取三样：账户的最新修订、本租户登没登过、交付方名下每段这种关系的最新修订。一条语句即一个快照。
- 三格判据：
  - **是**：账户在本租户已登记，且它的所属参与方就是交付方。或者交付方名下有一段关系，最新修订同时满足：角色是授权交付方；相对方等于账户此刻所属的参与方；`AppliesAt(开立时刻)`，即状态已生效且时刻落在有效区间内；范围的客户账户相同，且范围不收到节点或节点就是开立节点。
  - **未配置**：不是本人，且本租户关系册里一行这种角色都没有。任一修订、任一状态都算登过。
  - **否**：其余情形，含已登过时客户账户在本租户查无。
  - 读不动、坏快照、问法缺格：交回 error，不形成答案。
- 撤销、到期与替代都是同一段关系的下一修订，所以只看最新修订。真库用例「最新修订已撤销」钉住这一格。

**迁移**：`migrations/party_commercial/0039_party_relationship_authorized_delivering_party.sql`，号照协调口径取 0039（0038 归票 29）。
- 拆掉角色封闭集 CHECK `party_relationship_registration_role_closed`，再按六个角色名加回。
- 加索引 `party_relationship_registration_by_role_holder`（租户，角色，持有方）。
- 范围写法不另写 CHECK，理由写在迁移头注。

**登记口怎样认新角色**

- 三处角色名镜像各加一项：受控批量口 `cmd/parcel-commercial` 的 `partyRoleFromName`，HTTP 载荷的 `partyRoleFromName`，postgres 快照读回的 `partyRoleFrom`。
- 登记用例 `RegisterRelationship` 不分叉，重放与冲突的代数照旧。授权交付方关系另过一道引用门 `checkAuthorizedDeliveryAccount`，三格过不了都答`未受理`并带原因：范围里的客户账户须已登记；须在关系生效起点已生效；相对方须就是它所属的参与方。

**NO 侧**

- 端口 `internal/nodeoperations/ports/authorized_delivery.go`：`AuthorizedDeliveryView`、`AuthorizedDeliveryQuery`、`AuthorizedDeliveryAnswer`（是 `DeliveryAuthorized`、否 `DeliveryNotAuthorized`、未配置 `DeliveryAuthorizationNotConfigured`）。
  - 交付方用 `DeliveringPartyReference`，注明是决定载荷指名的 PC 参与方标识，不是设备线索。
  - 客户账户照原串收：NO 领域今天没有客户账户类型，那一格归票 08，这里不立第二个。
- 防腐适配器 `internal/nodeoperations/adapters/partycommercial/authorized_delivery.go`：`NewAuthorizedDeliveryView` 构造期拒缺件，逐格译问法、问 PC、译答复。PC 读不动原样包住交回、答复留空；三格以外的答复或译不动的问法答 `ErrUntranslatableAnswer`，不问 PC。没装进任何 `cmd/`，装配在票 12。

**演示种子**

- 新文件 `scripts/demo-seeds/data/commercial/register-authorized-delivery.json`：合成交付方 `SYN-PARTY-DELIVERER-01`，加一条授权交付关系 `SYN-REL-AUTHORIZED-DELIVERY-01`。
  - 持有方是该交付方，相对方是 `SYN-PARTY-SHIPPER-01`。
  - 范围 `customerAccount=SYN-ACCOUNT-01`，不收到节点：票 12 的演示动线在哪个节点收寄还没定，收到节点会让它多一个对不上的风险。
  - 带批准，自 2026-01-05 起生效。
- `seed.sh` 在 `register-parties.json` 之后加一行，README 数据表加一行。
- 没碰 `publish-batch.json` 与 `register-parties.json`，与票 29 的「只追加」约定无交集。

**代决与越权风险点**（授权依据同 Status 行）

1. **问法多一格开立节点。** ADR-0212 决定三、CONTEXT-MAP、UC-NO-006 与本票「做什么」里「授权交付判断读口」一项都写四格，没有节点。但 `PAR-NET-17` 与本票「是」判据都让范围可以收到节点，问法里没有节点就判不了。
   - 已改：CONTEXT-MAP 与 UC-NO-006 那一句（`b2e0d59f`）。
   - 没改：ADR-0212 原文。是否给它补一条复核记录，由推送方定。
   - 若 owner 不要节点维：拒登带 `node` 的范围，问法去掉节点格，改在 `AuthorizedDeliveryScope` 与两份端口。
2. **只看每段关系的最新修订。** 一段已生效的关系若再登一笔候选修订（内容改动待批），判断按候选答否，不回退到上一笔已生效修订。参与方关系「修订中途谁有效」是一般问题，不在本票定。
3. **登过即已配置。** 只登过候选、或全都已撤销、已到期的租户，答否，不答未配置。这照 ADR-0212「越权风险点」一节「按本租户一条都没登记来判」的字面。
4. **登记引用门是本票加的。** 票面只要求「只需认新角色」。加它是因为 PC CONTEXT 写了方向（相对方是该客户账户所属参与方），不加则册上会出现由无关参与方授出的关系。代价是同一批里账户要排在关系之前。CLI 的批内次序本就如此，演示种子也是这样排的。
5. **判断不看账户或交付方参与方本身是否停用。** 账户能不能收件不是授权交付判断要答的问题。若 owner 要停用的账户一律答否，改在 `JudgeAuthorizedDelivery` 一处。

**完成判据格 → 用例**

| 格 | 用例 |
|---|---|
| 领域与真库 | 领域 `internal/partycommercial/domain/authorized_delivery_test.go`：`TestAuthorizedDeliveryJudgment` 子用例逐格覆盖本人（空册也答是）、生效覆盖账户、覆盖账户与开立节点、候选、早于生效起点、晚于区间终点、已到期、已撤销、已替代、别的账户、别的节点、别的持有方、相对方不是账户所属参与方、空册未配置、别的租户的关系不算、账户查无、账户属别的租户；另有 `TestAuthorizedDeliveryJudgmentRefusesAnIncompleteQuery`、`TestAuthorizedDeliveryScopeHasOneSpelling`、`TestAuthorizedDeliveryScopeRejectsOtherSpellings`、`TestAuthorizedDeliveryRelationshipNeedsAStructuredScope`、`TestAuthorizedDeliveringPartyIsARelationshipRole`。真库 `internal/partycommercial/adapters/postgres/authorized_delivery_test.go`：`TestAuthorizedDeliveryJudgmentAgainstTheRelationshipRegister`（空册未配置、空册本人答是、别的租户登了也仍未配置、生效、候选、最新修订已撤销、区间已到期、别的账户、节点相符与不符、早于生效起点、无关系的交付方、本人；租户二各算各的）与 `TestAuthorizedDeliveryJudgmentFailsLoudly`（读不动、缺格、空 db） |
| 登记写面与受控批量口 | 用例：`internal/partycommercial/application/register_authorized_delivery_relationship_test.go::TestAuthorizedDeliveryRelationshipRegistersThroughTheRelationshipUseCase`（落册、重放答已登记、同修订异内容答冲突、账户未登记、相对方不是账户所属参与方、不透明范围都答未受理）。受控批量口真库：`cmd/parcel-commercial/register_authorized_delivery_test.go::TestRegisterPartiesBatchLandsAnAuthorizedDeliveryRelationship`；翻译：`TestPartyBatchTranslationAcceptsTheAuthorizedDeliveringPartyRole`。登记写面翻译：`internal/partycommercial/adapters/http/authorized_delivery_relationship_intake_test.go::TestOperatorRegistryIntakeTranslatesAnAuthorizedDeliveryRelationship`。HTTP 口经 `cmd/parcel-api` 的真库端到端没加：不碰 `cmd/parcel-api`，它与批量口走同一个用例 |
| NO 防腐适配器 | `internal/nodeoperations/adapters/partycommercial/authorized_delivery_test.go`：`TestAuthorizedDeliveryViewTranslatesEveryAnswer`（是、否、未配置各一格，并核问法逐格译过去）、`TestAuthorizedDeliveryViewPassesAReadFailureThrough`（读不动）、`TestAuthorizedDeliveryViewRefusesWhatItCannotTranslate`（集合外答复、空客户账户不问 PC、缺件构造） |
| 演示种子 | `cmd/parcel-commercial/register_authorized_delivery_test.go::TestDemoSeedAuthorizedDeliveryBatchLands`（照 `seed.sh` 次序真库灌三份种子、重放答原结果）。另在临时库 `psxm10_seed_tmp` 上实灌整份 `seed.sh`，退 0，日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm10-seed.log`；库上查得那一行：`SYN-TENANT-01|SYN-REL-AUTHORIZED-DELIVERY-01|1|SYN-PARTY-DELIVERER-01|SYN-PARTY-SHIPPER-01|AUTHORIZED_DELIVERING_PARTY|customerAccount=SYN-ACCOUNT-01|EFFECTIVE`。用完已删库，没碰 psb26_walk |

**先红后绿**

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm10-red.log`，两段。
  - 第一段：新用例先于实现跑，各包编译即红。原话如 `undefined: domain.AuthorizedDeliveringPartyRole`、`undefined: adapter.NewAuthorizedDeliveryJudgments`；NO 适配器包答 `no non-test Go files`。
  - 第二段：实现前用受控批量口读新种子文件（不连库，翻译即拒）。原话 `relationships/SYN-REL-AUTHORIZED-DELIVERY-01：未知参与方角色 "AUTHORIZED_DELIVERING_PARTY"`，`exit status 1`。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm10-green.log`。

**门**（在 `9ba6e15e` 的 Go 内容上跑）

- `gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0。
- 带 DSN `-p 1 -count=1` 跑 `./internal/partycommercial/... ./internal/nodeoperations/... ./cmd/... ./migrations/... ./internal/architecture/...`，33 个有测试的包全 ok，退 0。其中含 `cmd/parcel-commercial`、`cmd/parcel-api`。
- 跑测期间改过一处测试注释的措辞，事后对该包补跑了 vet 与测试。
- 全量 `./...` 没跑，留给推送方。
- 机制清点：`70b0b7ed`。NO 消费 PC 多一组缝，PC 迁移多一份，端口多两个，两口径缺口不变。

**越出地盘的改动**

- `docs/domain/CONTEXT-MAP.md`、`docs/application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md`：问法多开立节点，见代决 1。
- `cmd/parcel-commercial/register_parties.go` 的角色名镜像与 `cmd/parcel-commercial` 下一份新测试文件。地盘写了「受控批量口」，但它在 `cmd/` 下，单列备查；`cmd/parcel-api` 与 NO 的 `cmd/` 都没碰。
- `internal/partycommercial/application/register_party_identity.go`：引用门，见代决 4。
- `scripts/demo-seeds/README.md` 数据表一行。

**未做**

- 装配（票 12）：PC 读口与 NO 适配器没进任何 `cmd/`。
- 管理台：`apps/admin-web` 的 `partyRoleLabels` 只列原有角色，关系目录页与登记表单下拉不认新角色。属前端切片，没碰，需另票。
- 参与方关系的撤销、到期、替代没有登记写面。各角色同缺，是既有缺口：今天经登记口只能登候选或已批准。要收回一条授权交付关系，眼下只能登下一修订、写上区间终点。
- 「做什么」里「授权交付判断读口」一项仍写四格。没改票面原文，以本记录与 CONTEXT-MAP 为准。
- 非作者评审、推送方在干净检出上带 DSN 跑全量（最后一格未勾）。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `9ff7bc05`）

两轴合审，0 阻断。

- **Spec**：授权交付方作为参与方关系的一种角色落到；适用范围写进 `CommercialScopeReference` 只认一种拼法（键序固定、值转义），重放与冲突比快照摘要时不漂。判断读口一条语句一个快照取账户、登没登过、交付方名下各段关系的最新修订；是、否、未配置三格判据与 ADR-0212 一致，读不动交回错误不形成答案。NO 侧端口与防腐适配器就位，装配留给票 12。
- **作者自定的格认可**：问句多「开立节点」——`PAR-NET-17` 允许收到节点，不带就判不了；只认每段关系的最新修订——PC 的修订是同一段关系的状态流转（撤销、到期、替代都是下一修订，内容变更另起一段），最新修订即此刻状态；登过任一行即算已配置；登记时多一道「范围里的账户须在册、生效、归相对方」的门，出自 PC CONTEXT 新句的方向。
- **评审补（推送方，已做）**：ADR-0212 文末追加复核记录，写明问句多了开立节点，决定原文不动。
- **未做（不另立票，随票 13 一并看）**：管理台的参与方角色标签不认新角色；关系的撤销、到期、替代没有写面——这是各角色共有的旧缺口。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-psxm10` 直接叠在远端 main 之上快进推送；机制清点重生成零差。
- 推送方全量一次：钉 `61543aa0` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 149 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
