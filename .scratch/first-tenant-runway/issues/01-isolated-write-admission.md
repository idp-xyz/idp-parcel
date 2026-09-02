# 隔离形态写面放行——把 ADR-0078 的形状从读面扩到写面

Category: enhancement
Status: resolved（2026-09-02，MCP-5；ADR 为 [ADR-0091](../../../docs/adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)，实现覆盖 `/shipment-requests` 一口。其余命令面逐口替换另立，见文末 Comments 末条）

演示动线三堵墙的**墙一**。取证基线 `c9835bf`。

## 墙

[演示动线脚本](../../../docs/design/synthetic-demo-journey-script.md)第 5 步：`POST /shipment-requests` 与 `POST /shipment-requests/parcel-cancellations` 答 `403` + `ACCESS_CHANNEL_NOT_CONFIGURED`。成因是各上下文的写端点一律装 `UnconfiguredIntake{}`（`cmd/parcel-api` 的 `assembleBusinessEndpoints`），而 [ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 的隔离放行面只枚举运营查阅那几行，**把写端点显式排除在外**。

脚本给的重启条件是「`PAR-INT-01` 最低证据到位」——那是**生产**放行的条件，属实例半边。本票问的是另一件事：**隔离演示环境要不要一条同样苛刻的写面放行。**

## 做什么

**「一份新 ADR」那半已经完成**，见文末 Comments；本节其余部分描述的是仍要做的装配改动，论证段保留供追溯。

一份新 ADR，外加按它落地的装配改动。

ADR 要裁的是一句话：**隔离形态下按装配注入放行写端点，是否与 ADR-0055/0072 的裁定相容。**我的判断是相容，理由写在这里供裁决时驳：那两份 ADR 否决的是**运行时渠道登记表**（登记册形状等真实渠道证据、不预先替租户拟），而隔离放行根本不是登记表——它是装配期注入，生产装配里那条路径压根不存在。两者管的不是同一件事。

**开工第一步必须先把 ADR-0078 的 Decision 逐条读完。**如果它当初把「写面排除」写成了带理由的正面裁定（而不只是划定了本次范围），本票的论证要重做，不能靠「它没说不行」推进。

形状直接照抄 ADR-0078，一处不改：

- 装配期注入，不读任何运行时登记表；
- 启动必出声（同 ADR-0078 那行 `INFO`，放行的是写面这件事要在事后可查）；
- `SYN-` 前缀守卫，非 `SYN-` 前缀**进程启动即拒**、带原因退出、不静默回落；
- 生产装配里不存在通往它的代码路径。

## 必须守住的一格

**放行的是「有没有渠道」这道门，不是它后面的任何一道。** 过了 Intake 之后，提交编排仍要走完真实的来源保全、授权、生产归属、接受判断——墙二、墙三照旧拦着。本票若做完发现委托能一路建成，那说明放行放过头了，要回头查。

## 完成判据

ADR 落文并被接受；`cmd/parcel-api` 装配点按它改；三态对照（不设变量 / `SYN-TENANT-01` / 非 `SYN-` 前缀）在写面逐条复现并记进演示动线脚本；`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

演示动线脚本第 5 步与「对照组」两节同笔更新——本票改了那两节描述的行为，不留「行为已变、脚本仍说 403」的中间态。

## 参照

ADR-0078（隔离环境运营读按装配注入放行）、[ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0072](../../../docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)；`cmd/parcel-api` 的 `assembleBusinessEndpoints` 与 `unwired_orchestration.go`；`.scratch/syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md`。

## Comments

- 2026-09-02 MCP-5（做票 02 时连带解掉本票的 ADR 阻断）：**本票所等的 ADR 已落**，是 [ADR-0091](../../../docs/adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)，用户裁定它同时裁写面 Intake 与生产归属目录两格。

  **本票「开工第一步」那条要求已执行**：ADR-0078 Decision 逐条读过，Decision 四确是带理由的正面裁定（「先例钉窄：环境切换只此一处、只此一维」），不是只划范围。因此本票原论证不足以推进——ADR-0091 没有绕开它，是把适用面从枚举改成三条入格判据并显式停用那一句。同时补上本票原论证缺的一环：**写面有持久化，ADR-0078 Decision 二的「零持久化」论证在这一侧不成立**，可分辨物改由 `SYN-` 前缀承担。本票若照原论证做，会把一个已经不成立的理由抄进实现。

  **已经做掉的部分**：`IDP_PARCEL_ISOLATED_WRITE_TENANT` 开关本身、`SYN-` 前缀门禁、与读开关的取值一致性校验、启动日志、装配点的隔离入参——全在 `cmd/parcel-api/assemble_isolated_write.go`，随票 02 交付并有三态测试。**本票余下的只有一件**：在 `assembleBusinessEndpoints` 里把命令行的字面量 `UnconfiguredIntake{}` 换成隔离写面 Intake，并按 ADR-0078 Decision 二的同款纪律在各上下文自立注入式命令 Intake 类型。开关已经在，不必再造第二个。

  **本票「必须守住的一格」现在有对照可用**：墙二已降（票 02），所以本票做完之后委托**会**一路建成到`已提交`——票面原句「本票若做完发现委托能一路建成，那说明放行放过头了」写于墙二仍在时，按 ADR-0091 分批落地的顺序，这句判据的前提已经不成立，接手时按墙三（初始路由与可达性）是否仍拦着来判，不按这一句。

- 2026-09-02 MCP-5（接上条同轮完成实现）：**`/shipment-requests` 一口已放行，本票收口。**

  **实现比票面预想的大一档，原因写在这里。** 票面把余下的活写成「把装配点那一行的字面量换掉」，但换上去的东西得先存在，而提交命令的入参不是一个作用域就能凑齐的——它要来源信封、载荷摘要与期望规则修订三样。读面的注入式 Intake 只需前者的类比物，所以那套形状照抄不过来。三样各自的落法与依据：

  - **来源信封**整组注入。报文里连租户的位置都没有（`DisallowUnknownFields`），塞一个进去解码即拒。唯一从请求内容派生的是来源请求键，取客户委托参考——这**不是**对 `PAR-INT-01`「来源请求键由哪个渠道字段铸成」那一格的回答，`internal/accessidentity` 的 `RequestKeyDerivation` 仍是零生产实现，本次碰都没碰。
  - **载荷摘要**走领域侧 `CanonicalizeSubmissionPayload`（ADR-0014）。摘要只吃客户声明的引用，不吃本包现签的内部标识——否则同一份内容重发两次会得到两个摘要，重放被判成`接入冲突`。有用例钉住这一格，因为它坏掉时没有别的东西会红。
  - **期望规则修订**向归属权威预取，依据是 `UC-PS-001` 步骤 3B 把「准入范围与期望规则修订」判给试点准入控制装配。这一格是本轮唯一改变了门禁语义的地方：隔离形态下门禁只拦得住预取与提交之间登记册发生的变化。已写进 ADR-0091 Consequences。

  **内部标识现签而不从客户参考派生**：`SubmissionIdentityFactory` 的端口注释把这条写死了（「客户参考号不得变成内部标识」）。代价是重发两次会签出两组新标识，但那两组都用不上——重放在来源身份上就被认出。

  **只换了一口。** 撤回、取消、复核完成、主动拒绝与其余上下文的命令面仍挂不经任何变量的字面量 `UnconfiguredIntake{}`，有装配用例逐口钉住。`endpoints.go` 里那句「命令面的字面量换不了」已按实况改写，没留「行为已变、注释仍那么说」的中间态。

  **读开关换不了写行**有专门用例（`TestIsolatedReadAdmissionCannotOpenTheSubmissionLine`）：只给隔离读入参时提交口仍答 403。两个开关分设的全部意义就在这一格。

  **演示动线脚本第 5 步与「对照组」两节同笔更新**（本票完成判据要求）：第 5 步改成「委托侧两种跑法」两态表，对照组表从三行扩到五行，多出的两行是「读开关开到底也开不了写行」与「两开关取不同 SYN- 租户即启动即拒」。

  **生产接线棘轮少一条**：`CanonicalizeSubmissionPayload` 自此有非测试调用点，从 `production_wiring_baseline.txt` 出名单。该处原注释预言它与版本报出口「一起出名单」——只出了一条，注释已改写为实况。这一格是门禁自己拦下来的，不是我事先想到的。

  验证：`gofmt -l` 空、`go vet ./...` 退 0、`go test -p 1 -count=1 ./...` **含真库** 93 包 `ok`、0 `FAIL`、退 0（同刻探针 `TestFreezeScopesAreInvisibleToEachOther` 得 `PASS` 非 `SKIP`）。

  **未做，另立**：其余命令面逐口替换。它不属本票范围（本票的墙是「委托侧没有入库通道」，提交口一开这堵墙就有门了），且逐口换的纪律要求一次一口、每口各自把「未配置即拒」测试改写成放行测试。真要做时按 ADR-0091 的逐口条办。
