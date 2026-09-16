# 07 A 类尾巴：隔离身份 Intake 的外壳解码改调 `decodeStrict`（尾随内容拒）+ 两处注释去计数

Category: bug
Status: ready-for-agent
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

## Comments
