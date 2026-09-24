# 01 登记册 `PAR-NET-14` 写回拆分结论，被它挡住的旧票改去处

Category: enhancement
Status: resolved——2026-09-24 通道 5 完成（共享树 main，`6f484ad6`），完成记录见文末。此前：in-progress——2026-09-24 通道 5 认领（单 task-72652289-42a4-4bf3-8097-5bf12fbbf9d7），共享树 main 上做、逐文件 pathspec 提交、不推。此前：ready-for-agent
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

- [x] `PAR-NET-14` 一行有拆分结论，写法与 psb/02 一致。
- [x] 上列旧票各有去处，阻塞边指向实存的票；没有两张票同时是同一件工作的可执行来源。

## 完成记录（2026-09-24，通道 5，共享树 main）

**落点**：`6f484ad6`（登记册一行与三张旧票），本票票面随后一笔。

- **登记册 `PAR-NET-14`**：「最低证据」格末加「〔ADR-0146 拆分〕」一句，不删字。点名为判断方法、划为产品策略 → 票 04：「候选生成/过滤/排序规则」、日历截单与网络可用性的「采用方式」，以及「冻结边界」「已执行前缀和并发裁决」「自动/人工改路条件与权限」的判断结构。仍是租户取值：服务区域、业务时区、服务日历与截单的内容，冻结边界与改善阈值的取值，改路权限分派，选用哪种排序形态。行状态「待提供」不动。
- **`nr-route-evidence-views/01`**：needs-info → resolved；重启条件由 psb/04 替换，`NetworkEvidenceView` 取数侧移交 07，`InitialRouteEvidenceView` 移交 09、10。
- **`first-tenant-runway/03`**：blocked → resolved；解析层移交 07、09、10，Answer 的视图修订来源与 ADR-0068 同笔部分停用由 07 承接；Answer「二、`PAR-NET-14` 挡的是……」一节的结论记为已由 ADR-0146 决定七取代，正文不改。
- **`auto-reroute-demo-reachability/02`**：blocked → needs-triage；Blocked by 改指 08、10（另加 02 若登出「关务资格缺执行器」而另立的那张票）。

**判断项**

1. **冻结边界与改路条件用「……的判断结构」点名**，照 `PAR-NET-16` 已落的先例：登记规则说被点名的字此后不是租户证据，直接点名「冻结边界」会把它的取值也划出去，而取值仍归租户。
2. **只按决定七原文点名**：场外揽收的计划段或待路由揽收政策（租户选用哪种）、收寄与实测复核触发、集运兼容与路由指令生效规则、各类样本没点名，留作租户证据。后两格里若另含判断方法，另议，不在本票扩大决定七的拆法。
3. **`auto-reroute-demo-reachability/02` 选 needs-triage 而不是 ready-for-agent**：票面步骤写的是把 SYN 网络定义登进 `0007` 定义登记册的旧路，取数侧改读目录后要重写；阻塞边加 08，是因为它要走真实接受形成的判断键。
4. **另两张旧票选 resolved**：余下工作全由本票族承接，同一件工作不留两个可执行来源；原 Status / Blocked by 文字都以「此前：」保留。

**门**：纯 md，推送方自审。四份无 CR、无 BOM；新增链接逐个解析到实存文件；登记册该行列数与改前一致；提交前核过这四份相对 HEAD 只有本笔改动、索引里没有他人暂存。
