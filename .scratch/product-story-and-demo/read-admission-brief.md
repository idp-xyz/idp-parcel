# 隔离环境读面准入:裁决取证简报

Category: 材料(取证简报)
Status: 只报事实与选项,不做裁决;裁决由调度方上报用户定
取证时点:2026-08-25,锚提交 `5703b4d`;成文期间票 07 收口(`253b449` 实现、`06ccf9e` 票面 resolved),该笔只改五个页面组件的计数摘要守卫,不在本文所引文件之列——**所引事实在 `06ccf9e` 上逐项仍成立**。取证时树上另有频道 5 的在途未提交种子产物(scripts/demo-seeds/),不作为本文证据。

背景:journey-draft.md(`5703b4d`)勘察发现「读面数据态在当前装配下不可达」。票 07 收口时的处置是**只验未配置态、「数据态留集成轮」**(其票面 18:40 收口条:curl 直连与经代理各打六端点全 403;数据态候票 08 种子与集成轮),因此这一问现在落在**集成轮与票 08「七页可见种子数据」、票 04「页面负责如实展示」**上。本文为「隔离环境读面准入」裁决备齐四部分材料。

---

## 一、ADR-0055 的精确约束

出处:docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md(Accepted,2026-08-17;Status 行自带数目勘误:端点实为八个,TF 双端点此前被计作一项)。

**Decision 逐点要旨**:

1. **Decision 一**:业务端点不再等接入渠道参数才进装配,各以「未配置即拒」Intake 起步——不读业务内容、不采信自报身份、不构造命令,一律如实答「接入渠道未配置」。这一格住在 Intake 缝(各处理器第一参),不在路由层另设闸;真渠道 Intake 就位时**在装配点逐端点替换**,路由层与处理器不动。ADR-0022 的「但不进 `cmd/parcel-api` 的装配」一句据此停用;同段「禁止在两项未决前落地任何默认实现」**原文有效,本记录靠它划界而不是绕开它**。
2. **Decision 二**:未配置自成一格,错误码 `ACCESS_CHANNEL_NOT_CONFIGURED`,与 `MALFORMED_REQUEST`、`INTAKE_FAILED` 并列;分格判据是恢复动作不同(ADR-0029)——这一格要接入方去提供并配置渠道参数(客户委托面即 `PAR-INT-01`,外部结果面即 `PAR-INT-03`);错误码命名状态,不命名参数。
3. **Decision 三**:状态码取 403;404 是要治的折叠,401 邀请换凭证而此刻不存在凭证方案,5xx 会被重试库读成「过会儿就好」。答复对一切请求内容与自报身份一致。
4. **Decision 四**:与 ADR-0029「越权探针与真不存在同答」的关系——未配置格只披露产品表面(「本产品有此端点、渠道未配置」);租户数为零时也没有客户数据可泄;业务资源层面的探针同答约束在真渠道 Intake 就位后照常适用。
5. **Decision 五(原文照录)**:「本记录不解锁 Intake 的另两项未决。载荷规范化摘要与准入范围装配(`PAR-GOV-03..07`)仍然拦着真渠道 Intake;未配置即拒绕开它们只因它走不到那一步。真渠道落地时它们必须先行或同批。」

**两项未决机制的谱系与确切要求**:

- **原始出处是 ADR-0022 Decision 末段**(docs/adr/0022-http-status-carries-answer-formed-not-business-verdict.md):「接入认证与规范化摘要是两项显式未决,本记录不决定,并禁止在它们决定前落地任何默认实现。」其中:**接入认证**——来源信封的权威来源是接入适配器的认证结果,「采信客户自报的租户号会穿透 ADR-0003 的最高数据隔离边界」,认证方式属 `PAR-INT-01` 待提供;**规范化摘要**——载荷摘要须覆盖规范化业务内容,ADR-0014 只约束摘要形状带版本号,parcel-shipment 侧的具体形状当时未实现。
- **ADR-0072**(docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)三条更新:①接入渠道登记册、凭据验证、来源信封铸造归共享技术能力,**落点定为 `internal/accessidentity/`**——取证 SHA 上该目录**不存在**(`ls internal/` 十二个目录无此项),能力未开工;②「ADR-0055 的否决维持」:登记册表结构与凭据形态在 `PAR-INT-01` 最低证据到位前不立,替换点仍是 `assembleBusinessEndpoints` 逐端点换;③**载荷规范化摘要与本决定解耦**:PS 侧摘要进出边界已由 PS CONTEXT 定死,「属机制半边,独立成票现在就做…不等渠道,不被本 ADR 阻塞」。
- **`PAR-GOV-03..07`**(docs/product/PILOT-PARAMETER-REGISTER.md)五行当前登记**全部「待提供」**:PAR-GOV-03 限量生产准入与生产权威规则(接受前按提交版本整体纳入或排除)、04 各阶段候选版本与 Go/No-Go 授权、05 紧急暂停硬风险规则、06 恢复准入证据、07 回退/停新准入/对象级接管条件。

**拦写路径还是连读路径一并拦——ADR 原文的措辞盘点(只报句子,不下结论)**:

- Decision 五的宾语是「**真渠道 Intake**」,未区分读写。
- 两项机制各自的文本语境都在**写侧**:载荷规范化摘要挂在 PS 提交的内容摘要(ADR-0022/ADR-0014/ADR-0072 Decision 三);准入范围装配挂在「委托接受前依据版本化准入范围决定是否纳入限量生产」(PAR-GOV-03 行文、pn-08 交接件 4)。
- **读路径的拦截物,按 ADR-0077 Decision 三只点名一件**:「运营接入认证属接入渠道实例半边、未登记」(`PAR-INT-01`);同句禁「任何『开发用』采信实现」,并定「作用域只在真 Intake 之后由认证结果铸造」。
- **没有任何一句原文把两项未决机制点名到目录读路径上;也没有任何一句原文豁免读路径。** 豁免与否即本次裁决空间。票 04 v1/v2 分界句(「隔离环境的合成接入渠道也被 ADR-0055 Decision 五两项机制未决拦着」)的语境是**写路径从页面发起**(v2)。

**一处已接受 ADR 内部的张力(裁决要正面处理的)**:ADR-0077 Consequences 第二条原文——「接入认证参数未登记前,生产路径没有任何选项能让页面显示真数据……**隔离合成 S 环境里,种子经登记 CLI 灌入后页面可见合成数据,证据层级记 S**。」后半句声称的状态在取证 SHA 的代码里**没有任何机制支撑**:其自己的 Decision 三裁未配置即拒,生产代码无任何放行 Intake(见二)。同篇 Alternatives 已否决「前端 mock 演示数据」(mock 连 S 都不是)与「等 PAR-INT-01 一起做」(租户数为零时那是一件不会到来的事)。

## 二、现状取证

**装配层接线形状**(cmd/parcel-api/endpoints.go,`assembleBusinessEndpoints`):端点表在取证 SHA 上共 17 行,每行形如 `{Pattern: <路径>, Handler: <ctx>http.New*Endpoint(<ctx>http.UnconfiguredIntake{}, <读口或编排>)}`——Intake 以**字面量零值结构体**逐行写死,不经任何变量、配置或条件分支。七页对应的六个 GET 装配行:

```go
{Pattern: "/pricing-price-cards", Handler: pricinghttp.NewQueryPriceCardsEndpoint(pricinghttp.UnconfiguredIntake{}, priceCards)},
{Pattern: "/pricing-reference-series", Handler: pricinghttp.NewQueryReferenceSeriesEndpoint(pricinghttp.UnconfiguredIntake{}, referenceSeries)},
{Pattern: "/network-catalog", Handler: networkhttp.NewQueryNetworkCatalogEndpoint(networkhttp.UnconfiguredIntake{}, networkCatalog)},
{Pattern: "/customs-compliance-rules", Handler: customshttp.NewQueryComplianceRulesEndpoint(customshttp.UnconfiguredIntake{}, complianceRules)},
{Pattern: "/commercial-service-products", Handler: commercialhttp.NewQueryServiceProductsEndpoint(commercialhttp.UnconfiguredIntake{}, serviceProducts)},
{Pattern: "/commercial-policies", Handler: commercialhttp.NewQueryCommercialPoliciesEndpoint(commercialhttp.UnconfiguredIntake{}, commercialPolicies)},
```

读口(`priceCards` 等)由 main.go 构造真库读适配器交入(`pppostgres.NewOperationsCatalogue` / `nrpostgres.NewNetworkCatalog` / `ccpostgres.NewRuleCatalogue` / `pcpostgres.NewOperationsCatalogue`);endpoints.go 注释原话:「未配置 Intake 仍拒在它们之前,接入渠道就位前一次也不会被调到」。(摘录按符号 `assembleBusinessEndpoints` 锚定于取证 SHA,不锚行号——行号纪律见 AGENTS.md「改文档」。)

**UnconfiguredIntake 的确切形状**(五个上下文各一份同款文件 `internal/<ctx>/adapters/http/unconfigured_intake.go`,以 parcelpricing 为例):零字段 `type UnconfiguredIntake struct{}`;方法 `IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error)` **参数刻意匿名**——注释原话「连签名都不给『读一眼再决定』留位置」;一律返回哨兵 `ErrAccessChannelNotConfigured`;带编译期断言 `var _ PricingCatalogueIntake = UnconfiguredIntake{}`。注释自证分界:「它不是被禁的『开发用』采信实现——那条红线禁的是采信自报租户……这里的空登记册就是装配点本身。真渠道就位时在装配点替换,本类型随之退场。」

**Intake 接口与作用域形状**(internal/parcelpricing/adapters/http/catalogue_intake.go 等):`IntakeCatalogueQuery` 交回 `CatalogueQuery{Scope, Limit}`;Scope 是各上下文自立的 `OperationsQueryScope`(作用域引用+租户两维非零,**无客户维**;ADR-0077 Decision 二);注释原话:「作用域来自认证与授权结果……两样都不采信调用方自报」。

**测试替身的确切形状与所在文件**(均 `_test.go`,生产装配点无一行引用):

- `grantedCatalogueIntake{tenant string; limit int}`——internal/networkrouting/adapters/http/query_network_catalog_test.go 与 internal/customscompliance/adapters/http/query_compliance_rules_test.go 各一份;不读请求,凭空铸 `OperationsScopeReference("OPS-SCOPE-1")` 加注入的租户构造作用域。注释原话:「真渠道未登记(PAR-INT-01),生产装配点不会有这样的实现——它只在测试里存在,为的是隔离验证端点的分派与转写。」
- `intakeDouble`——internal/parcelpricing/adapters/http/query_pricing_catalogue_test.go 与 internal/partycommercial/adapters/http/query_commercial_catalogue_test.go,同性质。

**`PAR-INT-01` 登记行**(docs/product/PILOT-PARAMETER-REGISTER.md,原文要素照录):参数「客户生产委托接入渠道」;已确认约束「只启用客户当前主用的一种渠道;接受后资料修订复用同一受控入口或具有明确等效身份、幂等和审计边界」;**当前登记「待提供」**;最低证据「API、标准文件或门户的现行流程、委托与资料修订业务标识、重试和冲突边界」;证据责任「客户接入、技术」。

**前端一侧**(apps/admin-web):pages/catalogue-api.ts 把 `403 + ACCESS_CHANNEL_NOT_CONFIGURED` 译成头等结果 `{kind:'unconfigured'}`;vite.config.ts 的 dev 代理把 `/api` 剥前缀直转 parcel-api,**无 mock、无旁路**;main.tsx 三处 `configure*Api({basePrefix:'/api'})`。

## 三、既有先例:仓内有无「隔离/演示环境专用装配」

**结论:取证 SHA 上,本仓不存在任何按环境切换进程装配的先例。** 逐项取证:

- **cmd/ 八个入口的环境变量总共三个**(grep `os.Getenv|getenv(` 全命中):`IDP_PARCEL_HTTP_ADDR`(parcel-api 监听地址)、`IDP_PARCEL_POSTGRES_DSN`(八口共用连接串,未设即拒)、`IDP_PARCEL_DISPATCH_INTERVAL`(parcel-dispatch 节拍)。没有任何 demo/isolated/mode 类开关,没有第二个 main、没有构建标记分支。
- **唯一「演示」页 template-preview**(apps/admin-web/src/pages/template-preview/):纯前端模板橱窗,数据来自按显式路径引入的 demo.ts——文件注释原话「demo.ts 故意不进模板桶导出,本预览页是它唯一合法的消费者,按路径显式引入以保持『假数据只从演示入口注入』的单向依赖」;不发任何 HTTP 请求;在 page-registry `demoIds` 单列一档、导航单列「演示」区。**它是 UI 模板层先例,不是数据路径先例。**
- **compose.yaml**:文件头自我声明「只为 PostgreSQL 集成门禁准备的一次性数据库,不是部署描述符」;tmpfs 不留残留;「库里只允许出现命名夹具」。**它是测试门禁先例,不是隔离演示环境定义。**
- **SYN 合成种子**(cmd/parcel-dispatch/synthetic_v0_test.go、syn_pc_seed_test.go):隔离 S 夹具只以 `go test` 存在;注释原话「S 替身只出现在本测试文件,名字带 synS;生产 assemble.go 不种服务产品、不默认适用性、不 INSERT 网络定义」。
- **internal/platform/pgtest**:环境差异的既有处理方式是「无 DSN 诚实跳过、CI 缺 DSN 失败」——即**跳过/失败,不是换装配**。
- **登记 CLI 与票 08 的「隔离环境」措辞**(各 main.go 包注释、票 08 纪律节)均为纪律语(「验证用脱敏合成值(S 级只记 S)」「明确标注仅限隔离环境」),无代码级环境分支。

## 四、可选路径清单(逐项代价与风险;不排序、不推荐)

### a) 在隔离环境按 S 级登记合成接入渠道(把票 04 v2 的设想提前用于读面)

- **动哪些文件**:接入身份能力按 ADR-0072 Decision 一落 `internal/accessidentity/`(目前不存在,从零开工),或退而在五上下文 `adapters/http` 各立真 Intake 实现;`cmd/parcel-api` 装配点逐端点替换(ADR-0055/0072 预留的替换缝)。
- **是否需要小 ADR**:按 AGENTS.md「难逆转技术或产品取舍→新 ADR」——**大概率要**。它顶着两篇已接受 ADR 的措辞:ADR-0072 Decision 二「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据到位前不立」,ADR-0055 Alternatives 已否决「为接入渠道建运行时登记表」(理由:替它拟表就是替租户拟 `PAR-INT-01` 的样子)。合成渠道若自拟凭据形状,正落在这两句的覆盖面里。
- **与红线的关系**:「证据层级诚实」可靠 S 级登记+如实标注守住;「不写生产默认值」的压力点在凭据形态自拟。若同时覆盖写路径,按 ADR-0055 Decision 五原文两项未决「必须先行或同批」——载荷规范化摘要已被 ADR-0072 Decision 三解耦为「独立成票现在就做」,但准入范围装配(`PAR-GOV-03..07` 全待提供)无解耦记录;若只覆盖读路径,原文无句可引,豁免要一次显式裁决(见一)。

### b) 装配层新增「隔离环境读面准入」形态(仅目录读口)

- **动哪些文件**:五上下文 `adapters/http` 各加一个生产代码里的注入式放行 Intake(作用域由装配输入给定,不读请求——形状即测试替身 `grantedCatalogueIntake` 的生产化);`cmd/parcel-api` 装配点对六个 GET 目录行按某种显式输入(环境变量/独立 main/构建标记,三者仓内都无先例,见三)选择 Intake;前端零改动。
- **是否需要小 ADR**:**要**。两条理由:它在生产代码里立**第一个非未配置 Intake**;它立**第一例按环境切换装配**——本仓迄今用「跳过/失败」处理环境差异,从未用「换装配」。
- **与红线的关系**:「不采信自报身份」可守(作用域来自装配注入,不来自请求);压力点有二:①ADR-0022「禁止在两项未决前落地任何默认实现」是比 ADR-0077「禁『开发用』采信实现」更宽的句子,注入式不采信自报、但算不算「默认实现」,原文没有直接答案,涵盖与否即裁决;②ADR-0055 否决「开发用采信头部 Intake」时的后半句理由——「事后没有任何东西能把这些信封与真实认证结果区分开」——对任何放行实现都成立,需要结构性答复(如何让该形态进不了生产装配、其铸出的作用域如何自带 S 标记,属设计点,本文不展开)。

### c) 数据态验证不走页面:库内读回取证,页面数据态延至真实渠道登记后

- **动哪些文件**:零代码。只改票 07/08 的完成标准措辞(「手验数据态」改为 CLI 灌入后以 SQL/登记口读回或 `go test` 断言;页面手验只验未配置态)。
- **是否需要小 ADR**:机制上不要(不改任何代码取舍);但**产品口径要连改三处**——票 04 v1「页面负责如实展示(数据)」实质缩水为「页面如实展示未配置态」;PRODUCT-STORY「今天能演示什么」第四条「事实链由 CLI 灌入,页面全链可看」不再成立,按该文「发现冲突应当场修本文」自我条款要改;ADR-0077 Consequences「隔离合成 S 环境里……页面可见合成数据」句同样落空(它已与代码现状相顶,见一末段,任何路径都得处理这句,c 路径下它成为长期不成立)。
- **与红线的关系**:全部红线零压力;代价全在「产品就绪 = 可演示」的演示深度上——管理台在真实渠道登记前永远只演示未配置态。

### d) 取证中见到、为完整性列出的其它路径(均已有明文否决记录)

- **d1 等 `PAR-INT-01`/accessidentity 一起做**:ADR-0055 与 ADR-0077 的 Alternatives 各否决过一次,理由同款——「租户数为零时那是一件不会到来的事」。
- **d2 前端 mock 演示数据**:ADR-0077 Alternatives 明文否决——「mock 连 S 都不是,它没有经过任何真机制」;UnwiredModule 的设计立场同源(「看起来能用」误当「已交付」)。

---

**裁决归属提示**(引 AGENTS.md 冲突序,不代裁):a/b 两路触碰 ADR-0022/0055/0072/0077 四篇已接受记录的措辞空间,按「改难逆转技术或产品取舍→新 ADR 或 supersede;不改写已接受 ADR 历史」办;c 路触碰票面与 PRODUCT-STORY/ADR-0077 的既有声称。无论选哪路,ADR-0077 Consequences 那句「页面可见合成数据」与代码现状的张力都需要一次显式处置(兑现它、改写它、或 supersede 它)。
