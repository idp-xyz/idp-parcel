# 03 渠道包裹件号随面单交易包裹结果受理形成，渠道作废与换单改适用状态

Category: enhancement
Status: in-progress——2026-10-16 完工，待进 main；见「完成记录」。此前 in-progress——2026-10-16 Claude Code 会话认领，分支 `claude-psxm`，基线 `6c919d63`。此前 ready-for-agent——2026-10-16 Claude Code 会话立（[ADR-0184](../../../docs/adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 决定三第二项与决定七）
Blocked by: 02
父票：[01](./01-external-mark-relations-have-no-model-in-parcel-shipment.md)

## 做什么

1. 标识类型加渠道包裹件号（含尾程件号），分配方为该交易的渠道服务方，形成依据为那笔面单交易。迁移建适用变化表（挂在原关联上、每条关联至多一行终态），核对读口排除已有终态的关联。
2. 记录渠道结果时，受理的包裹结果与结果同一笔事务形成关联；未受理的不形成。
3. 追加渠道作废后续动作时，被覆盖包裹（逐件或整笔）由该交易形成的关联转`已作废`。
4. 换单形成的新交易（前序关系为替代）对同一包裹受理出新号时，原交易给这件包裹形成的关联转`已被替代`，回指新关联。
5. 渠道退款不改适用状态。

## 不做

- `已失效`：有效期规则没有登记处，ADR-0184 决定七不造。
- 面单交易的生产触发面——那是面单渠道票的事；本票只保证结果一旦被记录，号码就进册。

## 完成判据

- 受理结果的渠道包裹件号在节点识别出那一件；作废后答零个候选；换单后旧号零个候选、新号识别出同一件。
- 退款后号码照常识别。
- 适用变化只插不改；同一关联转终态至多一次。

## 完成记录（2026-10-16，Claude Code 会话，分支 `claude-psxm`，代码 `d8e60661` → `58c4c496` → `d8030e69` → `3fc6fee4`）

- **落点**：领域——标识类型加 `ChannelParcelNumber`；面单交易交出 `ExternalIdentifierRelationsFormedByChannelResults` 与 `ExternalIdentifierChanges`；适用变化 `ExternalIdentifierChange`（终态封闭二值 `已作废`、`已被替代`，起因 `ExternalIdentifierChangeCause`）；作废范围与终局判断共用 `LabelTransactionFollowUpAction.covers` 与一次遍历 `LabelTransaction.voidSpan`（终局判断取最晚、关联作废取最早）。持久化——迁移 `0025`（类型封闭集加一格；关联主键加入形成依据；适用变化表主键即关联键、外键回指、后继三列与「后继不是自己」）；`LabelTransactions.Save` 与快照同一笔事务先写关联、再写变化；`ExternalIdentifiers.FindParcelsByExternalIdentifier` 排除已有终态的关联。
- **补的格**：领域——受理结果形成、未受理与结果未回不形成；逐件与整笔作废、退款与原交易上的替代痕迹不改；替代只认前序关系为替代、后继指新号与新分配方，重试不算；同一件包裹重复作废只出一条、取最早；关闭路径终局按较晚那次作废生效。真库——号随结果那一笔进册、作废后零候选而另一件照常、退款不改、再保存不重复；换单旧号零候选新号识别、先作废后替代仍是先到的终态、后继列对；再签同号（换单沿用原号、作废后再签）成新关联且后继依据是新交易。
- **判据**：✅ 受理结果的号在节点识别出那一件、作废后零候选、换单后旧号零候选新号识别同一件；✅ 退款后照常识别；✅ 适用变化只插不改、同一关联终态一次。PS、NO、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`cmd/parcel-dispatch`、`tests/`、`internal/architecture` 带 DSN 全绿；`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出。机制清点 `c9a3f800`（迁移份数加一）。
- **评审**（用户 2026-10-16 改裁允许子代理；当时唯一的非作者通道 `idp-parcel-9b` 正在清 operator-channel/26，故用 `/code-review` 的隔离子代理，两轴分开）：
  - 第一轮（钉 `d8e60661`）——Standards **阻断 1**：换单沿用原号时新旧关联同键被吞，「已被替代」的后继指回自己、号答零候选；Spec 同指一处报非阻断，并补两格同根：作废后再签同号形不成新关联、后一笔交易的作废落空。处置：立 [ADR-0185](../../../docs/adr/0185-external-identifier-relation-identity-includes-its-formation-basis.md)，部分停用 ADR-0184 决定一的键，加入形成依据（`58c4c496`）。其余非阻断（核对读口头注假话、作废范围自判、起因类型混用、`Change()` 名不达意、后继列缺非空白约束）同笔修。
  - 第二轮（钉 `58c4c496`）——两轴均无阻断。非阻断：部分停用没照 ADR 索引规矩标、头注「至多一条」不实、两处作废遍历同形、`change.change` 难读、测试夹具重复、CONTEXT 生命周期「新的」歧义、ADR-0185 漏记「后补更早作废」一格、本记录待补。全部修（`d8030e69`），另据非作者会话 `idp-parcel-9b` 对票 02 的评审在 ADR-0184 追加复核记录（`83ca983b`）。
  - 第三轮（Standards，钉 `83ca983b`）——无阻断，五条处置成立，`voidSpan` 与原 `voidCovering` 在终局判断上等价；非阻断三条（字段 `kind` 改名、替代夹具不写死服务方、取最晚一侧补用例）修于 `3fc6fee4`。
- **已知局限**（ADR-0185 后果节）：终态按写入先后立住，不按业务时间——同一条关联先作废后被替代，先写入的那种立住；同一笔交易后补一条更早的作废，库里时刻仍是先写入那条。核对答案不受影响。
- 证据层级 `S`。
