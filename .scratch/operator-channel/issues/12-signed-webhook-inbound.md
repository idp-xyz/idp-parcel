# 12 签名 webhook 入向：只能推送的外部源经签名校验后以集成客户端身份进入

Category: enhancement
Status: resolved——2026-10-14 通道 1 把补笔重放到已进 main 的 `a1360371` 之上：`1ad7721a`→`79acc46f`、`4f488979`→`7b09abb9`，代码仍是 `13f34a33`。此前：2026-10-14 13:14 通道 1 重放进 main：远端 main 仍是 `210eafac`，`mcp4-oc12` 是它的快进，作者五笔 SHA 原样进入（`f13d344b` 认领、`f185673c`、`13f34a33` 代码、`ab88fb41` 清点、`1b1d4460` 完成记录）。评审无阻断。此前：完工，待评审与重放——2026-10-14 通道 4。分支 `mcp4-oc12`，基 `210eafac`（派单写的 `b3c8bd46` 在续做时已不是远端 main，照派单自新 tip 起），代码 tip `13f34a33`，清点 `ab88fb41`。认领与此前 ready-for-agent 见完成记录。
Blocked by: 11
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：入向连接器的签名校验件（平台件或 `internal/accessidentity`，本票定落点）；各上下文入向连接器接它的那一处归 psb/09、10、12 的连接器项。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三第三、四条。

## 做什么

1. 签名校验：HMAC 共享密钥与对方公钥两种形态；时间戳与重放窗口；密钥只存引用（租户取值）。
2. 校验通过后以该推送源绑定的集成客户端身份铸信封；签名不过、时间戳越窗、重放各答其格。

## 完成判据

- 两种签名形态各有用例；同一推送重放被识别；密钥轮换期新旧并存可配置。

## 完成记录

认领：2026-10-14 12:2x 通道 4（派单 task-8669890e ← 通道 1），`f13d344b`。派单写的基 `b3c8bd46` 在 10-13 22:06 建树后会话中断、一笔未落；续做时远端 main 已至 `210eafac`（其间进 main 的有票 18、19，19 改过本票要接的铸造器），照派单自新 tip 起。此前 ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）。

各笔（均已推 `origin/mcp4-oc12`，未进 main）：

- `f185673c` 领域：`SignedPush` / `ParseStandardWebhook`、`SignedPushMinter`，与令牌入口共用 `admitIntegrationClient`
- `13f34a33` `access_identity` 0006（推送源、签名密钥、停用、重放账）、postgres 登记口与装载口、`parcel-access-register` 三个子命令、`PAR-INT-12`
- `ab88fb41` 机制清点，干净检出钉 `13f34a33`

落点：`internal/accessidentity`，不另起平台件。见判断项 1。

证据层级只记 `S`。用例密钥在测试内生成，批文与迁移不放密钥本体，登记册当前登记为待提供。

### 判据

- 两种签名形态各有用例 ✅ `TestHMACSignedPushIsMintedAsTheBoundIntegrationClient`、`TestEd25519SignedPushIsMintedAsTheBoundIntegrationClient`。真库登记口 `TestPushSourceRegisterKeyAndRetireEndToEnd` 同一批登 `HMAC_SHA256` 与 `ED25519`，只存引用。
- 同一推送重放被识别 ✅ `TestTheSamePushReplayedIsRecognisedButARetrySignedAfreshIsNot`：同一份签名原文第二次答 `ErrPushReplayed`；同一推送标识、新时间戳、重签的重试不是重放。真库 `TestPushReceiptRecognisesTheSameSignedContentOnce` PASS、非 SKIP：摘要键第一次 `first=true`、第二次 `false`，换时间戳是另一行。
- 密钥轮换期新旧并存可配置 ✅ `TestKeyRotationAcceptsOldAndNewKeysExactlyWhileBothAreEffective`：重叠多久由登记上的起点与停用时刻决定，重叠期内任一把单签或两把并签都收，新钥起点前只认旧钥，旧钥停用后只认新钥。真库 `TestPushSigningKeyRotationIsARegistration` 与上面的登记口用例读回同一形状。产品不设重叠时长缺省。

做什么第 2 条的三格与依赖故障 ✅ `TestSignedPushAnswerGradesDoNotStandInForEachOther`：签名不过、越窗、重放互不认出；推送源册、受限存储、重放账、集成客户端册读写不动各答自己的依赖故障，不折进上面三格；受限存储没配答 `ErrAccessChannelNotConfigured`。窗口两端含在内：`TestSignedPushAtTheEdgesOfTheWindowIsAccepted`。

### 没做的

各上下文入向连接器接这一处，归 psb/09、10、12。`cmd/parcel-api` 端点表未碰。准入范围桥不在本票；请求带了 `Admission` 时照 `admitIntegrationClient` 走，`TestSignedPushAdmissionFollowsTheIntegrationClientMinter` 钉未配权威串答不在准入范围、读不动另答一格。

### 验证

工作树 `idp-parcel-mcp4-oc12`，go1.26.8。`go build ./...` 退 0；`go vet ./...` 退 0。`gofmt -w` 后提交，新 `.sql` 无 CR、无 BOM。

- `go test -count=1 ./internal/accessidentity/` 退 0
- 带 `IDP_PARCEL_POSTGRES_DSN=postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`：`go test -count=1 -p 1 ./internal/accessidentity/adapters/postgres/ ./cmd/parcel-access-register/` 退 0；`-v -run TestPush` 四例 PASS、非 SKIP；`TestPushSourceWritesRefuseToRunOutsideATransaction` PASS
- 同上 DSN：`go test -count=1 -p 1 ./migrations/ ./internal/platform/migrate/ ./internal/architecture/... ./cmd/parcel-api/ ./internal/accessidentity/adapters/oidc/ ./internal/accessidentity/adapters/pilotgovernance/ ./internal/customscompliance/adapters/accessidentity/ ./internal/networkrouting/adapters/accessidentity/ ./internal/parcelpricing/adapters/accessidentity/ ./internal/parcelshipment/adapters/accessidentity/ ./internal/partycommercial/adapters/accessidentity/ ./internal/settlementaccounting/adapters/accessidentity/ ./internal/transportfulfillment/adapters/accessidentity/ ./internal/visibilityexception/adapters/accessidentity/` 退 0（`cmd/parcel-api` 是反向依赖里的 cmd，带 DSN）

未跑全仓 `go test ./...`（作者那层按 parallel-sessions「全量只跑一次」，全量留给推送方）。

变异自证（`1ad7721a`，副本在 `/tmp/oc12-mut`，`go test -overlay`）：九处都退 1。M2、M7 第一轮存活，补了 `TestAnEd25519SignatureFromAnotherKeyIsRejected` 与 `TestALaterKeyOfTheSameFormStillVerifiesWhenTheEarlierOneDoesNotMatch` 之后再跑退 1。

- M1 HMAC 验签失败仍当通过 → `TestSignedPushAnswerGradesDoNotStandInForEachOther` 退 1
- M2 Ed25519 验签失败仍当通过 → `TestAnEd25519SignatureFromAnotherKeyIsRejected` 退 1
- M3 重放不答 `ErrPushReplayed` → `TestTheSamePushReplayedIsRecognisedButARetrySignedAfreshIsNot` 退 1
- M4 去掉窗口判断 → 同上分格用例退 1
- M5 停用仍当有效 → 分格用例与 `TestKeyRotationAcceptsOldAndNewKeysExactlyWhileBothAreEffective` 退 1
- M6 取不回密钥折成签名不过 → 分格用例与 `TestAMissingOldKeyDoesNotBlockTheNewKeyOfTheSameForm` 退 1
- M7 第一把对不上就不再试后面的 → `TestALaterKeyOfTheSameFormStillVerifiesWhenTheEarlierOneDoesNotMatch` 退 1
- M8 推送源册读不动折成签名不过 → 分格用例退 1
- M9 异内容答 `ALREADY_REGISTERED`（带 DSN）→ `TestPushSourceRegistrationRecordsReplaysAndNeverOverwrites` 退 1


## 判断项

1. **落点在 `internal/accessidentity`。** 校验通过要铸的是本上下文的 `IntegrationClientEnvelope`，并读集成客户端册。另起一个只答「签名成立」的平台件，每个连接器还得再铸一次信封。令牌入口与推送入口共用 `admitIntegrationClient`，授予与准入范围只写一处。
2. **绑定是登记行，不是部署参数。** 推送源随租户与外部方增减。`push_source` 外键回指集成客户端册的（发行方、`sub`、租户）；同一个推送源标识至多一行，租户取自这一行，不取自推送。标识已被别的租户占住答 `PUSH_SOURCE_BOUND_TO_ANOTHER_TENANT`，不说是哪个租户。
3. **窗口与时钟偏差是租户取值，按整秒登在推送源行上，产品不设缺省。** 窗口必须大于零；时钟偏差可以为零，缺席与零分开（批文用指针）。没登的推送源与签名不对同答 `ErrPushSignatureRejected`。受限存储没配另答 `ErrAccessChannelNotConfigured`。`PAR-INT-12` 当前登记：待提供。
4. **签名原文与两形取 Standard Webhooks。** `webhook-id.webhook-timestamp.请求体`；`v1` 为 HMAC-SHA256，`v1a` 为 Ed25519。认不出的签名标识跳过，一项都不剩才答签名不过。见越权风险点 1。
5. **密钥只存引用。** `PushSigningKeyResolver` 取回的 HMAC 材料是共享密钥字节，Ed25519 材料是 32 字节公钥，不是 `whsec_` 包装。公钥也只存引用，两形走同一条取用与轮换路径。本体的 `String` / `GoString` / `LogValue` 只交出替身（`TestPushSigningKeyMaterialNeverRenders`）。
6. **轮换是再登一把新钥、给旧钥登一笔停用。** 重叠多久由两行的时刻决定。生效与否看收到时刻，不看推送自报的签名时刻，避免用已停用的钥倒填一个停用前的时间戳。同形态两把并存时，某一把取不回先试其余的；一把都没验过且有取用失败，答 `ErrPushSigningKeyUnavailable` 而不是签名不过（`TestAMissingOldKeyDoesNotBlockTheNewKeyOfTheSameForm`）。部署没给存储则立刻交回未配置。
7. **重放键是签名原文的 SHA-256，存在 `access_identity.push_receipt`。** 列：租户、推送源、推送标识、摘要、签名时刻、`retain_until`（签名时刻加窗口）。不取推送标识做键：截获者改不动已签名的时间戳，窗口内能重放的只有逐字节相同的那一份；推送方换时间戳重签交给各命令口按载荷摘要判同身份重放（ADR-0149 决定四第一条）。过窗口后同一份原文先被越窗拦住。清扫过期行不在本票。
8. **重放账的写要落在调用方开的框架事务里**（`RecordPushReceipt` 走 `RequireExecutor`）。业务写回滚则这一行跟着消失。各上下文连接器把账和业务写放进同一事务，归 psb/09、10、12，本票没有那一处接线。
9. **推送入口不判「要求证书绑定令牌」。** 那一格属客户端凭据令牌（`MintIntegrationClient` 在核授予之前判）。推送不出示令牌。见越权风险点 2。

## 越权风险点

1. **签名原文与算法封闭集。** ADR-0149 决定三第三条只写「HMAC 或对方公钥」，没有点名 Standard Webhooks、Ed25519，也没有点名原文怎么拼。本票把这两形和 `id.timestamp.body` 收成产品的封闭集（`ParseStandardWebhook` 与 `push_signing_key` 的 `CHECK` 同笔）。owner 若要别的公钥算法或另一种原文，要改这一处，不能只加配置。
2. **推送路径不看证书绑定要求。** 册上 `certificateBoundTokenRequired` 为真的客户端，经签名推送进来时不因「没有绑定令牌」被拒。高保证在这条入口上只靠签名密钥。若 owner 认为只能推送的源也必须另有传输层客户端证书，那是本票没做的一格。
3. **重放账还没有生产调用方。** 接口要求与业务写同一事务，但本票按派单不接各上下文连接器，也不改 `cmd/parcel-api` 端点表。在那些接线落地之前，这份账不会被生产请求写到。

## Comments

**评审 ← 通道 1 · 钉 `1b1d4460` · 13:14**（基 `210eafac`，隔离检出 `/tmp/idp-review-oc12-mcp1`。作者是通道 4。用户令通道 1 直接评审，不另派。）

**Standards**

阻断：无。

非阻断：无。

无发现：密钥只存引用。`PushSigningKeyMaterial` 的 `String`、`GoString`、`LogValue` 只交替身。窗口与时钟偏差在 `NewPushSourceBinding` 不代填；批文 `WindowSeconds`、`ClockSkewSeconds` 用指针，缺席拒收。`0006_push_source_register.sql` 两列无 DEFAULT、不种行、不放密钥本体。`PAR-INT-12` 保持待提供。领域包不引 `pgx`。令牌与推送共用 `admitIntegrationClient`，证书绑定留在 `MintIntegrationClient`。

**Spec**

阻断：无。

非阻断：无。

无发现：判据三格有名。`TestHMACSignedPushIsMintedAsTheBoundIntegrationClient`、`TestEd25519SignedPushIsMintedAsTheBoundIntegrationClient`、`TestTheSamePushReplayedIsRecognisedButARetrySignedAfreshIsNot`、`TestKeyRotationAcceptsOldAndNewKeysExactlyWhileBothAreEffective`。签名不过、越窗、重放与依赖故障互不折进，见 `TestSignedPushAnswerGradesDoNotStandInForEachOther`。`MintSignedPush` 先验签、再判窗、再入账。diff 没有各上下文连接器，也没有 `cmd/parcel-api`。`//go:embed all:access_identity` 已含 0006，不改 `migrations.go` 与 `plan.go` 与模块已接线一致。

### 进 main 记录（推送方 · 通道 1 · 2026-10-14 13:14）

- **门**：上面这份评审两轴无阻断。
- **重放**：`git merge-base --is-ancestor 210eafac 1b1d4460` 成立，远端 main 仍是 `210eafac`。不 cherry-pick、不改 SHA。作者五笔原样快进。对照（分支 = main）：`f13d344b`、`f185673c`、`13f34a33`、`ab88fb41`、`1b1d4460`。
- **清点**：在这份检出上重跑 `tools/mechanism-inventory`，`MECHANISM-INVENTORY.md` 相对 `ab88fb41` 无漂移，不另立清点笔。
- **验证**：Go 与 SQL 与 `1b1d4460` 相同，本笔只改票面。`go build ./...` 与 `go vet ./...` 退 0。自 `210eafac` 起改动的 `.go`，`gofmt -l` 无输出。带 DSN 单跑 `TestPushReceiptRecognisesTheSameSignedContentOnce` 与 `TestPushSourceWritesRefuseToRunOutsideATransaction` 为 PASS、非 SKIP。带 DSN `go test -p 1 -count=1 ./...` 退 0：140 个包 `ok`、0 FAIL、14 个包无测试文件，输出里没有 `(cached)`。`0006_push_source_register.sql` 无 CR、无 BOM。推送条件：远端 main 仍是 `210eafac`，且其上只多作者五笔与本笔。已推，远端 main 曾到 `a1360371`。

**评审 ← 通道 3 · 钉 `13f34a33` · 13:08**（非作者，只读。完成记录在 `1b1d4460`。到达时 `1b1d4460` 已在 main。）

**Standards**

阻断：无。

非阻断：`ErrPushSignatureRejected` 头注写几种情形同答、不说哪一半不对。`MintSignedPush` 未登记时包装文案是 `push source is not registered`，`verifySignature` 验不过时是 `no signature verifies under a key effective at receipt`。哨兵相同；连接器若把 `Error()` 交回推送方，就能分出源在不在册。本票没有在线口。不挡合入，不改生产码。

**Spec**

阻断：无。非阻断：无。无发现：判据三条都有用例，连接器不接与票面「不做」一致。

**评审 ← 通道 1 · 钉 `4f488979` · 补笔**（基 `1b1d4460`，只看 `1ad7721a` 与 `4f488979`。生产码不动。）

**Standards** 阻断：无。非阻断：无。无发现：两格都是测试，不新增缺省，不碰密钥本体。

**Spec** 阻断：无。非阻断：无。无发现：`TestAnEd25519SignatureFromAnotherKeyIsRejected` 钉另一把私钥签的同一份原文答签名不过；`TestALaterKeyOfTheSameFormStillVerifiesWhenTheEarlierOneDoesNotMatch` 钉同形态前一把对不上时后一把仍验得过。票面只补变异自证，清点仍是 `ab88fb41`。

### 补笔进 main（推送方 · 通道 1）

- **重放**：main 已是 `a1360371`（`1b1d4460` 的子提交）。cherry-pick 到其上，SHA 换了：`1ad7721a`→`79acc46f`，`4f488979`→`7b09abb9`。无冲突。
- **门**：通道 3 与通道 1 对 `13f34a33` 都是无阻断；通道 1 对补笔无阻断。通道 3 的非阻断不挡。
- **验证**：钉 `7b09abb9`。两格新用例 PASS、非 SKIP。`gofmt -l` 对 `signed_push_test.go` 无输出。带 DSN `go test -p 1 -count=1 ./...` 退 0：140 个包 `ok`、0 FAIL、14 个包无测试文件，没有 `(cached)`。本笔只改票面。
