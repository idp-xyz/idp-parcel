# ADR-0339：费用补拍确认与客户对账单发布是两种运营决定；登了对账周期的账户在每个截止日关上之后自动截单

Status: Accepted（2026-10-28，Claude Code 会话代 SA owner 决（代决）：用户 2026-10-27 对会话提出的拆笔答「按你的建议开始吧」、2026-10-28 答「开」，随后对会话「首发不做到点截单」的取舍答「自动截单」，决定三据此改为到点截单；票 product-strategy-boundary/22 第 2 笔。回答 [ADR-0269](./0269-business-time-zone-and-cutoff-belong-to-the-statement-arrangement-of-the-settlement-account-and-the-cut-admits-by-attribution-date.md) 越权风险点 5「周期日历不在产品里」：登了对账周期的账户，本期最后一个归属日由到点截单照周期点名。越权风险点待 SA owner 复核。）<!-- 索引注记 -->｜**入口**：确认与截单编排已装在 parcel-api，却没有任何口能调到；截单的触发策略没定。**取法**：开两个运营决定口，各一种授予；另立对账周期册，登了周期的账户由 dispatch 在每个截止日关上之后自动截单；客户费用读口多透出归属日与所在对账单，供人列清单。
Date: 2026-10-28

## Context

取证于 main `2d0193336`，只记 `S`。

- ADR-0170 把确认与截单装在 parcel-api，前面挂了结算时点一道门，没有口能调到它们。ADR-0336 决定三定了要有运营确认口，形态留给本记录。
- 截单编排要人给的东西都在命令上：本期列哪几笔费用、本期最后一个归属日、申报合计、单号、账期与草稿版本、截单时刻。编排逐件判入不入本期，申报合计与入本期的行勾稽，不平即阻断。
- SA CONTEXT「对账安排」：「对账周期的日历不在这本册里，本期边界由截单命令点名」。产品里没有任何一处登记某户的账期日历。
- UC-SA-003 的触发一格写着「账期到达截单时刻、授权人员生成草稿/发布」。
- 客户费用读口不透出归属日，也不说这笔费用在不在某张对账单上，人列清单时无从判断。

## Decision

**一、运营确认口。** `POST /settlement-charge-confirmations`，挂操作者渠道「运营决定」能力面，决定种类 `CHARGE_CONFIRMATION`。载荷只点名一笔费用（`charge`），租户取认证结果。进 parcel-api 的 `settlementOrchestrations.Confirm`：先过确认时点门，再调确认编排，与 dispatch 那一路自动确认（ADR-0336）是同一道门、同一只编排。答复：时点没登答 `200 SETTLEMENT_MOMENT_UNCONFIGURED`、不调编排；确认了答 `201 CHARGE_CONFIRMED`；已确认、条件未满足、确认条件或确认事实没登都是答案（`200`）；时点册或编排依赖读不动、确认信封没交出去是没形成答案（`500`）。

**二、截单发布口。** `POST /settlement-statement-publications`，同一能力面，决定种类 `STATEMENT_PUBLICATION`，与确认分开授予。载荷照截单命令逐格收：单号、账户、账期、草稿版本、币种、费用清单、本期最后一个归属日、申报合计（按最小币单位的整数串）、截单时刻。发布时刻不在载荷里，取服务端时钟，因为它是这一动作落下的时刻。进 `settlementOrchestrations.CutOff`，先过截单时点门。答复判据同决定一；勾稽不平、本期无费用、截单未配置都是答案，逐件带没入本期的缘由。

**三、登了对账周期的账户到点自动截单。** 「何时截单」是产品策略，产品内置到点截单这一种执行器，运营发布口留作补截与没登周期的账户。

1. **对账周期册**按（租户、结算账户）登一份：切法是日截、周截（点名星期几）或月截（点名 1–28 号，或 0 表示月末），外加起算日。用哪种切法、哪天截是租户取值，没登就不自动截。对账周期仍是对账安排的一部分（CONTEXT「对账安排」词条），登在对账安排之下的第二本册上：业务时区与截单时刻那一本按版次追加、只作用于此后形成的归属日，周期一改要接上已截的各期，变更规则不同，所以分册。ADR-0269 决定一「对账周期的日历仍不在这一册里」与 CONTEXT「本期边界由截单命令点名」照旧成立：周期不在业务时区与截单时刻那一本里，截单命令仍点名本期最后一个归属日，只是由执行器发出。起算日只定第一个截止日从哪天之后算起，不是费用的下限：起算日之后的第一期把此前没出账的已确认费用都收进来。
2. **截止日**就是那一期的最后一个归属日，在对账安排业务时区里那一天的截单时刻关上。归属日在那一刻之后落到下一日，这一期不会再长出新的费用。
3. **执行器**在 `cmd/parcel-dispatch` 每一拍之后逐户跑，一户一笔事务。从截止记录最后一天之后、起算日之后算起，把此刻已关上的截止日按先后各截一期：
   - 收已确认、归属日不晚于截止日、结算币种是这一户币种、不在任何未作废已发布单上的费用；
   - 申报合计取它们结算金额之和；
   - 单号与账期从截止日派生，形如 `STMT/<账户>/<YYYYMMDD>` 与 `CYCLE/<截止日>`，版本 `v1`；截单时刻取截止日的截单时刻，发布时刻取此刻。
4. **截单时点门先问**：这一户没登 `CUT_OFF` 就什么都不做，不截、不记截止日。确认时判不出归属日的已确认费用先当场补判、判出即落库（CONTEXT「截单时当场判、判出即落库」），与截单编排判同一只；判不出（缺对账安排或归属日形态）这一户这一拍停住、不记截止日，报未配置。本期没有费用只记截止日、不发单。截单停在依赖上或对账单信封没交出去，这一户整笔回滚，下一拍重截；同一份命令重放答已存在，不会多出单。其余答案（勾稽不平、单号冲突等）照记截止日，不再每拍重问。
5. **客户费用读口**多透出两格供人列清单：`attributionDate`（已形成的结算归属日）与 `billedOn`（所在的未作废对账单号）。截单不读这两格，照旧当场判。

## 候选与反方

- **运营发布口只收账户与本期最后一个归属日，由产品自己收拢候选、自己算合计。** 人发起的那一次就没了勾稽：编排拿自己算的数与自己勾稽。否决，运营发布口的清单与合计仍由人给；到点截单是另一条路，由执行器给。
- **确认与发布共用一种授予。** 确认只把一笔费用按它自己的条件定格，发布固定一期的单号与合计，职责不同。否决。
- **首发只由人发起，不做到点截单。** 会话起初这样提，用户答「自动截单」，否决。
- **把周期写进业务时区与截单时刻那一本。** 那一本按版次追加、只作用于此后形成的归属日；周期一改要接上已截的各期，两者的变更规则不同。否决，在对账安排之下分两本册。
- **执行器按截止日逐个查有没有出过单，不另记截止记录。** 没有费用的截止日不出单，每拍都要重问一遍。否决，记截止记录。

## Consequences

- access-identity：决定种类 `CHARGE_CONFIRMATION`、`STATEMENT_PUBLICATION`；迁移 `access_identity/0034` 重建决定种类 CHECK。
- SA：`adapters/http` 两口与 `OperatorDecisionIntake` 的两个 Intake；`registrationjson` 三份载荷（含对账周期）；客户费用读口两格；领域 `StatementCycle`；编排 `CutOffStatementsOnCycleHandler`；迁移 `settlement_accounting/0049`（对账周期册与截止记录）。
- parcel-dispatch：`assemble_cycle_cut.go`，派发器外包一层，每拍之后逐户截。
- parcel-settlement-register：新命令 `statement-cycle`。
- parcel-api：特性文件 `endpoints_settlement_decisions.go`。
- 演示租户的两种授予、准入对照、权威区间、两个结算时点、对账安排与对账周期，随本票第 4 笔的种子落。

## 越权风险点

1. **周期只登一份、不改。** 换切法要接上已截的各期，改的规则还没定。
2. **执行器跨租户扫、逐户一笔事务，全在 dispatch 那一拍里。** 户数多了一拍会变长；出单量大的一期，候选清单会整份进一份截单命令。
3. **运营手工发布与到点截单撞上同一批费用。** 执行器先收候选再进出账锁；运营恰好先把其中几笔发进别的单，这一期就答勾稽不平、照记截止日，余下的费用滚进下一期，不丢费用，但这一期要人去看。
4. **出错的户每拍都重来。** 日志按等依赖、未配置、矛盾分开记，没有退避；户数多时每拍对每户各开一笔带行锁的事务。
5. **一笔判不出归属日的费用让这一户整户停截。** 某户新加了费用项目却没登它的归属日形态，这一户此后每拍都停在未配置，其余费用与后续各期也截不出，直到补登；手工截单对这样一件只逐件排除（`AT-SA-213`）。取整户停，是为了不让那一笔悄悄滑进错的一期。
6. **缺来源时点的费用到点截单不收、也不报。** 形态要来源发生时点而费用上没有，补登哪一册都补不上，执行器跳过它、每拍重判一次，日志与截止记录里都没有它；手工截单会把它按 `SOURCE_INSTANT_MISSING` 逐件指名。
7. **截止记录只记最后截到哪一天。** 停机几天再起，会把这几天已关上的截止日一次补截完，每一期照它自己的截止日收费用。
8. **运营发布口的申报合计用最小币单位的整数串。** 与客户费用读口的金额写法一致；若管理台要按主币单位录入，换写在页面侧。
9. **`billedOn` 只取一张。** 一笔费用按规则只会在一张未作废的单上；若有两张，读口按单号取第一张、不报异常，那是截单出账锁那一侧的不变量破了。

## Links

- [ADR-0170](./0170-settlement-orchestrations-enter-through-parcel-api.md)：决定二（时点门），本记录不改
- [ADR-0336](./0336-confirmation-facts-land-with-the-estimated-charge-and-an-effective-delivery-final-is-the-built-in-confirmation-basis.md)：决定三（运营确认口的存在），本记录定它的形态
- [ADR-0269](./0269-business-time-zone-and-cutoff-belong-to-the-statement-arrangement-of-the-settlement-account-and-the-cut-admits-by-attribution-date.md)：决定一（周期日历不在业务时区那一册）、决定三（截单时当场判）、决定四（截单按归属日收）、越权风险点 5（本记录回答）
- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)（决定一：运营决定按种类授予）、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)
- [settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)：「对账安排」与「结算账户、对账与争议」
- [UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md)：`AT-SA-221`；[UC-SA-003](../application/settlement-accounting/UC-SA-003-CUT-OFF-PUBLISH-AND-RECONCILE-CUSTOMER-STATEMENT.md)：`AT-SA-222`–`224`
- 票 [product-strategy-boundary/22](../../.scratch/product-strategy-boundary/issues/22-settlement-orchestrations-assembly-and-triggers.md)
