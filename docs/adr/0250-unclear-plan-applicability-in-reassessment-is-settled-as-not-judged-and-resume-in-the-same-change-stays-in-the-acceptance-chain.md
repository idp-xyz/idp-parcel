# ADR-0250：路由复核的「适用性看不清」入账未判、留下缺口——它的来源没有一个是依赖故障；ADR-0094 决定四「续办触发同笔落地」不约束路由复核

Status: Accepted（2026-10-21，Claude Code 会话代 NR owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/05，出处审计 [audit/nr-no.md](../../.scratch/docs-consistency/audit/nr-no.md) M8、L14。部分停用 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 决定一「重投推得动的照旧是未决……」一句里「适用性看不清」一格，被停原句与适用场景见「对已接受记录的作用」；回答 ADR-0235 越权风险点 1。ADR-0235 正文不改，只在其 Status 行与 Links 加前向指针。代码随票 [docs-consistency/code/26](../../.scratch/docs-consistency/code/26-unclear-plan-applicability-in-reassessment-is-retried-instead-of-settled.md)。越权风险点见文末，待 NR owner 复核，第 2 点知会 PS owner。）<!-- 索引注记 -->｜**分格**：复核里的「适用性看不清」只来自取数侧对选中候选答限制状态未知，来源有关务目录为空、受理基线缺端点国家、关务来源未接三种，重投都推不动；改答`未判`，原因 `PLAN_APPLICABILITY_UNCLEAR`，未判登记同行记下取数侧交回的缺口与再判条件，计划与适用性都不动。真依赖故障（关务目录读不动）在取数侧交错误，照旧走证据读不动、回滚重投。**续办纪律**：ADR-0094 决定四只管受理判断链；路由复核的每一次控制触发都从自己的位置重判，入账未判可以先于续办落地，续办仍归票 154
Date: 2026-10-21

## Context

- ADR-0235 决定一按恢复动作把复核未决分成两格：重投推得动的照旧是未决、回滚重投，名单里有「适用性看不清」；重投推不动的答`未判`、入账留原因。
- 「适用性看不清」在代码里是领域 `ReviewPlanApplicability` 遇选中候选的限制状态未知（`ConstraintStatusUnknown`）答 `PlanReviewUndecided`，复核编排把它折成 `PlanReviewInconclusive`、回滚重投。
- 限制状态未知今天只有三个来处，都在关务适用性取数侧：
  - 关务来源目录为空（`CUSTOMS_PORT_PATH_CATALOG_EMPTY`）：租户没登口岸路径目录，等登记。
  - 受理基线的地理投影缺端点国家（`CUSTOMS_ENDPOINT_COUNTRY_MISSING/<端>`）：基线一经固定不再改写，这一格永远补不上。
  - 关务来源未接（`CUSTOMS_APPLICABILITY_NOT_CONNECTED`）：装配没接关务判断口，等部署。
- 关务目录读不动（`CatalogUnreadable`）不在其中：取数侧把它作为错误交回，复核编排答「证据读不动」，回滚重投。
- 三格重投多少次都一样，照 ADR-0235 自己的判据该答`未判`。它们照旧占着交接分区重投到 `ABANDONED`；UC-NR-003 `AT-NR-067` 把它们写成依赖类未决，运维会去查一个没坏的依赖。
- ADR-0235 越权风险点 1 留了一问：ADR-0094 决定四说「第三格必须与它的续办触发同笔落地，否则不许落地」；ADR-0235 落`未判`而没带续办。ADR-0094 自己的索引注记写明适用场景是「接受判断链的未决处置」，两份记录之间没有停用关系，要不要跨上下文适用没人裁过。

## Decision

**一、「适用性看不清」从重投推得动一族移到`未判`。**

- 原计划适用性复核遇选中候选限制状态未知时，答`未判`，原因 `PLAN_APPLICABILITY_UNCLEAR`。计划与适用性都不动，不默认保留，也不默认失效。
- 未判登记同一行另记取数侧交回的缺口与再判条件，例如 `CUSTOMS_PORT_PATH_CATALOG_EMPTY` 与 `CUSTOMS_PORT_PATH_CATALOG_REGISTERED`。运维据此按缺口查哪几件包裹在等哪一项登记，续办（票 154）据此认该重驱哪几行。
- 判据仍只在复核编排一处，收寄采用与交接两路同改，适配器不持有原因字面量（ADR-0235 决定一）。
- 取数侧日后新增限制状态未知的来源，先答它重投变不变。会变的是依赖故障，在取数侧交错误，走证据读不动；交「状态未知」的，一律是重投推不动的缺口。
- 只管路由复核。初始路由一路的同格（限制状态未知落包裹级未决、整封回滚）归票 demo-chain-gaps/155。

**二、ADR-0094 决定四的「续办触发同笔落地」不约束路由复核。**

- ADR-0094 决定四的理由是：受理判断链上停住的那一次，此后不会再有别的触发来问，入账而没有续办，委托就永久停在`已提交`。它的适用场景按它自己的注记，限于接受判断链。
- 路由复核不是这样。每一次控制触发都从自己的位置重判一次：收寄采用一次，此后每交进一个节点一次。入账未判的那一次之后，包裹交进下一个节点时会被重新复核。漏掉的只是那一次位置上的复核，包裹不会停在一个再没人问的状态里。
- 所以路由复核的`未判`可以先于续办落地。续办仍按 ADR-0235 决定六归票 154。
- 例外照实写：包裹在最后一个节点上入账未判，之后只有交付、没有下一次控制触发，这一次就再没有复核。这正是票 154 要补的。

## 候选与反方

- **只把目录为空、端点国家缺两格移到`未判`，「关务来源未接」留在未决。** 否决：来源未接等的是部署，重投同样推不动；留在未决就是让没接关务的部署每次复核都烧到 `ABANDONED`。
- **在取数侧把三格译成「证据未配置」，复用 `REASSESS_EVIDENCE_NOT_CONFIGURED`。** 否决：证据未配置说的是判断时点没有适用的网络证据（ADR-0052），这里网络证据在、缺的是选中候选的关务资格；合成一格，运维按原因查时分不出该去登哪一样。
- **未判登记只记原因，不记缺口。** 否决：三种来源的补法各不相同，只记原因等于让续办与运维回头再问一次取数侧。
- **ADR-0094 决定四跨上下文适用，撤回 ADR-0235 的`未判`、改回重投。** 否决：代价回到 ADR-0215 越权风险点 10，分区堵到 `ABANDONED`，同样丢掉这次复核，还拖住同一分区后面的信封。
- **另立一份记录把 ADR-0094 决定四定为跨上下文纪律、同时给路由复核补续办。** 未采纳：续办的形状要与票 14 的同族修法一起定（ADR-0235 决定六），本记录不抢那张票。

## 对已接受记录的作用

- **部分停用** ADR-0235 决定一「重投推得动的照旧是未决，消费门回滚重投：复核登记册或路由历史读不回、证据读不动、适用性看不清、取号依赖故障，以及下面决定二的原因写不进。」一句里的「适用性看不清」一格：改答`未判`，原因 `PLAN_APPLICABILITY_UNCLEAR`（决定一）。**适用场景**：NR 路由复核的收寄采用与交接两路，原计划适用性复核遇选中候选限制状态未知。同句其余各格照旧重投；ADR-0235 其余各条不变。
- ADR-0215 决定四「复核停在未决（……适用性看不清……）」一句的未决范围已由 ADR-0235 改写，现行的重投名单写在 ADR-0235 决定一。本记录只改那一处，不另在 ADR-0215 上挂指针。
- **回答** ADR-0235 越权风险点 1 里「owner 若坚持 ADR-0094 决定四」那一问：不坚持，理由见决定二。无条款停用。
- ADR-0094：无条款停用。决定二读的正是它注记里已写的适用场景。

## Consequences

- 代码随票 docs-consistency/code/26：`ports.NotJudgedReason` 加 `PLAN_APPLICABILITY_UNCLEAR`；复核编排遇 `PlanReviewUndecided` 改答`未判`，`ReassessUndecidedReason` 去掉 `PlanReviewInconclusive`；未判登记加缺口与再判条件两列（network_routing 新迁移），这一原因必带、其余原因可空。
- UC-NR-003 结果表「当前计划适用性未决」一格并入`触发未判`；`AT-NR-051` 改写，`AT-NR-067` 去掉「适用性看不清」。
- 在 code/26 落地之前，生产上这三格照旧回滚重投到 `ABANDONED`。

## 越权风险点

1. **关务来源未接也入账未判**（NR owner）。没接关务的部署里，每一次复核都入账、不判，不再有失败码提醒它没接。缓解：未判登记按原因、按缺口列得出；部署检查该在上线前点名关务来源。
2. **受理基线缺端点国家是永久缺口**（NR、PS owner）。这件包裹此后每一次复核都入账未判，直到新的受理基线。补救在小包托运侧：受理时就该拒收缺国家的投影。本记录不改 PS。
3. **限制状态未知不再让计划停在未决**（NR owner）。原先回滚重投期间，消费门的失败码会指向这件包裹；入账之后，原计划照旧当前有效，只在未判登记里留一行。原计划可能本已被一条看不见的关务限制淘汰，在续办或下一次复核之前，包裹按原计划继续走。
4. **最后一个节点上的未判再无复核**（NR owner）。见决定二的例外，补法归票 154。

## Links

- [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md)：部分停用决定一「重投推得动的照旧是未决……」一句的「适用性看不清」一格；回答越权风险点 1
- [ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md)：决定四「第三格必须与它的续办触发同笔落地」，适用场景限接受判断链
- [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md)：决定四的未决范围已由 ADR-0235 改写
- [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：按恢复动作分格
- [ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)：证据未配置与本格分开
- [ADR-0249](./0249-offsite-pickup-reassessment-checks-hard-constraints-only-and-does-not-establish-a-controllable-node.md)：揽收复核只按硬约束判，限制状态未知同按本记录
- [UC-NR-003](../application/network-routing/UC-NR-003-REASSESS-ROUTE-AFTER-NETWORK-INTAKE.md)（`AT-NR-051`、`AT-NR-067`）
- 票 `.scratch/docs-consistency/issues/05-nr-no-docs.md`；代码票 `.scratch/docs-consistency/code/26-unclear-plan-applicability-in-reassessment-is-retried-instead-of-settled.md`；续办 `.scratch/demo-chain-gaps/issues/154-network-reassessment-not-judged-triggers-resume-after-registration.md`；初始路由同格 `.scratch/demo-chain-gaps/issues/155-initial-route-unconfigured-undecided-burns-the-acceptance-partition.md`
