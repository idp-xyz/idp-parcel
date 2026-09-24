# 落地 ADR-0101 价卡导入：模板 → 草稿 → 批准 → 发布

Category: enhancement
Status: in-progress——2026-09-25 通道 3 立（用户授权自决）；子票 01–06 已全数立好并写明阻塞边，同日转 ready-for-agent
出处：票 [operator-workspace-gaps/04](../operator-workspace-gaps/issues/04-price-card-template-import-per-adr-0101.md)，该票转为跟踪容器指到这里。用户 2026-09-25 经 IDP 队列通道 3 问「能帮我完成03/04吗，需要完成吗」；此前已授权通道 3 自决、独立完成（原话「你是业务和系统专家，请你自决」「下一步的工作都你自己完成，没其他人了」）。

## 为什么要做

[ADR-0101](../../docs/adr/0101-operator-facing-registration-payload-shape-is-product-defined.md) 已接受：把价卡导入判为机制半边的产品能力，并否决了「维持粘 JSON、等 `PAR-INT-01`」。不做的代价是每个租户的每张卡都要工程师手翻成登记快照 JSON。首租户实施周材料里，「运营配置员登价卡」（`.scratch/tenant-implementation-01/` 的 D-06）今天只能由开发方代劳。

形状的权威不在本 spec：
- 导入、草稿、批准、发布各自是什么，以 ADR-0101 决定二至六为准。
- 待批准载体的具体形状，照 party-commercial 已落地的 [ADR-0126](../../docs/adr/0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 决定三、四：一版一行，三口分设，身份取自信封，载荷里出现身份格即拒，预览与录入走同一段解码。
- 生命周期以 parcel-pricing `CONTEXT.md`「定价方案与价表版本」的生命周期为准。

## 本批自决的几格

1. **草稿状态四格对齐 CONTEXT 生命周期。**
   - `草稿`：解析了但没过构造门，只记逐格问题，不带方案快照。
   - `已校验`：过了构造门，方案快照与规范化摘要已算出。
   - `已批准`、`已发布`。
   - 已批准之前，同一版再导入即替换那一行；已批准之后答内容已固定（同 ADR-0126 决定三）。
   - 理由：ADR-0101 决定三写明前两态「由此有载体」，只收过了构造门的会让 `草稿` 一态仍无载体。
2. **审批职责规则由 parcel-pricing 自有一条，按租户。** 形状同 PC 的 `ApprovalDutyRule`，但不共用那一条。价卡批准是 PP 生命周期「已校验 → 已批准」的一步，租户对价卡与对商业版本的审批要求可以不同（比如价卡要定价主管那一级）。参数登记册另增一行，写口只给测试用，同 ADR-0126 决定五。
3. **原始文件本体不存。** 外置证据库的连接器还没有（`product-strategy-boundary/13`），草稿与现行登记一样只记文件名与 SHA-256。
4. **端点全部以 `UnconfiguredIntake{}` 进端点表**（ADR-0085 两阶段）。换真 Intake 归 `operator-channel/04`（登记写面换真）那一族，不在本批。
5. **受控 CLI 与 seed 路径不变**（ADR-0101 Consequences）。
6. **物理格式与模板规范归子票 01 的设计文档定**，ADR-0101 决定二把它交给了设计文档。

## 子票

| 号 | 题 | 形态 | 阻塞边 | 状态 |
|---|---|---|---|---|
| [01](./issues/01-template-and-validation-spec.md) | 设计文档《价卡导入模板与校验规范》 | 纯文档 | — | resolved |
| [02](./issues/02-template-decoding-and-preview-face.md) | 模板解码与预览口 | 走并行会话那条路 | 01 | in-progress |
| [03](./issues/03-draft-register-and-submission-face.md) | 草稿册、录入口与草稿查阅读口 | 走并行会话那条路 | 02 | ready-for-agent |
| [04](./issues/04-approval-and-publication.md) | 审批职责规则、批准与发布 | 走并行会话那条路 | 03 | ready-for-agent |
| [05](./issues/05-admin-import-tab.md) | 管理台「导入价卡」签 | 前端切片 | 03 | ready-for-agent |
| [06](./issues/06-admin-drafts-tab.md) | 管理台「草稿」签 | 前端切片 | 04、05 | ready-for-agent |

## 不做

- ADR-0101 决定七所列：回测、影响分析、毛利倒挂阈值、黄金样例都不作发布门，也不给已发布版本任何编辑口。
- 其他登记册的导入（ADR-0101 决定八，各册自裁）。
- 草稿撤回（ADR-0126 决定三同样没开；要时另立）。
