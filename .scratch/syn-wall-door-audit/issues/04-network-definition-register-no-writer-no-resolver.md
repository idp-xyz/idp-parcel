# 网络目录缺登记用例与进程级登记口——目录机制已在，三口仍恒答未配置

Category: enhancement
Status: resolved

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W09。票面于 2026-08-21 按基线 `9e5c5c0` 改写:原题「无写入方也无解析层」的前一半已被 NR-CATALOG-MECH(`3b9f212`,ADR-0068)部分推翻,范围随之收敛。文件名保留原 slug 作稳定票号,不随题改名。

## 墙

`NETWORK_EVIDENCE_NOT_CONFIGURED`(`assess_parcel_reachability.go` 的 `NetworkEvidenceNotConfigured`)、`ROUTE_EVIDENCE_NOT_CONFIGURED`(`create_initial_route.go` 的 `RouteEvidenceNotConfigured`)、`REASSESS_EVIDENCE_NOT_CONFIGURED`(`reassess_route.go` 的 `ReassessEvidenceNotConfigured`);另有 `ErrNetworkDefinitionUnresolvable`(`networkrouting/adapters/postgres/network_definition.go`)。

**这三堵墙不在本票的交付范围内**——理由见下节「本票不降墙」。它们仍列在此处,因为本票是 W09 名下唯一在办的票,不写清会让人以为做完就拆了墙。

## 现状:重核于 `9e5c5c0`

原票面写「门框在,门与楼板都不在」。逐件对今天的代码取证后,四件的实际分布是:

**仓储:已有,而且是两套表,分工由 ADR-0068 Decision 六钉死。**

- 迁移 `0007_network_definition` 的 `network_routing.network_definition`,按(租户+服务目的)登「有没有网络定义、视图修订哪版」,供三个证据视图答三格。
- 迁移 `0008_network_catalog` 的七张版本表(`logistics_node_version`、`network_connection_version`、`line_version`、`service_area_version`、`service_calendar_version`、`availability_adjustment`、`route_strategy_version`)加目录修订锚 `network_catalog_revision`,是定义原语本体,按租户存放。
- 服务区域的地理覆盖、服务日历的营业日/服务窗口/截单、路由策略的规则正文**刻意一列未建**(0008 头注:属 `PAR-NET-14`,形态定了再以新迁移扩列)。

**装载口:已有。** `NetworkCatalog.LoadDefinitionsAt` 单条语句取回七族适用行与修订(单语句单快照,事实与修订必然同版);稳定六族按 `[effective_from, effective_to)` 含 `asOf` 选版,临时调整族先取历史链最大版本作当前陈述再判生效窗口;未配置由修订行是否存在分辨,不由「查出零行」推断;同一身份两版同时适用经 `refuseAmbiguity` 交回 `ErrAmbiguousNetworkCatalog`。0007 那侧的读口 `NetworkDefinitions` 也在,且已接调度器两条链(`assemble.go` 的 `acceptanceConsumer` 与 `networkIntakeConsumer` 各调一次 `NewNetworkDefinitions`)。

**写入方:适配器级已有,生产调用方为零。** 七个 `Register*` 方法(`RegisterNodeVersion`、`RegisterConnectionVersion`、`RegisterLineVersion`、`RegisterServiceAreaVersion`、`RegisterServiceCalendarVersion`、`RegisterRouteStrategyVersion`、`RegisterAvailabilityAdjustment`)俱在,各以 `bumpRevision` 在同一事务里推进修订,登记未闭新版时前版接续闭合、调整族不接续。但全仓调用方只有 `network_catalog_test.go`。**0007 的 `network_definition` 则是彻底零写入方**:全仓 INSERT 只出现在 `network_definition_test.go`,读口那句「今天本表没有写入方,因此生产上恒答`未配置`」自 `0f266be` 一字未改;`bumpRevision` 写的是 `network_catalog_revision`,与 0007 不是同一张表,登目录不会让 0007 长出行来。

**登记口:零。** `internal/networkrouting/application/` 下只有 `assess_parcel_reachability.go`、`create_initial_route.go`、`reassess_route.go`、`validate_reachability_judgment.go` 四个用例,没有任何目录登记用例;`NewNetworkCatalog` 在 `cmd/` 下无调用方,没有进程级入口。

**解析层:零,原样。** 把目录折成逐候选事实(候选生成、过滤、排序)属 `PAR-NET-14`;ADR-0068 Decision 六明写「三个证据视图仍不读本目录」,护栏在 0008 头注与 `network_catalog.go` 的 `NetworkCatalog` 类型注释两处钉着。

## 缺的最小机制件

原票面第 1 件把「原语模式设计」与「登记口 + 写入方」捆成一件。`3b9f212` 已把模式与版本机制那一层单独取走,本票剩下的是它下面的两层:

1. **目录登记用例(应用层)。** 把七个 `Register*` 包成受理门齐备的登记用例:拒零值、逐族独立成败、错误代数逐格译成应用答案,事务不由本层开(与 `register_case_configuration.go`、`RegisterPriceCardHandler`、`RegisterReferenceSeriesHandler` 同一条纪律,环境事务由进程级入口给出)。
2. **进程级登记口。** 把用例接进一个受控入口,让运营方能从进程外登记网络定义。已有两个先例可照:`cmd/parcel-commercial`(PC 登记口,「是登记口不是后台 CRUD」)与 `cmd/parcel-pricing-register`(计价配置受控登记口,件④)。落哪个进程由做票人按这两个先例定,**不是难逆转取舍**,不必先走 ADR。

两件顺序执行,1 完成后 2 才有东西可接。

**不并入本票**(各自另票另裁):

- **解析层**——被 `PAR-NET-14` 阻断,且 ADR-0068 已把它连同「目录修订与视图修订的合流」一起后置。
- **内容列扩展**——同上,且不得改写已施加的 0008,须走新迁移。
- **0007 `network_definition` 的写入方**——它归属哪一层今天没定:该行应随目录登记自动形成,还是另有登记路径,正是 ADR-0068 判给解析层设计的「两表合流口径」。本票只登目录,不碰 0007 的写。

## 本票不降墙

做完 1 与 2 之后,`NETWORK_EVIDENCE_NOT_CONFIGURED` 等三堵墙**一堵都不会降**:三个证据视图只读 0007,而本票不给 0007 写入方;即便给了,`LoadNetworkEvidence` 与 `LoadInitialRouteEvidence` 在登记存在时走的是 `ErrNetworkDefinitionUnresolvable` 那条响亮上抛的分支,不是「已配置带事实」。

因此本票交付后 W09 的墙面状态由「无门(写入方 + 解析层双缺)」变为「无门(解析层缺)——目录半边已可从进程外配置」。这不是本票没做够:ADR-0068 Consequences 已明确接受「会出现一段目录可写可读、尚无人读它产出事实的时期,与 ADR-0017 接受的『接口已定、只有内存替身』同形状」。把这一段写在票面上,是为了让做票人不去为了「让墙降下来」而在 0007 或三口取数侧动手——那会撞穿 ADR-0068 的护栏。

## 红线

- `PAR-NET-01..15` 的实例值(真实节点/连接/线路)待提供是常态;本票只建登记机制,验证用 `SYN-*` 合成定义,S 级只记 S。
- 不得为纵向变绿在生产装配里种网络定义(0008 头注与装配点注释两处已禁),迁移不种默认行。
- 不得让三个证据视图改读 0008 目录(ADR-0068 Decision 六的护栏),也不得把 `ErrNetworkDefinitionUnresolvable` 退成`未配置`或空事实(ADR-0053 第四条第三格)。

## 参照

ADR-0068、ADR-0053、ADR-0052;`PAR-NET-14`;`docs/domain/network-routing/CONTEXT.md`;登记用例与进程口先例见 `internal/customscompliance/application/register_case_configuration.go` 与 `cmd/parcel-pricing-register`。

## Comments

- 2026-08-20 · MCP-3：对 3324ecb 重核四件（只读）。**票面大幅过时——「缺的最小机制件」
  第 1 件的模式设计半边已由 NR-CATALOG-MECH 落地（3b9f212，2026-08-20，ADR-0068）。**
  仓储表：**已有**——`migrations/network_routing/0008_network_catalog.sql` 七表骨架（节点/
  连接/线路/服务区域/服务日历/路由策略六类稳定定义 + 临时可用性调整 + 目录修订锚）；
  服务区域与日历的**内容列刻意未定**（0008 头注：属 PAR-NET-14，形态定了再以新迁移扩列）。
  装载口：**已有**——`NetworkCatalog.LoadDefinitionsAt`（单语句单快照、未配置与空目录
  分格、两版同时适用报 `ErrAmbiguousNetworkCatalog`）。写入方：**适配器级已有**——七个
  `Register*` 方法带修订锚同事务推进，但调用方只有测试；**应用层登记用例与进程级登记口
  仍零**（`NewNetworkCatalog` 未接 cmd）。0007 的 `network_definition` 登记表本身仍无写入
  方（读口注释原话未变，0f266be），三个证据视图仍只读 0007 恒答未配置——「解析层存在前
  三口取数侧不接」的护栏在 0008 头注与 `network_catalog.go` 注释两处钉着。解析层：仍零
  （设计上后置：候选生成/过滤/排序属 PAR-NET-14）。
  建议：票面按上述改写后 ready-for-agent，范围收敛为「登记**用例** + 进程级登记口
  （含 0007 登记行随目录登记如何形成——两表合流口径属解析层设计，先只登目录）」；
  解析层与内容列被 PAR-NET-14 阻断，另票另裁，勿并入。
- 2026-08-20 MCP-1：重核属实，票面先改写再 ready；本轮不派实现。解析层/内容列另票。
- 2026-08-21 · MCP-2：按派单改写票面（零代码），基线 `9e5c5c0`。四件重核结论与上一条
  MCP-3 一致，无一处需要推翻；本轮补的是它没展开的三点。

  **一、`3b9f212` 到底做了多少。** 原题「无写入方也无解析层」两半的命运不同，且前一半
  必须按**表**拆开才说得准。「无写入方」对 0008 目录**不再成立**：七个 `Register*` 连同
  `bumpRevision` 的同事务修订推进都在。但它对 0007 的 `network_definition` **完全成立**，
  且是原样成立——全仓 INSERT 只在 `network_definition_test.go`，`bumpRevision` 写的是
  `network_catalog_revision` 这张**另一张**表，登目录长不出 0007 的行。若按审计判「无门」
  时的口径（今天能不能从进程外配置），那么「无写入方」这一半**仍然成立**：`NewNetworkCatalog`
  在 `cmd/` 下无调用方，`internal/networkrouting/application/` 无登记用例，目录今天只有
  测试进得去。「无解析层」那一半**一字不改地成立**——ADR-0068 Decision 六自己就明写三口
  不读本目录，`LoadNetworkEvidence` 与 `LoadInitialRouteEvidence` 的 `ErrNetworkDefinitionUnresolvable`
  分支原样在。所以准确说法不是「前提被推翻」，是原票面把**模式设计**与**登记路径**捆成
  一件而 `3b9f212` 只取走了前者，剩下的一件今天确实还是零。

  **二、剩余范围。** 有剩余，不建议关票，已按上文改写为两件：登记用例 + 进程级登记口。
  进程口这一件曾疑似藏取舍（落 `parcel-api` 还是独立进程），核后**不是**——`cmd/parcel-commercial`
  与 `cmd/parcel-pricing-register` 两个受控登记口先例都在仓里，照做即可，无须先立 ADR。
  另补了原票面与 MCP-3 都没写进去的一节「本票不降墙」：两件做完三堵墙一堵不降，而
  ADR-0068 Consequences 已预先接受这段「可写可读、无人读它产出事实」的时期。不写这一节，
  做票人极可能为了让墙降下去而去动 0007 或三口取数侧，那正好撞穿 Decision 六的护栏。

  **三、W09/W10 归属。** W09 归本票，但**本票解不掉它**，只把它从「双缺」推到「解析层
  单缺」，拆墙要等解析层那张票。**W10 从来不在本票下**——派单里「审计把这两堵墙挂在本票
  下」与审计原文不符：`report.md` 清单 W10 行的「票」列写的是 **05**，其 `### W10 自动改路
  事实目录(无门)` 段末也是「→ 票 05」；票 05 现为 `ready-for-agent`，四件全由它做，与本票
  无交集（0008 头注明写冻结边界与改路条件等 `PAR-NET-14` 扩列，刻意没做票 05 的目录）。
  两票无重叠，也无需互相等待。

  **Status → `ready-for-agent`**（本轮由本笔直接改到票面）。理由：剩余两件的形状已由仓内
  先例定死，不依赖任何 `PAR-NET-14` 取值，机制半边放行；被阻断的解析层与内容列已在上文
  明确排除出范围。未发现难逆转取舍，故不报 ADR。

  本笔只动 `.md`，故无构建信号可报。

- 2026-08-21 · MCP-5：两件交付落地（基 `82eb4e1`，隔离树 `nr04-catalog-registration`），
  开工前连续性已核：`9e5c5c0..82eb4e1` 对 `internal/networkrouting` 与
  `migrations/network_routing` 零触碰，上文重核结论原样成立。

  **件一（目录登记用例）**：`internal/networkrouting/application/register_network_catalog.go`。
  七族各一方法、独立成败；受理门与 0008 的 CHECK 逐条同格（租户/身份码/版本号、节点连接
  线路的业务时区、连接端点在场且相异、段链非空且无空环、线路与策略的适用范围、生效时间
  非零、区间正序含空区间、封闭枚举两集、调整来源与解除窗口），拒绝以 `CatalogRefusalReason`
  逐格指名；事务不由本层开（与 `register_case_configuration.go`、`RegisterPriceCardHandler`、
  `RegisterReferenceSeriesHandler` 同一条纪律）。写入口错误上抛不折格——NR 目录写入没有
  出格答案（重复版本号由主键挡，ADR-0068 Consequences 接受），不照搬 VE 写入口的三格。

  **件二（进程级登记口）**：`cmd/parcel-network-register`。两先例中取 `parcel-pricing-register`
  的形状落**独立进程**——网络定义登记是治理动作不是在线请求面，不进 parcel-api 端点表；
  `-kind` 封闭七族 + `-file` 登记行 JSON（未知字段拒、封闭枚举逐格译、可选终点用指针表达
  不在场），环境事务 `WithinTransaction` 包用例，退出码 0/1/3（无治理格 2：版本冲突落
  未决错误文本，由人按约束名续办）。

  **支撑改动**：七类登记行类型与两个封闭枚举自 postgres 适配器上移 `ports`
  （`catalog_registration.go`）——登记用例要以它们表达受理门，而应用层不得依赖适配器
  （边界门禁）；读侧快照与选版留在适配器**不设端口**，理由写在 ports 文件头（Decision 六
  护栏：不给三口发邀请）。适配器加编译期钉 `ports.NetworkCatalogRegistry`。

  **红线核验**：0007 `network_definition` 零触碰；三口取数侧零触碰；迁移零改动、无默认行、
  无实例值；测试值全 `SYN-` 合成（S 级只记 S）。

  **验证**（隔离树，含真库）：gofmt / `go build ./...` / `go vet ./...` 零信号；
  `go test -p 1 -count=1 ./...` 全绿，其中单跑
  `TestCatalogRevisionAdvancesWithEveryRegistrationKind -v` 为 **PASS 非 SKIP**（DSN 生效）；
  `internal/architecture` 门禁全过（接线棘轮基线无 networkrouting 条目，本票不触）。

  **W09 墙面状态**照上文「本票不降墙」预告推进：「无门（写入方+解析层双缺）」→
  「无门（解析层缺）——目录半边已可从进程外登记」。三堵墙一堵未降，属 ADR-0068
  Consequences 明文接受期，拆墙等解析层票（`PAR-NET-14` 之后）。

- 2026-08-24 MCP-3（死现场抢救合入，受用户裁定执行）：上条评论所属提交（`6cf6c89`）
  从未落 main——它躺在 `nr04-catalog-registration` 孤儿分支上（工作树 mtime 停在
  2026-08-21 23:17，其后未再响应）。本笔按票 13/票 11 先例「逐行复核＋验证由本笔完成」
  办：单摘该笔到 main（cherry-pick 干净落地，零冲突）；连续性由其基线 `82eb4e1` 顺延核
  到今日 main（其间 2b68c89/6c5940c/c26b50f 三笔均不触 networkrouting 与
  migrations/network_routing）；逐行复核对照本票面两件交付与红线三条逐格成立——受理门
  与 0008 CHECK 同格拒零值、事务归进程口、读侧快照刻意不设端口（Decision 六护栏）、
  0007 与三口取数侧零触碰、迁移零改动。上条验证断言随死会话作废，验证由本笔在隔离树
  重做，结果记于本笔提交信。