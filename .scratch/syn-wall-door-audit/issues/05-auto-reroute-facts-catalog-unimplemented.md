# 自动改路四条件事实目录无生产实现,改路评估整段显式未配置

Category: enhancement
Status: resolved

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W10。

## 墙

无字符串哨兵——墙落在装配点:`cmd/parcel-dispatch/assemble.go` 的 `networkIntakeConsumer` 给 `ReassessRouteDeps.AutoReroute` 显式传 nil,注释:「自动改路四条件的事实目录没有生产实现。nil 是『显式未配置』的诚实表达……失效照常落库,改路评估整段不做——连建议都不形成,因为说不出『为什么没自动』。」

## 现状:门四件全缺

四条件事实目录(端口在 `networkrouting/ports`)无表、无装载口、无写入方、无登记口。

## 缺的最小机制件

自动改路条件事实目录:存储 + 装载口 + 登记口(改善阈值、自动/人工改路条件与权限、冻结边界,PAR-NET-14 的复核策略机制半边),以及 `ReassessRouteHandler` 在目录就位后的评估路径打通。

## 红线

- 阈值与条件全部属实例半边(PAR-NET-14 待提供);本票只建目录机制,验证用合成条件,S 级只记 S。
- 目录未配置时保持现状(整段不做、失效照常落库),不得半配置地只做建议不做说明。

## 参照

PAR-NET-14;`networkrouting/application/reassess_route.go` 端口注释。

## Comments

- 2026-08-20 MCP-1(采纳时互链):与 [`nr-route-evidence-views/issues/01`](../../nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md)
  同域——那票的机制四件(NR-CATALOG-MECH,含路由策略与临时可用性调整的版本化目录 schema)
  已开工,本票的「存储 + 装载口」半边可能被它部分覆盖。开工前先对齐范围,勿双做。
- 2026-08-20 · MCP-3：对 3324ecb 重核四件（只读）。**票面完全成立，且上一条的范围
  对齐关切可销**：NR-CATALOG-MECH 已入 main（3b9f212，迁移 0008），其头注明写「冻结
  边界与改路条件全属 PAR-NET-14……等形态定了再以新迁移扩列」——那批**刻意没做**本票
  的目录，无重叠。现状复核：`cmd/parcel-dispatch/assemble.go` 仍显式 `AutoReroute: nil`
  且注释原样（文件最后触碰 a771bc3，是 VE 接线未动此行）；`AutoReroute` 端口在
  `networkrouting/ports/ports.go`，全仓无适配器实现（匹配点只有 domain/reroute 与
  reassess_route 消费侧）；表/装载口/写入方/登记口四件全缺原样。
  建议：ready-for-agent，四件全由本票做；阈值与条件取值属 PAR-NET-14 待提供（机制
  半边不被阻断）。可参照 0008 的版本化先例（未闭区间部分唯一索引、修订锚）。
- 2026-08-20 MCP-1：采纳重核，Status → ready-for-agent。实现另派（占 assemble.go 时单独占号）。
- 2026-08-24 10:58 · MCP-6：认领本票（Status → in-progress），基线 `b51de75`（与远端
  main 一致，`ls-remote` 于 10:57 取证）。范围照票面四件全做；动
  `cmd/parcel-dispatch/assemble.go` 前将按票面纪律在频道单独占号。
- 2026-08-24 · MCP-6：四件交付完毕，随本笔提交，票转 resolved。

  **件一（存储）**：迁移 `network_routing/0009_auto_reroute_facts.sql`。判断键六维 +
  version 历史链（同键版本行只增不改，当前陈述取最大版本，照 `availability_adjustment`
  先例——四条件是「当下陈述」不是「计划生效」，区间制没有对应语义，故不搬 0008 的
  区间与修订锚）；三个折算结论布尔列 + 两份引用清单 jsonb（CHECK 限 array，空数组是
  「无未解限制」的有效陈述）+ `strategy_basis` 折算依据列（事实要说得出按什么折的）。
  「未配置」由零行表达，无中间态。`migrations.go` 是目录级嵌入（`all:network_routing`），
  零触碰。

  **件二（装载口）**：`adapters/postgres/auto_reroute_facts.go` 的
  `AutoRerouteFactsCatalog.LoadAutoRerouteFacts` 实现 `ports.AutoRerouteFactsView` 三格：
  最大版行→五件事实；零行→未配置；错误只留依赖故障与坏行。键不完整的读是错误不是
  未配置（身份不成立不读权威），空白清单元素在读回时经领域构造器炸成「数据坏了」。

  **件三（写入方）**：同文件 `RegisterAutoRerouteFacts` + `FindAutoRerouteFacts`，实现
  新端口 `ports.AutoRerouteFactsRegistry`（编译期钉住）。`已登记`用 ON CONFLICT DO
  NOTHING 加零行判定翻译，不捕 23505（撞键会把事务打进中止态，编排还要同事务读回
  比对——先例 `ReachabilityJudgments`，真库用例钉住「重复登记后同事务读回赢家」）；
  写口走 `RequireExecutor`，无环境事务即拒（票 06 钉住的同一格，真库用例在）。

  **件四（登记口）**：`application/register_auto_reroute_facts.go`。受理门逐格拒
  （键不完整/版本缺/依据缺/清单元素空白，`AutoRerouteFactsRefusalReason` 指名），
  一个默认值都不补；幂等与冲突分界在编排——写口答`已登记`后读回既有版逐字段比，
  同则`已存在`、异则`内容冲突`，绝不覆盖（先例：CC 案件配置登记册）；事务由进程级
  入口给出。登记时刻由 Clock 给，不由登记方带入。

  **评估路径打通**：`cmd/parcel-dispatch/assemble.go` 的 `networkIntakeConsumer` 把
  `AutoReroute: nil` 换成真适配器（占号后单独改这一处）。空册行为与先前 nil 等价
  （零行答未配置→失效照常落库、评估整段不做），登记过的键才走 7B/7C——应用层
  `rerouteAfterLapse` 的三态分派（自动改路/建议/禁行）此前已实现且有测试，本票不改它。

  **红线核验**：阈值与条件取值属 PAR-NET-14，一个未写死（表存折算结论与出处，不存
  阈值）；无默认行、无实例值；测试值全 SYN- 风格合成（S 级只记 S）；未配置时保持
  现状由真库用例与装配注释双钉。进程级入口（CLI）票面未列，未做——如需照
  `parcel-network-register` 先例另立票。

  **验证**（共享树包级，全量隔离树验证随推送前完成并记于提交信）：gofmt /
  `go build ./...` / `go vet` 零信号；`go test -count=1 ./internal/networkrouting/...`
  全绿含真库，单跑 `-run AutoReroute -v` 七用例真 PASS 非 SKIP（DSN 生效）。
