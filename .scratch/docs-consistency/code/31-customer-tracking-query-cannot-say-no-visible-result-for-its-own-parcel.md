# 31 VE 客户追踪查询对本账户已归属、视图未形成的包裹答 `VIEW_NOT_FOUND`，与无权查询分不开

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code31（基于 main `d19eeaafc`）。此前 ready-for-agent——R2（上线后尽快）：会撞（委托接受后、初始路由投影形成前，客户拿首方提交答复里的包裹号 `parcels[].parcelId` 去查，或 NR 一直形不成投影时），但有可靠绕法：客户系统稍后重查（首方客户今天没有读委托接受状态的口，绕法只有重查或请租户客服核对）。首方 API 兼容性基线发布前落地最省（ADR-0246 越权风险点 5）。2026-10-21 文档自洽 04 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc04。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-05（归 B）；裁定见 [ADR-0246](../../../docs/adr/0246-the-customer-tracking-query-answers-no-visible-result-for-a-parcel-attributed-to-the-requesting-account-before-its-view-forms.md)。取证于 main `205eb254`。

## 现象

`internal/visibilityexception/adapters/http/query_customer_tracking_view.go` 的业务结果是封闭两格 `CURRENT_VIEW` / `VIEW_NOT_FOUND`，头注写「包裹不存在、不属于请求账户、或视图尚未形成，一律这一格」。端点只读客户视图库，读不到就答 `VIEW_NOT_FOUND`。归属方查自己所在委托已`已接受`、还没形成视图的包裹，拿到的答复与无权查询相同。

## 证据

- 文档：ADR-0246 决定一至四；UC-VE-008 结果语义「当前无可见结果」一行与 `AT-VE-188`（本票同批改写与新立）；ADR-0223 越权风险点 3。
- 代码：`NewQueryCustomerTrackingViewEndpoint`、`TrackingViewReader`；归属反查口 `ports.ParcelCustomerAccountView`（今天只接在视图派生上）。

## 三类

产品策略：三格答法（ADR-0246 决定一、五）。机制：读不到视图时问一次归属。租户取值：无。

## 规模

S–M：端点加一个依赖（归属反查口）与一格 `NO_VISIBLE_RESULT`；`cmd/parcel-api` 的装配接上反查口；首方契约描述加取值；补用例；演示文档两处同步（见修法第 6 条）。

## 最小的诚实修法

1. 先读视图；读到照旧答 `CURRENT_VIEW`。
2. 读不到视图，经 `ParcelCustomerAccountView.FindCustomerAccount` 问一次归属：答出唯一账户且等于查询键的账户 → `NO_VISIBLE_RESULT`；答零份、答别的账户、答多于一份的具名错误 → `VIEW_NOT_FOUND`。
3. 反查读不动 → 与视图读不回同一格（`NO_ANSWER_FORMED`），不降成任何业务格。
4. 头注「一律这一格」改为三格；`queryResponse` 的取值说明随之改。`ports.ParcelCustomerAccountView` 头注与 `adapters/parcelshipment/customer_account_lookup.go` 里「多于一个候选是未决/待确认」只管派生一侧（不择一、不形成视图）；查询一侧按 ADR-0246 决定二把同一具名错误答成 `VIEW_NOT_FOUND`，头注要随之分清两侧。
5. 用例：归属方查视图未形成的包裹答 `NO_VISIBLE_RESULT`；别的账户查同一件、查不存在的号都答 `VIEW_NOT_FOUND` 且响应体逐字相同；反查读不动答未形成。Covers 点 UC-VE-008 `AT-VE-188`、`AT-VE-151`。
6. 演示文档同步，与代码同一笔：
   - `scripts/demo-seeds/README.md` 客户追踪视图那一条「视图在委托接受、初始路由投影形成之后由 dispatch 派生，之前查它答 `VIEW_NOT_FOUND`」改为：委托接受之前答 `VIEW_NOT_FOUND`；接受之后、视图派生出来之前，本账户查它答 `NO_VISIBLE_RESULT`。
   - `docs/design/synthetic-demo-journey-script.md` 首方查询那张「请求 | 答复」表加一行「首方令牌，本账户的包裹所在委托已`已接受`、视图尚未形成 | `200 NO_VISIBLE_RESULT`」；「别的账户的包裹，或不存在的包裹」一行不变。

## 要裁的

无（ADR-0246 已裁）。

## 完成记录（2026-10-25，分支 claude-code31）

用户授权原话「你作为业务和系统专家自决」；下列「代决」由作者会话代 VE owner 作出，待 VE owner 复核（第 1 条同时涉及 ADR-0246 越权风险点 1 待 PS owner 会签的那一读）。上一位作者开工即被额度打断、未留改动，本记录从 main `d19eeaafc` 起从头做。

### 做了什么（照「最小的诚实修法」逐条）

1. 先读视图，读到照旧答 `CURRENT_VIEW`，不问归属（`NewQueryCustomerTrackingViewEndpoint`）。
2. 读不到视图，`outcomeWithoutView` 经 `ports.ParcelCustomerAccountView.FindCustomerAccount` 问一次归属：答出唯一账户且等于查询键的账户 → `NO_VISIBLE_RESULT`；答零份、答别的账户、答 `ErrAmbiguousCustomerAccount` → `VIEW_NOT_FOUND`。端点构造器加第三个依赖 `attributions ports.ParcelCustomerAccountView`。
3. 反查的其余错误 → `500 NO_ANSWER_FORMED`，与视图读不回同一格，不降成任何业务格。视图读不回时不问归属。
4. 头注：`query_customer_tracking_view.go` 的业务结果常量改写为三格及各自续办；`queryResponse` 写明三格、`view` 只随 `CURRENT_VIEW`；`ports.ParcelCustomerAccountView` 头注按零/一/多逐格分写派生一侧与查询一侧；`customer_account_lookup.go` 的 `ErrAmbiguousCustomerAccount` 头注分清派生一侧落未决、查询一侧答 `VIEW_NOT_FOUND`。
5. 用例（Covers 点 `AT-VE-188`、`AT-VE-151`，另点 `AT-VE-168` 的客户归属读取半边）：
   - `internal/visibilityexception/adapters/http`：`TestTheAttributedAccountGetsNoVisibleResultBeforeItsViewForms`（响应体逐字 `{"outcome":"NO_VISIBLE_RESULT"}`）；`TestEveryOtherUnformedViewIsByteForByteAMissingParcel`（归属别户、多份声明别人来查、多份声明声明方之一来查，都与不存在逐字同答）；`TestAnUnreadableAttributionIsNoAnswerNotABusinessGrade`；`TestEveryViewlessQueryAsksTheAttributionExactlyOnce`（归属方、别户、不存在、多份声明各问一次）。既有用例补断言：读到视图不问归属、视图读不回不问归属；未配置面替身加「被问即失败」的归属口。
   - `cmd/parcel-api`：`TestTheProductionTrackingViewLineWalksAParcelThroughItsThreeGradesAgainstARealDatabase`——真库，经登记处里本特性的生产 `build` 与 `mount`，两台首方客户端（`SYN-ACCOUNT-01`、`SYN-ACCOUNT-02`）；一件包裹的委托`已提交`时归属方答 `VIEW_NOT_FOUND`（与查不存在逐字同答），`已接受`、视图未形成时答 `NO_VISIBLE_RESULT`，视图形成后答 `CURRENT_VIEW`；三个阶段另一账户查同一个号都与查不存在逐字同答。既有真库用例改接生产反查口。
   - 装配：`endpoints_customer_tracking_view.go` 的依赖结构加 `attributions`，`build` 经 `assemble_customer_tracking_view.go` 新增的 `buildCustomerTrackingViewAttributions` 装 `veps.ParcelCustomerAccountLookup`（底下是 `pspostgres.ShipmentRequests`，与 parcel-dispatch 视图派生同一构造）；未接线占位 `unwiredCustomerAttributions` 在 `endpoints_customer_tracking_view_test.go`。没动 `endpoints.go`、`main.go`、`endpoints_test.go`、`routes.go`。
6. 演示文档同一笔：`scripts/demo-seeds/README.md` 第 9 条改为委托接受之前答 `VIEW_NOT_FOUND`、接受之后视图派生之前本账户答 `NO_VISIBLE_RESULT`；`docs/design/synthetic-demo-journey-script.md` 首方查询表加「首方令牌，本账户的包裹所在委托已`已接受`、视图尚未形成 | `200 NO_VISIBLE_RESULT`」一行，「别的账户的包裹，或不存在的包裹」一行不变。
7. 首方契约 `docs/api/first-party-api-contract.md` 客户追踪视图条目改为三格各自的续办，写明归属读不动答 `500 NO_ANSWER_FORMED`。
- 另：UC-VE-008 结果语义「当前无可见结果」与 `AT-VE-188` 的现状缺口注改为已补（照 code/05 的写法）；AT 盘点 `AT-VE-188` 由④移入①，`AT-VE-151`、`AT-VE-168` 两行补点名。VE CONTEXT 的规则句早已是现行规则，未改。无迁移、无新写者、无新路由。

### 首方契约兼容性判断（ADR-0140 决定五、六）

加 `NO_VISIBLE_RESULT` **不是破坏性变更**。

- 决定五本仓特有的一条：给某端点的 `outcome` 词表新增取值不算破坏，条件是契约文档写明客户端遇到未识别的 `outcome` 视为已形成答案、不可自动处置、交人。契约文档「版本与兼容性」节已写这一条。本票不删、不改名任何 `outcome` 取值，也不删、不改名字段。
- 有一处语义收窄要单说：原先落 `VIEW_NOT_FOUND` 的一部分查询（归属方查已接受、视图未形成的包裹）改落新格。不认识新格的客户端照契约交人，不会把它读成号错；需要「核对查询标识」的情形仍全部落在 `VIEW_NOT_FOUND`，那一格的续办没有变。code/22 落契约文档时已在客户追踪视图条目里写明这一格将由本票落，那份基线自己预告过它。
- 决定六（弃用）只管已发布端点或字段的移除，本票不涉。
- 基线成立的说法两处不一：code/22 进 main 记录写「首方契约首个兼容性基线自本笔起成立」，契约文档「版本与兼容性」节写「首方契约还没有对外发布过基线」（按决定五，基线是对外发布时归档的生成物，生成器今天未落，见 code/82）。两种读法下本票的判断相同，本票未改那一句。

### 代决

1. **多份委托同时声明，HTTP 适配器按 `adapters/parcelshipment` 的哨兵 `ErrAmbiguousCustomerAccount` 判，不把哨兵移进 `ports`。** 歧义哨兵的词汇本就住在译名包：同包的派生适配器、parcel-dispatch 的未决哨兵名单都据它分格。移进 `ports` 要改派生侧与装配的既有引用，超出票面。代价是 `adapters/http` 多一条到本上下文跨上下文译名包的导入；架构门禁只拦跨模块伸手，本模块内适配器互引不拦（`TestBusinessModulesDoNotReachIntoEachOther` 实跑通过）。
2. **反查答复的其余错误（读不动，以及 `ErrCustomerAccountUntranslatable`）一律答未形成，不降成 `VIEW_NOT_FOUND`。** 票面第 3 条「读不动」按「没形成归属结论」读。查询一侧入参译不动今天不可达（VE 与 PS 的租户、包裹标识构造同一判据：非空白），能触发的只有 PS 行上的账户是空白——那是数据缺陷，答 5xx 比静默落成「不归你」诚实。
3. **读次数**：读到视图不问；视图读不回不问（没有可依据的「读不到」）；每次读不到视图问且只问一次，不论答出什么——由 `TestEveryViewlessQueryAsksTheAttributionExactlyOnce` 与既有两条用例的补断言钉住（ADR-0246 决定二、三）。
4. **parcel-api 另建一只 `pspostgres.ShipmentRequests` 喂反查口**，不与提交口等别的特性共享实例：仓储无状态，共享只会让依赖图看不出各自要什么（与 parcel-dispatch 两路派生「各建各的」同一理由，也是 ADR-0222 越权风险点里「委托查阅读适配器多建一只」的同性质）。
5. **未接线占位答未接线错误（落未形成），不顶成零份**；放在特性测试文件，不进 `unwired_orchestration.go`（ADR-0222 决定一：占位是测试类型，不进生产文件）。
6. **演示动线同段一并改**：表里「首方令牌，本账户的包裹」一行改为「……视图已形成」，否则与新行同一场景两套结果；同行「带版本、投影锚与四维」删「投影锚」——code/05 已从响应体删去 `basedOnProjection`，这是残留的不实代码事实。表下段「委托接受、视图派生之前查它答 `VIEW_NOT_FOUND`」一句与新行矛盾，随改为两段时间各答各的。演示 README 第 9 条「本账户的包裹答 200 `CURRENT_VIEW`」同理补「视图已形成」。2026-10-19 两段实走记录是当时的实测，不改。
7. **契约文档写 `VIEW_NOT_FOUND` 的续办时，照 ADR-0246 决定五补一句**：刚从提交答复拿到、委托还没接受的号也落这一格，稍后再查或请租户客服核对。否则客户系统拿刚提交的号立刻查，会照「核对你给的号」去怀疑号错。

### 越权风险点

1. **查询多一次对 PS 的读**（ADR-0246 越权风险点 1，PS owner 会签仍待）：只在读不到视图时发生，经 PS 自己的仓储读口 `FindCurrentAcceptedByParcel` 读 `parcel_shipment.shipment_request`，parcel-api 进程从此读这张表。客户系统对新号高频轮询时，这一读与轮询同阶；本票没有加缓存，也不该加——加缓存就是第二份映射。
2. **新格对违约的客户端**：不认识 `NO_VISIBLE_RESULT` 而又不守「未识别 `outcome` 交人」的集成方，会在委托接受后、视图形成前的窗口里撞上它。契约文档已写那条义务；首发前没有真实集成方。
3. **`adapters/http` 导入跨上下文译名包**：将来若架构门禁收紧为「HTTP 适配器不得导入 `adapters/<provider>`」，要把歧义哨兵移进 `ports` 再改这里（代决 1 的代价）。
4. **时延差**（ADR-0246 越权风险点 3）：反查答「是你的」与答「不是你的」快慢可能不同，本票照 ADR 不加常数时间。读次数已相同。
5. **演示未实走**：新格只由真库用例证（`S`），没在 `seed.sh` 灌的演示租户上实走——要在委托接受后、视图派生前那几拍里查，或让 NR 形不成初始路由投影。README 与动线表的新行是按代码与用例写的。
6. **基线说法不一**：见上节末条。本票未改契约文档那一句，也未立票；契约生成器（code/82）落地、首个基线真正归档时一并统一为宜。

### AT

- `AT-VE-188`：④ → ①，点名四条 HTTP 用例与生产装配真库用例。`AT-VE-151` 补点两条，`AT-VE-168` 补客户归属读取半边。总览计数未改，由推送方重算。

### 验证（分支 claude-code31，提交 `010091c4e`）

- 先红后绿：新用例先对只加参数、不改行为的端点跑，`TestTheAttributedAccountGetsNoVisibleResultBeforeItsViewForms`、`TestAnUnreadableAttributionIsNoAnswerNotABusinessGrade`、`TestEveryViewlessQueryAsksTheAttributionExactlyOnce` 三条红；实现后绿。
- 变异（每条改后跑、跑完原样复原）：M1 别户也答 `NO_VISIBLE_RESULT`（比账户改成只看「有归属」）——HTTP 三条红，生产装配真库用例红；M2 读不动降成 `VIEW_NOT_FOUND`——红；M3 多份声明落未形成——红；M4 读不动降成 `NO_VISIBLE_RESULT`——红；M5 多份声明答 `NO_VISIBLE_RESULT`——红；M6 读到视图也问归属——三条红；M7 退回改前「读不到一律 `VIEW_NOT_FOUND`」——HTTP 三条红，生产装配真库用例红。七处全被杀。
- `go build ./...`、`go vet ./...` 干净，`gofmt -l internal cmd` 无输出。
- `TMPDIR=… scripts/test-full.sh ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./cmd/parcel-ve-register/ ./internal/visibilityexception/... ./internal/architecture/...`：19 个包 ok（`ports` 无测试文件，不计），退出码 0。范围是改动包与 `go list -test` 算出的反向依赖（`cmd/parcel-dispatch`、`cmd/parcel-ve-register` 与 VE 各子包）加 `./internal/architecture/...`。
- 新用例与两条既有探针用例 `-count=3`（真库）：全部 PASS，退出码 0。
- 无迁移，未灌种子。
