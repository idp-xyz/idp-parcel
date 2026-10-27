# 76 PS 取消先后冲突留在册上，却没有解开它的业务决定面

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-code76（见文末「完成记录」）。此前 needs-triage——建议 R2（上线后尽快）：看得见的卡住。冲突已经记进 `parcel_shipment.cancellation_order_conflict`、消费门入账，按包裹查得出；但没有任何口能解开它：网络服务的冲突包裹没有责任起点，也不触发收寄后的路由复核，实物照旧在节点手里；面单渠道服务的冲突包裹没有终局，客户手里是`包裹已取消`，承运商却已收走。取消授权在生产未配置，取消形成不了，冲突还到不了；租户一登记取消授权即生效。2026-10-21 自 [code/35](./35-cancellation-and-boundary-fact-order-conflict-has-no-label-half-and-no-resolution-face.md) 拆出（用户「你作为业务和系统专家自决」授权），分支 claude-code21。分级由协调方定稿。
Blocked by: 无（判法与冲突册已随 code/35 落地；决定的去向集合与授权形状要 PS owner 认可，见「要裁的」）
出处：[ADR-0254](../../../docs/adr/0254-cancellation-and-boundary-fact-order-is-judged-by-their-business-times-and-a-late-earlier-boundary-fact-is-a-fact-conflict.md) 决定三与越权风险点 2；code/35「最小的诚实修法」第 2 条。

## 为什么从 code/35 拆出

code/35 有两半：判法与入账（面单渠道一侧比时间、收寄采用消费门对冲突入账、冲突留下），以及解开冲突的决定面。前一半是机制，ADR-0254 已经把每一格定死，随 code/35 落地。后一半要先定一个 ADR-0254 有意没定的形状——谁有权、去向有哪几个、每个去向驱动什么重判——它是要 PS owner 认可的产品策略，体量本身就是一张 M 票：命令、授权读口、只追加的决定记录、续办信封，外加各条消费链的重判入口。两半捆在一票里，前一半要等后一半的认可才能进 main，而前一半不进 main，冲突会照旧回滚重投到失败预算耗尽、在库里不留痕。

## 现象

- 冲突册只追加、按包裹列（`CancellationOrderConflictRegister.ListByParcel`），没有任何编排读它。
- 冲突那一格的消费结论是入账：网络服务的采用答 `ELIGIBILITY_UNDECIDED`、原因 `CANCELLATION_ORDER_CONFLICT`；面单渠道服务的终局判断答 `LABEL_SERVICE_NOT_FINAL`、原因 `CANCELLATION_ORDER_CONFLICT`。同一来源再投仍落同一格，冲突册不翻倍。
- 已提交的取消照旧成立，`ParcelCancellationView` 照旧交出它；下游已收到释放意图。

## 证据

- 代码：`internal/parcelshipment/domain/cancellation_order.go` 的 `JudgeCancellationOrder`；`internal/parcelshipment/ports/cancellation_boundary.go` 的 `CancellationOrderConflictRegister`；`internal/parcelshipment/adapters/postgres/cancellation_order_conflict.go`；迁移 `migrations/parcel_shipment/0029_cancellation_order_conflict.sql`。
- 文档：UC-PS-003 `AT-PS-107`、UC-PS-004 `AT-PS-108`、UC-PS-006 决定矩阵「事实冲突」一行与 `AT-PS-081` 后一种，「给开发的交接」各指本票。

## 三类

机制：决定面的命令、只追加记录、续办信封与各条消费链的重判入口。产品策略：决定的去向集合、每个去向驱动的重判、谁有权（授权规则的形状）——本票提出、交 PS owner 认可。租户取值：谁持有这个授权，属 `PAR-COM-14` 一类，未登记答未配置。

## 规模

M：一个授权决定面（命令、授权、只追加的冲突解开决定记录、续办信封）；收寄采用、面单终局判断与有效交付的终局采用各认一次「这份冲突已由某决定解开」；冲突册加「已解开」的读法（另起一行回指冲突，不改写冲突行）。

## 最小的诚实修法（提案，待认可）

1. 决定挂在一行冲突上（租户，包裹，边界种类，边界那一代，取消），只追加，不改写冲突行，也不改写已提交的取消。
2. 去向至少两个：
   - **认定取消依据不成立，包裹转入收寄后服务处置**：取消记录保留，另记一份「取消被业务决定认定在边界之后」的决定；随后由这份决定驱动重判——网络服务重驱收寄采用（采用编排读到这份决定即不再按取消判先后，照常判资格与责任起点），面单渠道服务重驱终局判断（形成非取消终局）。客户侧那份`包裹已取消`怎么改口、下游已释放的东西怎么补回，属收寄后处置（code/37）那一面，本票只交出入口。
   - **认定边界事实有误，交来源方更正**：不在本上下文改任何事实；来源方更正之后新的一代按 ADR-0254 决定二重判（冲突册按那一代另起一行或不再起行）。
3. 授权未登记答未配置，不默认任何角色可解开。
4. 重判入口除收寄采用重驱与面单终局判断重驱之外，还有**网络服务履约结果（有效交付）的终局采用**：包裹上取消在册时到达的有效交付，`FormParcelFinalHandler` 答 `SOURCE_NOT_ADOPTED`、依据 `PARCEL_CANCELLATION/<取消标识>`，不采用记录按（包裹、结果种类、来源版本）永久保全。决定面把冲突解开（判「取消不成立」）后，必须能把依据该取消的不采用记录作废或重判，使交付重新进入终局规则判断。只等运输履约重投不够：同一版本重投只答「已有结果」，原不采用记录原样返回，这件包裹会一直没有终局。来源：code/98 评审 N1。上线影响：取消授权在生产未配置、没有决定面，冲突形成不了，今天不触发。
5. 补用例：网络服务冲突经决定解开后形成责任起点；面单渠道冲突经决定解开后形成终局；依据该取消的有效交付不采用记录经决定解开后作废或重判、交付形成终局；授权未配置停在未决。

## 要裁的

- 去向集合与每个去向驱动的重判（ADR-0254 越权风险点 2，PS owner）。
- 「认定取消依据不成立」之后，取消在客户侧与委托汇总里怎么读——是取消失效的一种（需要新的取消状态）还是只由收寄后处置覆盖其后果；前者动 CONTEXT 生命周期，可能要另立 ADR。

## 完成记录

分支 `claude-code76`，基于 origin/main `57fc82782`（收尾 rebase；`.scratch/at-coverage-inventory.md` 文末追记冲突，两边并存、main 那段在前）。迁移 `parcel_shipment/0032`、`access_identity/0026`；新 ADR-0295（部分停用（接上）ADR-0254 决定三两句，回答其越权风险点 2）；`AT-PS-166` 至 `AT-PS-173`；无新票。

### 做了什么

- **决定面**：冲突册上一份冲突至多一个决定，只追加，主键与外键都是冲突行主键，去向封闭两值（`CANCELLATION_NOT_UPHELD` 认定取消依据不成立、`BOUNDARY_FACT_REFERRED` 认定边界事实有误交来源方更正），决定人（认证出的操作者，`发行方#sub`）与依据必填；并发只一笔成立，败者读回先到的那一份。迁移 `parcel_shipment/0032` 还建了回放原料表 `cancellation_blocked_replay`。
- **授权**：「运营决定」族新立 `CANCELLATION_ORDER_CONFLICT_RESOLUTION`（`access_identity/0026`，CHECK 取此前各份并集再加一词），与授权处置、释放拒收处理分开授予；租户未登记时没有人能解开，冲突停在未决。
- **端点**：`POST /cancellation-order-conflict-decisions`（已决定 201、已被决定 200、不可见 404 `CANCELLATION_ORDER_CONFLICT_NOT_VISIBLE`、没形成答案 500，回执不带操作者）；`GET /cancellation-order-conflicts`（主数据与运营查阅读面，租户取信封，页大小 200，未解开的冲突与全部计数、`truncated`）。新特性文件加 assemble 文件加特性测试里的未接线占位，没碰 `endpoints.go`、`main.go`、`endpoints_test.go`、`routes.go`。
- **有效取消**：`ParcelCancellations.FindCancellation` 用 `NOT EXISTS` 排除被认定不成立的取消；取消记录原样保留。取消编排、收寄采用、终局采用、面单终局判断、委托完成派生与节点作业服务结果读读的都是这一口，一处排除各处一致。
- **认定取消不成立的落地**：同笔作废依据 `PARCEL_CANCELLATION/<取消>` 的终局不采用行（只动未定案且依据恰是这份取消的行），同笔交一封 `parcel-shipment.parcel-cancellation.not-upheld` 信封（信封 ID 由冲突五维认领，分区取全部维）。parcel-dispatch 新续办门 `CancellationNotUpheldConsumer` 加适配器 `adapters/cancellationredrive`：先取包裹取消边界锁，按当前已接受委托取目标，依回放原料把来源交回收寄采用与终局采用，冲突落在面单渠道首次有效收寄上的再按 TF 链尾现读交面单终局判断；各条链结果过自己消费门的译表（`adoptconsume`、`finalconsume`）。
- **回放原料**：收寄采用判出冲突时随冲突行同笔落来源规格，终局采用因取消不采用时随不采用行同笔落责任结果规格（`CancellationOrderConflictRecord.Replay`、`FinalOutcomeRecord.CancellationReplay`）。
- **交来源方更正**：取消保持有效，不作废、不交信封；收寄采用对这一代答 `SOURCE_NOT_ADOPTED`、依据 `BOUNDARY_FACT_REFERRED/<取消>`（写采用账一行不采用记录，重放答已有结果），面单终局判断答 `CANCELLATION_STANDS`（领域输入加 `ReferredBoundaries`）；两处都不写第二行冲突。新依赖 `Decisions`，nil 等于决定面未接入。
- **文档**：ADR-0295；ADR-0254 的 Status 注记与 Links 加前向指针；UC-PS-006 决定矩阵「事实冲突」一行、规则一句、验收表 `AT-PS-166` 至 `AT-PS-173`、交接行改成已补；UC-PS-003、UC-PS-004 指向本票的交接行改成已补；PS CONTEXT 词条「有效取消」「取消先后冲突决定」、取消一条与包裹生命周期各补一句；盘点八行入①与一段追记。

### 代决

1. **立 ADR-0295。** 决定面的形状接上 ADR-0254 决定三「形状本记录不定」「今天没有这个决定面」两句，按「按口解」算部分停用，不能只挂读法注。
2. **去向集合封闭为两个，各自的效果由我定。** 认定取消不成立（边界事实对）：那份取消不是有效取消，被它挡住的来源重新进入原编排；交来源方更正（边界事实错）：取消保持有效，这一代边界事实不采用。票面提案的两个去向原样采纳。
3. **取消在客户侧与委托汇总里怎么读：不立新的取消状态。** 取消是只追加的已提交决定，改状态就是改写它；「谁在什么时候认定它失效」是独立的业务决定，有自己的留痕、授权与并发规则。用「有效取消」读法表达，CONTEXT 生命周期不动（只补一句说明这不是状态机里的一格），不另立 ADR。客户侧已收到的`包裹已取消`与下游已释放的东西怎么改口、补回，属收寄后服务处置（code/37），本票只交出续办信封这个口。
4. **授权新立一种决定种类，租户取值。** 不与授权处置、释放拒收处理共用（认错一份冲突会让已取消的包裹长出责任起点，或让已在网络手里的包裹挂着取消终局）。谁持有在操作者册里登记，与取消授权目录（PAR-COM-14）是两回事。未登记没有人能解开。
5. **一份冲突的决定不可改。** 来源方回复「事实无误」时，不是改决定，而是它以新的一代再到、再判出冲突、再决定。交来源方更正不交续办信封，没有被挡住的来源要重驱。
6. **续办靠回放原料，不重读来源方。** 节点收寄经来源编号、场外揽收与有效交付经对象与尝试取回，三种引用各不相同，复刻三套消费方适配器不如同笔存几个字符串。面单渠道一侧不存，边界事实本是 TF 的，按链尾现读。目标取回放那一刻的当前已接受委托，不存当时的。
7. **作废在决定同笔做，不放进续办。** 否则决定提交到续办之间到达的同一版本交付会撞上旧不采用行，原样读回。
8. **续办信封分区自成一条队（逐信封）。** 真链上复现过：同包裹的终局不采用意图停在下游消费的未决上，同队的续办信封被堵在它后面；续办读库里已落定的状态，不依赖队里在前的信封。分区登记表登「逐信封」。
9. **查阅挂主数据与运营查阅读面，同 ADR-0293；告警不做；回执不回显决定人。**
10. **不做：管理台页与演示种子**（沿 ADR-0293 先例另跟）、收寄后服务处置（code/37）。

### 越权风险点

1. **去向集合封闭为两个、各自的效果由本票定**（PS owner）。要第三种（例如取消成立但物流已不可追回、转异常案件）是另一个去向、另一份效果；去向词由库内 CHECK 与领域解析各守一遍，加一个是一处迁移加一格。
2. **一人决定即生效，没有二人复核**（PS owner）。认定取消不成立同笔作废不采用行、放行被挡住的来源，之后撤不回。缓解：授予分开、决定人与依据必填、留痕只追加、租户不登记授予就没有人能解开、决定前有查阅清单给出两边的业务时间。
3. **客户侧已收到的`包裹已取消`与下游已释放的东西不会自动改口、补回**（PS owner，知会 TF owner、VE owner）。本票只让 PS 一侧不把那份取消读成有效取消、让被挡住的来源重新进入编排，并交出续办信封；已发给下游的取消释放意图、下游投影里依据这份取消形成的事实、客户通知由 code/37 承接。在它落地之前客户可见状态可能与 PS 的有效取消读不一致。
4. **取消记录保留，同请求键重放取消读到历史答复。** `FindByKey` 仍读回原`包裹已取消`；有效与否只看有效取消读，下游释放意图只在这份取消此刻仍是有效取消时才随重放重发（评审 N2 后补）。
5. **作废是 DELETE 终局不采用行，不是标记。** 这是「终局账只追加」纪律的一个例外（理由、审计与未走第三种账记录的原因见 ADR-0295 决定三之一）；不采用记录的历史由决定留痕加回放原料表（保留，不随作废删）加续办后重新落的记录共同表达。若 owner 要账本严格只追加，要给 `final_outcome` 加作废标记列并改读写口，改动面不小。
6. **续办只重驱「被这份取消挡住」的来源。** 范围是冲突那一代来源与依据这份取消的终局不采用行：
   - `CANCELLED_BEFORE_INTAKE/<取消>` 的收寄前取消不采用记录不重判（来源发生晚于取消的业务时间，责任起点已由被解开的冲突来源占用，重判也只会落 `RESPONSIBILITY_ALREADY_STARTED`）；若冲突来源本身要不到责任起点（收寄资格未证明），这些记录是否也该重判要 owner 复核。
   - 同一份取消名下另有别的代冲突时，决定一份不连带另一份，另一份留在清单里由人再决定。
7. **决定提交到续办完成之间到达的新取消请求。** 网络服务一侧此时有效取消读为空、责任起点还没形成，新取消可能被接受，续办再把来源交回采用时又判出冲突、需要再一次决定；面单渠道服务一侧承运商首次有效收寄已在册，新取消答首次有效收寄拒绝而不是 `DISPOSITION_PENDING`。窗口由包裹取消边界锁排队，但语义上存在。
8. **面单渠道服务的续办按 TF 首次有效收寄链尾现读，不是冲突那一代**（PS owner，知会 TF owner）。链尾换了新一代的以新一代重判；失效或待确认不判。
9. **`Decisions` 为 nil 与决定面未接入同义，漏接时不响亮**（PS owner）。收寄采用的接线有真链用例守着（变异：去掉接线即红）；面单终局判断那一处接线没有可区分的持久效果（交来源方更正与冲突都是`不形成`终局并入账），只有应用层用例与代码审读守着。
10. **真链上形成责任起点与终局的几条用例，把收寄资格（PAR-COM-16 硬资格取证）与终局规则（PAR-COM-17）这些租户实例半边换成放行的替身。** 生产装配在 `cmd/parcel-dispatch` 里永远停在这两格前面；生产路由那一条用例停在「终局规则未配置」，证明续办门路由、作废、回放与未决名单登记（`dispatch.consumer_undecided`，不塌成 `publish_failed`）都对。
11. **触碰了 `cmd/parcel-dispatch/assemble.go`**（既有的收寄采用与面单终局判断两个构造器各多接一只决定视图）与 `internal/architecture/partition_subject_registry_test.go`（登一行），前者按令只动既有消费者构造器，后者是新信封口的架构门禁要求。管理台页、授予的运营界面与演示种子未随本票，租户在登授予之前没有人能解开冲突，API 可用。

### 测试与变异

- 先红：新端口、领域类型与适配器在实现前编译不过；一批真库、传输层与应用层用例在实现之后一次过绿，红的证据靠下面的变异。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`（`./...`，含 `./internal/accessidentity/...`、`./migrations/...`、`./internal/architecture/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`）：在 rebase 到 `57fc82782` 之后的树上前台跑，退 0，159 个包全绿。`gofmt -l` 无输出，`go build ./...`、`go vet ./...` 干净。
- 新用例 `-count=3` 稳定（postgres、application、domain、cancellationredrive、http、inbox、accessidentity/postgres、`cmd/parcel-api`、`cmd/parcel-dispatch`）；并发用例 `-count=5` 稳定（`TestSixConcurrentDecisionsOfOneConflictRecordExactlyOne`、`TestConcurrentDecidersOfOneWiredConflictLeaveOneDecision`）。
- 一次性库灌种子：`seed.sh` 退 0，决定表在、`operator_grant_decision_kind_decided` 的 CHECK 含新词；用后 `DROP DATABASE c76seed WITH (FORCE)`。
- `go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 退 1（生成物过期，协调方进 main 时重生成，不提交 `docs/adr/README.md`、总览计数、`MECHANISM-INVENTORY.md`）；`at-coverage-counts` 没退 2，八个新号在盘点分桶表都有行。
- 变异（备份、还原后 `cmp` 一致），全部被杀：
  1. 决定编排去掉包裹取消边界锁（锁序用例、取不到锁用例红）；
  2. 续办适配器去掉边界锁（次序与失败用例红）；
  3. 作废过宽，删所有未定案不采用行（`TestVoidingRemovesOnlyTheRefusalsGroundedOnThatCancellation` 红）；
  4. 有效取消读不排除被认定不成立的取消（postgres、`cmd/parcel-api`、`cmd/parcel-dispatch` 三处红）；
  5. 有效取消读把交来源方更正也排除（postgres 与 `cmd/parcel-api` 红）；
  6. 交来源方更正也交续办信封（应用层红）；
  7. 信封 ID 丢掉边界那一代（去重，一份冲突一封，两例红）；
  8. 收寄采用冲突行不带回放原料（应用层与真链红）；
  9. 收寄采用忽略交来源方更正（应用层与真链红）；
  10. 查阅读口不按租户过滤（红）；
  11. 收寄采用装配漏接决定视图（`cmd/parcel-dispatch` 红）；
  12. 面单终局领域层忽略交来源方更正（领域与应用层红）。

### 评审提示

- 请重点看：有效取消读的 SQL 排除是否漏了读方；作废 DELETE 的范围与 PBC-08 无事务拒跑；续办信封自成分区的取舍；回放原料随冲突行与不采用行同笔落库、重投不翻倍。

### 评审记录与修订轮

独立评审判**不可合入**：一条阻断（B1）、五条建议（N1 至 N5）与几条信息项。修订轮在同一分支追加提交（代码与用例一份、文档一份、本记录一份），已接受 ADR 的正文未动，改的只有未合入的 ADR-0295 与 UC、AT、CONTEXT 文档。收尾时重新 rebase 到 `origin/main`（`.scratch/at-coverage-inventory.md` 文末追记冲突，两边并存、main 那段在前）。

- **B1（阻断）：续办重新形成的终局没有交出 `parcel-shipment.final-outcome.formed` 信封。** 成因：信封 ID 是采用键（租户、包裹、结果种类、版本）的指纹，不分这一行是不采用还是终局；入队按（来源，事件标识）去重。不采用行当初已交过一封并已投递，决定删掉它后续办用同一键重新形成终局，交接查到那封已在、直接返回；结算侧终局计价时点与 VE 终局投影都停在「不采用」。**协调方裁定的修法**（不走 ADR-0138 的第三种账记录）：终局信封的交接身份折进该包裹的作废代次——已成立的 `CANCELLATION_NOT_UPHELD` 决定个数（决定只追加，单调）。没有作废决定的包裹代次为零，信封 ID 与原算法逐字相同；代次大于零时在采用键四维之后追加十进制代次一维；代次在交接的同一事务内、包裹取消边界锁之下读（`OutboxFinalOutcomeHandoff`，`adapters/postgres/final_outcome_handoff.go`）。分区键不变。新用例：评审的复现改写为 `cmd/parcel-dispatch/cancellation_not_upheld_final_envelope_test.go`（续办后 formed 信封比决定前多一封、ID 各不相同、同一分区）；`adapters/postgres/final_outcome_handoff_void_generation_test.go` 钉「代次零的 ID 等于独立算出的旧算法字面」「交来源方更正的决定、别的包裹、别的租户上的决定不抬代次」「每份作废决定多一封、同代次重放不翻倍」。变异（备份、还原后 `cmp` 一致）全部被杀：去掉代次（postgres 与 dispatch 两处红）；代次为零也加一维（字面 ID 用例与既有交接用例红）；计数不分决定种类；计数不按租户；计数不按包裹。
- **N1：ADR-0295 没讲清物理删除这个例外。** 补决定三之一（作废是物理删除；为何不是 ADR-0138 候选「丁」否决的删真终局行；审计靠决定行加回放原料表；不走第三种账记录的三条理由与代价；与 ADR-0138（Proposed）互不依赖）、决定六（B1 的下游后果与折代次）、候选两条（第三种记录、信封 ID 只加终局或不采用一维为何不够）、越权风险点补物理删除一条。订正适用范围：UC-PS-004 规则句「保留原终局历史」后补「历史不删针对真终局与被采用的行，唯一明写的例外是被认定不成立的取消所致的不采用行」、交接行；UC-PS-006 的 `AT-PS-169` 改物理删除、`AT-PS-173` 补新信封；CONTEXT 两处。CONTEXT「不得静默改写原决定历史」一句核对过，它说的是包裹级继续尝试判断的决定历史，不涉及终局账，未动。
- **N2：取消重放不看有效取消。已实现，没有退为风险点。** `CancelParcelHandler.existingResult` 对 `PARCEL_CANCELLED` 的历史记录先读有效取消：读到的就是这一份（比取消标识）才重发释放意图；读到别的取消或没有，答复仍是历史答复（`EXISTING_RESULT` 加原记录），不重发、不留续办引用；读不动答未决 `CANCELLATION_STORE_UNAVAILABLE` 带续办引用（ADR-0029）。牵动面只有这一个方法；既有重放用例让有效取消读答出被重放的那一份，其余路径不变。新用例 `application/cancel_parcel_replay_effective_test.go` 四格（评审复现、读到别的取消、仍有效、读不动）。变异全杀：去掉判断；不比取消标识；读不动当作仍有效。
- **N3：目录过滤有存活变异。** 新用例 `TestTheCatalogueKeepsAConflictThatDiffersFromADecidedOneInASingleDimension`：与已决定那一行只差一维（别的包裹、边界种类、边界那一代、取消、租户）的冲突行都留在清单里，含评审的（取消 X，边界 R）已决、（取消 Y，边界 R）仍在。五维各删一次，全部变红。
- **N4：「之后的取消请求答 `DISPOSITION_PENDING`」有窗口。** ADR 决定三收窄措辞（网络服务一侧责任起点形成之后才答待处置；面单渠道一侧答首次有效收寄拒绝），越权风险点补窗口一条（决定提交到续办完成之间网络服务一侧新取消会被接受，续办交回采用时需再决一次）。
- **N5：决定对锁与交付并发缺真库交错用例。** 新增 `adapters/postgres/cancellation_order_conflict_decision_race_test.go`：决定先手停在作废之后、交付后手（要被锁挡住，等决定提交后读到取消已不成立、形成终局、不留不采用行）；交付先手停在读完有效取消之后、决定后手（要被锁挡住，等交付提交后作废它落下的不采用行，作废 1 行）。`-count=5` 稳定。去掉决定的包裹取消边界锁，两格都变红。**续办的锁要如实说**：续办里每条链进来时都会再取一遍同一把锁（重入），去掉续办自己那一次，最终落库看不出差别，所以任何结局型的真库用例都杀不掉它；补的是次序型的真库用例 `cmd/parcel-dispatch/cancellation_redrive_lock_test.go`——另一笔事务握着这件包裹的锁时，续办必须等在咨询锁上、目标读一次都没做，放锁后才读；去掉续办的锁，它变红（`TestARedriven*` 仍绿，印证上面的判断）。
- **信息项。** ADR 越权风险点补：作废不检查回放原料是否在（迁移 0032 之前落的不采用行与冲突行没有原料，被作废后无人重驱，生产没有取消授权，只影响历史数据）；认定不成立是整取消级（同取消名下另一份冲突仍在清单、对它再决 `BOUNDARY_FACT_REFERRED` 语义上是空动作，再决 `CANCELLATION_NOT_UPHELD` 则抬代次、多一封无害的信封）；终局采用重放对同种类不同版本一律重派生不比先后（决定到续办之间若更新一版先形成终局，续办用旧版重放会把终局退回旧版，终局链既有性质，窗口放大它）；代次整包裹级（作废之后该包裹上任何交接重放都再出一封，依赖两个下游消费方按键回读幂等）。`decidedAt`：决定编排在形成处把时间截到微秒，`TestTheDecisionInstantIsTruncatedToTheStoresPrecision` 与 `cmd/parcel-api` 里决定用例的 201 与 200 读回一致断言（去掉截断两处红）。`domain/cancellation_order_conflict_decision.go` 头注的「不再被任何判断…」改成现行规则句。
- **代决补记。** 一、B1 的修法按协调方裁定走折代次，没有改成第三种账记录。二、续办取锁次序的用例用「目标读口计数」做探针，而不是调用顺序替身，因为它守的是真咨询锁挡住了续办。三、`dispatch` 夹具里新信封没有走到下游消费：同包裹的收寄信封在该夹具里停在下游未决，与终局信封同一条分区队，新信封排在它后面；下游按采用键回读且幂等由协调方裁定采信，新用例断言的是入队、ID 互异、同分区。
- **测试。** `go build ./...`、`go vet ./...` 干净，`gofmt -l` 无输出；`scripts/test-full.sh ./internal/parcelshipment/... ./internal/accessidentity/... ./internal/settlementaccounting/... ./internal/visibilityexception/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/... ./migrations/...` 在 rebase 到 `origin/main` 之后的树上前台跑，退 0；新用例 `-count=3` 稳定，并发用例 `-count=5` 稳定；`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，没有退 2），`go run ./tools/adr-index -check` 退 1（生成物过期），均不提交生成物。无迁移改动，没有灌种子。

### 复审修订轮

窄范围复审仍判**不可合入**：一条阻断（B2）与两条建议（N-a、N-b）。收尾时先 `git fetch` 再 rebase 到 `origin/main`（本轮开始时已是最新，无冲突）。

- **B2（阻断）：折代次让新信封入队并被结算消费，VE 终局投影仍停在「不采用」。** 复审用真链探针（`wireDispatcher` 装配 VE 与结算两个消费方）推翻了第一轮「下游按采用键回读、幂等，由协调方采信」里对 VE 的那一半——那是一次未经核实的采信：VE 的事实键是（租户、来源、事实引用、版本），版本取采用键的责任结果版本，重新形成的终局与被删的不采用行同键、内容不同（类型、时间），派生编排判 `SOURCE_CONFLICT`，消费门把它当业务终局入账、保留原事实、不发信号、不重投，信封被消费了而 `accepted_fact` 仍只有那一行不采用。结算侧确是按采用键回读，没有这个问题。**按协调方裁定的 (a) 修 VE，不排除**：
  - 信封载荷加可缺省的 `voidGeneration`（`finalOutcomePayload`，`omitempty`）：代次零时不序列化，载荷与只有四维的相同；大于零时带上。VE 消费门译码带出代次（缺省零，负数或非数字是毒丸）。
  - VE 译装器（`adapters/parcelshipment/derive_on_final_outcome.go` 的 `finalFactVersion` 与 `finalProjectionCommand`）：代次大于零的记录——已采用终局与再次不采用同样，后者同样会因时间不同判成冲突——用事实版本 `<采用键版本>#void-<代次>`，事实引用不变；前身：重派生沿用 PS 指名的前一代采用版本，否则指名同一采用键上一代次的版本（代次一的前身是代次零的原版本，即被删的不采用行当初投进来的那份）。先读过 VE 事实版本与 `Supersedes` 的现有语义（同一源上下文、同一事实引用的版本之间；被替代的留档、不参与派生；同一前身被两份指名是分叉），沿用 AT-VE-174 的先例，没有新机制。
  - 结算侧译码对未知字段宽容（`TestTheFinalOutcomeConsumerToleratesTheVoidGenerationField`），按采用键回读，计价请求按来源引用幂等。
  - 新用例：`cmd/parcel-dispatch/cancellation_not_upheld_final_envelope_test.go` 的 `TestARedrivenFinalReachesTheVisibilityProjectionAsAnAdoptedFinal`（决定加续办之后 VE 事实库两份留档、已采用事实指名不采用那一版为前身，当前投影恰一条已采用终局，同代次再拍不翻倍）与 `TestASecondVoidedCancellationClosesAnotherGenerationInTheVisibilityProjection`（第二份取消再被认定不成立，三代接续闭合）；VE 真库替代链 `TestAReformedFinalOutcomeSupersedesTheRefusalItReplacedInTheProjection`（含不带代次时恰是 `SOURCE_CONFLICT` 的反证）、译装器 `TestAVoidGenerationGivesTheReformedRecordItsOwnFactVersionAndSupersedesTheGenerationBefore`、VE 译码 `TestAFormedFinalOutcomeCarriesItsOptionalVoidGeneration`、PS 载荷（代次零不带字段、代次一带 1）在 `final_outcome_handoff_void_generation_test.go`。结算侧在 dispatch 夹具里没有包裹的商业解析与结算政策，新信封的结算那一路停在它自己的未决格直至预算耗尽，所以真链用例只断言 VE 一侧；结算侧的真链装配另见 `sell_evaluation_on_parcel_moment_test.go`。
  - 变异（备份、还原后 `cmp` 一致）全部被杀：VE 译装器忽略代次（版本与前身，译装器、VE 真库、两条真链用例都红）；不填 `Supersedes`（同上都红）；PS 载荷代次零也写字段（载荷用例红）；PS 载荷不带代次（载荷用例与真链用例红）；VE 译码忽略代次（译码用例与真链用例红）。
  - 文档：ADR-0295 决定六重写（两层问题、修法一与二、重复通知对结算侧与 VE 各自为何无害），候选补「VE 判冲突后不处理」，越权风险点第十五条改为如实写接续的两处不闭合；UC-VE-002 的 `AT-VE-174` 行与 UC-PS-006 的 `AT-PS-173` 行同步；盘点补相应用例。
  - **如实写的限制**（越权风险点，不是新机制能顺手补上的）：版本里的代次取自信封，不取自记录形成时，所以（1）代次跳着走时中间那一代的事实在 VE 里不存在，较早代次的原事实仍当前有效，可能同时在场两份内容相同的已采用终局；（2）重派生遇上作废代次，前身沿用 PS 指名的采用版本（不带后缀），前一代若是在代次大于零时形成的就指名落空。两处都不丢事实，要闭合得让记录自己带形成时的代次（账上多一列）。
- **N-a：代次的 ADR 引用写错了决定号。** 折代次在决定六。`final_outcome_handoff.go` 的 `voidGeneration` 与 `HandOffFinalOutcome` 注释、`ports.go` 的 `FinalOutcomeHandoff` 注释、`final_outcome_handoff_void_generation_test.go` 三处与 `cancellation_not_upheld_final_envelope_test.go` 的 Covers 都改到决定六（后者是决定三、四、六）；全仓 grep 核过，其余「决定三」的引用说的是作废的写法、有效取消读与锁，没有再指折代次的。
- **N-b：存活变异（`existingResult` 里的 `record.Kind == ports.RecordParcelCancelled` 判断）。** 补 `TestAReplayOfARefusalOrAPendingDispositionDoesNotAskForTheEffectiveCancellation`：重放拒绝、待处置记录而有效取消读故障时仍答 `EXISTING_RESULT` 加原记录，不问有效取消读（计数不变）、不交意图；去掉判断，红。
- **测试。** `gofmt -l` 无输出，`go build ./...`、`go vet ./...` 干净；`scripts/test-full.sh ./internal/parcelshipment/... ./internal/settlementaccounting/... ./internal/visibilityexception/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/architecture/... ./migrations/...` 前台跑，退 0（中途 `internal/architecture` 的事务回调断言门拦过一次：新用例在事务闭包里调了 `t.Fatalf`，已把断言搬到闭包外）；新用例 `-count=3` 稳定；`go run ./tools/at-coverage-counts -check` 退 1（只是总览过期，没有退 2），不提交生成物。无迁移改动。


### 第三轮窄复审（协调方）
- 复审可合入，B2 在真链上闭合（事实库留档各代、已采用事实指名不采用那一版为前身、当前投影恰一条已采用终局）；`Supersedes` 指向不在场的版本时 VE 既不拒收也不卡死（`DeriveProjectionHandler.Handle` 不校验存在性）；载荷兼容、版本字符串约束、幂等均通过；自做 7 处变异全部被杀。
- 剩一条建议（重派生遇上作废代次）协调方裁定：本票不加列。代次按包裹计，该包裹上存在过任何一份认定取消依据不成立的决定之后，对它做同种更正重派生，前身指名对不上带代次后缀的前一代，投影里暂有两份已采用终局（业务时间不同，不立分叉信号、不卡死）；暴露面限于登记了本决定种类授予的租户。如实写进 ADR-0295 越权风险点 15（2）与 UC-VE-002 的 `AT-VE-174` 行；要闭合需让记录带形成时的代次，归 owner 复核。两处测试注释措辞随手改成现行规则句。
