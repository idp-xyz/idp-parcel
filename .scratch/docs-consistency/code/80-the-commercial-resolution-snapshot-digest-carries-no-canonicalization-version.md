# 80 商业解析快照的内容摘要不带规范化版本，形状一变，新旧构建彼此答内容冲突或静默回到现选

Category: bug
Status: in-progress——隔离评审无阻断，评审意见已收，待进 main，分支 claude-code80。原分诊：needs-triage——R2（上线后尽快）：首个租户实例进生产之前改形有规则可依（ADR-0267 决定三，已存摘要只在演示库、重灌即消）；进生产之后快照再改一次形就没有规则可依——滚动发布或回滚时新旧构建并存，旧构建对新构建写下的快照重放 Save 答内容冲突、挡住接受，旧构建读新快照会把不认得的键丢掉，例如 code/61 的底座一格丢掉之后静默回到按锚点现选。今天演示租户不撞，绕法：上线前不再改快照形状。2026-10-21 协调方据票 code/61 隔离评审 N1 立（用户「你作为业务和系统专家自决」授权）。
Blocked by: 无
上下文：PC（参与方与商业），商业解析库。
出处：票 [code/61](./61-a-contract-tier-service-rule-reads-its-product-base-at-claim-time.md) 隔离评审 N1。

## 现象（读码，只记 `S`）

- `internal/partycommercial/adapters/postgres/commercial_resolution.go` 的 `CommercialResolutions.Save`：内容摘要是对 `json.Marshal(documentOfClosure(closure))` 整份字节的 sha256，写进 `content_digest` 列；撞键后只比这一列，同摘要答重放、异摘要答 `ResolutionContentConflict`。摘要不带产生它的规范化版本，快照文档本身也没有版本键。
- `closureDocument` 与 `adoptedDocument` 历次加键都是可缺键加 omitempty（结算选择器、信用选择器、声明的服务产品、服务形态、结算政策、信用依据，code/61 又加了 `productBase`），旧文档逐字节不变，带新键的文档摘要改变。读回走 `json.Unmarshal`，不认得的键静默丢掉。
- 后果分两头：
  - 旧构建重解同一键、同一视图并 Save，它算出的是不带新键的字节，与新构建已写下的行摘要不同，答内容冲突，委托接受停住。
  - 旧构建 `LoadResolution` 读到新构建写下的快照，丢掉新键：code/61 之后丢的是底座一格，读回成「未固定」，层次读口回到按锚点现选（ADR-0268 决定三那一路），已接受委托继承来的那一行又会随底座事后改口。

## 规则

- ADR-0014 Decision：「每个已保存的内容摘要必须携带产生它的规范化版本；不携带版本的摘要视为不完整。」以及摘要不一致「只有在规范化版本相同时才构成版本内容冲突」。
- ADR-0267 决定三：同号改形只在写明前提时成立；进生产之后只认「那一面不存摘要、两侧从本体现算」那一种。快照存摘要，进生产之后不在其内。
- code/61 代决 4 依的是 ADR-0267 决定三的实质：快照未接规范化版本，决定一不触发；前提成立（上线前，已存摘要只在演示库）。那一次不需要本票；本票管的是进生产之后的下一次。

## 三类

机制。产品策略：无。租户取值：无。

## 规模

M：快照文档加版本键，摘要随版本写下；撞键比对先比版本再比摘要；读回按版本分派、未知版本响亮失败，不静默丢键；补新旧两版并存的用例。

## 最小的诚实修法

1. 快照文档加版本键（例如 `snapshotShape`），今天的形定为第一版；`content_digest` 旁记版本，或把版本并进摘要串的前缀，照 ADR-0014 的做法。
2. `Save` 撞键：版本相同才按摘要判重放与冲突；版本不同不是冲突，按 ADR-0014 Decision 第二条处置（由领票时定：答已固定，还是按原版本重新规范化再比）。
3. `LoadResolution`：按版本键分派解码；遇到本构建不认得的版本响亮失败，不按旧形读、不丢键。滚动发布时旧构建读到新版快照停下报错，好过静默回到现选。
4. 版本键缺席的快照是本票之前的形，按第一版读（与 ADR-0268 决定三「没有底座一格的旧快照」并行不悖：那一格是第一版之内的可缺键）。
5. ADR-0014 的索引注记本票不动；是否需要另立记录说明「解析快照这一面接规范化版本」，领票时判。

## 相关

- ADR-0014、ADR-0267、ADR-0268。
- 票 code/61：给快照加底座一格那一次同号改形的前提与代决。

## 完成记录（2026-10-21，分支 claude-code80，基于 main `32dc1d98d`）

### 做了什么

- `09286d524` 快照接规范化版本 `PCR-1`（`internal/partycommercial/adapters/postgres/commercial_resolution.go`）：
  - 快照文档加 `canonicalization` 一键，`content_digest` 写 `PCR-1:<hex>`，摘要盖住含版本键的整份字节。落法与商业发布 `PCC-1:<hex>`、PS `PSC-1:<sha256>` 同：版本随串走，不加列，无迁移。
  - `Save` 撞键：按那一行摘要串的版本重写本次闭包再比（`renderedAs`）。同版本照摘要判重放与冲突；无版本那一形（接版本之前写下的行）按它自己的字节重算再比；本构建不认得的版本报 `domain.ErrCanonicalizationUnsupported`，不答冲突，也不答已记录。
  - `LoadResolution`：读 `snapshot` 与 `content_digest`，两处版本须一致；`PCR-1` 拒未知键；无版本那一形照旧宽读、按第一版的结构读；未知版本报 `ErrCanonicalizationUnsupported`。
  - 用例（真库，新文件 `commercial_resolution_canonicalization_test.go`）：新快照带版本；同版本重放答已记录、异摘要答冲突；无版本行同内容答已记录、异内容答冲突，钉 main `32dc1d98d` 实算的两个无前缀摘要（一份带底座一格）；无版本快照照读底座；未知版本 Save 与 Load 都报 `ErrCanonicalizationUnsupported`；第一版里顶层或已采用项多一个未知键读回报错；文档版本与摘要前缀两说时读回报错。既有 `TestAResolutionFixedWithTheServiceRuleDeclaredFirstReplaysAsRecorded` 把改动前那一行改成无版本行（钉的仍是基 1d67e27c 实算的串），先让它自己重放答已记录，再让重解的那份重放。
  - 变异核对：删拒未知键、无版本那一形不去版本键、删版本两说的比对，各有对应用例红，复原后绿。
- `1e6400978` 端口 `CommercialResolutionStore.Save` 头注补：已固定那份的版本本构建不认得时，重放与冲突都答不出，是 error。
- `e435d686b` 撞键遇未知版本那段注释改写理由。

PC CONTEXT、UC-PC-002 与 ADR 正文未改；AT 盘点未动（无对应行要移或加）。没有迁移。

### 代决

用户授权「你作为业务和系统专家自决……解决所有的问题」。

1. **版本名与落法**：`PCR-1`，前缀串加文档键，不加列。与 `PCC`、`PSC`、`SAC` 同一落法；单开一列会让没版本的旧串与有版本的新串在库上长成同一种东西（商业发布头注的理由）。仓内没有别的摘要前缀叫 `PCR-`（设计文档里的工作包编号 `PCR-W01` 等不是摘要前缀）。
2. **`PCR-1` 就是接版本那一刻的整形，连同历次同形加入的可缺键（含 code/61 的底座一格），不拆成「无底座 v1 / 有底座 v2」**。拆开等于回头给 code/61 那一次同号改形补换号；ADR-0267 决定四否决补换号的理由同样适用于此：前提成立（上线前），多一个要长期支持的版本也换不来任何可比性。底座一格在 `PCR-1` 里照旧可缺，缺席照 ADR-0268 决定三读。
3. **旧快照（无版本键）的读法：认作「无版本那一形」**——结构与 `PCR-1` 相同、读法相同；撞键时把本次闭包按那一形重写（`PCR-1` 去掉版本键）再比，存量不回写。重写出的是 main `32dc1d98d` 那一版构建写下的字节（钉住的两个无前缀摘要即此）；更早形状的存量行（例如 code/61 之前、没有底座一格的合同版闭包）与在 main 上的情况相同，重放答内容冲突，由 code/61 要求的重灌消掉。理由：ADR-0014 Decision「既有评价永远按其原规范化版本重放」；仓内 `SAC`、`CCC`、`TFC` 接版本时同样把无版本那一形留成可算的一版（`unversioned_payload_digest.go` 各处）；代价只是一个空键分支。不选「无版本即不完整、读回报错」：那样演示库运行期写下的行就都读不回、非重灌不可，生产上却得不到任何保护（生产没有无版本行）。
4. **撞键遇本构建不认得的版本：报 error，不答已记录**。票面给的两条路，认得的版本走「按原版本重新规范化再比」；不认得的版本无从重算，答已记录就是冒认，答冲突就是把「比不了」说成内容不同。Save 那一路的 error 经 PC 解析用例与 PS 适配器原样上抛，接受停在技术错误、可重试，不落`无法判定`。读回那一路不同：旧构建读到新版本快照、或在 `PCR-1` 里读到不认得的键，`ValidateCommercialBasisHandler.loadPrior` 与 `FormJudgmentAsOfHandler.loadPrior` 把这个 error 吞掉、不记日志，分别落成`解析未决`·`权威不可读`与 `JudgmentAsOfPending`；PS 侧随之是`无法判定`或未决，经 `ErrAcceptanceChainUndecided` 回滚后重投。这样归类合规——ADR-0029 把读不回归在重试那一格；那两处要不要记提供方日志另立 [code/85](./85-a-prior-resolution-that-cannot-be-read-back-is-swallowed-without-a-provider-log.md)。哨兵复用 `domain.ErrCanonicalizationUnsupported`（「前者要换构建」正是这一格），不另立。
5. **`PCR-1` 拒未知键，无版本那一形不拒**。前者让「形状变了却没换号」在旧构建上响亮失败；后者是因为接版本之前历次改过形，存量里可能留着已退役的键（例如结算选择器的合同维，`c6c057fc9` 删掉的），拒了只会让一份当时合法的快照读不回。ADR-0267 候选与反方把拒未知键列为可并存的另一层加固，与此不抵。
6. **文档版本键与摘要前缀两说时读回报错**：两处各说一版的行不是本适配器写下的。
7. **演示库**：本票不要求重灌。种子不形成闭包（一次性库 `code80seed` 上 `seed.sh --reset` 后 `party_commercial.commercial_resolution` 为 0 行）；运行期写下的无版本行照读、照比。code/61 要求的重灌照旧有效。
8. **不立新记录，不改 ADR-0014 注记**：本票是照 ADR-0014 Decision 接版本，不停用任何一句；也不是 ADR-0267 所说的同号改形——这一面此前没接版本，现在接上。PC CONTEXT 不改：「规范化版本」词条只说商业版本的内容摘要；ADR-0267 Consequences「规范化版本是摘要的技术合同，不进领域语言」。

### 越权风险点

1. **无版本那一形要长期算得出**（PC owner）。`renderedAs` 的空串分支与宽读路要一直留着；若 owner 判生产上线时库里不会有无版本行（今天成立：生产没有存量），可另立票退役，前提是首个租户实例进生产之前所有环境都已重灌。
2. **未知版本答 error，不是业务格**（PC、PS owner）。滚动发布或回滚期间，旧构建对新构建已固定的解析重放（Save）会报错、接受停在技术错误，运维看到的是错误日志，不是`无法判定`。旧构建读回新快照（第二、三阶段的 `loadPrior`）则不报错：error 被吞成`解析未决`·`权威不可读`或 `JudgmentAsOfPending`，PS 侧是`无法判定`或未决、经 `ErrAcceptanceChainUndecided` 回滚重投，今天不留任何提供方日志（code/85）。两路都比答内容冲突挡接受、或读回丢键好。
3. **`PCR-1` 拒未知键，同号改形在这一面也要所有在跑的构建先认得新键**（PC owner）。上线前按 ADR-0267 决定三同号改形仍合规，但若有一个旧构建还在跑，它读到新键会报错而不是丢键——这是本意，代价是演示环境升级时要整批换构建。
4. **接版本这一次本身依 ADR-0267 决定三的前提**（PC owner）。本票之前的构建读 `PCR-1` 快照会把 `canonicalization` 当未知键丢掉，Save 会因无前缀串对不上带前缀串而答内容冲突；这些构建不会进生产（首个租户实例进生产之前对全产品一次结束），这一格只到得了演示环境。
5. **不加 CHECK 约束摘要前缀**（PC owner）。库上无版本旧串与 `PCR-1` 新串并存，约束只能写成「无冒号或 `PCR-` 起头」，挡不住什么，也会挡住未来的版本；没加。

### 给 ADR-0267 决定四的建议（不改已接受记录正文）

解析快照在 `PCR-1` 之前没接规范化版本，历次加可缺键（`98ce0eead` 服务形态、`5911d3b63` 结算选择器与政策、`f2b043f6d` 信用、`f9d81b275` 比例基数、`010cb1af6` 声明的服务产品、`90f6124d1` 底座）不属决定一所说「已接规范化的册（面）」，不在其表内。若 owner 要一份完整账，可在下次修订 ADR-0267 或另立记录时补一句「PC 解析快照自票 code/80 起接 `PCR-1`，此前历次加键是在未接版本的一面上，前提同决定三上线前阶段」。

### 跑过的测试

- `go build ./...`、`go vet ./...`、`gofmt -l`（动过的文件）：干净。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./internal/partycommercial/... ./internal/visibilityexception/... ./internal/parcelshipment/... ./cmd/parcel-api ./cmd/parcel-dispatch ./cmd/parcel-commercial ./cmd/parcel-settlement-register ./internal/settlementaccounting/adapters/partycommercial ./internal/transportfulfillment/adapters/partycommercial ./internal/architecture/... ./migrations/...`：退 0，48 个包 ok（适配器包的反向依赖都在其中）。跑在 `09286d524` 上；其后两笔只改注释，`go build` 复跑干净。
- 新用例与改过的既有用例 `-count=3`（PC postgres 包）：ok。
- 演示种子：一次性库 `code80seed` 上 `seed.sh --reset` 灌完无错，用后已删。

## 评审记录（2026-10-21，协调方隔离评审）

结论：无阻断，可合入。评审重算钉住的两个无前缀摘要，与 main 写下的字节一致；六个变异只存活一个（「版本键在场却为空」那道校验）。收口各条，都在分支 `claude-code80` 上改：

- N1 票面：代决 4 与越权风险点 2 补读回那一路——`loadPrior` 两处吞掉 error、不记日志，落`解析未决`·`权威不可读`与 `JudgmentAsOfPending`，PS 侧`无法判定`或未决、经 `ErrAcceptanceChainUndecided` 回滚重投；「运维看到的是错误日志」一句限到 Save 那一路。记提供方日志另立 [code/85](./85-a-prior-resolution-that-cannot-be-read-back-is-swallowed-without-a-provider-log.md)（R3，PC）。
- N2 注释：`resolutionCanonicalizationVersion` 头注补「换号时先把无版本与 PCR-1 两支冻结成各自的写法，不再从现行结构体派生」（先例 SAC、CCC、TFC 冻结的无版本摘要函数），并指出嵌着的 `versionDocument` 也属本形状；`commercial_publication.go` 的 `versionDocument` 头注按符号名指回 `resolutionCanonicalizationVersion`。`TestANewSnapshotCarriesItsCanonicalization` 改钉完整金样串 `PCR-1:c26fd65e…`（`uniqueClosure` 夹具实算）。
- N3 用例：`TestASnapshotWhoseVersionDisagreesWithItsDigestIsNotRead` 加子用例「版本键在场却为空」（`jsonb_set(snapshot,'{canonicalization}','""')` 配无前缀摘要）。变异核对：去掉空键校验，这一格红，复原后绿。
- 票面措辞：代决 3 改实（重写出的是 main `32dc1d98d` 那一版的字节，更早形状的存量与 main 同，靠 code/61 的重灌消掉）；代决 1 改为「没有别的摘要前缀叫 `PCR-`」。
- 可选一条已收：`ErrCanonicalizationUnsupported` 头注放宽到商业发布与解析快照两面。
