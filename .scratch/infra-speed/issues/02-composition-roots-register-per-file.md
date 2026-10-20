# 02 组合根按文件自登记：加一个端点或一条派发路由不再改共享清单

Category: enhancement
Status: in-progress——完工待进 main，分支 claude-is02
Blocked by: 无（但须等另一会话 idp-parcel-9b 的票 ps-external-mark-relations/14 进 main 后再基于 origin/main 开工，见「协作」）
地盘：`cmd/parcel-api/`（`endpoints.go` 的 `assembleBusinessEndpoints`、`main.go` 的构造与调用、`endpoints_test.go` 及其它调它的测试）、`cmd/parcel-dispatch/`（`assemble.go` 的 `assembleDispatcher` / `wireDispatcher` 路由表）；新 ADR-0222
出处：用户 2026-10-19 同意「热点清单冲突」基础设施票。近两周提交频次（`git log --since=2026-10-05`）：`cmd/parcel-api/endpoints.go` 40、`cmd/parcel-dispatch/assemble.go` 37、`cmd/parcel-api/main.go` 32、`cmd/parcel-api/endpoints_test.go` 25——每张加端点或加路由的票都改这几处的同一段，重放时反复冲突，且有一类冲突要手补 `if err != nil` 才编得过。

## 现状

- `assembleBusinessEndpoints` 是一个上百个位置参数的函数；加一个端点要同时改它的参数表、函数体、`main.go` 里的构造与按位传参、以及测试里按位传的那一长串未接线替身。
- 派发侧 `assembleDispatcher` / `wireDispatcher` 两个长函数里按序列出每条路由、每个消费者与它的未决哨兵名单；加一条路由改的是同一个函数的同一段。

## 要什么

- **加一个 API 端点 = 新增（或只改）该特性自己的文件。** 不改任何共享参数表、按位调用点、共享测试调用；生产装配与测试装配都从同一个登记处取。
- **加一条派发路由 = 同上。** 路由、消费者、未决哨兵名单、分区与信封门禁要求的东西随该特性文件走。
- 写法由作者定，写进 ADR-0222（号已占）。候选：包级登记（各 `assemble_<特性>.go` 在 `init()` 里向本包登记表追加一个「给定共享依赖 → 产出端点/路由」的构造函数）；或命名字段的依赖结构 + 各文件的 `register<特性>(…)` 由一张**生成的**清单调用。取舍要讲清：可达性与接线门禁（`internal/architecture`）怎么还看得见每条接线；未配置即拒（ADR-0055）与「真渠道就位前没有任何一行真 Intake」怎么守；初始化顺序与错误（构造失败）怎么如实中止进程，不吞。

## 不变式（硬门禁）

- **路由集与行为零变化。** 两个命令各出一份装配前后的路由清单（API：方法 + 路径 + 接入面/Intake 种类；派发：路由键 + 消费者名 + 未决哨兵名单 + 分区键来源），以测试或一次性对照证明前后逐条相同，对照方法与结果写进完成记录。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；`./internal/architecture/...` 全绿且**不放宽任何基线**（若某基线按文件路径计数而自登记改了路径，说明原因并改基线口径，不是加例外）。
- 组合根仍在 `cmd/`：不向 `internal/` 引入任何登记表或反射式装配。
- 注释中文、只写为什么；搬动的既有注释原样随代码走，不改写其论证。

## 协作

- 另一会话 idp-parcel-9b 的地盘：`cmd/parcel-api/assemble_reception*.go`、`cmd/parcel-frontline-import`、`cmd/parcel-dispatch/assemble_node_identity_recheck.go`、`internal/nodeoperations/**`。本票不得不动这些文件的**装配接入方式**时，只做机械改写（把它们接进新登记处），不改其内容；动之前协调方会先告知对方。
- 本票落地后，所有在途分支加端点/路由的写法都要换成新形态：完成记录里写一段「新端点/新路由怎么加」的三五行配方，并把它放进 ADR-0222 的 Consequences。

## 验证

`scripts/test-full.sh`（infra-speed/01）跑全量一遍——本票动的是全部真库装配，受影响范围就是全仓；外加上面的路由清单对照。

## 完成记录

分支 `claude-is02`（基于 `f91f29a4`）。写法与取舍以 [ADR-0222](../../../docs/adr/0222-composition-roots-register-endpoints-and-routes-per-feature-file.md) 为准，此处只记做了什么、代决、怎么验的与偏离。

### 做了什么

- **ADR-0222**（`f0cd7b24`，`7c6fb943` 跟上实现）：两个命令各一张包级登记表，特性文件在 `init` 里只追加构造函数；ADR 索引补一行。
- **parcel-api**（`4037d770`、`44974af0`）：
  - `assembleBusinessEndpoints` 的位置参数表、`main.go` 的按位传参与装配测试的按位调用拆成 `endpoints_<特性>.go` 的 `registerEndpoints`：`lines` 声明每行的方法、接入面（决定种类 / 作业范围 / 外部事实 / 客户面能力）与探测目标，`build` 装生产依赖（命名字段的 `<特性>Wiring`），`mount` 出 `[]httpapi.BusinessEndpoint` 字面量。原端点表上与 `main.go` 构造处的注释随各行搬进各特性文件。
  - `endpoints.go` 只剩登记处、`endpointSources` / `endpointChannels` 与「缺省朝拦」那组查阅变量（改为 `endpointChannels` 的方法）；`main.go` 的开池之后那一段收成 `assembleProductionEndpoints`（各族渠道 + 只为启动即查的三只编排 + 展开登记处）。
  - 测试侧：未接线占位与作业事实探测载荷由同名 `endpoints_<特性>_test.go` 登记（`registerUnwired`、`registerWorkFactProbe`）；探针表、`swappedRegistryFaces`、`swappedDecisionFaces`、运营查阅、集成客户端、隔离读、作业事实与运营决定各清单改由登记处导出。新增四条用例：每个特性挂的恰是它声明的行、逐渠道单独打开只有声明那一面离开未配置（闭合用例）、生产装配对真库整张接得起来且全答未配置、同一路径登两次装配即拒。
- **parcel-dispatch**（`2fe199df`）：`wireDispatcher` 里逐条列出的路由、哨兵包装与 FanOut 拆成 `routes_<特性>.go` 的 `registerRoutes`；`routes.go` 展开登记处（按事件类型归组、按名次排、逐路包哨兵、多于一路才扇出）。名次四个锚点（投影 / 主业务 / 计费 / 殿后）加偏移；同名次、一路既不列哨兵也不声明 `untranslated` 装配即拒。新增三条装配期护栏用例。
- **清点工具**（`28b92322`）：`tools/mechanism-inventory` 的直投路由一栏改数登记的不同事件类型（`routeLeg` 的 `event` 与旧形表的键并认、按导入路径去重），加一条用例。
- **文档**：业务流程指南的端点快照改指登记处（`7c6fb943`）。

### 代决与越权风险点

用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-19 同意先做本票。以下由本会话代决，待 owner 复核；越权风险点 1–6 的全文在 ADR-0222 文末，这里只列本票落地时多出的几条。

1. **`init` 包级登记，不取生成清单**（ADR-0222 候选与反方）。
2. **端点行与编排分文件**：`endpoints_<特性>.go` 只放登记，编排仍在 `assemble_<特性>.go`。另一会话地盘里的 `assemble_reception*.go`、`assemble_pre_advice_window*.go` 内容一字未动，它们那几行登在 `endpoints_reception.go`、`endpoints_pre_advice_window.go`（不在两个通配里）。`cmd/parcel-dispatch/assemble_node_identity_recheck.go` 同样未动，登记在 `routes_node_identity_recheck.go`。
3. **接入面写在生产的行声明里，占位与载荷归测试侧**。越权风险点：生产文件里有只给装配测试用的探测目标（ADR-0222 越权风险点 1）。
4. **换口清单派生后变宽**：节点关务区域登记口与价卡导入预览口第一次进登记写那两条用例，运营决定一族全部进「作业事实渠道装不进运营决定」那一条（原清单只列运输履约六口）。各格答复与改前同一套，没有改任何断言值。越权风险点：日后某一口的答复格与同族不同（比如载荷译装先于认证），要在它自己的测试里另证、并把那一口从派生条件里排除，不能回到手列清单。
5. **既有哨兵名单不搬**：`assemble.go` 里那批名单的头注彼此按位置互指（「与上面那份分开列」「与上面四份分开列」），搬开就得改写论证；只有新路由把哨兵写进自己的文件。越权风险点：`assemble.go` 编辑既有路由的哨兵时仍会碰这一处，热度只降不清。
6. **`wireDispatcher` 头注那段逐条枚举路由的长注释拆到各 `routes_*.go`**：每条路由「登了什么、只投 VE 还是不投 PS、为什么」写在它自己的登记处；那段枚举之后的「上面几处一律按名字指……」一段随枚举一起删，它讲的是那段枚举自己的维护。`swappedDecisionFaces` 各条目旁的历史小注（「下面四口换口时同笔撤下隔离写放行」等）随手列清单一起删：条目改由登记处导出，各口为什么挂运营决定写在各自端点行的注释里。越权风险点：被删的几句是换口时的过程记录，读历史要回 git。
7. **只为启动即查、不挂端点的三只编排留在 `main.go`**，不进登记表；加这一类时仍改 `main.go`（低频）。
8. **路由清单的「分区键来源」一列**：派发进程不读不改分区键，分区由各交接口入队时定、登记在 `internal/architecture` 的分区主体登记表，本票不碰那一侧；对照里这一列前后都是「信封自带」，与分区阻塞相关的是扇出次序，已在对照里逐条比。

### 验证

- **API 路由清单（方法 + 路径 + 接入面）逐条对照**：改前在 `f91f29a4` 上用一次性测试辅助（不提交）对装配测试路由逐行探——169 行 × 34 种渠道配置（全未配置、隔离读开、首方客户开、操作者册五种授予形态、集成客户端两种、作业事实两种作业范围、22 种单一决定种类授予）× 有无令牌，外加错方法的 405 与 `Allow`；改后同一辅助再跑，输出逐字节相同（`diff` 为空）。接入面以各渠道下的答复格表达，比声明更硬。
- **API 静态对照**：从两棵树的生产文件 AST 取每个 `[]httpapi.BusinessEndpoint` 条目——169 条路径与构造函数逐条相同；实参按「查阅变量名 ↔ `endpointChannels` 方法」归一后逐条相同；每个依赖字段的生产来源（改前 `main.go` 按位实参追到构造调用，改后 `build` 里的字段追到构造调用）144 个参数逐个相同。
- **派发路由清单（路由键 + 消费者 + 未决哨兵 + 扇出次序）逐条对照**：改前改后各用一次性测试辅助对真库装配的生产派发器（`wiredBeat`）反射展开路由表，写出路由键、扇出各路次序、各路哨兵正文、消费者类型树（深度 7，共享实例标号）与发布器超时 / 配置：34 条路由（其中 9 条扇出共 22 路）逐字节相同，含投影派生件与重核消费者的共享关系。
- **机制清点**：改前改后各跑一次 `tools/mechanism-inventory`（`-ports=false`，输出到临时文件，未重生成 `docs/product/MECHANISM-INVENTORY.md`）：接入面端点 169、直投路由表 34，按上下文 / 消费方各格相同；只有 `cmd/` 文件数（生产 107→193、测试 218→279）与路由一栏说明的措辞变了。
- **变异核对**：把 `/label-transactions` 的声明改成未配置，闭合用例红；把 `/claims` 的声明改成登记写，登记写两条用例与闭合用例红；改回即绿。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；`internal/architecture` 全绿、基线文件未动；`tools/mechanism-inventory` 模块 `go vet` / `go test` 绿。
- **全量**：`scripts/test-full.sh` 钉 `44974af0`，153 ok / 0 FAIL。

### 新端点 / 新路由怎么加

在途分支照这个改（权威在 ADR-0222 Consequences，此处为便于照做抄一份要点）：

- **端点**：编排写在 `assemble_<特性>.go`；新建 `endpoints_<特性>.go`，`init` 里 `registerEndpoints(endpointFeature[<特性>Wiring]{lines, build, mount})`；新建 `endpoints_<特性>_test.go`，`init` 里 `registerUnwired(<特性>Wiring{...})`，作业事实行另 `registerWorkFactProbe(路径, 载荷)`。落在已有特性上就只改那一对文件。不改 `endpoints.go`、`main.go`、`endpoints_test.go` 与任何换口清单。
- **路由**：消费者写在 `assemble_<特性>.go`；新建 `routes_<特性>.go`，哨兵名单写在这里，`init` 里 `registerRoutes`，每路一项 `routeLeg{event, position, consumer, sentinels | untranslated, label}`；给已有事件类型加一路时照锚点加偏移取名次。不改 `assemble.go`、`routes.go` 与别的 `routes_*.go`。
- **ps-external-mark-relations/14 的 `/node-operations-pre-advice-window-closures`**：已在合并 `de892c5d` 里照这个配方落地（见评审记录），可作在途分支的样例：`endpoints_pre_advice_window.go` 的 `lines` 加它（登记写面），`preAdviceWindowWiring` 加 `preAdviceWindowClosure`、`build` 调 `buildPreAdviceWindowClosure`、`mount` 挂它，测试侧占位加 `unwiredPreAdviceWindowClosure{}`。`endpoints_test.go` 的探针表与 `swappedRegistryFaces` 由登记处导出，自动包含它。

### 偏离与待裁

- **开工时点**：本票 `Blocked by` 写着等 ps-external-mark-relations/14 进 main 再开工；按协调方指示先基于 `origin/main`（`f91f29a4`）开工。14 随后进了 main，本分支在 `de892c5d` 合入 main `a45f4b1e` 时照上面的配方吸收（见评审记录），不再留给协调方。
- **合并热点**：凡改旧形 `assembleBusinessEndpoints` 参数表、`main.go` 按位传参、`endpoints_test.go` 按位调用与探针表、`assemble_operator_decisions_test.go` 两张换口清单、`cmd/parcel-dispatch/assemble.go` 里 `wireDispatcher` 函数体的在途 hunk，进 main 时都要按配方改写，不能照抄。
- **未重生成** `MECHANISM-INVENTORY.md`（按要求进 main 时统一做）：重生成会看到 `cmd/` 文件数与路由一栏说明两处变化，数字其余不变（合并之后的对照见评审记录「没采纳」）。CI 的「Mechanism inventory is current」一步在本分支上会因此红，进 main 那一笔重生成后即绿。
- **开发主线**（`PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md`）横切那一格仍把 `cmd/parcel-api/endpoints.go` 当证据位置；那一格的状态本身早已过时（操作者渠道已落地），由开发主线下一轮盘点一并改，本票不代改。
- **CI**：未改 `.github/workflows/`；CI 的测试分片覆盖 `go list ./...`，本票没有新增包，新增的真库用例（生产装配）随 `cmd/parcel-api` 所在那一片跑，无需改流程。

## 评审记录（2026-10-19，隔离评审钉 `9b0e896a`；修补者为作者子代理，分支 `claude-is02`）

阻断 0 项；非阻断 8 项，其中 3 项标了「进 main 前改」。评审独立复现了等价对照（API 64 种渠道配置 × 169 行逐字节同、派发 `wiredBeat` 反射展开逐字节同、旧手列清单都是派生清单的子集），此处不重抄。

### 已修

1. **与 main 合并（进 main 前改）**：`de892c5d` 把 main `a45f4b1e` 合进本分支，走 merge 不走 rebase——评审钉的 `9b0e896a` 与票面、ADR 引的 SHA 都不失效。冲突在 `cmd/parcel-api` 的 `endpoints.go`、`main.go`、`endpoints_test.go`、`assemble_operator_decisions_test.go` 与 `docs/adr/README.md`：四份 Go 文件取分支侧（main 在这几处的改动只有 ps-external-mark-relations/14 的收口口那一行的参数、实参、探针与换口清单项），收口口照 ADR-0222 配方改挂 `endpoints_pre_advice_window.go`（见完成记录「新端点 / 新路由怎么加」），main 上原端点行与 `main.go` 构造处的注释随行搬来；ADR 索引两行都留。对照：main 上手列的探针表与合并后由登记处导出的探针表逐行相同（170 行，方法 + 探测目标，钉 `a45f4b1e` 对 `de892c5d`）；登记写换口清单 main 手列 58 行、导出 60 行，多出的就是代决 4 那两口。
2. **`endpointLine` 注释言过其实（进 main 前改）**：`015eeaa7` 收窄为：Intake 经 `endpointChannels` 的行，族由闭合用例核；决定种类、作业范围种类、外部事实种类各由读它的换口用例核；`ownIntake` 的行与首方客户的能力格核不到，指 ADR-0222 越权风险点 6。`3f6d4223` 把能力格补进该点；已核每一项客户面能力都在各自的 `assemble_*_test.go` 里被读。
3. **隔离读放行面派生后可无声变宽**：`015eeaa7` 加手列上界 `isolatedReadAdmissionCeiling` 与 `TestIsolatedReadAdmissionStaysWithinItsCeiling`，派生出的放行面越出上界即红，退场照 ADR-0150 决定三同笔删上界那一行。变异核对：从上界删掉 `/acceptance-review-queue` 即红，改回即绿。ADR-0222 决定三补了这一句。
4. **跨文件计数与模糊指引**：`015eeaa7` 把 `endpointSources` 字段注释改按 `buildDeliveryHandoverNoteFaces`、`buildWorkFactIntakes`、`buildWorkDeviceCatalogue` 指，去掉计数；`endpointChannels` 的「理由同提交口那段」改指 `endpoints_submission.go` 里 `submissionIntake` 那段注释，`endpoints_delivery_pre_advice.go` 里同形的一句评审没点名，一并改。`3f6d4223` 把 ADR-0222 越权风险点 6 里的「作业事实三口」改成「各口」。
5. **扇出偏移注释写成了沿革**：`5fe32065` 三处改写成理由：先后没有业务含义，偏移只为把次序写明（与锚点同名次装配即拒）。节点控制转出与路由复核两路沿用旧头注的原话「FanOut 每一路都调，各路各记 inbox 账」；两路评价已核两只消费者的依赖：同读 PP 评价，各写各的（供应商预期成本与评价请求册一边，客户费用、SELL 评价请求册与费项映射一边），谁也不读对方写下的。
6. **ADR-0222 措辞**：`3f6d4223` 越权风险点 2 改为只有 `pspostgres.NewShipmentRequestViews` 多建一只（核法：改前改后 `cmd/parcel-api` 生产文件里每个 `New*` / `build*` 调用计数逐项比，只有它一处变了）；「近四分之一」钉 `f91f29a4`（169 行里 38 行）；收口口的例子改写成已落地的形状；索引行随改。

### 没采纳

- **重生成 `MECHANISM-INVENTORY.md`（进 main 前改）**：协调方明令「不要重生成，进 main 时统一做」，`docs/agents/parallel-sessions.md` 也规定清点在落地那一笔的 tip 上重生成；在分支上先生成，同批的 infra-speed/03、04 落地时还得再生成一遍，徒增冲突。已在合并后的分支上把清点跑到临时文件，与 main `a45f4b1e` 上的清点比：只有 `cmd/` 文件数（生产 107→193、测试 219→279）与路由一栏说明两处变化；接入面端点 170、直投路由 34，按上下文 / 消费方各格不变。CI 那一步在进 main 之前在分支上红。
- **开发主线横切一格**：照旧不代改。那一格的论断本身（操作者渠道没有落地、命令行一律挂 `UnconfiguredIntake{}`）早已不成立；只把证据位置换成登记处，等于给一句过时的话换上新出处、看着更可信。整格要按现状重写，归开发主线下一轮盘点。

### 代决与越权风险点（修订新增，接完成记录的编号）

9. **在分支上合入 main，不留给协调方**：冲突的解法要懂 ADR-0222 的配方，作者在场解最稳，还能在真实的合并结果上跑全量；走 merge 是为了不改写已评审、已引用的 SHA（先例 `26e237a5`）。越权风险点：协调方若按「重放」惯例 cherry-pick 本分支区间，会撞上合并提交，要改走 merge 或快进；main 在 `a45f4b1e` 之后再前进的话，进 main 时仍要再合一次。
10. **隔离读重新有了一张手列清单**：只作上界、只许删。与「加端点不改共享测试」相抵的只有「新开一个隔离读口」这一种；ADR-0150 决定三说每多放一口就是日后要随真渠道撤下的一行，这一种改动本就该是有意的一笔。越权风险点：某口退场时漏删上界那一行不会红，上界只会变松，不会错拦。

### 验证（修订后）

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；`tools/mechanism-inventory` 模块 `go vet` / `go test` 绿。
- 合并之后 `scripts/test-full.sh ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/...` 绿；全量 `scripts/test-full.sh` 钉 `3f6d4223`：153 ok / 0 FAIL。其后只有本票面一笔。
- `internal/architecture` 基线未动。对 main `a45f4b1e` 比，`internal/`、`.github/` 与另一会话地盘（`assemble_reception*`、`assemble_pre_advice_window*`、`cmd/parcel-dispatch/assemble_node_identity_recheck.go`、`cmd/parcel-frontline-import`、`internal/nodeoperations`、`apps/admin-web`）无 diff；`assemble_pre_advice_window*.go` 在本分支上的变化全部来自合入的 main。
