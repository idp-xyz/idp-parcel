# 49 NO adapters/http 测试与注释照实写：收寄替身交回不同节点、死引用改指、去变更说明

Category: chore
Status: in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 序 6 评审遗留；用户授权代决）；分支 `claude-oc49`
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

- [ ] 收寄替身认证方交回与请求头 `Parcel-Work-Scope-Reference` 不同的节点，用例断言命令里的节点取自认证结果；把实现改成直接取头值时该用例转红（变异自检写进完成记录）。
- [ ] 死引用改指符号名；变更说明删去或改写成「为什么」。
- [ ] 改动文件 `git diff --check` 干净、无 CR / BOM；注释中文、只写为什么、不写变更说明、跨文件引用不用行号也不用计数。
- [ ] 改动包 `gofmt -l` 无输出，`go build ./...`、`go vet ./...` 退 0，改动包及反向依赖与 `./internal/architecture/...` 带 DSN `go test -p 1 -count=1` 全 ok。
- [ ] 非作者评审（隔离子代理两轴）0 阻断；推送方在干净检出上带 DSN 跑全量；机制清点有变则重生成。
