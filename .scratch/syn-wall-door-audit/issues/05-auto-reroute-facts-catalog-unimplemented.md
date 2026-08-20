# 自动改路四条件事实目录无生产实现,改路评估整段显式未配置

Category: enhancement
Status: ready-for-agent

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
