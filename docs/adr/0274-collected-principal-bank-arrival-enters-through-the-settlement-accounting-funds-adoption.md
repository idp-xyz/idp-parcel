# ADR-0274：代收本金的运营企业真实到账经 settlement-accounting 的唯一采用口进入本产品，代收与清分凭采用引用登记这一层

Status: Accepted（2026-10-21，Claude Code 会话代 CR、SA owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/10（10a 半边），出处审计 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) CM·M-2。无条款停用：[ADR-0137](./0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定四本就是全称句，本记录把代收与清分的真实到账层接到那一口上；[ADR-0082](./0082-collection-subledger-is-keyed-by-four-dimensions-and-posted-append-only.md) 与 [ADR-0271](./0271-collection-principal-enters-the-ledger-only-on-the-channel-collection-report-and-shortfall-comes-only-from-in-transit.md) 的依据门不变，本记录在真实到账这一层的证据上再加一道门。代码随票 [docs-consistency/code/54](../../.scratch/docs-consistency/code/54-collection-bank-arrival-does-not-reference-an-adopted-funds-fact.md)。越权风险点见文末，待 CR、SA owner 复核。）<!-- 索引注记 -->｜**入口**：运营企业真实到账作为外部资金事实，只经 settlement-accounting 的采用进入本产品；代收与清分登记真实到账那一层时，证据引用必须指向一条已采用的外部资金事实，不另开铸造口。**核对**：代收与清分登记时按引用向 settlement-accounting 的只读口读那一版，在册且币种与指令相同才接受；引用同一条已采用事实的各条真实到账，金额之和不超过该事实金额。**分工**：资金事实的来源身份与采用归 settlement-accounting，代收本金的移位与汇付归代收与清分；结算一侧怎样把被引用的那部分认作代收本金、不进运营余额，本记录不裁

Date: 2026-10-21

## Context

以下所引 GLOSSARY 与 collection-remittance CONTEXT 原文是本记录接受前的文字，已随本记录改写（见 Consequences）。

- 两份已接受的记录给了相反的入口：
  - [ADR-0137](./0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定四是全称句：「外部资金事实进入本产品只有 `settlement-accounting` 采用这一口」。CONTEXT-MAP 的 `settlement-accounting → customs-compliance` 一条照写「结算是外部资金事实进入本产品的唯一采用口」。
  - collection-remittance CONTEXT 与 [ADR-0082](./0082-collection-subledger-is-keyed-by-four-dimensions-and-posted-append-only.md) 让代收与清分自己接受「运营企业真实到账」：CR CONTEXT「代收事实」把它列为来源层级之一，Boundaries 写「本上下文只登记汇付主张与已接受的资金事实引用」；GLOSSARY「代收货款」写「银行或支付系统拥有运营企业真实到账事实；`collection-remittance` 关联这些事实形成代收分户账结果」。
- 代码让代收与清分自收真实到账：`internal/collectionremittance/domain/collection_fact.go` 的 `OperatorBankCredit` 带一个任意的 `EvidenceReference` 经受控 CLI（`cmd/parcel-collection-register`）登进代收与清分，不经 settlement-accounting 的采用信封，也不核引用指向什么。
- 一笔渠道回款在银行流水上是一条外部资金事实。两个口各铸一次，同一笔钱就在结算里作为一条已采用事实、在代收与清分里作为另一条真实到账各存一份，两份撞了怎么办，正是 ADR-0137 决定四去掉第二个铸造点时说的那一问：「若 CC 另开人工补录口，同一笔付款会有两个铸造点」。
- settlement-accounting CONTEXT 写入账余额时有「COD 代收本金不进」，已经默认 COD 的到账会经过结算的采用，但没有任何一处写它怎样到达代收与清分。
- COD 首发不在试点范围内，今天不阻断；但照 CONTEXT-MAP 实现的作者会在两处各开一个铸造点，或者把代收与清分的真实到账层堵死。

## Decision

**一、运营企业真实到账只经 settlement-accounting 的采用进入本产品。**（机制）代收与清分不开第二个铸造或补录入口：它登记的「运营企业真实到账」那一层代收事实，证据引用必须指向一条 settlement-accounting 已采用的外部资金事实（采用版本）。人工核实与更正同 ADR-0137 决定四，先在 settlement-accounting 采用、再被引用。

**二、登记时按引用核对，引用同一条事实的到账不超额。**（机制）代收与清分登记真实到账时，按证据引用向 settlement-accounting 的只读口读那一版已采用事实：不在册答依据不在册，币种与代收指令不同答受理门拒。引用同一条已采用事实的各条真实到账，金额之和不得超过该事实金额；超出的那一条答内容冲突，不落册。一条渠道回款常是多件包裹的合计，所以一条已采用事实可以支撑多条按指令拆开的真实到账；拆分本身是代收与清分的登记，不改写结算的事实。

**三、其余三层来源不变。**（机制）收件人付款、渠道代收报告与渠道回款通知都不是运营企业的资金事实，照旧由 `transport-fulfillment` 提供证据、代收与清分登记；它们不经结算的采用。

## 候选与反方

- **限定 ADR-0137 决定四只管税费付款核对与结算自己的收付映射，代收与清分的真实到账另成一口。** 否决：一条银行流水在产品里就有两个铸造点，同一笔渠道回款可以既被结算映射为客户收款、又被代收与清分移成`待清分`，两边各自守恒、合起来重记；而发现它要靠线下对账。ADR-0137 决定四去掉第二个入口的理由在这里照样成立。
- **代收与清分以 inbox 消费结算的采用信封，到账自动转成真实到账事实。** 否决：采用信封不知道这条钱属于哪一条代收指令，按指令拆分是代收与清分的判断，交给信封消费者只能猜；登记时按引用读回一次已经够核对。
- **代收与清分只核引用存在，不核金额之和。** 否决：同一条回款被拆成的各条到账加起来可以超过回款本身，多出的部分就是无来源的本金，与 collection-remittance CONTEXT「账上不得出现无来源的金额」相抵。

## 对已接受记录的作用

- 无条款停用。ADR-0137 决定四原样成立并扩到代收与清分；ADR-0082 决定四、决定五与 ADR-0271 决定二对真实到账的依据门都是必要条件，本记录只给真实到账的证据引用再加一道门。

## Consequences

- collection-remittance CONTEXT「代收义务与实收事实」补一条：运营企业真实到账凭 settlement-accounting 已采用的外部资金事实引用登记；Boundaries 里银行、支付与财务系统一条照此写明。settlement-accounting CONTEXT Boundaries 的代收与清分一条补同一句。
- GLOSSARY「代收货款」「代收事实」，CONTEXT-MAP `settlement-accounting ↔ collection-remittance` 一条与上下文关系图补这条交接。
- 新立 [UC-CR-001](../application/collection-remittance/UC-CR-001-REGISTER-COLLECTION-AND-POST-THE-SUBLEDGER.md) 的输入表与「给开发的交接」记下这一格。
- 代码随 code/54：代收与清分接受真实到账时按引用读 settlement-accounting 的 `ports.AdoptedFundsFactView`，核在册、币种与金额之和；CLI 的真实到账输入改为带采用引用。演示种子的真实到账要先在结算里采用一条合成资金事实再引用它。

## 越权风险点

1. **代 CR、SA owner 裁定入口**（CR、SA owner）。若 owner 认为代收本金的到账应另成一口（例如受托资金走独立的银行账户、与运营资金互不相见），推翻的是决定一，ADR-0137 决定四要另起记录收窄适用场景。
2. **结算一侧怎样处置被引用的那部分没有裁**（SA owner）。一条已采用事实被代收与清分引用为真实到账之后，它在 `UC-SA-005` 里映射到哪一格、怎样不进运营结算余额，本记录不裁；在 SA owner 定之前，那部分在结算一侧停在未分配，看得见、不进余额（settlement-accounting CONTEXT「COD 代收本金不进」）。code/54 把这一问列为要裁的。
3. **金额之和超出答内容冲突**（CR owner）。渠道回款扣了服务费再打款时，回款金额本就少于报告；按指令拆分的到账之和只会不大于回款，不会撞这一格。撞上的是拆分登错，答冲突让人回头核，不自动截断。
4. **登记时读一次，不跟随结算一侧的更正**（CR、SA owner）。结算日后更正或撤销那条已采用事实时，代收与清分不自动随之冲正；来源更正走代收与清分自己的追加更正事实（code/15）。是否要由结算的更正信封驱动代收与清分重判，留给后继记录。
5. **COD 首发不在试点范围**（CR owner）。本记录只定口径，代码随 code/54 在 COD 进入试点之前补上；在此之前受控 CLI 收的真实到账证据引用照旧不核。

## Links

- [ADR-0137](./0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md)：决定四「外部资金事实进入本产品只有 `settlement-accounting` 采用这一口」，本记录把代收与清分接到这一口上
- [ADR-0082](./0082-collection-subledger-is-keyed-by-four-dimensions-and-posted-append-only.md)：代收分户账的依据门；本记录在真实到账的证据上加门
- [ADR-0271](./0271-collection-principal-enters-the-ledger-only-on-the-channel-collection-report-and-shortfall-comes-only-from-in-transit.md)：真实到账只做移位记账；本记录定它凭什么登记
- [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md)：人工采用外部资金事实的运营决定面
- [collection-remittance CONTEXT](../domain/collection-remittance/CONTEXT.md)、[settlement-accounting CONTEXT](../domain/settlement-accounting/CONTEXT.md)、[CONTEXT-MAP](../domain/CONTEXT-MAP.md)
- 票 `.scratch/docs-consistency/issues/10-cross-cutting-docs.md`；代码票 `.scratch/docs-consistency/code/54-collection-bank-arrival-does-not-reference-an-adopted-funds-fact.md`
