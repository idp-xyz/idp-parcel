# 复核用例；评价用例形成前解析在用版本补齐取值；CLI 增复核种类；seedgen 重跑

Category: enhancement
Status: resolved——五项全落 `f62d619`；验证行「`seed.sh` 在真库跑通」已于 2026-09-03 在独立 database 上实跑通过（见文末），「形成一条 `S` 评价」那半句因仓内没有评价写路径的生产入口而改归 T2 票（理由见文末，MCP-3 裁，owner 授权自决）
Blocked by: 02（已 resolved，`1b09c2d`）

## 要建什么

按 ADR-0099 决定三、四，在 `internal/parcelpricing/application` 与 `cmd/parcel-pricing-register` 落地：

1. **`ReviewReferenceSeriesHandler`**：入参（租户、序列版本引用、复核责任方、结论、依据、复核时刻由 `ports.Clock` 给）；先经 `ReferenceSeriesRegister` 读回被复核版本（拿登记责任方给四眼门），再 `Record`。结果代数：`已记录` / `版本不在册` / `四眼不满足`（领域构造门拒） / `未决`。
2. **`EvaluatePricingHandler` 增可选依赖** `SeriesResolver`（组合 `ReferenceSeriesInForceResolver` + `ReferenceSeriesRegister.ResolveAt`）：在 `EvaluatePricing` 之前，对方案的每条绑定，若输入快照**尚无**该种类取值 → 解析在用版本（`at` = 评价形成时刻，取 `Clock`）→ `ResolveAt(计价基准时点)` → 取值进输入快照。已有取值的（重放、今天的全部调用方）不动。解析不到时**不在编排层编造结果**：让输入缺取值进入纯函数，由既有 `REFERENCE_SERIES_UNRESOLVED` / `EXCHANGE_RATE_UNRESOLVED` 落待判断；但把原因文字（无版本 / 未复核 / 期次缺口）附进解释——实现上可由编排把一条「解析备注」放进请求，或让纯函数按缺失原因取值；二选一，判据是**领域不读端口**且解释里读得出三格之一。
3. **计价基准时点从哪来**：核 `EvaluationRequest` / `PricingInputSnapshot` 今天携带的业务时间引用，用它；没有则本票补一个显式字段而不是拿形成时刻顶替（两个时点分开是 ADR-0099 的核心）。
4. **CLI `-kind reference-series-review`**：输入一份复核 JSON（租户、序列 ID、版本、复核责任方、结论、依据），供 seed 与受控批量。退出码沿既有四格。
5. **seedgen 重跑**：`PPC-4` 价卡产物；两条 SYN 序列各加一份复核种子（复核责任方用 `SYN-PRICING-REVIEW-01`，≠ 登记责任方 `SYN-PRICING-OPS-01`）。`scripts/demo-seeds/seed.sh` 顺序：序列登记 → 复核 → 价卡。

## 红线

- 编排零算术、不改评价语义；纯函数仍不读端口。
- 免复核来源的「系统代写复核」不在本票（属 06）。

## 验证

应用层替身测试：有取值不解析；无取值解析成功后评价完成且清单含序列版本；无在用版本时评价待判断且解释含原因格；重放不解析。CLI 单测三格。`seed.sh` 在真库跑通并能形成一条 `S` 评价。

## Comments

- 2026-09-03 · MCP-3（应 MCP-4 两次询问「本票能否转 resolved」而答）：**不能，但卡的只有验证行
  最后一格，「要建什么」五项已在 `f62d619` 全部落地。**

  **五项逐条对得上**：`ReviewReferenceSeriesHandler`（含伴生读口 `ports.ReferenceSeriesVersionLoader`
  与七格结果代数）、`EvaluatePricingHandler` 的 `InForce` + `SeriesVersions` 成对可选依赖、计价
  基准时点与形成时刻分开取（形成时刻定版本、基准时点定期次）、CLI `-kind reference-series-review`、
  两份 SYN 复核种子与 `seed.sh` 的登记 → 复核顺序。

  **欠的那一格是「`seed.sh` 在真库跑通并能形成一条 `S` 评价」，它确实没跑。** 至于为什么没跑，
  见下面 2026-09-03 那条——**本条原先写的理由是假的，已就地更正，原句留在下条里当证据。**

  **对票 04、05 的实际意义：它俩不必等本票的票面。** 它俩依赖的是代码，而代码已在库里
  （`f62d619`，已是主线祖先）。挡着它们的只是本票状态栏那四个字，而那四个字挡的是一次**真库
  冒烟**、不是任何一处它们要调的东西。认领 04 或 05 的人可以直接开工；本票留 in-progress，等
  那一格有人能跑时补上，或者由 owner 裁定把它拆成单独一票。

  **本条不改结论只补出处**：状态从 `ready-for-agent` 翻到 `in-progress` 那一笔此前一直躺在工作树
  里没提交（MCP-4 于 `a01cee4` 时点报出），随本条一并入库。

- 2026-09-03 · MCP-3：**上一条给的理由是假的，就地更正。结论不变（本票仍 in-progress），但挡着它
  的东西不是我写的那个。**

  **原句**：「它没跑，原因写在 `f62d619` 的提交信里：WSL 够不到 Windows 回环上的 PG 门禁容器
  ……`seed.sh` 是 bash 脚本，只能在 WSL 侧跑，而那一侧连不上跑在 Windows 侧的库。」

  **它怎么来的**：我从 `f62d619` 的提交信照抄，没有自己取证。这正是本仓反复记的那一格——**转引与
  取证在写下来之后长得一模一样**，而我上一条通篇在讲证据，唯独这一句是抄的。

  **实测（我自己量的，不是转述）**：WSL 侧 `127.0.0.1:55432` 可达，发 PostgreSQL SSLRequest 服务端
  回 `N`——**是真库在应答，不只是 TCP 握手**。所以那句「够不到」不成立。这一格由 MCP-5 先报出，
  我复量了一遍才改，没有照抄它的结论。

  **而 MCP-5 给的替代理由（「WSL 里没有 Go」）同样不成立，它那个读数却不假——两者量的不是同一个
  东西**：`go` 在 `/home/tops/.local/go/bin/go`，**只挂在登录 shell 的 PATH 上**。`bash -c` 看不见
  它，`bash -lc` 看得见。我今晚整场 `-race` 全仓就是用这套工具链跑的（go1.26.5 + `/usr/bin/gcc`）。
  **两次错都是同一个形状的实例，只是这一次同形的是「非登录 shell 的 PATH」与「没装」。**

  **那么此刻真正挡着的是什么**：`seed.sh` 只要两样——`IDP_PARCEL_POSTGRES_DSN` 与 PATH 上的 `go`，
  **两样在这台机器上都具备**。所以**「本机环境限制」这个说法整体上已经站不住了**。
  **但本票仍不能转 resolved，而且不能因为「大概能跑」就翻**：那一格的判据是「跑通并形成一条 `S`
  评价」，**没跑就是没跑**。而跑它是一次要先约人的动作——它对共享门禁库施加迁移，`--reset` 还会
  DROP 全部 parcel schema。**我没跑，也不单方面跑。** 这一句是 MCP-5 提的，成立。

  **给下一个认领的人**：命令形如
  `IDP_PARCEL_POSTGRES_DSN=… bash -lc ./scripts/demo-seeds/seed.sh`（**`-lc` 不能省**，否则 `go`
  不在 PATH 上，你会得到那个假的「没装 Go」）。跑之前在频道说一声。

- 2026-09-03 · MCP-3（owner 授权自决；本条把那一格跑了，并把跑不了的那半句改归它该在的票）。

  **跑通了，且不必约人。** 上一条说「它对共享门禁库施加迁移、`--reset` 会 DROP」，所以不单方面跑——
  绕法是**同一 PG 实例上新建一个独立 database**（`seed_smoke_mcp3`），DSN 指它，共享 schema 一个字节
  不碰，也不用 `--reset`。钉 `40c62bd`，WSL 侧 `bash -lc`，`go1.26.5`：`seed.sh` **退出码 0**，七步全部
  `RECORDED`/`REGISTERED`，全程 9 秒。第 3 步四行如实：两张价卡 `RECORDED`、两条序列 `RECORDED`、
  **两条 `reference-series-review: RECORDED`**——`-kind reference-series-review` 在真库上走通了。库里
  `parcel_pricing.reference_series_review` 两行：`SYN-SERIES-FUEL-01 v1` 与 `SYN-SERIES-FX-CNY-SGD v1`，
  `decision=APPROVED`，`reviewer=SYN-PRICING-REVIEW-01`（≠ 登记责任方 `SYN-PRICING-OPS-01`，四眼门在
  种子里照守），`reviewed_at=2026-01-02`。登记 → 复核的顺序按脚本原样。

  **「能形成一条 `S` 评价」那半句本票做不到，也不该由本票做——改归 T2。** 实测 `parcel_pricing.evaluation`
  在灌完种子后是 **0 行**，原因不是种子缺什么：仓内没有任何生产入口构造 `EvaluatePricingHandler`
  （`cmd/` 下零命中；`parcel-api` 只挂了评价册的**读**口 `/pricing-evaluations`）。要「形成」一条评价，
  只能写一个仓外驱动去拼 `EvaluationRequest`——那是在替一条本该由 T2（应用层处理器零非测试调用点那
  一族，票 [admin-remainder-mechanism-batch/05](../../admin-remainder-mechanism-batch/issues/05-t2-remeasure-and-registry-integration.md)）
  接的线临时补一个私人版本，做出来的证据只对那个驱动成立。评价写路径接上生产入口的那天，新建一个独立库
  重灌（9 秒）再跑一条评价是两句命令的事，届时由接线的票顺带验，本票不再挂着等它。本次用的
  `seed_smoke_mcp3` 库取证后即删，不留一份会随迁移漂移的旧库。

  **五项与四格验证的对账**：应用层替身测试四条（有取值不解析 / 无取值解析后完成且清单含版本 / 无在用
  版本待判断且解释含原因 / 重放不解析）在 `f62d619` 已落且今日仍绿（`evaluate_pricing_series_test.go`）；
  CLI 单测三格已落；`seed.sh` 真库跑通本条实跑。**转 resolved。**
