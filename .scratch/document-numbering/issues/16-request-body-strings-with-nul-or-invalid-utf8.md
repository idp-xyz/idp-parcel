# 16 写口请求体（JSON）里的字符串含 NUL 或非法 UTF-8，一律答 400、不落库、不悄悄改写

Category: bug
Status: in-progress——2026-11-02 作者完工、自验、推分支 `claude-docnum16`（完成记录见文末）；待非作者评审与进 main。此前 in-progress——2026-11-02 作者认领，分支 `claude-docnum16`，从票 15 的 `b29cf8216` 起
父票：[spec](../spec.md)
Blocked by: 无（建在票 [15](./15-query-values-with-nul-or-invalid-utf8-across-http-reads.md) 之上：共用判据 `httpapi.StorableText` 与登记处挂检查的做法都出自那一票）
出处：
- 票 15 完成记录：「请求体（不在本票修）」一节，与「留给协调方」的「请求体 NUL 答 500，另立票」一条。
- 协调方 2026-11-02 裁定：NUL 与非法 UTF-8 两种都拒，答该族既有坏请求形状的 400，处理器之后的写一笔都不发生。非法 UTF-8 不替换成 U+FFFD：悄悄改写调用方送来的值，比拒收更糟。

## 现象

票 15 完成记录实测于 `969205ae9`，真库：

- 请求体 JSON 字符串里写 `\u0000`，解码后进命令的是真 NUL。
- `/commercial-business-party-registrations`、`/visibility-catalogue-milestone-mapping-registrations`、`/network-catalog-node-registrations` 三口都答 `500 NO_ANSWER_FORMED`：库拒收，调用方据 5xx 重发一个永远不会好的请求（ADR-0029 按恢复动作分格）。
- 载荷里的原始非法 UTF-8 字节，被 `encoding/json` 换成 U+FFFD 后照常受理。node 登记答 201，落库的值已被悄悄改过。

## 要做的

1. **清点。** 各族写口（POST / PUT / PATCH）在哪里读请求体：经共用解码件，还是各自解码。代表口探三种坏值今天答什么、落了什么：
   - JSON 字符串里的 `\u0000`；
   - 原始非法 UTF-8 字节；
   - JSON 转义的孤代理（如 `\ud800`，`encoding/json` 换成 U+FFFD）。
2. **修。** 能放一处就放一处。请求体出现下面任一种，就答该族既有「请求体读不出」那一格的 400，处理器之后的写一笔都不发生：
   - 任一字符串含 NUL；
   - 原始字节不是合法 UTF-8；
   - `\u` 转义出孤代理。

   非法 UTF-8 不替换。首方族同样拦，答首方族的坏请求形状（问题详情）。
3. **测试。**
   - 共用件单测：NUL 转义、原始非法字节、孤代理转义都拒；合法中文、emoji、代理对、`\\u0000`（转义的反斜杠）照收。
   - 每族一个代表写口的 HTTP 用例，断言 400 且不落库。
   - `cmd/parcel-api` 真库测试：票 15 探到的三个登记口修后答 400，库里无行。

## 不做

- 不改任何口的契约形状、成功答复与既有坏请求分格。
- 不加长度上限、字符白名单。
- 不管票 13 的账单号规整（那是领域规则，另有票）。

## 完成判据

- 清点表（口族、解码方式、修前、修后）写进完成记录。
- 受影响包、`internal/architecture` 与 `cmd/...` 真库测试过。`go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出。
- 照「进 main 的步骤」交非作者评审。

## 完成记录（2026-11-02，作者，分支 `claude-docnum16`，从票 15 的 `b29cf8216` 起；已并入票 15 评审修订 `1e0019367`）

### 清点

**解码方式（静态，取证于 `b29cf8216`）。** 没有各上下文共用的 JSON 解码件。写口的请求体由各上下文适配器里自己的助手解，没有一口经 `internal/platform` 的件：

| 上下文 | 写口所属族 | 解请求体的助手 | 读错落哪 |
|---|---|---|---|
| party-commercial（含 `/reference-data/enablement-registrations`） | 登记册配置写 | 包内 `decodeStrict`，经 `authenticateAndDecode`、`decodeClosedDocument`、`DecodeCommercialPublicationPayload`、`DecodePublicationDraftReferencePayload` | `ErrMalformedRequest` → 400 旧形状，带 `detail` |
| customs-compliance | 登记写 / 运营决定 / 集成客户端 | `translateOnline`（`io.ReadAll` 后交 registrationjson 的 `decodeStrict`）；`decodeClosedPayload`；`readSinglePayload`；`decodeClosedDocument` | `ErrMalformedRequest` → 400 |
| network-routing | 登记写 / 运营决定 | `translateOnline`；`decodeClosed` | `ErrMalformedRequest` → 400 |
| visibility-exception | 登记写 / 首方索赔 | `translateOnline`；`decodeClaimDraft` | `ErrMalformedRegistration` → 400；`ErrMalformedClaim` → 400 问题详情 |
| node-operations | 登记写、运营决定、作业事实、首方预报与交接确认 | `decodeClosedDocument(request)` | `ErrMalformedRequest` → 400（首方各口为问题详情） |
| parcel-pricing | 登记写、试算 | 每种载荷一只 `Decode*Payload`；结构化工作簿 `readWorkbookJSON`；上传 `DecodePriceCardUpload`（multipart） | `ErrMalformedRequest` → 400 |
| parcel-shipment | 运营决定 / 首方各口 | `decodeDecisionPayload`；`decodeJSONPayload` | `ErrMalformedRequest` → 400（首方为问题详情） |
| transport-fulfillment | 登记写、运营决定、作业事实、集成客户端 | `decodeClosedPayload`；判断两口与有效时间规则口各一只 `Decode*Payload` | `ErrMalformedRequest` → 400 |
| settlement-accounting | 运营决定 / 集成客户端 | `readSinglePayload`（之后交 registrationjson 的 `decodeStrict`） | `ErrMalformedRequest` → 400 |
| 平台编号规则册 | 登记写、查阅读（预览） | `docnumberhttp` 的 `decodeClosedDocument` | `errMalformed` → 400 |

各助手有一处相同：读错与解码错一律包成本包的坏请求哨兵，答 400。修法靠的就是这一处（见「做法与取舍」）。上传两口（价卡预览、价卡草稿录入）另收 multipart/form-data，按 `Content-Type` 分派：`application/json` 走结构化工作簿，其余走上传解码。

**谁的构造门已经判 NUL（静态）。**

- 运输履约：构造门拒 NUL、答未受理。依据是票 tf-face-algebra/01、/14 的裁定，ADR-0213、ADR-0303，承运订舱答复对接文档的 `INPUT_NOT_ACCEPTED` 一行。
- 关务合规：domain 的 `newRequiredValue` 与报送事实门拒 NUL。在编排里构造的格答未受理一类业务答案（如技术回执的 `TechnicalReceiptNotAccepted`），在译装时构造的格答 400。
- 节点作业：适配器里判 NUL，答 400。
- 编号规则册：时区格判 NUL，答 400。
- 首方提交：外部标识号的门（ADR-0184）拒 NUL，答 400，`detail` 点名那一格。
- 其余上下文不判 NUL，NUL 进写答 500。结算的 `newRequiredValue` 只判空白。

**代表口实测（真库，修前于 `b29cf8216` 的行为，修后于本分支）。** 临时探针，不入库。每口带一份合规载荷，再带三份只差一格的载荷：JSON 串里写 `\u0000`、串里放原始 `0xFF` 字节、串里写 `\ud800`。

| 族 | 口 | 格 | `\u0000` 修前 | `0xFF`、`\ud800` 修前 | `\u0000` 修后 | `0xFF`、`\ud800` 修后 |
|---|---|---|---|---|---|---|
| 登记册配置写 | `/commercial-business-party-registrations` | `name` | 500 `NO_ANSWER_FORMED`，零行 | 201，落库的名称里坏处换成了 U+FFFD | 400 `MALFORMED_REQUEST`（带 `detail`），零行 | 同左 |
| 登记册配置写 | `/visibility-catalogue-milestone-mapping-registrations` | `version` | 500，零行 | 200 `REFUSED`：解码器已把版本换成带 U+FFFD 的值才去判；本探针与前一版区间重叠，没落行 | 400，零行 | 同左 |
| 登记册配置写 | `/network-catalog-node-registrations` | `code` | 500，零行 | 201，落库的代码末尾是 U+FFFD | 400，零行 | 同左 |
| 运营决定（运输履约） | `/transport-fulfillment-load-assignment-registrations` | `version` | 200 `INPUT_NOT_ACCEPTED`，零行 | 201，落库并回显的版本末尾是 U+FFFD | 不变：200 `INPUT_NOT_ACCEPTED`，零行 | 400，零行 |
| 作业事实登记（运输履约） | `/transport-fulfillment/movement-facts` | `location` | 200 `INPUT_NOT_ACCEPTED`，零行 | 201，落库的地点末尾是 U+FFFD | 不变 | 400，零行 |
| 集成客户端（关务） | `/customs-regulatory-credential-registrations` | `holderRef` | 400 `MALFORMED_REQUEST`，零行 | 201，受理的是解码器换过的值 | 不变：400，零行 | 400，零行 |
| 首方客户 | `/v1/shipment-requests` | `parcels[0].customerParcelReference` | 400，`detail` 点名该格 | 200（探针夹具无归属区间，合规载荷同答 `OWNERSHIP_UNRESOLVED`），来源保全落一行 | 400 问题详情，不带 `detail`，来源保全不落 | 同左 |
| 首方客户 | `/v1/shipment-requests` | `recipientAddress` | 200，来源保全落一行 | 同上 | 400，来源保全不落 | 同左 |

**逐行核对（修后，取证于 `c6cd7a46c`）。** 登记处全部非 GET 行共 161 行：146 行经共享渠道，15 行自建 Intake。每行发两份请求体，各自先交出停在坏处之后、串还没收尾的一段，再被读就记下：

- 停在原始 `0xFF` 之后的那一份：161 行都答 400，读到坏处即停。
- 停在 `\u0000` 之后的那一份：96 行答 400、即停。运输履约 46 行、关务 19 行读了过去，留给构造门。

把登记处的套壳摘掉重跑，失败 257 格，即 161 加 96。

### 提交

| 提交 | 内容 |
|---|---|
| `a687cd10c` | 立票，spec 子票表加一行 |
| `a1faa4072` | 并入票 15 评审修订（`origin/claude-docnum15` `1e0019367`），无冲突 |
| `c6cd7a46c` | 共用件、登记处挂检查与 NUL 豁免、上传两行声明、单测与真库用例、两份对外契约同步、一条既有用例补注；Status 不动 |
| 本笔 | 完成记录，Status → 完工待评审 |

### 做法与取舍

- **判据一处，放在 `internal/platform/httpapi`。** `StorableJSONBody` 包住请求体，边读边判，有三种坏处：
  - 原始字节不是 `StorableText`（含 NUL）；
  - 串里的 `\u0000` 转义；
  - `\u` 转义出孤代理。

  `UnrewrittenJSONBody` 与它共用同一个扫描，只放过 NUL。扫描只跟字符串与转义两种状态，所以 `\\u0000`（转义的反斜杠后面跟字面的 u0000）照收。读到坏处，这一次 Read 交回 0 字节与 `ErrUnstorableBody`，此后每次都是它。坏处所在的那一段一个字节也不交：判到时那个串还没收尾，解码器凑不成含它的值，不会带着 U+FFFD 的前半截往下走。若交出 n 字节再报错，解码器可能已在这 n 字节里凑齐一个值，先交出值、不看错。`TestADecoderNeverSeesTheChunkCarryingTheProblem` 钉着整段一次读完时的这一格。不判 JSON 语法，也不加长度上限、字符白名单。
- **挂在登记处，不逐口改。** 写法同票 15 的读口检查：
  - `registerEndpoints` 登记时按行算好检查（`requestBodyGuards`），`production` 与 `mountWith` 两个闭包都经 `guardRequestBodies` 套上，生产装配与装配测试挂的是同一道。
  - 只挂非 GET 行，GET 不读请求体。新加一行写口自动带上。
  - 没有一个上下文的处理器文件被改，settlement-accounting 也没动。
  - 没有绕过共用件的写口要改：本来就没有共用件，「去重」无从谈起。把各上下文的解码助手并成一只，要改的文件遍及清点表里的每一个上下文，所以没做。
- **自己不答，只换请求体，答复落在各口「请求体读不出」那一格。** 不在处理器之前读完再判，理由两条：
  - 各口先认证、后读请求体，有的口有用例钉「令牌不过，请求体一个字节都不读」（如 networkrouting 的 `TestNetworkOperatorRegistryIntakeAuthenticatesBeforeReadingTheBody`）。边读边判不动这个次序。
  - 各族坏请求的形状不同：旧形状带不带 `detail`，首方族是问题详情。读错落进各口自己的那一格，登记处不用再写一份形状，首方族也不必像票 15 那样另挂。也就没有按族预认定形状这件事（票 15 评审修订那一条在这里不发生）。

  代价：这一格的形状靠各口的解码助手把读错一律包成坏请求。两条逐行用例钉着它，哪一口把读错答成别的格就红。
- **NUL 的豁免（协调方 2026-11-02 第二条指示：修前本不答 500 的写口，挂上会改既有分格的，照票 15 的豁免思路办）。**
  - 进表照 `targetCheckExemptions` 的两条判法：修前 NUL 不答 500，且挂上会改那一格。
  - 运输履约、关务合规两个上下文的构造门按产品规则拒 NUL，答各自既有的一格（多是 200 的未受理一类）；挂上会改成 400 `MALFORMED_REQUEST`，于是进 `bodyNULExemptions`。表在 `cmd/parcel-api` 的请求体检查旁边，按路径前缀写。
  - 只豁免 NUL：这两个上下文的写口照判非法 UTF-8 与孤代理。那两种修前是被悄悄改写成 U+FFFD 受理，没有哪一格是为它们设的；协调方第一条裁定要求拒收。
  - 按上下文豁免、不逐行列，因为判 NUL 的是上下文的构造门，规则管整个上下文，新加的写口照样归那道门。`TestBodyNULExemptionsCoverDeclaredWriteLines` 核每个前缀都罩得住声明过的写口。
  - 节点作业、编号规则册、首方提交外部标识号的门也判 NUL，但答的就是 400 `MALFORMED_REQUEST`，挂上不改格，不进表。首方提交那一格答复少了 `detail`，见「留给协调方」。
- **上传两行放过 multipart。** 价卡预览与价卡草稿录入两行在登记处声明 `multipartUpload`。请求体是 multipart/form-data 时不判，上传的工作簿是二进制。其余行不看 `Content-Type`：那些处理器不看这个头、一律按 JSON 解，按头放过就是给坏请求体留门。`TestOnlyAMultipartUploadPassesTheBodyCheckUnjudged` 钉这三格。
- **判整份请求体。** 口不解的格、尾随的第二个值里的坏处，一样拒，与票 15 判整条请求目标同理。看不见的只有一种：不查尾随内容的口（例如试算、价卡登记的 `Decode*Payload`）解完头一个值就不再读，坏处若在其后、又不与那个值同在一次 Read 里，读不到也就不判。那一段不进命令，也不进库。
- **对外契约同步。**
  - [首方客户 API 契约](../../../docs/api/first-party-api-contract.md)「状态码」加一条：请求体不是文本答 `400 MALFORMED_REQUEST`，服务端不替换。
  - [承运订舱答复对接](../../../docs/api/carrier-booking-answer-integration.md)「载荷」一段：非法 UTF-8 与孤代理同答 400。标识带 NUL 仍是 `INPUT_NOT_ACCEPTED`，那一行不动。

### 验证（证据只记 `S`）

- 新用例：
  - `internal/platform/httpapi`：
    - `TestStorableJSONBodyRefusesOnlyWhatTheStoreCannotHoldOrTheDecoderWouldRewrite`：拒收与照收两张表。拒收表含 NUL 转义、原始 NUL、非法 UTF-8 各形、孤高位与孤低位各形、第二个值里的坏处；照收表含中文、全角、emoji、代理对转义、`\\u0000`、`\u0001`、显式 U+FFFD。每张表都在一次读完、逐字节读、每次读一半三种读法下跑。
    - `TestUnrewrittenJSONBodyLetsOnlyNULThrough`：只有 NUL 的那几份照交，其余照拒。
    - `TestADecoderNeverSeesTheChunkCarryingTheProblem`。
    - `TestRequireStorableJSONBodyOnlySwapsTheBody`：处理器不读则一个字节不读，坏处到处理器手上是 `ErrUnstorableBody`，合法请求体原样到。
  - `cmd/parcel-api`：
    - `TestEveryWriteLineReadsItsBodyThroughTheCheck`：经共享渠道的写口，各族渠道打开、授予与准入齐备，读口与编排是未接线占位。
    - `TestEveryOwnIntakeWriteLineReadsItsBodyThroughTheCheck`：自建 Intake 的写口，真库、登记处生产构造、授予齐备的合成铸造器。
    - `TestBodyNULExemptionsCoverDeclaredWriteLines`。
    - `TestOnlyAMultipartUploadPassesTheBodyCheckUnjudged`。
    - `TestUnstorableBodiesNeverReachTheRegistersAgainstARealDatabase`：票 15 探到的三个登记口经生产构造接真库，三种坏处都答 400、零行。参与方名称换成中文、emoji 原样、代理对转义、`\\u0000` 时答 201，读回逐字相同。
    - `TestEachFamilyAnswersUnstorableBodiesWithItsOwnBadRequestAgainstARealDatabase`：运营决定（装载分配）、作业事实（移动事实）、集成客户端（监管凭证）、首方（提交，问题详情）各一口。非法 UTF-8 与孤代理都答 400、零行；NUL 在前三口答各自构造门的那一格（两口 200 `INPUT_NOT_ACCEPTED`，凭证 400），首方答 400；合规载荷照常。
- 先红后绿：把 `registerEndpoints` 两个闭包里的 `bodyGuards` 换成 nil，逐行两条用例失败 257 格（161 加 96），三登记口那条也红：NUL 答 500，坏值落行。套回去全绿。
- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations tools` 无输出。
- `go test -count=1 -p 1 ./internal/... ./cmd/... ./migrations/...` 退 0：168 个包 ok、0 FAIL（含 PG `127.0.0.1:55432`），`internal/architecture`、`internal/platform/httpapi`、`cmd/parcel-api` 都在内。跑的是 `c6cd7a46c` 的 Go 内容（已并入票 15 修订）。
- `bash scripts/ci/generated-artifacts-pr.sh` 退 0：ADR 索引、AT 计数与提交的一致；机制清点过期，留推送方重生成。
- 既有用例不改断言。直挂处理器、钉首方外部标识号门的 `TestTheWiredSubmissionRefusesACustomerParcelReferenceTheRegisterCannotStore` 照旧绿，注释补一句：生产路由上带 NUL 的那一份由请求体检查先答 400、不带 `detail`。运输履约与关务钉 NUL 的用例（`storable_values_channel_test.go` 三条、`TestIntegrationClientCredentialRegistrationJudgesStorableValuesOnRealDB`）在生产路由上依旧成立，没动。

### 留给协调方

- **NUL 豁免照第二条指示做了，请裁是否保留。**
  - 运输履约、关务合规两个上下文的写口，NUL 交各自的构造门，只判非法 UTF-8 与孤代理。
  - 证据：运输履约有裁定与对接文档，两口实测 200 `INPUT_NOT_ACCEPTED`。关务静态读到构造门拒 NUL（技术回执答 `TechnicalReceiptNotAccepted`），凭证口实测 400。
  - 没有逐格探。这两个上下文若有哪一格的构造门漏判 NUL，那一格修后仍答 500，归该上下文另立票。
  - 若要回到第一条裁定的「一律 400」：删 `bodyNULExemptions` 两行，`TestEachFamilyAnswersUnstorableBodiesWithItsOwnBadRequestAgainstARealDatabase` 两口的 NUL 断言改回 400，订舱答复对接文档 `INPUT_NOT_ACCEPTED` 一行去掉「带 NUL」。
- **挂上即改、没有豁免的答复。**
  - 所有写口的非法 UTF-8 与孤代理：从「换成 U+FFFD 照常受理」改为 400、不落库。这是第一条裁定要的。
  - 首方提交客户包裹件号带 NUL：仍是 400 `MALFORMED_REQUEST`，但问题详情不再带 `detail` 点名 `parcels[0].customerParcelReference`。判在解码之前，读错到不了点名那一格的门。契约写明 `detail` 是散文、不据此分支，所以按「不改格」处理。若要保留点名，只能让首方族也豁免 NUL，那样地址等格的 NUL 又会照收（见下一条）。
  - 首方提交其余各格（如 `recipientAddress`）带 NUL：修前答 200，来源保全落一行；修后 400、不落。修前那一格没有哪张票为它设过。
  - 不查尾随内容的口：尾随的第二个值里带坏处，修前照收头一个值，修后与那个值同在一次 Read 里时答 400。这是判整份请求体的结果，同票 15。
- **两份对外契约改了，属改现行规则的文档，请评审一并过目。** 首方客户 API 契约「状态码」加一条；承运订舱答复对接「载荷」加一句。
- **生成物不重生成**（作者不占号），交推送方。
