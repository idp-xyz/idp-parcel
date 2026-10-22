# 61 合同版客户服务规则的产品底座在索赔时按锚点现选，不随接受时的闭包固定；底座多候选留到索赔时才报

Category: bug
Status: resolved——2026-10-21 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code61（见文末「完成记录」）。此前 ready-for-agent——建议 R2（上线后尽快）：包裹接受之后，产品底座那一版若被截止、同起点退役或有效性更正，客户合同继承来的首次索赔期限与最低材料会在事后改口；首发链路上要等运营对产品版客户服务规则收尾或更正才撞得上，今天演示租户不撞。绕法：无（运营可以不对仍被在途委托继承的产品版收尾，靠人记）。2026-10-21 文档自洽 11 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc11。
Blocked by: 无
出处：审计 [audit/cross.md](../audit/cross.md) B3·2、B3·3；裁定 [ADR-0268](../../../docs/adr/0268-a-contract-tier-service-rule-inherits-from-a-product-base-fixed-with-the-acceptance-time-closure.md)。

## 现象

- ADR-0268 决定一：闭包采纳挂合同的那一版客户服务规则时，继承底座在同一次解析里按同一份键与锚点选出、随闭包一并固定；已接受委托读底座只读闭包上固定的那一版。底座多于一版是客户服务规则这一项的`适用冲突`，在解析时就答；底座与采用的成员同受指名引用核对。
- 代码今天：闭包只记采纳的那一版；VE 索赔读两维时，层次读口每次向权威视图取此刻的登记册、按冻结闭包的锚点现选底座。选法只收状态为生效、选用区间（有效性更正之后的区间）含锚点的版本，所以底座在接受之后可以变。底座答`适用冲突`时，VE 适配器在索赔时报 `ErrUntranslatableAnswer`。
- 底座不过指名引用核对：闭包键上没声明服务产品时，壳上指名了另一个服务产品的产品版也能被选作底座。

## 证据

- `internal/partycommercial/application/customer_service_rule_layers.go`：`LoadCustomerServiceRuleLayers` 调 `reader.authority.LoadScope` 后经 `registry.CustomerServiceRuleProductBase(tenant, scope, anchor, declaredProduct)` 选底座。
- `internal/partycommercial/domain/commercial_resolution.go`：`CustomerServiceRuleProductBase` 与 `applicable`（「已收尾的版本在这里就落选」，并按 `selectionInterval` 判锚点）；`resolveCustomerServiceRuleBasis` 只采纳一版、不记底座。
- `internal/partycommercial/domain/reference_closure.go`：`namedReferencesConfirmed` 只核 `adopted` 里的成员。
- `internal/partycommercial/ports/customer_service_rule_layers.go` 头注：「底座不随闭包冻结，按传入的锚点读时选；已接受委托的底座只靠锚点固定与版本不回溯生效保持不变。」
- `internal/visibilityexception/adapters/partycommercial/claim_service_rules.go`：`productBase` 头注「底座不随闭包冻结，按锚点读时选；有效性更正改了底座区间时这里会改口」，`ApplicabilityConflict` 一格报 `ErrUntranslatableAnswer`；同一文件 `adoptedRuleVersion` 头注写「读一份已固定的闭包没有`适用冲突` / `解析未决` / `输入未受理`可答」，两说并存。`tiedToAdoptedContract` 头注把 ADR-0136 越权风险点 5 写成「ADR-0136 决定五」，ADR-0136 没有决定五。
- `internal/partycommercial/adapters/postgres/commercial_resolution.go`：闭包快照 `adoptedDocument` 按成员带可缺的服务产品、结算政策、信用依据各格，没有底座一格。
- 装配：`cmd/parcel-api/assemble_claims.go` 构造 `NewCustomerServiceRuleLayerReader(authority, contents)`。
- 文档：ADR-0136 决定一、二；ADR-0176 决定二、四与 Consequences；PC CONTEXT「委托被接受时固定其适用的…」与客户服务规则那几条 Rules；VE CONTEXT 索赔资格时点一条。

## 三类

机制：底座在哪一刻选定、存在哪里、读的时候读哪一份。产品策略：无（选法照 ADR-0176 决定二，不变）。租户取值：无（哪个租户给哪份合同挂哪一版仍是登记）。

## 规模

M：PC 领域解析加一格、快照文档加一格（jsonb，不加迁移）、层次读口改读快照、VE 适配器跟改头注与一处分派；补领域、真库与装配用例。

## 最小的诚实修法

1. 领域：`resolveCustomerServiceRuleBasis` 采纳合同层那一版时，按同一键、同一锚点、同一选法（`customerServiceRuleTiers` 的产品层，收窄同 `CustomerServiceRuleProductBase`）选底座：恰一版记在这一项的采纳上（继承底座），零版记没有底座，多版这一项答`适用冲突`。采纳的仍只有一版，`AdoptedFor(CustomerServiceRuleObject)` 交出的不变。
2. 领域：底座随采用的成员一起过 `namedReferencesConfirmed` 那一道判法；它壳上指名的服务产品不是闭包采用的那一个时，闭包答 `NamedReferenceNotConfirmed` 未决。
3. 快照：`adoptedDocument` 给客户服务规则那一项加可缺的底座版本一格，写进与读回对称（ADR-0079 决定九）；本票之前的快照没有这一格。
4. 层次读口：闭包上有底座一格时按它点读正文，不再 `LoadScope` 现选；没有底座一格（本票之前的快照）照今天现选，并在头注写明只出现在演示库与测试夹具里。端口签名要能分辨两种来路（例如改收闭包而不只收锚点与声明产品），由领票时定。
5. VE 适配器：`productBase` 头注改为底座随闭包固定；`ApplicabilityConflict` 一格在新快照上走不到，保留给存量快照。`tiedToAdoptedContract` 头注里「ADR-0136 决定五」改为「ADR-0136 越权风险点 5」，「ADR-0176 决定四那道点核」之后补一句，说明比较对象照 ADR-0268 决定二。
6. 用例：底座在接受之后被同起点退役，索赔仍读接受时那一版；底座在接受之后被有效性更正，同上；同范围两版产品底座同期生效，接受时答`适用冲突`；闭包键没声明服务产品、底座壳上指名别的服务产品，闭包未决。演示库 `scripts/demo-seeds/seed.sh --reset` 重灌。

## 相关

- UC-PC-002、UC-VE-007「给开发的交接」各记了这一缺口。
- ADR-0233：截止与同起点退役是底座事后离开新选择的来路。

## 完成记录（2026-10-21，分支 claude-code61，rebase 于 main `0a7937cd5`）

### 做了什么

- `fc0bff171` 领域：`resolveCustomerServiceRuleBasis` 采纳合同层那一版时，拿同一次分层（`customerServiceRuleTiers`）的产品层选底座——恰一版记在这一项采纳上，零版记「固定了、没有」，多版这一项答`适用冲突`；`AdoptedBasis.InheritedProductBase()` 交回（底座、在场、已固定）三格。`namedReferencesConfirmed` 经 `namingVersions` 把底座纳入核对，底座不进 `adoptedByKind`。重建门 `RehydrateAdoptedBasisSpec` 加三格，拒写不出的形（底座挂在非合同版那一格、挂在产品层那一版上、底座挂合同、底座别租户或别范围、底座不是客户服务规则、在场却未标已固定）。
- `90f6124d1` 快照：`adoptedDocument` 加可缺的 `productBase`（在场即已固定，内含整份 `versionDocument` 或缺席即「没有」），写读对称；缺这一格读回为「未固定」，不补成「没有」。jsonb，不加迁移。
- `c64609b40` 层次读口：`CustomerServiceRuleLayerView.LoadCustomerServiceRuleLayers(ctx, tenant, closure)` 改收整份闭包；固定了一版按它点读，固定了没有答`无适用依据`，旧快照才按闭包锚点与声明产品经 `CustomerServiceRuleProductBase` 现选。VE 适配器把闭包整份交给它；`productBase`、`contractTierDimensions`、`tiedToAdoptedContract`、`adoptedRule` 头注随改（「ADR-0136 决定五」订正为越权风险点 5，比较对象补 ADR-0268 决定二）；`cmd/parcel-api/assemble_claims.go` 装配注随改，装配代码不变。用例：领域七条、真库四条（事后同起点退役与有效性更正两格、固定了没有不拾后来的一版、两版底座接受时冲突、删掉底座一格的旧快照照旧现选）、既有层次读口真库用例改走旧快照一路并补问法立不住五格、VE 合同点核比闭包合同不比查询合同、parcel-api 装配用例补「底座更正后索赔照旧继承」。
- `917fed864` UC-PC-002、UC-VE-007「给开发的交接」里指向本票的现状缺口注改为已补。

PC CONTEXT、VE CONTEXT 与 ADR 正文未改（ADR-0268 票已改 CONTEXT）。AT 盘点无对应行，未动。

**合并后演示库须 `scripts/demo-seeds/seed.sh --reset` 重灌**（ADR-0268 决定三）：灌种子本身不形成闭包，但运行期接受过的合同版委托，快照没有底座一格，会照旧按锚点现选。

### 代决

用户授权「你作为业务和系统专家自决……解决所有的问题」。

1. 端口签名（票面留给领票时定）：改收整份闭包，不另加「底座版本」入参。来路只有闭包分得清；让 VE 先拆闭包再挑来路，等于把「哪一版算底座」的判断搬到消费侧。
2. 解析标识不纳入底座。底座是（键，登记册）的函数，而视图修订已覆盖范围内全部版本、状态与更正；纳入只会让既有解析标识改口（`TestAnExistingProductOnlyRegistrationResolvesAsBefore` 钉的那一个不受影响，产品层采纳本来就不记底座）。
3. 快照记整份版本文档而不只版本身份，与采用成员同形：点读口要壳（租户、范围、指名引用），只记身份就得回登记册查，读时又依赖登记册。
4. 快照不换号、不加迁移：快照未接规范化版本，ADR-0267 决定一不触发；按决定三的实质，同号改形的前提成立——首个租户实例进生产之前，已存摘要只在演示库，重灌即消。快照摘要不带版本本身违背 ADR-0014 Decision「不携带版本的摘要视为不完整」，进生产之后再改形就没有规则可依，另立 [code/80](./80-the-commercial-resolution-snapshot-digest-carries-no-canonicalization-version.md)。
5. 存量快照不回填：生产没有存量；演示库灌种子不形成闭包（一次性库 `code61seed` 上 `seed.sh --reset` 后 `party_commercial.commercial_resolution` 为 0 行），闭包要到运行期接受时才写；回填等于事后按此刻登记册给旧闭包补底座，正是本票要防的现选。ADR-0268 越权风险点 3 已认。
6. 固定了「没有」答`无适用依据`，不回落现选：那会把「接受时没有底座」换成「此刻有一版」。
7. 层次读口的问法收紧为：闭包唯一已解析、属显式租户、采纳了一版壳上指名客户合同的客户服务规则、那一版在闭包键的范围里；任一条不成立即 error。之前采纳产品层那一版的闭包问到这里会把它自己选成底座。
8. VE 适配器 `productBase` 里`适用冲突`、`解析未决`两格保留，只留给旧快照；不删，删了旧快照那一路就没有全函数翻译。
9. 原用例「锚点缺席答`解析未决`」删去：唯一已解析的闭包在重建门就立不起没有锚点的样子，端口收闭包之后这一格到不了。

### 越权风险点

1. **采纳产品层那一版的闭包问层次读口改答 error**（PC owner）。VE 今天只在合同版时问它，生产不撞；别的消费方若照旧拿产品层那一版来问会失败而不是拿回自己。
2. **演示库存量快照重放 Save 答内容冲突**（PC owner）。本票之前固定的合同版闭包若再被同一键、同一视图重解并 Save，快照多了底座一格，摘要不同；演示库重灌即消，生产没有存量。
3. **底座进指名引用核对会挡接受**（PC owner）。闭包键没声明服务产品、底座壳上指名的又不是闭包采用的那个产品时，接受停在`解析未决`（`NAMED_REFERENCE_NOT_CONFIRMED`）。实测演示租户：`SYN-ACCOUNT-02` 声明 `SYN-PROD-CN-SG-EXPRESS` 时闭包唯一解析、底座固定为 `SYN-SERVICE-RULE-01`；声明 `SYN-PROD-CN-SG-ECON` 时那一版底座在选法里就落选，不撞这一格（该声明今天另因接单规则包缺席答`无适用依据`，与本票无关）。
4. **VE 改动范围**（VE owner）：只动 `internal/visibilityexception/adapters/partycommercial/claim_service_rules.go` 与它的合同版用例文件，未碰 code/22、code/75 的文件。

### 跑过的测试

- `go build ./...`、`go vet ./...`、`gofmt -l`：干净。
- `TMPDIR=… scripts/test-full.sh ./internal/partycommercial/... ./internal/visibilityexception/... ./cmd/parcel-api ./cmd/parcel-dispatch ./cmd/parcel-commercial ./cmd/parcel-settlement-register ./internal/{networkrouting,nodeoperations,parcelshipment,settlementaccounting,transportfulfillment}/adapters/partycommercial ./internal/transportfulfillment/application ./internal/architecture/... ./migrations/...`：33 个包 ok。
- `scripts/test-full.sh ./internal/parcelshipment/...`：19 个包 ok（PS 未改，跑它是因为 PS 接受链走真解析）。
- 新用例 `-count=3`（PC 领域、PC postgres、VE 适配器、parcel-api）：4 个包 ok。
- 演示种子：一次性库 `code61seed` 上 `seed.sh --reset` 灌完无错，用后已删。

## 评审记录（2026-10-21，协调方隔离评审）

结论：无阻断；写读对称与变异测试已核。合入前收口的几件，均在 rebase 于 main `0a7937cd5` 之后的分支上改：

- N1：代决 4 的理由改为「快照未接规范化版本，ADR-0267 决定一不触发；按决定三的实质，前提成立」；快照摘要不带版本、进生产后再改形无规则可依那一格另立 [code/80](./80-the-commercial-resolution-snapshot-digest-carries-no-canonicalization-version.md)（R2）。ADR-0014 注记不动。
- N2：完成记录补「合并后演示库须 `seed.sh --reset` 重灌」。
- N3：VE 适配器 `productBase` 头注里`无适用依据`那一格按来路分写恢复动作：闭包上固定了没有，已接受委托无恢复动作；旧快照现选那一路，恢复动作是登一版产品版。
- N4：`TestTheRehydrationGateKeepsTheFixedProductBaseHonest` 拒绝表补「底座属别的租户」一格；变异核对：去掉重建门里的租户比较，这一格红，恢复后绿。
- 顺手：底座冲突时 `candidateCount` 报产品层的候选数；端口与 VE 适配器两处超长头注折行；`legacyClosureOver` 头注改用测试符号名指路，不写文件名。

## 进 main 记录（2026-10-21，协调方）

- 分支 `claude-code61` tip `8e996e7db` 在 main `0a7937cd5` 上快进；隔离评审无阻断，四条建议已收（变异补证别租户底座一格）。
- 协调方在 tip 上单独提交重生成的机制清点；ADR 索引与 AT 计数无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0，158 个包 ok；`go build ./...`、`go vet ./...` 干净；演示种子在一次性库上灌通，用后删库。共享演示库合并后须 `scripts/demo-seeds/seed.sh --reset` 重灌。
- 去向：code/80（PC 解析快照摘要不带规范化版本，R2）。
