# 自动改路四条件事实目录无生产实现,改路评估整段显式未配置

Category: enhancement
Status: needs-triage

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
