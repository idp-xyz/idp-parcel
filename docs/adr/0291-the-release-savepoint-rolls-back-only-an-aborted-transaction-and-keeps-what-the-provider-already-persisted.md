# ADR-0291：释放口外那一层 savepoint 只在调用方事务已中止时才退回——事务没中止，提供方已落库的那一半留着，组合策略下冻结那本的释放不随暴露账本读不回而撤掉；SQL 级错误那一格事务本就保不住，如实撤掉

Status: Accepted（2026-10-26，Claude Code 会话代 PS owner 裁定：协调方据用户「你作为业务和系统专家自决」授权，把票 [docs-consistency/code/96](../../.scratch/docs-consistency/code/96-the-release-savepoint-undoes-a-persisted-freeze-release-when-the-exposure-ledger-fails.md) 定级 R2（上线后尽快）、在票面三个候选里选 1（只改 PS 一侧的装饰器）；SA 释放编排两本账各开 savepoint（候选 2）要 SA owner、释放不进调用方事务（候选 3）会动接受链开头「先放后判」的保证，均未取。探测用哪一条语句、探测失败一律按已中止处理、形不成三格的错误同按事务状态落法由作者代决。裁决能力边界：读过票 code/96 全文、[ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 决定三、[ADR-0282](./0282-a-control-release-the-provider-refuses-is-registered-for-handling-instead-of-compensated.md) 决定一与越权风险点、`pspostgres.SavepointedControlRelease`、`ReleasePreAcceptanceControlHandler.Handle`、冻结与暴露两本账的真库适配器和 `LockControlScope`；未重读 SA owner 侧对释放编排的任何计划。部分停用（收窄）ADR-0282 决定一末条前半句，被停原句与适用场景见「对已接受记录的作用」；ADR-0282 正文不改，只在其 Status 行与 Links 加前向指针，ADR-0122 同样只在 Status 行与 Links 挂一句读法注。越权风险点见文末，待 PS owner 复核，SA owner 知会。）<!-- 索引注记 -->｜**判据**：释放口答非`已收口`或答错误时，看调用方事务此刻有没有中止——在 savepoint 里执行一条无副作用的语句，成功才算没中止，失败一律按已中止。**落法**：已中止就退回 savepoint；没中止就收起 savepoint，提供方已落库的那一半留着。落法只看事务状态，不看答复在哪一格。**组合策略**：冻结那本的释放已落库、暴露账本读不回而语句没出错时，冻结释放保住，续办重放时冻结账本按幂等交回原释放时刻、暴露账本再认领。**SQL 级错误**：事务已中止，退回 savepoint，冻结释放随之撤掉，如实记录

Date: 2026-10-26

## Context

- [ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 决定三：释放在冻结与暴露两本账各认领一次；「冻结那本已落库的释放在暴露账本读不回时不撤回：续办重放，冻结账本按幂等交回原释放答案，暴露账本再认领一次——两本各自收口，不必同事务。」`ReleasePreAcceptanceControlHandler.Handle` 照此写：取作用域锁，读冻结账本，认领并 `Save`，再读暴露账本；暴露账本读不回就交`未形成`，注释写明冻结那本已落库的释放不会被撤回。
- [ADR-0282](./0282-a-control-release-the-provider-refuses-is-registered-for-handling-instead-of-compensated.md) 决定一末条：「释放口外那一层 savepoint 对非`已收口`的答复与错误都退回，开、退、收 savepoint 本身失败按`可重试未形成`交回。」撤回、主动拒绝、授权处置、停进`等待受控补充`与形成决定的释放，都经 `SavepointedControlRelease` 跑在调用方那一笔事务里；savepoint 是为隔开 SQL 级错误而设：一条语句超时、锁等超时、权限或表被挪走的错误会把整笔事务打进中止态（25P02），不退回，调用方已经成立的那件事连提交都做不到。
- 两条合起来，在组合策略（预付冻结与信用校验同在）下相抵：暴露账本读不回时，编排按决定三交`未形成`，冻结释放已落库；装饰器见到非`已收口`就整段退回，冻结释放跟着撤掉，原本已放掉的冻结又占回去。终局各路（撤回、拒绝、处置）留下的补偿引用不持久、没有东西按它重放（票 code/72、code/73 的越权风险点，ADR-0282 越权风险点 5），那一笔冻结便一直占着，直到有人碰巧重放。
- 暴露账本"读不回"有两种，对事务的影响相反：
  - **SQL 级错误**——取锁、查询、写入的语句本身报错。服务端把事务置为中止，此后除退回以外的语句一律答 25P02。退回 savepoint 是唯一让调用方事务继续可用的办法，被退回的一切（包括冻结那本刚落的释放）都保不住。
  - **语句没出错**——暴露行的幂等指纹不是本适配器写下的形状、行重建不了账本，或读口交了 nil。事务一切正常，这时退回 savepoint 只撤掉一笔本该留下的释放。

## Decision

**一、装饰器在内层答非`已收口`（或答错误）之后探一次事务状态，已中止才退回，没中止就收起 savepoint。**（机制）

- 探测是在 savepoint 里执行一条无副作用的语句（`SELECT 1`）。成功就说明事务没中止；失败（25P02、探测自己出错、ctx 已取消、连接断开）一律按已中止处理，退回 savepoint。只认探测成功，是因为误判的两个方向代价不对称：把没中止的当成已中止，只是多退回一次、撤掉本可保住的那一半，回到整段撤掉的保守落法；把已中止的当成没中止，会在中止的事务上收起 savepoint，随后调用方的每一条写都答 25P02、整笔回滚。
- 内层答`已收口`不探测，直接收起 savepoint。
- 落法只看事务状态，不看答复落在哪一格：`可重试未形成`（语句没出错但账本读不回）、`确定未受理`（提供方在读账本之前就短路，没有东西可留可撤）、形不成三格之一的错误，事务没中止时一律收起；按格分开退只会多一条要守的分支，而同一格（`可重试未形成`）既可能来自 SQL 级错误、也可能不是，格本身分不出事务状态。
- 答复原样交回，各路据此留补偿、记册或停下，与 ADR-0282 决定一、二不变。开、退、收 savepoint 本身失败仍按`可重试未形成`交回。

**二、组合策略下，暴露账本读不回而语句没出错：冻结那本已落库的释放保住。**（机制）释放仍答`可重试未形成`，冻结账本上那一行已是已释放、带释放时刻，暴露那一行仍占着。续办重放时冻结账本按幂等交回原释放答案（释放时刻不变），暴露账本再认领一次。这是 ADR-0122 决定三在 PS 共享事务里的落实，决定三正文不改。

**三、SQL 级错误那一格如实撤掉。**（机制）暴露账本因 SQL 级错误读不回，事务已中止：装饰器退回 savepoint，冻结那本刚落的释放随之撤回，两本账都仍占着；调用方事务继续可用、照常提交，答复仍是`可重试未形成`，各路留补偿引用或停下。这不是本记录引入的退步——事务本就保不住那一半——要连这一格也保住只有候选 2（见下）。

**四、保不保住那一半与调用方整轮要不要回滚是两件事。**（机制）接受链形成决定处放不收口整轮回滚（[ADR-0290](./0290-an-acceptance-chain-round-that-finds-the-request-withdrawn-or-rejected-releases-the-occupation-it-formed.md)），那是应用层按自己的保证做的决定，整轮回滚时保住的那一半随整轮一并撤回；本层只保证"事务没中止就不替调用方多撤一段"。终局各路与暂停那一路提交时，保住的那一半随之落库。

## 候选与反方

- **候选 2：SA 释放编排两本账各开一个 savepoint。** 冻结那本释放落库后收起自己的点，暴露那本出错只退回暴露那一段，SQL 级错误那一格也能保住冻结释放，ADR-0122 决定三在共享事务里完整成立。未取：改的是 SA 应用层，要 SA owner；本记录不排斥它，若日后落地，PS 这一层的探测照旧安全（事务不会再因暴露那一段中止）。
- **候选 3：释放不进调用方事务，独立事务里释放、业务事实先提交。** 否决：两本各自收口照 ADR-0122 成立，但释放与业务事实不再同笔，接受链开头「先放后判」与 ADR-0290 整轮回滚撤回刚占下的冻结这两条保证要重新论证。
- **按答复格分：`可重试未形成`收起、其余退回。** 否决：同一格可来自 SQL 级错误，格分不出事务状态，按格收起会在中止的事务上收起 savepoint；事务状态才是该看的东西。
- **读连接上的事务状态而不执行语句**（pgx 的 `TxStatus`）：不可行。框架 `bentopg.Executor` 只暴露 `Query`/`QueryRow`/`Exec`，不暴露连接，只能经 SQL 探。
- **探测只认 25P02、别的错误按没中止。** 否决：探测自己因 ctx 取消、连接断开等失败时，不知道事务什么状态，按没中止落法就是把赌注押在不明状态上。

## 对已接受记录的作用

- **部分停用（收窄）** ADR-0282 决定一末条「释放口外那一层 savepoint 对非`已收口`的答复与错误都退回，开、退、收 savepoint 本身失败按`可重试未形成`交回。」前半句。**适用场景**：释放口答非`已收口`或答错误、而调用方事务此刻没中止（savepoint 里的探测语句成功）——不退回，收起 savepoint，提供方已落库的那一半留着；事务已中止（探测失败）时照原句退回。「开、退、收 savepoint 本身失败按`可重试未形成`交回」一半不变，决定一其余各条与决定二至四不变。
- **不停用** ADR-0122 决定三，挂读法注：「冻结那本已落库的释放在暴露账本读不回时不撤回」在 PS 的共享事务里，暴露账本读不回而语句没出错时成立；因 SQL 级错误读不回时事务已中止，释放口外那一层 savepoint 退回，冻结那本的释放随之撤掉。决定三的"两本各自收口，不必同事务"对独立事务成立，对共享事务只到事务不中止为止。

## Consequences

- 代码随票 code/96：`SavepointedControlRelease` 在内层答非`已收口`或答错误后探事务状态，已中止退回、没中止收起；头注写明两类"读不回"和探测的保守方向。不加迁移，不改 SA 释放编排、不改端口答复形状。
- 终局各路与暂停那一路：暴露账本语句没出错地读不回时，冻结那本的释放留在库里，暴露那本的占用仍在，补偿引用照旧不持久；比之前多放掉一本，没放掉的那一本仍等人重放。
- 接受链放不收口整轮回滚不变；保住的那一半在整轮回滚时一并撤回。
- 真库用例：装饰器层用一张临时表证"事务没中止留、中止撤"；cmd 层用生产的 SA 释放编排与真实两本账证组合策略（暴露行指纹损坏的真数据、读口交错误两种）冻结释放保住、续办重放两本各自收口，SQL 级错误（暴露表被挪走）冻结释放撤掉而事务照常提交，确定未受理与冻结账本读不回两格不动账本，已收口不回归。

## 越权风险点

1. **SQL 级错误那一格冻结释放仍撤。** 组合策略下暴露账本因 SQL 级错误（锁等超时、语句超时、权限、表被挪走）读不回时，冻结那本的释放照旧随退回撤掉，补偿引用不持久、没人重放，那一笔冻结仍占着。要根治只有候选 2（SA owner 改编排）。本记录只修了"语句没出错"那一格。
2. **保住的只是冻结那一半。** 暴露账本读不回的那一笔仍占着，补偿引用不持久，终局各路没有东西替它重放（ADR-0282 越权风险点 5 同源）。比整笔撤掉少占一本账，不是收口。
3. **形不成三格的错误同按事务状态落法**（作者代决）。`ReleasePreAcceptanceControlHandler.Handle` 的 `release exposure` 之类编程错误出在冻结那本已落库之后，事务没中止时也收起 savepoint，冻结释放留着。依据：每本账各自收口，冻结那半本身是完整合法的状态；若 owner 要错误一律整段退回，在探测之前加一个 `releaseErr != nil` 分支即可。
4. **探测多一次往返。** 只发生在内层答非`已收口`或答错误时，一条 `SELECT 1`；`已收口`那条主路不探。
5. **探测失败一律按已中止处理。** 连接抖动时探测失败、事务其实没中止，会被多退回一次，回到整段撤掉的保守落法，不出错、不丢帧；代价是偶尔撤掉本可保住的那一半。
6. **冻结账本 `Save` 逐行 UPSERT，中途非 SQL 原因失败会留下前面已写的行。** 唯一的非 SQL 失败是"入册的冻结没有幂等指纹"（领域或适配器的 bug），已写的行都是逐行幂等推进的合法状态，收起 savepoint 留着它们无害。

## Links

- 票 `.scratch/docs-consistency/code/96-the-release-savepoint-undoes-a-persisted-freeze-release-when-the-exposure-ledger-fails.md`；出处票 `.scratch/docs-consistency/code/71-release-and-control-ports-answer-a-closed-set-instead-of-a-bare-error.md`（隔离评审信息 3）与 `.scratch/docs-consistency/code/72-a-customer-supplement-pause-releases-the-paused-versions-occupation.md`（评审 N1 引入这一层 savepoint）
- [ADR-0282](./0282-a-control-release-the-provider-refuses-is-registered-for-handling-instead-of-compensated.md)：部分停用（收窄）决定一末条前半句
- [ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md)：决定三不停用，挂读法注
- [ADR-0290](./0290-an-acceptance-chain-round-that-finds-the-request-withdrawn-or-rejected-releases-the-occupation-it-formed.md)：接受链放不收口整轮回滚，保住的那一半随整轮回滚撤回
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [UC-PS-005](../application/parcel-shipment/UC-PS-005-WITHDRAW-SUBMITTED-SHIPMENT-REQUEST.md)：规则「撤回提交成功后不等待财务释放才生效」，`AT-PS-160`
- `internal/parcelshipment/adapters/postgres/control_release_savepoint.go`、`internal/settlementaccounting/application/release_pre_acceptance_control.go`
