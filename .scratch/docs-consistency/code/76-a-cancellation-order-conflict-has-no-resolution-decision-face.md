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
4. **取消记录保留，同请求键重放取消读到历史答复。** `FindByKey` 仍读回原`包裹已取消`；有效与否只看有效取消读。
5. **作废是 DELETE 终局不采用行，不是标记。** 这是「终局账只追加」纪律的一个例外；不采用记录的历史由决定留痕加回放原料表（保留，不随作废删）加续办后重新落的记录共同表达。若 owner 要账本严格只追加，要给 `final_outcome` 加作废标记列并改读写口，改动面不小。
6. **续办只重驱「被这份取消挡住」的来源。** 范围是冲突那一代来源与依据这份取消的终局不采用行：
   - `CANCELLED_BEFORE_INTAKE/<取消>` 的收寄前取消不采用记录不重判（来源发生晚于取消的业务时间，责任起点已由被解开的冲突来源占用，重判也只会落 `RESPONSIBILITY_ALREADY_STARTED`）；若冲突来源本身要不到责任起点（收寄资格未证明），这些记录是否也该重判要 owner 复核。
   - 同一份取消名下另有别的代冲突时，决定一份不连带另一份，另一份留在清单里由人再决定。
7. **决定提交到续办完成之间到达的新取消请求。** 此时有效取消读为空、责任起点还没形成，新取消可能被接受，续办再把来源交回采用时又判出冲突、需要再一次决定。窗口由包裹取消边界锁排队，但语义上存在。
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
