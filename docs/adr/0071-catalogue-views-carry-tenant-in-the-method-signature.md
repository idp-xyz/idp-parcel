# ADR-0071: visibility-exception 的目录与策略视图把租户放进方法签名，不在构造期绑定

Status: Proposed  
Date: 2026-08-21

## Context

`internal/visibilityexception/ports` 里的读侧端口分两派，而两派对同一件事——租户这一维放在哪里——给出相反的答案。

**仓储一派让租户在签名上看得见。** `ProjectionStore`、`CustomerViewStore`、`AcceptedFactStore` 一类的每个方法要么直接收 `tenant domain.TenantID`，要么收一个带 `Tenant` 维的键结构（`ports.FactKey`）。`ProjectionStore` 的注释直接给出理由：

> 租户是最高数据隔离边界（ADR-0003）：TrackedParcelReference 只是字符串引用，缺租户维两个租户的同名包裹就会共用一份投影。

`CustomerViewStore` 同句加重一层：「租户是最高数据隔离边界（ADR-0003），跨越它必须在签名上看得见」。

**目录与策略一派把租户放进构造器。** 六个视图属这一派，对应六个 PostgreSQL 适配器：

| 端口 | 方法 | 适配器 |
|---|---|---|
| `MilestoneMappingView` | `ClassifyFact(ctx, fact)` | `MilestoneMappings` |
| `DisclosurePolicyView` | `AssessDisclosure(ctx, customer, projection)` | `DisclosurePolicies` |
| `NotificationPolicyView` | `DirectNotification(ctx, disclosure)` | `NotificationPolicies` |
| `EligibilityRuleView` | `RulesForClaim(ctx, query)` | `ClaimEligibilityRules` |
| `TriageRuleView` | `TriageSignal(ctx, query)` | `TriageRules` |
| `ActiveCaseView` | `CaseActive(ctx, caseID)` | `ExceptionCases` |

六个适配器结构体都是 `{db, tenant}`，构造器都是 `New*(db, tenant)`。理由写在 `ExceptionCases` 上，其余五个逐个引它：

> 租户在装配期固定：ActiveCaseView.CaseActive 的签名里没有租户，而案件标识只在租户内唯一（ADR-0003）。把租户放进构造器，跨租户就仍然是装配期看得见的一次选择，而不是查询期一个可以忘掉的参数。

同一段还写明零值租户合法：

> 允许零值租户，且那不是错误：本产品今天还没有租户，零值即「租户未登记」，此时任何案件都不在场。构造期拒绝零值会让整个 VE 装配不起来，而 CaseActive 交回 found=false 恰好是安全方向。

两派各自成立过。问题是它们在同一个编排里并排出现。

### 分歧发生在单个 handler 内部

`DeriveProjectionHandler.Handle` 有五个依赖要用租户。`Facts.FindByParcel`、`Projections.FindCurrent`、`Projections.Save` 与 `Downstream.HandOffProjection` 四个都收 `command.TenantID`；只有 `Mapping.ClassifyFact(ctx, fact)` 不收。而 `DeriveProjectionCommand` 的注释写着「租户显式随命令到达（ADR-0003）：事实引用只在租户内唯一，编排不替来源补租户」——租户就在手边，唯独第五口接不住。

`DeriveCustomerViewHandler.Handle` 同形：`Views.FindCurrent` 与 `Views.Save` 收租户，`Policy.AssessDisclosure` 不收。

最尖锐的一处在 `HandleClaimHandler`。`ports.go` 把 `ClaimEvidenceView` 描述为 `EligibilityRuleView` 的对偶：

> 它答「事实是什么」，与目录答的「规则是什么」分列两个端口——最低材料要求这一维正是靠两边相减才核得出来。

两口在同一次资格判断里相减，签名却不一致：`Evidence.ReceivedMaterials(ctx, tenant, batch, item)` 带租户，`Eligibility.RulesForClaim(ctx, query)` 不带。**一对按设计成对的端口，在最高隔离边界这一维上说了两种话。**

### 构造期绑定的理由在派发进程里已经失效

「装配期看得见的一次选择」预设装配期知道租户。`cmd/parcel-dispatch` 不满足这个前提：它长驻、多租户，装配期是进程启动那一刻，那时没有任何租户。

组合根因此在 `assemble.go` 里造了两个包装类型顶住——`tenantBoundProjectionDerive` 与 `tenantBoundCustomerViewDerive`。两者都在每次 `Handle` 里读 `command.TenantID`、现场构造视图、再构造编排并调用。`tenantBoundProjectionDerive` 一个类型上挂了九个 `var _` 接口断言（`venodeops.ProjectionHandler`、`vetf` 四口、`vecc` 两口、`veps.FinalProjectionHandler`、`venr.InitialRouteProjectionHandler`）。

于是租户还是变成了查询期的参数——只不过挪进了 `cmd/`，由手写代码兜着。构造期绑定想避免的那件事没有被避免，只是换了地方发生，而且换到了一个更差的地方。

### 守卫落在结构上守不住的位置

两个包装各自手写同一道零值检查：

```go
if command.TenantID.String() == "" {
    return veapplication.DeriveProjectionResult{}, errors.New("parcel-dispatch: projection tenant is empty")
}
```

这道检查是唯一的防线，因为 `NewMilestoneMappings(db, 零值)` 按上引理由不报错。`assemble.go` 的注释也只能以禁令形式记下来：「禁止 `NewMilestoneMappings(db, 空租户)`——看起来接了库、永远读不到行。」

漏抄的后果是：视图恒答 `found=false`，编排落 `PROJECTION_DERIVED` + 未归类（`MAPPING_NOT_CONFIGURED`）。**而这正是首发唯一走得到的正确分支**——今天没有租户，目录本就是空的，正确接线与漏接线的可观察行为逐字节相同。没有任何测试分得出两者，`go vet` 与编译器更不会有信号。按 [AGENTS.md](../../AGENTS.md) 对同类形状的判语，这属于「结构上守不了，只能靠不写」的那一类。

规模上不是孤例。六个视图今天只接了两个，另外四个（`NotificationPolicies`、`ClaimEligibilityRules`、`TriageRules`、`ExceptionCases`）的消费者还没进装配；`cmd/parcel-api` 的 VE 端点解开 `unwired*` 守卫时会按请求再要一遍同样的东西。每多接一个消费者，就多一份手抄的守卫和一次漏抄的机会。

### 同一个文件里已经有一处拒绝了同一形状

这不是一条要新立的原则。`assemble.go` 的 `intakeQualificationEvidence` 面对结构相同的一格时当场报错，理由写在那里：

> 填了表却没认领段：两个字段要一起才立得住。放过去就是一张永远问不到的表，判断结果与真的没配置一模一样，而要人做的事相反。

**同一个文件、同一类失败、同一句诊断——一处拒绝，一处只能靠手抄守卫。** 差别不在认识水平，在结构：收寄硬资格那条路只有 `intakeQualificationEvidence` 一个工厂，所有消费者从它过，那道检查因此写一次就够，且被 `intake_qualification_evidence_wiring_test.go` 的 `TestAQualificationRegistryWithoutAClaimedAuthorityIsRefused` 钉住；租户这条路每个消费者各建各的包装，检查随之各抄一份，没有一份有测试。

本记录要的比那一处更进一格：那里仍是运行期检查加一个钉住它的测试，本记录要把同一格升成编译错误。理由是两处的可测性不同——`intakeQualificationEvidence` 是个纯函数，给它一个坏输入看它报不报错就行；而「某个装配点漏写了租户守卫」这件事没有对应的被测对象，要测只能去测「有没有人忘了写」，那不是测试能表达的命题。

同一文件里另有一处相邻表述值得对照，它本身是对的，而对照出租户这一维缺的正是什么。`nodeQualificationAuthority` 的注释把零值读作「本部署没认领任何段」而非「忘了填」，并说明「后者要有人去补，前者是今天的真话」——这个区分正是本记录想要的。但它成立有个前提：**零值与漏填得在类型上分得开**。收寄硬资格那一维满足，因为「没认领任何段」是个有名字的取值（`nodeQualificationAuthority{}` 经 `intakeQualificationEvidence` 换出 `UnconfiguredIntakeQualificationEvidence{}`），漏填则被上引那道检查挡在构造期。

租户这一维不满足：零值租户与漏传租户是同一个 `domain.TenantID{}`，要人做的事相反，取值却一模一样。决定一把租户挪进签名，图的就是让这两件事不再共用一个值——漏传不再是一个合法零值，而是一个编译错误。

### 现有机制在能力上就够不着这一格

不是没人写门禁，是写不出来。三条各自量过：

- **判「有没有非测试调用点」的那类门禁抓不到它，因为这两个构造器有。** `NewMilestoneMappings` 与 `NewDisclosurePolicies` 的生产调用点就在 `tenantBoundProjectionDerive.Handle` 与 `tenantBoundCustomerViewDerive.Handle` 里，非 `_test.go`。按那条判据它们是「已接线」，门禁恒绿。[生产接线棘轮票](../../.scratch/production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)因此拦不住本记录这一类，反之本记录也不该被当成那张票的重复。
- **`internal/architecture` 的门禁全走纯语法。** 取源只有两条路，都不建类型信息：`parseRepositorySources` 做 `parser.ParseFile(path, nil, 0)`，`loadSources` 更只取 `parser.ImportsOnly`；`types.ExprString` 用在渲染表达式字面，不作类型判定。「某个调用点少传了一个 `domain.TenantID`」要类型信息才认得出，现有扫描器在能力上到不了——这是能力问题不是覆盖面问题，加几条规则不解决。（实测于 `3df0160`。**此处刻意不写门禁份数**：份数是对一个会动的东西的引用，写下时对、多一套就过期，而不会有任何东西因此变红。同一 SHA 上本包已有三处注释栽在这上面——`envelope_partition_gate_test.go`、`transaction_closure_gate_test.go`、`identity_prefix_gate_test.go` 各写着一句「本包另 N 套」，三个 N 还互不相同，那是门禁逐次增加留下的漂移痕迹；修正已并入[生产接线棘轮票](../../.scratch/production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)，随它新增门禁那一笔一起改。**本段刻意锚在 SHA 上**：不锚的话，那三处被修好之后这句举证自己就成了假话——正是它在讲的那个毛病。同 AGENTS.md 不用行号那条：行号会指错，计数会数错，两者失效都无声。）
- **测试层面没有对应的被测对象。** 「某个装配点漏写了租户守卫」不是某个函数的行为，而是一段没被写下来的代码。要测只能去测「有没有人忘了写」，那不是测试能表达的命题。

三条合起来正是本记录选编译期而非再加一道门禁的理由：**前两条说现有工具够不着，第三条说再造一个也够不着。** 唯一够得着的是类型检查器——它验的恰好就是「每个调用点是否都给出了签名要求的东西」，与这一格要守的是同一件事。这也是本记录与「多加一道校验」类方案的分界：**不是把守卫写得更严，是把它挪到一个不需要有人记得写的位置。**

换个说法也成立，且它解释了为什么偏偏是签名而不是别处：这一格散在 N 个装配点上，而**端口签名是那 N 个入口唯一的公共上游**——改它一次对全部入口生效，且第 N+1 个入口出现时不需要任何人记得跟上。这与决定六的判据是同一件事的两面：错误状态在类型上没有名字时，就只能去改那个所有入口都必须经过的地方。

### 改动面（按代码清点，非转述）

以六个方法名清点，全仓 32 份文件、73 处：

| 类别 | 份数 | 明细 |
|---|---|---|
| 端口声明 | 1 | `ports/ports.go`（6 处） |
| 生产适配器 | 5 | `adapters/postgres/` 下 `rule_catalog.go`（2 处，`MilestoneMappings` 与 `TriageRules` 同文件）、`disclosure_policy.go`、`notification_policy.go`、`claim_eligibility.go`、`active_case.go` |
| 生产编排 | 6 | `application/` 下 `derive_projection.go`、`derive_customer_view.go`、`notify_customer.go`、`handle_claim.go`、`raise_signal.go`、`send_disposition_request.go` |
| 测试 | 20 | 真库适配器测试 5 份（共 40 处）、编排测试 6 份、消费侧适配器测试 7 份、`veconsume` 测试 1 份、HTTP 测试 1 份 |

另有六个构造器的调用点（5 份测试 + `assemble.go`），以及 `assemble.go` 的两个包装类型本身。多数改动是机械的替身签名对齐。

## Decision

**一、六个视图的租户进方法签名，构造器不再收租户。**

`New*(db, tenant)` 收窄为 `New*(db)`，适配器结构体去掉 `tenant` 字段，租户改由每次调用传入。理由与 `ProjectionStore` 那句一字不差地成立：租户是最高数据隔离边界，跨越它必须在签名上看得见。目录行与投影行同样按租户分片，同样是「缺租户维两个租户共用一份」，没有理由只对其中一类要求签名可见。

**二、租户的落位形式随入参形状定，与本仓既有两种写法对齐，不新造第三种。**

- 平铺入参的四口取 `(ctx, tenant, ...)`，与同文件 `ClaimEvidenceView.ReceivedMaterials`、`ParcelCustomerAccountView.FindCustomerAccount` 一致：
  - `ClassifyFact(ctx, tenant, fact)`
  - `AssessDisclosure(ctx, tenant, customer, projection)`
  - `DirectNotification(ctx, tenant, disclosure)`
  - `CaseActive(ctx, tenant, caseID)`
- 带查询结构的两口给结构补 `Tenant` 字段，与同文件 `ports.FactKey` 一致：`TriageQuery` 与 `EligibilityQuery` 各加一维，方法签名不动。

分派依据不是随意的：有键或查询结构时租户是那个结构的一维，没有时它是一个独立入参。同文件 `AcceptedFactStore` 一个接口上就并用了两种写法——`FindByKey(ctx, key FactKey)` 走结构字段，`FindByParcel(ctx, tenant, parcel)` 走参数——可见这不是两种方言而是同一条规则的两种落法。**不得让租户同时以字段和参数两种形式出现在同一口上**，那会给「以哪个为准」留一道缝。

**三、领域类型不因此加租户维。**

`AcceptedSourceFact`、`TrackingProjection`、`CaseID`、`DisclosureDecision` 都不加租户字段。租户是数据隔离边界，不是这些概念的组成部分；给领域类型加一维持久化分片键，会让它们在任何非持久化用途上都多带一个说不出业务含义的字段。租户走签名，正是为了不走这条路。

**四、`cmd/parcel-dispatch` 的两个租户现绑包装随之删除。**

`tenantBoundProjectionDerive` 与 `tenantBoundCustomerViewDerive` 连同两者共十个接口断言（前者九个，后者一个 `veps.CustomerViewDeriveHandler`）一并去掉。目录视图与编排 handler 都回到装配期构造一次，消费侧适配器直接收 `*veapplication.DeriveProjectionHandler`。手写的零值检查不再需要——租户从命令流到端口，中途少传一维就编译不过。

**五、两处注释随本记录被接受一并处理，方式不同。**

- `ExceptionCases` 那段「租户在装配期固定」及其余五处引用它的注释**收窄**：同一条口径不得在仓里留两份互相打架的说法（AGENTS.md 单一权威）。
- `assemble.go` 的 `nodeQualificationAuthority` 那段「零值读作本部署没认领任何段，不是忘了填」**不收窄，改为指向本记录决定六**。它在自己那一维是对的，问题只在没说适用条件，而下一个人恰恰在改装配代码时读到它——反例（两个单字段零值的租户包装）就在同一文件里不到两百行外，两处挨着、一处给方法一处不适用，却没有任何东西说明为何不同。**注释里补的是一个指针不是一份复述**，理由同上。
- 顺同一笔带走第三处：`intakeQualificationEvidence` 的注释里「构造期**两道**校验都会放行」应**去掉那个数**（改写成「构造期的段内校验」之类）。它数的是 `psnodeops.NewNodeExecutionQualificationEvidence` 里的校验条数——**一个跨包计数**，而 AGENTS.md 那条红线今天只写了「不用行号」。行号与计数是同一个毛病的两面，那句注释守住了前半条、在同一句里栽了后半条。它与上一条落在同一个文件的相邻注释块，决定四本就要改这个文件，零额外成本。

三处都不构成独立改动，也**不为它们单开一笔**：理由与上一条相同。

此三项不随接受动作遗忘。**都在接受之后才动**：指向一份 `Proposed` 记录当约束，比不指更乱。落地时它们不是独立一笔——决定四本就要改 `assemble.go`、决定一本就要改 `active_case.go`，三句注释顺同一笔带走即可，**不必为几行注释单开隔离树跑全量套件**。

**六、判据的一般形式：这一维的「合法空白」与「有人忘了传」在类型上分不分得开。**

本记录的核心论证不依赖租户这一维的任何特殊性，因此把它写成通则，供后续同类判断引用：

- **分得开**——空白是一个有名字的取值，或半份状态在构造期可判。`nodeQualificationAuthority` 属此类：`prefix` 与 `qualifying` 成对，有表无前缀检得出来因而拒得掉，两份皆空才读作「没认领任何段」。此时把检查收进**一个工厂**、写一次并用用例钉住即可，`intakeQualificationEvidence` 是本仓已有的正确样例。
- **分不开**——单字段零值同时表示两件事。`domain.TenantID{}` 属此类：它既是「本产品还没有租户」也是「这个装配点忘了传」。此时手抄守卫治不了根，因为它生效的前提正是有人记得写，而漏写与正确的可观察结果相同。两条出路：改类型让它分得开，或如实承认这一维只有纪律没有机制。**本记录选前者。**

判据落在「分不分得开」而不落在「危不危险」：后者要靠人判，且上面两处的危险程度相同、结论却相反。这也是「门禁补不了、只能改类型」那一层假绿的具体判定法——**能不能守住不取决于它多隐蔽，取决于错误状态在类型上有没有名字。**

## Consequences

- 守卫从「手写且守不住」变成编译期强制。漏传租户不再表现为「恒答未配置」这个与正确行为不可区分的形状，而是一个编译错误。这是本记录要买的唯一一样东西，其余都是它的连带。
- **这一格的不可见有期限，而期限结束时它会集中显形，不是陆续暴露。** 今天没有租户，六个视图一律答未配置，接对与接错同样「正确」。第一个租户登记那一刻判据翻转：接对的开始答真实目录行，漏传租户的仍答未配置——而那时未配置不再是真话，它成了一次静默的空白响应。缺陷因此不是被逐个发现，是同一天一起发作，发作现场还是一个刚上线的租户。
- 这与「测试没真跑」那几类不可见性质不同，值得分开记：那几类是静态的，今天不修明天也一样，且随时可查；本类的不可见来自**一个会结束的状态**，查不出来是因为今天还没有能判对错的基准。**每多接一个靠人抄守卫的消费者，就多一发留到那天才响的存货。** 这条只陈述本记录裁的这一格，不改产品就绪判据——那一处的权威在开发主线，本记录不碰。
- 组合根退回只做「选哪个实现、按什么键登记」。两个包装类型连注释在 `3df0160` 上共 112 行（客户视图那个 33 行、投影那个含构造函数 79 行），10 个接口断言随之消失。**净减少小于这个数**：编排 handler 仍要在装配期构造一次，那部分代码会加回来——此处不估净值，真改完才数得准。`cmd/parcel-api` 将来接 VE 端点时不必再复制第三份包装。
- 32 份文件要改，其中 20 份是测试。真库测试 5 份改动集中（共 40 处调用），编排测试与消费侧适配器测试多为替身签名对齐。改动量不小但机械，无一处需要重新判断业务语义。
- 每次 `Handle` 不再重建视图对象。这不是本记录的目的，附带发生，不作为理由——重建的是两个字段的结构体，本就不构成成本论据。
- **接受之前不得动代码。** Status 为 Proposed，按 [ADR-0070](./0070-customs-rule-registries-split-recording-from-selection.md) 同一条纪律：据草案改代码是抢跑。
- 本记录只裁 visibility-exception。其余九个上下文是否存在同形分歧未清点，不在本记录范围内；若清出同类，照本记录的理由办，但要各自立记录还是并入本记录，留给那次清点决定。

## Alternatives considered

- **维持构造期绑定，把两个包装挪进 VE 自己的装配包。** 否决：只搬家。守卫仍是手写，仍守不住；第三个消费者仍会漏抄；端口上那道分歧原样留着，下一个读 `ports.go` 的人仍要在同一份文件里读到两种关于租户的说法。
- **只让六个构造器拒绝零值租户。** 否决作为终局方案：它确实把守卫从 `cmd/` 挪进适配器包因而可测，是本记录之外唯一有实质改善的中间态。但它不解决根因——`cmd/` 仍须知道「这类视图要按 Handle 现绑」这条不成文规则，包装类型仍留在组合根，端口分歧仍在。且它与 `ExceptionCases` 现有的「允许零值不是错误」直接冲突，要改的注释与本记录要改的是同一批，付一样的文档代价买一半的效果。**若本记录被否决，此案可作为退路单独提。**
- **给领域类型加租户维，让视图从入参里取。** 否决：见决定三。且它只解决 `ClassifyFact` 与 `AssessDisclosure` 两口，`CaseActive(ctx, caseID)` 收的是标识不是聚合，无处可加。
- **用 `context` 携带租户。** 否决：撞本仓通例。[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)记全部编排复现的五条形状，第一条即「租户显式入参而不从 context 里补」。为六个端口破这一条，等于用一个隐式通道换六处显式签名，恰好把「签名上看得见」这条要求反过来。
- **什么都不做，只补一条 ADR 把「多租户长驻进程里目录视图必须现绑」写成明规则。** 否决：把一条守不住的约定升格为明文，不会让它变得守得住。漏抄仍然无信号，而明文会让漏抄的人显得更没道理——那是追责而不是防错。

## Links

- [ADR-0003：采用集团租户、法人责任与货主客户账户三级边界](./0003-group-tenant-legal-entity-customer-account.md)：租户为最高业务数据隔离边界的出处，两派注释引的都是它
- [ADR-0040：商业版本身份键携带 TenantID，跨租户同号互不可见](./0040-commercial-version-key-carries-tenant-id.md)、[ADR-0041：业务参与方与货主客户账户携带 TenantID](./0041-business-party-and-customer-account-carry-tenant-id.md)：同一条隔离边界在身份键上的两次落地，本记录是它在读侧端口上的第三次
- [ADR-0049：集成事件的发布通道首发采用进程内直投](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：`cmd/parcel-dispatch` 何以成为长驻多租户进程、组合根依赖面何以最宽
- [visibility-exception CONTEXT](../domain/visibility-exception/CONTEXT.md)：六个目录与策略视图各自「未配置」格的语义出处
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：「租户显式入参而不从 context 里补」的出处；「端口没接满」一节记的正是这六口所属的那批
- [ADR-0070](./0070-customs-rule-registries-split-recording-from-selection.md)：Proposed 期间不得据草案改代码，本记录沿用同一条纪律
- [生产接线棘轮票](../../.scratch/production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)：按「有没有非测试调用点」判接线的那道门禁。本记录这一类**在它的判据下恒绿**，两者不重叠——它守「只有测试接了」，本记录守「接了但漏了一维，而漏与不漏的可观察结果相同」。清单清空不代表接线这一面已经守住
