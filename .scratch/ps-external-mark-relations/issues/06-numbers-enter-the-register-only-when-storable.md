# 06 外部标识号码进册前没有「存得下」的门：超长号码撞索引上限答 500，NUL 撞库面答 500；按号核对遇到不可能在册的号也答错误

Category: bug
Status: in-progress——2026-10-19 评审两条阻断由作者子代理修完（rebase 到 origin/main `38916b00`），待复核。此前 in-progress——2026-10-19 作者子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 立并代决、认领，分支 `claude-psxm06`
Blocked by: 无
父票：[05](./05-a-unique-hit-in-someone-elses-number-space-is-not-enough.md)
出处：no-reception-source/03 完成记录「发现」（PS 关联册按号码建的 btree 索引有长度上限，号码进册前无长度门）；本会话复核。
地盘：
- internal/parcelshipment/domain/external_identifier.go 及其测试（`NewExternalIdentifierNumber`）
- internal/parcelshipment/adapters/postgres/external_identifier.go 及其测试（按号核对读口）
- 往外部标识关联册写号的各条入口及其测试（提交 Intake 的客户包裹件号、面单渠道结果的渠道包裹件号——先 grep 全部写入方）
- cmd/parcel-api 下新起真库用例文件
- docs/adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md（文末追加复核记录）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「需要」，即立票并接着做。

## 现象（读码，未实测）

- `NewExternalIdentifierNumber` 只拒全空白。号码原样落进 `parcel_shipment.external_identifier_relation`，表上按号码建了 btree 索引；Postgres btree 索引行有长度上限（约 2.7KB），超长号码在写入那一刻报错，整笔事务回滚、入口答 500——重发同样的号永远一样。
- 号码带 NUL 时 Postgres 的 text 存不下，同样答 500。
- 读的一侧：NO 收寄经 `FindParcelsByExternalIdentifier` 拿设备原样上报的号来核对。一个不可能在册的号（超长）若在读口报错而不是答「零候选」，就会被 NO 当成「身份读不动」：no-reception-source/05 之后落「待识别 · 未核对」并自投重核门铃，而门铃重投每次都撞同一个错，永远续不上。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制，不涉租户取值）

1. **存得下的门定在领域构造处一处**：`NewExternalIdentifierNumber` 拒 NUL，拒 UTF-8 字节长度超过 512 的号。上限是「存得下、索引得了」的机制界，不是业务口径——远低于索引行上限，远高于任何现行条码与运单号的长度，与本仓意图信封 Subject 的上限同量级；租户真实号码长度不需要也不允许在这里配置。号码本身其余字符照 ADR-0184 一个不动（不折算、不去内部空白、不拒其他控制字符）。
2. **写入方因此答坏输入，不答 500**：提交 Intake 的客户包裹件号越界答 400 坏报文并点名该格；面单渠道结果里的渠道包裹件号越界照该路现有的坏结果答法（先查清），不让整笔接受或结果登记以 500 回滚。
3. **按号核对读口把「不可能在册的号」答零候选**：号过不了构造门时，`FindParcelsByExternalIdentifier` 直接答零候选、无歧义信号，不报错——这样的号确定不在册，答「查无此号」是真话；报错会被 NO 读成身份读不动、落进永远续不上的重核。
4. 难逆转的那一半（号码的存得下界）在 ADR-0184 文末追加复核记录，决定原文不动。

越权风险点：代 PS owner 定了 512 字节这一界。owner 复核时看有没有渠道的号码可能更长（例如把整段二维码内容当号码交来）。

## 完成判据

- [x] 领域：NUL、513 字节答构造错误；512 字节与带回车换行的号照收（逐字）。
- [x] 提交 Intake：越界的客户包裹件号答 400 点名该格，委托不落；面单渠道结果照该路坏结果答法，不 500。各一条用例，修前先红（真库用例至少一条经生产装配）。
- [x] 读口：越界号答零候选、无信号、不报错；经生产收寄装配真库用例：设备送来越界号，答待识别（已核对），不落未核对、不响门铃。
- [x] ADR-0184 文末复核记录；`git diff` 该文件只有新增行。
- [x] Go 门禁：`gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `internal/parcelshipment/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`internal/architecture` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-psxm06`）

起点 main `a7dfad8f`。证据层级 **S**：夹具全为合成值，隔离合成库上跑。

**提交**

| 提交 | 内容 |
|---|---|
| `f7ed10af` | 领域构造门、提交口点名该格、面单渠道结果拒收、NO 核对答零候选、门铃 Subject 用例与注释改前提，各层用例 |
| `9c590d4b` | ADR-0184 复核记录（2026-10-19 那一节追加一条，只有新增行） |
| `6df94d97` | 机制清点在 `9c590d4b` 的干净检出上重生成 |
| 本笔 | 本完成记录 |

**落点**

- 领域 `NewExternalIdentifierNumber`：全空白照旧拒；带 NUL、UTF-8 字节数超过 512 的拒，拒因 `ErrExternalIdentifierNumberNotStorable`（新哨兵，错误文本只说为什么，不带号码本身）。界是包内常量 `externalIdentifierNumberMaxBytes`，头注写明是机制界、不是租户取值。其余字符一个不拒：回车换行、Tab、GS 分隔符照收。
- 门只此一处。写入方与核对一侧都问它，见下。

**写入方清单与各自答法**（grep `NewExternalIdentifierNumber` 的全部调用方、`insertExternalIdentifierRelations` 的全部调用方；`insertExternalIdentifierChanges` 写的 `successor_number` 一并看了）

| 写入方 | 号从哪来 | 答法 |
|---|---|---|
| 提交口 `IsolatedSubmissionIntake.customerParcelNumbers`（`NewExternalIdentifierNumber` 调用方） | 草案逐件 `customerParcelReference` 去两端空白后的值 | 400 `MALFORMED_REQUEST`，问题体 `error.field` 点名 `parcels[i].customerParcelReference`；在编排之前拒，来源保全与委托都不落 |
| 委托仓储 `Save` → `insertExternalIdentifierRelations`（`ExternalIdentifierRelationsFormedByAcceptance`） | 委托快照上的配对，提交时已过门 | 不另设答法：进不了快照的号到不了这里 |
| 面单交易仓储 `Save` → `insertExternalIdentifierRelations`（`ExternalIdentifierRelationsFormedByChannelResults`） | 渠道结果受理件的 `ChannelParcelIdentifier`，`channelRelationOf` 不经构造器直接转成号码 | 门落在 `LabelTransaction.RecordChannelResult`：受理件的号问 `NewExternalIdentifierNumber`，过不了就交 `ErrInvalidLabelTransaction`（同时包着 `ErrExternalIdentifierNumberNotStorable`）。编排 `LabelTransactionHandler.refuse` 把它译成 `LabelTransactionNotAccepted`、不报错，不 `Save`，交回读出的那一份，交易停在原状态 |
| `insertExternalIdentifierChanges` 的 `successor_number` | 替代交易的新关联，同样出自 `RecordChannelResult` | 同上一行，记录结果那一步已拒 |
| 委托仓储重建快照（`shipment_request.go` 里的 `NewExternalIdentifierNumber` 调用方） | 库里已落的快照 | 本票不碰那份文件（并行子代理的地盘）。风险见「未做」 |

面单渠道结果「该路现有的坏结果答法」查清了：`RecordChannelResult` 对结构立不起来的结果（受理不带标识、未受理不带原因等）一律交 `ErrInvalidLabelTransaction`，编排 `refuse` 把它译成`未受理`。本票照用，没有另造一格。门不放进 `matchResultsToCoverage`：重建也走那里，已落库的结果要读得回（领域用例里钉了一格）。这条写链今天没有生产触发面：`cmd/parcel-api` 只有事务壳 `transactionalLabelTransactions.RecordChannelResult`，没有端点或连接器调它。所以它的真库用例经真编排、真仓储、真关联册，不经生产装配。

「点名该格」的形状：PS 的 `problemResponse` 头注明说刻意不带自由文本（`UC-PS-001` 不许错误信息泄露别的客户），所以没加 `detail` 散文，加的是结构化的 `field`。它是调用方自己报文里那一格的路径，只随能点名到格的 400 在场，`omitempty`，别的问题答复逐字节不变。实现是 `submit_shipment_request.go` 里的 `malformedFieldError`：同时 `Unwrap` 出 `ErrMalformedRequest` 与领域拒因，端点 `errors.As` 取路径。

**读口答法**：`FindParcelsByExternalIdentifier` 收的是成品类型 `domain.ExternalIdentifierNumber`，过不了门的号构造不出来，交不进读口，所以读口本身没改。零候选落在构造那一步：NO 的 `ParcelIdentityView.ResolveParcelIdentity` 构造 PS 号码遇 `ErrExternalIdentifierNumberNotStorable` 时，答零候选、无歧义信号、不报错，不去问 PS。其余构造错误照旧是 `ErrUntranslatableAnswer`。端口 `ParcelExternalIdentifierView` 头注补了一段，写明这条约定。

**修前的实际现象**（红日志原话）：

- 提交口：513 字节的「答 201 {"outcome":"SUBMITTED",…}」，号码进了快照，要到接受时才撞册。带 NUL 的「答 500 {"error":{"code":"NO_ANSWER_FORMED"}}」，且来源保全已落一行（来源保全与建单各一笔事务）。
- 面单渠道结果：513 字节的「outcome = "APPLIED"」，进了册。带 NUL 的「save label transaction: ERROR: unsupported Unicode escape sequence (SQLSTATE 22P05)」。4000 字节不可压缩的「insert external identifier relation: ERROR: index row size 4024 exceeds btree version 4 maximum 2704 for index "external_identifier_relation_by_number" (SQLSTATE 54000)」。
- 读侧：票面「现象」第三条与修前实测不符。修前越界的号在读口不报错：查询只比较、不构造索引行，答零候选、已核对，真库用例修前就是绿的。那个陷阱要等构造门落地、NO 未改的中间态才现形：「identityCheck":"UNCHECKED"」「untranslatable answer: mark: parcel shipment: external identifier number cannot enter the register: it is 513 bytes, over the 512-byte limit」。所以门与 NO 的零候选必须同笔进。带 NUL 的号到不了这一步：设备收寄口 `receptionCommand` 早已对 NUL 答坏报文，NO 收寄库的来源文档也存不下 NUL。

**完成判据格 → 用例**

| 格 | 用例 |
|---|---|
| 领域 | `internal/parcelshipment/domain/external_identifier_test.go::TestAnExternalIdentifierNumberMustBeStorable`（含 171 个汉字＝513 字节也拒、170 个汉字加两位＝512 字节照收，界按字节） |
| 提交 Intake | 真库、生产装配：`cmd/parcel-api/external_identifier_number_gate_test.go::TestTheWiredSubmissionRefusesACustomerParcelReferenceTheRegisterCannotStore`（隔离提交 Intake 与提交编排都由装配函数造，登记册放一条匹配的合成权威区间；513 字节与带 NUL 两格答 400、`field` 点名、来源保全与委托零行；512 字节照收、快照逐字）。Intake 单元：`internal/parcelshipment/adapters/http/isolated_write_intake_test.go::TestIsolatedSubmissionIntakeRefusesACustomerParcelReferenceTheRegisterCannotStore` |
| 面单渠道结果 | 真库：`internal/parcelshipment/adapters/postgres/label_transaction_channel_number_gate_test.go::TestAChannelResultWhoseNumberCannotEnterTheRegisterIsNotAccepted`（513 字节、带 NUL、4000 字节不可压缩三格答`未受理`、不报错、交易停在已提交渠道、册零行；512 字节不可压缩的落册并经读口查回）。领域：`TestAChannelResultWhoseAcceptedNumberCannotEnterTheRegisterIsRefused`（含重建不过门那一格） |
| 读口 | 真库、生产装配：`cmd/parcel-api/external_identifier_number_gate_test.go::TestTheWiredReceptionAnswersAMarkTheRegisterCannotHoldAsCheckedPending`（513 与 4000 字节两格答 200 `PENDING_IDENTIFICATION`、`identityCheck` `CHECKED`、无候选、控制照建，重核门铃零封）。适配器：`internal/nodeoperations/adapters/parcelshipment/identity_test.go::TestAMarkTheRegisterCannotHoldHasNoCandidatesAndIsNotAnError`（PS 替身带候选与信号，也答零候选、无信号、PS 没被问） |
| ADR | `git diff --numstat` 该文件 8 增 0 删 |

真库用例里的号用不可压缩的串：Postgres 写索引行前会先压缩，一长串同一个字符压得下去，撞不到索引行上限。

**先红后绿**：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm06-red.log` 有三段。第一段是修前，只加了空哨兵供用例编译：领域、Intake、适配器各自断言级红，面单真库三格与提交口真库红，收寄真库绿。第二段把提交口真库用例拆成逐格子用例后补跑，两格的现象见上。第三段是中间态：构造门已落、NO 未改，读侧两条用例红。绿：`/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm06-green.log`。红之后改过一处断言：提交口真库用例原先断言 `error.detail`，查到 `problemResponse` 不带自由文本的理由后改断言 `error.field`，状态码与落库断言没动。

**门**（在 `f7ed10af` 的 Go 内容上跑，之后的提交不碰 Go 文件）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `-p 1 -count=1` 跑 `./internal/parcelshipment/... ./internal/nodeoperations/... ./cmd/parcel-api/... ./internal/architecture/...`，30 个包全 ok。同用这只 NO 适配器的 `./cmd/parcel-dispatch/...`、`./cmd/parcel-frontline-import/...` 也跑了，ok。`cmd/parcel-label-transaction` 不存在，面单渠道结果写号那条路的真库用例在 `internal/parcelshipment/adapters/postgres`，已含在上面。全量 `./...` 没跑，留给推送方。

**越出地盘的改动**

- `internal/parcelshipment/domain/label_transaction.go`：`RecordChannelResult` 加门。它就是面单渠道结果这条写册路的入口，算在「各条入口」里，单列备查。
- `internal/parcelshipment/adapters/http/submit_shipment_request.go`：`malformedFieldError` 与 `problemDetail.Field`，提交端点 400 那一支取路径。
- `internal/parcelshipment/ports/external_identifier.go`：端口头注补一段。
- `internal/parcelshipment/adapters/postgres/external_identifier_formed_handoff.go` 与其测试：两段注释原写「号码进册前只拦全空白，没有长度门」，门落地后成了假话，改了。用例 `TestANumberTheSubjectCannotCarryStillRingsByItsDigest` 原用 640 字节的号，现在进不了册，在全门里红了；改用册收得下的最长号（512 字节）与带 Tab 的号，Subject 断言放宽成「收得下的原样、收不下的摘要、都不越上限」，带 Tab 的仍须是摘要。`externalIdentifierSubject` 的长度分支在今天的两条界下走不到，照留：两条界各自独立，这里不假设哪条更紧。
- `internal/nodeoperations/adapters/parcelshipment/identity.go`：派活时已点名。
- `docs/product/MECHANISM-INVENTORY.md`：重生成。
- 没碰：`internal/parcelshipment/adapters/postgres/shipment_request.go`、NO 重核编排、演示库。

**未做**

- 「走 Go 那条路」不勾：本分支没推 origin、没进 main（派活时明令不推），进 main 归推送方。
- 门落地前已进委托快照、超过 512 字节的客户包裹件号：委托仓储重建快照时经 `NewExternalIdentifierNumber`，这样的委托会读不回来。修法是让快照重建走不设界的路，落在 `shipment_request.go`，不在本票地盘，记进了 ADR-0184 复核记录「留给 PS owner」。真实租户未上线。演示种子不灌客户包裹件号；演示库里经管理台录入的号有没有超界，没查（不动演示库）。
- 非法 UTF-8：Postgres text 同样存不下，但裁定没点它。今天各入口都经 JSON 解码，非法字节已被换成 U+FFFD，到不了门前，所以没设门。
- 读口 `FindParcelsByExternalIdentifier` 没改，理由见「读口答法」。

## 评审阻断修补（2026-10-19，会话 idp-parcel-9b 的作者子代理，分支 `claude-psxm06`）

评审给了两条阻断，都在本分支修。先把分支 rebase 到 origin/main `38916b00`（first-party-api/00 已进 main）。上一节「完成记录」里的 SHA 是 rebase 之前的，它们与 rebase 之后的对照如下。上一节写的 `error.field`、`IsolatedSubmissionIntake` 与那条留给 PS owner 的旧快照风险，都以本节为准。

**提交**（rebase 之后）

| 提交 | 内容 |
|---|---|
| `57d0cab0` | 即上一节 `f7ed10af`。rebase 时 `submit_shipment_request.go` 撞了 main 的 `writeIntakeProblem`，取 main 那一行；`customerParcelNumbers` 的改动由 git 自动并进 `customer_submission_intake.go` |
| `aeb332f6` | 即上一节 `9c590d4b`（ADR-0184 复核记录） |
| `050a83cd` | 即上一节的完成记录那一笔 |
| （丢弃） | 上一节 `6df94d97` 的机制清点，与 main 的重生成冲突，跳过，由下面重生成那一笔替代 |
| `b438d619` | 阻断一 |
| `22e8fb46` | 阻断二 |
| `989f7662` | ADR-0184 复核记录第 2 条随修补改写。改的是本分支新加、尚未进 main 的行；对 origin/main，该文件仍是 8 增 0 删 |
| `5aa3fd32` | 机制清点在 `989f7662` 的干净检出上重生成 |
| 本笔 | 本节 |

`57d0cab0` 单拿出来时 `go build ./...` 过。`internal/parcelshipment/adapters/http` 与 `cmd/parcel-api` 两包的测试编译不过：用例还引着 main 已撤的 `newIsolatedIntake`、`buildIsolatedSubmissionIntake`，到 `b438d619` 才改过来。门只钉在 tip 上。

**阻断一：生产提交口换成了首方客户渠道 Intake**

- **首方 Intake 也经 `NewExternalIdentifierNumber`。** main 把 `isolated_write_intake.go` 改成了 `customer_submission_intake.go`。`customerParcelNumbers` 原样留着：首方载荷逐件的 `customerParcelReference` 去两端空白后，经它配成客户包裹件号。所以本票在那个函数里的改动随 rebase 落到 `CustomerSubmissionIntake` 上，越界号在首方口上同样答 `ErrMalformedRequest`。
- **`IsolatedSubmissionIntake` 已不存在。** first-party-api/00 撤了这一类型与 `buildIsolatedSubmissionIntake`，仓里没有调用方。它已不是生产路，无可保留。
- **答复形状照首方契约。** 先读了首方 Intake 现有的坏载荷答法。它经本包共用的 `writeIntakeProblem` 答 400 `{"error":{"code":"MALFORMED_REQUEST"}}`，没有说明。ADR-0140 Decision 三给「改请求才会好」的 4xx 留的成员是 `detail`：散文，只说调用方自己这份载荷错在哪，客户端不据此分支。所以把上一节加的 `field` 换成了 `detail`。
  - 只有本包立的 `malformedFieldError` 带 `detail`，在 `writeIntakeProblem` 里取。别处包着 `ErrMalformedRequest` 交上来的错照旧只交 `code`：那些措辞不由本包把关，所以别的答复逐字节不变。
  - 越界那一格的 `detail` 形如「parcels[0].customerParcelReference: parcel shipment: external identifier number cannot enter the register: it is 513 bytes, over the 512-byte limit」，带 NUL 的那一格说「it carries a NUL character」。只点那一格的路径与拒因，不带号码本身，不涉任何别的客户。
  - `problemResponse` 头注随之改成「不带底层失败的措辞」，`detail` 的边界在字段头注里引 ADR-0140 Decision 三。
- **ADR-0140 只做了 `detail` 这一格。** 问题体仍是本包现行的 `{"error":{…}}`、`application/json`。ADR-0140 Decision 三的 `application/problem+json` 与 `type`/`title`/`status`/`requestId` 是整仓 `writeProblem` 收进 `internal/platform/httpapi` 那件事，各处理器至今都没换。本票不越做；那一格落地时，`detail` 原样搬过去。
- **真库用例改走现行生产装配。** `TestTheWiredSubmissionRefusesACustomerParcelReferenceTheRegisterCannotStore` 的装法照 `TestACustomerClientSubmitsThroughTheFirstPartyLineAgainstARealDatabase`：客户渠道册登一个合成客户端并授提交能力，发行方替身签首方受众的令牌，再接 `buildCustomerClientCredentialVerifier`、`buildCustomerClientMinter`、`buildProductionOwnership`、`buildCustomerSubmissionIntake`、`buildSubmissionOrchestration`。归属坐标取写开关那一格与一条匹配的合成区间，同 main 那条用例。断言如下：
  - 513 字节与带 NUL 两格都答 400 `MALFORMED_REQUEST`。
  - `error.detail` 以「parcels[0].customerParcelReference: 」起头，答复里不出现号码本身。
  - 来源保全与委托都是零行。
  - 512 字节答 201 `SUBMITTED`，快照里逐字是那个号。
- 首方 Intake 的单元用例改名为 `TestCustomerSubmissionIntakeRefusesACustomerParcelReferenceTheRegisterCannotStore`，夹具换成首方的 `newCustomerIntake` 与 `customerDraft`。

**阻断二：门立起来之前存下的超长号要读得回**

- 领域新加 `RehydrateExternalIdentifierNumber`，只拒全空白。头注照面单交易的门不放进 `matchResultsToCoverage` 的理由写：重建要读得回，而且只给仓储还原用。
- `internal/architecture` 的重建面门禁要求每个 `Rehydrate*` 入口都归类，它进了受限名单 `rehydrationEntryIdentifiers`，只对持久化适配器开放：开放给别人，新号就能绕过门进册。这是越出地盘的一行，不归类，那条门禁就红。
- 委托仓储只动了 `customerParcelNumberDocument.declaration` 里那一处调用，`git diff origin/main` 该文件 1 增 1 删。拒绝、撤回交信封那一边没碰，与 no-reception-identity/07 的合并归推送方。
- 真库用例 `internal/parcelshipment/adapters/postgres/shipment_request_legacy_number_test.go::TestASnapshotStoredBeforeTheNumberGateStillReadsBack`，新起一个文件，不碰并行那边会改的 `shipment_request_test.go`：
  - 经仓储插一份带占位号的委托，再用 `jsonb_set` 直写成 600 字节的客户包裹件号，模拟门前留下的行。
  - 经 `FindBySourceIdentity` 读回成功，配对逐字。
  - 读回之后照常接受，号进册，关联册一行。
- 领域用例 `TestARehydratedExternalIdentifierNumberIsNotHeldToTheEntryGate`：同一个 600 字节的号，新号构造拒、还原照收，全空白仍拒。

**先红后绿**：同一份红日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm06-red.log` 末尾追加了「红（评审阻断修补，修前）」一段，跑在 rebase 之后、改代码之前。原话如下：

- 「读回带 600 字节客户包裹件号的委托：found = false err = find shipment request: parcel shipment: external identifier number cannot enter the register: it is 600 bytes, over the 512-byte limit」。
- 提交口两格「detail "", want 点名 parcels[0].customerParcelReference」：rebase 之后首方口已答 400，只是没有点名。
- 首方 Intake 单元用例那时已经绿。

修后全绿，见下面的门。

**门**（在 `989f7662` 的 Go 内容上跑，之后两笔不碰 Go 文件）：

- `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0。
- 带 DSN `-p 1 -count=1` 跑 `./internal/parcelshipment/...`、`./internal/nodeoperations/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/...`、`./internal/architecture/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-frontline-import/...`，37 个包全 ok，日志 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/psxm06-gate-review.log`。
  - 首方 Intake 在 `internal/parcelshipment/adapters/http`，它的认证翻译在 `internal/parcelshipment/adapters/accessidentity`，首方铸造器在 `internal/accessidentity`，三处都在上面。
  - 第一次跑时 `internal/architecture` 的 `TestEveryIdentifierOnTheRehydrationSurfaceIsClassified` 红，补进受限名单后绿。
- 全量 `./...` 没跑。

**越出地盘的改动（本节新增）**

- `internal/parcelshipment/adapters/http/query_shipment_request_views.go`：本包共用的 `writeIntakeProblem` 在 `MALFORMED_REQUEST` 那一支取 `detail`。
- `internal/architecture/rehydration_gate_test.go`：受限名单加一行。
- `internal/parcelshipment/adapters/postgres/shipment_request.go`：派活时点名的那一处调用。

**发现，需裁定（未改）**

门落地之前已经进册的超长号，NO 查不到。阻断二让门前进了快照的超长号读得回，接受时号照常进册，上面的真库用例就是这样走的；门前已接受的委托，号也早在册里。但现场扫到同一个号时，NO 的 `ParcelIdentityView` 照裁定三，构造失败就答零个候选、不去问 PS，册里那一件于是查不到：本该唯一关联，结果落待识别。这与阻断二是同一类回归，只是在读的一侧。

建议的修法只改 NO 适配器那一处判断：

- 带 NUL 的号照旧直接答零个候选。Postgres 的 text 存不下 NUL，它确定不在册，去问反而会报错。
- 只是超长的号仍去问 PS。读口按号比较、不构造索引行，长号不报错：本票修前的红日志里，513 字节与 4000 字节的号经真库都答零候选、已核对。

这改的是裁定三「过不了构造门就直接答零候选」的字面，所以没有自行改，留给会话裁定。ADR-0184 复核记录第 2 条「留给 PS owner 与 NO owner」已写明这一问。
