# VE（visibility-exception）文档自洽审计

- 读的版本：只读 worktree `/home/tops/workspace/idp-parcel-audit`，detached @ `6b0074d0`；在途分支未读。
- 口径：`.scratch/docs-consistency/README.md` 目标 1–6、A/B/C、严重度；AGENTS.md「改文档」「写代码注释」。
- 范围：`docs/domain/visibility-exception/CONTEXT.md`；`docs/application/visibility-exception/` 全部 8 份 UC；VE 相关 ADR（0051、0065、0066、0074、0076、0077、0078、0102、0104、0136、0139、0142、0176、0197、0205、0206、0219、0223、0224、0225、0226，并按需读 0100、0150）；CONTEXT-MAP、GLOSSARY 的 VE 条目；`internal/visibilityexception/**`、`cmd/parcel-dispatch` 的 VE 各路、`cmd/parcel-api` 的 VE 端点，以及演示种子 `scripts/demo-seeds/data/visibility/`（参考配置）。
- 计数凡出现在「证据」里，都是论点本身，取证于 `6b0074d0`。

共 22 条：高 2、中 14、低 6。

---

## 高

### VE-01 客户全程追踪视图的已发布版本被原地覆盖，且每个投影新版本都换一代视图

- **类别**：4（文档与代码一致）
- **位置**：`docs/domain/visibility-exception/CONTEXT.md`「客户可见性与通知」三句——
  - 「来源更正、事实有效性变化、身份谱系变化、ETA 新版本或终局更正必须重新派生客户视图并形成追加更正或明确替代关系。原已发布版本保留，但不再有效的内容不得继续显示为当前事实。」
  - 「客户全程追踪视图的版本由披露更正与替代关系驱动，不与内部投影版本一一对应；投影重新派生本身不要求形成新的客户视图版本，也不为每个投影版本各留一代。」
  - 「已经向客户发布的里程碑、ETA、异常说明或通知出现更正时，必须以追加更新或明确替代关系表达，不得静默删除或改写历史披露。」
  - Lifecycles「客户全程追踪视图」：「……形成新的客户视图版本并标明更正或替代关系；原发布历史保留。」
  - UC-VE-008 `AT-VE-161`：「形成追加更正/替代视图，原版本保留且不再作为当前结果」。
- **证据**：
  - `internal/visibilityexception/adapters/postgres/customer_view.go` 的 `CustomerViews.Save`：替代分支是 `UPDATE visibility_exception.customer_view SET version = $4 … WHERE … AND version = $16`。表主键是（租户，客户账户，包裹），见 `migrations/visibility_exception/0001_customer_view.sql`。前一版那一行被原地改写，`prior_version` 指着一份读不回来的行。
  - `ports.CustomerViewStore` 的注释写「原版本由替代关系承担历史，库只管当前」，`domain.RehydrateCustomerView` 的注释写「库里只有当前版本」。投影上同样的形状已被 ADR-0065 否决：Context 一说它「指针在，靶子不在」，决定二说「原版本的留存以存下来为准，不以可重放为准」。ADR-0065 Consequences 还明写「客户视图那一层不变：CONTEXT 既有硬句……原样适用」，所以 CONTEXT 这几句对客户视图仍然生效。
  - `application.DeriveCustomerViewHandler.Handle` 只要 `current.BasedOn() != command.Projection.Version()` 就调 `current.Supersede(...)`，四维内容没变也一样。结果是每个投影新版本都换一代客户视图，与「不与内部投影版本一一对应」正好相反。
  - 覆盖「原已发布版本保留」的测试是 `derive_customer_view_test.go::TestANewerProjectionSupersedesTheCurrentViewKeepingHistory`，但它用的是内存替身。真库测试 `customer_view_test.go::TestSupersedeAdvancesTheVersionWithOptimisticLock` 只验「指回」，不验「留存」。
  - 票 `ve-customer-view-per-projection-version/01` 裁「不需要按投影版本留代」，理由是「main 的视图链……CONTEXT『原发布历史保留』已保证历史不丢」。这个前提在代码里不成立。
  - 产品文档 `docs/product/IDP-PARCEL-BUSINESS-PROCESS-AND-MODULE-GUIDE.md` 写「投影和客户视图只增版本」，同样与代码不符。
- **归类**：C（代码错）
- **严重度**：高。这是对外发布物，CONTEXT 明令「不得静默删除或改写历史披露」，而该路径已在 `cmd/parcel-dispatch` 生产装配里跑。
- **建议修法**：立代码票。
  - `customer_view` 照迁移 0016 的投影形状改成版本表加当前标记，只增不改写。
  - 只在四维内容或披露依据变化时才形成新版本，投影换版本本身不触发。
  - 同时重开票 ve-customer-view-per-projection-version/01 的裁定前提，在票上补一句即可，CONTEXT 不用改。

### VE-02 客户视图的 ETA 维在没有任何客户可见 ETA 判断时也答 `SHOWN`

- **类别**：4
- **位置**：
  - CONTEXT「客户可见性与通知」：「客户全程追踪视图只基于当前有效的全程追踪投影、获准对客展示的 ETA、`parcel-shipment` 提供的客户服务终局，以及已经批准披露的客户可见异常形成。」
  - Lifecycles「客户全程追踪视图」：「某一维信息待确认或不可披露 → 只对该维返回待确认、暂不可用或不展示，不虚构里程碑、ETA、终局或异常说明。」
  - UC-VE-008 主流程第 4 步：「读取 `UC-VE-003` 的客户可见性判断；只有达到适用质量与合同门槛的 ETA 才进入视图，不能使用计划时间或内部预测填充。」
  - `AT-VE-158`：「不展示 ETA，也不使用计划或承诺时间填充」。
- **证据**：
  - `internal/visibilityexception/application/derive_customer_view.go` 的 `dimensionsFrom` 里，ETA 一维只翻译披露策略的答复（`dimensionFrom(answer.ETA)`）。头注自己承认：「ETA 一维要的是 `UC-VE-003` 的对客可见判断（`AT-VE-158`），那份判断不在投影里，不在这里补。」
  - 终局一维有 `domain.FinalDimensionOver` 兜底，ETA 维没有。
  - `NewFormETAHandler` 在 `cmd/` 下没有调用方，生产路径上从不形成 ETA，更谈不上客户可见 ETA 判断。
  - 演示策略 `scripts/demo-seeds/data/visibility/07-disclosure-policy-v1.json` 给 `SYN-ACCOUNT-01` 的 `eta` 配的是 `SHOWN` / `SYN-VE-CONTENT-ETA-BAND-V1`。客户读到「ETA：展示」，背后却没有任何 ETA。
  - HTTP 响应体 `dimensionBody` 只带 `contentRef`，不带任何 ETA 值。
  - 里程碑一维有同样的毛病，但较轻：投影里只有未归类条目时（例如只有初始路由那一份），策略照样答 `SHOWN`。
- **归类**：C
- **严重度**：高。这是对客结果，读者会误判「ETA 已对客可见」。
- **建议修法**：立代码票。
  - ETA 维照 `FinalDimensionOver` 的做法加一道：没有当前有效的客户可见 ETA 判断就答待确认。
  - 里程碑维在投影没有已归类条目时也答待确认。
  - 另在 UC-VE-008 结果语义里写明 `contentRef` 指的是什么（是策略允许展示的内容集，不是内容本身）。

---

## 中

### VE-03 被替代条目不在投影版本里，与 CONTEXT、ADR-0065 决定七相反

- **类别**：4
- **位置**：
  - CONTEXT「追踪投影与里程碑」：「被替代的条目继续留在投影版本内并标明替代关系；标准追踪里程碑、各追踪维度和追踪摘要只由当前有效即未被替代的条目派生。」
  - 同节：「运营追踪查阅面向投影的全部并行维度、冲突关系与被替代条目」。
  - Lifecycles：「被指名的前身条目转为已被替代并继续保留」。
  - ADR-0065 决定七：「被替代条目留在版本内但不参与派生。」
- **证据**：
  - `application.DeriveProjectionHandler.Handle` 先跑 `domain.CurrentlyEffective(facts)` 再逐条归类成条目，所以被替代事实不进新版本的条目，只留在 `accepted_fact` 库里。头注写的是：「留在事实库是为可追溯，不进条目是为不同时呈现两个互斥结果」。
  - 测试 `derive_projection_test.go::TestASupersededFactStaysArchivedButLeavesTheEntries` 断言 v1 不在条目里。
  - 运营读口 `adapters/http/query_tracking_projections.go` 的 `projectionBodyOf` 只转写条目，当前版因此看不见被替代条目。
  - 管理台 `apps/admin-web/src/pages/visibility/TrackingProjectionPage.tsx` 的 `currentEffectiveEntries` 按 CONTEXT 的假设（「被替代条目继续在场是为可追溯」）在前端再滤一遍。在后端现在的形状下，这一步什么也不滤：前后端对同一读模型的理解相反。
- **归类**：C。若 owner 认定「事实库留档已够」，就改走 B：新 ADR 部分停用 ADR-0065 决定七那句，再改 CONTEXT 两处与 UC-VE-002 `AT-VE-044` 的措辞。
- **严重度**：中
- **建议修法**：二选一后立票。
  - 按文档改代码：被替代条目留进版本，带「已被替代」的派生标记，不参与派生。
  - 或按代码改文档：走上面那条 B。

### VE-04 替代链分叉先按业务时间裁决，裁得出就不立信号，裁出的顺序也被丢弃

- **类别**：4
- **位置**：
  - CONTEXT：「同一份已接受事实被两份以上事实同时指名为前身时，替代链分叉，本上下文不得择一。分叉按冲突处理：保留各项事实及其关系，投影保持信息待确认并形成适用异常信号。」
  - ADR-0065 决定六：「替代链分叉进冲突。同一前身被两份以上事实指名时 VE 不择一：保留各方与关系，投影保持信息待确认并形成适用异常信号。」
  - ADR-0065 决定三：「VE 不从业务时间、到达先后或任何其他线索推断谁更正了谁」。
  - UC-VE-002 `AT-VE-043`。
- **证据**：
  - `DeriveProjectionHandler.judgeForks` 对每处分叉调 `domain.ResolveByBusinessTime`，`judgment.Resolved()` 为真就 `continue`：两个后继业务时间不同的分叉不立信号。
  - 裁出的顺序不落在任何地方，既不进投影，也不存成判断。两个后继照常归类、照常派生里程碑，于是投影上同时有两个互斥结果，也不是「信息待确认」。
  - 测试 `TestAForkResolvableByBusinessTimeRaisesNoSignal` 断言「业务时间排得出先后的分叉不是异常」。
  - 这一判法出自票 `mechanism-executor-triage/08` 的 VE-d 组，不来自任何 ADR。
  - 另：冲突关系 `application.FactConflict` 只随本轮结果返回，不持久化；冲突信号规则没配置时什么都不留。CONTEXT「必须保留各项事实及冲突关系」因此只有「事实」那一半成立。
- **归类**：B。CONTEXT「分叉按冲突处理」可以读成「先走冲突裁决」，ADR-0065 决定六却无条件要求待确认加信号；代码两头都不完全对。
- **严重度**：中
- **建议修法**：立 ADR 代决三件事：
  - 分叉要不要先走业务时间裁决，与 ADR-0065 决定三、六是什么关系；
  - 裁得出时顺序落在哪里、投影怎么呈现；
  - 冲突关系要不要持久化。

  代决后改 CONTEXT 那一句与 `AT-VE-043`，再按需立代码票。

### VE-05 客户视图把「本账户已归属、视图未形成」与「无权」合成同一格 `VIEW_NOT_FOUND`

- **类别**：2（UC 与 ADR、代码一致）
- **位置**：UC-VE-008——
  - 结果语义：「当前无可见结果 | 对象已授权，但当前没有满足对客展示条件的里程碑、ETA 或终局；不表示未收寄、未运输或遗失」；
  - 元数据：「未形成结束 | 视图派生、授权判断或结果保存未完成，不能伪装为无轨迹或无权查询」；
  - 主流程第 8 步：「返回当前视图、当前无可见结果、信息待确认、无权查询、标识冲突或未形成」。
- **证据**：
  - `adapters/http/query_customer_tracking_view.go` 的业务结果是封闭两格 `CURRENT_VIEW` / `VIEW_NOT_FOUND`，头注写「包裹不存在、不属于请求账户、或视图尚未形成，一律这一格」。
  - ADR-0223 越权风险点 3：「接受前查视图答 `VIEW_NOT_FOUND`，与不存在不分……要分开得在 VE 的答复代数里加一格，那会给枚举开口，归 VE owner；本记录不改。」
  - ADR-0076 Context 四把这一格的三种含义称作「ADR-0029 纪律与 UC-VE-008 验收」，与 UC-VE-008 自己的结果语义相反。
  - 实际后果：包裹已接受、初始路由投影还没形成时（或 NR 一直形不成投影时），本账户查自己的包裹，拿到的答复与无权查询相同。
- **归类**：B
- **严重度**：中
- **建议修法**：代决「视图未形成时，对本账户已归属的包裹答什么」。
  - 若维持合并：在 UC-VE-008 结果语义里写明这一格覆盖哪些情形，并收回「当前无可见结果」对这一情形的承诺。
  - 若分开：立代码票加一格，按客户归属反查确认是本账户的包裹。

### VE-06 `AT-VE-169` 重号：UC-VE-003 与 UC-VE-008 各用一次

- **类别**：2（同一编号两条不同验收）
- **位置**：
  - `UC-VE-003-FORM-ETA-AND-VISIBILITY-GAP.md`：「`AT-VE-169` | 内部 ETA 已形成，且适用质量、合同和客户服务规则条件明确满足 | …」
  - `UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md`：「`AT-VE-169` | 包裹全部源事实已入账后，货主客户账户关系才建立 | 在客户归属确立时按当前投影形成客户视图，不等待新的源事实」
- **证据**：
  - 代码里十余处注释写「UC-VE-008 AT-VE-169」，例如 `adapters/inbox/acceptance_decision_consumer.go`、`adapters/parcelshipment/derive_customer_view_on_acceptance.go`、`cmd/parcel-dispatch/routes_customer_view.go`、`cmd/parcel-dispatch/acceptance_rederive_test.go`。
  - `.scratch/at-coverage-inventory.md` 却只把 `AT-VE-169` 登成 UC-VE-003 的 ETA 那一行，归入 ④「机制未落地」。已被测试钉住的那条（UC-VE-008）在盘点里查不到，未落地的那条反而占着这个号。
  - 谁先占号：ADR-0065 Consequences「`AT-VE` 现有 001 至 170 连续无空位」（2026-08-19）说明 UC-VE-003 的两行在先；UC-VE-008 那行是票 ve-008-late-account-rederive/01（2026-08-20）后加的。
- **归类**：A
- **严重度**：中
- **建议修法**：UC-VE-003 的 ETA 那一行没有任何代码引用，改用下一个空号，再重生成 AT 盘点。也可以反过来改 UC-VE-008 那行，但要连带改十余处代码注释。

### VE-07 CONTEXT 没有「入账不派生」这一走向，三份 ADR 都在用

- **类别**：1（CONTEXT 是现行规则唯一出处）
- **位置**：VE CONTEXT「追踪投影与里程碑」与 Lifecycles「全程追踪投影」只写了两种走向：已接受事实进来就派生；冲突则待确认并形成信号。没有第三种。
- **证据**：三份已接受 ADR 立了第三种走向——事实记进事实库，但不进投影条目、也不形成信号——代码照此实现：
  - ADR-0197 决定五：「记了同层冲突的那一份：入账不派生。」
  - ADR-0205 文末补记：「没声明段、段不在册、那一刻段里没有在控对象，都入账不派生；段册读不动是未决。」
  - ADR-0224 决定三：「拒收与待确认不让对象进段，入账收工」。

  UC-VE-002 `AT-VE-180`、UC-TF-005 `AT-TF-159` 都以「入账不（补）派生」为预期结果。只读 CONTEXT 的作者不会知道有这一格。
- **归类**：A
- **严重度**：中
- **建议修法**：
  - 在 CONTEXT「追踪投影与里程碑」补一条「可入账而不派生」的条件族：源上下文自己标记的同层冲突；声明的段不在册，或那一刻段里没有在控对象；控制事实没让对象进段。
  - 在 Lifecycles 投影一节补一行。
  - 三份 ADR 只作理由来源，由 CONTEXT 引用。

### VE-08 关务同层冲突不形成「待确认」与信号，与 `AT-VE-006`、CONTEXT 冲突规则相反

- **类别**：4
- **位置**：
  - UC-VE-001 `AT-VE-006`：「两项已接受关务结果冲突且关务尚不能裁决 | 保留冲突引用并形成适用信号；不选择更宽松或更严重来源」。
  - CONTEXT：「冲突仍无法裁决时，必须保留各项事实及冲突关系，投影保持信息待确认并形成适用异常信号」。
- **证据**：
  - `adapters/customscompliance/derive_on_external_result.go` 遇 `record.LayerConflict` 就入账不派生，不立信号。
  - ADR-0197 越权风险点 5：「同层冲突不进投影，VE 也就看不见『关务进展待确认』……VE 侧的冲突信号……需要 CC 先给出冲突关系，另立票。」
  - ADR-0226 索引注记：「VE 冲突信号另票」。
  - 承接票 `demo-chain-gaps/124` 状态为 needs-triage。
- **归类**：C（已有票 124）
- **严重度**：中
- **建议修法**：不另立票。修订票只在 `AT-VE-006` 旁注明当前缺口并指向票 124；代码由票 124 做。

### VE-09 异常信号不带事实依据与因果条件，发作期永不结束（出发门禁阻断信号、冲突信号同病）

- **类别**：4
- **位置**：CONTEXT——
  - 「每个信号必须保存对象、类型、规则版本、判断时间、事实依据、可信度、当前发作期和变化历史。」
  - 「同一对象、类型、因果条件和连续影响期内的重复命中更新同一信号发作期。条件明确解除后，该发作期结束；再次发生时建立关联的新发作期。防抖、宽限和恢复边界必须具有规则版本。」
  - 「必须分别记录风险条件首次成立、信号生成……」
  - 另见 UC-VE-004 `AT-VE-065`。
- **证据**：
  - `application.RaiseSignalCommand` 只有租户、类型、包裹、规则、可信度、命中时刻，没有事实依据，也没有因果条件。它的头注却引同一句 CONTEXT，说「编排不替命中补任何一样」。
  - `domain.SignalEpisode` 按（包裹，类型）定位，没有因果条件这一维；`RecordHit` 只记次数与末次时间；生产代码里 `End` 没有调用方。
  - ADR-0205 文末补记越权风险点 22、23 已记下这一缺口，其复核记录写「复核为应修订，见票 demo-chain-gaps/135」。票 135 状态 ready-for-agent，冲突信号的存量问题也在同票。
  - UC-VE-004 正文已如实注明这一缺口。
- **归类**：C（已有票 135）
- **严重度**：中
- **建议修法**：CONTEXT 不动（它是对的）。票 135 做完后，删去 UC-VE-004 的暂挂说明，并修正 `RaiseSignalCommand` 头注里与字段不符的那一句。

### VE-10 段里在控的袋或集运单元被当成一件包裹投影

- **类别**：4
- **位置**：
  - CONTEXT Language「全程追踪投影」：「面向明确包裹及其身份谱系……」。
  - Rules：「包裹身份、外部标识和身份谱系由 `parcel-shipment` 提供……集运、装载或同批关系不合并包裹身份和客户轨迹。」
- **证据**：
  - ADR-0205 文末补记越权风险点 20：「段里在控的若是袋或集运单元，VE 把它当一件包裹投」。
  - ADR-0225 越权风险点 9：「全程追踪仍不按成员投」。
  - `adapters/parcelshipment/customer_account_lookup.go` 头注：「追踪包裹引用可能装着集运单元号」。
  - 票 `demo-chain-gaps/123` 状态为 needs-triage。
- **归类**：C（已有票 123）
- **严重度**：中
- **建议修法**：不另立票，由票 123 承接；修订票只在 UC-VE-002 的 `AT-VE-046` 附近注一句当前缺口。

### VE-11 ETA 存储整行覆盖，两种来源的预测同里程碑互相覆盖

- **类别**：4
- **位置**：
  - CONTEXT「ETA 与可见性缺口」：「承运商提供的预测与运营企业派生的预测分别保留来源和口径。新的 ETA 形成新版本，不覆盖历史预测，也不修改客户承诺、路由计划或实际事实。」
  - GLOSSARY「预计到达时间（ETA）」：「新的 ETA 不覆盖历史」。
  - UC-VE-003 `AT-VE-051`：「两个来源都提供 ETA | 来源和口径分别保留……」；`AT-VE-057`：「原判断保留」。
- **证据**：
  - `adapters/postgres/eta_gap_notification.go` 的 `ETAs.Save` 注释写「同（租户+包裹+里程碑）整行更新——刷新不是第二行」，SQL 是 `ON CONFLICT (tenant_id, parcel_ref, milestone_ref) DO UPDATE`。
  - `ports.ETAStore` 注释写「历史版本由 Refresh 的指回关系承担，库只管当前」，又是 ADR-0065 否决过的形状。
  - 键里没有来源这一维：`application.FormETAHandler.FormETA` 按（租户，包裹，里程碑）`FindCurrent`，会取到另一种来源的预测，再 `Refresh` 盖掉它。
  - 同一函数头注「历史预测不覆盖」与存储的实际行为相反。
  - `FormETAHandler` 在 `cmd/` 下没有调用方，尚未上生产路径。
- **归类**：C
- **严重度**：中（未接线，但谁来接线都会继承这个错）
- **建议修法**：立代码票，照 ADR-0065 / 迁移 0016 的形状改成版本只增，键加来源维；同笔改正两处注释。

### VE-12 披露「待授权」装不下内容草稿

- **类别**：4
- **位置**：
  - UC-VE-001 结果语义契约：「客户披露待授权 | 已具备候选内容但需要有权角色确认 | 客户、对象、内容草稿、依据、所需授权和时限」；`AT-VE-024`：「形成客户披露待授权和内容草稿」。
  - UC-VE-006：「草稿/待授权 | 内容已生成但尚未取得发布授权」；`AT-VE-100`：「形成草稿和待授权，不提交渠道」。
  - 参数登记册 `PAR-VIS-07`：「默认草稿经授权确认」。
- **证据**：`domain.DecideDisclosure` 定「披露格必带内容快照，非披露格必不带」，`AwaitingAuthorization` 一带内容就是 `ErrInvalidDisclosure`；`DisclosureDecision.Content()` 也只在披露格给出。结果是待授权决定里没有草稿，有权角色无从确认内容。
- **归类**：C
- **严重度**：中
- **建议修法**：立代码票：待授权格带「内容草稿」引用，与通知的内容快照分开；暂不披露格仍然不带内容。

### VE-13 VE 消费的事实类型词没有单一权威出处

- **类别**：6（单一权威）
- **位置**：CONTEXT「已接受事实必须携带源上下文拥有的事实类型；标准里程碑映射按源上下文与事实类型版本化登记……」。
- **证据**：
  - 租户登里程碑映射用的键（`adapters/registrationjson` 的 `factKind`）在代码里有 26 个词：`node-intake`、`offsite-pickup`、`effective-delivery`、`handover-handed-over` / `-refused` / `-pending-confirmation`、`movement-departure` / `-in-transit` / `-arrival`、`external-result-*` 八个、`customs-case-established`、`declaration-submission-formed`、`final-outcome-adopted` / `-not-adopted`、`initial-route-formed` / `-no-current-route`、`exception-journey-alternate` / `-return`、`external-carrier-tracking`。
  - 文档里只有三处有定义：`external-carrier-tracking` 在 GLOSSARY「外部承运轨迹事实」与 TF CONTEXT；`external-result-*` 在 ADR-0197 决定二；`movement-*` 在 ADR-0205 文末补记。其余没有任何文档定义，全是 VE 消费侧适配器自己铸的，源上下文的 CONTEXT 并不「拥有」它们。
  - ADR-0197 越权风险点 2、ADR-0205 补记越权风险点 18 都说「词一经租户映射引用，改名要迁每个租户的映射」，也就是说这些词是租户可见的配置键。
- **归类**：A
- **严重度**：中
- **建议修法**：选一处立单一权威表——各源 CONTEXT 各列本上下文交给 VE 的类型词，或在 UC-VE-002 立一张表并注明每个词的所有者。GLOSSARY 那一句与两份 ADR 改为引用这张表。

### VE-14 CONTEXT-MAP 与 UC-VE-008 把「客户查询授权」归给 party-commercial

- **类别**：1
- **位置**：
  - `docs/domain/CONTEXT-MAP.md` 的 `party-commercial / parcel-shipment → visibility-exception` 一条：「参与方与商业提供货主客户账户、责任法人、服务产品、客户合同、客户查询授权及适用异常服务规则」。
  - UC-VE-008 权威依据 `ACCOUNT` 行：「货主客户账户、参与方关系和授权依据 | 参与方与商业上下文」。
- **证据**：
  - ADR-0139 决定二：「渠道登记册结构是 `internal/accessidentity` 的客户渠道册……」；决定四：`/customer-tracking-view` 走首方客户渠道 Intake。
  - UC-VE-008 自己的首发约束段写「挡住查询的是……客户渠道册上的查阅授予（`PAR-INT-13`）」，与它的 `ACCOUNT` 行矛盾。
  - 代码走的是 `adapters/accessidentity/customer_tracking_view_authenticator.go`。PC CONTEXT 里没有「查询授权」。
- **归类**：A
- **严重度**：中
- **建议修法**：
  - CONTEXT-MAP 那条边去掉「客户查询授权」，或改写成「客户查询授权由共享身份能力的客户渠道册给出（ADR-0139）」。这一处与横切组协调。
  - UC-VE-008 `ACCOUNT` 行把授权依据拆出来，指向 ADR-0139 与 `PAR-INT-13`。

### VE-15 ADR-0076 决定三仍说运营接入认证属实例半边，与 ADR-0100 矛盾而无停用关系

- **类别**：3（ADR 停用链）
- **位置**：ADR-0076 决定三：「运营接入面的认证方式属接入渠道实例半边、未登记……未登记前装配 UnconfiguredIntake，一律 403 `ACCESS_CHANNEL_NOT_CONFIGURED`」；Consequences：「接入渠道认证参数未登记前，没有任何选项能让页面显示真轨迹——本记录不改变该深度上限，真轨迹属实例半边」。Status 行只有 `Accepted`，没有前向指针。
- **证据**：
  - ADR-0100 决定一：「运营操作者身份是产品自有的接入渠道族，属机制半边」。它部分停用了 ADR-0085、ADR-0072，没有停用 ADR-0076（以及 ADR-0077）的决定三。
  - ADR-0100 决定四写的覆盖面是「ADR-0085 Decision 一那一族登记端点与 ADR-0077 那一族目录查阅端点」，没有点名 `/tracking-projections`。
  - 代码 `adapters/http/operator_read_intake.go` 自称「操作者渠道在运营查阅面的 Intake（ADR-0100 决定四）」，接在 `/tracking-projections` 上，答 401/403 的 `OPERATOR_*` 各格。
  - 同类情形：ADR-0078 决定一排除 `/customer-tracking-view` 时给的理由「那正是 `PAR-INT-01` 拥有的实例半边语义」，已被 ADR-0139 决定一收窄（首方渠道不归 `PAR-INT-01`）；ADR-0139 的部分停用清单没有列 ADR-0078。排除这个结论本身仍然成立。
- **归类**：B
- **严重度**：中
- **建议修法**：
  - 代决一份短 ADR，明文部分停用 ADR-0076（与 ADR-0077）决定三那一句，并把 `/tracking-projections` 列入操作者族的覆盖面；两头加前向指针。
  - ADR-0078 那句理由同笔加注记，指向 ADR-0139。
  - ADR-0077 属横切组，需协调。

### VE-16 演示租户把冲突信号规则登成 `CUSTOMS_HOLD`

- **类别**：4（参考配置与规则一致）
- **位置**：CONTEXT「关务相关异常只能消费 `customs-compliance` 已经接受且范围明确的事实或判断……查验、扣留、税费或内部限制等正常程序事实也只有命中已登记异常或客户影响规则时才形成异常信号。」
- **证据**：
  - `scripts/demo-seeds/data/visibility/09-conflict-signal-rule-v1.json` 的 `"signalKind": "CUSTOMS_HOLD"`。冲突信号只由 `DeriveProjectionHandler.raiseConflict` 形成（替代链分叉裁不了时），来源可以是运输交接、交付等任何源，根本不是关务事实。
  - 同一信号类型在 `02-triage-rules-v1.json` 里走 `ATTACH_TO_EXISTING`，在 `08-exception-disclosure-rules-v1.json` 里对 `SYN-ACCOUNT-01` 可披露（内容 `SYN-VE-CONTENT-CUSTOMS-HOLD-V1`）。
  - 后果：一次交接更正分叉，在演示动线上会变成「关务扣留」，并可对客披露。`scripts/demo-seeds/README.md` 也把这一行写进了「对外怎么说」一段。
- **归类**：C（参考配置错）
- **严重度**：中
- **建议修法**：立票改种子：冲突信号用一个中性类型词（例如来源冲突类），并同步分诊、披露两份种子与 README。

---

## 低

### VE-17 追偿动作种类是封闭二值，缺「补充主张」

- **类别**：4
- **位置**：CONTEXT Language「追偿通知或主张动作」：「预先通知、正式主张和补充主张可以是不同动作」；UC-VE-007 主流程第 8 步：「分别准备预先通知、正式主张或补充主张」；输入语义「预先通知、正式主张或补充主张的独立动作」。
- **证据**：`domain.RecoveryActionKind` 的注释是「追偿动作的封闭二值：预先通知与正式主张」，只有 `PreliminaryNotice`、`FormalAssertion` 两个值，`valid()` 也只认这两个。
- **归类**：C
- **严重度**：低（首发以受控模拟为主）
- **建议修法**：立代码票加第三值；不改的话，也要在 UC-VE-007 里注明首发只支持两种动作。

### VE-18 GLOSSARY 缺两个跨上下文用到的 VE 词，并有一处异名、一处未定义词

- **类别**：6
- **位置与证据**：
  - 「客户归属」只在 VE CONTEXT 里定义，但 UC-CC-001（「保存客户归属和来源资料引用」、`AT-CC-437`）与 SCENARIOS 都在用，GLOSSARY 没有这个词条。
  - 「客户全程追踪视图」是 VE 两大发布物之一，CONTEXT-MAP 与多份 ADR 都用到它，GLOSSARY 也没有词条；而同族的「全程追踪投影」「追踪摘要」都有。
  - 同一概念两个名字：CONTEXT Language 的词头是「追偿事项」，GLOSSARY 的词头是「供应商或保险追偿」，两处定义同义。GLOSSARY「应追偿金额」那条还把「追偿事项」列为避免使用词——它的本意是「别把金额叫成追偿事项」，读者容易读反。
  - GLOSSARY「状态」词条写「全程追踪状态是面向包裹旅程的派生投影」。「全程追踪状态」这个词在任何地方都没有定义，而且与 CONTEXT「追踪摘要……不是一条覆盖各维度的统一状态机」的口径相抵。
- **归类**：A
- **严重度**：低
- **建议修法**：GLOSSARY 补「客户归属」「客户全程追踪视图」两个词条，CONTEXT 改为引用；「追偿事项」与「供应商或保险追偿」统一词头，另一个列为同义；「状态」词条那句改成「全程追踪投影是……」。

### VE-19 VE 行为的验收行仍暂挂在 UC-TF-005

- **类别**：2
- **位置**：UC-VE-002：「移动事实已形成那一路的消费验收暂挂在 UC-TF-005 `AT-TF-157`、`AT-TF-159`、`AT-TF-160`、`AT-TF-161`……待 VE owner 复核时迁入本用例或另编 `AT-VE` 号」；UC-VE-004 对 `AT-TF-158`、`160`、`161` 有同样一句。
- **证据**：UC-TF-005 那几行的预期结果全是全程追踪自己的行为——投影派生、信号形成、入账不派生、更正替代。两份 VE UC 都只说「待迁」，没有去处。
- **归类**：A
- **严重度**：低
- **建议修法**：VE 组修订时直接迁入 UC-VE-002、UC-VE-004 并另编 `AT-VE` 号；UC-TF-005 原行改为引用。这一步与 TF 组协调。

### VE-20 VE 的 Go 注释里有会腐的计数

- **类别**：5（引用规范；AGENTS.md「改文档」明令 Go 注释同受约束）
- **位置与证据**：
  - `adapters/parcelshipment/derive_on_final_outcome.go` 包注释：「终局投影是四路投影里唯一一条来源为 parcel-shipment 的」。实际 `cmd/parcel-dispatch` 里共用 `projectionDerive` 的 VE 投影各路已有 14 路（12 路正向，2 路补派生）。
  - `cmd/parcel-dispatch/assemble.go` 的 `newTenantBoundProjectionDerive` 头注：「九路投影共用一份……不要为揽收、交付、交接、终局、初始路由、异常旅程、关务案件、申报提交再复制八份包装」。外部追踪、外部结果、移动事实与两路补派生都不在列。
  - `adapters/postgres/notification_policy.go`：「理由同本包另外三个视图」。同包注释写「租户在装配期固定」的视图共有 8 个。
  - `adapters/transportfulfillment/derive_on_transport_handover.go` 与 `derive_on_effective_delivery.go`：「第三个源接上来时依同一条不变量……」。给出替代关系的适配器现已有 6 个（交接、交付、移动事实、外部追踪、终局、申报提交）。
- **归类**：A
- **严重度**：低
- **建议修法**：计数改为符号名（如「`routes_*.go` 里各投影路」）或直接删掉；「第三个源」改成「新的源接上来时」。

### VE-21 VE 的 Go 注释里有不是原文的「」引文和错指

- **类别**：5
- **位置与证据**：
  - `adapters/http/query_claims_recovery_records.go` 引「CONTEXT『三判分步』」与「CONTEXT 边界『赔付金额的计算』」：两句在 VE CONTEXT 里都查不到。
  - `application/send_disposition_request.go` 引「（CONTEXT）已经把『关闭后还要动作』引去关联新案件」：CONTEXT 里没有这句原文。
  - `adapters/customscompliance/derive_on_external_result.go` 引「CONTEXT『不得按最后到达直接改变当前判断』」：这句出自 CC CONTEXT，写在 VE 包里不加限定，读者会以为是 VE CONTEXT。
  - `application/derive_customer_view.go` 写「`UC-VE-003` 的对客可见判断（`AT-VE-158`）」：`AT-VE-158` 在 UC-VE-008。
- **归类**：A
- **严重度**：低
- **建议修法**：引文改成逐字原文，或去掉引号改为转述并指明出处；CC 那句写成「CC CONTEXT」；`AT-VE-158` 前标「UC-VE-008」。

### VE-22 VE 相关 ADR 的 Status 行把同一次部分停用写了两遍

- **类别**：3（infra-speed/07 已并入本轮）
- **位置与证据**：
  - ADR-0078 的 Status 行括注里写了一遍「**部分停用**，2026-09-02：Decision 四中『按环境选择的只有装配点上查阅行的 Intake 一件事』一句已由 ADR-0091 停用」，索引注记里又写一遍「**部分停用**：其 Decision 四中『按环境选择的只有装配点上查阅行的 Intake 一件事』一句已由 ADR-0091 停用……」。
  - ADR-0104 的 Status 行括注与索引注记，各写了一遍 ADR-0176 消解「客户服务规则没有对应的回退物——产品没有默认期限」这一句的前提。
  - ADR-0205 只在括注里写「被停原句见索引注记」，属于指引，不算重复。
- **归类**：A
- **严重度**：低
- **建议修法**：交给横切组的 infra-speed/07 一并处理：括注只留指引，索引注记保留全文。

---

## 已核无问题的要点

| 要点 | 核对结果 |
|---|---|
| VE CONTEXT、8 份 UC 及所列 VE ADR 的相对链接与锚点 | 脚本逐条解析，全部能解析 |
| VE CONTEXT 与 8 份 UC 里的行号、跨文件计数、跨文件位置引用 | 未发现。UC-VE-002「上表」、UC-VE-008「上段」「上列」都是同文件内的指称，不属跨文件引用 |
| VE 代码注释里的行号引用 | 未发现 |
| ADR-0065 → CONTEXT（来源事实替代关系、只增不改写、分叉、可按版本读回） | CONTEXT 已收进；投影库按迁移 0016 版本只增、当前标记显式（被替代条目见 VE-03，分叉见 VE-04） |
| ADR-0197 决定二类型词表 vs `externalResultKind` | 逐词一致；参考映射只把 `external-result-release-full` 映成 `CUSTOMS_CLEARED`；路由只投 VE、不 FanOut |
| ADR-0205 文末补记：类型词、参考映射、路由 | `movement-departure/-in-transit/-arrival` 一致；`DEPARTED` 一致；`DEPARTURE_GATE_BLOCKED` / `SYN-VE-DEPARTURE-GATE-V1` / `SOURCE_CONFIRMED` 与种子一致；移动事实一路只投 VE |
| ADR-0224 两路补派生 | `routes_movement_fact_backfill.go` 名次为 `positionProjection + 10`，排在投影之后、主业务之前；`AT-VE-178..182` 与 `AT-TF-157/159..161/273/274` 结果一致 |
| ADR-0225 决定七与 `AT-VE-183` | 决定七存在；测试 `TestABlockedBagSignalsTheBlockedMemberNotTheBag` 钉住 |
| ADR-0205 ← ADR-0225 部分停用 | 被停两句在 ADR-0205 正文逐字存在；两头 Status 行与 Links 都有指针 |
| ADR-0197 ← ADR-0226 | 所引「CC 的层内一致性核对把同层、同版本、同范围下原始语义不同的两份一律判冲突」在 ADR-0197 越权风险点 5 里逐字存在；Links 双向 |
| ADR-0205 ← ADR-0219 / ADR-0224 | Status 行所引越权风险点 1、14 的原句存在；Links 双向 |
| ADR-0104 ← ADR-0176 | 所引原句存在；两头指针都在（写两遍的问题见 VE-22） |
| ADR-0136 → CONTEXT「首次索赔期限与最低材料两维……按目标包裹所属委托被接受时固定的商业依据选用」 | CONTEXT 已收进；`adapters/partycommercial/claim_service_rules.go` 按 PS 回指转 PC 闭包，不取系统时间 |
| ADR-0051 等待补充 | `EligibilityScreen` 三态（`ELIGIBLE` / `INELIGIBLE` / `AWAITING_SUPPLEMENT`）与 CONTEXT 资格审核三岔一致 |
| 分诊走向、案件主状态 | `ATTACH_TO_EXISTING` / `AUTO_ESTABLISH` / `MANUAL_REVIEW` / `NO_CASE` 对应 CONTEXT 四走向；`AWAITING_RESPONSE` / `IN_PROGRESS` / `CLOSED` 对应「待响应 → 处理中 → 已关闭」；归并以 `MERGED` 结论关闭，不是第四主状态 |
| 披露结论、通知节点 | `DISCLOSE` / `NOT_YET_DISCLOSABLE` / `AWAITING_AUTHORIZATION` 对应「披露 / 暂不披露 / 待授权」；通知六节点与 CONTEXT「已生成、已提交消息渠道、渠道已接受、已送达、失败和客户确认」一致（草稿问题见 VE-12） |
| 处置请求判断与取消答复 | 接受、部分接受、拒绝、要求补充；取消已接受、部分取消、已无法取消、拒绝取消，与 CONTEXT 一致；受理有效期届满有 `ErrRequestWindowExpired` |
| 追偿动作节点 | 准备完成、对外提交、渠道接受、送达、对方确认、提交失败、送达失败，七格与 CONTEXT 一致（动作种类见 VE-17） |
| 客户追踪视图查阅口错误码 | `ACCESS_CHANNEL_NOT_CONFIGURED` / `CUSTOMER_CLIENT_CREDENTIAL_REJECTED` / `CUSTOMER_CLIENT_NOT_GRANTED` / `IDENTITY_DEPENDENCY_UNAVAILABLE` 与 ADR-0139 决定四一致；`AT-VE-171..173` 有测试 |
| 运营追踪查阅读口 | 租户级、无客户维；`PROJECTION_NOT_FORMED` 如实作答，符合 ADR-0076 决定四与 CONTEXT「投影未形成……如实作答」 |
| 外部承运轨迹事实的三个时间 | 发生、有效、接收各归各位，有效时间待判断即拒，符合 ADR-0102；类型词与 GLOSSARY 的 `external-carrier-tracking` 一致 |
| `AT-VE-160` 终局维 | `FinalDimensionOver` 在投影无 PS 已采用终局时答待确认 |
| UC 所引参数号与验收矩阵号 | `PAR-VIS-01..09`、`PAR-INT-03/06/13`、`PAR-COM-06/07`、`PAR-NFR-04/06`、`PAR-SET-03`、`VIS-01..11`、`CUS-04` 等在登记册或矩阵里都能找到 |
| 代码注释里的「UC-VE-00x AT-VE-nnn」配对 | 除 VE-21 所列 `AT-VE-158` 一处外，与 UC 表全部对得上；代码引用的 `AT-VE` 号在 UC 里都存在 |
| `AT-VE` 号段空位 177、184、185 | 全仓（docs、internal、cmd、.scratch）无引用，不算问题 |
