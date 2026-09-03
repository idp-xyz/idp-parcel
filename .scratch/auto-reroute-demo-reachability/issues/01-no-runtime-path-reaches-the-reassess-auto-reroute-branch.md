# demo 里自动改路那条路走不到，而装配点看着像活的

Category: enhancement
Status: draft——依赖链未复核，见「开工前先复核依赖」
Blocked by: 见下（`synthetic-vertical-closure/design.md` 记的那条链，**该表已知有过期行**）

## 症状

隔离演示环境里 `ReassessRouteHandler` 永远走不到自动改路那一支：事实目录恒答未配置，
于是「失效照常落库、改路评估整段不做」，`rerouteAfterLapse` 的三态分派（自动改路 /
建议 / 禁行）一次也执行不到。

**而这在装配点上读不出来。** `cmd/parcel-dispatch/assemble.go` 已经把
`ReassessRouteDeps.AutoReroute` 从 `nil` 换成真适配器（`NewAutoRerouteFactsCatalog`），
读那一行只看得见「已经不是 nil 了」。这是本仓反复记的那一族：**已接线看着像可用。**

## 成因不是缺种子

立票 [`admin-write-faces/05`](../../admin-write-faces/issues/05-auto-reroute-facts-has-no-registration-entry.md)
时先把它当成「seed 加一行」，实现时才核出来建不了。取证：

- 读口按判断键取行——`reassess_route.go` 调 `LoadAutoRerouteFacts(ctx, trigger.Key())`。
- 那个键是 `InitialRouteJudgmentKey` 六维，后四维（委托请求、接受基线版本、申报包裹、
  服务目的）是**运行时产物**：委托提交 → 接受决定形成基线 → 逐包裹派生判断范围。
- 而 `seed.sh` 灌的全是配置类主数据。全仓种子数据里 `shipment_request` /
  `declared_parcel` / `acceptance_baseline` 一个都不出现（代收那份 `SYN-PARCEL-COD-01`
  是代收册自己发明的引用，不是路由过的真包裹）。

**所以静态种子只能造一个永远命不中的键，而那比空表更坏**：表非空了，目录看起来已配置，
每次真实复核仍拿到 `configured=false`——「未配置」与「配置了但不是这个键」在读口答案上
从此同形。空表至少如实说「这个判断键从未登记过事实」。

**这张票要的是一条运行时路径**，不是一行数据：委托提交 → 接受 → 初始路由 → 触发复核，
然后在那一刻按真实键登记事实。登记入口已经有了（`parcel-network-register -kind
auto-reroute-facts`，`admin-write-faces/05` 交付），缺的是把键喂给它的那条路。

`seed.sh` 不是这条路的落点——它是主数据灌入脚本，不是流程驱动器。

## 开工前先复核依赖

`.scratch/synthetic-vertical-closure/design.md` 的推进表里有一行 `CONS-INTAKE-REASSESS`
（「采用信封已有消费者；接 CONS-INTAKE 后跑第二跳」），它记的依赖是 `CONS-INTAKE`，而
`CONS-INTAKE` 又记着依赖 `PS-PARCEL-INDEX`（按包裹反查来源身份）。

**但那张表不能直接采信。** 同一行的「期望业务结果」写着「复核行落库；`AutoReroute`
仍 nil」——这句在 `syn-wall-door-audit/05` 交付四件并把 `assemble.go` 的 nil 换成真适配器
之后就不成立了，而写它的人没回来改。一行已知过期，其余行的现状同样得重取证再用。

所以：**认领本票的第一步是回到那张表逐行重核，而不是照它排期。** 复核完把结论写回本票，
再决定要不要拆实现票。

## 红线

- 不得为了让路走通而在 `seed.sh` 里塞一行事实——理由见上，那正是本票要避免的形状。
- 阈值与条件取值全属实例半边（`PAR-NET-14` 待提供）；跑通用的四条件只记 `S`。
- 不改 `cmd/parcel-dispatch/assemble.go` 里已经接好的那一行；本票缺的是上游，不是装配。

## Comments

- 2026-09-03 · MCP-4：立票。发现于 `admin-write-faces/05` 的实现过程——那张票原本写着
  「seed 加一份合成条件」，核不过去，遂撤销该项并把成因分出来独立成票。本票只写票面，
  未动任何代码，也未碰 `synthetic-vertical-closure/design.md`（不在本会话地盘；那行过期
  的话在此指出，由该目录的持有者决定改不改）。
