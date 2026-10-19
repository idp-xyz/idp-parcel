# 47 演示种子与 README 的注释和说明照实写：授予与对照做了什么、管理台实际有哪几页

Category: chore
Status: resolved——2026-10-18 进 main（与 40、47、49 同批，代码验证钉 `b3b246d7`），见「进 main 记录」。此前 in-progress——2026-10-18 完工；评审 0 阻断（会话读 diff 审，见 Comments），待进 main。此前 in-progress——2026-10-18 作者子代理完工，待非作者评审。此前 in-progress——2026-10-18 会话 `idp-parcel-9b` 立并认领（票 26 序 7、序 4 评审遗留；用户授权代决）；分支 `claude-oc47`
Blocked by: 无
父票：[26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)（票 26 评审遗留的后续）
出处：票 [24](./24-demo-configurator-holds-the-shipment-side-decision-grants.md) Comments 评审 Spec 非阻断 1–3、Standards 非阻断 1；票 [15](./15-operation-decision-faces-take-operator-intake.md) Comments「钉 `60244fa6`」那条评审 Spec 非阻断 1。
地盘：
- scripts/demo-seeds/seed.sh（只改注释）
- scripts/demo-seeds/README.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

对端会话会在 seed.sh 加一行调用（加完告知 SHA）；本票 rebase 到它之后，不碰那一行，也不动任何数据。

## 裁定

无需裁定（机械项）。改法照研判代理在 main `8d22729f` 上核过的现状与正确说法；动手前对着最新文件再核一遍。

## 做什么

### C7-1（仍在：True；要裁：False）
- 出处：票 operator-channel/15 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `60244fa6`」Spec 非阻断 1（README 半边）；同段 `data/governance/09`–`22` 是本簇核查时顺带并入的同类问题（按文件序号指区间，与计数同构）
- 位置：scripts/demo-seeds/README.md ·「操作者渠道：演示发行方与起 parcel-api」一节第 3 步，`IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY` 说明段（以「最后一件是本产品在治理登记册里的权威串」开头的那段）
- 现状：最后一件是本产品在治理登记册里的权威串（ADR-0180 决定三），运营决定与外部事实各口铸信封前拿它认区间：不设，这几口授予齐备也答 403 `OUTSIDE_ADMISSION_SCOPE`；设了，种子里演示租户的准入范围对照（`data/access/03`）与权威区间（`data/governance/09`–`22`）才认得上。取值与隔离形态的权威串同串，隔离写开关开着而两值不等进程启动即拒。管理台上的关段、建派送任务、有效时间判断与承运商首次有效收寄判断四口走的也是这一条（票 operator-channel/15 起不再经隔离写开关）：不设这一件，配置员登录后在这几页上答的是 `OUTSIDE_ADMISSION_SCOPE`。
- 改法：整段替换为（保持原段的三格缩进）：
   最后一件是本产品在治理登记册里的权威串（ADR-0180 决定三），运营决定与外部事实各口铸信封前拿它认区间：不设，
   这几口授予齐备也答 403 `OUTSIDE_ADMISSION_SCOPE`；设了，种子里演示租户的准入范围对照（`data/access/03`）与
   `data/governance/` 里能力为 `OPERATION_DECISION`、`EXTERNAL_FACT` 的权威区间才认得上。取值与隔离形态的权威串同串，
   隔离写开关开着而两值不等进程启动即拒。隔离写开关不放行任何运营决定口（放行名单见 `isolatedWriteAdmittedCommandLines`），
   运营决定各口都只能经这一件认区间。管理台上提交运营决定的页面，如「接受前人工复核」（`AcceptanceReviewPage`：复核完成、
   主动拒绝）、「授权处置」（`AuthorizedDispositionPage`）、「外部轨迹有效时间判断」（`EffectiveTimeJudgmentPage`），不设
   这一件时提交答的就是 `OUTSIDE_ADMISSION_SCOPE`；管理台上没有页面的运营决定口只能直接调 API。
- 备注：事实（main 8d22729f）：apps/admin-web/src 里调运营决定口的只有 `pages/shipment-request/api.ts` 的 `completeManualReview`、`rejectShipmentRequest`、`disposeShipmentRequest`（分别由 `AcceptanceReviewPage`、`AuthorizedDispositionPage` 调；导航标签「接受前人工复核」「授权处置」），以及 `pages/operations/effective-time-judgment-api.ts` 的 `judgeEffectiveTime`（由 `EffectiveTimeJudgmentPage` 调，标签「外部轨迹有效时间判断」）。按路径与路径片段全量 grep（segment-closure / dispatch-task / load-assignment / participation-termination / carrier-first / continued-attempt）无任何调用：关段、建派送任务、承运商首次有效收寄判断、装载分配、终止参与、受控关闭、受控重开在管理台都没有页面。原文点名的「四口」里只有有效时间判断有页；原文还漏了委托侧有页的那几口同样受这一件管。两条请求都带令牌：`shipment-request/api.ts` 的 `exchange` 与 `catalogue-api.ts` 的 `postMasterData` 都经 `withBearer`。权威串未设时，`buildAdmissionScope` 交回 `UnconfiguredAdmissionScope`；`OperatorMinter.MintOperator` 先核授予、再判准入（头注「准入范围判在授予之后」），PS 与 TF 的 `OperatorAuthenticator` 对每种决定都带 `AdmissionRequirement`。实测：现有 `TestSwappedDecisionFacesAnswerFromTheOperatorChannel`（`swappedDecisionFaces` 覆盖委托侧与 TF 全部运营决定口；`admissionScopeUnset()` 就是权威串未设时的生产路径）在 8d22729f 带 DSN 跑绿，「granted, admission not registered」那一格各口都答 403 OUTSIDE_ADMISSION_SCOPE。`isolatedWriteAdmittedCommandLines` 在 main 上只剩 `/shipment-requests`。governance 区间：逐份读 `data/governance/*.json` 的 capability 字段，能力为 OPERATION_DECISION/EXTERNAL_FACT 的恰好就是原文 `09`–`22` 那一批，所以新写法与原意等价。这个序号区间自 1777a0ec 起已随换口改过两回（09–16→09–20→09–22），下次给 CARRIER_TRACKING 之类的外部事实登区间，它又得有人记得去改，所以按能力指。管理台复核页与处置页只对队列里已有的委托提交，所以措辞写「提交答的」，而不写「页上答的」。

### C7-2（仍在：True；要裁：False）
- 出处：票 operator-channel/15 Comments「钉 `60244fa6`」评审 Spec 非阻断 1 末句「`seed.sh` 对照段的注释同病」；票 operator-channel/24 Comments「评审 ← 会话 idp-parcel-9b 起的隔离子代理 · 钉 `e8673ac1`」Standards 非阻断 1 后半（「配置员的十一种运营决定都在这里有对照」）
- 位置：scripts/demo-seeds/seed.sh ·「== 操作者册登记（access-identity：…）==」段，`admission-scope-mapping-register` 那一行上方的注释（以「# 准入范围对照（ADR-0180 决定一、二；票 operator-channel/19）」开头）
- 现状：# 准入范围对照（ADR-0180 决定一、二；票 operator-channel/19）：演示租户在上面那几种（能力、事实类型）上的生产写
# 落在哪个治理对象范围。对照住接入身份、与操作者授予同处，不进 data/governance/；它只换坐标，区间才答「谁在写
# 生产」。演示租户越过准入范围只有这一条普通登记的路（ADR-0150 决定一）。配置员的十一种运营决定都在这里有对照：
# 管理台上的各运营决定口经它走，终止参与另供 probe-operator-channel.sh 取证。
- 改法：这四行注释整段替换为：
# 准入范围对照（ADR-0180 决定一、二；票 operator-channel/19）：演示租户在上面那几种（能力、事实类型）上的生产写
# 落在哪个治理对象范围。对照住接入身份、与操作者授予同处，不进 data/governance/；它只换坐标，区间才答「谁在写
# 生产」。演示租户越过准入范围只有这一条普通登记的路（ADR-0150 决定一）。配置员持有哪一种运营决定，这里就要有那一种
# 的对照，少了那一行，那一口授予齐备也答 OUTSIDE_ADMISSION_SCOPE；运营决定各口不论由管理台的页面调还是直接调 API 都经
# 它走，终止参与另供 probe-operator-channel.sh 取证。
- 备注：60244fa6 时这句写的是「管理台上的关段、建派送任务与两个判断口经它走」，点名了不存在的页面。e8673ac1 改成「配置员的十一种运营决定都在这里有对照：管理台上的各运营决定口经它走」，病根还在，只是说得更含糊：读起来像每个运营决定口都有管理台页面，也像对照只对管理台页面起作用；再加上一个计数（数的是 `accessidentity` 的 `decisionKinds` 与 `data/access/02`、`03` 的行）。对照缺一行就答不在准入范围，实据在 `adapters/pilotgovernance` 的 `AdmissionScope.Admits`：按（租户、能力、事实类型）取对照，取不到 `return false, nil`；`MintOperator` 把它译成 `ErrOutsideAdmissionScope`。管理台有页的口与 API-only 的口见 C7-1 的 note。纯注释，演示租户的合成取值不动，证据仍只记 S。

### C7-3（仍在：True；要裁：False）
- 出处：票 operator-channel/24 Comments「评审 … · 钉 `e8673ac1`」Spec 非阻断 1（决定方说法只对受控关闭与重开成立）与 Standards 非阻断 1 前半（「另授全部十一种运营决定——TF 六种…与委托侧五种」）
- 位置：scripts/demo-seeds/seed.sh ·「== 操作者册登记（access-identity：合成操作者主体与能力面授予；票 operator-channel/01、17）==」段开头注释，即 `operator-register` 那一行上方，以「# 两个合成主体绑演示租户」开头、到「…不按配置推算。」为止的六行
- 现状：# 两个合成主体绑演示租户：配置员扮运营主管，授登记册配置写、主数据与运营查阅读两格，另授全部十一种运营决定——TF 六种
# （装载分配、终止参与、关段、建派送任务、有效时间判断、承运商首次有效收寄判断）与委托侧五种（复核完成、主动拒绝、
# 授权处置、受控关闭、受控重开；票 operator-channel/24）。授予只开门：决定方、授权角色与授权依据快照仍全由 party-commercial
# 的授权答复给出，委托侧各口越过 Intake 后如实停在各自的授权缺口上。查阅员只授查阅读，继续证「授予不覆盖即拒」。
# 证据层级只记 S，不登任何真实操作者。发行方是演示发行方 Dex（operator-channel/compose.yaml），主体是
# 它给 dex.yaml 里两个静态用户真签出的令牌里的 sub（operator-token.sh … claims 读出），不按配置推算。
- 改法：这六行整段替换为下面九行；其后「# 只登演示部署信任的那只发行方签得出的主体：…授予号与依据也另起。」三行不动：
# 合成主体绑演示租户：配置员扮运营主管，持有全部运营决定（票 operator-channel/24「裁定」）——授登记册配置写、主数据与
# 运营查阅读，运营决定按 accessidentity 的 decisionKinds 逐种各授一行（各种名目见 README 授予表）；decisionKinds 多一种，
# data/access/02 的授予、03 的准入范围对照与 data/governance/ 的权威区间各补一行。授予只开门，不替任何人形成决定；
# 决定方从哪来逐口照 ADR-0151 决定二：复核完成、主动拒绝、授权处置取信封里的提交操作者作复核人、决定人、处置人，他有
# 没有权、凭哪一份授权由编排问 party-commercial；受控关闭与重开的命令里没有提交操作者，决定方、授权角色与授权依据快照
# 都由授权答复给出。委托侧各口越过 Intake 的前提与停点见 README「操作者渠道」一节授予表后的说明。查阅员只授查阅读，
# 继续证「授予不覆盖即拒」。
# 证据层级只记 S，不登任何真实操作者。发行方是演示发行方 Dex（operator-channel/compose.yaml），主体是
# 它给 dex.yaml 里静态用户真签出的令牌里的 sub（operator-token.sh … claims 读出），不按配置推算。
- 备注：代码事实（main 8d22729f）：`shipmenthttp.OperatorDecisionIntake` 的 `IntakeManualReviewCompletion` 用 `domain.NewReviewerReference(operator.Operator)`，`IntakeActiveRejection` 用 `domain.NewDeciderReference(operator.Operator)`，`IntakeAuthorizedDisposition` 用 `domain.NewDisposerReference(operator.Operator)`，三者都取信封里的提交操作者。编排 `CompleteManualReviewHandler` / `RejectShipmentRequestHandler` / `DisposeShipmentRequestHandler` 只向 party-commercial 问准不准，准了就把答复里的授权引用写进去（各处 `Authority: authority.Authority`）。受控关闭与重开：`FormControlledClosureCommand` / `FormReopeningCommand` 没有提交操作者格，`FormContinuedAttemptDecisionHandler` 的 spec 取 `granted.Decider`、`granted.AuthorityRole`、`granted.Authority`（`ports.ContinuedAttemptDecisionAuthorization` 头注：三项一起由 party-commercial 给出）。ADR-0151 决定二原文分两条写，与此一致。实测：`internal/parcelshipment/adapters/http` 的 `TestManualReviewCompletionIsAddressedServerSideAndSignedByTheAuthenticatedOperator`、`TestActiveRejectionAndDispositionTakeTheDeciderFromTheAuthenticatedOperator` 在 8d22729f 跑绿。改法去掉了「十一种 / TF 六种 / 委托侧五种 / 两格 / 两个」这几处计数，改指 `decisionKinds` 这个符号；逐项名目单一放在 README 授予表，seed.sh 不再抄第二份。「越过 Intake 的前提」只在 README 写一处（见 C7-5），这里只放指针。票 24「裁定」节里有同一句不准的话，票面是历史记录，评审已记在 Comments，不改。三类判定（ADR-0146）：纯注释，配置员的授予是演示租户 SYN-TENANT-01 的合成取值，不变，没有写成任何默认值。

### C7-4（仍在：True；要裁：False）
- 出处：票 operator-channel/24 Comments「评审 … · 钉 `e8673ac1`」Spec 非阻断 2（「组成」表 `data/access/` 行与授予表不一致；自 `1777a0ec` 起过时）
- 位置：scripts/demo-seeds/README.md ·「组成」表 `data/access/` 那一行
- 现状：| `data/access/` | `cmd/parcel-access-register` | 操作者册（ADR-0100 决定二第三条）：两个合成操作者主体绑演示租户，配置员授登记册配置写与主数据与运营查阅读两格，查阅员只授查阅读；主体是演示发行方 Dex 给 `operator-channel/dex.yaml` 两个静态用户真签出的 `sub`，换发行方就按新发行方的标识另登一批 |
- 改法：整行替换为：
| `data/access/` | `cmd/parcel-access-register` 的 `operator-register` / `operator-grant` / `admission-scope-mapping-register` | 操作者册（ADR-0100 决定二第三条）：合成操作者主体绑演示租户，各自持有哪些授予以上面「操作者渠道」一节的授予表为准，这里不另列；主体是演示发行方 Dex 给 `operator-channel/dex.yaml` 里静态用户真签出的 `sub`，换发行方就按新发行方的标识另登一批。另有演示租户的准入范围对照（ADR-0180 决定一、二）：各（能力、事实类型）的生产写落在哪个治理对象范围，与 `data/governance/` 里本产品的权威区间配上，运营决定与外部事实各口才越得过准入范围 |
- 备注：这一行有两处过时。（1）「配置员授…两格」：1777a0ec（票 operator-channel/19）起，配置员已另有运营决定的授予（git show 1777a0ec 的 02-grants.json），60244fa6 与 e8673ac1 又各补了一批，这一行一直没跟上。（2）同一笔 1777a0ec 新增的 `data/access/03-admission-scope-mappings.json`（准入范围对照）这一行完全没提。seed.sh 用同一个 CLI 的 `admission-scope-mapping-register` 灌它，而本表别的行（如 `data/settlement/`）在入库通道列写到子命令。改法让授予只在授予表定义一处（「单一权威」），本行只放指针，同时补上对照，也去掉了「两个 / 两格」这类计数。

### C7-5（仍在：True；要裁：False）
- 出处：票 operator-channel/24 Comments「评审 … · 钉 `e8673ac1`」Spec 非阻断 3（授予表「委托侧五口越过 Intake 后停在各自的授权缺口上」缺前提：查不到委托或包裹仍答 403 `OPERATOR_NOT_GRANTED`）；同格「五口」也是 Standards 非阻断 1 那一类计数
- 位置：scripts/demo-seeds/README.md ·「操作者渠道：演示发行方与起 parcel-api」一节的授予表，`syn-operator-config-01@syn-tenant-01.example.invalid` 那一行的授予格；以及该表与下文「单要一枚令牌：」之间
- 现状：| `syn-operator-config-01@syn-tenant-01.example.invalid` | 登记册配置写、主数据与运营查阅读、运营决定（装载分配、终止参与、关段、建派送任务、有效时间判断、承运商首次有效收寄判断；复核完成、主动拒绝、授权处置、受控关闭、受控重开——委托侧五口越过 Intake 后停在各自的授权缺口上） |
- 改法：（一）授予格整行替换为：
| `syn-operator-config-01@syn-tenant-01.example.invalid` | 登记册配置写、主数据与运营查阅读、运营决定（装载分配、终止参与、关段、建派送任务、有效时间判断、承运商首次有效收寄判断；复核完成、主动拒绝、授权处置、受控关闭、受控重开——委托侧这几种越过 Intake 的前提见表后说明） |
（二）在授予表之后、「单要一枚令牌：」之前插入一段（前后各空一行）：
委托侧各种运营决定的授予只开门。权威串设了之后，越过 Intake 还要指名的对象在演示租户下查得到：复核完成、主动拒绝、
授权处置按委托标识查，受控关闭与重开按包裹查它当前`已接受`的委托（ADR-0183 决定一）；查不到与越权探针同答 403
`OPERATOR_NOT_GRANTED`（ADR-0151 决定二「委托寻址」）。本包只灌主数据、不造委托，所以只灌种子时打这几口看到的仍是
`OPERATOR_NOT_GRANTED`；`submit-one-shipment.sh` 落下一笔`已提交`委托之后，按委托标识寻址的口查得到它，受控关闭与
重开要等那一笔成了`已接受`才查得到。查得到之后各口停在各自的授权缺口上（ADR-0151 决定七、ADR-0183 决定六）。
- 备注：代码事实（main 8d22729f）：`OperatorDecisionIntake.locate` 走 `ports.OperatorDecisionTargets.FindOperatorDecisionTarget`，postgres 实现按（租户、委托标识）取单行，不限状态；`OperatorDecisionIntake.locateParcel` 走 `CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel`，SQL 带 `state = ShipmentRequestAccepted`。两处查不到都交回 `ErrOperatorNotGranted`，由 `writeIntakeProblem` 映射成 403 `OPERATOR_NOT_GRANTED`。本包不造委托：seed.sh 里没有任何写 `parcel_shipment.shipment_request` 的入口；`submit-one-shipment.sh` 头注写着「不进 seed.sh。种子灌的是主数据」；docs/design/synthetic-demo-journey-script.md 记过灌种子后「库内委托侧逐张 0 行」。实测（只读探针，不提交）：/tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide/C7/c7_probe_test.go 经 /tmp/claude-1000/-home-tops-workspace-idp-parcel/39d8f2f8-cde5-4dd1-bb34-aef8ce7c660c/scratchpad/decide/C7/overlay.json 挂进 cmd/parcel-api，带 DSN 跑 `go test -overlay=… -run TestC7Probe -count=1 -v ./cmd/parcel-api/`，两条都 PASS。装配用生产的 `buildAdmissionScope`（权威串 = `isolatedSelfAuthority`）、`registerOperationDecisionAdmission`（同种子登法）、postgres 寻址读口与 `build*Orchestration`，只有核验方与操作者册是替身。PROBE1：授予、对照、区间、权威串都齐，库里没有委托，委托侧那几口全部答 403 OPERATOR_NOT_GRANTED。PROBE2：库里有一份`已提交`委托（租户 SYN-TENANT-01），复核完成答 500 NO_ANSWER_FORMED，主动拒绝答 200 UNDECIDED / REJECTION_AUTHORITY_UNAVAILABLE，授权处置答 200 AUTHORITY_RULES_NOT_CONFIGURED（各停在授权缺口），受控关闭与受控重开仍答 403 OPERATOR_NOT_GRANTED（委托没到`已接受`）。现有 `TestClosureAndReopeningThroughTheOperatorChannelAgainstARealDatabase` 证明包裹查得到之后，生产装配上关闭答 200 UNDECIDED / AUTHORITY_UNAVAILABLE，8d22729f 跑绿。另据 docs/design/synthetic-demo-journey-script.md 2026-10-14 补记（票 psb/26），演示委托在 parcel-dispatch 推动下可以成`已接受`。停点只引 ADR-0151 决定七与 ADR-0183 决定六，不在 README 里抄具体答复码：授权映射一接上（psb/07），答复码就会变，抄进来就是第二套口径。seed.sh 那边只放指针（见 C7-3）。核到 origin/main 17bbb4a1（票 28 已进 main）：`locateParcel` 的 403 语义没变（票 28 只把纯载荷校验挪到寻址之前、证据判空改用同一判据），ADR-0183 决定一、六原文没动，本条照旧成立。

### C7-6（仍在：True；要裁：False）
- 出处：本簇核查顺带（非评审条目；与票 operator-channel/24 Standards 非阻断 1 同类的计数，外加 AGENTS.md「写代码注释」里「不写变更说明」那一条）
- 位置：scripts/demo-seeds/seed.sh ·「== 试点治理登记（pilot-governance：…）==」段里运营决定区间那一串：`17-authority-interval-operation-decision-segment-closure.json` 上方与 `21-authority-interval-operation-decision-controlled-closure.json` 上方各一行注释
- 现状：# TF 余下四种运营决定随换口补（票 operator-channel/15）：关段、建派送任务、有效时间判断、承运商首次有效收寄判断。
…
# 委托侧受控关闭与重开随换口补（票 operator-channel/15；ADR-0183）。
- 改法：删去这两行注释，下面的 `authority-interval` 调用不动。上方以「# 运营决定与外部事实各口的生产权威区间（ADR-0180 决定四；票 operator-channel/19）」开头的那段，已经说明了这一串里每一行区间的来由。
- 备注：票 15 已 resolved，运营决定的区间已全部登齐（逐份读 data/governance 的 capability/factKind：`accessidentity` 的 `decisionKinds` 每一种都有区间）。这两行里的「余下四种」「随换口补」说的是当初分批登的过程，属于变更说明，「余下」今天也读不通。不建议改成按族起名的小标题：前面 12、13 两份（装载分配、终止参与）同样是 TF 管理台上的决定，却排在委托侧那批中间，按族起名反而会分错组；直接删最干净。低优先级，可以并进 C7-2/C7-3 那一笔。

### C7-7（仍在：True；要裁：False）
- 出处：本簇核查顺带（非评审条目）：seed.sh 里已无声变错的总数
- 位置：scripts/demo-seeds/seed.sh · 各段 `echo "== N/7 …"` 进度行（从 `echo "== 0/7 编译登记 CLI 与迁移助手 =="` 到 `echo "== 7/7 代收与清分登记（…）=="`）
- 现状：echo "== 0/7 编译登记 CLI 与迁移助手 ==" … echo "== 7/7 代收与清分登记（collection-remittance：分户账、指令、事实与记账） =="，其后还有不带编号的「== 试点治理登记…」「== 操作者册登记…」「== 结算账户与受理前估价采用登记…」
- 改法：去掉带编号那几行 echo 里的「N/7 」前缀，只留段名，例如：`echo "== 编译登记 CLI 与迁移助手 =="`、`echo "== 施加迁移计划（${RESET_FLAG:-不重置}） =="`、…、`echo "== 代收与清分登记（collection-remittance：分户账、指令、事实与记账） =="`。后面那几段本来就不带编号，不动。
- 备注：在 8d22729f 上，「7/7」之后还有治理、操作者册、结算三段，跑的人看到 7/7 会以为已经灌完。这几段是后来逐票追加的，总数没人回头改，属于同一种「被数那一侧增减而数无声变错」。它数的是同一文件里的段，不属「改文档」那条管的跨文件计数，但已经实际错了。全仓 grep 过，没有任何脚本解析这几行输出（parcel.sh 的 seed 子命令只转调 seed.sh）。低优先级。

### C7-8（仍在：True；要裁：False）
- 出处：本簇核查顺带（非评审条目）：README 里已无声变错的计数
- 位置：scripts/demo-seeds/README.md · 标题「合成 S 主数据种子包（仅限隔离环境）」下的第一段
- 现状：票 `.scratch/master-data-wiring/issues/08-synthetic-seed-pack.md` 的产物：一套 `SYN-` 前缀的
合成主数据种子，经四个登记 CLI 灌入本机演示库，让主数据区七页有数据可看。
- 改法：整段替换为：
票 `.scratch/master-data-wiring/issues/08-synthetic-seed-pack.md` 的产物：一套 `SYN-` 前缀的
合成主数据种子，经各上下文的登记 CLI（逐个见下面「组成」表的「入库通道」一列）灌入本机演示库，让管理台主数据区的页面有数据可看。
- 备注：这句写于 8f2d9f5f（2026-08-25）。取证锚在 8d22729f（数本身就是论点）：seed.sh 的 `go build` 现编九个登记 CLI 加迁移助手，不是「四个」；`apps/admin-web/src/navigation.ts` 的「主数据」区已有十七个条目，不是「七页」。改法不再数，也不写「各页」：作业设备册在主数据区，而本包不灌设备，写「各页」就是过度断言。同文件「已知边界」里以「七页真数据展示还依赖查询端点的目录 Intake 配置（PAR-INT-01 未决期间装配 UnconfiguredIntake…）」开头的那条也过时了：票 operator-channel/05 已 resolved，目录查阅面逐口换了操作者 Intake；ADR-0100 决定一已把 PAR-INT-01 收回到客户渠道；`assembleBusinessEndpoints` 里还有一部分读口留在隔离读或未配置。这条要逐页核实才能给出准确说法，不在本簇的机械改动范围内，建议随下一次碰 README 的票按票 05 的完成记录逐页改写。本条不给替换稿。

## 不做

- 不改种子数据、授予、对照与区间。
- 不碰对端加的那一行调用。

## 完成判据

- [x] 上列各条逐条改完或注明「已不存在」；改后的说法与代码一致（引符号名为证）。
- [x] 改动文件 `git diff --check` 干净、无 CR / BOM；注释中文、只写为什么、不写变更说明、跨文件引用不用行号也不用计数。
- [x] `bash -n scripts/demo-seeds/seed.sh` 退 0。
- [ ] 非作者评审（隔离子代理两轴）0 阻断。

## 完成记录（2026-10-18，会话 idp-parcel-9b 的实施子代理，分支 claude-oc47）

**落点**（基于 `b58b10ef`；动手前对着该版文件与代码把「做什么」各条备注里的事实复核了一遍，均仍成立）：

- `scripts/demo-seeds/seed.sh`（只改注释与进度行文字）：`cbd40430`（C7-2、C7-3、C7-6）、`650b45a4`（C7-7）。
- `scripts/demo-seeds/README.md`：`1cbe5053`（C7-1、C7-5）、`07aba1ad`（C7-4、C7-8）。
- 替换稿逐字照各条「改法」。偏离一处：C7-7 去掉编号之后，代收段注释里「别指望本节像前六步那样幂等」没了锚点（网络目录段本来也不幂等），改成「别指望本节重放幂等」，随 `650b45a4` 提交。
- C7-8 备注里「已知边界」以「七页真数据展示还依赖查询端点的目录 Intake 配置」开头的那条，照票面不改。
- 对端会话要在 seed.sh 加的那一行调用：完工时 fetch 的 origin/main 仍是 `b58b10ef`，上面还没有这一行，所以本分支没有 rebase。本票没改任何调用行，进 main 时就算冲突，也只会落在相邻的注释上。

**完成判据逐格**：

1. 各条逐条改完、说法与代码一致：会话 scratchpad 里的核对脚本 `oc47/check.sh`（只读，不提交）对 C7-1…C7-8 逐格核「旧说法已去 / 新说法在」，「不做」三格核：对 `b58b10ef` 比，非注释、非进度行逐字不变；`data/` 未动。说法与代码一致的符号证据，在本分支带 DSN 跑绿：
   - `TestSwappedDecisionFacesAnswerFromTheOperatorChannel`：权威串未设时，授予齐备的各运营决定口答 403 `OUTSIDE_ADMISSION_SCOPE`。
   - `TestIsolatedWriteAdmissionSwitchesOnlyTheAdmittedCommandLines`、`TestIsolatedWriteAdmissionNamesTheAdmittedCommandLines`：`isolatedWriteAdmittedCommandLines` 里没有运营决定口。
   - `TestManualReviewCompletionIsAddressedServerSideAndSignedByTheAuthenticatedOperator`、`TestActiveRejectionAndDispositionTakeTheDeciderFromTheAuthenticatedOperator`：复核人、决定人、处置人都取信封里的提交操作者。
   - `TestClosureAndReopeningThroughTheOperatorChannelAgainstARealDatabase`：按包裹查到`已接受`委托之后，停在授权缺口上。

   管理台页面一格用 grep 复核：`apps/admin-web/src` 里调运营决定口的只有 `completeManualReview`、`rejectShipmentRequest`、`disposeShipmentRequest`、`judgeEffectiveTime`。`data/governance/` 里能力为 `OPERATION_DECISION` 的区间、`data/access/02` 的授予、`03` 的对照，都与 `accessidentity` 的 `decisionKinds` 逐种对得上。
2. `git diff --check b58b10ef` 干净；核对脚本的 CR、BOM 两格都过。新写的注释与说明只写为什么，不写变更说明；跨文件引用用符号名（`decisionKinds`、`isolatedWriteAdmittedCommandLines`、页面组件名）与文件路径，不用行号，也不用计数。
3. `bash -n scripts/demo-seeds/seed.sh` 退 0，核对脚本里也有这一格。
4. 非作者评审：待评审。

**先红后绿**：本票没有运行时行为，红绿落在核对脚本上。改前对 `b58b10ef` 跑：C7-1…C7-8 每一条都至少有一格 RED，脚本退 1；「不做」三格、`bash -n`、CR / BOM 都 ok。改后对 `07aba1ad` 跑：全部 ok，退 0。

**门**：

- 没有 `.go` 改动，gofmt 不适用。`go build ./...`、`go vet ./...` 都退 0。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1 ./internal/architecture/... ./scripts/demo-seeds/...` ok（后者没有测试文件）。本票没改 Go 包，所以不存在「改动包」或「直接引用改动包的包」；grep 过，Go 测试只读 `scripts/demo-seeds/data/` 下的文件，不读 seed.sh 与 README。
- 上面第 1 格列的取证用例逐个 `-run` 跑绿。
- 前端没改，没跑。
- 机制清点重生成，无差。
- seed.sh 没有实跑：只改了注释与 echo 文字，以 `bash -n` 为准；实跑要往演示库灌数，不在本票范围。

**证据层级**：S。纯文档与注释，演示租户 SYN-TENANT-01 的合成取值一行未动，没有写成任何默认值。

**评审**：待非作者评审。

**留给评审与后续**（票面没列，没改）：

- README 里还有两处在数 dex.yaml 的静态用户：「操作者渠道」第 2 步的「Dex 两个静态用户的主体」，以及「组成」表 `operator-channel/` 行的「两个合成静态用户」。和 C7-3、C7-4 去掉的是同一类问题。
- seed.sh 治理段总述的末句「只登今天已换口的那几种；余下的口换口时随那一笔补」，对运营决定已经不成立（`decisionKinds` 每一种都已登区间），对外部事实可能还成立，要等外部事实各口逐一核过才能改写。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `0ccc6d88`（基线 `b58b10ef`）· 2026-10-18**：用户 2026-10-18 选「精简续做」，评审由会话本身读 diff 做（隔离子代理那一轮随 workflow 停下未跑完）；不算非作者评审，如实记下。
  - **Spec**：阻断无。seed.sh 与 README 的改动只在注释、说明与进度行字样，数据与命令未动；授予「只开门」改成逐口照 ADR-0151 决定二写决定方从哪来，委托侧前提照 ADR-0183 决定一写清；管理台页面点名到 `AcceptanceReviewPage`、`AuthorizedDispositionPage`、`EffectiveTimeJudgmentPage`。
  - **Standards**：阻断无，非阻断无。「十一种 / 六种 / 五种」与「N/7」这类计数都已去掉，改指 `decisionKinds` 与目录；`bash -n scripts/demo-seeds/seed.sh` 退 0。

### 进 main 记录（2026-10-18，会话 idp-parcel-9b）

- 原分支 `claude-oc47` 各笔重放到 40 之后（新 SHA `b0759c43`…`e76e2e68`；原 SHA 作封存出处）。同批三票 40、47、49 一次推送。
- 推送方全量一次：钉 `b3b246d7` 的隔离 detached 检出，带 DSN `go test -p 1 -count=1 -v ./...` 退 0——有测试的包 142 个 ok、0 FAIL；`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）。同一检出上改动 `.go` 的 `gofmt -l` 无输出，`go build ./...`、`go vet ./...` 退 0，机制清点重生成零差。
- 评审见 Comments。
