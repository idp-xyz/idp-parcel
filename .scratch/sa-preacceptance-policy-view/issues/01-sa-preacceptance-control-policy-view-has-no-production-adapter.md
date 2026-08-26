# SA 接受前控制策略视图无生产适配器——提供方表面已具备，键形裁决已出

Category: enhancement
Status: in-progress（MCP-2 认领于 2026-08-26；开工前重核见下方「重核」节）

发现并定形于[第二十六轮重盘](../../mechanism-reinventory-r26/report.md)第四节（盘于
`e5301f8`）。判据 B 剩余 7 口中唯一「可做未做」的一口：其余六口各有留待依据，这一口的
提供方表面已经在了，缺的是一道键形裁决加一个消费适配器。裁决随本票记载（承用户 08-26
「作为业务和系统专家自决」委托），实现按本票另开工。

## 事实链（实读代码取证，锚 `e5301f8`）

1. **ADR-0054 的前提已过期**：它写作时（08-17）「`party-commercial` 侧连领域类型都还没有」。
   票 03（PC 声明发布写入方）之后，PC 现有 `domain/pre_acceptance_control.go`、
   `ports.PreAcceptanceControlDeclarationView`（found=false=实例未配置）与真库适配器
   `postgres.PreAcceptanceControlDeclarations`（`var _` 断言在位）。
2. **种子已发布真声明**：`scripts/demo-seeds` 商业发布批含财务控制策略
   `SYN-FIN-CONTROL-01`，客户合同正文绑定（预付适用）。适配器落地当天就有可验数据。
3. **键形错位**：SA `ports.PreAcceptanceControlPolicyView.LoadControlPolicy(tenant, scope)`
   按结算作用域（法人/账户/币种）问；PC 声明按（租户+客户合同版本）键入
   （`PAR-COM-15` 列在合同版本下，库上 `object_kind` CHECK 钉在 2）。签名装不下这次翻译。
4. **回指标识是已钦定的形状**：PS 侧快照刻意不持有合同版本（「不持有任何商业版本内容」，
   `CommercialBasisSnapshot`），但持有 `ResolutionID`；PC 侧解析闭包按标识落库
   （`CommercialResolutionStore.Save` 不覆盖）且开有只读口
   `CommercialResolutionView.LoadResolution(tenant, resolutionID) → (CommercialClosure, bool, error)`，
   注释明写「消费方只要回指标识（ADR-0027 / ADR-0062）」。
5. **答案的另一半已分好工**：ADR-0054 后果节钉住——`要求`格需要的结算方式与实际采用政策
   走 ADR-0044 结算政策解析，不由控制策略声明重造；已采用结算政策在同一份闭包里。

## 三案与裁决

- **甲：SA→PC 适配器自行反查（作用域→合同）**。否——反查目录是一份新的实例数据（无处取、
  等于发明账户映射的第二处定义）；经 PC 解析应用重解则需要商业查询入参（客户账户/服务
  产品），`SettlementScope` 装不下；SA 适配器也不得 import PS 的 `CommercialBasisResolver`
  （ADR-0025：适配器只许同时导入两个上下文）。
- **乙：消费方（PS 侧）拼好策略再喂给 SA**。否——「本上下文只消费它，绝不自行推导」约束的
  是 SA 与商业侧之间的问答；在 PS 拼答案等于把 SA↔PC 的缝搬进 PS，一决策两处定义。
- **丙（裁定采纳）：命令携带商业解析引用回显，SA→PC 适配器凭标识读 PC 落库闭包**。
  `ApplyPreAcceptanceControlCommand` 与 `LoadControlPolicy` 入参各扩一格解析引用
  （resolution ID 回显，来源是 PS→SA 适配器已有的幂等重解——`PolicyBackedControlScopeSource`
  同一来路，施加与释放两径天然同引用）；新包
  `internal/settlementaccounting/adapters/partycommercial/`（ADR-0054 预留的落点）实现
  SA 视图：凭（租户+解析标识）经 `CommercialResolutionView` 取闭包 → 取已采用客户合同版本
  → 经 `PreAcceptanceControlDeclarationView` 读「要不要」声明；`要求`格的方式与采用政策取自
  闭包内已采用结算政策（ADR-0044 分工）；声明未登记即 found=false（SA 编排既有
  `CONTROL_POLICY_NOT_CONFIGURED` 格接住）；闭包读不回/解析引用无效走 error 格。

## 实现范围（开工时对新 tip 重核）

- SA：`ports.PreAcceptanceControlPolicyView` 签名扩解析引用格；
  `ApplyPreAcceptanceControlCommand`（与释放命令如需）随之；编排把引用透传给视图；
  既有测试替身与用例跟随签名（跟随只改签名不改语义，同 ADR-0054 当年的跟随口径）。
- PS→SA 适配器：把解析引用装进命令（`ControlScopeSource` 已内含解析，取同一来路——加宽
  该接口返回或并列一个引用源，实现时定，两径必须同引用）。
- 新增 SA→PC 适配器包与真库测试：种子形状下 `SYN-FIN-CONTROL-01` 可读回、未登记合同答
  found=false、坏引用走 error；全仓真库套件绿。
- 这道签名与回显语义的裁决随实现落一份 ADR（引用本票与 r26 第四节，编号取当时下一号）。
- 占号核对：不碰 `cmd/parcel-dispatch/assemble.go`；`cmd/parcel-api` 的接受链装配若因命令
  形状变动需跟随，属跟随不属占号（无端点增删）。

## 完成标准

- 判据 B 复点该口从缺转有（r26 工具重跑，`.scratch/mechanism-reinventory-r26/tool/`）；
- SA 编排在种子租户下走通「要求-预付」分支到冻结（作用域与金额两缝仍显式未配置即停在
  `CONTROL_SCOPE_NOT_CONFIGURED`——那不属本票，不得为验它而造账户映射）；
- 未登记路径停 `CONTROL_POLICY_NOT_CONFIGURED`，与调不通格分开（ADR-0054/0029 维持）。

## 重核（2026-08-26 · MCP-2 认领，锚 `a7aa75b`）

票面「实现范围」首条要求开工时对新 tip 重核，照做，四条事实全部仍成立：

1. `ports.PreAcceptanceControlPolicyView.LoadControlPolicy` 仍是 `(ctx, tenant, scope)` 三参，键形错位未变。
2. `internal/settlementaccounting/adapters/` 下**只有** `postgres/` 一个子包，票面预留的 `partycommercial/` 落点仍空。
3. PC 侧提供方表面在位：`domain/pre_acceptance_control.go`、`ports.PreAcceptanceControlDeclarationView`、`postgres.PreAcceptanceControlDeclarations`。
4. 种子仍含 `SYN-FIN-CONTROL-01`，且这一轮接线后它在管理台 `party-contracts` 页与 `commercial-policies` 的接受前财务控制页签上都已可见（票 01 / 07 的 DOM 取证），落地当天即有可验数据这一条比写票时更硬。

**地盘核对（`docs/agents/parallel-sessions.md`）**：本票动 `internal/settlementaccounting/**`、新增 `internal/settlementaccounting/adapters/partycommercial/`、以及 PS→SA 适配器；MCP-1 同期在 `internal/visibilityexception/**` 与 `internal/customscompliance/**`（前沿票 02/05/06），**无重叠**。唯一可能相碰的是 `cmd/parcel-api` 的接受链装配跟随，与 MCP-1 那两票的端点增删不在同一 hunk 区，且本票不增删端点。
