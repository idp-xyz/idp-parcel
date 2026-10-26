# 17 参与方关系没有撤销、到期、替代的写面：一段关系一旦生效，只能靠再登一版带终点的修订来结束

Category: enhancement
Status: in-progress——2026-10-25 作者完工，待隔离评审与进 main，分支 claude-psxm17 tip 为写本完成记录的那一笔（代码最后一笔 `af6187bf2`；见「完成记录」）。此前 in-progress——2026-10-25 会话 `tops-20`（Claude Code）改认领：原认领会话 `idp-parcel-9b` 已不在，分支 `claude-psxm17` 当时 0 提交，快进到 main `9ea060682` 复用；协调方 tops-26 同意（它在本票期间不开 PS 的 code 票）。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（infra-speed/02 已进 main，Blocked by 解除），分支 `claude-psxm17`。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立并代决；等另一会话的 infra-speed/02（组合根按文件自登记）进 main 再开工——本票要加端点
Blocked by: infra-speed/02 进 main（另一会话的票，加端点的配方随它定）
父票：[10](./10-authorized-delivery-relationship-in-party-commercial.md)
出处：票 10 完成记录「没做」：这是各参与方关系角色共有的旧缺口，授权交付关系让它变得要紧——撤掉一个交付方的授权要能即时生效。

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「yes」。

## 现象（读码）

PC 领域的参与方关系有撤销、到期、替代三种结束（`PartyRelationship` 的生命周期各是下一修订），但登记写面与受控批量口只会登候选与生效；结束一段关系今天只能再登一版带终点的修订，撤销与替代没有口。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制）

- 在参与方关系的登记写面能力面上加三个写口：撤销（即时结束，带依据）、到期（指定终点，带依据）、替代（以接替的持有方结束本关系并起一段新关系，照领域 `SupersededBy`）。三口走领域既有的结束方法，不另立判断；重放与内容冲突代数照该能力面现有写口。
- 受控批量口同形加三种。
- 管理台关系详情给「撤销」「到期」两个动作（替代先只开写口与 CLI，界面另议）。
- 授权交付判断（票 10）读最新修订即可自然生效，不改判断。

## 完成判据

- [x] 领域、编排、PG、HTTP、CLI 各层用例；撤销后授权交付判断即时答否（真库）。
- [x] 管理台两个动作与 node:test；三道门。
- [ ] Go 门禁；全量用 `scripts/test-full.sh`；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。

## 完成记录（2026-10-25，会话 tops-20 的作者子代理，分支 `claude-psxm17`）

起点 main `86666316d`。证据层级 **S**：夹具全为 `SYN-` 或 `tenant-1` 一类合成值，隔离合成库上跑。

**提交**

| 提交 | 内容 |
|---|---|
| `693c66425` | 领域门、编排、PG 读回、迁移 0042，各层用例（含真库「撤销后授权交付判断即时答否」） |
| `9c5c55bd6` | 共用批文形状（registrationjson）、HTTP 三口、parcel-api 自登记特性文件、CLI 子命令，各层用例 |
| `af6187bf2` | 管理台关系详情的撤销、到期两个动作与 node:test |
| 本笔 | 本完成记录 |

**各层改了什么**

- 领域 `internal/partycommercial/domain/party_relationship.go`：
  - `Expire(basis, at)`：到期收依据，必带；其余门照旧（已生效、区间有终点、到期时刻不早于终点）。
  - `SupersededBy`：接替方不能是原持有方，也不能是相对方。
  - 新 `SuccessorSpec(successor, basis, at)`：推出接替关系的正文。持有方换成接替方，相对方、角色、范围照旧；区间起点取替代时刻与原起点中较晚者，终点同原关系；依据是这次替代的依据。不判状态（重放时原关系已是`已替代`）。
- 编排：新文件 `internal/partycommercial/application/end_party_relationship.go`，`PartyRelationshipEndingHandler` 的 `Revoke` / `Expire` / `Supersede`。
  - 形照身份停用：取最新修订 → 领域转换 → 存下一笔修订。
  - 重放与冲突照停用口（`deactivationReplay` 的判据）：同修订同内容答`重复`，同修订异内容答`内容冲突`，已结束又换落点答`未受理`；关系不在册答`未找到`；修订错位答`未受理`。
  - 多一道门：结束时刻不得晚于登记时刻（`ports.Clock`）。
  - 替代先登接替关系（经 `RegisterRelationship`，过关系登记的全部门）、后落原关系的终止修订；原关系那一笔落不下时交回 error，让调用方整笔回滚。
  - 结果代数多一格 `PartyRelationshipEnded`（线上名 `ENDED`），HTTP 答 201。
- `RegisterRelationship`（`register_party_identity.go`）：已结束的关系不再收新修订。
- PG `party_identity_registry.go`：EXPIRED 快照读回带终止依据。落库形状本来就能表达终止时刻、终止依据、接替方三格，没改列。
- 迁移 `migrations/party_commercial/0042_party_relationship_ending_mirrors.sql`：号照协调口径取 0042（0041 归 tops-26 的 code/25）。收紧 `end_coherent`：到期也必带终止依据；替代的接替方不能是原持有方或相对方。
- 共用批文：新文件 `internal/partycommercial/adapters/registrationjson/party_relationship_ending.go`，外壳 `PartyRelationshipEndingBatchDocument` 分 `revocations` / `expirations` / `supersessions` 三节，受控批量口与在线三口共用一份形状与逐项翻译。
- HTTP：新文件 `internal/partycommercial/adapters/http/end_party_relationship.go`。三口 `/commercial-party-relationship-revocations`、`-expirations`、`-supersessions`，挂操作者渠道的登记册 Intake，各收本节恰一项、租户取认证结果。`register_party_identity.go` 只改 201 那一格。
- parcel-api：新特性文件 `cmd/parcel-api/endpoints_commercial_relationship_ending.go`，照 ADR-0222 自登记三行（登记册配置写面）、自建事务壳；装配测试的未接线占位在同名 `_test.go`。没动 `endpoints_commercial_registration.go` 与 `assemble_commercial_registration.go`。
- 受控批量口：新文件 `cmd/parcel-commercial/end_party_relationships.go`，子命令 `end-party-relationships`；`main.go` 只加分派一格与用法串。
- 管理台：
  - 新文件 `apps/admin-web/src/pages/party/PartyRelationshipEndActions.tsx` 与 `party-relationship-end-form.ts`（纯逻辑，`.test.ts` 钉着）。
  - `api.ts` 只在末尾追加端点、调用与结束口答案词表；`BusinessPartiesPage.tsx` 只在关系抽屉挂组件、传刷新回调。
  - 形状照协调方转达的 tops-54 建议第 4、6 条：撤销、到期是抽屉头部的次要按钮，撤销用危险样式；关系不在`已生效`时禁用并写明原因（到期另要区间有终点）；时刻用 `WallTimeField` 按本地时区填；确认框复述关系一句话、生效边界（本地时间 + UTC）、依据与落点修订，写明「只影响边界之后的新决定，已作出的决定不变；不可撤回」；答 `ENDED` 时重取关系册，重取保留旧答案，表格不回骨架，抽屉这一行就地换新。第 5 条（范围写成「客户账户 …」）没做。
- 没动：`ports.go`、`docs/**`（含 PC CONTEXT、GLOSSARY、CONTEXT-MAP、UC、ADR）、`scripts/demo-seeds`、`components/` 下共享件、`PartyRelationshipRegistrationForm`、`IdentityDeactivationForm`、`party-registration-fields.tsx`（只 import）。

**代决与越权风险点**（授权依据同 Status 行：用户授权「你作为业务和系统专家你来自决」）

1. **到期的语义：登记已到期，不是预定未来终点。**
   - 票面写「到期（指定终点，带依据）」，领域 `Expire(at)` 不带依据，且要求区间已有终点、`at` 不早于终点。
   - 取法：到期口走领域 `Expire`，「指定终点」落成「指定到期时刻」——不早于区间终点（领域门），也不晚于登记时刻（用例门，见 2）。「带依据」照票面，领域 `Expire` 加必带依据。开区间的关系不能到期，只能撤销或替代。
   - 不取「设一个未来终点」的理由：关系状态是登记进去的事实，`AppliesAt` 只按状态答。一笔 `EXPIRED` 修订一落册，关系就对任何锚点都答否。指定未来终点若落成 `EXPIRED`，等于提前撤销；要做成预定，只有两条路：新立「收短有效区间」的领域转换，或改 `AppliesAt` 按终止时点答。后者会改授权交付判断（票 10 的「已到期」用例按状态答否），违背本票「不改判断」。
   - 预定未来终点今天的走法：经既有登记口登一版带区间终点的修订（重述正文并带批准）；到期之后再经到期口登记。
   - CONTEXT 没改：「已生效 → 已到期、已撤销或已替代：自明确生效边界起不再支持新的商业决定」与本取法不冲突。
   - 若 owner 要预定到期：新立领域转换「已生效 → 已生效（区间收短，带依据与批准）」，到期口改收未来终点并落这一笔；判断不必改。
2. **结束时刻不得晚于登记时刻**（新门，撤销、到期、替代三口共有，取 `ports.Clock`）。理由同 1：终止修订落册即让关系不再支持新决定；写未来时刻，册面会说「到那时才结束」，判断此刻却已不认它。代价是浏览器或批文的钟比服务端快时被拒，拒因带两个时刻。
3. **替代同笔登记接替关系，正文由领域推出。**
   - 票面「以接替的持有方结束本关系并起一段新关系」，领域 `SupersededBy` 只记接替方。本票让替代口在同一事务里登记接替关系。
   - 接替关系正文取法见上面领域 `SuccessorSpec` 一条；调用方只给接替关系标识、依据与批准，批准必带——否则原关系结束而接替关系只是候选，中间空出一段谁都不持有的时间。相对方、角色或范围也要变的，走撤销加另登新关系。
   - 领域 `SupersededBy` 新加一道门：接替方不能是原持有方或相对方。
   - 这几条是对 CONTEXT「已生效 → 已替代」的细化，CONTEXT 没写。若要写进 CONTEXT，建议在「参与方关系」生命周期一节加一句：「替代只换持有方：接替关系由接替方持有，相对方、角色与适用范围不变，自替代时刻起接住原关系余下的有效区间。」本票没改 CONTEXT，留协调方定。
4. **已结束的关系不再收新修订**（`RegisterRelationship` 新门）。不加则撤销之后可再登一笔已生效修订，悄悄把撤销翻回；授权交付判断只读最新修订，撤销就不再确定。判据同已停用身份不收新修订。重新合作要登一段新关系。
5. **撤销时刻可以回溯，且回溯的撤销对任何锚点都答否。** 领域 `Revoke` 只要求时刻非零；`AppliesAt` 按状态答，撤销之后连撤销时刻之前的锚点也答否。这是领域既有语义，票 10 的用例已钉；本票没改。若 owner 要撤销只影响撤销时刻之后的锚点，得改 `AppliesAt`，会改授权交付判断，须另票。
6. **新结果格 `ENDED`。** 不借`已登记`或`已停用`：关系的终止不叫停用，登记方也要分得清这一笔是立起还是结束一段关系。`未找到`扩到关系结束口。管理台另立结束口答案词表，没改 `partyIdentityOutcomeLabels`。
7. **迁移 0042。** 本票之前没有写口落过终止修订，册上没有会被新 CHECK 拒的既有行；用例里那几行都是本票自己写的。号取 0042，0041 留给 tops-26；两份迁移互不相干，进 main 次序不影响施加。
8. **管理台按行状态禁用按钮。** 照协调方转达的 tops-54 建议，不是表单裁门：按钮禁用只是不开表单，送出去的一律由服务端判。到期按钮在开区间时也禁用，因为到期时刻的预填值就是区间终点。终点到没到不在前端判，那要拿浏览器的钟比。落点修订自动取这一行修订加一、不给改（停用表单给改）：动作挂在这一行上，声明的就是这一行的册面；册面被推进时服务端答修订错位。
9. **共用批文用 registrationjson。** 身份族在线口与 CLI 各一份同形副本的旧做法没沿用，照版本收尾那一族的新做法共用一份。新 Intake 没给 `UnconfiguredIntake` 加实现，同版本收尾先例：装配点挂的是操作者 Intake。
10. **没做**
    - 替代的管理台入口（票面「界面另议」）。
    - 演示种子没加结束样例：灌一段撤销会让票 12 要用的授权交付关系失效。要演示已撤销状态，另加一段专用关系即可。
    - 管理台没有起页面实测（不占 5173），只过了三道门与纯逻辑用例。

**完成判据格 → 用例**

| 格 | 用例 |
|---|---|
| 领域 | `internal/partycommercial/domain/party_relationship_ending_test.go`：`TestExpiryNeedsABasisAndAReachedEnd`、`TestSupersessionNeedsADifferentHolder`、`TestTheSuccessorTakesOverOnlyTheHolder`；既有 `TestEndingARelationshipStopsNewDecisionsButKeepsHistory`、`TestAuthorizedDeliveryJudgment` 跟上新签名 |
| 编排 | `internal/partycommercial/application/end_party_relationship_test.go`：`TestRevokingARelationshipEndsItOnce`（含已结束关系不收新修订）、`TestExpiringARelationshipNeedsItsEndToHaveArrived`、`TestSupersedingARelationshipRegistersItsSuccessorInTheSameStroke`、`TestASupersessionThatCannotEndTheOldRelationshipFailsTheWholeStroke` |
| PG（真库） | `internal/partycommercial/adapters/postgres/party_relationship_ending_test.go`：`TestEndedRelationshipsLandAndReadBackFromTheRegister`、`TestTheRegisterRefusesEndingsTheDomainRefuses`（0042）、`TestRevokingAnAuthorizedDeliveryRelationshipAnswersNoAtOnce`（**撤销后授权交付判断即时答否**；替代后原交付方答否、接替方答是） |
| HTTP | `internal/partycommercial/adapters/http/end_party_relationship_test.go`：形状门、越权 403 不读载荷、答复代数；`internal/partycommercial/adapters/registrationjson/party_relationship_ending_test.go`；进程装配真库端到端 `cmd/parcel-api/endpoints_commercial_relationship_ending_test.go::TestTheOperatorChannelEndsPartyRelationshipsAgainstARealDatabase`；端点表与接入面由既有 `TestEachLineOpensOnlyOnItsDeclaredAccessFace` 覆盖 |
| CLI | `cmd/parcel-commercial/end_party_relationships_test.go`：`TestRelationshipEndingBatchTranslatesTheThreeLines`、`TestEndPartyRelationshipsBatchLandsOnARealDatabase`（真库，含撤销后授权交付判断答否、重放退 0、冲突退 2） |
| 管理台两个动作与 node:test | `apps/admin-web/src/pages/party/party-relationship-end-form.test.ts` 七条；三道门见下 |
| Go 门禁、全量、机制清点 | 作者只跑受影响范围（见下）；全量 `scripts/test-full.sh` 与机制清点重生成归推送方 |
| 走 Go 那条路 | 分支 `claude-psxm17`，每笔提交后已推；待非作者评审与推送方 |

**自验**（Go 与管理台都在 `af6187bf2` 的内容上跑；之后只改本票面）

- Go（含 PG：`IDP_PARCEL_POSTGRES_DSN` 指共享门禁库 55432，用例自建自删物理库）：
  - `go build ./...`、`go vet ./...` 退 0；本分支改过的 `.go` 过 `gofmt -l` 无输出（新写的都先 `gofmt -w`）。
  - 协调方点名的范围：带 DSN `go test -p 1 -count=1 ./internal/partycommercial/... ./internal/nodeoperations/... ./cmd/parcel-commercial/... ./cmd/parcel-api/... ./migrations/... ./internal/architecture/...`，有测试的包 21 个全 ok，退 0。`internal/architecture` 里「管理台路径 ⊆ 端点表」那道门随之核过两条新路径。
  - 改过的 PC 包另有反向依赖不在上面那组里，按 parallel-sessions「全量只跑一次」作者那条一并带 DSN 跑：`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`cmd/parcel-settlement-register`、`internal/parcelshipment/adapters/{partycommercial,postgres}`、`internal/{networkrouting,settlementaccounting,transportfulfillment,visibilityexception}/adapters/partycommercial`，9 个包全 ok，退 0。
  - 新用例逐个以 `-v` 核过确实跑了、没有 SKIP。
- 管理台三道门（Node 22，`pnpm install --frozen-lockfile` 装依赖）：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 608 条全过（含本票七条）；`pnpm exec vite build` 成（只有既有的 chunk 体积提示）。
- 全量 `scripts/test-full.sh`、机制清点重生成：没跑，归推送方。
