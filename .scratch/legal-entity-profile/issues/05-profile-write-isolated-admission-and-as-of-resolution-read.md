# 05 服务端：资料登记写口进隔离写准入、法人资料按时点解析读口

Category: enhancement
Status: resolved · 已进 main——2026-09-24 评审 ← 通道 1（非作者）可接受、两轴无阻断；通道 1 备好重放后崩于推前，推送方由本票作者通道 3 按用户令代行，在 main `ef18e3db` 之上重放：代码笔 `82fc0145` / `755253e0`、完成记录 `f4b549aa`，清点 `d228415b`；分支 `mcp3-lep05` 作封存出处，新旧 SHA 对照见 Comments「进 main 记录」。票 04 被本票挡住的登记表单与当前有效区随之解除阻塞。此前 resolved——2026-09-24 通道 3 在分支 `mcp3-lep05` 上做完（基 origin/main `4c3f7b89`；代码 tip `6be912f3`，含清点 `f23b2e9d`），分支已推 origin。进 main 由推送方安排非作者评审后重放，只取本票的笔：`31694f7f`、`6be912f3`、`f23b2e9d` 与本笔票面；`0f11bc00` 是共享树 main 上认领笔 `256665ed` 的拣入，重放时为空。完成记录见文末。此前 in-progress——通道 3 认领（派单 `task-0d7de977`）；此前 ready-for-agent——通道 3 立（通道 1 就派单 `task-a5cd823d` 裁定「拆」：04 的「法人资料」区缺的两处服务端单立本票）
Blocked by: 无
地盘：party-commercial http 适配器（隔离写 Intake 加资料登记一口、新增按时点解析读口端点）；`cmd/parcel-api` 端点表两行（资料登记写行换 Intake 变量、解析读口新行）、装配入参与路由 / 隔离准入测试表。不碰领域与应用判断。
出处：[ADR-0091](../../../docs/adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)（写路径逐口放行、可分辨物由 `SYN-` 前缀承担）；[ADR-0077](../../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)（目录查阅通例）；[ADR-0145](../../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定五、六；[票 03](./03-legal-entity-profile-revisions-and-as-of-resolution.md) 完成记录「未做 / 风险」里「按时点解析只到应用用例与端口」与「写口未进隔离写准入」；[票 04](./04-admin-web-identity-fields-and-profile-face.md)「法人详情加『法人资料』区」。

## 做什么

1. **资料登记写口按 ADR-0091 逐口放进隔离写准入**：`IsolatedPartyIdentityIntake` 加 `IntakeLegalEntityProfileRegistration`，载荷形状与受控 CLI `register-legal-entity-profiles` 的一项同形、不收 `tenantId`（租户取自准入作用域），可分辨性照身份族五口的先例；`cmd/parcel-api/endpoints.go` 资料登记那一行把字面量 `UnconfiguredIntake{}` 换成隔离写 Intake 变量。
2. **法人资料按时点解析读口**：如 `GET /commercial-group-legal-entities/{legalEntityId}/profile-resolution?at=…`（路径以既有商业目录读口的命名为准），按 ADR-0077 挂商业目录 Intake、进隔离读准入。答复把 `ResolveLegalEntityProfileHandler` 的各格如实译出：已解析（修订引用与内容）、资料不全两成因（缺开票资料时带出有效的那一笔）、法人未登记 / 未生效 / 已停用；不在读口里另算。`at` 由调用方给，缺或形状不对答 400，不拿服务端时钟代填——同一问两次要得同一答。

## 不做

- 不动领域判断与应用用例（`ResolveLegalEntityProfile`、资料门）。
- 不开生产操作者渠道（归 operator-channel 票族）。
- 不改管理台（归票 04）。

## 完成判据

- 隔离写三态：未启用答 403、启用对合规载荷答 201 `REGISTERED`、带 `tenantId` 答 400；真库一条经隔离写 Intake 与生产装配登下一笔资料修订。
- 解析读口端点用例：各格逐一转写，读失败答没形成答案；隔离读放行表加一行，启用态经真路由答 500（未接线读口）而不是 400。
- 端点表与路由探针表同步；作者自验 `cmd/*` 带 DSN。

## 完成记录（2026-09-24，通道 3，分支 `mcp3-lep05`）

**落点**

| 笔 | 段 | 做了什么 |
|---|---|---|
| `31694f7f` | http | `IsolatedPartyIdentityIntake.IntakeLegalEntityProfileRegistration`（外壳镜像受控 CLI 文档、去掉 `tenantId`，一份载荷恰一项，给了的格过领域构造门）；`NewQueryLegalEntityProfileResolutionEndpoint`（挂商业目录 Intake，`at` 必带，解析各格原样译出） |
| `6be912f3` | 装配 | 资料登记写行改走 `legalEntityProfileRegistrationIntake`、随 `isolatedPartyIdentity` 换值，启动日志放行名单加一行；解析读口新行，`main` 以资料登记册与身份登记册构造解析用例；未接线占位；路由探针、隔离读放行、隔离写放行三张表各加一行 |
| `f23b2e9d` | 清点 | 机制清点在本分支重生成 |

**完成判据**

- ✅ 隔离写三态：未启用答 403（`TestEveryAssembledEndpointAnswersUnconfigured`）；启用后本口真去读请求（`TestIsolatedWriteAdmissionSwitchesOnlyTheAdmittedCommandLines` 二分为 400，日志名单与二分表一致由 `TestIsolatedWriteAdmissionNamesTheAdmittedCommandLines` 守）；真库 `TestIsolatedLegalEntityProfileRegistrationLandsAndResolvesAgainstARealDatabase` 经隔离写 Intake 与生产装配登下资料：合规载荷 201 `REGISTERED`、带 `tenantId` 400，再登一笔未来生效的修订也 201。
- ✅ 解析读口：`TestLegalEntityProfileResolutionEndpointTranscribesEachOutcome` 六格逐一转写（已解析、资料不全两成因、法人未登记 / 未生效 / 已停用）；`TestLegalEntityProfileResolutionEndpointGuards` 守缺 / 错 `at`（不调解析）、未配置、非 GET、空路径、解析失败答没形成答案；隔离读放行表加一行，启用态经真路由答 500。上面那条真库用例经隔离读 Intake 解析：今天答第一笔、2030 年答第二笔。
- ✅ Intake：`TestIsolatedIntakeTranslatesALegalEntityProfileRegistration`、`TestIsolatedIntakeRefusesMalformedLegalEntityProfiles`。
- ✅ 端点表与路由探针表同步；作者自验见下。

**门**（钉 `6be912f3`，WSL，go1.26.8，DSN 为门禁库 55432）：`go build ./...`、`go vet ./...` 退 0；改动的 `.go` `gofmt -l` 无输出；先单跑真库用例是 `PASS` 不是 `SKIP`。改动包（PC http 适配器、`cmd/parcel-api`——后者也是前者唯一的反向依赖）与 `internal/architecture` 共 3 包 `-p 1 -count=1 -v`：全 ok，`--- PASS` 465、`--- FAIL` 0、`--- SKIP` 0。全量由推送方跑。

自基线 `4c3f7b89` 以来动过的 `.go`：`internal/partycommercial/adapters/http/` 下 `isolated_write_intake.go`、`query_legal_entity_profile_resolution.go`（新）、`isolated_profile_intake_test.go`（新）、`legal_entity_profile_resolution_endpoint_test.go`（新）；`cmd/parcel-api/` 下 `endpoints.go`、`main.go`、`assemble_isolated_write.go`、`unwired_orchestration.go`、`endpoints_test.go`、`isolated_read_test.go`、`isolated_write_test.go`、`assemble_commercial_registration_test.go`、`isolated_write_profile_test.go`（新）。无 `.sql`。

**判断项**

1. **载荷与受控 CLI 的一项同形，子格沿修订历史读口的答复体**：一份资料在读写两个方向上同一个样子，管理台表单与资料页用同一套键。
2. **缺注册地址不在 Intake 拦，交用例答`未受理`**：判据同身份层三格——Intake 只管「给了就得立得住」，缺不缺由用例判。受控 CLI 在触库前拒这一格，两个入口各随各自的惯例，都不代填。
3. **资料的翻译在 CLI 与隔离 Intake 各写一份**：照身份族「各边界各自译」的先例，不抽共用函数；两份键名与构造门一致，错误各包成自己入口的形状。
4. **解析读口 `at` 必带，不拿服务端时钟代填**：同一问两次要得同一答；页面传它自己的当前时刻。
5. **答复带有效的那一笔**：已解析时就是可以固定的那一笔；缺开票资料时也带，读的人要知道该给哪一笔之后补登。能不能开立只看 outcome。
6. **解析读口的第二参是应用读用例，不是读口**：哪一笔有效只能由领域对时点判出，装配点不预挑；`main` 另构造一只身份登记册适配器供它读法人，与写编排里那只同类型、不带状态。
7. **资料登记口与身份族同一个隔离 Intake 类型**：ADR-0091 逐口放行——这个类型实现了哪几口，装配点就只换得了哪几行，多换一行编译期就红。
8. **真库用例的铺底抽成 `registerLegalEntityWithIdentityLayer`**：票 03 的装配用例改用它，免得同样的铺底抄两份。

**未做 / 风险**

- 管理台接这两口归票 04（其「法人资料」区的登记表单与当前有效区 Blocked by 本票）。
- 生产操作者渠道不在本票（归 operator-channel 票族）；本口今天只在隔离写准入下开。
- `cmd/parcel-api/endpoints.go` 是共享接线文件，通道 2 的 routing-first-cut/03 与 operator-channel/08 也可能改它：各改各的行，后进 main 的一方解冲突。

## Comments

### 评审 ← 通道 1（推送方自跑；非作者，隔离子代理认证失败未起）· 钉 `6be912f3`（基 `4c3f7b89`，只读，门禁未重跑） · 2026-09-24 18:1x（改派背景：通道 5 17:52 后无动静、通道 2 / 4 在做实施票、隔离子代理两次认证失败；按 parallel-sessions「没有空闲通道时推送方自己跑」，如实记）

（原文照录自通道 1 崩前在隔离检出里备好、未推的进 main 记录笔 `400ed651`，一字未改。）

**Standards** — 阻断：无。非阻断：无。注释中文、跨文件引用用符号名；`cmd/parcel-api/endpoints.go` 身份族那段注释把「五口」改成不计数的「各口」，合 AGENTS.md「改文档」的计数规矩。

**Spec** — 阻断：无。非阻断：无。

逐点：
① `IsolatedPartyIdentityIntake.IntakeLegalEntityProfileRegistration`：自报 `tenantId` 经 `refuseSelfReportedTenant` 拒，租户取准入作用域；一份载荷恰一项；给了的格逐个过领域构造门、形状错包 `ErrMalformedRequest`；缺注册地址交用例答`未受理`——合票面做什么 1「不收 `tenantId`」「可分辨性照身份族五口的先例」。
② `NewQueryLegalEntityProfileResolutionEndpoint`：只收 GET；先过 `CommercialCatalogueIntake` 再读路径；`at` 按 RFC 3339 必带，缺或错答 400、不拿服务端时钟代填——合做什么 2「同一问两次要得同一答」；解析失败答没形成答案，领域答出无名格另有兜底；已解析、资料不全（带成因与有效的那一笔）、法人未登记 / 未生效 / 已停用原样交出，不另算。
③ 装配：资料登记行的 Intake 变量缺省为 `UnconfiguredIntake{}`，只在隔离身份族开关启用时换值，生产形态不变；解析读口挂商业目录 Intake，随隔离读准入一格换值；交入的是应用读用例而不是读口，注释写明理由。
④ 完成判据三条在票面完成记录里逐条对到用例名。

结论：可接受。

### 进 main 记录（推送方 · 通道 3 代行；通道 3 是本票作者）

- **门**：评审 ← 通道 1（非作者）可接受、两轴无阻断，合 parallel-sessions「推送方只在评审为无阻断时重放」。通道 1 崩前已在 main `256665ed` 之上试重放、备好本记录的前身，未跑全量、未推；其后 main 前进到 `ef18e3db`（routing-first-cut/03）。通道 1、2 已崩，用户令通道 3 代行推送方——推送方由作者代行这一格如实记，评审仍是非作者那份。
- **重放**：在 main `ef18e3db` 之上 cherry-pick 为 `82fc0145`（← `31694f7f`）/ `755253e0`（← `6be912f3`）/ `f4b549aa`（← `d15b3b7b`），无冲突；认领拣入 `0f11bc00` 与 main 上 `256665ed` 等价，跳过；分支清点笔 `f23b2e9d` 不重放，批 tip 干净检出重生成为 `d228415b`。中间进 main 的 routing-first-cut/03 与本票没有重叠的 `.go` / `.sql`（只重叠清点），按 parallel-sessions 直接在新 tip 上重放；本票两笔代码动过的 13 份文件，blob 与分支 `6be912f3` 全同。
- **验证**：钉 `d228415b`（与本记录一笔只差 `.md`）：全仓 build / vet 退 0，本票 `.go` gofmt 无输出；先单跑真库用例 `TestIsolatedLegalEntityProfileRegistrationLandsAndResolvesAgainstARealDatabase` 是 PASS 非 SKIP；带 DSN `go test -p 1 -count=1 ./...` 118 包 ok、0 FAIL（另 15 包无测试文件）。
