# 49 NO adapters/http 测试与注释照实写：收寄替身交回不同节点、死引用改指、去变更说明

Category: chore
Status: resolved——2026-10-18 进 main（与 40、47、49 同批，代码验证钉 `b3b246d7`），见「进 main 记录」。此前 in-progress——2026-10-18 完工；非作者评审第 1 轮 0 阻断，非阻断已修，待进 main。此前 in-progress——2026-10-18 作者子代理完工，待非作者评审；分支 `claude-oc49`，见文末「完成记录」。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 序 6 评审遗留；用户授权代决）；分支 `claude-oc49`
Blocked by: 无
父票：[26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)（票 26 评审遗留的后续）
出处：票 [23](./23-node-operations-isolated-command-intake-retires.md) Comments 评审 Spec 非阻断 1、Standards 非阻断 1–2。
地盘：
- internal/nodeoperations/adapters/http/work_fact_intake_test.go
- internal/nodeoperations/adapters/http/reception_payload.go（只改注释）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

不碰 receive_delivered_unit*.go（归 operator-channel/29）。

## 裁定

无需裁定（机械项）。

## 做什么

### C8-9（仍在：True；要裁：False）
- 出处：票 operator-channel/23 Comments 评审（钉 9c551dc4）Spec 非阻断 1：「替身认证方交回的节点，和 `receptionRequest` 里 `Parcel-Work-Scope-Reference` 头的值相同……测不出文件头那句「租户与节点两格只来自认证结果」」
- 位置：internal/nodeoperations/adapters/http/work_fact_intake_test.go · const workFactNode；receptionAuthenticatorFake.AuthenticateNodeReception；receptionRequest；TestWorkFactReceptionIntakeTranslatesReceptionWithTheAuthenticatedTenantAndNode；TestWorkFactReceptionIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst
- 现状：const (
	workFactTenant = "SYN-TENANT-01"
	workFactNode   = "SYN-NODE-SHA-HUB"
)
…
	request.Header.Set("Parcel-Work-Scope-Reference", workFactNode)
…
		t.Fatalf("Node = %q, want %q（认证结果，不是请求里的自报）", got, workFactNode)
…
	if len(asked) != 1 || asked[0] != "SYN-DEVICE-NODE|NODE|"+workFactNode {
- 改法：把一个常量拆成两个，替身照旧交回 workFactNode：
const (
	workFactTenant = "SYN-TENANT-01"
	// workFactNode 是替身认证方交回的节点；presentedScopeReference 是请求头 Parcel-Work-Scope-Reference 出示的引用。两值刻意不同：
	// 相同时 Intake 改拿头值铸节点也照样全绿，「节点只来自认证结果」那一格就测不出来。
	workFactNode            = "SYN-NODE-SZX-01"
	presentedScopeReference = "SYN-NODE-SHA-HUB"
)
然后改三处：
- receptionRequest：`request.Header.Set("Parcel-Work-Scope-Reference", presentedScopeReference)`
- TestWorkFactReceptionIntakeTranslatesReceptionWithTheAuthenticatedTenantAndNode：仍断言 `command.Node.String() == workFactNode`（认证结果），失败消息改为 `t.Fatalf("Node = %q, want %q（认证结果，不是请求头 Parcel-Work-Scope-Reference 出示的 %q）", got, workFactNode, presentedScopeReference)`
- TestWorkFactReceptionIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst：认证方被问到的仍应是头里那一份，断言改为 `asked[0] != "SYN-DEVICE-NODE|NODE|"+presentedScopeReference`
其余用例不动。
- 备注：变异实测（钉 8d22729f，用 go test -overlay；探针都在 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide/C8/）。变异体是 mutant_work_fact_intake.go：IntakeReception 丢掉认证结果里的节点，改拿 Parcel-Work-Scope-Reference 头值铸节点。四种组合的结果：
① 原码加原测试：ok。
② 变异体加原测试：ok，评审说的缺口复现。
③ 原码加改后测试：本包全 PASS，vet 退 0，gofmt 无输出。
④ 变异体加改后测试：FAIL，输出为 `Node = "SYN-NODE-SHA-HUB", want "SYN-NODE-SZX-01"（认证结果，不是请求头 Parcel-Work-Scope-Reference 出示的 "SYN-NODE-SHA-HUB"）`。
生产认证方 nodeoperations/adapters/accessidentity 的 WorkFactAuthenticator.AuthenticateNodeReception 取的是 envelope.WorkScope().Reference()；铸造方核过设备的登记范围，所以生产上这两个值相等。让替身交回一个不同的节点只是为了测出差别，并不是在模拟真实行为，新注释写明了这一点。SYN-NODE-SZX-01 是仓内已有的合成节点标识，证据只记 S。与 C1 不同文件。

### C8-10（仍在：True；要裁：False）
- 出处：票 operator-channel/23 评审（钉 9c551dc4）Standards 非阻断 (1)：「注释写「两道判据同 partycommercial 的隔离身份 Intake」，那个类型在 `4735f9dd` 已删，是死引用」
- 位置：internal/nodeoperations/adapters/http/reception_payload.go · decodeClosedDocument 头注末句
- 现状：// 尾随的第二个 JSON 值也拒：一份载荷只许一个文档，放过它就是无声丢掉一段输入。两道判据同 partycommercial 的隔离身份
// Intake。
- 改法：换成：
// 尾随的第二个 JSON 值也拒：一份载荷只许一个文档，放过它就是无声丢掉一段输入。两道判据同 partycommercial 适配器
// （`internal/partycommercial/adapters/http`）的 decodeClosedDocument。
- 备注：4735f9dd 删掉了 IsolatedPartyIdentityIntake，并把 isolated_write_intake.go 改名为 party_identity_payload.go。现在的对应物是 commercialhttp 包里的 decodeClosedDocument[Document any]，它交给同包的 decodeStrict 去做，判据与 NO 这边相同：先 DisallowUnknownFields，再读一次必须得到 io.EOF，否则按尾随内容拒。
编排方原本建议改指文件 party_identity_payload.go，这里改成「包路径加符号名」：这条死引用正是由文件改名加类型删除造成的，文件名比符号更容易再次失效。只改注释。与 C1 不同文件。

### C8-11（仍在：True；要裁：False）
- 出处：票 operator-channel/23 评审（钉 9c551dc4）Standards 非阻断 (2)：`reception_payload.go` 文件头「它原住在隔离命令 Intake 的文件里…行为一字未改」、测试文件头「原挂在隔离命令 Intake 上…期望值不变」（后者的「票 23」没带命名空间）
- 位置：internal/nodeoperations/adapters/http/reception_payload.go 文件头注释；internal/nodeoperations/adapters/http/work_fact_intake_test.go 文件头注释
- 现状：// 它原住在隔离命令 Intake 的文件里，随那只 Intake 退场挪出（票 operator-channel/23），行为一字未改。
——
// 租户即拒、形状错即拒。这组译装用例原挂在隔离命令 Intake 上，随它退场迁来（票 23；ADR-0150 决定三），期望值不变。
// 期望值取自载荷字面量而不是重算：这里证的是「译装没换字」。
- 改法：(a) reception_payload.go：删掉「它原住在隔离命令 Intake 的文件里，随那只 Intake 退场挪出（票 operator-channel/23），行为一字未改。」这一句，保留前一句「本文件是节点收寄口的线格式与译装：…（ADR-0023）。」。
(b) work_fact_intake_test.go：引文那段改成「// 租户即拒、形状错即拒。期望值取自载荷字面量而不是重算：这里证的是「译装没换字」。」，也就是删掉变更说明那句，保留讲方法的那句。
- 备注：这两句都只记录了某一笔做过什么，搬迁本身在 git 历史里查得到（9c551dc4）。work_fact_intake_test.go 与 C8-9 是同一个文件，建议同一笔提交。只改注释。与 C1 不同文件。

## 完成判据

- [x] 收寄替身认证方交回与请求头 `Parcel-Work-Scope-Reference` 不同的节点，用例断言命令里的节点取自认证结果；把实现改成直接取头值时该用例转红（变异自检写进完成记录）。
- [x] 死引用改指符号名；变更说明删去或改写成「为什么」。
- [x] 改动文件 `git diff --check` 干净、无 CR / BOM；注释中文、只写为什么、不写变更说明、跨文件引用不用行号也不用计数。
- [x] 改动包 `gofmt -l` 无输出，`go build ./...`、`go vet ./...` 退 0，改动包及反向依赖与 `./internal/architecture/...` 带 DSN `go test -p 1 -count=1` 全 ok。
- [ ] 非作者评审（隔离子代理两轴）0 阻断；推送方在干净检出上带 DSN 跑全量；机制清点有变则重生成。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-oc49）

**基线**：分支基 `b58b10ef`。两笔改动：`46333ca6`（测试，C8-9 与 C8-11 b，同一文件同一笔）、`b8aac194`（只改注释，C8-10 与 C8-11 a）；本记录另一笔在其后。

**落点**：只在地盘内。
- `internal/nodeoperations/adapters/http/work_fact_intake_test.go`：常量拆成 `workFactNode`（替身认证方交回，`SYN-NODE-SZX-01`）与 `presentedScopeReference`（请求头 `Parcel-Work-Scope-Reference` 出示，`SYN-NODE-SHA-HUB`）；`receptionRequest` 改出示后者；两条用例的断言照「改法」改；常量注释写明两值刻意不同的理由，并写明生产上两值相等、替身答别的节点不是在模拟真实答复。文件头删去变更说明那句，保留讲方法的那句。
- `internal/nodeoperations/adapters/http/reception_payload.go`：文件头删去「原住在隔离命令 Intake 的文件里……」那句；`decodeClosedDocument` 头注的死引用改指 `internal/partycommercial/adapters/http` 的 `decodeClosedDocument`（核过：它经同包 `decodeStrict` 判，先 `DisallowUnknownFields`，再读一次必须得到 `io.EOF`，与本包相同）。只改注释。

**完成判据**（钉 `b8aac194`；Linux（WSL2），go1.26.8，真库为 55432 门禁库）
- ✅ 节点取自认证结果：`TestWorkFactReceptionIntakeTranslatesReceptionWithTheAuthenticatedTenantAndNode`（替身交回的节点与请求头出示的不同，断言命令里是前者）；认证方被问的仍是头里那一份：`TestWorkFactReceptionIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst`。变异自检见下文「先红后绿」。
- ✅ 死引用改指符号名（包路径加 `decodeClosedDocument`）；两处变更说明删去。两文件再扫「原挂 / 原住 / 迁来 / 挪出 / 一字未改 / 不带命名空间的票号」，无残留。
- ✅ 两文件 `git diff --check` 干净，CR 计数 0，文件首三字节为 `pac`（无 BOM）；注释中文、只写为什么，跨文件引用用包路径加符号名，不用行号也不用计数。
- ✅ 门全过，见下文「门」。
- ⬜ 非作者评审未做（待非作者评审）；推送方全量归推送方；机制清点已重生成，与现行文档逐字相同，无提交。

**先红后绿**（变异取证，`go test -overlay` 把 `work_fact_intake.go` 换成变异体：`IntakeReception` 丢掉认证结果里的节点，改拿 `Parcel-Work-Scope-Reference` 头值铸节点；变异体只在会话草稿区，不进提交）
1. 原码加原测试：`ok`。
2. 变异体加原测试：`ok`——评审说的缺口复现（两值相同，测不出）。
3. 变异体加改后测试：**红**，`--- FAIL: TestWorkFactReceptionIntakeTranslatesReceptionWithTheAuthenticatedTenantAndNode`，输出 `Node = "SYN-NODE-SHA-HUB", want "SYN-NODE-SZX-01"（认证结果，不是请求头 Parcel-Work-Scope-Reference 出示的 "SYN-NODE-SHA-HUB"）`。红落在行为上，不是编译错。
4. 原码加改后测试：**绿**，本包收寄 Intake 各用例全 PASS（`-v` 逐条核过）。

生产实现本就正确，本票只补测试的辨别力，所以「绿」不需要改实现。

**门**（作者自验，工作树 `/home/tops/workspace/idp-parcel-claude-oc49`）
- `gofmt -l` 两个改动文件：无输出。
- `go build ./...`、`go vet ./...`：退 0。
- `IDP_PARCEL_POSTGRES_DSN='postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable' go test -p 1 -count=1`，范围是改动包 `./internal/nodeoperations/adapters/http/...`、直接引用它的 `./internal/nodeoperations/adapters/accessidentity/...`（无测试文件）与 `./cmd/parcel-api/...`，以及 `./internal/architecture/...`：全 ok。
- 机制清点 `tools/mechanism-inventory` 重生成：无差。

**证据层级**：全部 `S`（`SYN-NODE-SZX-01`、`SYN-NODE-SHA-HUB` 都是仓内已有的合成标识）。

**评审**：待非作者评审。

## Comments

- **评审 ← 会话 idp-parcel-9b 起的隔离子代理（非作者）· 钉 `68184011`（基线 `b58b10ef`）· 2026-10-18 · 第 1 轮**
  - **Spec**：阻断无，非阻断无。C8-9 常量拆分、请求头改出示 `presentedScopeReference`、两条断言与票面一致；变异体（Intake 改拿头值铸节点）在改后用例上转红、在原用例上为绿。C8-10 死引用改指包路径加 `decodeClosedDocument`；C8-11 两处变更说明已删。
  - **Standards**：阻断无。非阻断 1：`workFactNode` 头注「生产上铸造方核过设备的登记范围」讲别包行为却没指符号。
  - **处置**：非阻断已在 `e0f84cbc` 改指 `WorkFactMinter.MintWorkFact` 与 `WorkFactAuthenticator.AuthenticateNodeReception`。

### 进 main 记录（2026-10-18，会话 idp-parcel-9b）

- 原分支 `claude-oc49` 各笔重放到 47 之后（新 SHA `2928d641`…`b3b246d7`；原 SHA 作封存出处）。同批三票 40、47、49 一次推送。
- 推送方全量一次：钉 `b3b246d7` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 142 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上改动 `.go` 的 `gofmt -l` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
- 评审见 Comments。
