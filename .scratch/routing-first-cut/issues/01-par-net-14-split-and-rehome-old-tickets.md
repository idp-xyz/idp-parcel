# 01 登记册 `PAR-NET-14` 写回拆分结论，被它挡住的旧票改去处

Category: enhancement
Status: draft
Blocked by: [psb/02](../../product-strategy-boundary/issues/02-split-parameter-register-and-retriage-deferrals.md) 登记册那一笔落地（写法照它；它落地并报 SHA 后，在共享树上单独 pathspec 提这一行，避开邻行冲突）
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「拆 `PAR-NET-14`」那一步
地盘：[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-NET-14` 一行（其余行归 psb/02）；下列旧票的状态行与阻塞行。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七（拆法原文）与 Consequences「参数登记册……此后只装租户取值」。

## 做什么

1. `PAR-NET-14` 按 ADR-0146 决定七拆：产品策略那半指向本票族（psb/04 的子票），租户取值那半留在本行。行状态仍只由租户取值决定，不因产品策略落地而改。
2. [`nr-route-evidence-views/01`](../../nr-route-evidence-views/issues/01-cut-the-mechanism-half-of-par-net-14-from-its-rule-values.md) 的重启条件（登记册该行状态变化）由 psb/04 替换：写明去处，状态按 [triage-labels](../../../docs/agents/triage-labels.md) 选。
3. [`first-tenant-runway/03`](../../first-tenant-runway/issues/03-network-resolution-layer.md)（网络解析层）与本票族的 07、09、10 是同一件事。同一件工作不留两个可执行来源：按票面写明由哪几张子票承接后收口；其 Answer 里仍成立的结论由 07 承接，不在此复述。
4. [`auto-reroute-demo-reachability/02`](../../auto-reroute-demo-reachability/issues/02-syn-vertical-run-reaches-reroute-after-lapse.md) 的阻塞边改指让初始路由真能形成计划的那张子票（10）。

## 不做

- 不改登记册其余行；不给任何租户取值填值。

## 完成判据

- [ ] `PAR-NET-14` 一行有拆分结论，写法与 psb/02 一致。
- [ ] 上列旧票各有去处，阻塞边指向实存的票；没有两张票同时是同一件工作的可执行来源。
