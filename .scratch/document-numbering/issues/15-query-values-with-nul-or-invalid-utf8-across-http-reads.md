# 15 查询值含 NUL 或非法 UTF-8 的读口一律答 400、不读库

Category: bug
Status: in-progress——2026-11-02 作者完工、自验、推分支 `claude-docnum15`（完成记录见文末）；待非作者评审与进 main
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

**修前就不答 500 的。** 分派参数（`registry`、`register`、`family`、`kind`、`view`）是封闭集，坏值答 400。时刻参数解不开，答 400。目录翻页的 `after`、`sort` 由 `cataloguepage` 解。参考数据三口读内存表。登记册配置写族的两行 GET（价卡草稿、发布词表）与 `/document-numbering/rules` 探不到 5xx。修后它们照样挂上检查，答复不变（见下）。

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
| 本笔 | 完成记录，Status → 完工待评审 |

### 做法与取舍

- **判据一份，放在 `internal/platform/httpapi`。** `StorableText` 判单个值：合法 UTF-8 且不含 NUL。`StorableRequestTarget` 判整条请求目标：解码后的路径，与查询串里每个键、每个值。`RefuseUnstorableReadTarget` 只拦 GET，坏目标交给装配处给的拒法，处理器一步不走；别的方法原样交给处理器，405 仍由它自己答。`httpapi` 只依赖标准库，不碰任何上下文，architecture 门禁不受影响。
- **挂在 `cmd/parcel-api` 的登记处，不逐口去改。** `registerEndpoints` 登记时按行算好拒法（`unstorableTargetRefusals`）。`production` 与 `mountWith` 两个闭包出端点时都经 `guardReadTargets` 套上，生产装配与装配测试挂的是同一道。只挂 GET 行：命令口不读查询串，挂上去会让一条今天照常受理的命令因为一个它不读的键被拒。新加一行 GET 自动带上。族写漏了在 init 里就停。settlement-accounting 的处理器文件一个没动。
- **按族定形状。** 首方客户族不在装配处挂：本族先认证、后读查询串（`TestCustomerTrackingViewIntakeAuthenticatesBeforeReadingTheQueryString`），令牌不过的调用方看不到查询串校验的结果。所以它的读口仍在 Intake 里、认证之后判，`refuseUnstorableQueryValue` 改调 `httpapi.StorableText`，不留第二份判据；票 12 的 `TestUnstorableQueryValuesAreBadRequestsOnBothLines` 原样通过。其余各族处理器今天写的坏请求都是旧形状 `{"error":{"code"}}`，检查照它的字节写，由 `writeLegacyMalformedRequest` 出。真库用例拿 `/tracking-projections` 处理器自己答的坏请求（两个定位参数同在）与之逐字节比。哪一族迁到问题详情（ADR-0140 决定三的后续票），就在 `unstorableTargetRefusal` 里把它挪出那一格。
- **检查在认证之前。** 渠道未配置时，坏目标答 400 而不是 403。理由与各口把参数在场、封闭集当传输形状、先于 Intake 判相同：`/tracking-projections` 两个定位参数同在即 400，`/commercial-customer-accounts` 解码先于 Intake。判的是整条目标、每一格一视同仁，答复与端点、与键都无关，不泄露「本端点认哪些键」。
- **判整条目标，不只判各口读的那几格。** 哪几格进读只有处理器知道，在它之前判就只能一视同仁。代价：一个口不读的键带着 NUL，修前照常答，修后答 400。这条请求目标本就不是文本，所以照拒。转义坏掉或带分号的那一段，`ParseQuery` 与 `URL.Query()` 都丢下不交，检查也不判，仍归各口自己的解码。
- **不做**：没加长度上限、字符白名单。中文、全角、emoji 照收：单测与两条装配用例都证了合法多字节与 ASCII 答复相同。没改任何口的契约形状、成功答复与既有坏请求分格。transport-fulfillment 三口与若干领域构造器里原有的 NUL 判断留着没动：它们在处理器文件里，对 GET 读口已成冗余，但对别的调用面仍有用。

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

