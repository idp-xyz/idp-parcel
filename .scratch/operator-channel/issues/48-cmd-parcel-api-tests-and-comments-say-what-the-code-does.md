# 48 cmd/parcel-api 测试与注释照实写：隔离形态残名改名、去变更说明、序号引用改引文

Category: chore
Status: ready-for-agent——2026-10-18 暂缓：用户因 token 花费选「精简续做」，本票没做完；分支 `claude-oc48`（基 `b58b10ef`，工作树 `/home/tops/workspace/idp-parcel-claude-oc48`）上留有4 笔（隔离形态残名、Covers、作业事实装配表、isolatedWriteAdmission 头注），未经评审，接手前先 rebase 到最新 main 并核一遍。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 序 4、序 6、序 2 评审遗留；用户授权代决）；分支 `claude-oc48`
Blocked by: 无
父票：[26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)（票 26 评审遗留的后续）
出处：票 [15](./15-operation-decision-faces-take-operator-intake.md) Comments「钉 `60244fa6`」那条评审 Standards 非阻断 1–3（cmd 部分）；票 [23](./23-node-operations-isolated-command-intake-retires.md) Comments 评审 Standards 非阻断 3；票 [25](./25-node-reception-carries-parcel-shipment-service-result-markers.md) Comments 评审 Standards 非阻断 2 的同类横扫。
地盘：
- cmd/parcel-api/assemble_isolated_write.go（只改注释）
- cmd/parcel-api/assemble_work_fact_operator_test.go
- cmd/parcel-api/transport_fulfillment_command_lines_test.go
- cmd/parcel-api/isolated_write_reception_test.go（改名）
- C1-6 草案点名的各测试文件里的序号引用（含 internal/accessidentity/adapters/pilotgovernance 与 internal/partycommercial/application 两处测试）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

本票合并三份研判草案的 cmd 部分。endpoints.go 整份归对端会话的 operator-channel/35，本票不碰；那几份草案里改 endpoints.go 的条目已转交 35。assemble_reception* 归 operator-channel/29，本票不碰。

## 裁定

- 无业务裁定，都是注释、命名与测试表的机械修正。
- 序号引用那一问由本会话代决：凡 Go 注释以序号指票面条目（「完成判据第N条」「裁定第N条」「做什么第N条」）一律改成引文；UC 应用流程表自带的步骤标号近于符号，可保留。

## 做什么

### 一、隔离形态残名与变更说明（簇 C8 的 cmd 部分）

##### C8-1（仍在：True；要裁：False）
- 出处：票 operator-channel/15 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `60244fa6`」Standards 非阻断 (1)：「`operationDecisionFacesLeftOnIsolatedWrite` 意为「还留在隔离写上」，本笔之后已不成立」
- 位置：cmd/parcel-api/assemble_work_fact_operator_test.go · var operationDecisionFacesLeftOnIsolatedWrite 及其头注；同文件 TestWorkFactChannelLeavesOperationDecisionFacesUnconfigured
- 现状：// 这四口归运营决定（票 15），作业事实渠道就位之后仍答未配置。
var operationDecisionFacesLeftOnIsolatedWrite = []string{
…
	for _, pattern := range operationDecisionFacesLeftOnIsolatedWrite {
- 改法：改名为 transportDecisionFacesOutsideWorkFact。全部引用点：在 main 8d22729f 上执行 `grep -rn operationDecisionFacesLeftOnIsolatedWrite --include=*.go .`，只命中本文件的两处：(1) var 声明；(2) TestWorkFactChannelLeavesOperationDecisionFacesUnconfigured 里的 `for _, pattern := range …`。票 operator-channel/15 评审记录里提到的旧名是历史记录，不改。
头注整句换成：
// transportDecisionFacesOutsideWorkFact 是 ADR-0151 决定六点名归运营决定、不归作业事实登记的运输履约各口。
// 只配作业事实渠道时，它们答的是运营决定那一面的未配置：作业事实的认证与授予够不着它们。
列表内容不动；测试名 TestWorkFactChannelLeavesOperationDecisionFacesUnconfigured 仍然属实，不改。不要用 operationDecisionTransportFaces 这个名字：同包的 transport_fulfillment_command_lines_test.go 里已有同名变量（类型是 []accessidentity.DecisionKind），会编译冲突，也容易看混。
- 备注：旧名描述的是过渡状态。自 60244fa6 起，这几口的端点行直接挂 operatorDecisions.transport；isolatedWriteAdmittedCommandLines 只剩 "/shipment-requests"。新名锚在 ADR-0151 决定六上，这个分类不会随换口而变：决定六点名的正是这几口，其中三口划出 ADR-0149 决定一，承运商首次有效收寄判断「同归决定一」。新头注顺带去掉了没带命名空间的「票 15」和计数「这四口」。
考虑过改成遍历 assemble_operator_decisions_test.go 的 swappedDecisionFaces，没有采用：那张表收的是「已换口的全部运营决定口」，分类与本表不同；并过去会把委托侧各口也拉进作业事实用例，语义变宽。
实测：用 overlay 换入后 go vet 退 0，本用例 PASS。与 C1 不同文件。
【整簇补丁】先执行 `git mv cmd/parcel-api/isolated_write_reception_test.go cmd/parcel-api/node_operations_reception_line_test.go`，再 `git apply /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide/C8/c8_after_git_mv.diff`。这个补丁已在临时副本上核过：`git apply --check` 通过，应用后各文件与 overlay 版本逐字节相同。同目录下的 ov_c8_all.json 就是这套 overlay。钉 8d22729f 带 DSN 实测 cmd/parcel-api 与 NO adapters/http 两包：gofmt 无输出，vet 退 0，`go test -p 1 -count=1 -v` 为 140 PASS / 0 SKIP / 0 FAIL；main 上同样是 140 PASS，顶层用例名只差 C8-7 那一次改名。
【研判副作用】有一次执行 `go build -overlay … ./cmd/parcel-api/` 时没加 -o，在仓库根写出了 parcel-api 二进制，覆盖了原有的同名构建物。这个文件被 .gitignore 的 `/parcel-*` 忽略，git 状态没变（`git status --short` 为空，HEAD 仍是 8d22729f）。新二进制的 vcs.revision=8d22729f、vcs.modified=false；overlay 对非测试文件只改了注释，所以它与 main 构建等价。之后的构建都改用 `-o /dev/null`。

##### C8-4（仍在：True；要裁：False）
- 出处：票 operator-channel/15 评审（钉 60244fa6）Standards 非阻断 (2) 前半：`isolatedWriteAdmission` 的字段注释是变更说明；票 operator-channel/23 评审（钉 9c551dc4）Standards 非阻断 (2)：「`isolatedWriteAdmission` 里「…节点作业那一格随票 23」」
- 位置：cmd/parcel-api/assemble_isolated_write.go · type isolatedWriteAdmission：tenant 字段之后、结构体右括号之前那行不依附任何字段的注释
- 现状：	// 主链各上下文的隔离命令 Intake 已随真渠道逐口撤下：运输履约那一格随票 operator-channel/15、节点作业那一格随票 23。
- 改法：整行删除，不写替代句。结构体头注已经写明现行规则：「命令面按端点逐口放行（Consequences），又随真渠道逐口撤下（ADR-0150 决定三）：哪几口此刻仍经写开关放行由 admittedCommandLines 说」。删掉这行不丢信息。
- 备注：这行的来历：在 60244fa6，它是 nodeOperations 字段上的字段注释，写的是「运输履约那一格随运营决定四口换操作者渠道撤下」，说的是同一笔删掉的 transportFulfillment 字段。9c551dc4 删 nodeOperations 字段时，把它改写成现在这行：说的仍是已删的字段，而且「票 23」没带命名空间（.scratch 下不止一个目录有 23 号票）。
实测：删掉后 go vet 退 0，cmd/parcel-api 全包带 DSN 测试绿。与 C1 不同文件。

##### C8-7（仍在：True；要裁：False）
- 出处：票 operator-channel/23 Comments 评审（钉 9c551dc4）Standards 非阻断 (3) 前半：「`isolated_write_reception_test.go` 及其用例名，自 `ec00b795` 起测的已是作业事实渠道（TF 侧同类文件已在 `60244fa6` 改名）」
- 位置：cmd/parcel-api/isolated_write_reception_test.go（文件名）· TestIsolatedReceptionAnswersBusinessOutcomesAgainstARealDatabase（用例名与 Covers 头注）
- 现状：文件：cmd/parcel-api/isolated_write_reception_test.go
// Covers: 票 operator-channel/10 完成判据第二条——节点收寄口改走作业事实渠道。编排走生产装配（buildReceptionOrchestration），
// 中间是生产的端点构造函数。作业范围种类必须是 NODE，节点引用取作业范围的引用。
func TestIsolatedReceptionAnswersBusinessOutcomesAgainstARealDatabase(t *testing.T) {
- 改法：1. `git mv cmd/parcel-api/isolated_write_reception_test.go cmd/parcel-api/node_operations_reception_line_test.go`
2. 用例改名为 TestNodeOperationsReceptionLineAnswersBusinessOutcomesAgainstARealDatabase。
3. Covers 头注开头那段换成：
// Covers: 票 operator-channel/10 完成判据「隔离放行用例改写为真渠道答复格用例」在节点收寄口上的那一格——收寄口走作业事实渠道。
// 编排走生产装配（buildReceptionOrchestration），中间是生产的端点构造函数。作业范围种类必须是 NODE，节点引用取作业范围的引用。
文件里其余内容原样随文件走，不改，包括 stampWorkFact、workFactVerifier、workFactOperators、workFactDevices、mustScope、workFactReceptionForTest、workFactTransportForTest 这些夹具（assemble_work_fact_operator_test.go 与 transport_fulfillment_command_lines_test.go 都在用）。
TF 侧参照：60244fa6 把 cmd/parcel-api/isolated_write_transport_fulfillment_test.go 改名为 transport_fulfillment_command_lines_test.go，用例 TestIsolatedTransportFulfillmentLinesAnswerBusinessOutcomesAgainstARealDatabase 改名为 TestTransportFulfillmentCommandLinesAnswerBusinessOutcomesAgainstARealDatabase。NO 只有收寄一口，所以取 reception_line。
- 备注：旧名在仓内的其他出现只在 .scratch 历史记录里：operator-channel/08、10，ps-external-mark-relations/02，以及票 23 的评审本身。那些是当时的取证记录，不改；.github 与 scripts 都没有引用。「完成判据第二条」是按位置指一个无编号列表，顺手换成引文；引文已核过，在票 10「完成判据」里逐字出现。
实测（钉 8d22729f 加 overlay：删旧路径、加新路径，带 DSN）：新用例 PASS，不是 SKIP；cmd/parcel-api 与 NO adapters/http 两包的顶层用例名集合与 main 相比只差这一次改名。机制清点不受影响：tools/mechanism-inventory 的 CensusFiles 只按 `_test.go` 后缀计数，改名不增删文件。
与 C1：本文件不在 assemble_reception* 之列，但它调用 assemble_reception.go 的 buildReceptionOrchestration，并断言明确拒收答 INTAKE_NOT_FORMED、号码不在册的明确接收答 PENDING_IDENTIFICATION。C1 若改这个构造函数的签名或这两支的答复，就得改本文件，与本条的改名相撞（rename 对 modify）。建议改名单独提交一笔，后落的一方变基即可，git 的改名识别会把对方的改动带过去。按票 25 评审「拒收、仅扫描、待识别、身份冲突四支都不问读口」，C1 改「读不动」那一支时预计不碰这两条断言。

##### C8-8（仍在：True；要裁：False）
- 出处：票 operator-channel/15 评审（钉 60244fa6）Standards 非阻断 (3) 中 cmd/parcel-api 部分：「`TestTransportFulfillmentCommandLines…` 头注「票 15 完成判据第三、四条」，按位置指一个没有编号的列表，也没有引原文」。同条里的 `decisionLines` 头注「本包六个运营决定口」在 TF adapters/http 的 operator_decision_intake_test.go，不归 C8
- 位置：cmd/parcel-api/transport_fulfillment_command_lines_test.go · TestTransportFulfillmentCommandLinesAnswerBusinessOutcomesAgainstARealDatabase 的 Covers 头注首句；同文件 type transportCommandLine 头注末句（同类，一并改）
- 现状：// Covers: 票 operator-channel/10 完成判据第二条与票 15 完成判据第三、四条——作业事实各口走作业事实渠道（作业事实登记授予 +
——
// 表里已没有经隔离写放行的行（ADR-0150 决定三；票 operator-channel/10 余下半边、票 15 余下四口）。
- 改法：(a) Covers 首行换成下面这两行，每段引文都不折行，后续行不动：
// Covers: 票 operator-channel/10 完成判据「隔离放行用例改写为真渠道答复格用例」，与票 operator-channel/15 完成判据
// 「TF 各口答业务结果」「其隔离用例改写为真渠道答复格用例」——作业事实各口走作业事实渠道（作业事实登记授予 +
(b) transportCommandLine 头注末句「表里已没有经隔离写放行的行（ADR-0150 决定三；票 operator-channel/10 余下半边、票 15 余下四口）。」换成「写开关换不了表里任何一行（ADR-0150 决定三）。」
- 备注：引文已逐字核过：用 `grep -c` 查，「TF 各口答业务结果」「其隔离用例改写为真渠道答复格用例」在票 15 正文里各出现一次，「隔离放行用例改写为真渠道答复格用例」在票 10 正文里出现一次。两张票的「完成判据」都是无编号列表，所以「第二条」「第三、四条」都是按位置指。(b) 的旧句是变更说明，还带着没有命名空间的「票 15」。
TF adapters/http 的 `decisionLines` 头注属于同一条评审，但不在 C8 的地盘（C8 只管 cmd/parcel-api 与 NO adapters/http），留给 TF 那一簇。只改注释。与 C1 不同文件。

### 二、作业事实装配测试的表与真库表的 Covers（簇 C4 第二张草案，去掉 endpoints.go 部分）

#### 现象（钉 `8d22729f`；`c68e4342` 没有改这几份文件）

- 头注与行注相抵。`assembleBusinessEndpoints` 头注写「写开关此刻只剩这一行可换」，同一函数里却有：
  - `/node-operations-records` 上方「走写开关的逐口变量（票 operator-channel/08）」；
  - `/transport-fulfillment/delivery-attempts` 上方「隔离形态经写开关逐口放行」；
  - 控制事实一组「登记与执行三行经写开关逐口放行……两个更正口仍挂字面量 UnconfiguredIntake{}」；
  - `/transport-fulfillment-delivery-dispatch-triggers` 上方「隔离形态经写开关放行这一口（票 operator-channel/08）」。
  这些行的实参都已是 `workFactTransport` 一族的变量。
- 变更说明：
  - 头段「这句从前说的是「命令面全都换不了」……已不在例外之列。剩下那几行的字面量纪律一字未松。」；
  - 主链变量段「这段原是 ADR-0091 的逐口隔离放行（票 operator-channel/08），各口已随真渠道撤下放行。」；
  - 作业事实变量块「……隔离写放行同笔撤下……（票 operator-channel/15 余下四口，隔离写放行同笔撤下）」；
  - TF admin 写面「关段与建派送任务两行的隔离写放行随换口同笔撤下」；
  - 有效时间判断、承运商首次有效收寄判断两段的「（票 operator-channel/15，隔离写放行同笔撤下）」；
  - `isolatedWriteAdmission` 末句「主链各上下文的隔离命令 Intake 已随真渠道逐口撤下……」。
- 头段删掉历史句以后，余下的「其余命令面仍挂字面量 `UnconfiguredIntake{}`」也不实：集成客户端各行走 `integrationClients.*`，登记写面走 `operatorRegistries.*`。
- 计数：`/node-operations-records` 上方「（NO 三册、TF 四册）」数的是两个查阅端点的 registry 封闭集。自 `f56f6107` 给 TF 加了 `registryCarrierMasterDocument` 起，这个数已经错了。
- 运输履约登记写面各行（凭证登记与适用变更、有效时间规则、总单登记与修订）的行注写「同挂字面量 UnconfiguredIntake{}」「登记方身份没有可采信的渠道前 Intake 恒堵」，实参却是 `operatorRegistries.transport`，即操作者渠道的登记册配置写（ADR-0100 决定四；票 operator-channel/04）。
- 控制事实段「揽收更正口……不进段——段侧重派生另立票」：票 tf-segment-lifecycle-closure/10 之后编排已经会重派生；生产上没重派生是装配缺口，归「揽收更正装配缺段登记册」票。这句不该再说「另立票」。
- `assemble_work_fact_operator_test.go` · `operationDecisionFacesLeftOnIsolatedWrite`：名字和注释「作业事实渠道就位之后仍答未配置」说的是「还留在隔离写上」。实际上 `TestWorkFactChannelLeavesOperationDecisionFacesUnconfigured` 答未配置，是因为路由里给的是 `unconfiguredOperatorDecisions()`。表里还缺运营决定中的装载分配与终止参与。
- `transport_fulfillment_command_lines_test.go` · `TestTransportFulfillmentCommandLinesAnswerBusinessOutcomesAgainstARealDatabase` 的 Covers：
  - 「票 operator-channel/10 完成判据第二条与票 15 完成判据第三、四条」是按位置指向无编号的列表；
  - 「四口的隔离写放行已撤，这几行就是改写后的真渠道答复格用例」是变更说明；
  - 「运营决定四口」是计数。
  `transportCommandLine` 头注「表里已没有经隔离写放行的行（……）」是同一类问题。

#### 裁定

这是机械修正。照 AGENTS.md「写代码注释」「改文档」两节：只写现行规则和为什么，不写变更说明，跨文件只用符号名或引文，不计数。有两处取舍：

- 测试表取运营决定的全集，即 ADR-0151 决定一里运输履约那几口，不取只列决定六那几口。理由：用例名说的是「运营决定各口」，全集断言更强，而且实测仍绿。
- 真库表的 Covers 只引它证明的业务判据（票 15「越过 Intake 之后：TF 各口答业务结果」），不引「隔离用例改写为真渠道答复格用例」这一类改写史。

#### 做什么

1. 头段：删去「这句从前说的是……一字未松。」。「其余命令面仍挂字面量」改为先点名各条真渠道，再说其余：作业事实各口走 `workFactReception` / `workFactTransport`，运营决定各口走 `operatorDecisions`，登记写面走 `operatorRegistries`，集成客户端各口走 `integrationClients`；其余才挂字面量 `UnconfiguredIntake{}`、不经由任何变量，两个隔离开关都换不了它们。
2. 主链变量段：删去「这段原是 ADR-0091 的逐口隔离放行……撤下放行。」。作业事实变量块改为：「作业事实各口挂操作者渠道的「作业事实登记」能力面：首登（ADR-0149 决定二）与更正（随原事实归族，ADR-0151 决定四）；写开关换不了它们（ADR-0150 决定三）。关段、建派送任务、有效时间判断、承运商首次有效收寄判断归运营决定（ADR-0151 决定六），端点行直接挂 operatorDecisions.transport，不在这组变量里。」
3. `/node-operations-records` 上方：删去「（NO 三册、TF 四册）」和「走写开关的逐口变量（票 operator-channel/08）」，改写「命令行挂各自上下文的作业事实渠道，读开关换不了」。
4. `/transport-fulfillment/delivery-attempts` 上方：删去「在那之前生产形态如实答未配置，隔离形态经写开关逐口放行」，改写「挂操作者渠道的「作业事实登记」能力面（ADR-0149 决定二）」。
5. 控制事实一组：「登记与执行三行经写开关逐口放行（……），两个更正口仍挂字面量 UnconfiguredIntake{}」改为「登记、执行与更正各行都挂作业事实渠道（首登 ADR-0149 决定二；更正随原事实，ADR-0151 决定四），写开关换不了这几行（ADR-0150 决定三）」。「命令里带着段引用……来源不明的实际履约段」那段理由保留。揽收更正那句删去「，不进段——段侧重派生另立票」，并把文件名「assemble_offsite_pickup_correction.go」改为指向符号 `buildOffsitePickupCorrectionOrchestration`。（C8 草稿未含）
6. TF admin 写面：「关段与建派送任务两行的隔离写放行随换口同笔撤下（……）」改为「写开关换不了这几行（ADR-0150 决定三）」。
7. 派送发起：删去「真渠道未就位前如实答未配置；隔离形态经写开关放行这一口（票 operator-channel/08）」；保留「谁按拍调、拍频多大属调用方（实例半边）」和「只让一拍能被调用、不替调用方定拍频」；写明这一口挂作业事实渠道（ADR-0149；不在 ADR-0151 决定六改归运营决定的那几口里）。
8. 有效时间判断、承运商首次有效收寄判断两段：「（票 operator-channel/15，隔离写放行同笔撤下）」改为「（ADR-0151 决定一）」；「读开关换不了它」改为「读写两个隔离开关都换不了它」。
9. 运输履约登记写面各行（凭证、有效时间规则、总单）：「写准入不另立形，同挂字面量 UnconfiguredIntake{}」和「登记方身份没有可采信的渠道前 Intake 恒堵」改为「挂操作者渠道的「登记册配置写」能力面（ADR-0100 决定四；票 operator-channel/04）」。各行「为什么读开关不能换它」的理由句保留。（C8 草稿未含）
10. `assemble_work_fact_operator_test.go`：`operationDecisionFacesLeftOnIsolatedWrite` 改名为 `transportOperationDecisionFaces`，补上 `/transport-fulfillment-load-assignment-registrations` 与 `/transport-fulfillment-participation-terminations`。注释写：「运输履约归「运营决定」能力面的各口（ADR-0151 决定一）：作业事实渠道装不进它们，路由只装作业事实渠道时它们答未配置。」（C8 草稿用的是只列决定六那几口的另一个名字，并入时以本条为准。）
11. `transport_fulfillment_command_lines_test.go`：
    - Covers 首句改为：「票 operator-channel/15 完成判据「越过 Intake 之后：TF 各口答业务结果」——作业事实行走作业事实渠道（……），运营决定行走操作者渠道（……），越过 Intake 都答业务结果；写开关换不了这几行（ADR-0150 决定三）。」其后的描述照旧。
    - 删去「四口的隔离写放行已撤，这几行就是改写后的真渠道答复格用例」。
    - `transportCommandLine` 头注：「运营决定四口」改为「运营决定行」；「表里已没有经隔离写放行的行（……）」改为「写开关换不了表里任何一行（ADR-0150 决定三）」。
    （C8 草稿只改了其中一部分。）
12. `assemble_isolated_write.go`：删去 `isolatedWriteAdmission` 末尾那句。写开关放行哪些口，由 `isolatedWriteAdmittedCommandLines` 一处说明。

#### 不做

- 非运输履约各行（委托侧运营决定、计价、网络目录等）的同类行注：归「非运输履约段行注对账」票。
- 揽收更正装配缺段登记册：归「揽收更正装配缺段登记册」票。本票只删那句里过时的「另立票」。
- 不改任何实参，不改装配行为。

#### 完成判据

- [ ] `assembleBusinessEndpoints` 的主链变量段和运输履约各行的行注里，不再出现「写开关逐口放行」「经写开关放行」「同笔撤下」「随换口」「这段原是」「这句从前」「NO 三册」「TF 四册」「同挂字面量」「恒堵」「另立票」。
- [ ] 全仓不再有 `operationDecisionFacesLeftOnIsolatedWrite`；`TestWorkFactChannelLeavesOperationDecisionFacesUnconfigured` 跑运营决定全集仍 PASS。
- [ ] 真库表的 Covers 与表头里不再有「第二条」「第三、四条」「四口」「已撤」「改写后」。
- [ ] gofmt、`go build ./...`、`go vet ./...` 通过；带 DSN 跑 `go test -p 1 -count=1 ./cmd/parcel-api/` 全 ok，其中 `TestTransportFulfillmentCommandLinesAnswerBusinessOutcomesAgainstARealDatabase` 是 PASS 而不是 SKIP。
- [ ] 走 Go 那条路（parallel-sessions）。
- [ ] 票 15 Comments「钉 `60244fa6`」那条评审下追记处置：Standards (1)(2)(3) 的 cmd 部分指向本票；(1) 里 `OperatorDecisionIntake` 的头注已由 `7ec601ab` 修掉。票 22 Comments 追记 cmd 部分。票 26「评审结果」序 4、序 5 的去向格补上本票号。

### 三、序号引用横扫（簇 C1 派生草案）

#### 现象

Go 注释与测试头注，用序号去指别处的条目。常见两种：
- 「票 X 完成判据第N条」：完成判据多半是无编号列表，序号纯属自己数出来的；
- 「裁定第N条」：票面会改，中间插一条，后面的序号就全错。

在 main 8d22729f 上，这两种写法出现在本票地盘所列的文件里，operator-channel/29 要改的那几件除外。

更宽泛的「第N条」写法在 cmd/parcel-dispatch 等处也有。其中有些指的是同一个测试里列表的位置，比如「第一条事实」，不属于本类。

#### 要裁的问题

1. 本类怎么判别：
   - 算：指向另一个文件里某一条的序号引用；
   - 不算：同一函数内对列表位置的序数；
   - 视为符号、可以保留：ADR 自带的决定编号、UC 表格里自带的步骤标签。
2. 要不要立门禁，还是只横扫一次？门禁必须能按上一条区分开，否则会误报。

#### 做什么

- 按定好的判别标准，在当时的 origin/main 上重扫一遍，逐处改成引文。引原句，不引已被取代的裁定。

#### 不做

- 不碰operator-channel/29 地盘里的文件。

#### 完成判据

- [ ] 判别定义已写进票面；重扫出的清单与逐处改法，写进完成记录。
- [ ] go build ./...、go vet ./... 退 0，gofmt -l . 无输出；相关包的测试全绿。

## 完成判据（总）

- [ ] 上面三节各条逐条改完或注明「已不存在」；凡草案里改 endpoints.go 的条目，本票一律不做（归 35）。
- [ ] 改动文件 `git diff --check` 干净、无 CR / BOM；注释中文、只写为什么、不写变更说明、跨文件引用不用行号也不用计数。
- [ ] 改动包 `gofmt -l` 无输出，`go build ./...`、`go vet ./...` 退 0，改动包及反向依赖与 `./internal/architecture/...` 带 DSN `go test -p 1 -count=1` 全 ok。
- [ ] 非作者评审（隔离子代理两轴）0 阻断；推送方在干净检出上带 DSN 跑全量；机制清点有变则重生成。
