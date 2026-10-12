# 07 PC 闭包不按键上的客户账户收窄客户合同候选：同一范围两户各一份合同即两户都`适用冲突`

Category: bug
Status: resolved——2026-10-12 11:28 通道 1 快进进 main（远端 `9ba5d623` → `26c63e26`，分支 SHA 原样进 main）；评审是作者自审（用户令独立完成），见文末 Comments 与进 main 记录。完成判据二随 06 取证。此前：完工，待重放——2026-10-12 通道 1 在分支 `mcp1-csr07` 上做完，分支已推 origin，完成记录见文末。此前：in-progress——2026-10-12 11:3x 通道 1 认领（用户令「全部独立完成」），分支 `mcp1-csr07`，基 `9ba5d623`。此前：ready-for-agent——2026-10-12 11:2x 用户经 IDP 队列对「分诊」节推荐答「同意」，决定落为 [ADR-0177](../../../docs/adr/0177-customer-contract-names-its-customer-account-and-the-closure-narrows-by-it.md)（Accepted）；实施按其 Consequences 的落地位，走并行会话那条路。此前：needs-info——2026-10-12 11:1x 通道 1 分诊（见文末「分诊」）：是代码没兑现 PC `CONTEXT` 既有不变量的缺口，不是新产品问题；但兑现它要给客户合同的规范化文档加键、动 PCC-1，属难逆转取舍，须先有一篇 ADR。此前：needs-triage——2026-10-12 10:5x 通道 1 立（用户令独立完成），出自 [06](06-demo-contract-02-custom-first-claim-deadline-inherits-materials.md) 灌种子后的真库取证；只读取证，未动 PC 代码。
Blocked by: 无（用户决定已落 ADR-0177）
父票：[spec](../spec.md)
地盘：`internal/partycommercial/domain` 的闭包解析（客户合同那一格的候选选法）；碰 Go，走并行会话那条路。

## 现象（钉 main `8c41be83`）

把 06 那份种子（分支 `mcp1-csr06` 的 `2b76eab4`：第二客户 `SYN-PARTY-SHIPPER-02` / `SYN-ACCOUNT-02`、同范围 `SYN-SCOPE-01` 的第二份客户合同 `SYN-CONTRACT-02` 及其预付结算政策、账户 02 的解析键）用 `seed.sh` 灌进一只干净库，零报错。随后按两户各自登记的解析键行经 PS `CommercialResolutionKeys.FormResolutionKey` 成键、交 PC 真 `ResolveCommercialBasisHandler` 对同库发布登记册解闭包，两户都答 `APPLICABILITY_CONFLICT`，冲突依据含 `CUSTOMER_CONTRACT`。

对照：同一探针对 main 未改的种子（只有 `SYN-CONTRACT-01`）解账户 01，冲突依据只有 `SERVICE_PRODUCT`，没有 `CUSTOMER_CONTRACT`。（`SERVICE_PRODUCT` 那一格是探针成键时没带委托声明的服务产品，真提交会带，与本票无关。）

所以多出的那一格冲突是第二份合同带进来的：**范围里有两份生效的客户合同，闭包对任何一户都选不出唯一的一份**。探针是一次性的、未提交；两只库已删。

## 为什么是缺口

`ResolutionKey` 带 `CustomerAccountID`，注释说「少任何一维，都会让一个客户的解析回答另一个客户」；但客户合同那一格的候选选法今天不看这一维（在 `internal/partycommercial/domain` 里检索 `CustomerAccountID`，除键的定义、最小身份核与身份串之外没有一处读它）。一租户一范围只有一户时看不出来；第二户一进来，第一户就回退。

这也挡住了 ADR-0176 要的那件事本身：合同版客户服务规则按「壳恰指名闭包解出的客户合同」选，闭包连合同都解不出唯一，合同层无从谈起。

## 待裁

合同凭什么认它属于键上那一户。种子里现成的线索有三条，各有代价，本票不代裁：

- 合同正文的 `contractDelegations[].delegator`（`CUSTOMER_ACCOUNT` 类）——今天是资料修订委派，拿它兼作归属会让一份没有委派的合同找不到主。
- 参与方登记里客户关系 `relationships[].basis` 指名合同、`holder` 是货主参与方、客户账户 `customerPartyId` 指回同一参与方——三段间接，跨两册。
- 给合同壳加一条指名客户账户的引用（`references` 新键）——改规范化文档，PCC-1 要按「加 omitempty 键不换号」的先例另核（ADR-0176 决定五的同一纪律）。

## 完成判据（分诊后细化）

- [x] 同一范围两户各一份合同时，各户闭包解出自己那一份；只有一户时行为不变。
- [ ] 06 的种子灌库后两户闭包不在 `CUSTOMER_CONTRACT` 上冲突——属 06 续做那一步（分支 `mcp1-csr06` 的合同 02 要补账户、重算摘要再灌），本票进 main 后在 06 上取证。

## 完成记录（2026-10-12，通道 1，分支 `mcp1-csr07` 基 `9ba5d623`）

**落点**：`b6ac8471` 认领；`887bc0ad` ADR-0177 决定三补记对 ADR-0014 的部分停用（Status 前向指针与 README 一行）；`776ea847` PC 领域 / 应用 / postgres / 迁移 0037 / http 载荷 / `cmd/parcel-commercial` 批文翻译 / 演示种子合同 01；`31b73af7` 管理台客户合同表单；`457e7ebc` 清点（PC 测试 164 → 166、party_commercial 迁移 36 → 37）。

**完成判据**

- ✅ 两户各解各的：领域 `TestEachCustomerAccountResolvesItsOwnContractInASharedScope`；真库 `TestEachAccountResolvesItsOwnContractFromTheStoredScope`（两份合同正文经 `SaveCustomerContractContent` 落库、`LoadForScope` 读回的登记册上各户唯一解出自己那一份，第三户 `无适用依据`，同版本换户再登 `内容冲突`），`-v` 下 PASS。只有一户：`TestAContractWithoutARegisteredBodyStaysACandidate`（只有壳的合同照旧唯一解析）；既有闭包、cmd 各真库用例照绿（它们的合同多为只有壳）。
- 另钉：别户合同不参选（`无适用依据`）、同一户两份仍冲突、收窄只碰客户合同、登记口拒他类与空账户、构造门与规范化拒缺账户、载荷点名 `customerContract.contractContent.customerAccount`、旧行无账户点读报错（`TestALegacyContractRowWithoutAnAccountIsAnErrorNotAGuess`）。

**red 与判别力**：本票没有先跑 red——选法收窄先写进了 `applicable`，新用例是之后补的。判别力因此只靠变异取证（M1、M2 正是「没有收窄」与「收窄恒放行」两种实现前形态）。`go test -overlay` 临时变异八处：M1 选法不收窄、M2 账户比对恒真（首写编不过，改写后重跑）、M3 `LoadForScope` 不登账户、M5 正文 validate 不要账户、M7 旧行不报错、M8 构造门不要账户——各有用例红；M4 重放不比账户——真库用例红，表现为夹具事务里 120 s 超时而不是断言行；M6 规范化键加 omitempty——等价变异（validate 保证账户非空，omitempty 永不生效），没有用例能分辨，如实记下。

**门**（钉 `31b73af7` 的工作树，WSL，go1.26.8，DSN 门禁库 55432）：改动 `.go` `gofmt -l` 无输出；`go build ./...`、`go vet ./...` 退 0；PC 各包的反向依赖（`go list -test` 反查，21 个包）连同 `./internal/architecture/...` 带 DSN `-p 1 -count=1`：21 ok / 0 FAIL，54 s。admin-web 三道门（Node v22.23.3）：tsc 退 0、487 / 487 pass、vite build 退 0。全量留给推送方重放后那一跑。证据层级 S。

**判断项**

1. **账户只进正文与选法，不进壳的指名引用**（ADR-0177 决定五）：壳只收商业对象类别；登记册另存「合同版本 → 账户」一格，`LoadForScope` 左连正文装进去。
2. **正文未登记的合同照旧参选**（ADR-0177 决定二所引 ADR-0153 形状）：只有壳、没有正文的合同不知道属于谁，选法不把它折成「不属于任何人」；它的问题由正文点读口答。代价：同范围一份只有壳的合同与一户的合同并存时，那一户仍冲突——`TestAContractWithoutARegisteredBodyStaysACandidate` 钉住。
3. **`ViewRevision` 不另加一段**：账户是合同规范化正文的一格，变了合同的内容摘要就变，修订已随版本那一段跟着变；另加一段只会让同一件事算两次。
4. **迁移列可空**：本迁移之前写下的行没有账户可补、也不该猜；写入面恒写，读到空的旧行时点读报错、选法当它「正文未登记」。演示库 `seed.sh --reset` 重灌。
5. **对 ADR-0014 的部分停用补记进 ADR-0177 决定三**：实施时发现钉串用例写着「串变了就是换号」，出自 ADR-0014 Consequences 经 ADR-0126 继承；ADR-0177 起草时只点了 ADR-0176 决定五的 omitempty 先例，漏了这一条。补记照 README「部分停用」制度办（ADR-0014 Status 前向指针 + README 一行，适用场景限本册本次），不改 ADR-0014 正文。**越权风险点 · 待用户复核**——用户 11:2x「同意」的是分诊推荐里「PCC-1 摘要随之变、写进 ADR」那一句，没有逐字看到 ADR-0014 这一条。
6. **责任法人那一半不做**（ADR-0177 决定四），留作后续另记。

**未做 / 风险**：06 的种子（合同 02 补账户、重灌、两户闭包取证）属 06；管理台表单只加了一格输入，未走浏览器。

## Comments

### 评审 ← 通道 1（作者自审，不是非作者评审）· 钉 `26c63e26` · 11:2x

用户令「全部独立完成」，没有第二个会话可评；自审看不到作者自己的盲点，宜由 PC owner 事后补一份非作者评审，重点是判断项 2、5。

- **Standards**：阻断无。非阻断：判断项 5——ADR-0014 的部分停用是实施时补记的，用户的「同意」没有逐字覆盖它。无发现：新增注释全中文、无行号与跨文件计数；domain 不引 adapters；迁移只加列与 CHECK、不改既有约束；生产代码不写死任何租户取值（演示种子的 `SYN-ACCOUNT-01` 是 S 实例）。
- **Spec**：阻断无。非阻断：判断项 2 留下的那一格（只有壳的合同与一户合同并存仍冲突）是 ADR-0153 形状的已知代价。无发现：ADR-0177 决定一至五逐条对过落点；完成判据一两格有用例；判据二归 06。

## 进 main 记录（通道 1 · 2026-10-12 11:28）

- 远端 main `9ba5d623` → `26c63e26`：分支 `mcp1-csr07` 自基未分叉，快进，`b6ac8471`、`887bc0ad`、`776ea847`、`31b73af7`、`457e7ebc`、`d59f3a72`、`26c63e26` 原样进 main。11:28:54 推前 `ls-remote` 答 `9ba5d623`，其下只有本票七笔。spec 子票表 07 行随 `796e4610` 先同步，本记录随其后一笔。
- 验证（推送方，钉 `26c63e26` 的隔离 detached worktree，WSL，go1.26.8，DSN 门禁库 55432）：`gofmt -l .` 无输出；`go build ./...`、`go vet ./...` 退 0；清点重跑 `git status --porcelain` 为空；带 DSN 全仓 `go test -count=1 -p 1 ./...` 退 0，138 ok / 0 FAIL / 0 cached，150 s。
- 解锁：[06](06-demo-contract-02-custom-first-claim-deadline-inherits-materials.md) 的「Blocked by 07」已解，06 票面未改。
- 分支 `mcp1-csr07` 本地改名 `merged/mcp1-csr07`、作者树与验证树已拆；远端分支留着。

## 分诊（2026-10-12 11:1x，通道 1，钉 main `a0aae6a1`；只读）

**归类：bug——代码没兑现 [PC `CONTEXT`](../../../docs/domain/party-commercial/CONTEXT.md) 已写的不变量。** 三处原话：

- 不变量一节：「货主客户账户必须明确关联其客户参与方，**合同版本必须明确关联责任法人参与方和货主客户账户**。」
- 「客户合同」词条：「运营企业责任法人与**明确货主客户账户**在一个适用期间内接受的商务和服务条件。」
- 商业依据解析：「针对明确租户、**客户账户**、责任法人候选、服务范围和商业选择锚点，返回唯一适用商业版本集合」。

**代码事实。** 客户合同正文（`domain/customer_contract.go`）没有任何指向客户账户或责任法人的格；版本壳的 `references` 只收商业对象类别，客户账户是参与方身份、不在其列。通用选法 `CommercialRegistry.applicable` 只按租户、类别、范围、声明的服务产品、生效状态与选用区间筛，不读键上的 `CustomerAccountID`。所以「一个范围一份合同」是今天能工作的唯一形态，06 的取证就是第二份进来的那一刻。

**不是「范围即客户」的设计。** 先排除过这一读法：若范围就是客户维，ADR-0176 要的「同一范围并存产品版与合同版、合同版缺行继承同范围产品版」就只能一户一范围、每户把产品与规则包在自己范围里各登一遍——那正是 spec 「Problem Statement」要消掉的「把客户差异写成产品差异」的同族做法；`CONTEXT` 的不变量也明写合同关联客户账户，而不是关联范围。

**推荐（待 ADR）：客户合同正文显式指名货主客户账户，闭包按键上的客户账户收窄客户合同候选。**

- 落位：合同正文加一格客户账户（必填，与 `CONTEXT`「必须明确关联」同强度）；`applicable` 对客户合同那一类多一道「正文指名的账户 = 键上的账户」。结算政策、合同版客户服务规则都已跟着闭包解出的合同走，不必再动。
- 责任法人那一半不同时做：键上的「责任法人候选」是另一维、另一套选法，并进来会把本票变成两件事；ADR 里点名留作后续。
- 难逆转之处：规范化文档加键、PCC-1 摘要随之变。现在没有任何租户的生产数据，已登记的只有演示种子与测试夹具，新键可不走 omitempty、直接必填，演示种子按新摘要重灌即可；但这是对 ADR-0176 决定五同一条纪律（「若要改则按加 omitempty 键不换号的先例另核」）的一次显式取舍，必须写进 ADR，不在实现票里顺手做。
- 不选的两条：合同委派的 `delegator`——委派回答「谁能替谁决定」，一份没有委派的合同就找不到主，拿它兼作归属是一词两义；客户关系 `basis` + `holder` + 账户 `customerPartyId` 三段间接——跨参与方册与发布册两处权威，一户多账户时还要再选一次。

**决定权。** 这一笔动规范化与摘要，按 AGENTS.md「改难逆转技术或产品取舍 → 新 ADR」归 ADR；写不写、按不按推荐写，由用户定，通道 1 不代裁。定了之后本票转 ready-for-agent，实施落在 `internal/partycommercial` 的 domain + 发布批文翻译（`cmd/parcel-commercial`），走并行会话那条路。
