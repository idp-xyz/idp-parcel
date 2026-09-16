# 07 A 类尾巴：隔离身份 Intake 的外壳解码改调 `decodeStrict`（尾随内容拒）+ 注释去计数（立票写两处、要做的写三处、实做四处；标题落地时对齐，评审零量级项）

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
- 新用例 `TestIsolatedPartyIdentityIntakeRefusesTrailingContentAfterThePayload`：在 HTTP 缝上对登记口（`NewRegisterLegalEntityEndpoint`）与停用口（`NewDeactivatePartyIdentityEndpoint`）各投一次「合规文档 + ` {"x":1}`」，钉 400、`MALFORMED_REQUEST`、无 outcome、`scriptedPartyRegistrar.called` 为假。红时两口都答 500 `UNNAMED_OUTCOME`——命令已交出、编排被调到，正是票面说的静默放过（500 是测试替身 `scriptedPartyRegistrar` 交回零值结果、`writePartyRegistryAnswer` 对无名结果的答复；生产上真编排会答 201，第二个文档无声消失——评审 S2 补句，推送方代落）；绿后如上。既有 `TestIsolatedPartyIdentityIntake*` 全部照旧绿。
- `git grep -e '封闭三值' -e '三处译' -- internal/partycommercial/adapters/http` 零命中（用 git 自己匹配，不经 PowerShell 解码）。
- 清点预报零差：在 `5dff14d7` 干净树跑 `tools/mechanism-inventory -out` 到临时路径，与已提交文件 `git diff --no-index` 退 0。

要做的逐条：① `decodeClosedDocument` 改为 `decodeStrict(request.Body, &document)`，错误照旧包 `ErrMalformedRequest`，自带的 `json.NewDecoder` + `DisallowUnknownFields` 删去；停用口经同一个 `decodeClosedDocument[deactivationBatchDocument]`，一处改两口齐，用例仍对停用口单投一次以钉住这一点。② 去计数四处而非票面的三处：`identityKindFromName` 头注、`partyRoleFromName` 头注、`PartyIdentityDeactivationIntake` 头注（`register_party_identity.go`，地盘外、票面点名；同句里的「三册共一个口」一并改「各册」），外加测试文件第五口头注的「封闭三值」——完成判据是对整个包目录 `rg` 归零，不改它归不了零。③ 可选 N2 助手：**未做**。理由：纯重构、无行为差、无红可写（/tdd「重构不在红绿环里」）；本票紧跟 06 评审，再动约二十行会让评审者的 diff 变大而零行为；现有写法整齐可读，`ErrMalformedRequest` 包装一眼可见。下次真要动这批 `fmt.Errorf` 时再收。

判断项：

1. 红写在 HTTP 缝而不是 Intake 方法缝：票面完成判据写的是「400 `MALFORMED_REQUEST`、无 outcome」，那是登记方看得见的三样，只有过了端点才证得到；Intake 级的 `errors.Is` 由同文件既有用例覆盖。
2. 尾随内容用 ` {"x":1}`（合法的第二个 JSON 值）而不是垃圾字节：与票面「为什么」的例子同形，钉的是「一份载荷只许一个文档」这条判据本身，不是「后面跟了坏字节」。（作者原写「垃圾字节改前也会被第一次 `Decode` 拒，红不出静默放过那一格」不成立——`encoding/json` 对顶层对象读到 `}` 即收尾、不看后文，`{"a":1} garbage` 第一次 `Decode` 返回 nil、第二次才报 `invalid character 'g'`，垃圾字节改前同样静默放过、同样能红；评审 ← 通道 2 本机 `go run` 探针，S1，推送方代改理由、用例不动。）
3. `decodeClosedDocument` 头注改为写「与发布口共用判据」的理由与本票编号，不复述 `decodeStrict` 怎么做。

**进 main 记录**（推送方 = 通道 1 新会话，19:3x 接手；前任 19:2x 占号后会话已断，其广播的那一跑全量实际未起）：零重放——分支基 `2e8183d6` = 远端 main，三笔 `23f3582e` / `5dff14d7` / `19047d51` 原 SHA ff；`%TEMP%\idp-land07` detached `19047d51` 干净检出上清点重生成 porcelain 空（零差，无单独清点笔）、`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**，退出码 0（19:37:45→19:39:38，113 s）；探针 `TestIsolatedLegalEntityRegistrationLandsAgainstARealDatabase` PASS 非 SKIP，新用例两口子测试 PASS；评审 ← 通道 2 19:49 无阻断（下）→ `ls-remote` 核 `2e8183d6` 未动 → 19:49:10 `push 19047d51:main` 成，**远端 main = `19047d51`**，CI run 35092371232；共享树 ff 同 SHA。非阻断三条代落：N1 纯注释笔 `1a2ceef9`（`register_party_identity.go` 头注一词，自审）；S1 / S2 改本票面文本（见上）；零量级项对齐标题。

## Comments

**评审 ← 通道 2 · 钉 `5dff14d7` · 19:49**（`task-8d992f9a`；通道 2 为 19:41 认领的新会话，复用前任 19:25 留下的干净检出 `%TEMP%\idp-review-07`，评完已拆；作者分支未动、未跑全仓、未占 55432。本机核：`gofmt -l` 三件空、`go vet` 本包 0、`-run TestIsolatedPartyIdentityIntake` ok；把 `isolated_write_intake.go` 临时换回 `2e8183d6` 版再跑新用例：登记口 / 停用口皆 FAIL、答 `500 {"error":{"code":"UNNAMED_OUTCOME"}}`——红属实，已恢复、status 空。）

- **Standards（阻断 0 / 非阻断 1）**：阻断无。① `decodeClosedDocument` 只剩 `decodeStrict(request.Body, &document)` + `ErrMalformedRequest` 包裹，自带解码器已删，无第二份解码逻辑。② 红态 500 来自 `scriptedPartyRegistrar.answer()` 交回零值 `PartyRegistryResult`，`writePartyRegistryAnswer` 对无名结果答 500——那是替身的答复；生产上真编排会答 201，第二个文档无声消失。放过发生在 Intake，与 06 N1 一致。票面「为什么」的「静默放过」不必改口；完成记录可补半句（S2）。③ 新用例断 400 / `MALFORMED_REQUEST` / 无 outcome / `registrar.called` 为假，登记口与停用口各一格，成立。④ 四处注释说得通、无新计数。**N1（低）**：`register_party_identity.go` `PartyIdentityDeactivationIntake` 头注「命令自带身份种类（封闭集的名称镜像…）」——命令字段装的是 `application.PartyIdentityKind` 本身，「名称镜像」是 HTTP 侧 `identityKindFromName` 干的事，放在命令上不精确；写「封闭集」即可。词级，推送方代落可顺手改。⑤ `git diff -U0 23f3582e 5dff14d7` 滤去注释行后为空。新头注引「票 admin-web-group-legal-entities/07」与全仓 `cmd/` `internal/` 既有写法同形，接受。
- **Spec（阻断 0 / 非阻断 2）**：阻断无。要做的 1 ✓；2 ✓ 三处 + 测试第五口头注一处——第四处不改则 rg 判据归不了零，超票面但为判据所需；3 可选未做，理由（纯重构无红、不加大紧跟 06 评审的 diff）站得住，票面自己写了「不做也关票」。不做三条未碰：只 3 件，`publication_draft_payload.go` 与发布口零改动；五口载荷 struct、`exactlyOne`、`refuseSelfReportedTenant` 未动；两个镜像函数只改注释。完成判据：vet / gofmt / 定向用例本机 0；全量由作者与通道 1 各跑一遍（19:40 广播 115 ok、探针 PASS）；`git grep -e 封闭三值 -e 三处译 -- internal/partycommercial/adapters/http` 在 `5dff14d7` 零命中；清点零差。票面笔 `19047d51` 判断项 1、3 ✓。**S1**：判断项 2「垃圾字节改前也会被第一次 Decode 拒」不成立——`encoding/json` 对顶层对象读到 `}` 即收尾、不看后文；本机 `go run` 探针 `{"a":1} garbage` 第一次 Decode err=nil，第二次才报 `invalid character 'g'`——垃圾字节改前同样静默放过、同样能红。选 ` {"x":1}` 本身没错（与票面「为什么」的例子同形），但理由要改写；票面文本、非代码，推送方代落时改。**S2**：完成记录「红时两口都答 500 UNNAMED_OUTCOME——…正是票面说的静默放过」结论对、措辞可补：500 为替身零值答复，生产上是 201 + 第二个文档无声消失，半句即可。零量级：票标题「两处注释去计数」vs 要做的 2「三处」vs 实做四处，票内预存不一致，非本 diff 引入，落地时可对齐标题。
- **汇总**：Standards 0 阻断 / 1 非阻断（N1 词级）；Spec 0 阻断 / 2 非阻断（S1 判断项 2 对 `encoding/json` 的断言不成立；S2 完成记录补半句）。无阻断，可 push。

**推送方处置（通道 1 · 19:5x）**：N1 → `1a2ceef9`（纯注释、自审）；S1 → 判断项 2 理由改写（上）；S2 → 完成记录补句（上）；零量级 → 标题对齐。三条均随本簿记笔代落，作者无需再动。
