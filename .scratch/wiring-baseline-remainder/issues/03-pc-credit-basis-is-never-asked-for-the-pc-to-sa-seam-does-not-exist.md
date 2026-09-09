# 信用政策正文已入册、`CreditBasis` 无人索取：PC→SA 的授信额度缝不存在

Category: enhancement
Status: resolved——2026-09-08 MCP-3（task-5031a8a1，分支 `mcp3-wbr03` 基 `2c0008c3`）落 ADR-0127 并实施判据 1–4 全部，自验 tip `eed205aa` 后在拆验证树那一步 crash（用户 17:4x 报）；通道 4 接手只做簿记（本完成记录 + 完工报）并在隔离检出独立重验 `eed205aa`，未改代码。进 main 的 SHA 由推送方重放后另记。此前 in-progress：2026-09-08 11:0x，MCP-1 代裁**甲**（owner 授权自决口径，见 Comments）：闭包解析加 `resolveCreditPolicyBasis` 一步、`ResolveCreditPolicy` 作四维选择器、`CreditBasis` 随 Resolution 交出，独立一篇 **ADR-0127**；判据 1 已裁，判据 2（提供方口 + SA 消费适配器，可碰 `settlementaccounting/adapters/partycommercial`）与 3（剪行按第二种）由 MCP-6 在 `mcp6-wbr03-05` 接。此前 blocked：2026-09-08，MCP-6（task 5f716c71，用户 02:0x 自 MCP-3 改派；分支 `mcp6-wbr03-05` 基 `4524cfd4`）：完成判据 4 的理由行已补进基线（条目上方七行），判据 1 那格「四维选择在哪一层还有多候选」经取证是**解析语义的改口、要 ADR**，按派单纪律停下报 MCP-1 裁。此前 draft：只读取证（MCP-6，锚 `2efef58e`），PC 地盘归 MCP-3；交 MCP-1 派
Blocked by: 无（「要先裁的一格」已由 MCP-1 2026-09-08 裁甲；与 `party-commercial-context-gaps/07` 的 ADR-0115 分立，独立 ADR-0127）

## 条目

`internal/partycommercial/domain ResolveCreditPolicy`（`credit_policy.go`）。基线理由行：无单独理由，落在 PC 组「三条」里；triage spec 记「整能力未接：信用政策无正文表」——**那句已过期**，正文表随 `party-commercial-context-gaps/03` 落地，见下。

## 它是什么

`ResolveCreditPolicy(policies, query)` 按（责任法人、权限等级、费用类型、时点）在一组信用政策正文里选唯一适用的一条，产出 `CreditBasis`（出自哪个政策版本、授权额度、`applicable`）；零候选答 `ErrNoApplicableCreditPolicy` 而不作答（注释：「缺政策既不是无限信用也不是零额度，该是哪一种只有拥有该商业依据的一方能说」），区间重叠答`适用冲突`。`CreditBasis` 的注释把消费方点了名：「是本上下文交给 settlement-accounting 的东西」。

## 已有的层（锚 `2efef58e`）

- 正文表与写口：`ports.CommercialRegistry.SaveCreditPolicy`（`party-commercial-context-gaps/03`），写入代数 `CreditPolicySaveOutcome`。
- 点读口：`ports.CreditPolicyContentView.LoadCreditPolicy(tenant, version)`，实现 `adapters/postgres/credit_policy.go` 的 `CreditPolicyContents`；目录读 `ListCreditPolicies`。
- 版本壳在闭包封闭集：PS 的商业依据解析键含 `CreditPolicyObject`（`parcelshipment/adapters/partycommercial/commercial_resolution_keys.go`）。

## 缺的层

- `LoadCreditPolicy` 在 PC 之外**零非测试调用**——正文登了没人读。
- SA 消费侧适配器 `settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go` 只读结算政策（预付 / 账期方式），正文里没有一处提到信用。
- `settlementaccounting/adapters/postgres/operational_position.go` 注释写「授信额度来自商业侧信用政策」，而 `credit_minor` 是**登记进来的状况事实**，不从 PC 读。于是信用暴露结果（`FinancialControlCreditExposed`，ADR-0047）形成时没有一份出自政策版本的额度依据可比。

## 该有的调用方

UC-SA-002 步 7「按已唯一解析的结算政策范围和商业策略形成估价、冻结、**信用暴露**或限制；不适用时保存依据」的账期分支——SA 消费侧适配器向 PC 索取 `CreditBasis`；PC 侧提供方口（用例或端口）按闭包选出的信用政策版本点读正文，多份候选按 `ResolveCreditPolicy` 选唯一 / 报冲突 / 报无依据。`AT-SA-171` / `AT-SA-172` 描述的正是这一步该答的形状（业务 B 只形成信用暴露 / 限制结果；同时命中预付与账期即模式适用冲突）。

## 三分

**支路未接，且先缺一条缝**（PC→SA 授信额度），与 triage 票 03 对 `FormSupplierExpectedCost`（BUY 评价→SA）的判法同形：缝本身是要做的机制，不是「等」。

## 要先裁的一格

闭包已按范围选出唯一版本壳，正文表一版一行——那 `ResolveCreditPolicy` 的四维选择在哪一层还有多候选可选？

- 若一版恒一行：它退化成对已选版本正文的一次 `covers` 校验（法人 / 等级 / 费用类型对不上即`无适用依据`），选择语义并入闭包——`ports.go` 自注这是「解析语义的改动，不是登记正文的连带」，得裁。
- 若同一政策版本下要按（法人、等级、费用类型）分多条正文：正文表形状要改（一版多行），`ResolveCreditPolicy` 才是它的选择器。

这与 `party-commercial-context-gaps/07`（接受前财务控制策略正文表未建）是同一族——控制**怎么做**与控制**依据多少额度**是相邻两格，建议同一轮 `/domain-modeling` 一起裁，很可能同一篇 ADR。

## 能不能归到已认可的留待

不能。SA 三口目录读口在认可留待，但那是登记面形状等实例证据；本条缺的是缝与执行器。

## 完成判据（落地那笔连理由行一起改；MCP-1 2026-09-07 裁）

1. 「要先裁的一格」有裁决（一版一行→并入闭包的 `covers` 校验，或一版多行→`ResolveCreditPolicy` 作选择器），与 `party-commercial-context-gaps/07` 同轮；落 ADR 时向 MCP-1 取号。
2. PC 侧有向 SA 供 `CreditBasis` 的提供方口（用例或端口），按裁决调 `ResolveCreditPolicy` 或经闭包读 `LoadCreditPolicy`；SA 消费侧适配器在 UC-SA-002 步 7 账期分支真索取它，形成信用暴露 / 限制结果时有出自政策版本的额度依据可比（`AT-SA-171` / `AT-SA-172`）。
3. 剪基线行：先按头注三分成因（全仓 `ResolveCreditPolicy` 只此一处声明才是第二种；若裁决把它并入闭包而删掉，则是第一种、头注记一句），在自己那笔的干净检出上两法同得记数、钉 SHA。
4. **若 1–2 之前先要补理由行**（今天这条**没有**理由行，PC 组注释只讲了另两条的族界），在条目上方加：

   > 信用政策选择门，`CreditBasis` 是本上下文交给 settlement-accounting 的授信依据。**调用方是 PC 侧向 SA 供授信额度的提供方口**（UC-SA-002 步 7 账期分支的消费侧适配器索取它），那条 PC→SA 缝今天不存在：缺提供方口、缺 SA 消费侧适配器，且「四维选择在哪一层还有多候选」那格未裁（与 party-commercial-context-gaps/07 同轮）。三件落地（wiring-baseline-remainder/03）那天这一条出名单。

## 边界

本票不改代码、不改基线。基线行剪掉的时刻是 PC 提供方口真调 `ResolveCreditPolicy` 那一笔。（立票时的边界；落地笔见 Comments。）

## 完成记录

分支 `mcp3-wbr03`，merge-base `2c0008c3`（main 此后到 `a69c16f0` 只多 `.md` 与 `apps/admin-web`，与本分支唯一重叠文件是 `docs/product/MECHANISM-INVENTORY.md`——生成物，推送方在 tip 重生成兑底；未 rebase）。作者 MCP-3；下表 SHA 为分支上的，作封存出处；进 main 的 SHA 由推送方重放后广播、届时并列补记。

| 分支 SHA | 内容 |
|---|---|
| `85c4209f` | feat(partycommercial)：闭包解析加 `resolveCreditPolicyBasis`（候选按版本租户+范围收窄，再由 `ResolveCreditPolicy` 选唯一 / `适用冲突` / `无适用依据`；光有版本没正文 = 无适用依据）；键上新增 `CreditSelector`（等级 × 费用类型，含则必填不含则必缺）、解析身份换代；`CreditBasis` 随 `Resolution.AdoptedCreditBasis` / `AdoptedBasis.CreditBasis` 交出；信用政策进 `CommercialRegistry` 与 `ViewRevision`；重建门收额度。`credit_basis_resolution_test.go` 九例 |
| `f0caf25a` | feat(partycommercial/postgres)：`LoadForScope` LEFT JOIN `0020`（一版一行不放大）、`registerCreditPolicy` 进册；闭包快照带 `credit` 选择器与 `creditBasis` 额度两格（金额 / 比例恰一）；`0020` 头注那句改以 `credit_policy.go` 头注与 ADR-0127 为准（迁移按 checksum 不动）。真库两例 |
| `fdf6819e` | docs(adr)：ADR-0127 决定三改一句（`0020` 头注守 checksum 不改） |
| `a532b500` | feat(settlementaccounting)：新端口 `CreditBasisView.LoadCreditBasis`（三格照 ADR-0054）；领域 `CreditBasis` / `CreditPolicyReference` / `CreditStanding.WithAuthorizedLimit`；`exposeCredit` 先索取依据再读状况、额度换政策授权金额、结果带 `CreditPolicy()`；比例额度停 `CREDIT_RATIO_BASE_UNDECIDED`；`NotFormedReason` 加三格；`Deps.CreditBasis` nil 沿旧路（三步法 expand 段，理由 ADR-0127 决定五）。应用层四例、领域两例 |
| `4162b421` | feat(settlementaccounting/partycommercial)：SA→PC 消费侧适配器 `CreditBasis`——凭回指从已固定闭包快照取额度与出处，不点读 `0020`；三格分明。真库四例 |
| `84191cb7` | feat(parcel-dispatch)：接受前财务控制装配接上 SA→PC 授信依据适配器（与控制策略视图共用同一只解析库、同一条回指） |
| `84b9a313` | chore(architecture)：剪 `production_wiring_baseline.txt` PC 段 `ResolveCreditPolicy`（成因第二种，理由行改写为历史并写明「出名单不等于缝全部闭合」的两件）与 `production_type_reachability_baseline.txt` 的 `CreditBasis`；两法同得 wiring 6→5（PC 4→3）、type reachability 23→22，钉父提交 `84191cb7`，只对该检出成立 |
| `eed205aa` | docs(product)：机制清点在 `84b9a313` 干净树上重生成（SA→PC 消费缝 1→2、SA 生产文件 78→80、测试 57→60，PC 测试 108→109，端口声明 365→366） |
| （本笔） | docs(scratch)：本票 Status → resolved + 本完成记录（通道 4 接手簿记） |

**逐条对完成判据**：1 「要先裁的一格」→ MCP-1 裁甲，ADR-0127 独立一篇（Accepted，Status 里写明代裁口径与落文时读过 / 未读的范围）；2 提供方口 = `domain/commercial_resolution.go` 的 `resolveCreditPolicyBasis`——`ResolveCommercialBasis` 对 `RequiredBasis == CreditPolicyObject` 转入它，候选按版本租户+范围收窄后交 `ResolveCreditPolicy` 选唯一 / `ErrCreditPolicyConflict`→`适用冲突` / 零候选→`无适用依据`；`CreditBasis` 经 `Resolution.AdoptedCreditBasis()` 交出，闭包侧 `ResolveCommercialClosure` 为信用成员用 `creditBasisKey()` 形成单依据键、把结果装进 `AdoptedBasis.CreditBasis()`，快照经 `adoptedDocument.creditBasis` 两格落库、`RehydrateAdoptedBasisSpec.CreditLimit/HasCreditBasis` 读回；SA 侧那只适配器 `settlementaccounting/adapters/partycommercial/credit_basis.go`（`NewCreditBasis(closures)`，nil 拒）凭 `CommercialResolutionReference` 取闭包、只译不判，`ApplyPreAcceptanceControlDeps.CreditBasis` 那一项在 `cmd/parcel-dispatch/assemble.go` 的 `acceptanceFinancialControl` 接上；`exposeCredit` 先索取依据再读状况，额度经 `CreditStanding.WithAuthorizedLimit` 换上，结果带 `CreditPolicy()`（`AT-SA-171` 政策半边成立；`AT-SA-172` 模式冲突仍由既有策略读口答）；3 剪行按第二种，两法同得、钉 SHA，见 `84b9a313`；4 理由行此前已由 MCP-6 补（`5df16244`），本轮剪掉时改写为历史。

**地盘之外多的三处**（占号广播 17:0x 已点名，都是新增、不改既有签名）：① `internal/settlementaccounting/domain`——新文件 `credit_basis.go`（`CreditPolicyReference`、`CreditBasis`、`NewCreditAmountBasis` / `NewCreditRatioBasis`），`credit_exposure.go` 加 `CreditStanding.WithAuthorizedLimit` 与 `LimitMinor`；② `internal/settlementaccounting/ports`——新端口 `CreditBasisView`；③ `internal/settlementaccounting/application/apply_pre_acceptance_control.go`——`Deps` 加 `CreditBasis` 一项（nil 沿旧路 = expand 段）、`NotFormedReason` 尾部追加三格、结果加 `CreditPolicy()`。装配点在 `cmd/parcel-dispatch/assemble.go`（不在 `parcel-api`），只加一只适配器与 `Deps` 一行。

**验证**：
- 作者 MCP-3（据用户提供的崩溃前终端截图，钉 `eed205aa` detached 检出）：gofmt 空、build / vet 0；全仓 `go test -p 1 -count=1` 17:31:45→17:42:01 exit 0，ok=100 / FAIL 0 / no-test 16；探针 SA→PC 适配器包无 DSN PASS=3 SKIP=10。拆验证树一步退 255 后 crash。
- 通道 4 独立重验（干净 detached 检出 `eed205aa`，`$env:TEMP\idp-verify-wbr03-mcp4`，已拆、不带 `--force`）：gofmt -l 空；`go build` / `go vet ./...` 退 0；含 DSN `go test -p 1 -count=1 -v ./...` **7548 PASS / 0 FAIL / 0 SKIP，100 包 ok / 0 FAIL / 16 无测试**，17:48:07→17:58:01；探针 `internal/settlementaccounting/adapters/partycommercial` 无 DSN PASS 3 / SKIP 14、有 DSN PASS 23 / SKIP 0；同检出重跑清点生成器零差。`-race` 本机无 cgo 未跑。

**红线自查**（通道 4 读了 `2c0008c3..eed205aa` 全部代码 diff，28 文件）：触及 `internal/partycommercial/{domain,ports,adapters/postgres}`、`internal/settlementaccounting/{domain,ports,application,adapters/partycommercial}`、`cmd/parcel-dispatch/assemble.go`、两份 architecture 基线、ADR-0127 + README 一行、清点；**未碰** `internal/parcelshipment/**`、`migrations/**`（`0020` 不动）、`authority_grant.go` / `acceptance_content.go` / PC http；实例值（等级、费用类型、额度）全由夹具给、无默认；比例额度基数未裁 → 停格不折算。

**未落 / 拆出**（ADR-0127 Consequences 点名，归各自地盘）：① PS 登记面 `commercial_resolution_keys.go` 不承载信用二维，含 `CreditPolicyObject` 的登记行会形成立不起来的键——PS 另立票；② SA contract 段（`Deps.CreditBasis` mandatory 化）与暴露账本行持久化政策引用（要 SA 迁移）——SA 后续项，随 PS 夹具补 `CreditBasisView` 替身那笔一起落；③ 比例额度的基数——`BD-*` 一类，等自己的裁决。

**越权风险点**（供评审）：① 解析身份换代（`RES-` / `CLO-` / `CONT-` 指纹含 `CreditSelector`）——无租户故无迁移，但这是键形变化；② expand 段留 nil 沿旧路一格——ADR-0127 决定五写明 contract 何时收；③ `NotFormedReason` 新增三格未经产品判断单独裁；④ `resolveCreditPolicyBasis` 冲突格 `candidateCount` 固定记 2，与既有 `resolvePriceRuleBasis` / `resolveSettlementPolicyBasis` 同款——三份以上候选时数字不准，是沿用的既有取舍，不是本票新开。接手方通道 4 读过全部 diff 但非作者、未跑 `/code-review` 两轴——语义评审仍靠非作者通道。

## Comments

- 2026-09-08 02:3x · MCP-6（task 5f716c71；分支 `mcp6-wbr03-05` 基 `4524cfd4`）：**补理由行 + 取证「要先裁的一格」，停下报 MCP-1。**
  基线 `ResolveCreditPolicy` 条目上方按完成判据 4 加了理由行（调用方是谁、缝缺哪两半、未裁的一格是什么、哪天出名单），名单一行
  未动（两法同得 6 / PC 4，与 05 剪后同）。**取证**（锚 `4524cfd4`）：`0020_credit_policy.sql` 的 `credit_policy` 主键是四元组
  （tenant, object_kind, object_id, version_label），**一版一行**；`domain.CreditPolicy` 一行带（法人, 等级, 费用类型, 额度），
  `ResolveCreditPolicy(policies, query)` 对一组行按 `covers` 选唯一 / 报冲突 / 报无依据。所以「多候选」不可能来自同一版本的多行，
  只能来自**同一范围内多个信用政策对象各自的生效版本**——这与结算政策同形：`commercial_resolution.go` 的 `resolveSettlementPolicyBasis`
  已经在闭包解析里按六维选结算政策（零候选`无适用依据`、多候选`适用冲突`），信用政策今天却没有对应的 `resolveCreditPolicyBasis`
  一步，闭包只把 `CreditPolicyObject` 当版本壳按范围选。**两条路**：(甲) 照结算政策的形，在闭包解析里加一步、`ResolveCreditPolicy`
  作四维选择器、`CreditBasis` 随闭包交出——`ports.go` 自注这是「解析语义的改动，不是登记正文的连带」，要 ADR；(乙) 闭包只认唯一
  版本壳、`ResolveCreditPolicy` 退化成对已选版本正文的 `covers` 校验——同样改解析语义（对不上答`无适用依据`），也要 ADR，且
  多法人 / 多等级的租户只能把每一格拆成不同范围。我的倾向是甲（与结算政策一致、不逼租户拆范围），但这是难逆转的解析语义取舍，
  派单纪律写明「先停下报 MCP-1，不在实施票里顺手定」，故本笔不裁、不建缝。SA 侧的形已看过：`settlementaccounting/adapters/
  partycommercial/pre_acceptance_control_policy.go` 只读结算政策，账期分支要的 `CreditBasis` 无处来；接线时是那只适配器旁加一只
  读 PC 闭包交出的信用依据、装配在 SA 的接受前控制编排。**待 MCP-1**：裁甲 / 乙 + 取 ADR 号；裁后本票判据 2、3 另笔接（可能落
  同一 ADR 于 pc-gaps/07 的 ADR-0115 之后作补充记录，或独立一篇）。
- 2026-09-08 11:0x · MCP-1 代裁（owner 授权自决口径；由 MCP-6 落票面）：**裁甲。** 照结算政策的形，在闭包解析里加
  `resolveCreditPolicyBasis` 一步（镜像 `resolveSettlementPolicyBasis`：候选先按版本的租户与范围收窄，再由 `ResolveCreditPolicy`
  按（法人、等级、费用类型、时点）选唯一 / `适用冲突` / `无适用依据`），`CreditBasis` 随 Resolution 交出。理由：价格规则与结算
  政策两个政策类对象已在同一闭包里用类别专属选择器解析（ADR-0044 镜像 `resolvePriceRuleBasis`），信用政策是第三个；乙会让同一
  闭包里两套解析语义并存、且逼租户把（法人、等级）拆成范围——正是 `ports.go` 自注那句要拦的。**号 ADR-0127**（0126 在
  `mcp5-awf08`，0127 全 ref 无文件，MCP-1 10:5x 查），独立一篇、不作 ADR-0115 补充；Context 里写「此前 `ResolveCreditPolicy`
  零生产调用」时钉 SHA 不写行号。判据 2 的 SA 消费侧适配器在 SA 地盘，本单可碰 `settlementaccounting/adapters/partycommercial`；
  判据 3 剪行按**第二种**（真接上）。Status blocked→in-progress，Blocked by 清；ADR-0127 + 判据 2–3 由 MCP-6 接着做，每小步提交并推。
- 2026-09-08 17:5x · 通道 4（接手 MCP-3 task-5031a8a1；用户 17:4x 报通道 3 crash 并指令接手，已知会 MCP-1）：MCP-3 已在分支
  `mcp3-wbr03` 落齐 ADR-0127 与判据 1–4（八笔到 `eed205aa`，树 status 零行、与 origin 同步），崩溃前自验全绿、停在拆验证树那一步；
  本票面此前一字未改。接手只做三件：清掉残留空壳 `idp-verify-wbr03`（`.git` 链接已无、worktree 登记已无、目录空），在隔离检出独立重验
  `eed205aa`（见「完成记录·验证」），写本完成记录并发完工报。未改任何代码——「先写自己的 red 再读对方代码」那条适用于接手在途实现，
  这里实现已完、验证已绿，接手的是簿记；语义评审留给非作者通道。

- 2026-09-08 18:2x · MCP-5 **非作者合入前评审**（改派自 MCP-6；`/code-review` 两轴，钉 `mcp3-wbr03` 代码 tip `eed205aa`，票面 tip
  `05e0e80f`，评审基线 merge-base `2c0008c3`；隔离 detached 检出 `%TEMP%\idp-review-wbr03`，验后拆，不带 `--force`）。两轴串行自跑、
  分开记、不合并排序；逐行读了 `2c0008c3..eed205aa` 全部代码 diff。**自跑**（含 DSN）：`gofmt -l ./cmd ./internal` 空；
  `go build ./...` / `go vet ./...` 退 0；`go test -count=1 ./internal/architecture/...` ok；
  `go test -count=1 ./internal/partycommercial/... ./internal/settlementaccounting/... ./cmd/parcel-dispatch/...` 全 `ok`
  （PC postgres 68.8s、SA postgres 41.4s、parcel-dispatch 30.2s——真 PG 跑过）。基线两数我在 `84191cb7` / `84b9a313` / `eed205aa`
  三个检出上用 `git grep -E '^[^#[:space:]]'` 复核：wiring 6 / 5 / 5（PC 段 4 / 3 / 3）、type reachability 23 / 22 / 22，与头注一致。
  照「全量只跑一次」不跑全仓。
  - **Standards · 阻断：无。非阻断 1**：
    1. **装配点无探针**——`cmd/parcel-dispatch/assemble.go` 的 `acceptanceFinancialControl` 接 `sapartycommercial.NewCreditBasis` 那三行，
       `cmd/parcel-dispatch` 的测试里零处断言（`NewCreditBasis` / `CreditBasis:` 在 cmd 测试零命中）；「生产装配已接真适配器」今天只由
       读代码与编译作证，与 wbr/04 在 `parcel-api` 装配层种真 grant 的三格相比少一层。非阻断：三行、编译过、`Deps` 字段名与端口类型
       都由编译器守；但 `Deps.CreditBasis` 为 nil 是合法的（见 Spec 越权点 ②），漏接不会红——这正是需要一枚探针的场合。建议随 SA
       contract 段那笔一起补（contract 收了之后漏接会在构造期红，探针可省）。
    - 其余核过：领域包无 HTTP / pgx 依赖（PC / SA `domain` 只新增值对象与解析函数；`internal/architecture` 边界门禁绿）；
      `resolveCreditPolicyBasis` 与 `resolveSettlementPolicyBasis` 逐段同形（收窄 → `NewXxxQuery` 失败即`输入未受理` → 选唯一 /
      `ApplicabilityConflict` / `NoApplicableBasis`），`CreditSelector` 的 `declared` / `empty` / `fingerprint` 与 `SettlementSelector`
      同纪律；SA `CreditStanding.WithAuthorizedLimit` 只加不改（副本返回、原值不动），`LimitMinor` 只加读口；SA 适配器 `credit_basis.go`
      与 `pre_acceptance_control_policy.go` 并列同形（回指换闭包 → 取成员 → 三格；`scope` 收下不问、理由同一条）；`creditBasisFrom`
      是两格封闭之间的全函数、两空走 error 不吸收；重建门拒「额度不挂在 `CreditPolicyObject`」与「零值额度」（ADR-0028）。注释全中文；
      跨文件引用皆符号名或 ADR 号；基线头注「6→5 / 4→3 / 23→22 钉父 `84191cb7`」是「数本身是论点」的合规写法；ADR-0127 Context
      钉 `2c0008c3` 不写行号；加了第三个政策选择器后 `commercial_registry.go` 头注已从两类改三类，全仓未搜到残留的「两个政策」类计数。
      测试覆盖：PC 域选唯一 / 冲突 / 无依据（裸版本、他范围）/ 选择器必填必缺 / 闭包携带与冲突 / `ViewRevision` 变化 / 重建门收与拒；
      SA 适配器命中 / 未配置 / 坏回指 / nil 拒装；SA 编排额度取政策非登记、已占用仍取自家账本、三格各停在读状况之前、nil 沿旧路不带
      政策；真库 PC 快照往返与 `0020` 进册两路。
  - **Spec · 阻断：无。非阻断 0。** 对 11:0x 裁决甲逐句：镜像 `resolveSettlementPolicyBasis` ✓（候选先按版本租户与范围收窄，再由
    `ResolveCreditPolicy` 按法人 / 等级 / 费用类型 / 时点选唯一 / `适用冲突` / `无适用依据`；法人取 `LegalEntityCandidate`、时点取锚点，
    键上不重复携带）；`CreditBasis` 随 Resolution 交出 ✓（`Resolution.AdoptedCreditBasis` → 闭包 `AdoptedBasis.CreditBasis` →
    `creditBasisDocument` 两格入快照 → `RehydrateAdoptedBasisSpec.CreditLimit / HasCreditBasis` 读回，出处即本项 `Version` 不另存）；
    SA 侧那只适配器旁加一只且装配进接受前控制编排 ✓（`credit_basis.go` 与 `acceptanceFinancialControl`）；剪行按第二种 ✓（复核见上）；
    ADR-0127 独立一篇不作 0115 补充 ✓，Context 钉 SHA ✓。完成判据 1–4 逐项在场（1 裁甲入 ADR；2 提供方口 = `resolveCreditPolicyBasis`
    经闭包交出、SA `exposeCredit` 先索取依据再读状况、额度经 `WithAuthorizedLimit` 换上、结果带 `CreditPolicy()`；3 剪行钉 SHA；
    4 理由行改写为历史）。未确认参数：`CreditSelector` 两维由键（租户实例）给、无默认；比例额度基数未裁停 `CREDIT_RATIO_BASE_UNDECIDED`
    不折算 ✓；`Deps.CreditBasis` nil 沿旧路——旧路取的是登记状况 `limit_minor`（一条登记事实，非写死常量），生产装配已接真适配器，
    所以不构成「未确认参数写死为生产默认」，属机制半边的过渡格（见下 ②）。`AT-SA-171` 政策半边在形成时成立、账本行持久化归 SA 后续
    并已在 ADR Consequences 点名——判据 2 原话是「形成……时有出自政策版本的额度依据可比」，满足。
    - **四条越权点，各一句「是否在裁决字面内」**：
      ① 解析身份换代（`RES-` / `CLO-` / `CONT-` 指纹含 `CreditSelector`）——**在字面内**：裁决要「镜像 `resolveSettlementPolicyBasis`」，
        选择器进键即进指纹是 ADR-0044 先例的必然后果，ADR-0127 决定二如实记了；无租户无生产固定解析，不需迁移。
      ② `Deps.CreditBasis` nil 沿旧路——**字面外、不相悖**：裁决只说「旁加一只且装配进编排」，没说依赖强弱；作者为不碰 PS 夹具留 expand
        段并在 ADR 决定五写明 contract 何时收，取舍合三步法。风险在「漏接不红」（见 Standards 1），SA contract 段是该收的那一格。
      ③ `NotFormedReason` 新增三格——**字面外、合红线**：三格照 `PreAcceptanceControlPolicyView`（ADR-0054）的三格形状译过来，
        `CREDIT_RATIO_BASE_UNDECIDED` 正是「未确认参数显式未决、不写死」那条红线在代码上的样子；值追加在 iota 尾部不改既有序。产品是否
        要另裁这三格的措辞归 owner，评审只判它不与裁决相悖。
      ④ 冲突格 `candidateCount` 固定记 2——**在字面内**：镜像的就是 `resolveSettlementPolicyBasis` / `resolvePriceRuleBasis` 的既有写法，
        三份以上候选时数字不准是三处共有的沿用取舍，不由本票新开；要改应三处一起、另立票。
    - 备案（作者已记的未落三件，同意归各自地盘）：PS 登记面不承载信用二维 → PS 另立票；SA contract 段 + 账本行政策引用 → SA 后续；
      比例额度基数 → `BD-*`。
  - **结论**：Standards 阻断 0 / 非阻断 1；Spec 阻断 0 / 非阻断 0；越权点四条皆不与裁决相悖（两条在字面内、两条字面外但合红线 / 三步法）。
    **可进 main**。本条写在分支 `mcp5-wbr03-review`（基 origin/main `140bce84`，只动本文件），与作者分支上的 `94eda949` / `05e0e80f`
    两笔票面同在文末追加，合并时作者两笔在前、本条在后。

## 进 main 记录（2026-09-08 18:3x，通道 1 重放）

- **分支→main 逐笔**（`git cherry-pick` 于 `140bce84` 之上，隔离 detached 树 `idp-replay-wbr03`，随后 rebase 到 `bfde8108`——中间 main 只多一笔 `scripts/branch-state.ps1` 修复，不碰 Go）：`95193261`（ADR-0127 立篇）、`85c4209f`、`f0caf25a`、`fdf6819e`、`a532b500`、`4162b421`、`84191cb7` 七笔零冲突；`84b9a313` 在 `production_wiring_baseline.txt` 与已进 main 的 wbr/04（`d41862bd`）相撞——头注两段各留（04 在前、03 在后）并加一段推送方重放注钉父提交，PC 段两条目都出名单（`ResolveCreditPolicy` 与 `ManualReviewRequirementFor` 各自的「曾在这里」注都留），`production_type_reachability_baseline.txt` 干净落下；`94eda949`、`05e0e80f` 票面两笔零冲突；评审 `e53bd152`（Comments 末与作者两笔同 hunk，作者在前、评审在后，作者 27 行 / 评审 51 行逐行在场）；本笔只加本节 + spec 状态行。代码接手方 MCP-4 交的逐笔漏了 `95193261` 与 `05e0e80f`，重放按 `2c0008c3..05e0e80f` 全列补齐。
- **main 上的 SHA**（rebase 后定稿，补记于推后）：`95193261→0794297a`、`85c4209f→c89b89ee`、`f0caf25a→f2b043f6`、`fdf6819e→d7d2b6ea`、`a532b500→064fac32`、`4162b421→6d8f11a2`、`84191cb7→4cb991f4`、`84b9a313→3642d35f`、`94eda949→19da45db`、`05e0e80f→17caa7d4`；清点 `7127f404`；评审 `e53bd152→cbd6f7c9`；簿记 `6d6e95cb`。
- **不带**：`eed205aa` 清点笔——它基 `2c0008c3` 缺 TF 0019 与 wbr/04，数字只对该检出成立；在重放 tip 干净检出重生成（settlementaccounting 生产 78→80 / 测试 57→60、partycommercial 测试 108→109、SA→PC 消费缝 1→2、端口 366→367、合计 858→860 / 807→811）。
- **树等价**：分支触及文件里对 tip 仍有差的只有 `MECHANISM-INVENTORY.md`（上一条）与 `production_wiring_baseline.txt`（只差 wbr/04 那半：`ManualReviewRequirementFor` 条目已出名单 + 04 的头注与「曾在这里」注 + 推送方注），其余零差。
- **验证钉 `075c71eb`**（rebase 前的 tip；之后只多 `.ps1` 与 .md）：gofmt -l 空；go build / go vet 退 0；`internal/architecture` 门禁 ok（并后基线两条目都出名单、门禁仍绿）；含 DSN `go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL / 16 无测试 / 0 cached**（18:11:33→18:21:16，`partycommercial/adapters/postgres` 62s、`cmd/parcel-dispatch` 26s 非缓存）；日志 `%TEMP%\mcp1-wbr03-fulltest.log`。
- **评审非阻断**（Standards 1 / Spec 0，见 Comments）随票记：`cmd/parcel-dispatch` 接 `NewCreditBasis` 那几行在 cmd 测试零断言、Deps.CreditBasis nil 合法故漏接不红——随 SA contract 段一起收。**归 owner 复核**：越权风险点 ①–④（评审判 ① ④ 在字面内，② ③ 字面外不相悖）。spec 状态行本笔改：PC 三条 03/04/05 全部进 main，PS 二条 01/02 仍 draft 待派。
- **owner 复核 2026-09-09 认可**（用户经 IDP 队列通道 1 授权代裁，越权风险点 ①–④ 逐条）：① 解析身份换代——无租户、无存量键，键形变化此刻代价最低（ADR-0014 那句），且不换就等于让登记了信用政策的范围与没登记的算同一个解析身份；② expand 段留 nil——ADR-0127 决定五自己写明 contract 何时收，是三步法不是容忍，contract 段已立票 [09](./09-sa-credit-basis-contract-and-exposure-ledger-policy-reference.md)；③ NotFormedReason 三格——机制半边的结果代数，各答一种恢复动作（补装配 / 补登记 / 等裁决），不是产品判断，产品要并格走 ADR-0127 supersede；④ candidateCount 固定 2——沿用既有取舍，三个解析器同款，不在本票单改，若要修一并修（不立票，等触及时顺手）。**未落三件**去向：① → [08](./08-ps-resolution-key-registration-carries-the-credit-selector.md)（取补两维）；② → [09](./09-sa-credit-basis-contract-and-exposure-ledger-policy-reference.md)；③ → [10](./10-credit-ratio-base-is-declared-on-the-credit-policy-content.md)（基数由政策正文声明）。
- **未落三件之② 已落（2026-09-09 20:5x 通道 3，分支 `mcp3-wbr09` 基 `d5a35960`）**：contract 段 `d45a7a34`（`ApplyPreAcceptanceControlDeps` 七件 mandatory、`NewApplyPreAcceptanceControlHandler` 返 `(handler, error)`、`exposeCredit` 删旧路），暴露账本行政策引用 `d23956d7`（迁移 0017 `credit_policy_ref` + `CreditExposure.Policy()` 经重建门与 postgres 往返）。上条「进 main 记录」里的评审非阻断「Deps.CreditBasis nil 合法故漏接不红」一并收——漏接现在在装配期以 `ErrNilDependency` 拒。完成记录与判断题在 [09](./09-sa-credit-basis-contract-and-exposure-ledger-policy-reference.md)；进 main 后的 SHA 由推送方在那里对照。
