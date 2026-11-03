# 16 写口请求体（JSON）里的字符串含 NUL 或非法 UTF-8，一律答 400、不落库、不悄悄改写

Category: bug
Status: resolved——2026-11-02 进 main（记录见文末「进 main 记录」）
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
| 运营决定（运输履约） | `/transport-fulfillment-load-assignment-registrations` | `version` | 200 `INPUT_NOT_ACCEPTED`，零行 | 201，落库并回显的版本末尾是 U+FFFD | 400，零行（评审修订后；见下） | 400，零行 |
| 作业事实登记（运输履约） | `/transport-fulfillment/movement-facts` | `location` | 200 `INPUT_NOT_ACCEPTED`，零行 | 201，落库的地点末尾是 U+FFFD | 400，零行（同上） | 400，零行 |
| 集成客户端（关务） | `/customs-regulatory-credential-registrations` | `holderRef` | 400 `MALFORMED_REQUEST`，零行 | 201，受理的是解码器换过的值 | 400，零行 | 400，零行 |
| 首方客户 | `/v1/shipment-requests` | `parcels[0].customerParcelReference` | 400，`detail` 点名该格 | 200（探针夹具无归属区间，合规载荷同答 `OWNERSHIP_UNRESOLVED`），来源保全落一行 | 400 问题详情，不带 `detail`，来源保全不落 | 同左 |
| 首方客户 | `/v1/shipment-requests` | `recipientAddress` | 200，来源保全落一行 | 同上 | 400，来源保全不落 | 同左 |
| 登记册配置写（运输履约，评审探到） | `/transport-fulfillment-delivery-dispatch-reask-cadence-registrations` | `basis` | 500 `NO_ANSWER_FORMED` | 201，坏值落行 | 400，零行 | 同左 |
| 登记册配置写（关务，评审探到） | `/customs-case-requirement-registrations` | `basis` | 500 `NO_ANSWER_FORMED` | 201，坏值落行 | 400，零行 | 同左 |

后两行的「修前」是评审修订时把套壳摘掉、在新用例里重跑得到的。

**逐行核对（修后，取证于 `c3c0018fa`）。** 登记处全部非 GET 行共 161 行：146 行经共享渠道，15 行自建 Intake。每行发两份请求体，各自先交出停在坏处之后、串还没收尾的一段，再被读就记下。停在 `\u0000` 之后的、停在原始 `0xFF` 之后的，161 行都答 400、读到坏处即停。把登记处的套壳摘掉重跑，失败 322 格，即 161 的两倍。

（`c6cd7a46c` 时 NUL 那一份只有 96 行即停，运输履约 46 行、关务 19 行按豁免读了过去；豁免已在评审修订里删掉。）

### 提交

| 提交 | 内容 |
|---|---|
| `a687cd10c` | 立票，spec 子票表加一行 |
| `a1faa4072` | 并入票 15 评审修订（`origin/claude-docnum15` `1e0019367`），无冲突 |
| `c6cd7a46c` | 共用件、登记处挂检查与 NUL 豁免、上传两行声明、单测与真库用例、两份对外契约同步、一条既有用例补注；Status 不动 |
| `9fd8c17e3` | 完成记录，Status → 完工待评审 |
| 评审修订各笔 | 见文末「评审修订」 |

### 做法与取舍

- **判据一份，放在 `internal/platform/httpapi`。** `StorableJSONBody` 包住请求体，边读边判，有三种坏处：
  - 原始字节不是 `StorableText`（含 NUL）；
  - 串里的 `\u0000` 转义；
  - `\u` 转义出孤代理。

  扫描只跟字符串与转义两种状态，所以 `\\u0000`（转义的反斜杠后面跟字面的 u0000）照收。读到坏处，这一次 Read 交回 0 字节与 `ErrUnstorableBody`，此后每次都是它。坏处所在的那一段一个字节也不交：判到时那个串还没收尾，解码器凑不成含它的值，不会带着 U+FFFD 的前半截往下走。若交出 n 字节再报错，解码器可能已在这 n 字节里凑齐一个值，先交出值、不看错。`TestADecoderNeverSeesTheChunkCarryingTheProblem` 钉着整段一次读完时的这一格。不判 JSON 语法，也不加长度上限、字符白名单。
- **挂在登记处，不逐口改。** 写法同票 15 的读口检查：
  - `registerEndpoints` 登记时按行算好检查（`requestBodyGuards`），`production` 与 `mountWith` 两个闭包都经 `guardRequestBodies` 套上，生产装配与装配测试挂的是同一道。
  - 只挂非 GET 行，GET 不读请求体。新加一行写口自动带上。
  - 没有一个上下文的处理器文件被改，settlement-accounting 也没动。
  - 没有绕过共用件的写口要改：本来就没有共用件，「去重」无从谈起。把各上下文的解码助手并成一只，要改的文件遍及清点表里的每一个上下文，所以没做。
- **自己不答，只换请求体，答复落在各口「请求体读不出」那一格。** 不在处理器之前读完再判，理由两条：
  - 各口先认证、后读请求体，有的口有用例钉「令牌不过，请求体一个字节都不读」（如 networkrouting 的 `TestNetworkOperatorRegistryIntakeAuthenticatesBeforeReadingTheBody`）。边读边判不动这个次序。
  - 各族坏请求的形状不同：旧形状带不带 `detail`，首方族是问题详情。读错落进各口自己的那一格，登记处不用再写一份形状，首方族也不必像票 15 那样另挂。也就没有按族预认定形状这件事（票 15 评审修订那一条在这里不发生）。

  代价：这一格的形状靠各口的解码助手把读错一律包成坏请求。两条逐行用例钉着它，哪一口把读错答成别的格就红。
- **NUL 一律拦，不按上下文豁免（评审修订，协调方裁）。** `c6cd7a46c` 曾按路径前缀放过运输履约、关务合规的 NUL，交各自构造门。评审真库实探：前缀罩进了修前就答 500 的口（两口的 `basis` 只判空白），两个上下文还有多处自由文本没逐一探。前缀豁免罩不准，逐格核验又不封顶，统一在入站拦下才确定不出 500。代价是这两个上下文部分口 NUL 的答复由 200 `INPUT_NOT_ACCEPTED`（或关务的未受理一类）改为 400 `MALFORMED_REQUEST`；两格都是「改输入」，恢复动作不变。
  - 节点作业、编号规则册、首方提交外部标识号的门也判 NUL，答的本就是 400 `MALFORMED_REQUEST`，挂上不改格。首方提交那一格答复少了 `detail`，见「留给协调方」。
- **上传两行放过 multipart。** 价卡预览与价卡草稿录入两行在登记处声明 `multipartUpload`。请求体是 multipart/form-data 时不判，上传的工作簿是二进制；表单里唯一的文本、文件名，由上传解码 `DecodePriceCardUpload` 判（评审修订）。其余行不看 `Content-Type`：那些处理器不看这个头、一律按 JSON 解，按头放过就是给坏请求体留门。`TestOnlyAMultipartUploadPassesTheBodyCheckUnjudged` 钉这三格。
- **判整份请求体。** 口不解的格、尾随的第二个值里的坏处，一样拒，与票 15 判整条请求目标同理。看不见的只有一种：不查尾随内容的口（例如试算、价卡登记的 `Decode*Payload`）解完头一个值就不再读，坏处若在其后、又不与那个值同在一次 Read 里，读不到也就不判。那一段不进命令，也不进库。
- **对外契约同步。**
  - [首方客户 API 契约](../../../docs/api/first-party-api-contract.md)「状态码」加一条：请求体不是文本答 `400 MALFORMED_REQUEST`，服务端不替换。
  - [承运订舱答复对接](../../../docs/api/carrier-booking-answer-integration.md)「载荷」一段：任一字符串带 NUL、非法 UTF-8、孤代理同答 400；`INPUT_NOT_ACCEPTED` 一行去掉「带 NUL」。

### 验证（证据只记 `S`）

- 新用例：
  - `internal/platform/httpapi`：
    - `TestStorableJSONBodyRefusesOnlyWhatTheStoreCannotHoldOrTheDecoderWouldRewrite`：拒收与照收两张表。拒收表含 NUL 转义、原始 NUL、非法 UTF-8 各形、孤高位与孤低位各形、第二个值里的坏处；照收表含中文、全角、emoji、代理对转义、`\\u0000`、`\u0001`、显式 U+FFFD。每张表都在一次读完、逐字节读、每次读一半三种读法下跑。
    - `TestADecoderNeverSeesTheChunkCarryingTheProblem`。
    - `TestRequireStorableJSONBodyOnlySwapsTheBody`：处理器不读则一个字节不读，坏处到处理器手上是 `ErrUnstorableBody`，合法请求体原样到。
  - `cmd/parcel-api`：
    - `TestEveryWriteLineReadsItsBodyThroughTheCheck`：经共享渠道的写口，各族渠道打开、授予与准入齐备，读口与编排是未接线占位。
    - `TestEveryOwnIntakeWriteLineReadsItsBodyThroughTheCheck`：自建 Intake 的写口，真库、登记处生产构造、授予齐备的合成铸造器。
    - `TestOnlyAMultipartUploadPassesTheBodyCheckUnjudged`。
    - `TestUnstorableBodiesNeverReachTheRegistersAgainstARealDatabase`：票 15 探到的三个登记口，加评审探到的运输履约重问节奏、关务建案要求两口，经生产构造接真库，三种坏处都答 400、零行。参与方名称换成中文、emoji 原样、代理对转义、`\\u0000` 时答 201，读回逐字相同。
    - `TestEachFamilyAnswersUnstorableBodiesWithItsOwnBadRequestAgainstARealDatabase`：运营决定（装载分配）、作业事实（移动事实）、集成客户端（监管凭证）、首方（提交，问题详情）各一口，三种坏处都答 400、零行，合规载荷照常。
  - `internal/parcelpricing/adapters/http`：`TestUploadIntakesRefuseAFileNameTheStoreCannotHold`（评审修订，见下）。
- 先红后绿：把 `registerEndpoints` 两个闭包里的 `bodyGuards` 换成 nil，逐行两条用例失败 322 格，登记口那条也红：五口 NUL 都答 500 或落行，坏值落行。套回去全绿。
- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations tools` 无输出。
- `go test -count=1 -p 1 ./internal/... ./cmd/... ./migrations/...` 退 0：168 个包 ok、0 FAIL（含 PG `127.0.0.1:55432`），`internal/architecture`、`internal/platform/httpapi`、`cmd/parcel-api` 都在内。跑的是 `c6cd7a46c` 的 Go 内容（已并入票 15 修订）；评审修订后的全量见「评审修订」。
- `bash scripts/ci/generated-artifacts-pr.sh` 退 0：ADR 索引、AT 计数与提交的一致；机制清点过期，留推送方重生成。
- 既有用例不改断言。直挂处理器、钉首方外部标识号门的 `TestTheWiredSubmissionRefusesACustomerParcelReferenceTheRegisterCannotStore` 照旧绿，注释补一句：生产路由上带 NUL 的那一份由请求体检查先答 400、不带 `detail`。运输履约与关务钉 NUL 的用例（`storable_values_channel_test.go` 三条、`TestIntegrationClientCredentialRegistrationJudgesStorableValuesOnRealDB`）也直挂处理器，照旧绿；它们钉的是检查背后的构造门，生产路由上 NUL 先由检查答 400。

### 留给协调方

- **挂上即改的答复。**
  - 所有写口的非法 UTF-8 与孤代理：从「换成 U+FFFD 照常受理」改为 400、不落库。这是第一条裁定要的。
  - 运输履约、关务合规部分口的 NUL：从 200 `INPUT_NOT_ACCEPTED`（或关务的未受理一类）改为 400。这是评审修订的裁定，理由见「做法与取舍」。
  - 首方提交客户包裹件号带 NUL：仍是 400 `MALFORMED_REQUEST`，但问题详情不再带 `detail` 点名 `parcels[0].customerParcelReference`。判在解码之前，读错到不了点名那一格的门。契约写明 `detail` 是散文、不据此分支，所以按「不改格」处理。
  - 首方提交其余各格（如 `recipientAddress`）带 NUL：修前答 200，来源保全落一行；修后 400、不落。修前那一格没有哪张票为它设过。
  - 不查尾随内容的口：尾随的第二个值里带坏处，修前照收头一个值；修后与那个值同在一次 Read 里时答 400。这是判整份请求体的结果，同票 15。
- **两份对外契约改了，属改现行规则的文档。** 首方客户 API 契约「状态码」加一条；承运订舱答复对接「载荷」加一句，`INPUT_NOT_ACCEPTED` 一行去掉「带 NUL」。
- **生成物不重生成**（作者不占号），交推送方。

## 评审修订（2026-11-02，作者，钉评审 `9fd8c17e3`：Standards 阻断 0、Spec 阻断 1、非阻断 1）

先并 `origin/main` `0d3d23d21`（票 13、14、15 已进 main），合并笔 `926a86b57`。冲突只在 spec 子票表，以 main 为准，票 16 那一行接在其后。

1. **阻断（Spec）：NUL 豁免罩进了修前就答 500 的口。协调方裁：删豁免，一律 400。**
   - 评审真库实探：`/transport-fulfillment-delivery-dispatch-reask-cadence-registrations` 的 `basis` 与 `/customs-case-requirement-registrations` 的 `basis` 都只判空白，带 NUL 在豁免下仍答 `500 NO_ANSWER_FORMED`。
   - 改法：
     - 删 `cmd/parcel-api` 的 `bodyNULExemptions` 与挂法；`requestBodyGuard` 回到只看方法与 `multipartUpload`。
     - 删 `httpapi.UnrewrittenJSONBody` 及其单测，那是只给豁免用的。
     - 文件头写明不按上下文豁免的理由：前缀罩不准，逐格核验不封顶。
   - 用例：
     - 逐行两条改回两份坏处都即停；删 `TestBodyNULExemptionsCoverDeclaredWriteLines`。
     - 四族代表口那条里，装载分配、移动事实的 NUL 断言改为 400。
     - 登记口真库那条补上面两口：三种坏处答 400、库里无行，合规载荷答 201。套壳摘掉时这两口 NUL 答 500，非法 UTF-8 落行。
   - 文档：承运订舱答复对接「载荷」一段改成 NUL 也同答 400；`INPUT_NOT_ACCEPTED` 一行去掉「带 NUL」。
   - 完成记录「做法与取舍」「代表口实测」「逐行核对」已改准。运输履约、关务部分口 NUL 的答复由 200 `INPUT_NOT_ACCEPTED`（关务为未受理一类）改为 400。理由：前缀豁免罩不准、逐格核验不封顶，统一在入站拦下才确定不出 500。
2. **非阻断：价卡上传的文件名没判。顺手修了。**
   - `pricinghttp.DecodePriceCardUpload` 在拿到文件格之后用 `httpapi.StorableText` 判 `header.Filename`。不合格包 `ErrMalformedRequest`，答该口既有坏请求那一格 400，工作簿不读、不解。预览与录入两口共用这一个解码，一处改两口都管。
   - 用例 `TestUploadIntakesRefuseAFileNameTheStoreCannotHold`：Intake 级，不用工作簿夹具。`filename*=UTF-8''SYN%00.xlsx`、`%FF`、`%ED%A0%80` 三种，预览与录入两口都答 `ErrMalformedRequest`；`价卡📦.xlsx` 照收、原样进命令。把这道判摘掉时六格全红：Go 的 multipart 确实原样解出 NUL 与坏字节。
   - 登记处守卫的注释补了一句：文件名由上传解码自己判。

**提交**：`926a86b57`（并 main）、`c3c0018fa`（修订代码与文档）、本笔（票面）。

**验证**（证据只记 `S`）：
- `go build ./...`、`go vet ./...` 退 0；`gofmt -l cmd internal migrations tools` 无输出。
- `go test -count=1 -p 1 ./internal/... ./cmd/... ./migrations/...` 退 0：168 个包 ok、0 FAIL（含 PG `127.0.0.1:55432`），`internal/architecture`、`internal/parcelpricing/adapters/http`、`internal/platform/httpapi`、`cmd/parcel-api` 都在内。跑的是 `c3c0018fa` 的 Go 内容。
- 先红后绿：
  - 套壳摘掉，逐行两条失败 322 格；登记口那条五口都红。
  - 文件名那道判摘掉，新用例六格红。
  - 两处套回去全绿。

## 评审记录（2026-11-02，非作者隔离评审）

- 初评钉 `9fd8c17e3`：Standards 阻断 0、Spec 阻断 1（`bodyNULExemptions` 按前缀放过运输履约、关务的 NUL，罩进了修前就答 500 的口——重问节奏登记与建案要求登记的 `basis`）。协调方裁删豁免、一律 400（豁免是协调方上一轮提议的，判断失误）。非阻断：价卡上传文件名可带 NUL / 非法 UTF-8 进库——作者顺手修。
- 复评钉 `2cd60c8c0`：两轴阻断为零。非阻断：`cmd/parcel-api/storable_values_channel_test.go` 文件头与 Covers 仍说真渠道 NUL 答 200——协调方进 main 时注明端点直挂（见下）。

## 进 main 记录（2026-11-02，协调方）

- 在 origin/main `0d3d23d21` 上 `git merge --no-ff origin/claude-docnum16`（tip `2cd60c8c0`），无冲突；合并提交 `7deef3b72`。
- 协调方一笔 `659a5bebd`：真渠道 NUL 用例注明端点直挂、生产路由先由请求体检查答 400。
- main tip 上重生成机制清点（`6b449ce6b`）；ADR 索引、AT 盘点计数无变化。
- 全量真库测试 `scripts/test-full.sh`：退 0，174 个包 ok。一次性库 `seed_docnum16` 灌种子：退 0。
- 无迁移；演示环境换 parcel-api 二进制。
