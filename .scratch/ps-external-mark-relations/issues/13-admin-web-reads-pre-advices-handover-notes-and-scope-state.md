# 13 管理台读面：送站预报与送站交接单的查阅，待识别实物显示交付范围核对态，交接单详情带「确认绑定」动作

Category: enhancement
Status: resolved——2026-10-19 进 main（代码验证钉 `b7bf59e8`），见「进 main 记录」。此前 in-progress——2026-10-19 评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（09、11、12 已进 main），分支 `claude-psxm13`，走 Go 那条路。此前 ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by:
- [09](./09-pre-advice-submission-on-the-first-party-customer-channel.md) 进 main：预报要先有来路，读面才有东西可看。
- [11](./11-identity-check-judges-the-verifiable-delivery-scope.md) 进 main：交付范围核对态落册之后，复核目录才带得出它。
- [12](./12-handover-notes-are-opened-filled-closed-and-confirmed.md) 进 main：交接单与「送站交接确认」运营决定口由它落。
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定五（核对态是答复与复核目录里的信息格）；[UC-NO-006](../../../docs/application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md) 结果语义契约与「权限、隔离与审计」；UC-NO-002 结果语义契约之后讲交付范围核对态的那一段。
地盘：
- `internal/nodeoperations/adapters/http`：运营查阅读的预报列表与详情、交接单列表与详情（新文件），及测试
- `internal/nodeoperations/ports/catalogue_read.go`、`adapters/postgres/review_catalogue.go`：复核目录带交付范围核对态、未证原因与依据引用；预报与交接单的查阅读端口与实现
- `cmd/parcel-api`：端点表、装配
- `apps/admin-web/src/pages/operations/**`：预报页、交接单页、待识别实物页的核对态一栏、交接单详情的「确认绑定」动作
- `.scratch/at-coverage-inventory.md`

## 做什么

1. **查阅读口**（运营查阅读能力面）。
   - 预报：按节点与预计送达日列，详情给号码集合、客户账户、撤回与否。
   - 交接单：按节点与开立日列，详情给声称的客户账户、每个账户的绑定结论与途径、成员与各自形成方式、是否关闭、范围是否成立。
   - 跨租户与查无同答。
2. **复核目录**。待识别实物多带三格：交付范围核对态（已证、未证附原因、未配置）、依据引用、此刻归入的交接单（若有）。管理台待识别实物页显示这几格，让人分得出「缺范围」「范围成立但冲突」「零候选」与「未核对」。
3. **确认绑定动作**。交接单详情对每个待确认的客户账户给「确认」按钮：选交付方参与方（读 PC 的参与方目录）、填依据引用，提交到票 12 的运营决定口。答复各格照实呈现，「未配置」「不在授权交付关系内」写明恢复动作。
4. **走 Go 那条路**：随本票分支、非作者评审与推送方全量，不走前端切片的 main 直做（同 no-reception-identity/06 的先例）。admin-web 三道门照 [workflow 本机环境](../../../docs/agents/workflow.md#本机环境) 的口径。

## 不做

- 交接单的开立与归入（作业事实登记，属一线作业端）。
- 客户面读口。
- 预报有效时窗与授权交付关系的登记页（各自登记写面已在票 08、10；管理台页另立，若需要）。

## 完成判据

- [x] 读口用例：预报与交接单的列表与详情，跨租户与查无同答；未授予运营查阅读答其格。
- [x] 复核目录用例：交付范围核对态、未证原因、依据引用与归入的交接单都带出。
- [x] admin-web 三道门退 0；核对态判读、确认动作的载荷拼法与各格答复的呈现有用例（落在判读纯函数上，照 identity-disposition 的先例）。
- [x] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`internal/architecture/...` 全绿；机制清点重生成。
- [x] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

M

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-psxm13`）

起点 main `129e9a38`，收口前两次 rebase：先到 `ea61d108`（demo-chain-gaps/61、63、65、69 进 main 之后），再到 `0237cef3`（demo-chain-gaps/67 进 main 之后，只在机制清点上冲突，按新树重生成）。第一次那回 `cmd/parcel-api` 的 `endpoints.go`、`endpoints_test.go`、`main.go` 与出发前问询口在同一处追加，冲突两边都留、问询口在前；`review_catalogue.go`、`ports/catalogue_read.go`、`query_node_operations_records.go` 自动合并，待识别行多出的格与节点控制转移册并存。授权：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」；「代决」各条照此。权威：ADR-0212 决定三、五、七，UC-NO-006「权限、隔离与审计」与结果语义契约，UC-NO-002 结果语义契约之后讲交付范围核对态那一段，ADR-0100 决定四、ADR-0055、ADR-0022。没有立新 ADR：查阅读与管理台没遇到上述各篇没裁的难逆转取舍，实施取舍记在「代决」。证据层级 **S**：夹具全为 `SYN-` 合成值，隔离合成库上跑；没做演示实走。

### 提交

| 提交 | 内容 |
|---|---|
| `c5e64637` | Go：查阅读端口、postgres 读口、HTTP 四口、复核目录待识别行多两样依据、节点作业查阅册收寄行与待识别行带核对态各键、parcel-api 装配；各层用例 |
| `4aaa18f9` | 管理台：节点作业查阅页多送站预报、送站交接单两区，交接单详情的「确认」，待识别实物区多三列，重核答复带核对态；三份判读模块与用例 |
| `bdfe7557` | 机制清点重生成 |
| `f46cd310` | 只动注释：票面条目改写成条目名，领域词表与答复键不再写个数 |
| 本笔 | 本完成记录、AT 盘点追记十八 |

### 落点

- 端口：新文件 `internal/nodeoperations/ports/delivery_scope_catalogue_read.go`（`DeliveryScopeCatalogueRead`、`DeliveryScopeCatalogueFilter`、`DeliveryHandoverNoteCatalogueRow`、`DeliveryHandoverNoteDetail`）；`ports/catalogue_read.go` 的 `UnidentifiedItemCatalogueRow` 末尾多 `DeliveryScopeBasis domain.UnitDeliveryScopeBasis`。
- postgres：新文件 `adapters/postgres/delivery_scope_catalogue.go`（`DeliveryScopeCatalogue`；列表按页内标识再取撤回、关闭与结论，详情就是两本册自己的 `FindDeliveryPreAdvice`、`FindDeliveryHandoverNote` 加 `ListDeliveryHandoverMembers`；还原一律走两本册写侧同一组 scan；另有 `unitScopeBasisFrom`）；`review_catalogue.go` 的 `ListUnidentifiedItems` 只追加三列。
- HTTP：新文件 `adapters/http/query_delivery_scope_records.go`（四个端点构造、`DeliveryScopeCatalogueReader`、筛选读法与答复形状）；`query_node_operations_records.go` 的收寄行与待识别行带核对态各键（复用 `deliveryScopeCheckFields`），待识别行另带依据与归入。
- 装配：`cmd/parcel-api` 端点表、参数表、`main.go` 各追加（见下「共享文件里加的确切行」）。
- 管理台 `apps/admin-web/src/pages/operations/`：新文件 `delivery-scope-api.ts`（镜像）、`delivery-scope-check.ts`、`delivery-pre-advice.ts`、`delivery-handover-note.ts`（判读纯函数）与三份 `*.test.ts`，`DeliveryPreAdvicesSection.tsx`、`DeliveryHandoverNotesSection.tsx`、`DeliveryScopeFilters.tsx`（组件）；改 `NodeOperationsReviewPage.tsx`（两区 chip、待识别实物区三列）、`identity-recheck.ts` 与 `identity-recheck-api.ts`（行转写三格、重核答复带核对态与新一格）、`records-api.ts`（镜像多几格）。

### 各读口与答复

都挂主数据与运营查阅读（与 `/node-operations-records` 同一个 Intake，作用域只有租户），门的次序是方法 → Intake → 请求内容。

| 口 | 请求 | 答复 |
|---|---|---|
| `GET /node-operations-delivery-pre-advices` | 可选 `node`（逐字）、`expectedArrivalFrom`（闭）、`expectedArrivalUntil`（开），RFC 3339 | `200 DELIVERY_PRE_ADVICES_LISTED`，`preAdvices[]`，预计送达倒序、同刻按标识；空册答 `[]` |
| `GET /node-operations-delivery-pre-advices/{preAdviceId}` | 路径标识 | `200 DELIVERY_PRE_ADVICE_FOUND`，`preAdvice`；查无与别的租户的同名预报同答 `404 DELIVERY_PRE_ADVICE_NOT_VISIBLE` |
| `GET /node-operations-delivery-handover-notes` | 可选 `node`、`openedFrom`（闭）、`openedUntil`（开） | `200 DELIVERY_HANDOVER_NOTES_LISTED`，`handoverNotes[]`，开立倒序、同刻按标识 |
| `GET /node-operations-delivery-handover-notes/{noteId}` | 路径标识 | `200 DELIVERY_HANDOVER_NOTE_FOUND`，`handoverNote` 与 `members[]`（`factId`、`unit`、`membership`、`formedAt`，按落册先后）；查无与跨租户同答 `404 DELIVERY_HANDOVER_NOTE_NOT_VISIBLE` |

- 预报一份：提交口回显的那几格（`preAdviceId`、`customerAccount`、`node`、`expectedArrivalAt`、`customerParcelNumbers`、可缺的 `deliveringParty`、`formedAt`，取 `preAdviceBodyOf`），加 `withdrawn` 与撤回才有的 `withdrawnAt`。
- 交接单一张：`noteId`、`node`、`deliveringParty`、`openedAt`、`closed`（关闭才带 `closureFactId`、`closedAt`）、`memberCount`、`claims[]`、`scopeEstablished`（成立才带 `scopeEstablishedAt`，取领域 `ScopeEstablishedAt`）。`claims` 是声称的每个账户一格：没结论的只带 `customerAccount`；有结论带 `conclusion`、`route`、`concluder`、`concludedAt`，运营决定另带 `deliveringParticipant`、`basis`。
- 信封各格：未配置 `403 ACCESS_CHANNEL_NOT_CONFIGURED`、令牌不过 `401 OPERATOR_CREDENTIAL_REJECTED`、没有查阅读授予（含只持登记写）`403 OPERATOR_NOT_GRANTED`、身份依赖读不动 `503`；都不带 `outcome`、不读请求内容。筛选键封闭：未知键（含自报租户）、重复键、空白节点、读不成 RFC 3339、起点不早于终点都答 `400 MALFORMED_REQUEST`；详情空白标识答 400。读不动答 `500 NO_ANSWER_FORMED`。

### 复核目录多的格

- 待识别册（`/node-operations-records?registry=unidentified-item` 的 `items`）每行多：`deliveryScopeCheck`、`deliveryScopeUnverifiedReason`、`deliveryScopeInapplicability`（与收寄答复同名同值，这一版没记即缺席）；`deliveryScopeBasis`（这一版来源里上报的依据引用，`{kind, id}`，与收寄载荷同形，来源没带或未保全即缺席）；`assignedHandoverNote`（这件实物此刻最后归入的交接单，只看归入、业务时间倒序、同刻按落册先后，没归入过即缺席）。
- 收寄册（`registry=reception`）每行多核对态各键：票 11 落了端口那一格、HTTP 归本票。
- 两样依据分开给，不替看的人取「此刻起作用的那一份」：核对态记的是做那一版判断时的推导，此后的归入要等范围成立、经重核才进判断。

### 管理台各页与动作

都在「节点作业查阅」页（`node-operations-review`）里，chip 多两区，导航不加条目。

- **送站预报区**：按物流节点与「预计送达日」列（日期控件，`localDayBoundsOf` 换成本地零点到次日本地零点）；列标识、客户账户、节点、预计送达、号码数、交付方线索、撤回、登记时刻；点一行出详情卡：号码集合带引号逐字示出、撤回的含义。只读。
- **送站交接单区**：按物流节点与「开立日」列；列交接单、节点、交付方线索、开立、开放或关闭、成员数、声称的账户、绑定进度（已确认、已否认、待确认各几个）、可验证交付范围（已成立带时刻；不成立分「有账户否认」与「还有账户待确认」）。点一行出详情卡：声称的每个账户的结论、途径、作结论方与时刻、交付方参与方与依据引用；名下成员与形成方式（收寄带来、归入）、事实身份、业务时间。
- **「确认」动作**：只在没结论的账户上出现（已确认的写「已确认，不再确认」，已否认的写「客户已否认：结论是终态……运营决定改不了」）。点开表单：交付方参与方从参与方目录 `GET /commercial-business-parties` 选（按标识排，停用与已登记未生效的照列并标状态）；目录读不到时照实说明并可逐字填参与方标识；依据引用必填。提交到 `POST /node-operations/delivery-handover-confirmations`（载荷 `noteId`、`customerAccount`、`deliveringParticipant`、`basis`，不带租户与操作者）。答复照实：`BINDING_CONFIRMED` 写这次的结论与范围成立没有（成立写「名下仍待识别的实物会自动重核」），落了才重读详情与列表；`EXISTING_RESULT`、`EXISTING_CONCLUSION` 回显在册那一份；`NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP` 写「换一位有授权交付关系的交付方参与方，或先在参与方与商业登记，也可由客户账户自己确认」；`AUTHORIZED_DELIVERY_NOT_CONFIGURED` 写「本租户一条授权交付关系都没登记……租户先登记（PAR-NET-17）；交付的就是账户所属参与方本身时选它即可」；两格都写「未落结论」。问题码各有一句（404 看不见写「或这个客户账户不在它的声称之列」，500 写「授权交付判断读不动……未落结论」），未配置、未登录、未授予照全站口径。
- **待识别实物区**多三列：「为何待识别」（`pendingCauseOf`：未核对、候选空且没带核对态、范围成立但冲突、没有范围的冲突、零候选、缺范围·未证、缺范围·未配置、范围成立但范围内无相容候选、这一版没记范围核对态，各一句）；「交付范围核对」（已证；未证 · 原因（细分）；未配置；未记——附恢复动作：未配置指向登记时窗后重核，绑定未全部确认指向去交接单区确认，其余未证指向归入一张开放的交接单）；「范围依据 · 此刻归入」。重核答复多一行核对态，认得票 11 新加的 `AMBIGUITY_GONE_SCOPE_UNVERIFIED`（追加了一版、不交意图、缺的是范围）。

### 共享文件里加的确切行（只追加，不重排）

- `cmd/parcel-api/endpoints.go`：`assembleBusinessEndpoints` 参数表末尾（`departureGatePreview tfhttp.DepartureGatePreviewer,` 之后）`deliveryScopeRecords nodeopshttp.DeliveryScopeCatalogueReader,`；端点表末尾三行注释加四行 `{Pattern: "/node-operations-delivery-pre-advices", Handler: nodeopshttp.NewQueryDeliveryPreAdvicesEndpoint(nodeOperationsCatalogueIntake, deliveryScopeRecords)}`、`…/{preAdviceId}`（`NewQueryDeliveryPreAdviceEndpoint`）、`/node-operations-delivery-handover-notes`（`NewQueryDeliveryHandoverNotesEndpoint`）、`…/{noteId}`（`NewQueryDeliveryHandoverNoteEndpoint`）。
- `cmd/parcel-api/main.go`：`transportFulfillmentRecords` 那一段之后一行注释加 `deliveryScopeRecords, err := nopostgres.NewDeliveryScopeCatalogue(db)` 一段；`assembleBusinessEndpoints(` 实参末尾 `deliveryScopeRecords,`。
- `cmd/parcel-api/endpoints_test.go`：`businessEndpointProbes` 末尾四行（四条路径，GET）；`assembleUnwiredBusinessEndpointsWithOperatorIntakes` 实参末尾 `unwiredDeliveryScopeRecords{},`（占位类型在新文件 `assemble_delivery_scope_catalogue_test.go`）。
- `cmd/parcel-api/assemble_catalogue_read_operator_test.go`：`swappedOperationsReadPatterns` 末尾四行。
- `internal/nodeoperations/ports/catalogue_read.go`：`UnidentifiedItemCatalogueRow` 末尾三行注释加 `DeliveryScopeBasis domain.UnitDeliveryScopeBasis`。
- `internal/nodeoperations/adapters/postgres/review_catalogue.go`：`ListUnidentifiedItems` 头注末尾两行；SELECT 末尾三列（`source_document` 的 `scope_basis_kind`、`scope_basis_id`，与按（租户，实物）取最后一次归入的子查询）；Scan 末尾 `&basis[0], &basis[1], &basis[2]`；一处 `unitScopeBasisFrom` 调用。
- `internal/nodeoperations/adapters/http/query_node_operations_records.go`：`serveReceptions` 的字面量改成先 `body :=` 再补一行 `body.deliveryScopeCheckFields = …` 后 append（不改原有对齐）；`serveUnidentifiedItems` 两行赋值；`receptionCatalogueBody` 末尾嵌入 `deliveryScopeCheckFields`（一行注释）；`unidentifiedItemBody` 末尾嵌入它加 `DeliveryScopeBasis`、`AssignedHandoverNote`（两行注释）。

### 完成判据格 → 用例

| 格 | 用例 |
|---|---|
| 读口：列表与详情、跨租户与查无同答 | 存储 `internal/nodeoperations/adapters/postgres/delivery_scope_catalogue_test.go::TestPreAdvicesAreListedByNodeAndExpectedArrivalWithinTheTenant`（按节点、按预计送达左闭右开、只给起点、页大小、页大小 0 拒、别的租户同标识不串、按标识读本租户那一份、别的租户才有与不存在都 found=false）、`::TestHandoverNotesAreListedByNodeAndOpeningWithClaimsAndMembers`（成员数不算别的租户同名交接单、关闭、范围成立时刻、否认与没结论、运营决定证据、详情成员与形成方式、查无）；HTTP `internal/nodeoperations/adapters/http/query_delivery_scope_records_test.go::TestPreAdviceReadsTranscribeTheRegister`、`::TestHandoverNoteReadsCarryClaimsClosureMembersAndScope`、`::TestDeliveryScopeListsTakeTheirFiltersFromTheQuery`、`::TestDeliveryScopeListsRefuseMalformedFilters`、`::TestDeliveryScopeDetailsRefuseABlankIdentifier`、`::TestDeliveryScopeReadsAnswer500WhenTheRegisterFails`、`::TestDeliveryScopeReadsRefuseNonGetMethods`；真库经同一个读口构造与操作者查阅读 Intake `cmd/parcel-api/assemble_delivery_scope_catalogue_test.go::TestDeliveryScopeReadsAnswerOnlyTheEnvelopeTenantAgainstARealDatabase`（只看得到信封租户的，跨租户与查无逐字节同答 404，不带令牌 401） |
| 未授予运营查阅读答其格 | 传输层 `::TestDeliveryScopeReadsAnswerTheEnvelopeGradesFirst`（四口 × 未配置、令牌不过、未授予、依赖读不动；坏筛选也先答信封）；经装配路由 `cmd/parcel-api/assemble_catalogue_read_operator_test.go::TestSwappedCatalogueReadFacesAnswerFromTheOperatorChannel`（多四条路径：未配置、无令牌、只持登记写答 `OPERATOR_NOT_GRANTED`、授予齐备走到读口——详情口的路径参数名与 Pattern 对不上会在这里答 400 而红）；`cmd/parcel-api/endpoints_test.go::TestEveryAssembledEndpointAnswersUnconfigured`（多四条探针） |
| 复核目录：核对态、未证原因、依据引用与归入的交接单 | 存储 `internal/nodeoperations/adapters/postgres/review_catalogue_scope_basis_test.go::TestUnidentifiedItemsCarryTheScopeBasisAndTheLastAssignedNote`（来源带交接单、带预报、没带；归入取业务时间最后一次、不认收寄成员、别的租户同名实物的归入不串；未核对那一行都不带）；HTTP `::TestUnidentifiedItemsCarryTheScopeCheckBasisAndAssignedNote`（待识别行各键与缺席、收寄行核对态各键） |
| 管理台：核对态判读、确认载荷、答复呈现 | `apps/admin-web/src/pages/operations/delivery-scope-check.test.ts`（「为何待识别」四格分得开、其余几格、核对态各写各的字、恢复动作、依据与归入、待识别行三格、重核答复带核对态与新一格）；`delivery-handover-note.test.ts`（交接单行、声称账户与「确认」可否、成员行、确认载荷、确认答复 landed 与范围、「不在授权交付关系内」与「未配置」两句、问题码、交付方参与方候选）；`delivery-pre-advice.test.ts`（本地日界、列表路径、预报行、读口问题码） |

### 先红后绿

- 红：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm13-red.log`。Go 用例先写、生产代码一行未改（钉 `129e9a38`，工作树只有测试改动），三个包 `[build failed]`，现象原话是「undefined: adapter.DeliveryScopeCatalogue」「undefined: ports.DeliveryScopeCatalogueFilter」「undefined: nodeopshttp.NewQueryDeliveryPreAdvicesEndpoint」「undefined: nopostgres.NewDeliveryScopeCatalogue」一类，各包报到「too many errors」即止。管理台三份判读用例先写（钉第一笔 Go 提交 rebase 之前的 `63de63ca`，判读模块与镜像未写），`run-tests` 在编译段退 2，原话「Cannot find module './delivery-handover-note'」「'deliveryScopeCheck' does not exist in type 'Partial<UnidentifiedItemRecord>'」；随后补的重核答复一条单独红：「'deliveryScopeCheck' does not exist in type 'IdentityRecheckResponseBody'」。
- 绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm13-green.log`，两段：第一次 rebase 后钉 `c32ea809`，第二次 rebase 后钉 `88aa2aec`。

### 门

钉 `88aa2aec`（第二次 rebase 后、本记录改 SHA 之前的那一笔；`origin/main` `0237cef3` 之上）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 55432 DSN 以 `-p 1 -count=1` 跑 `./internal/nodeoperations/... ./internal/partycommercial/... ./cmd/parcel-api/... ./internal/architecture/...`，有测试的包全 ok、0 FAIL。管理台（Node v22.23.3，`pnpm install --frozen-lockfile`，lockfile 未动）：`tsc -b --noEmit` 退 0、`node scripts/run-tests.mjs` 590 条全过、`vite build` 退 0。第一次 rebase 后在 `c32ea809` 上同一组门也全过，两次都记在绿日志里。没跑全量 `./...`，留给推送方。机制清点在新树上重生成（`bdfe7557`），之后再跑零差：nodeoperations 生产与测试文件各 100→103、postgres 适配器 27→28、http 适配器 14→15，`cmd/` 测试 211→212，接入面端点 165→169，端口声明 539→540，基线口径缺口不变。

### 代决

1. **路径取查阅面的连字符前缀**：`/node-operations-delivery-pre-advices`、`/node-operations-delivery-handover-notes`，详情是 `/{标识}` 子路径。不挂在命令口的 `/node-operations/delivery-…` 下：路由按路径注册、不分方法，同一路径上的 GET 只能落到开立口、提交口那个处理器里。
2. **「日」由看的人所在时区定**：读口只收瞬间区间（左闭右开），管理台把日期换成本地零点到次日零点。不在服务端按 UTC 或某个租户时区切日——那是一项租户取值，服务端替它定就是写死默认。
3. **筛选键封闭**：未知键、重复键答 400，不静默丢掉：拼错一个键的人会把全租户列表当成筛过的。
4. **门次序照 ADR-0055**：方法 → Intake → 请求内容（路径与查询串也是请求内容）。没照 `/node-operations-records`「册名先于渠道」那一种：那一口的册名是分派维，这里的筛选是内容。
5. **详情查无答 404**，码取两本册命令口已在用的 `DELIVERY_PRE_ADVICE_NOT_VISIBLE`、`DELIVERY_HANDOVER_NOTE_NOT_VISIBLE`，与跨租户逐字节同答。没学参与方修订历史口的「200 + 空」：那是列表形状，这里是单个对象。
6. **读回经领域还原门**，范围成立与成立时刻取领域派生；一行坏记录让整页答不出。
7. **没结论的账户只带账户一格**：缺席即待确认，不在线上立一个领域里没有的「PENDING」词。作结论方（`concluder`）给出，客户渠道的来源请求键与预报的来源请求键、内容摘要不回显。
8. **复核目录两样依据分开给**（来源里的、最后归入的），不给「此刻起作用的」那一份（理由见上）。收寄册的行也带核对态各键（票 11 把这一格留给本票）。
9. **管理台不加导航条目**：两区放进「节点作业查阅」页的 chip，详情用页下方的卡片（同处置面板的先例）；地盘因此不出 `pages/operations/**`。
10. **交付方参与方从参与方目录选、不预筛**：能不能替这个账户送站由授权交付判断答，页面不按关系册先筛（那要在前端解析参与方关系的范围写法，等于第二份口径）；目录读不到时允许逐字填标识，判断照旧由参与方与商业作答。
11. **「为何待识别」的分格与次序**写在 `pendingCauseOf` 头注：未核对先说；记冲突按有没有范围分；零候选不看范围；剩下的才按核对态说缺什么。
12. **重核答复补 `AMBIGUITY_GONE_SCOPE_UNVERIFIED` 一格与核对态一行**：票 11 加了这一格结果，管理台此前会把它显示成未收录的码。

### 越权风险点（待 owner 复核）

1. **两本册的列表没有专门索引**：按（租户，节点，业务时刻）筛、按业务时刻倒序取一页，今天是租户内顺扫加排序。试点量级够用；量上来要加索引，那是一份迁移，本票没加（`node_operations` 的迁移号要与在途各票协调，`0014` 已留给票 14），另票取号。
2. **目录读不到时可逐字填交付方参与方**（代决 10）：填错的标识由授权交付判断答「不在授权交付关系内」或「未配置」，不会落结论；owner 若要求只许从目录选，删掉那个输入框一处。
3. **作结论方原样给持查阅读授予的人看**：运营决定写 `OPERATOR/<发行方>#<sub>`，客户渠道写客户端来源格；与身份冲突处置在管理台显示操作者同一口径。owner 若认为查阅读不该看到客户端标识，改在 `handoverClaimReviewBody` 一处。

### 越出地盘的改动

- `docs/product/MECHANISM-INVENTORY.md`：生成物重生成。
- 其余都在票面地盘内：`internal/nodeoperations/ports` 新文件与 `catalogue_read.go`、`adapters/postgres` 新文件与 `review_catalogue.go`、`adapters/http`、`cmd/parcel-api`、`apps/admin-web/src/pages/operations/**`、`.scratch/at-coverage-inventory.md`。管理台引了 `pages/party/api.ts` 的 `listBusinessParties` 与 `pages/party/presentation.ts` 的词表，没改那两份。

### AT 盘点

`.scratch/at-coverage-inventory.md` 追记十八：本票的查阅读、复核目录多的格与管理台在 UC-NO-006、UC-NO-002 验收表里没有对应行，新用例的 Covers 注释点的是票面条目，不硬点 AT 号；①②③④不变。

### 未做

- 完成判据末格「走 Go 那条路」：分支与作者自验已做；非作者评审与推送方在干净检出上带 DSN 跑全量不归作者，该格未勾。
- 没有浏览器实走与截图，也没有演示实走：页面经 `tsc`、判读用例与 `vite build` 证，读口经真库用例证。演示文档「送站交接单」一段没补管理台这一步。
- 节点收寄区没加核对态一列：HTTP 已带，页面只按票面在待识别实物区显示；要加是 `reception-rows.ts` 一处（会改那份整行断言）。
- 列表索引（越权风险点 1）。
- 票面「不做」照旧：交接单的开立与归入、客户面读口、时窗与授权交付关系的登记页。

## Comments

### 评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审；2026-10-19，钉 `c00a81cc`）

两轴合审，0 阻断。

- **Spec**：四个查阅读口（预报列表与详情、交接单列表与详情）走与 `/node-operations-records` 同一只运营查阅读 Intake，判序方法 → 信封 → 请求内容，跨租户与查无同答 404，坏筛选 400、读不动 500。交接单详情给每个声称账户的结论、途径、作结论方与成员形成方式，范围成立取领域判断不另算。复核目录待识别实物多交付范围核对态三格、依据引用与此刻归入的交接单。管理台在节点作业复核页加预报区、交接单区与「确认」动作（只对未下结论的账户，交到票 12 的运营决定口；不在授权交付关系内、未配置两格各写恢复动作且写明结论未落），待识别实物区三栏分得出缺范围、范围内冲突、零候选与未核对；三套纯函数配 node:test，三道门过。
- **作者自定的格认可**：参与方目录读不动时允许手填交付方参与方（越权风险点已列）；持读授予者看得到作结论方。
- **Standards**：共享文件只追加，两次 rebase 两边都保留；注释里的序数与计数引用已改成符号名。
- **未做（不另立票）**：两条列表查询没加索引（要协调迁移号，数据量起来时随 NO 读面性能票做）；没有浏览器实走与截图。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 分支 `claude-psxm13`（作者已 rebase 到另一会话 demo-chain-gaps/61 等之后）直接叠在远端 main 之上快进推送；机制清点重生成零差。管理台三道门由作者在本分支跑过（590/590），推送后由 CI 的 admin-web job 复核。
- 推送方全量一次：钉 `b7bf59e8` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 153 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
