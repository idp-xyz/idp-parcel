# ADR-0222：组合根按特性文件自登记——parcel-api 的端点行与 parcel-dispatch 的路由各由特性文件在 init 里登进本包登记处，生产与装配测试取同一份

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，并于 2026-10-19 同意先做两张基础设施票（全量提速、热点清单去冲突）；票 infra-speed/02。越权风险点见文末，待 owner 复核。）
Date: 2026-10-19

## Context

- 两个命令的组合根各有一处「每张票都改同一段」的共享清单：
  - `cmd/parcel-api`：`assembleBusinessEndpoints` 收上百个位置参数。加一个端点要同时改它的参数表与函数体、`main.go` 的构造与按位传参、装配测试按位传的那一长串未接线占位、探针表 `businessEndpointProbes`；挂操作者渠道的端点还要改 `swappedRegistryFaces`、`swappedDecisionFaces` 一类换口清单。
  - `cmd/parcel-dispatch`：`wireDispatcher` 一个函数里按序列出每条路由、每个消费者、它的未决哨兵包装与扇出次序；未决哨兵名单也排在同一个文件里。加一条路由或给已有扇出加一路，改的是同一个函数的同一段。
- 频次（`git log --since=2026-10-05`，实测于 `f91f29a4`；数本身是论点）：`cmd/parcel-api/endpoints.go` 40、`cmd/parcel-dispatch/assemble.go` 37、`cmd/parcel-api/main.go` 32、`cmd/parcel-api/endpoints_test.go` 25、`cmd/parcel-api/assemble_operator_decisions_test.go` 23。
- 冲突的形态：两条在途分支各往参数表末尾加一参、各往按位调用末尾加一实参，重放时几处一起冲突；`main.go` 那一处还常要手补 `if err != nil` 才编得过。按位传参另有一种不冲突的错：同型的两个实参换了位置，编译照绿。
- 约束：
  - 组合根在 `cmd/`。`internal/` 不进任何登记表或反射式装配；两个命令各是一个 `package main`，不共用登记代码。
  - [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：每个端点以未配置即拒起步，真渠道就位前没有一行真 Intake；读写两道隔离开关各换各的行（[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 决定四、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三）。
  - 门禁与清点按语法认接线：`internal/architecture` 的管理台端点消费者门禁认 `[]httpapi.BusinessEndpoint` 字面量里的 `Pattern`；`tools/mechanism-inventory` 按同一字面量数端点、按构造函数所在包归属，按 `map[eventing.EventType]dispatch.Consumer` 字面量的键数路由；接线与类型可达性两道棘轮数 `cmd/` 生产代码里的引用。
  - 部署坏了要在开池之后、起服务之前带原因退出（`openDatabase` 与 `assembleDispatcher` 头注），构造失败不得吞。

## Decision

**一、每个命令一张包级登记表，由特性文件在 `init` 里追加构造函数。**（机制）

- `init` 只追加闭包：不碰库、不读环境、不构造任何东西，也就不会失败。构造在开池之后由装配点逐项调用。
- parcel-api：`registerEndpoints(endpointFeature[D]{lines, build, unwired, mount})`。
  - `lines`：本特性每一行的方法、接入面（决定三）与装配测试的探测目标。
  - `build`：从生产共享件（`endpointSources`：库、隔离写开关、本产品权威串、各族核验方与铸造器、操作者册、设备册、准入范围）装本特性的编排与读口，交回命名字段的依赖结构 `D`。
  - `unwired`：交回同形的未接线占位，装配测试用。
  - `mount`：拿各特性共用的接入渠道（`endpointChannels`：隔离读、首方提交、操作者运营决定与登记写、集成客户端、作业事实两只）与 `D`，交回 `[]httpapi.BusinessEndpoint` 字面量。
  - 生产：`assembleBusinessEndpoints(sources, channels)` 逐特性 `build` 再 `mount`；装配测试：同一张表逐特性 `unwired` 再 `mount`。两边走的是同一个 `mount`，生产与测试不再各列一份端点表。
- parcel-dispatch：`registerRoutes(func(dispatchSources) ([]routeLeg, error))`。
  - `routeLeg`：事件类型、扇出名次（决定四）、本路在路由条目处翻译的未决哨兵、消费者。
  - `dispatchSources`：库、outbox 与 inbox 两只存储、部署形态参数、时钟，以及 VE 各投影共用的那一只租户绑定派生。
  - `wireDispatcher` 逐特性构造，按事件类型归组、按名次排、逐路包哨兵，一路的直接进路由表，多于一路才 `FanOut`，再交 `NewDirectPublisher`。

**二、文件约定。**（机制）

- parcel-api：一个特性的端点行（连同原先写在端点表与 `main.go` 构造处的注释）在 `endpoints_<特性>.go`；编排的装配仍在 `assemble_<特性>.go`。`endpoints.go` 只剩登记处、共享渠道与查阅变量组；`main.go` 只剩部署参数、各族渠道的构造与装配调用。
- parcel-dispatch：一条路由的登记与本路的未决哨兵名单在 `routes_<特性>.go`；消费者的构造仍在 `assemble*.go`。`assemble.go` 只剩部署参数、装配点、登记处的展开与消费者构造。
- 特性按「一只编排或一只读口带出的那几行」切，不按上下文切。

**三、每行声明接入面；装配测试的各张清单由登记处导出。**（机制）

- 接入面按能打开这一行的那一族取值：未配置即拒、隔离读、首方客户（带客户面能力）、主数据与运营查阅读、登记册配置写、运营决定（带决定种类）、作业事实登记（带作业范围种类）、集成客户端（带外部事实种类）。另记一格：Intake 由本特性从生产共享件自建（作业设备册、送站预报、送站交接单、客户追踪视图等），装配测试里它恒为未配置。
- 探针表与各张换口清单（登记写、运营决定、运营查阅、集成客户端、隔离读、作业事实）改由登记处按接入面导出，不再手列。
- 声明与 `mount` 写的 Intake 可能不一致，所以另立一条闭合用例：逐个渠道单独打开，只有声明了那一面的行答复离开「接入渠道未配置」。声明错了，不是那条用例红，就是对应那一面的用例红。
- 作业事实各行要一份能越过载荷译装的探测载荷，那是测试数据，由同名的 `endpoints_<特性>_test.go` 在 `init` 里登记；登记处里有作业事实行而没有载荷，装配测试红。

**四、未配置即拒与「没有一行真 Intake」照旧成立。**（机制）

- 每行用哪只 Intake 仍写在 `mount` 那一行上。缺省是字面量 `UnconfiguredIntake{}`；查阅变量组（`endpointChannels` 的方法）缺省同样取未配置，渠道就位才换。首方提交口与作业事实各口在特性里各立一个变量、各一个 `if`，隔离开关换不了它们。
- 端点登记处本身就是 ADR-0055 说的那本空登记册：装配测试对登记处的每一行在全未配置下断言 403 `ACCESS_CHANNEL_NOT_CONFIGURED`。新特性一登记就进这条用例，不再靠有人记得往探针表加一行。

**五、门禁与清点怎么还看得见每条接线。**（机制）

- 端点：`mount` 里照旧写 `[]httpapi.BusinessEndpoint{{Pattern: "...", Handler: <上下文>http.NewXxxEndpoint(...)}}` 字面量。门禁与清点按字面量认、不认函数名与文件名，口径不变，数不变。
- 路由：路由键改写在 `routeLeg` 字面量的 `event` 字段上。清点工具改为数 `cmd/` 生产文件里登记的不同事件类型（`routeLeg` 的 `event` 与旧形字面量的键并认，同一事件类型的多路扇出算一条），数与改前相同。
- 两道棘轮：构造与引用都还在 `cmd/` 生产文件里，基线不动。事务闭包、信封 ID、信封分区与分区主体登记各门禁看的是 `internal/` 与测试代码，本记录不碰。

**六、构造顺序与失败。**（机制）

- parcel-api：开池之后先在 `main.go` 构造各族渠道，再按登记表逐特性 `build`。任一失败原样交回，`run` 返回错误，进程以非零码退出，与改前逐个 `if err != nil { return err }` 是同一条路。只为启动即查而装、不挂端点的那几只编排（面单渠道、请求评价、结算编排）留在 `main.go`，不进登记表。
- 同一路径登记两次：装配点答错误中止。改前路由层让后一个静默盖掉前一个，只有装配测试看得见。
- parcel-dispatch：同一事件类型两路名次相同，装配答错误中止——扇出次序不许由文件名定。
- 特性之间的构造顺序随文件名（Go 按编译时的文件次序跑 `init`）。端点表与路由表各自的排列因此与改前不同：路由层按路径匹配、不看次序；派发按事件类型取路由；同一类信封上各路的次序由名次定，与改前逐条相同。

**七、扇出名次。**（机制）

- 名次是整数，小者先调；四个锚点各带一句理由：
  - `positionProjection`：VE 投影与补派生先行，不堵在资格墙、终局规则墙或路由证据墙上。
  - `positionPrimary`：这一类信封的主业务那一路。
  - `positionBilling`：计费那一路，排在履约与追踪之后。
  - `positionLast`：殿后，TF 实际履约发生项（[ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)）。
- 同一锚点下要分先后时写成锚点加偏移，偏移的理由写在那一路的登记处。

三类：七条都是**机制**；没有产品策略与租户取值。

## 对已接受记录的作用

- 不停用任何条款。ADR-0055、0078、0085、0091、0100、0139、0140、0149、0150、0151 正文里的「装配点」「`assembleBusinessEndpoints`」「端点表」此后指端点登记处：`assembleBusinessEndpoints` 仍是生产装配的入口，展开的是登记表；「在装配点逐端点替换」的那一行现在在 `endpoints_<特性>.go`。
- ADR-0049 决定三「没有订阅者的事件类型显式失败并入账、接得住才登记」不变：只登记本进程接得住的类型，这一判断现在由加路由的那个特性文件做。

## 候选与反方

- **命名字段的依赖结构 + 由 `go generate` 生成的调用清单。** 否决。生成物本身是共享文件，两条分支各加一行仍冲突（虽可重生成解决）；多一个生成器，还要一道「生成物是否最新」的门禁。`init` 登记不需要任何生成物。
- **只把位置参数换成一个大的命名字段结构。** 否决。消掉了按位错位，但结构体与 `main.go` 的构造仍是每加一个端点都改的共享处，热点只换了个形状。
- **一个上下文一个登记文件。** 否决。运输履约一家就占端点表近四分之一，热点只是换了个文件。
- **反射式装配（按类型自动注入）。** 否决。组合根要读得出每条接线，反射把接线藏进运行期，门禁也就看不见了；本票明令不引入。
- **探针与接入面清单留在测试侧，按特性拆开。** 否决。「这一行挂哪一面」是生产事实，与 `mount` 写在同一个文件里才不会漂开；拆到测试侧仍要两处同步，还要给每个没有测试文件的特性新开一个。作业事实的探测载荷是测试数据，单独按特性留在测试侧（决定三）。
- **扇出次序按登记先后（文件名）定。** 否决。次序有业务含义（先 VE、计费在后、实际履约殿后），文件改名不该改次序。

## Consequences

- **新端点怎么加：**
  1. 编排装配写在 `assemble_<特性>.go`（`build<特性>(db)`）；要新的未接线占位类型时写在同一文件。
  2. 新建 `endpoints_<特性>.go`，`init` 里 `registerEndpoints(endpointFeature[D]{...})`：`lines` 每行一项（方法、接入面，带分派参数或路径参数时写探测目标）；`build` 调第一步；`unwired` 交占位；`mount` 写 `[]httpapi.BusinessEndpoint` 字面量，Intake 从 `channels` 取，或在本特性里照「缺省未配置、渠道就位才换」立变量。
  3. 新行落在已有特性上时只改那一个 `endpoints_<特性>.go`：`lines` 加一项、`mount` 加一行，`D` 要加字段时同文件改 `build` 与 `unwired`。
  4. 不改 `endpoints.go`、`main.go`、`endpoints_test.go` 与任何换口清单；作业事实行另在 `endpoints_<特性>_test.go` 登一份探测载荷。
  - 例：在途的 `/node-operations-pre-advice-window-closures` 落在 `endpoints_pre_advice_window.go`：`lines` 加 `{method: POST, face: registryWriteFace()}`，`mount` 加一行，`D` 加关闭编排的字段。登记写的换口用例自动包含它。
- **新路由怎么加：**
  1. 消费者构造写在 `assemble_<特性>.go`。
  2. 新建 `routes_<特性>.go`：本路的未决哨兵名单写在这里，`init` 里 `registerRoutes`，每路一项 `routeLeg{event, position, sentinels, consumer}`。
  3. 给已有事件类型加一路：照决定七取名次。两路同名次，装配即拒。
  4. 不改 `assemble.go` 的装配点与别的 `routes_*.go`。
- 在途分支：凡改 `assembleBusinessEndpoints` 参数表、`main.go` 按位传参、`endpoints_test.go` 按位调用、探针表、换口清单或 `wireDispatcher` 函数体的，进 main 前都要换成上面的形态。
- 生产装配第一次有了真库用例：`assembleProductionEndpoints` 在未配置环境下对真库装配整张表，断言每一行都在、都答未配置。改前生产装配只在进程启动时走得到。
- 仍是共享处、加端点不必改的：各族渠道的构造（`main.go`、`assemble_operator_decisions.go` 等）与 `endpointChannels`。只有新开一个接入渠道族，或给一个新上下文开操作者 Intake 时才改。
- 未接线占位留在 `unwired_orchestration.go`；它们现在由各特性登记的 `unwired` 引用，只在装配测试那条路上被调到。

## 越权风险点

1. **测试用的探测目标写在生产文件里**（parcel-api owner）。`lines` 的探测目标带 `SYN-` 合成标识（路径参数与分派参数），只给装配测试用，生产装配不读。好处是一行的声明只有一处；代价是生产文件里多一格测试才用的数据。
2. **无状态读适配器从共用一只改为各特性各建一只**（parcel-api owner）。委托查阅、商业目录、案件复核等读适配器改前在 `main.go` 建一只、按位传给几行；现在每个特性在自己的 `build` 里建。它们都只包着库、不带状态，答复相同；日后若哪只读适配器带缓存，各建一只就是各一份缓存。
3. **特性之间的构造顺序随文件名**（两个命令的 owner）。今天各 `build` 与消费者构造都只组装适配器、不改库，顺序不影响结果；日后若有构造依赖先后，要显式写进同一个特性，不能靠文件名。
4. **清点口径改了措辞**（产品基线 owner）。`tools/mechanism-inventory` 的路由一栏改为「登记的不同事件类型数」，数与改前相同；重生成 `MECHANISM-INVENTORY.md` 时那句说明会换措辞。
5. **第一次在本仓用 `init`**（两个命令的 owner）。只在两个 `package main` 里、只做追加，不读环境、不碰库；`internal/` 里照旧一个都没有。

## Links

- 票 `.scratch/infra-speed/issues/02-composition-roots-register-per-file.md`
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置即拒；端点登记处就是那本空登记册
- [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)：进程内直投；接得住才登记
- [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 决定四、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三：读写开关各换各的行
- [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)：实际履约发生项殿后
