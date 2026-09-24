# 12 customs-compliance：按路由候选作答的关务适用性判断口

Category: enhancement
Status: needs-triage——2026-09-24 通道 3 立（派单 `task-1f910231` ← 通道 1；出自 02 的取证与 [psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 格 6）。建在 [ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md)（Proposed）决定三上，判断口的形状与作答层级归 CC owner（0148 越权风险点 4）：0148 接受、CC owner 在分诊时定下文「待 CC owner 定」各问之后，转 ready-for-agent
Blocked by: 02（ADR-0148 接受之笔 02 才 resolved）；「接到 NR 取数侧」那一项另等 07
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「演示租户上一票已接受的委托能形成初始路由」那条关键路径上挡路的缝（切片计划的子票表由通道 5 补入）
归档：psb/04 与 ADR-0148 决定三都写这张票「另立、不在本票族里补」，指的是 NR 取数侧各票不代 CC 补执行器；本票就是那张另立的票，地盘在 CC。放在本目录、编号 12 是派单的归档选择（通道 1 与通道 5 协调）。
地盘：customs-compliance 的领域、应用与端口（新判断口）及其读侧适配器；customs-compliance `CONTEXT.md` 相关词条与规则（经 CC owner）；「接到 NR 取数侧」那一项另含 network-routing 的 `adapters/customscompliance` 消费方适配器。
出处：ADR-0148 决定一（端口取回的事实带出处、与判断一并留痕）、决定三与越权风险点 4；[CONTEXT-MAP](../../../docs/domain/CONTEXT-MAP.md)「customs-compliance ↔ network-routing」；customs-compliance [`CONTEXT.md`](../../../docs/domain/customs-compliance/CONTEXT.md)「Boundaries and ownership」「本上下文拥有合规候选区域、口岸、申报路径和关务适用性判断」；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二、三。

## 现状

CC 有口岸目录与申报路径目录两本登记册（`ports.PortsPathsRegistry` 写、`ports.PortsPathsView` 按时点点读），没有按路由候选作答的判断口。口岸目录只登口岸标识与生效区间，`CandidatePortEntry` 的注释写明「所属区域与适用性判断都不在册（区域维未建模，适用性是判断链的产物）」；申报路径是口岸、方向、申报模式三维。CC `CONTEXT.md` 里也还没有「关务适用性判断」这个词条。ADR-0148 决定三定：补上之前，NR 取数侧对候选的关务一格答状态未知，初始路由停在路由判断未决。CN→SG 的候选都含关务段，所以它挡在演示动线上。

## 产品与租户的分界（ADR-0146 决定二）

- **产品策略**：判断方法——候选要过关务边界时，对照该租户在判断时点在册的口岸与申报路径（以及裁定并入的限制）逐候选作答；判断口的形状——输入的候选关务投影、答案代数、出处字段；「不可用」与「状态未知」分得开，状态未知不折成可用。
- **租户取值**：该租户的口岸目录与申报路径目录内容（用哪些口岸、走哪种方向与申报模式）、内部合规限制实例、报关服务方的选择、采用哪一版参考配置。
- **参考配置**：若判断要用公开标准的实例数据（例如某国公开的口岸代码及其所属关务区域），按 ADR-0146 决定三出参考配置、显式采用才生效；采用路径按 ADR-0147（通道 4 在票 [psb/03](../../product-strategy-boundary/issues/03-reference-configuration-adoption-pattern.md) 中定，待重放进 main）。首版随附哪些，归 CC owner 与用户定。

## 待 CC owner 定（分诊时）

1. **作答层级**：答到区域、口岸还是申报路径。答到区域要先建模区域维——今天口岸目录没有它。
2. **候选关务投影由谁组装、含哪些格**：NR 按候选段链与目录折出（节点与口岸的对应落在 NR 目录还是 CC 目录），还是 CC 按 NR 交来的段链自行判断跨境点。这一问牵动 NR 目录内容列（07）与 CC 目录的边界，需与 NR owner 一起定。
3. **答案代数**：至少分得开可用、不可用（带理由）与状态未知；CONTEXT-MAP 那条边写关务提供「合规候选区域、口岸、申报路径、限制及解除结果」——内部合规限制的覆盖并入本判断，还是另答。
4. **留不留判断记录**：作答落成 CC 的版本化判断（同「合规判断」词条：带规则版本、依据、决定方式），还是只由读口即时作答、出处由 NR 随路由判断留痕。

## 做什么

1. customs-compliance `CONTEXT.md` 补「关务适用性判断」词条与规则，按上面各问的裁定写；先改 CONTEXT，再改引用它的用例。
2. 领域与应用：按裁定的判断方法逐候选作答。口岸未登记或判断时点未生效、申报路径三维对不上 → 不可用并说清是哪一格；目录为空或依赖读不到 → 状态未知，不从「查无记录」推出可用（与 CC Rules「不得按无记录……推导无内部限制」同一纪律）。
3. 端口：按裁定的形状出判断口；答案带出处（判断标识、所依目录版本与规则版本），供 NR 随路由判断留痕（ADR-0148 决定一）。同一租户、同一时点、同一候选投影，答案一致。
4. 接到 NR 取数侧：network-routing 的消费方适配器调用本判断口，把 07 里候选关务一格的状态未知换成真答；证据结构的出处字段照 07 已定的形状。
5. 若裁定随附参考配置：按 ADR-0147 的采用路径出一份样板，演示租户显式采用；不随附就不做这一项。

## 与 psb/10 的分工

[psb/10](../../product-strategy-boundary/issues/10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) 是 CC 的申报侧：申报发送通道、公开监管结果代码与法规税则等参考配置、法定义务目录、立案与提交的触发面。本票是 CC 对路由的答复口：按候选的关务适用性。两票不共用可执行项。相邻的只有一处：本票若随附公开口岸与关务区域的参考配置，与 psb/10「公开法规规则源、税则与税费版本」那一项的按地区参考配置走同一条采用路径，但各出各的配置，不合成一份。

## 不做

- 不定任何租户取值：口岸与申报路径目录内容、区域归属、限制实例、报关服务方；不预置默认口岸，目录为空也不答可用。
- 不做申报就绪判断（那是按申报范围的判断，CC 已有），也不做 psb/10 各项。
- 不改 NR 的候选生成、排序、冻结与改路（03–07、09、10）；改路改变关务区域、口岸或报关服务方时重新请求判断，由 NR 那几张票负责发起，本票只保证判断口不给「沿用上次答案」的捷径。
- 本票不附带 ADR；判断口形状若经 CC owner 判为难逆转取舍，由其另议。

## 完成判据

- [ ] CC `CONTEXT.md` 有「关务适用性判断」词条与规则，出自 CC owner 的分诊裁定。
- [ ] 真库用例：合成口岸与申报路径目录上，候选得出可用、不可用（口岸未登记或未生效、申报路径三维对不上；限制若并入再加一条）与状态未知（目录为空、依赖读不到）各一；状态未知不折成可用。
- [ ] 同输入重复作答结果一致，答案带出处。
- [ ] 接到 NR 取数侧后，合成网络上含关务段的候选不再停在状态未知；psb/05 格 6 的取证据此更新（证据只记 `S`）。
- [ ] 不进参数登记册；演示数据全为 `SYN-` 合成值。
