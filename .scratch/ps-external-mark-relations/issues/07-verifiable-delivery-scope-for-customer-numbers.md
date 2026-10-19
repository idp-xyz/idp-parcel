# 07 可验证的交付范围：客户编号空间里的唯一命中，要有「这次送站属于这个客户账户」的证据才算证据充分

Category: enhancement
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 立并代决方向、认领（文档与拆票，ADR 占号 0212），分支 `claude-psxm07`
Blocked by: 无（文档与拆票）；实施票另接阻塞边
父票：[05](./05-a-unique-hit-in-someone-elses-number-space-is-not-enough.md)
出处：票 05「要裁的一问」候选 (c)，裁定「(c) 可验证交付范围记作终局方向，随一线客户端收寄流程另立」；ADR-0195、ADR-0184 决定六及其复核记录「决定六与 UC-NO-002「权限、隔离与审计」」的「日后」一段。
地盘：
- docs/adr/0212-*.md（新）、docs/adr/README.md；ADR-0184、ADR-0195 文末复核记录与前向指针（部分停用照 adr/README 约定）
- docs/domain/parcel-shipment/CONTEXT.md（「按外部标识核对包裹身份」）、docs/domain/node-operations/CONTEXT.md、docs/domain/CONTEXT-MAP.md、docs/domain/GLOSSARY.md
- docs/application/node-operations/UC-NO-002-RECEIVE-CUSTOMER-DELIVERED-PARCEL.md 与必要时新 UC
- docs/product/PILOT-PARAMETER-REGISTER.md（授权交付关系一类租户取值的登记行，只登「未配置」）
- .scratch/ps-external-mark-relations/issues/08- 起的实施票

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「需要」。

## 为什么

ADR-0184 决定六：身份核对在租户内按号码取候选，不按交付方自报收窄（自报一错，冲突就变成唯一关联）。代价（复核记录「其二」）：号只在一份已接受委托里在册时，别的实物贴着同号送站会唯一命中那一件。ADR-0195 用歧义信号收住了「另一方已提交」那一格；「另一方从未报过这个号」「另一方委托已被拒绝或撤回」两格仍是已知代价，终局方向是可验证的交付范围。

## 方向裁定（2026-10-19，会话 idp-parcel-9b 代决；细节由本票的 ADR 定）

1. **交付范围是一件可验证的事实，不是自报。** 一次送站属于哪个客户账户，凭两类证据之一：客户账户事先交来的**送站预报**（列出这一批要送的号码或作业实物，经客户渠道提交——首方客户渠道族见 ADR-0139，另一会话的 first-party-api 票在落客户渠道册与铸造器，预报的提交口应挂在那一族上），或在节点当场形成的**交接单**（交付方与客户账户的绑定由客户渠道或运营决定确认）。设备自报的交付方仍只是线索。
2. **范围参与核对的方式：消歧，不收窄。** 候选仍在租户内按号取（ADR-0184 决定六不废）；交付范围只用来判「唯一命中是否证据充分」：唯一命中的那件包裹属于本次送站可验证范围里的客户账户 → 证据充分、关联；不属于或本次送站没有可验证范围 → 这一格怎么答由 ADR 定（候选：待识别并记「范围未证」一类原因，不记冲突）。多候选时范围能否把候选消歧到一件，也由 ADR 定。
3. **授权交付关系是租户取值**：哪个交付方可以替哪个客户账户送货、预报的有效时窗，留空答`未配置`，登记册只登行不填值；演示租户在种子里配。
4. **先改 CONTEXT 再改 UC**：PS CONTEXT「按外部标识核对包裹身份」与 CONTEXT-MAP `parcel-shipment → node-operations` 同句先改，再写 ADR-0212 部分停用 ADR-0184 决定六中与之相抵的句子（照 adr/README 约定），ADR-0195 的歧义信号在范围可验证时怎样并存写明。
5. **拆票**：模型与预报册（NO 还是 PS 拥有，由 ADR 定并写理由）、预报提交口（挂首方客户渠道族，与另一会话协调）、核对时的范围判断、交接单的确认、管理台读面，各成实施票、带阻塞边；与 first-party-api 的依赖写成 Blocked by。

越权风险点：代 PS 与 NO owner 定了「消歧不收窄」与「设备自报只是线索」；owner 复核时看真实租户有没有「无预报的散客送站」需要另一条路。

## 完成判据

- [ ] ADR-0212、CONTEXT（PS、NO）、CONTEXT-MAP、GLOSSARY、UC 照 AGENTS.md「改文档」同步；ADR-0184、0195 部分停用照约定；参数登记册登「未配置」行。
- [ ] 实施票已立（`.scratch/ps-external-mark-relations/issues/08-` 起），阻塞边接上（含对 first-party-api 的依赖）。
- [ ] 纯 `.md`，推送方自审。
