# 52 集成客户端 Intake 拒自报租户只认精确键：载荷带 `tenantid` 一类大小写变体即改写认证租户——跨租户登凭证与资金事实、探询他租户在册内容、给他租户植入毒行

Category: bug
Status: in-progress——2026-10-18 作者子代理完工，待评审。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（操作者与集成客户端渠道逐口核代数时发现；跨租户写入，已可触发，优先；用户授权代决）；分支 `claude-oc52`
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)（票 26 评审遗留研判的逐口核）
出处：逐口核代数研判（workflow review26-face-algebra-sweep）监管凭证登记口一行，探针与日志路径见「复现」。
地盘：
- internal/customscompliance/adapters/http/integration_client_intake.go 及其测试
- internal/customscompliance/adapters/registrationjson/translate.go 及其测试
- internal/settlementaccounting/adapters/http/integration_client_intake.go 及其测试
- internal/settlementaccounting/adapters/registrationjson/translate.go 及其测试
- cmd/parcel-api/（新增真渠道真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；逐口实测由隔离子代理做，结论未另经核验代理复核（因 token 花费精简），实施前须先复现。

## 现象
- 集成客户端绑在 SYN-TENANT-01，也按 SYN-TENANT-01 过了准入。载荷自报 `"tenantid":"SYN-TENANT-OTHER"` 时，监管凭证照登，答 201 `REGISTERED`，却落在 SYN-TENANT-OTHER，SYN-TENANT-OTHER 的列面读得到这一行；`tenantiD` 也一样。
- 其余变体：
  - `TenantId`、`tenantID`、`TENANTID` 不被拒，但落在认证租户；
  - Unicode 混淆写法（`tenantİd` 等）答 400；
  - 精确键 `tenantId` 答 400。
- 能做的事：
  - 跨租户探询：对 SYN-TENANT-OTHER 在册凭证猜中全部内容答 `EXISTING`，猜错额度答 `CONTENT_CONFLICT`，可据此判出他租户在册凭证的存在与内容。
  - 植入毒行：经变体键把零时刻毒行登进 SYN-TENANT-OTHER 后，那个租户的凭证列面答 500。
  - 抢注：被植入的租户日后自己登同一身份，答 `CONTENT_CONFLICT`。
  - 判断：被植入的凭证会被他租户的 `JudgeCredentialApplicabilityHandler.Handle` 读到（读码）。
- SA：资金事实采用、更正两口的 Intake 层实测，`tenantid` 与 `tenantiD` 都把命令租户改写成 SYN-TENANT-OTHER；`tenantId` 答 400；其余变体落在认证租户。落库没有实测，但代码路径与 CC 同形。
- 对照：
  - CC 外部结果口不受影响，因为 `externalResultDocument` 没有租户格，变体都是未知字段。
  - CC 与 VE 的操作者在线登记口用 `refuseSelfReportedTenant`，也只认精确键，变体不被拒；但走 `injectedTenant`，租户仍是认证那一个，不越租户（核验实测）。

## 复现（S）
- CC，真库：在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行
  `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v ./cmd/parcel-api/`
  - /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/customs-regulatory-credential-registrations/zz_probe_http_test.go（overlay-http.json）
  - 同目录 adversarial/zz_probe_adv_http_test.go 的 P2、P3、P4（overlay-adv-http.json）
  - 同目录 verify/zz_verify_http_test.go 的 V4、V5（overlay-verify.json，与 verify/rj、verify/sa 两份一起挂）
  - 本轮日志：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/customs-regulatory-credential-registrations/synth/run-adv.log、/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/customs-regulatory-credential-registrations/synth/run-verify.log
- SA，Intake 层：
  `go test -overlay /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/settlement-external-funds-fact-registrations/overlay-tenant-case.json -count=1 -run TestProbeDecide2SettlementTenantKeyCaseVariants -v ./internal/settlementaccounting/adapters/http/`
  - 探针：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/settlement-external-funds-fact-registrations/zz_probe_tenant_case_test.go
  - 日志：同目录 run-tenant-case.log

## 应当
- 租户只取认证结果（ADR-0149 决定三；`IntegrationClientIntake` 头注写的是「租户只取认证结果……载荷里自报租户即拒」）。
- 载荷里任何能被解码成租户格的键，都按自报租户答 400。

## 根因
- 两处同名同形的 `snapshotWithAuthenticatedTenant`（CC、SA 各一份）只用 `snapshot["tenantId"]` 精确匹配来拒自报，然后把认证租户拼回 `tenantId`，把 map 重新 `json.Marshal`。
- map 按键排序，`tenantid`、`tenantiD` 排在 `tenantId` 之后。
- 译装 `RegulatoryCredentialFromJSON`、`ExternalFundsFactFromJSON`、`ExternalFundsFactCorrectionFromJSON` 用 `decodeStrict` 解进带 `json:"tenantId"` 的结构体。`encoding/json` 匹配字段名不分大小写，后出现的值胜出，于是变体键的值盖掉了认证租户。

## 修复方向（机制）
1. 不再让认证租户经 JSON 快照往返。
2. 给在线口加带租户的译装：CC 加 `RegulatoryCredentialFromJSONForTenant(raw, tenant)`；SA 两口同样各加一个。它们走 registrationjson 已有的 `injectedTenant` 路，解码用一份不含租户格的文档类型；这样任何大小写的租户键都是未知字段，`DisallowUnknownFields` 直接拒成 400。
3. 拒自报保留一条专门的提示，判定改成与 `encoding/json` 同一折叠规则：`strings.EqualFold(key, "tenantId")`。
4. 删去两份 `snapshotWithAuthenticatedTenant`。

## 不做
- CC 与 VE 操作者在线登记口的 `refuseSelfReportedTenant` 只认精确键。它不越租户，但违反「自报即拒」，属 `operatorRegistries` 那一族，抄送 CC、VE owner，另行处理。

## 是否潜伏
已可触发。生产装配上，`integrationClients.customs`、`integrationClients.settlement` 恒为真 Intake。剩下的只是部署与租户取值：发行方三参数，以及客户端登记与授予（`parcel-access-register integration-client-register` / `integration-client-grant` 可登）。任何持有授予的集成客户端都能写入、探询别的租户。

## 完成判据
- [ ] 译装单测（CC、SA 各自 registrationjson）：在线译装对 `tenantId`、`TenantId`、`tenantID`、`tenantid`、`tenantiD`、`TENANTID` 各答错；命令租户恒等于注入的那一个。
- [ ] Intake 单测（CC、SA 各自 http）：上列各变体都答 400 `MALFORMED_REQUEST`；不带租户键时，命令租户等于认证租户。
- [ ] 真渠道真库回归（`cmd/parcel-api` 新测试文件，链路同复现；SA 用 `integrationClients.settlement` + 生产编排）：
  - 上列各变体答 400，两个租户的册上都是零行；
  - 跨租户探询那一格（猜中、猜错）答 400，不再是 `EXISTING` / `CONTENT_CONFLICT`。
- [ ] 变异自检：在线口改回快照往返，上列用例转红。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/customscompliance/... ./internal/settlementaccounting/... ./cmd/parcel-api/... ./internal/architecture/...` 全 ok。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验；非作者评审，两轴分开，Spec 轴含租户隔离；推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-oc52）

基线 `c783151b`。四笔：`1e946778` 红（Intake 单测与真库回归）→ `2b8a5883` CC、SA 修复与译装单测 → `6ee6d48b` VE 操作者在线口拒自报改同折叠 → `782d8add` 机制清点重生成。

落点：
- 两份 `snapshotWithAuthenticatedTenant` 删去；CC、SA 的 `IntegrationClientIntake` 改用 `readSinglePayload` 收原文（单个 JSON 对象、无尾随），交给在线译装。
- CC `registrationjson` 加 `RegulatoryCredentialFromJSONForTenant`（走 `injectedTenant`）；SA `registrationjson` 加 `ExternalFundsFactFromJSONForTenant`、`ExternalFundsFactCorrectionFromJSONForTenant`。在线口解不含租户格的内容类型（`regulatoryCredentialContent`、`externalFundsFactContent`、`externalFundsFactCorrectionContent`），受控批量口的文档类型嵌同一份内容类型，形状仍一份口径。
- 拒自报保留专门提示 `ErrSelfReportedTenant`（SA 新立），判定改 `strings.EqualFold(key, "tenantId")`。CC 操作者在线登记各口共用这一道，随之收紧。
- 「不做」一节的 CC、VE 操作者在线口：会话指派「代价小则一并改」。CC 随上一条收紧；VE 的 `refuseSelfReportedTenant` 同样改了（越出地盘，单独一笔 `6ee6d48b`）。红时实测：VE 例外披露规则口带 `TenantId` 键整份照登，租户仍是注入那一个。

完成判据逐格：
- ✅ 译装单测：CC `TestOnlineRegulatoryCredentialTranslationRefusesEveryLetterCaseOfTheTenantKey`、`TestOnlineRegulatoryCredentialTranslationTakesTheInjectedTenant`，另有 `TestOnlineTranslationRefusesEveryLetterCaseOfTheTenantKeyOnOperatorFaces`；SA `online_tenant_test.go` 的 `TestOnlineFundsTranslationRefusesEveryLetterCaseOfTheTenantKey`、`TestOnlineFundsTranslationTakesTheInjectedTenant`。六个变体都答 `ErrSelfReportedTenant`；`tenantİd` 按未知字段拒。
- ✅ Intake 单测：CC `TestIntegrationClientIntakeRefusesEveryLetterCaseOfASelfReportedTenantOnCredentials`，SA `TestIntegrationClientIntakeRefusesEveryLetterCaseOfASelfReportedTenantOnFundsFacts`（采用、更正两口），六个变体都答 `ErrMalformedRequest`。不带租户键时取认证租户，由原有的 `…UnderTheAuthenticatedTenant` 证。
- ✅ 真渠道真库回归：`cmd/parcel-api/assemble_integration_client_tenant_key_test.go`。链路是 `buildIntegrationClientMinter` → `buildAdmissionScope`（真库对照与区间）→ `buildIntegrationClientIntakes` → 生产编排（`buildCustomsRegistrationOrchestration`、`buildSettlementRegistrationOrchestration` 配 `intakes.settlement`）。断言：
  - 六个变体都答 400 `MALFORMED_REQUEST`，两个租户的册上都是零行；
  - 他租户在册凭证，猜中与猜错额度都答 400，册上仍只有他租户那一行；
  - SA 更正各变体答 400，对照事实仍是一版；
  - 不带租户键的对照登记答 201，落在认证租户。
- ✅ 变异自检：
  - Intake 改回快照往返（取 `1e946778` 的两份 Intake）：CC、SA 的 Intake 单测与两条真库回归转红。
  - 拒自报改回精确键：只有三条译装单测转红，Intake 与真库仍绿——不含租户格的严格解码是真正的门，前置判定只管提示。
- ✅ Go 门禁：`gofmt -l` 无输出，`go build ./...`、`go vet ./...` 退 0。带 DSN 跑 `go test -p 1 -count=1`，包含 `./internal/customscompliance/... ./internal/settlementaccounting/... ./internal/visibilityexception/adapters/... ./cmd/parcel-api/... ./cmd/parcel-customs-register/... ./cmd/parcel-settlement-register/... ./cmd/parcel-ve-register/... ./internal/architecture/...`，结果 38 个包 ok、0 FAIL。未跑全仓全量。
- ⬜ 票 15 补记回写行：未加。票 15 补记记的是操作者决定面的重放代数，集成客户端口不在其中；票 34、36 的同类行都附进 main 的 SHA，本票还没进 main。这一格是否适用、何时写，留给会话或合入方定。
- ⬜ Go 那条路：作者自验已做。非作者评审与推送方全量未做。

先红后绿证据（S）：
- 红：`1e946778` 上真库回归，`tenantid`、`tenantiD` 把凭证登进 SYN-TENANT-OTHER，SA 资金事实也一样（SA 落库首次实测）；`TenantId`、`tenantID`、`TENANTID` 答 201，落在认证租户；探询答 200。日志 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc52-red-realdb.log
- 绿：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc52-green-realdb.log
- 变异：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc52-mut/mutation-snapshot.log、/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc52-mut/mutation-exactkey.log
- 门：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/oc52-gate.log

证据层级：S（隔离合成，SYN- 值，演示租户与 SYN-TENANT-OTHER）。

顺带看到，未动：`internal/networkrouting/adapters/registrationjson` 的拒自报只认 `tenant_id` 精确键，`internal/parcelpricing/domain/registered_price_card.go` 只认 `tenant` 精确键。是否走注入、会不会越租户，都没核。

评审：待会话读 diff 审。
