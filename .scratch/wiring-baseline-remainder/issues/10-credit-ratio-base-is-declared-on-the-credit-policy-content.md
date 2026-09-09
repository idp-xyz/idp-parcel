# 比例额度的基数由信用政策正文自己声明：封闭集、不给默认、缺席在构造门拒

Category: enhancement
Status: in-progress——2026-09-09 22:2x 通道 3 认领，分支 `mcp3-wbr10` 基 `90c025ca`，单 task-519de030-3eec-46c4-9468-f190e8926899。此前：2026-09-09 通道 1 代裁立票（用户经 IDP 队列授权「你自决，目标是全部解决」）：wbr/03「未落三件」之③、ADR-0127 决定四「比例额度的基数今天未裁……
那是 `BD-*` 一类，等它自己的裁决」。裁决方向见下；封闭集的成员表交本票 `/domain-modeling` 一格定、落 ADR-0129（号由通道 1 给）
Blocked by: 无（PC 半边与 SA 半边同票；SA 半边的 contract 段是 [09](./09-sa-credit-basis-contract-and-exposure-ledger-policy-reference.md)，本票不依赖它——`CREDIT_RATIO_BASE_UNDECIDED` 那格今天就在，本票是让它不再被走到）

## 缺口

`CreditLimit` 两格封闭（金额 / 比例），比例格的注释写「比例相对于什么基数由消费方的业务判断给出」；SA `exposeCredit` 遇比例额度停在 `CREDIT_RATIO_BASE_UNDECIDED`，
不折成金额、不默认。今天没有任何一处能声明「比例相对于什么」，所以**比例额度在产品里发布得出来、永远用不上**——与「发布得出来、管理台看不见」同一类缺口。

## 裁决方向（通道 1 代裁；owner 授权自决口径）

1. **基数是信用政策正文的一格，不是消费方的判断。** CONTEXT 说信用政策「按责任法人、业务角色、费用类型、金额或比例形成版本」且「只提供业务判断依据」——
   一个「30%」没有分母不是依据；分母属于同一条政策声明，登记它的人就是能说出「30% 的什么」的人。让 SA 猜是把租户的商业判断搬进消费方，ADR-0127 决定四
   拒绝这么做是对的，本票把那一格补到它该在的地方。
2. **封闭集、不给默认、缺席在构造门拒。** `CreditLimit` 比例格必须带 `CreditRatioBase`（封闭集）；比例在场而基数缺席 → `NewCreditLimit` 拒（`ErrInvalidCreditPolicy`
   一族），不再让它流到 SA 才停在`待判断`。金额格不带基数、带了拒（「含则必填、不含则必缺」，ADR-0044 同形）。
3. **封闭集成员由 `/domain-modeling` 一格定，判据：SA 凭自己的账本与状况登记能不能算出来。** 候选：`DEPOSIT_BALANCE`（该结算账户当前预付 / 保证金余额——
   SA 运营结算余额有它）、`PRIOR_PERIOD_CONFIRMED_CHARGES`（上一结算周期已确认费用合计——SA 确认费用记录按周期可汇）。**不收**「客户合同声明的基数金额」
   这类要 PC 再长一格的候选——那是第二张票，且今天没有任何 CONTEXT 句说合同有这一格。成员表与每格 SA 的取数路径写进 ADR-0129 Decision；拿不准的成员
   单列越权风险点，不写进集合。
4. **SA 折算**：`exposeCredit` 对比例额度按声明的基数取 SA 自己的数、乘比例、进 `CreditStanding.WithAuthorizedLimit`；基数取不到（账户无余额记录 / 无上一周期）
   落 `NotFormedReason` 既有的不可用格或新加一格（作者定、写理由），**不折成 0 也不折成无限**。`CREDIT_RATIO_BASE_UNDECIDED` 保留给重建门读到的存量正文
   （构造门之前登进去的、无基数的比例额度）——无租户故理论上为零，但重建门按 ADR-0028 只校验不重算，那一格是它的合法答案。
5. **发布路**：`creditPolicyBodyDocument`（受控批文）与 `CreditPolicyBodyPayload`（表单载荷，票 awf/16）各加 `ratioBase` 一格（可缺，比例在场时服务端点名）；
   PCC-1 不换号（加一键，ADR-0126 决定一同法，`canonicalCreditPolicyBody` 加 `ratioBase,omitempty`）；词表读口（awf/20）加 `CREDIT_POLICY` 的 `ratioBase` 一集；
   表单下拉不内置、不预选。0020 正文表加一列（PC 迁移，号顺延）。

**越权风险点（单列）**：① 「基数属政策正文」是从 CONTEXT「只提供业务判断依据」推的，CONTEXT 没有逐字写基数；② 封闭集候选两格是按「SA 能自算」选的，
业务上可能还要「合同声明基数」——本票明确不收，留给将来有租户提出时另立。

## 完成判据

比例额度不带基数在构造门拒；带基数的比例额度经 ADR-0127 那条路到 SA 后折成金额进授信额度（真库用例一正一反）；受控批文 / 表单 / 词表 / 读面各一格；
ADR-0129 落文、PC CONTEXT 信用政策那句补「比例额度声明其基数」半句、README 一行；含 DSN 跑 PC 四包 + SA + `cmd/parcel-commercial` + `cmd/parcel-dispatch`；
admin-web tsc / run-tests 绿。

## 边界

不动 ADR-0127 正文（回指即可）；不动结算政策；不给 PC 合同加任何格。
