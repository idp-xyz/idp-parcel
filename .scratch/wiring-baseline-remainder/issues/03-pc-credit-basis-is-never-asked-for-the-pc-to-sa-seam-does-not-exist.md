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
