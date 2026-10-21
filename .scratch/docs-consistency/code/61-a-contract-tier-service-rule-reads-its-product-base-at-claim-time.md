# 61 合同版客户服务规则的产品底座在索赔时按锚点现选，不随接受时的闭包固定；底座多候选留到索赔时才报

Category: bug
Status: ready-for-agent——建议 R2（上线后尽快）：包裹接受之后，产品底座那一版若被截止、同起点退役或有效性更正，客户合同继承来的首次索赔期限与最低材料会在事后改口；首发链路上要等运营对产品版客户服务规则收尾或更正才撞得上，今天演示租户不撞。绕法：无（运营可以不对仍被在途委托继承的产品版收尾，靠人记）。2026-10-21 文档自洽 11 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc11。
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
5. VE 适配器：`productBase` 头注改为底座随闭包固定；`ApplicabilityConflict` 一格在新快照上走不到，保留给存量快照。`tiedToAdoptedContract` 头注里「ADR-0136 决定五」改为「ADR-0136 越权风险点 5」，「ADR-0176 决定四那道点核」补「比较对象照 ADR-0268 决定二」。
6. 用例：底座在接受之后被同起点退役，索赔仍读接受时那一版；底座在接受之后被有效性更正，同上；同范围两版产品底座同期生效，接受时答`适用冲突`；闭包键没声明服务产品、底座壳上指名别的服务产品，闭包未决。演示库 `scripts/demo-seeds --reset` 重灌。

## 相关

- UC-PC-002、UC-VE-007「给开发的交接」各记了这一缺口。
- ADR-0233：截止与同起点退役是底座事后离开新选择的来路。
