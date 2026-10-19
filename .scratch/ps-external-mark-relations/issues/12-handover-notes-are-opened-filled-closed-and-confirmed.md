# 12 送站交接单：作业事实登记开立、归入、关闭；客户渠道与运营决定两路确认绑定；范围成立时自投门铃重核

Category: enhancement
Status: resolved——2026-10-19 进 main（代码验证钉 `ec3dc885`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（阻断一条已修，会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理修完评审阻断（以送站交接单为依据推范围须是它的成员），待复审。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（08、10、11 已进 main），分支 `claude-psxm12`；access_identity 迁移 0016、node_operations 迁移 0016、治理种子 governance/37。此前 ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by:
- [08](./08-pre-advice-and-handover-note-model-and-window-register.md) 进 main：交接单的领域与册。
- [10](./10-authorized-delivery-relationship-in-party-commercial.md) 进 main：授权交付判断与 NO 侧适配器。
- [11](./11-identity-check-judges-the-verifiable-delivery-scope.md) 进 main：门铃触发的重核要带上范围的判断。
- 首方客户渠道 Intake 已由 [first-party-api/00](../../first-party-api/issues/00-first-party-customer-intake.md) 进 main（`38916b00`），客户渠道确认口照它的形状挂，不设阻塞边；与 [09](./09-pre-advice-submission-on-the-first-party-customer-channel.md)、first-party-api/01 同改客户面能力的枚举与 CHECK，谁后进谁 rebase。
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定三、六、九；NO CONTEXT 生命周期「送站交接单」；[UC-NO-006](../../../docs/application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md) 与 `AT-NO-071` 至 `AT-NO-075`。
地盘：
- `internal/accessidentity`：作业事实登记的事实类型加开立、归入、关闭；运营决定加决定种类「送站交接确认」；客户面能力加「送站交接确认」；各自测试
- `migrations/access_identity/`：上面几处 CHECK，取当时下一个空号（另一会话正在连续取号，不写死号；可并成一份）
- `internal/nodeoperations/application`：开立、归入、关闭、两路确认、范围成立与门铃的编排，及测试
- `internal/nodeoperations/adapters/http`：作业事实登记三口、运营决定一口、客户面一口，及测试
- `internal/nodeoperations/adapters/postgres`：门铃，照 `identity_recheck_doorbell.go` 的做法；及测试
- 重核消费者认新门铃：`internal/nodeoperations/adapters/inbox` 与 `cmd/parcel-dispatch`（路由表一行；装配进既有的身份重核装配）
- `internal/nodeoperations/adapters/partycommercial`：票 10 的适配器装进 `cmd/parcel-api`
- `cmd/parcel-api`：端点表、装配、真库用例新文件
- `internal/architecture/partition_subject_registry_test.go`：门铃若用新分区键，加一行
- `scripts/demo-seeds`：运营决定与作业事实登记的授予、准入范围对照、治理种子的生产权威区间（照 no-reception-identity/06 的先例），合成货主系统的「送站交接确认」授予
- `docs/product/MECHANISM-INVENTORY.md`（重生成）、`.scratch/at-coverage-inventory.md`

## 做什么

1. **开立、归入、关闭**（ADR-0212 决定三，「作业事实登记」能力面）。
   - 事实身份由设备签发（ADR-0023），重放照该能力面的既有代数。
   - 开立：节点、交付方线索、声称的客户账户（至少一个）、开立时刻。
   - 归入：交接单须开放；作业实物须在同一节点有当前收寄，当前有效版本待识别、不带关联、不记身份冲突。不合条件答交接单状态不符。
   - 关闭冻结成员。
2. **运营决定确认**（「运营决定」能力面，决定种类字面量本票定，建议 `HANDOVER_NOTE_CONFIRMATION`）。
   - 载荷：交接单、客户账户、交付方参与方、依据引用。
   - 客户账户须在声称之列。
   - 经票 10 的适配器问授权交付判断：是即落确认；否答不在授权交付关系内；未配置答未配置；读不动不形成答案。
   - 运营决定只能确认，不能否认。不限于已关闭的交接单。
3. **客户渠道结论**（首方客户渠道族，客户面能力字面量本票定，建议 `HANDOVER_NOTE_CONFIRMATION`，照「一口一词」）。
   - 客户账户取自信封，须在声称之列；不在的与查无此单同答。
   - 只对已关闭的交接单，结论是确认或否认。
   - 来源请求键照首方族的推导口，从接入请求标识取。
   - 生产写口，准入范围在铸造侧判（ADR-0139 决定八）。
4. **结论代数**：同结论重投答已有结果；异结论答已有结论，并回显在册结论。
5. **范围成立与门铃**（决定六）。
   - 最后一个声称的客户账户得到确认时，同一笔事务给本上下文自投门铃（租户，交接单）；重核对象是名下当前有效版本仍待识别、不带关联、不记身份冲突的作业实物。
   - 向范围已成立的交接单归入一件时，同一笔自投（租户，那一版判断版本），只重核那一件。
   - 用既有的身份重核 inbox 消费者接，判法走票 11 的同一份身份判断。
   - 任一绑定被否认，不交门铃。
6. **答复分格**。信封格：客户面照 ADR-0139 决定四，操作者面照 ADR-0151 决定三与 ADR-0100 决定四。业务格照 UC-NO-006 结果语义契约，线上名本票定、写进完成记录。
7. **逐口核**（ADR-0151 决定三）：运营决定口凭哪条代数，写进完成记录。

## 不做

- 管理台的交接单页与确认按钮（票 13）。
- 客户查看交接单明细的客户面读口。
- 一线过渡导入模板里的交接单行（另票，若需要）。
- 运营决定否认绑定。

## 完成判据

- [x] 编排用例 `AT-NO-071`：开立、收寄带它、归入、关闭；关闭后归入、归入不合条件的实物，都答交接单状态不符；归入不改写来源。
- [x] 编排用例 `AT-NO-072`：授权交付判断答是、否、未配置，以及交付方是本人，依次答绑定已确认、不在授权交付关系内、未配置、绑定已确认；读不动不形成答案。
- [x] 编排与 HTTP 用例 `AT-NO-073`：客户渠道对未关闭的答状态不符；已关闭、声称了它的答确认或否认；没声称它的与查无此单同答；同结论重投答已有结果，异结论答已有结论。
- [x] 编排用例 `AT-NO-074`：声称两个客户账户只确认其一时不成立、不交门铃；另一个否认后不再成立，成员可以归入另一张交接单。
- [x] 真库用例 `AT-NO-075`（`cmd/parcel-api` 与 `cmd/parcel-dispatch`，经生产装配）：没带依据而落待识别的同号实物，归入交接单 → 运营决定确认 → 门铃 → 重核追加关联版，业务时间取原收寄时刻、不另起控制起点，意图一封；名下已关联或记冲突的实物不受影响。修前先红。
- [x] 信封各格：作业事实登记、运营决定、客户面三族各自的未配置、令牌无效、未授予、不在准入范围；别族令牌一律拒。
- [x] 演示实走（`S`）：不带预报送一件，落待识别 → 开交接单并归入 → 运营决定确认 → 识别。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/accessidentity/...`、`internal/nodeoperations/...`、`internal/partycommercial/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-access-register`、`migrations/...`、`scripts/demo-seeds/...`、`internal/architecture/...` 全绿；机制清点重生成。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

L

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-psxm12`）

起点 main `d59ba763`，收口前 rebase 到 `origin/main` `8e5125c2`（无冲突）。授权：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」；下面「代决」各条照此。权威：ADR-0212 决定三、六、九，UC-NO-006 与 `AT-NO-071` 至 `AT-NO-075`，ADR-0139 决定四、八，ADR-0149 决定二、四，ADR-0151 决定一至三，ADR-0180 决定四。没有立新 ADR：没遇到上述各篇没裁的难逆转取舍，实施取舍记在「代决」。证据层级 **S**：夹具、种子与演示全为 `SYN-` 合成值，隔离合成库上跑。

### 提交

| 提交 | 内容 |
|---|---|
| `4a9ac524` | 实现与用例：接入身份三处词与迁移 `access_identity/0016`；NO 领域、端口、编排、postgres、inbox、HTTP、认证适配；迁移 `node_operations/0016`；parcel-api 与 parcel-dispatch 装配、前线导入补收寄成员口；分区主体登记一行；既有用例随新依赖与新格改 |
| `27a3bd97` | 演示种子（授予两行、对照五行、治理区间 `governance/37` 至 `41`、`seed.sh` 五行登记）、种子 README、演示文档「送站交接单」一段与实走记录 |
| `1ad7bca9` | 交接单册与收寄库补两个新端口的实现断言（机制清点基线口径据此认得出） |
| `68ad8bef` | 机制清点重生成 |
| `7d9c8c21` | 本完成记录与 AT 盘点追记十七 |
| `afd7c005` | 评审阻断的修复：以送站交接单为依据推范围须是它的成员（领域、端口、postgres、编排；迁移 `node_operations/0016` 改名并重建收寄册细分 CHECK）；用例先红后绿 |
| `28668fae` | ADR-0212 文末复核记录（决定原文不动）；演示文档答复表补一行 |
| `98866b09` | 机制清点重生成（修复之后） |
| 本笔 | 完成记录随修复改写、AT 盘点那一行补两条用例 |

### 落点

- 接入身份 `internal/accessidentity`：`operation_decision.go` 加 `DecisionDeliveryHandoverConfirmation`；`customer_client.go` 加 `CustomerCapabilityDeliveryHandoverConfirmation`，`customer_client_pre_advice.go` 的「铸造侧判准入」名单加它；新文件 `customer_client_handover.go`（信封、`MintDeliveryHandoverConfirmation`、推导口）、`work_fact_admission.go`（`WorkFactKind` 三种、`WorkFactAdmission`）；`admission_scope_mapping.go` 的 `checkAdmissionRequirement` 认作业事实登记那一格。
- 迁移：`migrations/access_identity/0016_delivery_handover_confirmation_decision_and_customer_capability.sql`（决定种类 CHECK 以 0015 全集为底、客户面能力 CHECK 以 0014 全集为底，各加一词，头注写「之后谁再重建取并集」）；`migrations/node_operations/0016_delivery_handover_binding_source_and_member_inapplicability.sql`（结论多一列来源请求键，客户渠道必带、运营决定必不带；收寄册依据不适用的细分 CHECK 以 0015 全集为底加 `NOT_A_HANDOVER_MEMBER`，头注写取并集）。
- NO 领域 `domain/delivery_handover_note.go`：结论带 `SourceRequestKey`，`DeliveryHandoverNoteState.ScopeEstablishedAt`。
- NO 端口：`ports/delivery_handover.go`（新：`HandoverAssignmentReceptions`、`DeliveryScopeEstablishedIntent`、`DeliveryScopeEstablishedDoorbell`）；`delivery_scope_registers.go` 的交接单册多 `FindDeliveryHandoverMember`，另立 `HandoverReceptionMembership`；`ports.go` 的 `PendingIdentityLookup` 多 `FindRecheckableInHandoverNote`。
- NO 编排：新文件 `application/delivery_handover_note.go`（`DeliveryHandoverNoteHandler` 五个入口）；`recheck_pending_identity.go` 多 `RecheckByHandoverNote`；`receive_delivered_unit.go` 依赖多 `Handover`，收寄带交接单标识时落成员（`joinHandoverNote`）。
- NO postgres：`delivery_handover_note.go`（落结论先取交接单排他锁、来源请求键一列、按事实身份读成员）；新文件 `reception_handover_recheck_lookup.go`（按交接单找重核对象）、`delivery_scope_established_doorbell.go`（门铃）。
- NO inbox：新文件 `adapters/inbox/delivery_scope_established_consumer.go`（第四扇门与 `RecheckWithinEstablishedScope`）。
- NO HTTP：新文件 `adapters/http/delivery_handover_note.go`（五口、三种 Intake、答复）。
- NO 认证适配：新文件 `adapters/accessidentity/delivery_handover_authenticator.go`（`WorkFactAuthenticator.AuthenticateDeliveryHandoverFact`、`CustomerHandoverConclusionAuthenticator`）。
- 装配：新文件 `cmd/parcel-api/assemble_delivery_handover_note.go`；`cmd/parcel-api/assemble_reception.go`、`cmd/parcel-frontline-import/intake_import.go` 收寄依赖多交接单册；`cmd/parcel-dispatch/assemble_node_identity_recheck.go` 重核装配多一扇门。

### 各命令口

| 口 | 族与能力 | 载荷（封闭） |
|---|---|---|
| `POST /node-operations/delivery-handover-notes` 开立 | 作业事实登记，准入事实类型 `DELIVERY_HANDOVER_NOTE_OPENING` | `noteId`、`deliveringParty`、`claimedAccounts`（至少一个、不重复）、`openedAt` |
| `POST /node-operations/delivery-handover-assignments` 归入 | 作业事实登记，`DELIVERY_HANDOVER_NOTE_ASSIGNMENT` | `factId`、`noteId`、`unit`、`assignedAt` |
| `POST /node-operations/delivery-handover-closures` 关闭 | 作业事实登记，`DELIVERY_HANDOVER_NOTE_CLOSURE` | `factId`、`noteId`、`closedAt` |
| `POST /node-operations/delivery-handover-confirmations` 运营决定确认 | 运营决定，决定种类 `DELIVERY_HANDOVER_CONFIRMATION` | `noteId`、`customerAccount`、`deliveringParticipant`、`basis` |
| `POST /node-operations/delivery-handover-conclusions` 客户确认或否认 | 首方客户渠道，客户面能力 `DELIVERY_HANDOVER_CONFIRMATION`（铸造侧判准入） | `accessRequestId`、`noteId`、`conclusion`（`CONFIRMED` / `DENIED`） |

租户、节点（作业范围引用，种类只认 `NODE`）、提交操作者（写作 `OPERATOR/<发行方>#<sub>`）、客户账户、客户端与来源请求键只从各自的信封来；自报即未知字段，答 400。交接单标识、事实身份与业务时间照 ADR-0023 由设备签发、从载荷如实收；结论时刻取服务端时钟。

### 答复各格

- **信封格**：作业事实三口照收寄口（ADR-0149 决定二）：`403 ACCESS_CHANNEL_NOT_CONFIGURED`、`401 OPERATOR_CREDENTIAL_REJECTED`、`403 OPERATOR_NOT_GRANTED`、`403 DEVICE_NOT_IN_USE`、`403 OUTSIDE_ADMISSION_SCOPE`、`503 IDENTITY_DEPENDENCY_UNAVAILABLE`，头缺或作业范围种类不是 `NODE` 答 `400 MALFORMED_REQUEST`。运营决定口照 ADR-0151 决定三与 ADR-0100 决定四：未配置、`401 OPERATOR_CREDENTIAL_REJECTED`、`403 OPERATOR_NOT_GRANTED`（含只持别的决定种类）、`403 OUTSIDE_ADMISSION_SCOPE`、503。客户口照 ADR-0139 决定四：未配置、`401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`、`403 CUSTOMER_CLIENT_NOT_GRANTED`（含只持预报或委托提交授予）、`403 OUTSIDE_ADMISSION_SCOPE`、503、缺接入请求标识答 400。别族令牌：作业事实与运营决定口答 `401 OPERATOR_CREDENTIAL_REJECTED`，客户口答 `401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED`。信封各格都不带 `outcome`、认证没过不读载荷。
- **业务格**（UC-NO-006 结果语义契约的线上名）：`DELIVERY_HANDOVER_NOTE_OPENED`、`UNIT_ASSIGNED`、`DELIVERY_HANDOVER_NOTE_CLOSED`、`BINDING_CONFIRMED`、`BINDING_DENIED` 答 201；`EXISTING_RESULT`（已有结果）、`EXISTING_CONCLUSION`（已有结论）、`SOURCE_CONFLICT`、`HANDOVER_NOTE_STATE_MISMATCH`（交接单状态不符，另带信息格 `stateMismatch`：`NOTE_CLOSED` / `NOTE_NOT_CLOSED` / `UNIT_NOT_ASSIGNABLE`）、`NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP`、`AUTHORIZED_DELIVERY_NOT_CONFIGURED` 答 200；交接单看不见（不存在、不是作业范围那个节点开立的、客户账户不在声称之列）答 `404 DELIVERY_HANDOVER_NOTE_NOT_VISIBLE`；拒形答 400；读写不动、PC 读不动、门铃入不了队答 `500 NO_ANSWER_FORMED`。「可验证交付范围已成立」不另立 outcome，随答复带 `scopeEstablishedAt`（声称的客户账户全部确认时最后一个确认的时刻）。
- **响应体**：操作者面四口回显 `handoverNote`（标识、节点、交付方线索、声称、开立时刻、关闭事实身份与时刻）、`member`、`binding`（运营决定的另带交付方参与方与依据引用）、`scopeEstablishedAt`；客户口只回 `noteId` 与信封里那个账户的 `binding`，不回声称了谁、范围成立没有。

### 两路确认

- **运营决定**：交接单看不见或账户不在声称之列答 404 → 这个账户已有结论即按结论答已有结果或已有结论、不问 PC → 问授权交付判断（租户，交付方参与方，客户账户，开立节点，开立时刻）：是即落确认（途径 `OPERATION_DECISION`、作结论方是信封上的操作者、带交付方参与方与依据引用）；否答 `NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP`、未配置答 `AUTHORIZED_DELIVERY_NOT_CONFIGURED`，都不落结论；读不动交回 error、不形成答案。不限于已关闭的交接单；交付方是账户所属参与方本身时 PC 答是。
- **客户渠道**：账户只取信封，不在声称之列与查无此单同答 404 → 交接单未关闭答状态不符（`NOTE_NOT_CLOSED`）→ 已有结论按结论答 → 落确认或否认（途径 `CUSTOMER_CHANNEL`、作结论方是客户端、来源请求键随结论落册）。
- **结论代数**：（交接单，客户账户）至多一个结论；同结论重投答 `EXISTING_RESULT`，异结论答 `EXISTING_CONCLUSION`，都回显在册那一份。两路共用一张结论表，客户先确认、运营决定再来确认也答已有结果。

### 范围成立门铃

- **触发**：最后一个声称的客户账户得到确认的那一笔，同一笔事务自投（租户，交接单）一封；归入一张范围已成立的交接单的那一笔，为那件实物在开立节点上的当前有效版本各投一封（租户，交接单，判断版本）。任一账户否认，范围不成立，不投。落结论取交接单那一行的排他锁、落成员取共享锁：并发的几笔确认排成一队，恰有最后一笔看到成立；归入与最后一笔确认之间，要么归入那一笔看到成立替这一件响门铃，要么确认那一笔按交接单重核时读得到这个成员。
- **信封**：事件类型 `node-operations.delivery-scope.established`（`nopostgres.DeliveryScopeEstablishedEventType`，消费方 `noinbox.DeliveryScopeEstablishedEventType` 自写一份同字面，组合根一条用例钉住）；ID 是（租户，交接单）或（租户，交接单，判断版本）的定长指纹，口名 `delivery-scope-established` / `delivery-scope-established-unit`；分区键是（租户，交接单）的定长指纹（口名 `delivery-scope-established-note`），同一张交接单的门铃排一条队；Subject 是交接单标识；载荷只有 `tenantId`、`handoverNote`、归入那一种另带 `judgmentVersion`。
- **消费**：重核消费者 `node-operations/recheck-pending-identities` 的第四扇门。只指名交接单的走 `RecheckByHandoverNote`（名下收寄与归入的成员里，开立节点上当前有效版本属于重核对象——待识别、不带关联、不记冲突或来由是只因歧义信号、来源已保全——的各件），带判断版本的走 `Recheck`。判法是收寄与重核同一份身份判断（票 11），范围按每一件存下的依据与此刻的证据重算。身份核对读不动照别的门铃记未决。
- **路由行**：`cmd/parcel-dispatch/assemble.go` 直投路由表 `noinbox.DeliveryScopeEstablishedEventType: identityRechecks,`；`assemble_node_identity_recheck.go` 的 `recheckGates` 多一行 `noinbox.DeliveryScopeEstablishedEventType: established,`。

### PC 授权交付判断的装配

`buildDeliveryHandoverNoteOrchestration`（`cmd/parcel-api/assemble_delivery_handover_note.go`）：`pcpostgres.NewAuthorizedDeliveryJudgments(db)` → `nopc.NewAuthorizedDeliveryView`（票 10 的防腐适配器）→ `DeliveryHandoverDeps.AuthorizedDelivery`。PC 读不动原样上抛，编排不形成答案、事务回滚。

### 逐口核（ADR-0151 决定三）

运营决定确认口凭领域自带的结论代数：重放身份是（租户，交接单，客户账户）这一主键，同结论答已有结果、异结论答已有结论，不另造规范化摘要。作业事实三口凭设备签发的事实身份（交接单标识、归入与关闭的事实身份）认重放、按内容比对分已有结果与来源冲突。客户口的来源请求键随结论落册作审计，不作重放身份。

### 共享文件里加的确切行（只追加，装配另起新文件）

- `cmd/parcel-api/endpoints.go`：`assembleBusinessEndpoints` 参数表末尾 `deliveryHandoverNotes deliveryHandoverNoteFaces,`；端点表末尾三行注释加五行 `{Pattern: "/node-operations/delivery-handover-notes", …NewOpenDeliveryHandoverNoteEndpoint(deliveryHandoverNotes.openingIntake(), deliveryHandoverNotes.handler)}`、`…/delivery-handover-assignments`（`NewAssignToDeliveryHandoverNoteEndpoint`）、`…/delivery-handover-closures`（`NewCloseDeliveryHandoverNoteEndpoint`）、`…/delivery-handover-confirmations`（`NewConfirmDeliveryHandoverEndpoint(operatorDecisions.nodeOperations, …)`）、`…/delivery-handover-conclusions`（`NewConcludeDeliveryHandoverEndpoint(deliveryHandoverNotes.conclusionIntake(), …)`）。
- `cmd/parcel-api/main.go`：`deliveryPreAdvices` 那一段之后加两行注释与 `deliveryHandoverNotes, err := buildDeliveryHandoverNoteFaces(db, operatorVerifier, operatorRegistry, deviceRegistry, admission, customerClientMinter)` 一段；`assembleBusinessEndpoints(` 实参末尾 `deliveryHandoverNotes,`。
- `cmd/parcel-api/endpoints_test.go`：`businessEndpointProbes` 末尾五行（五条路径）；`assembleUnwiredBusinessEndpointsWithOperatorIntakes` 实参末尾 `deliveryHandoverNoteFaces{handler: unwiredDeliveryHandoverNotes{}},`（占位类型在 `assemble_delivery_handover_note_test.go`）。
- `cmd/parcel-api/assemble_operator_decisions_test.go`：`swappedDecisionFaces` 末尾一行注释加 `"/node-operations/delivery-handover-confirmations": accessidentity.DecisionDeliveryHandoverConfirmation,`。
- `cmd/parcel-dispatch/assemble.go`：直投路由表第二段末尾 `noinbox.DeliveryScopeEstablishedEventType:                identityRechecks,`。
- `internal/accessidentity`：`operation_decision.go` 常量一段与 `decisionKinds` 末尾一项；`customer_client.go` 常量两行注释加一行、`customerCapabilities` 末尾一项；`customer_client_pre_advice.go` 的 `customerCapabilitiesAdmittedAtMint` 末尾一项。
- `scripts/demo-seeds/seed.sh`：`governance/36` 那一行之后两行注释加五行 `authority-interval` 登记（37 至 41）。
- `internal/architecture/partition_subject_registry_test.go`：节点作业那一段之后另起一段（一行注释加一行）登门铃文件，主体「租户/送站交接单」；键比那一段各行都长，并进去 gofmt 会重排整段对齐。

### 迁移与种子号

access_identity `0016`、node_operations `0016`（`0014` 没占）、治理种子 `governance/37` 至 `41`（37 运营决定、38 客户面、39 至 41 作业事实三种；建前 `ls` 过空号）。

### 完成判据格 → 用例

| 格 | 用例 |
|---|---|
| `AT-NO-071` | 编排 `internal/nodeoperations/application/delivery_handover_note_test.go::TestOpeningAssigningAndClosingAHandoverNote`（开立、原样重投、换内容；收寄带它落收寄成员；归入落成员、不改写来源、不追加判断；同一归入事实重投与换实物；归入已识别、记多候选冲突、在这个节点没有收寄的实物答状态不符；关闭、重投、另一份关闭、同一关闭事实换时刻；关闭之后归入答状态不符；关闭之后与不在册的依据收寄不落成员）、`TestAHandoverNoteIsVisibleOnlyAtItsOwnNode`；存储 `internal/nodeoperations/adapters/postgres/delivery_handover_recheck_test.go::TestAMemberIsFoundByItsFactIdentity`；HTTP `internal/nodeoperations/adapters/http/delivery_handover_note_test.go::TestHandoverWorkFactsTranslateTheirPayloads`；真库 `cmd/parcel-api/assemble_delivery_handover_note_test.go::TestADeliveryHandoverNoteIsOpenedFilledAndConfirmedAgainstARealDatabase` |
| `AT-NO-072` | 编排 `TestAnOperationDecisionConfirmsOnlyWithinTheAuthorizedDeliveryRelationship`（是、否、未配置、本人四格，问法逐格；读不动不形成答案；账户不在声称之列与查无此单同答、不问 PC）；真库同上那条（PC 真读口：交付方不是本人且一条关系都没登记答未配置、交付方是账户所属参与方本人答已确认） |
| `AT-NO-073` | 编排 `TestTheCustomerChannelConcludesOnlyOnAClosedNoteThatClaimsIt`；HTTP `TestHandoverLinesMapTheirOutcomes`（各格状态码、客户面不回显声称）、`TestTheConfirmationAndConclusionIntakesTakeIdentityFromTheEnvelope`；铸造 `internal/accessidentity/delivery_handover_test.go::TestTheHandoverConfirmationLineMintsItsEnvelopeFromTheBinding`；认证 `internal/nodeoperations/adapters/accessidentity/delivery_handover_authenticator_test.go::TestTheCustomerHandoverAuthenticatorTranslatesTheEnvelopeAndEveryGrade` |
| `AT-NO-074` | 编排 `TestAScopeNeedsEveryClaimConfirmedAndADenialFreesTheMembers`；并发 `TestTheLastTwoConfirmationsSerializeOnTheNote`；另 `TestAssigningIntoAnEstablishedNoteRingsForThatUnit`（归入已成立的交接单那一封） |
| `AT-NO-075` | parcel-api 半边 `TestADeliveryHandoverNoteIsOpenedFilledAndConfirmedAgainstARealDatabase`（经 `buildReceptionOrchestration` 与 `buildDeliveryHandoverNoteOrchestration`：最后一个确认那一笔门铃恰一封、重投不多）；parcel-dispatch 半边 `cmd/parcel-dispatch/node_identity_recheck_scope_established_test.go::TestTheWiredDispatchRechecksAHandoverNoteOnceItsScopeIsEstablished`（经 `wireDispatcher`：门铃 → 重核追加关联版、前版、原收寄时刻、控制依据不变、核对态已证、意图恰一封；名下已关联与记多候选冲突的两件不追加）、`TestAScopeEstablishedDoorbellReachesTheRecheckConsumerThroughTheRouteTable`、`TestTheRecheckConsumerAndTheScopeDoorbellAgreeOnTheEventType`；编排 `internal/nodeoperations/application/recheck_by_handover_note_test.go::TestARecheckByHandoverNoteRechecksOnlyItsRecheckableMembers`；存储 `TestRecheckableMembersAreFoundByTheirHandoverNote`、门铃 `delivery_scope_established_doorbell_test.go` 两条；消费 `internal/nodeoperations/adapters/inbox/delivery_scope_established_consumer_test.go` 三条 |
| 信封各格 | 作业事实 `cmd/parcel-api/assemble_delivery_handover_note_test.go::TestHandoverWorkFactFacesAnswerOnTheOperatorChannel`（未配置、无令牌、只持查阅读、设备不在用、不在准入范围、齐备走到编排）；三族与别族令牌 `TestHandoverLinesRefuseOtherFamiliesAndJudgeAdmission`；运营决定一族 `assemble_operator_decisions_test.go::TestSwappedDecisionFacesRefuseAGrantOfAnotherDecisionKind`、`TestSwappedDecisionFacesAnswerFromTheOperatorChannel`（多一行）；路由 `endpoints_test.go::TestEveryAssembledEndpointAnswersUnconfigured`（多五行）；传输层 `TestHandoverLinesAnswerTheEnvelopeGrades`、`TestHandoverLinesAnswerUnconfiguredAndKeepTheMethodGate`、`TestHandoverLinesRefuseMalformedPayloads`；认证 `TestTheHandoverWorkFactAuthenticatorAsksForEachFactsAdmission`；铸造与词 `internal/accessidentity/delivery_handover_test.go` 其余四条；库面 CHECK `internal/accessidentity/adapters/postgres/delivery_handover_grant_test.go` 两条 |
| 领域与库面新格 | `internal/nodeoperations/domain/delivery_handover_conclusion_test.go` 两条；`TestACustomerConclusionKeepsItsSourceRequestKey` |
| 演示实走 | 见下「演示实走」 |

### 先红后绿

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm12-red.log`。用例先写、生产代码一行未改（钉 `d59ba763`），十一个包 `[build failed]`，现象原话是「undefined: accessidentity.DecisionDeliveryHandoverConfirmation」「minter.MintDeliveryHandoverConfirmation undefined (type *accessidentity.CustomerClientMinter has no field or method MintDeliveryHandoverConfirmation)」「undefined: nodeopshttp.DeliveryHandoverFact」「authenticator.AuthenticateDeliveryHandoverFact undefined」一类，日志里这类行 89 条，各包报到「too many errors」即止；`internal/architecture` 另红一条原话「登记表里的 internal/nodeoperations/adapters/postgres/delivery_scope_established_doorbell.go 已不构造带 PartitionKey 的信封」。`AT-NO-075` 两条真库用例在这一步同属编译即红（门铃、编排与装配函数都还没有）。
- 实现后另做过一次反证：把落结论那一处的排他锁临时去掉，`TestTheLastTwoConfirmationsSerializeOnTheNote` 红（原话「先一笔看到成立 true、后一笔 false, want 只有后一笔看到」），恢复后绿，没提交那次改动。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm12-green.log`，钉 rebase 后的 `27a3bd97`。

### 门

rebase 到 `8e5125c2` 之后：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 55432 DSN 以 `-p 1 -count=1` 跑 `./internal/accessidentity/... ./internal/nodeoperations/... ./internal/partycommercial/... ./cmd/... ./migrations/... ./internal/architecture/... ./scripts/demo-seeds/...`，有测试的包 40 个 ok、0 FAIL（含 `cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-access-register`、`cmd/parcel-frontline-import`）。`1ad7bca9` 只加编译期断言，之后补跑了 build、vet 与 `internal/nodeoperations/adapters/postgres`、`internal/architecture`。没跑全量 `./...`，留给推送方。机制清点在 `1ad7bca9` 上重生成（`68ad8bef`）：access_identity 迁移 15→16、node_operations 14→15，接入面端点 159→164，直投路由表 33→34，端口声明 530→533，基线口径缺口不变。

### 演示实走（`S`）

一次性库 `psxm12_demo`（门禁库服务器上建、用完删），`seed.sh` 退 0，送站交接确认两格授予、五行对照、五条区间都在册。共享的集成客户端发行方仍答 `invalid_client`，另起私有 Dex（同一份 `integration-dex.yaml`，发布在 `127.0.0.1:5564`，issuer 串仍是 `http://127.0.0.1:5558/dex`），用完删容器；操作者发行方用共享的 5556（只取令牌）。在 `4a9ac524` 上编出的 `parcel-api`（19212）与 `parcel-dispatch`：`submit-one-shipment.sh` 一单经 dispatch 到`已接受`；不带预报送一件同号，答 `200 PENDING_IDENTIFICATION`、`UNVERIFIED/NO_BASIS`；开交接单 `201`、归入 `201`、关闭 `201`、关闭后再归入 `200 … / NOTE_CLOSED`；运营决定指名无关系的交付方答 `200 NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP`，指名种子里有授权交付关系的 `SYN-PARTY-DELIVERER-01` 答 `201 BINDING_CONFIRMED` 带 `scopeEstablishedAt`；门铃一封经 dispatch 投给重核消费者，册上追加 `IDENTITY-RECHECK/<原版本>` 一版 `INTAKE_FORMED`、关联那件包裹、接收时刻与控制依据照原收寄、核对态 `VERIFIED`，收寄意图一封投出——识别了。客户口另走一遍：未关闭答 `NOTE_NOT_CLOSED`、关闭后确认 `201`（落来源请求键）、改口否认 `EXISTING_CONCLUSION`、不存在的 404、操作者令牌 401。答复原文在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm12-walk-handover.log`、`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm12-walk-customer.log`、`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm12-walk-db.log`，种子日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm12-seed-run.log`；记录也写进演示文档「送站交接单」一段。走完已停自己起的两个进程、删私有 Dex 容器、删库；`psb26_walk` 未碰，别的进程与容器未碰。

### 评审阻断的修复（2026-10-19，会话 idp-parcel-9b 代决，授权同票面）

- **裁定**：以送站交接单为依据推可验证交付范围时，这件作业实物须是该交接单的成员（收寄带来的或归入的都算）；不是成员答「依据不适用」，细分新加 `NOT_A_HANDOVER_MEMBER`（`domain.ScopeNotAHandoverMember`）。交接单未关闭时，带它标识的收寄先落收寄成员、再推导，成员即成立；关闭之后落不进，推导答不是成员。重核按此刻的成员册同一判法。判法与理由在 ADR-0212 文末复核记录（`28668fae`），决定原文不动。
- **推导**：`domain.DeliveryScopeDerivation` 多一格 `HandoverMember`；`deriveHandoverNoteScope` 在节点之后、绑定之前判它——不是成员时补齐绑定也补不出范围，恢复动作是归入一张开放的交接单，先报这一格才把人指对。开放与关闭都按同一条判（不是成员即不适用）：开放期间带单号收进来的件已由编排先落成员，走到推导时仍不是成员的，只有这一条之前没落成员的旧行。
- **成员信息怎样交进来**：范围证据读口 `ports.DeliveryScopeEvidence` 多一问 `IsDeliveryHandoverMember(租户，交接单，作业实物)`，身份判断（`identityJudge.deriveScope`）在交接单在册时问一次、填进推导。不选「编排先查」：收寄与重核共用这一份身份判断，放在判断里两处自然同一判法；也不选「交接单证据状态带整份成员清单」：一张交接单名下可以有很多件，推一件只要这一格。生产上是交接单册 `DeliveryHandoverNotes.IsDeliveryHandoverMember`（走成员表按交接单的索引，`ReadExecutor` 读得到同一笔事务里刚落的收寄成员），`DeliveryScopeEvidence` 转交。
- **收寄次序**：`receive` 在身份判断之前调 `joinHandoverNote`（原先在落册之后）；它只依赖命令里的租户、来源标识、实物与收寄时刻。同一来源重投或并发输给另一方，撞的是同一个（收寄，来源标识），成员不多一个。锁次序不变：收寄先取实物锁、再取交接单行的共享锁，与归入同序；确认只取交接单行的排他锁。
- **库面**：`node_operations/0016` 改名为 `0016_delivery_handover_binding_source_and_member_inapplicability.sql`（本票之前没进 main），追加重建 `reception_delivery_scope_inapplicability_closed`：以 0015 的全集为底加 `NOT_A_HANDOVER_MEMBER`，头注写「之后谁再重建取并集」。
- **用例**：
  - 领域 `internal/nodeoperations/domain/delivery_scope_membership_test.go::TestAHandoverNoteScopeReachesOnlyItsMembers`（已关闭交接单的成员已证、非成员不适用·不是成员、开放交接单的非成员同答、绑定没全部确认的非成员先报不是成员；新细分线上名往返、只挂在依据不适用上）。
  - 编排 `internal/nodeoperations/application/delivery_handover_note_test.go::TestAReceptionCarryingAClosedNoteItIsNotAMemberOfIsNotApplicable`（范围已成立、开放期间带它收寄的那件已证、关联；关闭之后带它收寄的另一件不关联、依据不适用·不是交接单成员、成员不多；那一件重核判断不变）。
  - 存储 `internal/nodeoperations/adapters/postgres/delivery_handover_recheck_test.go::TestMembershipIsAnsweredPerNoteAndUnit`（两种成员、非成员、别的交接单、别的租户；范围证据读口同答；新细分过得了库面 CHECK、原样读回）。
  - 真库 `cmd/parcel-api/assemble_delivery_handover_note_test.go::TestAReceptionCarryingAClosedNoteIsNotApplicableUnlessItIsAMemberAgainstARealDatabase`（经 `buildReceptionOrchestration`、`buildDeliveryHandoverNoteOrchestration` 与 `buildIdentityRecheckOrchestration`：开放期间带单号送站 `INTAKE_FORMED`、`VERIFIED//`；关闭之后另一件 `UNVERIFIED/BASIS_NOT_APPLICABLE/NOT_A_HANDOVER_MEMBER`、不关联、成员仍只一个；人工重核 `STILL_UNIDENTIFIED`）。
  - 随新规则改的既有用例（不改断言）：领域 `handoverDerivation` 夹具与「最后归入优先」那一条补 `HandoverMember: true`，细分词表那一条多一格；编排的范围证据替身多答成员一问（读同一夹具的交接单册替身与 `assigned`）；票 11 的 `TestARecheckRederivesTheScopeFromTheStoredBasisAndTheEvidenceNow` 补一行夹具（交接单也登在交接单册替身上，收寄才落得进成员）；HTTP 的 `noScopeEvidence` 多一问。
- **先红后绿**：红追加在原红日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm12-red.log` 末尾「评审阻断」一段，钉 `7d9c8c21`、工作树只有测试改动。领域与存储两包编译即红（「stranger.HandoverMember undefined」「undefined: domain.ScopeNotAHandoverMember」「fixture.notes.IsDeliveryHandoverMember undefined」）；行为红两条，正是这个漏洞：编排「关闭之后带它收寄：INTAKE_FORMED 冲突 false 核对态 VERIFIED//, want 不关联的待识别、依据不适用·不是交接单成员」，parcel-api 真库「outcome = INTAKE_FORMED, want PENDING_IDENTIFICATION」。绿 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm12-green-fix.log`。
- **门**（钉 `98866b09`，`origin/main` 仍是 `8e5125c2`，无需再 rebase）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN 以 `-p 1 -count=1` 跑 `./internal/accessidentity/... ./internal/nodeoperations/... ./internal/partycommercial/... ./cmd/... ./migrations/... ./internal/architecture/... ./scripts/demo-seeds/...`，有测试的包 40 个 ok、0 FAIL（`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-frontline-import`、`cmd/parcel-access-register` 都在其中）。票 11 的两份真库夹具（`confirmedHandoverNoteOnRealDatabase`、`confirmedHandoverNoteForDispatch`）开的交接单都没关，收寄带单号先落成员，照旧答已证、不用改。
- **机制清点**：`98866b09`，nodeoperations 测试文件多一份，其余不变。
- **没重走演示**：演示动线里的交接单归入的是不带依据的那一件（成员经归入），关闭之后不再带单号收寄，答复不变；改动的那一格由真库用例钉住。

### 代决

1. **两处字面量都取 `DELIVERY_HANDOVER_CONFIRMATION`**（运营决定种类与客户面能力），不取票面建议的 `HANDOVER_NOTE_CONFIRMATION`：与 `DELIVERY_PRE_ADVICE_*` 同一族名，UC 与 ADR 的词是「送站交接确认」，派单也这样建议。
2. **作业事实登记面判准入只从交接单三种事实起**：能力词取能力面本名 `WORK_FACT_REGISTRATION`（同运营决定口取 `OPERATION_DECISION` 的取法），事实类型 `DELIVERY_HANDOVER_NOTE_OPENING` / `…_ASSIGNMENT` / `…_CLOSURE`；词表只收在铸造侧判准入的那几种。收寄口与运输履约各口照旧不判，没有顺手接上。
3. **收寄带交接单标识即落收寄成员**（事实身份取收寄来源标识、业务时间取收寄时刻），只在明确接收那一支、在身份判断之前落；交接单不在册或已关闭时不落、收寄照常，推导随后答依据不在册或不是交接单成员。没有它，门铃按成员找重核对象时找不到带着交接单收进来的待识别；先落再推导，是评审阻断那条判法要的次序（见「评审阻断的修复」）。
4. **归入条件**：在交接单开立节点上有当前收寄（形成或待识别的判断链），且每一条都属于重核对象；有一条已关联即不能归入。作业事实信封的节点不是交接单开立节点时答查无此单（404），与越权探针同答。
5. **重放与冲突**：开立按交接单标识比内容（节点、交付方线索、声称、开立时刻）；归入按（归入，事实身份）比交接单、实物与业务时间；关闭同一事实身份同时刻答已有结果、异时刻答来源冲突，另一份关闭事实到来时答已有结果并回显第一份（关闭是终态，同预报撤回的先例）。
6. **结论代数只比结论**：（交接单，客户账户）已有结论时，同结论答已有结果、异结论答已有结论，不比途径、交付方参与方或依据；已有结论时不再问 PC。客户口的来源请求键不作重放身份（随结论存作审计）。
7. **运营决定指名没声称的账户答 404**（与查无此单同答），不另立一格。
8. **「交接单状态不符」另带信息格 `stateMismatch`**（三值），不加 outcome：三种恢复动作不同。
9. **客户口答复只回自己那一个账户的结论与交接单标识**，不回声称与范围成立：两者都会让一个客户账户看到别的账户在不在、确认没有。
10. **结论表多一列来源请求键**（node_operations 0016），客户渠道必带、运营决定必不带；领域构造门同规则。
11. **落结论先取交接单那一行排他锁**，与加成员的共享锁互斥：最后一笔确认恰有一笔看到成立，归入与确认之间不漏门铃。
12. **门铃一种事件类型、两种载荷**（带不带判断版本），分区按交接单；路由表只多一行。
13. **范围成立时刻派生**：声称账户全部确认时取最后一个确认的时刻，不落列。
14. **路径**：`/node-operations/delivery-handover-notes`、`…-assignments`、`…-closures`、`…-confirmations`、`…-conclusions`，随本上下文命令口前缀，一口一个子资源。

### 越权风险点（待 owner 复核）

1. **关闭之后带交接单标识的收寄仍按它成立范围——已裁已修**：评审裁为阻断，会话 `idp-parcel-9b` 代决在本票修（`afd7c005`），判法写进 ADR-0212 文末复核记录：以交接单为依据推范围，这件实物须是它的成员，不是成员答依据不适用·`NOT_A_HANDOVER_MEMBER`。见「评审阻断的修复」。
2. **作业事实登记面只有交接单三种判准入**（代决 2），收寄口不判：同一能力面两种口径并存。要统一，收寄口接准入随另票，演示种子同笔补收寄的对照与区间。
3. **字面量与票面建议不同**（代决 1）：管理台（票 13）与运营授予表照 `DELIVERY_HANDOVER_CONFIRMATION`。
4. **运营决定先于客户确认时，客户再否认答已有结论**：确认是终态（ADR-0212 决定三），客户事后不认这次送站只能走别的路。这是 ADR 原句的直接后果，列出来是因为客户面上第一次出现「别人替我确认过，我否认不了」。
5. **归入了别的交接单的实物仍在原交接单名下被重核**：按交接单重核时交回成员里属于重核对象的每一件，不看它最后归入的是不是这一张；重核按它的有效依据判，原交接单的范围对它不起作用、判断不变。只是多一次身份核对读。

### 越出地盘的改动

- `internal/nodeoperations/domain/delivery_handover_note.go`（及新用例 `delivery_handover_conclusion_test.go`）：结论多来源请求键、范围成立时刻（代决 10、13）；票面地盘没列领域。
- `internal/nodeoperations/ports/`：新文件 `delivery_handover.go`，`delivery_scope_registers.go` 与 `ports.go` 各多一问。
- `internal/nodeoperations/adapters/accessidentity/delivery_handover_authenticator.go`（及用例）：两族认证适配；票面地盘没列。
- `cmd/parcel-frontline-import/intake_import.go`：收寄依赖多交接单册（收寄编排构造期要它）。
- 既有用例随新依赖与新格改（不改断言）：`receive_delivered_unit_test.go`（替身多交接单册）、`dispose_identity_conflict_test.go`、`adapters/http/receive_delivered_unit_test.go`（新替身 `noHandoverMembership`）、`adapters/http/work_fact_intake_test.go`、`cmd/parcel-dispatch` 三份收寄夹具与 `delivery_scope_fixture_test.go`（`handoverMembershipFor`）、`cmd/parcel-frontline-import/intake_handoff_continuation_test.go`；客户渠道结论补来源请求键：`domain/delivery_handover_note_test.go`、`application/delivery_scope_judgment_test.go`、`adapters/postgres/delivery_scope_registers_test.go`、`cmd/parcel-api/delivery_scope_fixture_test.go`、`cmd/parcel-dispatch/delivery_scope_fixture_test.go`。
- `scripts/demo-seeds/README.md`、`docs/design/synthetic-demo-journey-script.md`：授予表、首方客户渠道第 8 项与「送站交接单」一段。
- `docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md`：文末追加一条复核记录（评审阻断的判法，派单要求），决定原文不动。
- 没碰 `docs/adr/README.md`、ADR 决定原文、NO 节点控制那几格（`reception.go` 读控制处、复核目录注册）。

### AT 盘点

`.scratch/at-coverage-inventory.md` 追记十七：`AT-NO-071`、`AT-NO-072`、`AT-NO-073`、`AT-NO-075` 由②入①，`AT-NO-074` 行内补命令面与并发面。①338→342、②228→224（NO 43→47、25→21），合计不变。列进①的各条用例 Covers 注释都点了 AT 号，扫描口径认得出。评审阻断修复之后，`AT-NO-071` 那一行补上编排与真库两条非成员用例，桶数不变。

### 未做

- 完成判据末格「走 Go 那条路」：分支与作者自验已做；非作者评审与推送方在干净检出上带 DSN 跑全量不归作者，该格未勾。
- 票面「不做」照旧：管理台交接单页与确认按钮（票 13）、客户查看交接单明细的读口、前线导入模板的交接单行、运营决定否认。
- 收寄口没接准入范围（越权风险点 2）。
- 演示脚本没另立：实走用 curl，载荷与答复写在演示文档那一段。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，首轮钉 `7d9c8c21`，修补后钉 `8a593f23`）

- **首轮阻断（已修，`afd7c005`）**：交接单关闭之后，带它单号送站的非成员实物仍按推导拿到这张交接单的范围，绕过了客户对关闭时冻结的成员清单所作的确认。推送方代决：以交接单为依据须是成员，否则答依据不适用·`NOT_A_HANDOVER_MEMBER`；收寄改为先落收寄成员再推导，重核同一判法；`node_operations/0016` 重建细分 CHECK 加新词；ADR-0212 文末追加复核记录。编排与经生产装配的真库用例先红（原话 `INTAKE_FORMED`，want 待识别）后绿。
- **Spec 其余**：ADR-0212 的交接单三步与两路确认落到。作业事实登记三口只在 NODE 作业范围、按三种事实判准入；运营决定确认先看已有结论再问 PC 授权交付判断（问句含开立节点），答否或未配置不落结论、读不动不形成答案；客户渠道只对已关闭、声称了自己账户的交接单下结论，余者同答 404；结论按（交接单，客户账户）只比结论。最后一个账户确认时与归入已成立的交接单时各交「交付范围成立」门铃，落结论取交接单行排他锁、并发确认恰一笔看到成立（反证实测）。重核按交接单只重核可重核的成员。
- **作者自定的格认可**：两处字面量取 `DELIVERY_HANDOVER_CONFIRMATION`（与预报一族同名）；收寄带交接单标识即落收寄成员（不落则门铃找不到这件）。
- **与另一会话的共享文件**：`cmd/parcel-api` 四份、`cmd/parcel-dispatch/assemble.go`、`internal/accessidentity` 枚举、`seed.sh` 只追加；access_identity 0016 以 main 上全集为底、治理种子 37–41。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-psxm12`（作者已 rebase 到 `8e5125c2` 并修评审阻断）直接叠在远端 main 之上快进推送；机制清点重生成零差。
- 推送方全量一次：钉 `ec3dc885` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 152 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
