# 01 CONTEXT 运营查阅语言 + ADR(MCP-5)

Category: enhancement
Status: open

地盘、阻塞边与纪律见 ../spec.md;取证依据见 `.scratch/admin-web-uiux-20260824/tracking-scope-decision-brief.md`(下称简报),两处不复述。

## 活

1. **CONTEXT.md 补「运营查阅」授权语言**(docs/domain/visibility-exception/CONTEXT.md):
   - 简报第二节末条取证:CONTEXT 现只有客户查询的授权词条,内部运营人员读投影的授权语义(谁、什么范围、要不要按客户分区)无对应表述。
   - 要补的语义:运营查阅的主体(租户内运营人员)、作用域(租户级,租户是最高数据隔离边界,引 ADR-0003)、与客户可见性的分界(运营查阅面向内部投影全维度,不背客户探针合并义务与披露删减;客户视图词条「它不同于内部全程追踪投影」的另半边);与「追踪摘要」词条「只用于阅读和检索」的衔接。
   - 用 CONTEXT 现有小节结构与原词,不自造译法;若既有 UC 需要引用更新,一并改(改领域语言先 CONTEXT 后 UC,AGENTS.md 顺序)。
2. **新 ADR**(docs/adr/):
   - 决定:运营追踪查阅走独立读口消费 `ProjectionStore`,不复用客户视图端点 GET /customer-tracking-view。
   - 记录四层结构性理由(引简报,覆盖/键形状/内容删减/探针合并)与被否决的选项 A(复用加作用域参数:覆盖缺口无解、一端点两套披露语义、UC-VE-008 验收口径被撕)。
   - 立作用域模型形状:租户级运营作用域——本仓现有 AuthorizedQueryScope 先例是客户账户作用域,租户级全景作用域无先例,ADR 要把这个形状钉住(含空作用域语义、与 PAR-INT-01 的关系:认证未登记前装配 UnconfiguredIntake 一律 403,ADR-0055)。
   - Status 按 docs/adr 现行惯例写;拿不准 Accepted 的授权来源就写 Proposed 并在报告里说明,由 1 号转用户确认。
3. **adr/README.md 索引行**:只加自己行,不动邻行(索引纪律)。

## 完成标准

两文档(CONTEXT.md 增补、新 ADR)与索引行落库,send_to_session 1 报 SHA 与 ADR 编号;票 02 以该 SHA 为开工条件。
