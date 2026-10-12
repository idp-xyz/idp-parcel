# 01 合同层归属有三处没人核：归属不明的客户合同对每一户参选、合同正文的账户引用不核在册、壳指名合同而正文挂产品的规则版本放行

Category: bug
Status: resolved——2026-10-12 13:3x 通道 2 快进进 main（远端 `b9a01048` → `bcf67053`，分支 SHA 原样进 main）；用户 13:2x 令「其他的都 crash 了，你自己独立完成吧」，评审是推送方自审，不是非作者评审——派给通道 3 的评审单 `task-6c3fbc70` 未答。见文末完成记录、Comments 与进 main 记录。此前 in-progress——2026-10-12 13:1x 通道 2 认领（用户授权自决），分支 `mcp2-ctb01` 基 `b9a01048`；分诊裁定见「分诊」，决定落为 [ADR-0178](../../../docs/adr/0178-customer-contract-versions-carry-their-body-and-contract-shells-bind-service-rule-bodies.md)。此前：needs-triage——2026-10-12 12:4x 通道 2 立，出自对票 customer-service-rule-contract-applicability/04、07 的事后非作者评审（见两票 Comments「评审 ← 通道 2」）；只读取证，未动代码。
Blocked by: 无
地盘：`internal/partycommercial` 的闭包选法、发布用例与客户服务规则的适用一致性核对；碰 Go，走[并行会话](../../../docs/agents/parallel-sessions.md)那条路。

## 现象（钉 main `bb118c4f`）

**一、归属不明的客户合同对每一户都参选。** `CommercialRegistry.admitsCustomerAccount` 对没有登记账户的客户合同版本答「参选」
（ADR-0177 决定二引 ADR-0153 的形状）。落得到这一格的有两种：经受控批文只发壳、没有正文的客户合同；迁移 0037 之前写下的正文行
（账户列为空，`LoadForScope` 不登）。后果两面：一户有自己的合同时与它同层冲突（07 判断项 2 已记）；一户**没有**自己的合同时
唯一解到它，挂在它上面的合同版客户服务规则、钉住它的规则包随之被这一户采纳——spec customer-service-rule-contract-applicability
用户故事 9 要排除的跨户串读。

**二、合同正文的账户引用不核在册。** 迁移 0037 头注说「账户在不在册由写入用例把门」，但这一列唯一的写入路径是发布用例，
`PublishCommercialAuthorityHandler` 只持发布登记册、时钟与交接，没有参与方身份读口；悬空的账户引用照常落库。后果是 fail-closed
的（这份合同对任何键都不参选，本该用它的那一户答`无适用依据`），不串户，只是与头注说的不符；迁移按字节校验，头注改不得。

**三、壳指名客户合同而正文挂服务产品的客户服务规则版本放行。** `ConsistentCustomerServiceRuleApplicability` 对「壳只指名了另一类」
放行，这一形态发布前与点读后都过得去。ADR-0176 之后「壳指名合同」就是合同层，它自相矛盾；只会被壳上那份合同的那一户采纳，
不串户，但 VE 的 `tiedToAdoptedContract` 见正文挂产品就放行，PC 选错层时没有第二道拦（04 作者自审已点到）。

## 为什么是缺口

PC `CONTEXT` 不变量「合同版本必须明确关联…货主客户账户」。ADR-0153 里没指名服务产品的成员确实对范围内各产品都适用；客户合同
在 ADR-0177 决定一之后账户必填，没登账户说的是「归属不明」，不是「对各户都适用」——把前者的形状搬给后者，等于让归属不明的合同
充当整个范围的合同。第三处同理：分层按壳定，壳与正文对「挂哪一类」说法不一时没有一处判它。

## 待裁（三处都是机制）

- 一：A. 闭包对没登账户的客户合同不参选（代价：只发壳的合同从此不能被任何一户采纳，测试夹具里只有壳的合同要补正文）；
  B. 发布面对客户合同要求正文在场（这一册的壳不再能单独发布）；C. 维持现状，在 ADR-0177 补记这一面的代价。A、B 改的是
  ADR-0177 决定二的形状，须先有 ADR 或补记。
- 二：发布用例为客户合同加一道账户在册的核（要接参与方身份读口），或在适配器与 ADR 里写明今天不核及其 fail-closed 后果。
- 三：在 `ConsistentCustomerServiceRuleApplicability` 补「壳指名客户合同时正文必须挂同一份合同」，发布前与点读后两处同时生效；
  补之前先查在册数据与夹具里有没有这一形态。

## 分诊（13:1x，通道 2，用户授权自决；钉 `b9a01048`）

三处都是机制，不涉租户取值。一与三改的是已接受 ADR 的形状或其下的核对，决定落为 [ADR-0178](../../../docs/adr/0178-customer-contract-versions-carry-their-body-and-contract-shells-bind-service-rule-bodies.md)（Accepted）。

- **一 → 选 B：发布用例对缺合同正文的客户合同版本答`未受理`。** 先在 `b9a01048` 上两种都试改、跑受影响包的真库测试量代价：A（选法让没登
  账户的合同对谁都不参选）红 102 个顶层用例，横跨 PC、PS、VE 与两个 cmd 包，只有壳的合同在夹具里随处可见；B 红的集中在发布用例与批文两个包。
  B 从写入边界上堵住无主合同的唯一来路（HTTP 载荷早已要求正文），A 多防的只有 0037 之前的旧行，那些行没有生产数据、按 ADR-0177 决定五重灌即消失。
  门排在声明构造之后、写入之前，答`未受理`而不是错误：恢复动作是改批文，答成错误会被批文工具读成技术失败。选法不改。
- **二 → 维持开放引用，不核在册。** 与 PC 发布正文里其余参与方引用同一纪律；悬空账户的后果是 fail-closed，不串户。要核就得给发布用例接身份
  读口、并让发布排在身份登记之后，另一件事。迁移头注那句由 ADR-0178 决定三更正，适配器写账户处加注。
- **三 → 收紧 `ConsistentCustomerServiceRuleApplicability`：壳指名客户合同时正文必须挂同一份合同。** 试改只红钉旧行为的那一条领域用例。反向
  （壳只指名产品而正文挂合同）不动：归产品层，由 VE 点核把关。

## 完成判据（分诊后细化）

- [x] 一：只有壳的客户合同版本发布答`未受理`、一行不写，有用例；其余通道的门仍先答各自的格；受影响夹具补正文、摘要按正文重算。
- [x] 二：ADR-0178 决定三写明不核及后果，适配器写账户处有注。
- [x] 三：壳指名合同而正文挂产品的版本被拒、挂同一份合同的放行、壳只指名产品而正文挂合同的仍放行，各有断言。
- [x] 门的判别力有取证（变异或先红）；受影响包带 DSN 全绿；推送方在合入 SHA 上全量一跑。

## 完成记录（2026-10-12，通道 2，分支 `mcp2-ctb01` 基 `b9a01048`）

**落点**：`f0488ec0` 认领与分诊；`251cc91c` [ADR-0178](../../../docs/adr/0178-customer-contract-versions-carry-their-body-and-contract-shells-bind-service-rule-bodies.md)
（Accepted）、ADR-0126 Status 区与 Links 前向指针、ADR 索引两行；`8e5b650e` 代码与测试；`bcf67053` 对账门头注补一句。

- `internal/partycommercial/application`：`PublishCommercialAuthorityHandler.Handle` 在 `declarationWrites` 之后、`SaveVersion` 之前拒缺合同正文的
  客户合同版本，答 `NOT_ACCEPTED`，成因归 `ErrInvalidCustomerContract` 一族；`publicationContentOf` 与 `reconcileDeclaredDigest` 头注随之说清。
- `internal/partycommercial/domain`：`ConsistentCustomerServiceRuleApplicability` 先核「壳指名客户合同则正文必须挂同一份合同」，再核产品那一格。
- postgres：`SaveCustomerContractContent` 写账户处注明开放引用、不核在册，迁移 0037 头注那句以 ADR-0178 为准；`LoadForScope` 装载账户处注明只有壳的
  合同只剩旧数据。
- 测试：两条「没有正文」子用例改断言 `未受理` 且一行不写；指名引用放行、委派发布、委派内容冲突、合同层交付条件四条夹具给客户合同补正文、摘要用
  `customerContractSpec` 按正文算，报告按通道顺序断言；`cmd/parcel-commercial` 三份批文夹具补 `contractContent` 并钉算出的 `PCC-1` 串（由对账门在
  `未受理` 里报出、再钉回）；领域一致性用例补两格。

**完成判据**

- ✅ 一：`TestACustomerContractBodyIsReconciledAgainstTheCanonicalDigest` / `TestACustomerContractDeliveryConditionsAreCoveredByTheContractDigest` 的
  「没有正文」两格答 `未受理` 且版本与声明都没写；挂错拥有者、重复委派、指名引用未发布、内容冲突各条用例照绿，说明这道门没有抢在它们前面。载体发布路
  经同一个用例；HTTP 载荷早已要求合同正文，预览不经此门。
- ✅ 二：ADR-0178 决定三；写账户处的注。
- ✅ 三：`TestCustomerServiceRuleApplicabilityIsCheckedAgainstTheShellReference` 三格。演示种子在 `bcf67053` 的分支树上对一只临时库 `seed.sh`
  零报错（两份合同正文、两版客户服务规则照常发布），临时库已删。
- ✅ 判别力：本票没有先跑 red，靠变异取证——`go test -overlay` 把门改成恒不触发，上面两格「没有正文」转红；把一致性核对恢复成「壳指名合同、正文挂
  产品」放行，领域那条用例转红；不带变异的对照两包都绿。
- ✅ 门：见进 main 记录。证据层级 S。

**未做 / 风险**

- 0037 之前写下、账户为空的旧行在选法里仍对各户参选，ADR-0178 明文接受（今天无生产数据，`seed.sh --reset` 即消失）；若日后有库在 ADR-0177 之前
  积了真数据，这一格要重新评估。
- 派给通道 3 的评审单 `task-6c3fbc70` 未答；那个会话重启后若照单评，评的是 `8e5b650e`，与进 main 的 `bcf67053` 只差一行注释。

## Comments

### 评审 ← 通道 2（推送方自审，不是非作者评审）· 钉 `bcf67053` · 13:3x

用户令独立完成，其余通道都已崩溃，没有第二个会话可评；自审看不到作者自己的盲点，宜由 PC owner 事后补一份非作者评审，重点是决定一那道门的位置与答法。

- **Standards**：阻断无。非阻断两条：（1）四条夹具的报告按下标断言（`reports[1]`），依赖 `declarationWrites` 的通道顺序——顺序是确定的，但日后调整
  通道顺序时它们会红在下标上而不是语义上；（2）门的成因文本是英文，与同文件其余成因一致。无发现：新注释全中文、写的是约束（排序、为何答`未受理`、
  迁移头注改不得）；无行号；跨文件引用用符号名与引文；ADR 里的实测数锚在 `b9a01048`；领域不引适配器；没有新立错误哨兵。
- **Spec**：阻断无。非阻断一条：旧行那一格（见「未做 / 风险」）。无发现：ADR-0178 三条决定逐条有落点与断言；完成判据四条逐条对过证据。

## 进 main 记录（通道 2 · 2026-10-12 13:35）

- 远端 main `b9a01048` → `bcf67053`：分支 `mcp2-ctb01` 自基未分叉，快进，`f0488ec0`、`251cc91c`、`8e5b650e`、`bcf67053` 原样进 main。13:35:01 推前
  `ls-remote` 答 `b9a01048`。
- 验证（推送方，钉 `bcf67053` 的分离工作树，WSL，go1.26.8，DSN 门禁库 55432）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；
  `go test -count=1 -p 1 ./...` 退 0，138 ok / 0 FAIL / 14 无测试 / 0 cached，155 s。`8e5b650e` 上同样一跑，157 s，同数。
- 清点：`8e5b650e` 的干净检出上重生成无差异；`bcf67053` 只改注释，不另生成。
- 分支 `mcp2-ctb01` 本地改名 `merged/mcp2-ctb01`、隔离树已拆；远端分支留着。占号随本笔释出。
