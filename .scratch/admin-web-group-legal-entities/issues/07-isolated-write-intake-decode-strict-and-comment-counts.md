# 07 A 类尾巴：隔离身份 Intake 的外壳解码改调 `decodeStrict`（尾随内容拒）+ 两处注释去计数

Category: bug
Status: resolved
Blocked by: 无（06 已进 main `f6569f51`）
地盘：`internal/partycommercial/adapters/http/isolated_write_intake.go`、同名 `_test.go`；不动 `publication_draft_payload.go`
出处：票 06 评审 ← 通道 2（钉 `835690b8`）Standards 非阻断 N1 / N3，可选 N2。

## 为什么

`decodeClosedDocument`（`isolated_write_intake.go`）与同包既有 `decodeStrict`（`publication_draft_payload.go`）同形，
但少了后者那道**尾随内容拒**：`{"legalEntities":[…]} {"x":1}` 在身份族五口静默放过第二个 JSON 值，在发布口答 400。
同一个包对「一份载荷只许一个文档」给出两种答复，而作者自己在 `exactlyOne` 注释里反对「无声丢掉」——此处自相矛盾。
这是同包两道解码助手的行为分叉，不是新规则。

另两处注释数的是别包的东西（AGENTS「不用计数」）：`identityKindFromName` 头注「`application.PartyIdentityKind` 封闭三值」、
`partyRoleFromName` 头注「三处译的是同一个封闭集」。同包 `PartyIdentityDeactivationIntake` 头注（`register_party_identity.go`）
里的「封闭三值」是它们沿用的源头，一并去数字。

## 要做的

1. `decodeClosedDocument` 改为调 `decodeStrict(request.Body, &document)`，错误照旧包 `ErrMalformedRequest`；删掉自己那份
   `json.NewDecoder` + `DisallowUnknownFields`。先写 red：身份口对带尾随 JSON 值的载荷答 400 `MALFORMED_REQUEST`（挑一口即可，
   `decodeClosedDocument` 是五口共用的一处；停用口若不经它，也补一格）。
2. 三处注释去计数：说「封闭集的名称镜像」、「各边界各自译同一个封闭集」，不说几值、几处。
3. 可选（N2，判断题；做就单独一笔）：`fmt.Errorf("%w: X[0].f: %v", ErrMalformedRequest, err)` 约二十处同形包装收成一个
   `malformed(field string, err error) error` 助手。不做也关票，票面写明做没做。

## 不做

- 不动 `decodeStrict` 本身、不动发布口。
- 不改五口的载荷形状、不改 `exactlyOne` / `refuseSelfReportedTenant`。
- `partyRoleFromName` / `identityKindFromName` 与 CLI、postgres 适配器的镜像（Repeated Switches）不合并——注释已给理由
  （名字属边界不属模型），评审接受。

## 完成判据

- `go build ./...`、`go vet ./...`；`internal/partycommercial/adapters/http` 与 `cmd/parcel-api` `go test -count=1`（后者带 DSN）。
- 新用例：尾随内容 → 400 `MALFORMED_REQUEST`、无 outcome；既有 `TestIsolatedPartyIdentityIntake*` 全部照旧绿。
- `rg '封闭三值|三处译' internal/partycommercial/adapters/http` 零命中。
- 清点预报零差（不增删文件）。

## 完成记录（2026-09-16，分支 `mcp4-adminweb07`，基 main `2e8183d6`）

作者：`23f3582e`（要做的 1，红→绿）与 `5dff14d7`（要做的 2，纯注释）为通道 4 所作（同通道、06 的收尾会话，即 06 评审所指的作者一侧）。隔离树 `D:/tops/idp-parcel-mcp4-adminweb07`，每笔推 origin。

对完成判据：

- `gofmt -l` 空 / `go build ./...` 0 / `go vet ./...` 0；`go test -count=1 ./internal/partycommercial/adapters/http/` ok（`-v` 182 PASS / 0 FAIL / 0 SKIP）；带 DSN `go test -p 1 -count=1 ./cmd/parcel-api/` ok（反向依赖；`-v` 探针 `TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase` PASS 非 SKIP，包内 `--- SKIP` 为 0）。
- 新用例 `TestIsolatedPartyIdentityIntakeRefusesTrailingContentAfterThePayload`：在 HTTP 缝上对登记口（`NewRegisterLegalEntityEndpoint`）与停用口（`NewDeactivatePartyIdentityEndpoint`）各投一次「合规文档 + ` {"x":1}`」，钉 400、`MALFORMED_REQUEST`、无 outcome、`scriptedPartyRegistrar.called` 为假。红时两口都答 500 `UNNAMED_OUTCOME`——命令已交出、编排被调到，正是票面说的静默放过；绿后如上。既有 `TestIsolatedPartyIdentityIntake*` 全部照旧绿。
- `git grep -e '封闭三值' -e '三处译' -- internal/partycommercial/adapters/http` 零命中（用 git 自己匹配，不经 PowerShell 解码）。
- 清点预报零差：在 `5dff14d7` 干净树跑 `tools/mechanism-inventory -out` 到临时路径，与已提交文件 `git diff --no-index` 退 0。

要做的逐条：① `decodeClosedDocument` 改为 `decodeStrict(request.Body, &document)`，错误照旧包 `ErrMalformedRequest`，自带的 `json.NewDecoder` + `DisallowUnknownFields` 删去；停用口经同一个 `decodeClosedDocument[deactivationBatchDocument]`，一处改两口齐，用例仍对停用口单投一次以钉住这一点。② 去计数四处而非票面的三处：`identityKindFromName` 头注、`partyRoleFromName` 头注、`PartyIdentityDeactivationIntake` 头注（`register_party_identity.go`，地盘外、票面点名；同句里的「三册共一个口」一并改「各册」），外加测试文件第五口头注的「封闭三值」——完成判据是对整个包目录 `rg` 归零，不改它归不了零。③ 可选 N2 助手：**未做**。理由：纯重构、无行为差、无红可写（/tdd「重构不在红绿环里」）；本票紧跟 06 评审，再动约二十行会让评审者的 diff 变大而零行为；现有写法整齐可读，`ErrMalformedRequest` 包装一眼可见。下次真要动这批 `fmt.Errorf` 时再收。

判断项：

1. 红写在 HTTP 缝而不是 Intake 方法缝：票面完成判据写的是「400 `MALFORMED_REQUEST`、无 outcome」，那是登记方看得见的三样，只有过了端点才证得到；Intake 级的 `errors.Is` 由同文件既有用例覆盖。
2. 尾随内容用 ` {"x":1}`（合法的第二个 JSON 值）而不是垃圾字节：垃圾字节改前也会被第一次 `Decode` 拒，红不出「静默放过」那一格。
3. `decodeClosedDocument` 头注改为写「与发布口共用判据」的理由与本票编号，不复述 `decodeStrict` 怎么做。

**进 main 记录**：待推送方重放后补；评审派通道 2。

## Comments
