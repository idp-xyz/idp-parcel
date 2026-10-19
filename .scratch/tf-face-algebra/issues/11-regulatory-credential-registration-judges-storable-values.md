# 11 监管凭证登记按登记册存得下的值判：亚微秒有效期原样重放答 CONTENT_CONFLICT，零时刻毒行让整租户凭证列面 500，同一微秒有效期、越界额度与 NUL 答 500

Category: bug
Status: in-progress——2026-10-18 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-18 作者子代理完工，待评审。此前 needs-triage——2026-10-18 会话 `idp-parcel-9b` 立（操作者与集成客户端渠道逐口核代数研判；实测发现未经对抗核验，裁定待做，动手前先复现）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：逐口核研判（workflow review26-face-algebra-sweep）：/customs-regulatory-credential-registrations
地盘：
- internal/customscompliance/domain/payload_canonicalization.go 及其测试
- internal/customscompliance/domain/compliance_judgment.go（`RegisterCredential`）及其测试
- internal/customscompliance/domain/disposition_verification.go（`newRequiredValue`）及其测试
- internal/customscompliance/domain/（新增登记册精度函数所在文件）
- internal/customscompliance/adapters/postgres/（新增真库回归测试文件）
- cmd/parcel-api/（新增真渠道真库回归测试文件）
- cmd/parcel-customs-register/（新增真库回归测试文件）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象
1. 重放误判冲突。有效期在微秒以下有非零位时，原样重放答 200 `CONTENT_CONFLICT`；受控 CLI 退出码 2，提示「先核对既有登记」，每次重投都一样。
   - 触发看的是微秒以下有没有非零位，不是小数位数：`.0000000Z`、`.1234560Z`、`.123456000Z` 都答 `EXISTING`；`.1234561Z`、`.1234567Z`、`.123456789Z`、`.000000001Z` 都答 `CONTENT_CONFLICT`。
   - 后果：首投超时后重试，会被告知「另一张凭证」，调用方被诱导另起身份重登。
2. 零时刻毒行。`validFrom` 落在零时刻之后一百纳秒时：
   - 首登 201，此后原样重投答 500 `NO_ANSWER_FORMED`；
   - `CredentialView.LoadCredential` 报「rebuild credential: customs compliance: invalid regulatory credential」；
   - 该租户整张凭证列面 `/customs-credentials` 答 500。也就是说，一行毒行会拖垮整个租户的列面。
3. 以下输入都答 500 `NO_ANSWER_FORMED`，重试结果相同：
   - 有效期两端落在同一微秒内：领域放行，库 CHECK `regulatory_credential_validity_ordered` 拒；
   - `uses=2147483648`：库列是 `integer`；
   - 凭证标识、机构、持有人任一带 NUL。

## 复现（S）
在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/wt-decide2 下执行 `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -overlay <overlay> -count=1 -p 1 -run '<用例>' -v <包>`。链路：替身核验方 → `buildIntegrationClientMinter` → 生产准入桥 `buildAdmissionScope`（真库登对照与区间）→ `buildIntegrationClientIntakes` → `customshttp.NewRegisterRegulatoryCredentialEndpoint(…, buildCustomsRegistrationOrchestration(db).regulatoryCredential)`。
- 原探针：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/customs-regulatory-credential-registrations/zz_probe_http_test.go（overlay-http.json）、同目录 zz_probe_test.go（overlay.json）、cli/zz_probe_cli_test.go（挂进 `cmd/parcel-customs-register`）。
- 对抗核验：同目录 adversarial/zz_probe_adv_http_test.go 的 `TestProbeDecide2CredentialAdversarial`，子格 P1、P5、P6、P7、P8。
- 核验：同目录 verify/zz_verify_http_test.go 的 `TestVerifyDecide2CredentialClaims`，子格 V1、V2、V3。
- 本轮日志：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/customs-regulatory-credential-registrations/synth/run-adv.log、/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide2/customs-regulatory-credential-registrations/synth/run-verify.log。

## 应当
- 写得进的就读得回，原样重放就答 `EXISTING`。
- 永远存不进去的输入，答「改输入」：`NOT_ACCEPTED`（200），不答 500。依据是 ADR-0031「`error` 留给『这次写入没能完成』」，与票 30 对严格先后门的定性相同。

## 根因
- `CanonicalizeCredentialRegistrationPayload` 的 valid_from、valid_to 走 `canonicalInstant`（`time.RFC3339Nano`）；`domain.RegisterCredential` 只做 `.UTC()`，不截精度。库面 `timestamptz` 只存到微秒，所以在册一侧读回的值与命令一侧不同串。这个误判早于 CCC-1：`b4d28563` 之前的 `sameCredential` 就按纳秒 `Equal` 比。
- `RegisterCredential` 用 `validFrom.IsZero()` 判零，又用「`!validTo.After(validFrom)`」判先后，都按纳秒算。没有额度上限门。
- `newRequiredValue` 不拒 NUL。
- 以上任一库面错误都会让事务进入中止态。编排虽把它折成`未决`，提交仍被回滚，最终答 500。

## 修复方向（机制）
1. 在 CC 领域定一处登记册精度函数，取名同 TF 的 `AtRegistryPrecision`，实现为 `at.UTC().Truncate(time.Microsecond)`。头注写成 CC 的决定，并按符号名引 TF 的同名函数与 accessidentity 的 `sameInstant` 作同族先例。不做 platform 共用，理由同票 30 裁定：精度随持久化形态而变。
2. 相等类：`CanonicalizeCredentialRegistrationPayload` 的期限两端先过这个函数再拼串。这一形状的摘要从不落库，两侧都是现算，就地改不涉及 ADR-0014 的已存摘要兼容。`canonicalInstant` 本身不动：CC 其余口存的是落库前算好的摘要。
3. 严格先后与判零：`RegisterCredential` 的「终点晚于起点」与「非零」两侧都先折到登记册精度再判；额度加上不超过库列上限的门。不成立时答 `ErrInvalidCredential`，编排已把它映射为 `NOT_ACCEPTED`。
4. `newRequiredValue` 拒含 NUL 的值。它为 CC 各口共用，完成记录要列出受影响的口。
5. 存量毒行不在产品内自动修，完成记录写出查存量的语句：`valid_from` 或 `valid_to` 为零点的行。

## 是否潜伏
已可触发，两条路径：
- 集成客户端渠道：只差部署与租户取值（发行方三参数、客户端登记、`REGULATORY_CREDENTIAL` 授予、自我权威串）。演示种子已带对照 `SYN-MAP-EF-REGULATORY-CREDENTIAL-01` 与对应治理区间。
- 受控 CLI `parcel-customs-register regulatory-credential`：走同一只 `RegisterCredentialHandler`，不经渠道闸，第 1 条今天就能触发。

## 完成判据
- [ ] 领域单测：
  - 精度函数：去掉纳秒尾巴、跨一微秒判不同、非 UTC 输入与其 UTC 同值。
  - `RegisterCredential`：同一微秒区间、零点后不足一微秒、超出 `integer` 的额度，各答 `ErrInvalidCredential`；整秒、`.123456Z` 照常。
  - CCC-1 形状对「纳秒版 / 微秒版」判等，且两个方向都断言。
- [ ] 真库回归：
  - `adapters/postgres` 新测试文件，编排加真登记册：`.1234567Z`、`.123456789Z`、`.000000001Z` 首登后原样重放答 `EXISTING`；+1µs 答 `CONTENT_CONFLICT`。
  - 同一租户先登一张正常凭证、再投零点毒行输入：后者答 `NOT_ACCEPTED`，列面照常。
- [ ] HTTP 真渠道回归（`cmd/parcel-api` 新测试文件，链路同复现）：
  - 上列重放格答 200 `EXISTING`；
  - 同一微秒区间、越界额度、NUL 引用各答 200 `NOT_ACCEPTED`，不是 500；
  - 库零行。
- [ ] CLI 真库回归（`cmd/parcel-customs-register` 新测试文件）：`.123456789Z` 第二遍退出码 0，打印 EXISTING。
- [ ] 变异自检：把任一处改回按纳秒比，或撤掉额度门、NUL 门，对应用例转红；完成记录写明。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退出码 0，`gofmt -l .` 无输出；带 DSN 跑 `go test -p 1 -count=1 ./internal/customscompliance/... ./cmd/parcel-api/... ./cmd/parcel-customs-register/... ./internal/architecture/...` 全 ok，`-v` 下 CC `adapters/postgres` 无 `--- SKIP`。
- [ ] 新增生产文件后，在干净检出上重生成 `docs/product/MECHANISM-INVENTORY.md`，随笔提交。
- [ ] 票 15 补记下追加回写行（只加行）。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：作者自验；非作者评审，两轴分开；推送方在干净检出上带 DSN 跑全量。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-tfa11）

基线 `7272f6cd`，代码 tip `e2e62db2`。照「修复方向（机制）」五条落，未另裁。

### 落点
1. `internal/customscompliance/domain/registry_precision.go` 新增 `AtRegistryPrecision`（`at.UTC().Truncate(time.Microsecond)`），头注写成 CC 自己的决定，按符号名引 `transportfulfillment/domain` 的 `AtRegistryPrecision` 与 `accessidentity/adapters/postgres` 的 `sameInstant` 作同族先例；不进 platform。
2. `CanonicalizeCredentialRegistrationPayload` 的期限两端先过它再拼串；`canonicalInstant` 未动。凭证登记那两格 CCC-1 金样随之改写（摘要从不落库，两侧现算）。
3. `RegisterCredential` 的「非零」与「终点晚于起点」两侧先折到登记册精度再判；额度加 `uses > maxRegistryCredentialUses`（`math.MaxInt32`，额度列 `integer`）门；均答 `ErrInvalidCredential`，编排已映射 `NOT_ACCEPTED`。存值仍是原值。
4. `newRequiredValue` 拒含 NUL，新哨兵 `ErrValueContainsNUL`。受影响的口：凡经下列构造的 CC 各口——`NewTenantID`、`NewRegulatoryDecisionID`、`NewRegulatoryAuthorityReference`、`NewLegalActionReference`、`NewDecisionScopeReference`、`NewExecutorReference`、`NewExecutionFactReference`、`NewCredentialID`、`NewCredentialHolderReference`、`NewComplianceTopicReference`、`NewComplianceRuleVersionReference`、`NewCredentialGateBasisReference`、`NewResponsibleRoleReference`、`NewOperationsScopeReference`、`NewRouteCandidateReference`、`NewLegalObligorReference`、`NewPaymentRequirementSource`、`NewResponsibilityTargetReference`、`NewFundsFactVersion`、`ProvidedFundsPayer`、`NewDeclarationUnitID`、`NewDeclaredParcelReference`、`NewCustomsProcedureReference`、`NewReadinessBasisReference`、`NewSubmissionVersionID`、`NewDossierSnapshotReference`、`NewRoleSnapshotReference`、`NewSubmissionAuthorityReference`、`NewSafeResendReference`、`NewCustomsCaseID`、`NewRegulatoryJurisdictionReference`、`NewObligationScopeReference`、`NewSourceAuthorityRole`、`NewInterpretationRuleReference`、`NewFollowUpTriggerReference`、`NewAssessedDutyReference`、`NewExternalFundsFactReference`、`NewPreconditionReference`、`NewExternalManifestID`、`NewManifestSourceVersion`、`NewCarrierResponsibilityReference`、`NewCustomsPortReference`、`NewDeclarationPathReference`、`NewDeclarationModeReference`、`NewRestrictionID`、`NewRegulatoryReleaseReference`。含 NUL 的值在各口答该口对空白值的那一格（译装期构造的口即 400 `MALFORMED_REQUEST`），不再进库答 500。
5. 存量毒行不在产品内修。查存量（已在真库上对一行零点毒行、一行正常行验过，只命中前者）：
   ```sql
   SELECT tenant_id, credential_id, valid_from, valid_to
   FROM customs_compliance.regulatory_credential
   WHERE valid_from = timestamptz '0001-01-01 00:00:00+00'
      OR valid_to   = timestamptz '0001-01-01 00:00:00+00'
   ORDER BY tenant_id, credential_id;
   ```

### 各格对应用例
- 精度函数：`domain/registry_precision_test.go` · `TestTheRegistryPrecisionKeepsMicrosecondsAndDropsTheTail`（去尾、跨一微秒判不同、非 UTC 与其 UTC 同值）。
- 构造门：`domain/compliance_judgment_test.go` · `TestCredentialRegistrationJudgesTheValuesTheRegistryCanStore`（同一微秒区间、零点后不足一微秒、`MaxInt32+1` 各答 `ErrInvalidCredential`；整秒、`.123456Z`、跨一微秒、零点后恰一微秒、额度恰在上限、未提供照常）。
- CCC-1 判等：`domain/payload_canonicalization_test.go` · `TestCredentialRegistrationShapeJudgesValidityAtRegistryPrecision`（纳秒版对微秒版、微秒版对换偏移的纳秒版判同；起点、终点各跨一微秒判异）。
- NUL：`domain/disposition_verification_test.go` · `TestARequiredValueRefusesNUL`。
- 真库：`adapters/postgres/credential_registry_precision_test.go`——`.1234567Z`、`.123456789Z`、`.000000001Z` 重放 `EXISTING`、+1µs `CONTENT_CONFLICT`；先登正常凭证再投零点毒行答 `NOT_ACCEPTED` 且列面照常；同一微秒区间 `NOT_ACCEPTED` 册上零行。
- HTTP 真渠道：`cmd/parcel-api/assemble_integration_client_credential_precision_test.go`（链路复用票 operator-channel/52 的 `integrationClientChannelOnRealDB`）——重放三格 200 `EXISTING`、+1µs 200 `CONTENT_CONFLICT`；同一微秒区间、零点毒行、`uses=2147483648` 两遍都 200 `NOT_ACCEPTED`；NUL 于凭证身份、机构、持有人；册上零行。
- CLI 真库：`cmd/parcel-customs-register/credential_precision_test.go`——`.123456789Z` 第二遍退出码 0、打印 EXISTING。

### 先红后绿
- 红（修复前代码，领域格用恒等桩 overlay 补出未定义符号）：领域四组全红；真库重放三格 `CONTENT_CONFLICT`，零点毒行 `REGISTERED` 后列面报「rebuild credential catalogue: customs compliance: invalid regulatory credential」，同一微秒区间「commit unexpectedly resulted in rollback」；HTTP 重放 200 `CONTENT_CONFLICT`，同一微秒、额度、三处 NUL 都 500 `NO_ANSWER_FORMED`，零点毒行首投 201、重投 500；CLI 第二遍退出码 2 `CONTENT_CONFLICT`。现象 1–3 全部复现。
- 绿：上列用例全过。
- 变异自检（overlay 换入单处改动）：拼串改回纳秒→金样、判等、真库重放、HTTP 重放、CLI 转红；构造门改回纳秒→构造门两格、真库毒行与同一微秒、HTTP 两格转红；撤额度门→构造门额度格、HTTP 额度格转红；撤 NUL 门→NUL 五格、HTTP 三格转红。

### 与完成判据的出入
- NUL 一格答的是 400 `MALFORMED_REQUEST`，不是判据写的 200 `NOT_ACCEPTED`：修复方向把门放在 `newRequiredValue`，引用在 Intake 译装时构造，拒绝走的是空白引用那条路。用例钉的是「与空白引用同一格、不是 500、册上零行」。同属「改输入」，按 ADR-0031 不是 500。
- 「票 15 补记下追加回写行」不在本票地盘，未做，留会话。

### 门
`gofmt -l`（改动的 .go）无输出；`go build ./...`、`go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./internal/customscompliance/... ./cmd/parcel-customs-register/... ./cmd/parcel-api/... ./internal/architecture/...` 12 包 ok、0 FAIL，`-v` 下 CC `adapters/postgres` 无 `--- SKIP`。机制清点在本分支重生成，有差（customscompliance 生产 +1、测试 +2，`cmd/` 测试 +2），单独提交 `e2e62db2`。未跑全仓全量。

证据层级：`S`（隔离合成，SYN- 值）。

评审：待会话读 diff 审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `6d98256c`（基线 `7272f6cd`）· 2026-10-18**：用户选「精简续做」，评审由会话本身读 diff 做；代码出自本会话起的实施子代理，不算非作者评审，如实记下。
  - **Spec**：阻断无。CC 领域 `AtRegistryPrecision` 是规则的唯一陈述，头注按符号名引 TF 同名函数与 accessidentity `sameInstant`；CCC-1 期限两端先折、`canonicalInstant` 不动；构造门的判零与先后两侧先折；额度不超过库列 integer 上限；`newRequiredValue` 拒 NUL。现象三格都先红复现、修后转绿，变异自检四处都转红。
  - **Standards**：阻断无。认可一处与判据的出入：NUL 答 400 `MALFORMED_REQUEST` 而不是 200 `NOT_ACCEPTED`——门在共用的必备值构造里，译装时即拒，与空白引用同一格，不是 500、册上零行，合「存不进去的输入在构造期答改输入」。
  - **无发现**：合成值只用 `SYN-`；查存量毒行的 SQL 写在完成记录、真库验过。
