# 74 身份后来才成立的集运成员，永远拿不到它随袋走过的那条移动事实

Category: enhancement
Status: needs-triage——R2（上线后尽快）：袋里一件待识别或身份冲突的实物随袋出发，识别成功之后追踪上永远缺这一条出发。2026-10-21 协调方据票 demo-chain-gaps/123 隔离评审立（作者子代理代笔）。
Blocked by: 无（「要裁的」第一条若裁成另立 ADR，落地前先接受那一份）
上下文：VE（全程追踪）、NO（集运成员、收寄身份）。
出处：票 [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md) 完成记录越权风险点 4。

## 现象

票 123 之后，移动事实那一路与补派生对段里在控的集运单元按事实那一刻的成员展开（`internal/visibilityexception/adapters/transportfulfillment/consolidation_members.go` 的 `trackedParcelsAt`）。成员身份此刻不成立（待识别、身份冲突、没有收寄记录）的那一件没有可投的追踪包裹，入账不派生、不挡其余成员。之后那件实物识别成功、或身份冲突经处置成立，节点作业形成新的收寄版本，但没有任何东西回头把它当时随袋走过的移动事实补给它：那件包裹的追踪上永久缺这条出发、移动或到达。证据只记 `S`（读码）。

## 三类

机制。规模 M。

## 最小的诚实修法（形状仿 ADR-0224）

- 在识别成功、或身份冲突处置成立的那一封 NO 信封上挂一路补派生：取那件作业实物的成员关系事实，找它在哪几件单元里、各在哪段时间；按单元找它在控的段（TF 按对象列段那一口），按段列出声明了这一段的在册移动事实；凡事实发生那一刻它正是那件单元的直接成员（逐层向上到段里在控的那一件）、那件单元在段里在控的，就在新认出的包裹上补派生。
- 派生走 `deriveMovementFactOnto`，同一把键 `movement-fact/<包裹>/<事实>`，与正常路径、ADR-0224 补派生谁先谁后都只留一份。
- 只补投影，不补信号、不重问关务，同 ADR-0224。

## 要裁的

- **扩 ADR-0224 还是另立 ADR**：ADR-0224 定的是「控制事实让对象进段」时的补派生，触发方与判据都在 TF；这一路的触发方是 NO 的身份事实，判据要沿成员关系向上找段，不是同一条决定的简单延伸。倾向另立一份、引 ADR-0224 的判据与幂等键。
- **向上找的口径**：按成员关系事实的业务时间逐层向上到段里在控的单元（与票 123 向下展开同一口径的逆向），还是只认直接装在段里在控单元里的那一层。
- **触发信封**：识别成功（收寄新版本形成）与身份冲突处置成立是不是同一个信封类型，各挂一路还是共用。
- **身份后来又被纠正成另一件包裹**：先前补到旧包裹上的那一份怎么处置（VE 不自行使源事实失效）。
- **扩到交接与交付**：[code/75](./75-tracking-projects-control-facts-on-a-consolidation-unit-as-one-parcel.md) (b) 落地、交接与交付投到单元成员上之后，本票的补派生从移动事实扩到交接与交付；那时身份后来才成立的成员一并由本票补。

## 既有票

- [demo-chain-gaps/123](../../demo-chain-gaps/issues/123-tracking-projects-a-departing-bag-as-one-parcel-and-its-members-get-no-departure.md)；[demo-chain-gaps/53](../../demo-chain-gaps/issues/53-movement-facts-recorded-before-their-segment-never-reach-tracking.md)（ADR-0224 补派生）；[114](../../demo-chain-gaps/issues/114-deterministic-failures-answered-as-retryable-undecided-across-consumers.md)
