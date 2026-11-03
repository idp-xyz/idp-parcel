# 15 查询值含 NUL 或非法 UTF-8 的读口一律答 400、不读库

Category: bug
Status: resolved——2026-11-02 进 main（记录见文末「进 main 记录」）；请求体同一缺陷见票 16
父票：[spec](../spec.md)
Blocked by: 无
出处：票 [12](./12-customer-tracking-by-shipment-number.md) 复评（钉 `93dfe3f06`）——操作者族 `serveCurrentProjection`（visibility-exception `query_tracking_projections.go`）的 `parcel` 含 NUL 会进读，库拒收参数答 `500`。票 12 只修了首方客户追踪两口（`refuseUnstorableQueryValue`）。

## 要做的

1. 全仓清点：各上下文 HTTP 读口里，查询串或路径参数原样带进 SQL 参数的格（`internal/*/adapters/http`）。逐个探：含 NUL、非法 UTF-8 时今天答什么。
2. 凡答 `500` 的，改为在该口的入站解析处答 `400`、不读库（依据 ADR-0029：坏输入改了才会好，不能报成可重试）。答复体与该族既有坏请求同形。能共用一处检查就共用（例如 `internal/platform` 的 HTTP 共用件），不在每口各写一份；共用件放哪里照包布局与 architecture 测试。
3. 每个改过的口补 HTTP 用例（NUL、非法 UTF-8 各一，断言 `400` 且不读）；合法多字节 UTF-8（中文、全角、emoji）照收不误拒。

## 不做

- 不改任何口的契约形状、成功答复与既有坏请求的分格。
- 不做长度上限、字符集白名单之类的新校验——只拦库存不下的值。

## 完成判据

- 清点表（口、参数、修前答复、修后答复）写进完成记录。
- Go 受影响包加 `internal/architecture`、`cmd/...` 真库测试过；`go build ./...`、`go vet ./...` 退 0。
- 照「进 main 的步骤」交非作者评审。

## 完成记录（2026-11-02，作者，分支 `claude-docnum15`，从 `969205ae9` 起）

### 清点

**做法。** 先静态找：`internal/*/adapters/http` 与 `internal/platform` 下读 `URL.Query()`、`url.ParseQuery`、`PathValue` 的处理器与 Intake，`cmd/parcel-api` 不直接读查询串。再用一只临时探针（不入库）在真库上实测：按登记处装上全部非首方的 GET 行，读口接真库，渠道用授予齐备的合成操作者与隔离读入参。逐行打四种坏值：NUL（`%00`）与三种非法 UTF-8（`%FF`、`%C0%AF`、`%ED%A0%80`）。打的位置有三类：静态清点出的全部查询参数名，多分派口的每一个分派值，每个路径参数段。修前修后各跑一遍。

**结果**（实测于 `969205ae9`）。9 个上下文 28 行修前答 `500 NO_ANSWER_FORMED`，都是查询值或路径参数原样进了 SQL 参数。其中 26 行属操作者族（主数据与运营查阅读），2 行属隔离读族（委托查阅与复核队列）。修后这些格一律答 `400`，答复体 `{"error":{"code":"MALFORMED_REQUEST"}}`（application/json），与两族处理器既有的坏请求同形。处理器、Intake 与读口都不进。探针重跑，GET 行零个 5xx。

| 上下文 | 口 | 参数 | 修前（NUL / 非法 UTF-8） | 修后 |
|---|---|---|---|---|
| visibility-exception | `/tracking-projections` | `parcel`、`version` | 500 / 500 | 400 |
| parcel-shipment（隔离读） | `/shipment-request-views` | `shipmentRequestId`、`documentNumber`、`shipmentRequestIds` | 500 / 500 | 400 |
| parcel-shipment（隔离读） | `/acceptance-review-queue` | `shipmentRequestId` | 500 / 500 | 400 |
| parcel-shipment | `/channel-selection-decisions` | `decisionId`；`view=tied` 下的 `scope`、`mapping` | 500 / 500 | 400 |
| parcel-shipment | `/post-intake-disposition-manual-items` | `parcel` | 500 / 500 | 400 |
| party-commercial | `/commercial-business-parties/{partyId}/revisions` | 路径 `partyId` | 500 / 500 | 400 |
| party-commercial | `/commercial-group-legal-entities/{legalEntityId}/revisions`、`…/profile-revisions`、`…/profile-resolution` | 路径 `legalEntityId` | 500 / 500 | 400 |
| party-commercial | `/commercial-product-channel-mappings/{mappingId}/revisions` | 路径 `mappingId` | 500 / 500 | 400 |
| party-commercial | `/commercial-publication-draft-readbacks/{kind}/{objectId}/{version}` | 路径 `objectId`、`version`（`kind` 是封闭集，修前已答 400） | 500 / 500 | 400 |
| party-commercial | `/commercial-customer-accounts` | `q`、`customerPartyId` | 500 / 500 | 400 |
| customs-compliance | `/customs-case-records` | `case` | 500 / 500 | 400 |
| customs-compliance | `/customs-submission-records`、`/customs-external-result-records` | `version` | 500 / 500 | 400 |
| customs-compliance | `/customs-action-admission-judgements` | `object` | 500 / 500 | 400 |
| network-routing | `/network-catalog` | `q`（七个 `family` 都是） | 500 / 500 | 400 |
| node-operations | `/node-operations-delivery-pre-advices`、`/node-operations-delivery-handover-notes` | `node`、`documentNumber` | 500 / 500 | 400 |
| node-operations | `/node-operations-delivery-pre-advices/{preAdviceId}`、`/node-operations-delivery-handover-notes/{noteId}` | 路径参数 | 500 / 500 | 400 |
| parcel-pricing | `/pricing-price-cards/content` | `id`、`version` | 500 / 500 | 400 |
| settlement-accounting | `/settlement-statement-lines` | `statement` | 500 / 500 | 400 |
| settlement-accounting | `/settlement-supplier-bill-lines` | `claim`、`version` | 500 / 500 | 400 |
| settlement-accounting | `/settlement-audited-payables` | `claim`、`documentNumber`、`supplier` | 500 / 500 | 400 |
| transport-fulfillment | `/transport-fulfillment-carrier-first-effective-pickups` | `object` | 400 / 500 | 400 |
| transport-fulfillment | `/transport-fulfillment-external-tracking-facts` | `source` | 400 / 500 | 400 |
| transport-fulfillment | `/transport-fulfillment-records?registry=booking-answer-bounce` | `booking`、`source` | 400 / 500 | 400 |
| visibility-exception（首方） | `/v1/customer-tracking-view`、`/v1/customer-shipment-tracking` | `parcel`、`documentNumber` | 400 / 400（票 12） | 不变，判据改调共用件 |

transport-fulfillment 三口原先只拦 NUL、不拦非法 UTF-8。

**修前就不答 500 的。**
- 分派参数（`registry`、`register`、`family`、`kind`、`view`）是封闭集，坏值答 400。
- 时刻参数解不开，答 400。
- 目录翻页的 `after`、`sort` 由 `cataloguepage` 解。
- 登记册配置写族的两行 GET（价卡草稿、发布词表）与 `/document-numbering/rules` 探不到 5xx。这些口照样挂上检查，坏目标的答复有变，见下一段。
- 基础数据三口（`/reference-data/countries`、`/currencies`、`/units`）只读内存表。操作者身份三口（`/work-devices`、`/operator-binding`、`/operator-grants`）不读查询串。这六口豁免、不挂检查，答复与修前逐字相同（评审修订 1）。

**修后答复有变、修前不是 500 的格**（实测于 `b29cf8216`，与同一份代码摘掉检查后的答复逐格比；豁免的六口已除外）：
- **不读的键带坏值。** 挂检查的各行修前照常答，修后答 400。照常答多是 200；节点作业两个路径详情口是 404 不可见。这是「判整条目标」的代价，评审已接受。
- **四口 400 少了一格响应体。** 这四口修前本就答 400 `MALFORMED_REQUEST`，修后码与状态不变：
  - `/commercial-publication-vocabularies` 的 `kind` 坏值丢了 `problems`；
  - `/reference-data/enablement` 的 `kind` 坏值丢了 `detail`；
  - `/commercial-customer-accounts` 与 `/network-catalog` 的坏键或坏值丢了 `detail`。
- **路径里的小写转义 `%ff`。**
  - Go 对这种写法另存 `RawPath`，chi 按 `RawPath` 路由，路径参数拿到的是没解码的 `SYN%ffX`，纯 ASCII。所以修前处理器照常答：party-commercial 五个路径参数口答 200，节点作业两个路径详情口答 404 不可见。
  - 检查按解码后的路径判，修后答 400。检查比处理器严，评审已接受。

**请求体（不在本票修）。** JSON 字符串里写 `\u0000`，解码后进命令的是真 NUL。抽了三个上下文的登记册配置写口，各口带一份合规载荷与只差一格 NUL 的载荷，真库实测：

| 口 | 格 | 今天答 |
|---|---|---|
| `/commercial-business-party-registrations` | `partyId`、`name`、`basis` 任一含 NUL | 500 `NO_ANSWER_FORMED`（合规载荷 201） |
| `/visibility-catalogue-milestone-mapping-registrations` | `version` 含 NUL | 500 `NO_ANSWER_FORMED`（合规载荷 201） |
| `/network-catalog-node-registrations` | `code` 含 NUL | 500 `NO_ANSWER_FORMED`；`display_name` 含 NUL 已答 400 |

载荷里的非法 UTF-8 原始字节不答 500：`encoding/json` 换成 U+FFFD 之后照常受理（node 登记 201），落库的值已被悄悄改过。已经拦住请求体 NUL 的有：transport-fulfillment 作业事实（票 tf-face-algebra/01，答 200 `未受理`），node-operations 收寄、预报、协作各口，编号规则册两口（都答 400）。

### 提交

| 提交 | 内容 |
|---|---|
| `0171ee53a` | `httpapi` 共用判据与读口检查、登记处挂检查、首方两口改调共用判据、单测与真库装配用例；Status → in-progress |
| `b29cf8216` | 完成记录，Status → 完工待评审 |
| `6abe252b6` | 评审修订：豁免六口、去掉族的预认定、注释改成真实次序、两条新用例 |
| 本笔 | 评审修订记录；完成记录里答复变化与「不做」改准 |

### 做法与取舍

- **判据一份，放在 `internal/platform/httpapi`。** `StorableText` 判单个值：合法 UTF-8 且不含 NUL。`StorableRequestTarget` 判整条请求目标：解码后的路径，与查询串里每个键、每个值。`RefuseUnstorableReadTarget` 只拦 GET，坏目标交给装配处给的拒法，处理器一步不走；别的方法原样交给处理器，405 仍由它自己答。`httpapi` 只依赖标准库，不碰任何上下文，architecture 门禁不受影响。
- **挂在 `cmd/parcel-api` 的登记处，不逐口去改。** `registerEndpoints` 登记时按行算好拒法（`unstorableTargetRefusals`）。`production` 与 `mountWith` 两个闭包出端点时都经 `guardReadTargets` 套上，生产装配与装配测试挂的是同一道。只挂 GET 行：命令口不读查询串，挂上去会让一条今天照常受理的命令因为一个它不读的键被拒。新加一行 GET 自动带上。族写漏了在 init 里就停。settlement-accounting 的处理器文件一个没动。
- **按族定形状。** 首方客户族不在装配处挂：本族先认证、后读查询串（`TestCustomerTrackingViewIntakeAuthenticatesBeforeReadingTheQueryString`），令牌不过的调用方看不到查询串校验的结果。所以它的读口仍在 Intake 里、认证之后判，`refuseUnstorableQueryValue` 改调 `httpapi.StorableText`，不留第二份判据；票 12 的 `TestUnstorableQueryValuesAreBadRequestsOnBothLines` 原样通过。今天挂 GET 行的另三族（主数据与运营查阅读、登记册配置写、隔离读）处理器写的坏请求是旧形状 `{"error":{"code"}}`，检查照它的字节写；别的族挂上 GET 行时先定形状，没定就在 init 里停（评审修订 4），由 `writeLegacyMalformedRequest` 出。真库用例拿 `/tracking-projections` 处理器自己答的坏请求（两个定位参数同在）与之逐字节比。哪一族迁到问题详情（ADR-0140 决定三的后续票），就在 `unstorableTargetRefusal` 里把它挪出那一格。
- **检查在认证之前。** 渠道未配置时，坏目标答 400 而不是 403。理由与各口把参数在场、封闭集当传输形状、先于 Intake 判相同：`/tracking-projections` 两个定位参数同在即 400，`/commercial-customer-accounts` 解码先于 Intake。判的是整条目标、每一格一视同仁，答复与端点、与键都无关，不泄露「本端点认哪些键」。
- **判整条目标，不只判各口读的那几格。** 哪几格进读只有处理器知道，在它之前判就只能一视同仁。代价：一个口不读的键带着 NUL，修前照常答，修后答 400。这条请求目标本就不是文本，所以照拒。转义坏掉或带分号的那一段，`ParseQuery` 与 `URL.Query()` 都丢下不交，检查也不判，仍归各口自己的解码。
- **不做**：没加长度上限、字符白名单。中文、全角、emoji 照收：单测与两条装配用例都证了合法多字节与 ASCII 答复相同。没改任何口的契约形状与成功答复。既有坏请求只在坏目标上有变：
  - 修前答 500 的，改答 400；
  - 不读的键带坏值，由照常答改答 400；
  - 四口 400 的响应体少了 `detail` 或 `problems`（见清点「修后答复有变」）。
  修前分格里没有 `MALFORMED_REQUEST` 的六口豁免，答复一字不变。transport-fulfillment 三口与若干领域构造器里原有的 NUL 判断留着没动：它们在处理器文件里，对 GET 读口已成冗余，但对别的调用面仍有用。

### 验证（证据只记 `S`）

- 先红后绿：把 `guardReadTargets` 的套壳临时摘掉，三条新装配用例全红。真库那条答 `500 NO_ANSWER_FORMED`，装配那条答 `403` 渠道未配置，替身那条进了 Intake。套回去全绿。
- 新用例：
  - `internal/platform/httpapi`：`TestStorableTextRefusesOnlyWhatTheDatabaseCannotStore`、`TestStorableRequestTargetJudgesTheDecodedPathKeysAndValues`、`TestRefuseUnstorableReadTargetStopsBadReadsBeforeTheHandler`（处理器替身计数为零；POST 坏目标照进处理器）。
  - `cmd/parcel-api`：
    - `TestUnstorableReadValuesNeverReachTheIntakeOrTheReader`：操作者族取 `/tracking-projections`，隔离读族取 `/shipment-request-views`，都用 Intake 替身，读口给 nil。四种坏值都答 400，Intake 一次没被问；合法多字节进了 Intake。
    - `TestEveryRegisteredReadLineCarriesTheTargetCheck`：登记处每一行非首方 GET，坏查询值与坏路径段都答 400 旧形状，合法多字节与 ASCII 答复逐字相同。首方 GET 行与全部命令行，同样的坏目标照旧答 403 渠道未配置。
    - `TestUnstorableReadValuesAreBadRequestsAgainstARealDatabase`：真库、生产装配，清点表每个上下文取一口，共十口：四种坏值都答 400；合法多字节与 ASCII 同状态，且不是 400、也不是 500。
- 临时探针修后重跑：GET 行零个 5xx，各行基线答复与修前逐字相同（时间戳除外）。
- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations tools` 无输出。
- `go test -count=1 -p 1 ./internal/... ./cmd/... ./migrations/...` 退 0：168 个包 ok、0 FAIL（含 PG `127.0.0.1:55432`），其中 `internal/architecture`、`internal/platform/httpapi`、`internal/visibilityexception/adapters/http`、`cmd/parcel-api` 都在内。跑的是 `0171ee53a` 的 Go 内容，只差 `unstorable_read_targets_test.go` 里一行注释（提交前改的）。
- `bash scripts/ci/generated-artifacts-pr.sh` 退 0：ADR 索引、AT 计数与提交的一致；机制清点过期，留推送方重生成。

### 留给协调方

- **请求体 NUL 答 500，另立票。** 见清点「请求体」一节：登记册配置写族实测三个上下文都中，其余各族多半同病。修法不同于本票：请求体只在各口解码之后才知道哪几格进库，没有整条目标可判。可选的修法有两种：在各口共用的 JSON 解码处拒 NUL，或由领域构造器一律拒。另有一事：载荷里的非法 UTF-8 被 `encoding/json` 悄悄换成 U+FFFD 后照常落库，要不要改成拒收，请一并裁。
- **请评审过目的两处取舍。**
  - 检查在认证之前：渠道未配置时，坏目标答 400 而不是 403。
  - 判整条目标：一个口不读的键带 NUL，修前照常答，修后答 400。
  - 两处的理由见「做法与取舍」。若裁定要改成认证之后判，就得逐个上下文改 Intake，并给各处理器的 Intake 错误映射补一格坏请求。settlement-accounting 的处理器也在其列。
- **旧形状写在组合根。** `writeLegacyMalformedRequest` 在 `cmd/parcel-api` 里另写了一份旧形状，由真库用例与处理器自己答的那份逐字节比。操作者族迁到问题详情时要一起改，改法见 `unstorableTargetRefusal` 的头注。


## 评审修订（2026-11-02，作者，协调方转来的评审，钉 `b29cf8216`）

评审结论：Standards 阻断 0，Spec 阻断 1。作者请裁的三处：认证前检查可接受；判整条目标可接受；请求体 NUL 已立票 16。本轮追加提交，不 rebase。

1. **Spec 阻断：六口不挂检查**（`6abe252b6`）。
   - 缺陷：基础数据三口与操作者身份三口修前不把查询值带进库，也不答 500，挂上检查改掉了既有分格。例如 `?region=SYN%00X` 由 400 `REGION_NOT_RECOGNIZED` 变成 400 `MALFORMED_REQUEST`，`?q=SYN%00X` 由 200 变成 400，操作者身份三口凭空多出一格 400。
   - 改法：`cmd/parcel-api/read_target_guard.go` 立一张 `targetCheckExemptions` 表，豁免只写在这一处。理由写在表值（`referenceDataExemption`、`operatorIdentityExemption`）：「不把查询值带进库，既有分格不动」。进表要两条同时成立，写在表头：
     - 这一行不把任何查询值或路径段带进库；
     - 它的分格里没有 `MALFORMED_REQUEST`。
   - `unstorableTargetRefusal` 先查这张表。
   - 新用例：
     - `TestExemptReadLinesAnswerExactlyAsTheirHandlersDo`：真库、真铸造器。六口对四种坏值的答复，经登记处装配与不挂检查的同一处理器逐字节相同。钉住三格：`region` 带 NUL 答 400 `REGION_NOT_RECOGNIZED`，`category` 带非法 UTF-8 答 400 `CATEGORY_NOT_RECOGNIZED`，`q` 与操作者身份三口的坏查询值答 200。
     - `TestTargetCheckExemptionsNameDeclaredReadLines`：豁免表只点名登记处声明过的 GET 行，每行有理由，且算不出拒法。
   - `TestEveryRegisteredReadLineCarriesTheTargetCheck` 把豁免行归入不挂的那一类。
   - 完成记录「修后…答复不变」已改准，见清点「修前就不答 500 的」「修后答复有变」与「做法与取舍」的「不做」。
2. **Spec 非阻断：响应体变化补进完成记录。** 见清点「修后答复有变」：
   - 不读的键带坏值，由照常答改答 400；
   - 四口 400 少了 `detail` 或 `problems`；
   - 路径里小写 `%ff` 由 200 或 404 改答 400。

   发布词表口没做到仍带 `problems`。要带，只有两条路：
   - 让检查懂这一口的逐格问题形状。检查对一切读口同答一份，为一口开例外，等于把处理器的答复写第二份。
   - 把这一口也豁免。可它的分格本有 `MALFORMED_REQUEST`，不合豁免的第二条判据。

   所以改的是头注：`NewQueryPublicationVocabularyEndpoint` 写明，坏编码由装配处的检查先答 400 `MALFORMED_REQUEST`，不带 `problems`。
3. **Standards：与 ADR-0055 决定三怎么相处。** `read_target_guard.go` 文件头补一段：请求目标解码后是不是文本属传输形状，与方法、与请求行能否解析同一层，不算请求内容。未配置格对一切能解成文本的请求仍同答 403。同步改了已不成立的注释，都改成现在的真实次序：值在 Intake 之后读，只有编码由装配处的检查在处理器之前判。
   - party-commercial 四个路径参数口的行内注释：`NewQueryBusinessPartyRevisionsEndpoint`、`NewQueryLegalEntityRevisionsEndpoint`、`NewQueryProductChannelMappingRevisionsEndpoint`、`NewQueryPublicationDraftEndpoint`。
   - `NewQueryShipmentRequestViewsEndpoint` 头注里「值在 Intake 之后才读，未配置的渠道照旧先答 403」那句。
4. **Standards：`unstorableTargetRefusal` 去掉预认定。**
   - 旧形状一格只列今天有 GET 行、处理器确写旧形状的三族：主数据与运营查阅读、登记册配置写、隔离读。
   - 未配置即拒、运营决定、作业事实、集成客户端四族今天没有 GET 行，不再预先认定。哪族挂上 GET 行而没在这里定形状，登记处的 init 就停。
   - 头注已改准；完成记录「按族定形状」一条同改。
5. **`writeLegacyMalformedRequest` 第二份旧形状**：评审认为可接受，不改。

**验证（证据只记 `S`）。**
- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations tools` 无输出。
- 先红后绿：把豁免判断临时摘掉，三条用例全红——`TestExemptReadLinesAnswerExactlyAsTheirHandlersDo`（`region` 带坏值装配后答 `MALFORMED_REQUEST`）、`TestTargetCheckExemptionsNameDeclaredReadLines`、`TestEveryRegisteredReadLineCarriesTheTargetCheck`。套回去全绿。
- 全量：`go test -count=1 -p 1 ./internal/... ./cmd/... ./migrations/...` 退 0，168 个包 ok、0 FAIL（含 PG `127.0.0.1:55432`）。跑的是 `6abe252b6` 的 Go 内容。

## 评审记录（2026-11-02，非作者隔离评审）

- 初评钉 `b29cf8216`：Spec 阻断 1（guard 改了六个修前不答 500 的口的既有分格）——协调方裁这六口不挂；作者请裁的认证前检查、判整条目标两处评审认为可接受；请求体 NUL 另立票 16。
- 复评钉 `1e0019367`：两轴阻断为零。非阻断只记不追：豁免表头第二条判据宜钉成「处理器所在包的失败码集」。送站读口文件头注释过时一条，协调方进 main 时改（见下）。

## 进 main 记录（2026-11-02，协调方）

- 在 origin/main `969205ae9` 上依次 `git merge --no-ff origin/claude-docnum13`（tip `70531b003`，合并提交 `329bf3beb`）、`origin/claude-docnum14`（tip `32eb59c1f`，合并提交 `7943aa513`）、`origin/claude-docnum15`（tip `1e0019367`，合并提交 `5ce46a359`），均无冲突；三票同一次全量。
- 协调方两笔：`546861ac4`（票 13 第四轮复评非阻断——审核口所需决定种类不在已认之列时缺省答未授予，关上放行路径）、`7bc032259`（票 15 复评非阻断——送站读口门的次序注释跟上装配处的请求目标检查）。
- main tip 上重生成 ADR 索引（`b638bb595`）、AT 盘点总览计数（`ca173b8dc`）、机制清点（`f26f0ca60`）。
- 全量真库测试 `scripts/test-full.sh`：退 0，174 个包 ok。一次性库 `seed_docnum131415` 灌种子：退 0。
