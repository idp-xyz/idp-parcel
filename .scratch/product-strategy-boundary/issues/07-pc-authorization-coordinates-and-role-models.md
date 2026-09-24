# 07 party-commercial：授权请求坐标的推导与各上下文的角色模型

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 随票 02 立（登记册逐行拆分划出的产品策略，PC 一张）；第 1 项先经 PC owner 复核改判，第 2 项先出角色模型草案
Blocked by: 无；角色能不能落到真人，受开发主线重定级表「横切」行第一项（ADR-0100 操作者渠道未落地）所限，那是机制缺口、不在本票
地盘：party-commercial 授权授予的领域与应用层（角色—动作的授予格）、`cmd/parcel-api` 里 `RequestSource` 的装配点；各上下文的动作清单由该上下文给，本票只收拢成角色模型。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md)——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-COM-13`、`PAR-COM-14`、`PAR-CUS-03`、`PAR-CUS-06`、`PAR-CUS-07`、`PAR-SET-05`、`PAR-GOV-05` 行内「〔ADR-0146 拆分〕」点名的部分；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二「角色模型（有哪些角色、各能做什么）归产品策略，把人分派到角色归租户取值」。

## 做什么

1. **授权请求坐标的推导**（`PAR-COM-14`「主动拒绝及决定前撤回的角色/客户授权」、`PAR-COM-13`「客户/运营代录请求方与实际决定方」）。`RequestSource` 生产为 nil（`cmd/parcel-api` 的 `assemble_review.go`、`assemble_withdrawal.go`、`assemble_customer_amendment.go`、`assemble_continued_attempt_decision.go`），代码注释判它属 `PAR-COM-14` 实例半边。从请求方与委托推出法人、权限等级与商业范围是方法，授权授予与原因目录才是租户取值。已知缺口：[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表「横切」行第三项（改判，待 PC owner 复核）。
2. **角色模型**：有哪些角色、各能做哪些动作，逐上下文收拢成可授予的格；人（或主体）到角色的分派留在登记册。登记册里已点名的：
   - 客户资料修订的请求方、决定方与委派（`PAR-COM-13`）；
   - 主动拒绝与决定前撤回（`PAR-COM-14`）；
   - 商业发布审批——形态已由 [ADR-0126](../../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 定为「批准者须否异于录入者」与「批准者须持的授予格」，本项只核它进角色模型后是否一致（`PAR-COM-18` 本行全为租户取值）；
   - 关务角色与结果处理权限（`PAR-CUS-03` 列举的各项权限即其草稿）、关闭请求 / 决定与受控重开（`PAR-CUS-06`）、内部合规限制的形成与解除（`PAR-CUS-07`）；
   - 供应商账单审核的申请 / 决定分权（`PAR-SET-05`；越权升级的判断结构归票 12）；
   - 紧急暂停值守（`PAR-GOV-05`）。
3. **待核：逐项时点的新鲜度与提交前复核**（`PAR-COM-14`「新鲜度和提交前复核规则」）。`UC-PC-002` 步骤 8 的提交前重解已有编排；核新鲜度判断是否在其内，缺的补。

## 不做

- 不把任何人分派到角色，不替租户定授权范围或原因目录。
- 不落操作者渠道本身（ADR-0100）。

## 完成判据

- 第 1 项经 PC owner 复核后有执行器，或记明改判不成立的理由；第 2 项有一份角色模型（落 PC `CONTEXT.md` 与授予格），各上下文 owner 各复核一次；登记册对应行同步收短。

## Comments

- 2026-09-25 · 通道 2（随 [ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) 复核）：第 2 项的动作清单还缺「授权处置」。party-commercial 的授权动作封闭集（`AuthorizedAction`）里没有这一格，`/shipment-requests/authorized-dispositions` 因此装的是 `UnconfiguredAuthorizedDispositionAuthorizer`、对每次处置答「授权规则未配置」（ADR-0132 越权风险点 2 写「归 PC 另票」，至今无票）。动作词是机制，谁持这一格是租户取值；加上之后把装配点的未配置授权器换成翻译适配器（形照复核那一只）。[operator-channel/15](../../operator-channel/issues/15-operation-decision-faces-take-operator-intake.md) 换上操作者 Intake 后，这一口越过 Intake 停的就是这一格。
