# 第二十六轮机制半边重盘（端口两口径复点 + 八切片三判据 + 全绿证据）

Category: chore
Status: resolved

盘于 `e5301f8`（= 开盘时 origin/main tip，工作树除本报告族外干净）。承
[开发计划](../development-plan-2026-08-20.md) T4/M4：①按 r25 口径两口径重算端口；②八切片
逐三判据重核；③全绿（含 `-race` 含真库）证据。上一轮端口盘点为
[r25](../port-inventory-r25/report.md)（superseded，方法论沿用），上一次八切片重盘为基线
「机制半边现状」节的第二十四轮（盘于 `4c928b0`）。

**结论一句话**：四个切片首次达标（PN-01/04/05/08）；其余四个的差量收敛为**三张本轮新开的
可派票 + 四口登记面形状留待实例证据的视图 + 一片按裁定后置的子域 + 一口真实外部渠道**；
全量真库 `-race` 套件零失败零竞态。就绪候选材料齐备但按判据尚不可宣布，差量见第六节。

## 一、工具与其校验（本轮数字为何可信）

程序源码存档于 [tool/](./tool/main.go)，原始输出在 [raw/](./raw/)。两口径：

- **判据 A（基线口径，与 r24/r25 可比）**：`internal/*/ports` 包声明的接口，其名字在
  `internal/**` 路径含 `/adapters/` 或 `/platform/` 的非测试 `.go` 文件正文里**整词**出现过，
  即记「有生产实现」。r25 已证它虚高（被适配器当依赖引用也算）。
- **判据 B（精确口径，r25 第六节留的活，本轮补齐）**：`go/types.Implements`——
  `internal/...` 与 `cmd/...` 的生产包（`Tests=false`，测试替身不入内）里存在具体命名类型
  `T` 或 `*T` 完整实现该接口，才记「有生产实现」。r25 曾用「方法集名字覆盖」近似并自证会
  撞名误判（`CustomsCaseStore` 被判给 `postgres.DeclarationSubmissions`），本轮为真实现判定，
  该类误判在类型层不可能发生。

**校验**：同一程序对 r25 的发布树 `d40b03a` 复算（[raw/r25-recheck-d40b03a.txt](./raw/r25-recheck-d40b03a.txt)）
——接口总数 **200**、判据 A 缺 **13**、扫描适配器生产文件 **166**、逐上下文分布
（CC 0、NR 3、NO 1、PP 0、PS 4、PC 0、PG 0、SA 4、TF 0、VE 1）与 r25 第一节表格**逐格一致**。
工具与 r25 头条的可比性经复现证实。r25 遗留的「r24 基线记 65 与其重算 62 差 3」维持未还原，
绝对数与更早记载的可比仍以差式为准（同 r25 的口径声明）。

## 二、端口两口径复点（事实）

盘于 `e5301f8`，原始输出 [raw/r26-head-e5301f8.txt](./raw/r26-head-e5301f8.txt)。
扫描适配器/平台生产文件 269（r25：166）。

| 上下文 | 接口总数 | 判据 A 缺 | 判据 B 缺 |
|---|---:|---:|---:|
| customscompliance | 38 | 0 | 0 |
| networkrouting | 17 | 0 | 0 |
| nodeoperations | 12 | 1 | 0 |
| parcelpricing | 7 | 0 | 0 |
| parcelshipment | 38 | 2 | 2 |
| partycommercial | 17 | 0 | 0 |
| pilotgovernance | 9 | 0 | 0 |
| settlementaccounting | 36 | 4 | 4 |
| transportfulfillment | 24 | 0 | 0 |
| visibilityexception | 37 | 2 | 1 |
| **合计** | **235** | **9** | **7** |

**差式 vs r25（`d40b03a`）**：总数 200→235（+35 全为新声明，随票 03/06/09/15、目录读口、
运营读端点与隔离读准入等批次进来）；判据 A 缺 13→9——关 5 开 1：NR 三口
（`NetworkEvidenceView`/`InitialRouteEvidenceView`/`AutoRerouteFactsView`）随目录机制与受控
改路批次关闭，PS 两口（`ProductionOwnershipAuthority`/`WithdrawalAuthorizer`）随生产归属桥
与撤回授权接线关闭（各票 resolved 记录可回溯）；新开的一口是 VE `ClaimEvidenceView`
（新声明端口，见下）。

### 判据 A 的两处系统性偏差（本轮取证）

- **虚高样本已消化**：r25 第四节点名的 `Clock`×10 与 PC 两声明口
  （`AsOfPolicyDeclaration`/`AcceptanceContentDeclaration`），本轮判据 B 全部已有真生产实现
  ——系统时钟落在五个 `cmd` 组合根（parcel-api/dispatch/commercial/governance-register/
  ve-register 各一个 `systemClock`），两声明口随票 03 补上真库读口。样本清零不等于虚高机制
  消失，B 口径继续作精确对照。
- **虚低（本轮新发现）**：判据 A 只扫 `internal/**`，看不见 `cmd/` 侧实现。两例：
  NO `ParcelIdentityView` 与 VE `ClaimEvidenceView` 由 `cmd/parcel-api` 的显式未配置桩实现
  （`assemble_reception.go` 的 `unconfiguredParcelIdentityView`、`assemble_claims.go` 的
  `unconfiguredClaimEvidence`，ADR-0063 形状）——A 记缺、B 记有。桩的语义是「缝已接、答案
  是未配置」，不是假实现；两口各自的实底见第三、五节。

## 三、判据 B 剩余 7 口的分类（分类是判断，依据逐条给出）

| 端口 | 卡点分类 | 依据（端口注释原话/勘察） |
|---|---|---|
| SA `PreAcceptanceControlPolicyView` | **可做未做**（本轮唯一） | 提供方表面已具备而消费适配器缺一道键形裁决——裁决已出，详见第四节与[新票](../sa-preacceptance-policy-view/issues/01-sa-preacceptance-control-policy-view-has-no-production-adapter.md) |
| SA `ContractResponsibilityView` | 登记面形状留待实例证据 | 「合同责任目录未配置——回收需要合同依据（实例半边），未配置停在未决，不默认可回收」 |
| SA `ClaimAmountRuleView` | 登记面形状留待实例证据 | 「没有规则版本不形成金额（实例半边，AT-SA-147）」 |
| SA `SupplierAuditAuthorityView` | 登记面形状留待实例证据 | 「授权未配置——实例半边未提供时审核停在未决，不默认放行也不虚构授权人」 |
| PS `SourceDataRuleDeclaration` | 登记面形状留待实例证据 | 「由 `PAR-COM-13` 与真实合同、产品、线路和关务规则登记，属实例半边；本上下文只消费登记结论」 |
| PS `SourceDataAmendmentAuthorizer` | 建模未决 | 「真实请求方、实际决定方与授权入口仍是 `BD-PS-009` 待确认的实例参数」 |
| VE `NotificationChannelGateway` | 实例半边（真实外部渠道） | 「真实渠道与其凭证属实例参数，今天没有实现，唯一实现是测试替身」 |

「登记面形状留待实例证据」四口要说清口径张力：基线「横切缺口」节写过「视图的**内容**属实例
半边，视图的**实现**不是」——即实现计机制欠账；而四口的端口注释（权威声明处）都把未配置态
判给实例半边。本轮**维持 r25 的分类不翻案**：这四口的登记面（表形 + 写入口）该不该在无租户
时预造，属 ADR-0072「登记册形状等真实证据」同款的取舍；预造登记面就是在没有事实的地方立
形状（frontline-client-shape 裁定的同一判据）。收口路径两条，记录在案：随首个租户登记面
成形时实现，或未来裁断轮翻案预造——两条都不是本轮盘点的事。四口的消费侧全部以显式未配置/
未决格在场（骨架不作假）。

**7 口之外、端口计数看不见的机制余量两条**（藏在装配桩的成因里，本轮从
[接线票 04](../parcel-api-remaining-endpoint-wiring/issues/04-ve-claims-wiring.md) 的 Comments
里提出来立票）：

1. VE `EligibilityRuleView` 判据 A/B 都记「已实现」（真适配器 `ClaimEligibilityRules` 在），
   但它把租户钉在装配期（为受控登记口而设），多租户入口接不上，只能以报错桩顶位——缺的是
   **带租户维的读法**，属机制半边。→ [新票 ve-claims-read-seams/01](../ve-claims-read-seams/issues/01-eligibility-rule-view-needs-a-tenant-dimensioned-read.md)。
2. VE `ClaimEvidenceView` 的实底是**材料归集面机制未建**（桩按端口自设 known=false 格如实答
   「归集无从查起」）。→ [新票 ve-claims-read-seams/02](../ve-claims-read-seams/issues/02-claim-evidence-collection-surface-is-unbuilt.md)。

## 四、SA `PreAcceptanceControlPolicyView`：提供方表面已具备，缺的是一道键形裁决

ADR-0054（08-17）写作时的前提「`party-commercial` 侧连领域类型都还没有」已被票 03 推翻，
事实链五条（均为本轮实读代码取证）：

1. PC 现有 `domain/pre_acceptance_control.go`、`ports.PreAcceptanceControlDeclarationView`
   与真库适配器 `postgres.PreAcceptanceControlDeclarations`（`var _` 断言在位，
   found=false=实例未配置）。
2. 种子已发布真声明：`scripts/demo-seeds` 商业发布批含财务控制策略 `SYN-FIN-CONTROL-01`，
   客户合同正文绑定（预付适用）。
3. **键形错位**：SA 端口按（租户 + 结算作用域=法人/账户/币种）问，PC 声明按（租户 + 客户
   合同版本）键入，签名装不下这次翻译。
4. 调用侧本就持有答案的来路：PS→SA 适配器的 `PolicyBackedControlScopeSource` 经
   `CommercialBasisResolver` 幂等重解，作用域正是从解析回显派生的；PS 侧快照**刻意不持有**
   合同版本（「不持有任何商业版本内容」），但持有 `ResolutionID`，而 PC 侧解析闭包已按标识
   落库且开有只读口 `CommercialResolutionView.LoadResolution`——「消费方只回指标识」正是
   ADR-0027/0062 钦定的形状。
5. ADR-0054 后果节已钉分工：`要求`格的方式与采用政策走 ADR-0044 结算政策解析，不由声明重造。

**裁决**（承用户 08-26「作为业务和系统专家自决」委托）：SA 控制命令与视图入参扩一格
**商业解析引用回显**；SA→PC 消费适配器（`internal/settlementaccounting/adapters/partycommercial/`，
ADR-0054 预留的落点）凭解析标识读 PC 落库闭包取已采用合同版本，再按合同版本读控制声明；
方式与采用政策取自闭包内已采用结算政策。三案对比与实现范围录
[票面](../sa-preacceptance-policy-view/issues/01-sa-preacceptance-control-policy-view-has-no-production-adapter.md)
（ready-for-agent）。本轮不实现：盘点轮不动生产代码，签名改动横跨 SA/PS 两包按票另开工。

**追补（2026-08-26 · MCP-2，本节盘点结论不改，只记后续）**：该票已 resolved，裁决落
[ADR-0079](../../docs/adr/0079-pre-acceptance-control-policy-view-asks-by-commercial-resolution-reference.md)。
同一工具在 `b69bdaa` 重跑（[raw/r26-recheck-b69bdaa.txt](./raw/r26-recheck-b69bdaa.txt)，
`tool/go.sum` 本轮补齐——原先缺它工具跑不起来）：**判据 B 缺 7 → 6**，本口从 A 全局、
A 同上下文、B 三份名单里同时消失，第三节表格首行的「可做未做（本轮唯一）」一格因此清空。
同次比对里 VE `ClaimEvidenceView` 也从两份 A 名单消失、VE 接口数 37 → 39，那是 MCP-1
同期前沿票的产物，与本节无关。实现期另撞出一处持久化缺陷（采用结算政策的解析闭包写得进
读不回），同笔修复，记在票面「实现期发现」节——它是**端口计数看不见的又一条机制余量**，
与第三节末尾「7 口之外、端口计数看不见的机制余量」那一组同型：判据 A/B 都记「已实现」，
坏的是那个实现读不回自己写的东西。

## 五、八切片逐三判据（判断）

三判据的全局证据底座，先摆事实：

- **受控案例可复算**：全量真库 `-race` 套件退出码 **0**——WSL go1.26.5 + cgo（Windows 侧无
  gcc 跑不了 `-race`，复现口径记档：走 WSL），DSN 指 `postgres:16.14` 门禁容器
  （127.0.0.1:55432），**79 包全 ok、零 FAIL、零 DATA RACE**，2026-08-26 13:30:46→13:39:25，
  日志存档 [raw/race-run-2026-08-26.log](./raw/race-run-2026-08-26.log)。测试面 538 份测试
  文件对 529 份生产文件（internal+cmd）：PS 88、VE 73、CC 58、PC 54、NR 44、PP 42、TF 41、
  SA 40、NO 20、PG 14，另 platform 11、architecture 9。
- **骨架完整**：生产代码（internal+cmd 非测试）`TODO|FIXME|not implemented|panic(` 命中
  **0 行**（本轮重扫）。应用编排生产文件 59→**69**（CC 12、PS 12、SA 9、TF 8、VE 8、NR 6、
  PC 5、NO 3、PG 3、PP 3），增量对应其后收口的票（CC 更正/补充与版本维、NR 目录机制、
  PC 发布写入方、PP 登记口、PG 治理登记等）。真库适配器 132→**160** 个生产文件
  （SA 29、CC 27、VE 22、TF 21、PS 16、PC 16、NR 12、NO 7、PP 5、PG 5）。
- **参数显式未配置**：维持——本轮所有新桩（第二节虚低两例）与隔离读准入（ADR-0078 的
  `SYN-` 前缀门禁）均为显式未配置/合成隔离形状；无业务阈值常量入生产代码（基线判据沿用，
  依赖面未变：chi/pgx/bento 三个直接依赖）。

逐切片定级（状态三种沿基线：达标/部分/未开始）：

| 切片 | 定级 | 差量（只列机制半边） |
|---|---|---|
| PN-01 治理记录 | **达标** | 无——PG 9 口 0 缺，编排/测试/真库全绿；真实 `PAR-GOV-*` 参数与阶段决定属实例半边不计 |
| PN-02 商业解析与接受 | 部分 | SA 控制策略视图票（第四节，可做未做）；`SourceDataRuleDeclaration`/`SourceDataAmendmentAuthorizer` 各留待 `PAR-COM-13` 登记与 `BD-PS-009` 建模（有记录的留待，不阻骨架诚实）；作用域账户目录与控制金额两缝实例半边（既有记录） |
| PN-03 网络收寄与节点作业 | 部分 | 外部标识关系子域在 PS 零代码（CONTEXT 判归 PS 所有，[ps-external-mark-relations/01](../ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md) needs-info 记裁：不由消费侧端口倒逼开子域，等排期）；NO `ParcelIdentityView` 以显式未配置桩在场 |
| PN-04 运输履约与终局 | **达标** | 无——TF 24 口 0 缺，编排八例、真库 21 适配器全绿 |
| PN-05 关务 | **达标** | 无——CC 38 口 0 缺（r25 时已清零，本轮维持），编排 12 份 |
| PN-06 可见性与异常 | 部分 | VE 两票（资格规则带租户维读法、材料归集面，第三节）；`NotificationChannelGateway` 等真实渠道凭证（实例半边） |
| PN-07 计价与结算 | 部分 | SA 三口登记面留待实例证据（第三节口径张力段）；PP 7 口 0 缺 |
| PN-08 治理编排 | **达标** | 同 PN-01；B-06 Bento 双消费者绑定属跨仓闸门动作，在 T4 之外另行报批（08-25 已记） |

## 六、就绪候选结论（交用户，宣布是用户的决定）

产品就绪的判据是基线那三条对八切片全部成立。本轮之后的诚实读数：

- **可复算**与**显式未配置**两条判据：全库满足，且有全绿+零竞态+真库实跑的复算证据；
  S 级演示动线已有权威脚本且页面层取证到位
  （[synthetic-demo-journey-script](../../docs/design/synthetic-demo-journey-script.md)）——
  「可以演示」这半句今天就成立。
- **骨架完整**：四切片达标；差量全部点名且三张可派票已开（本轮新开：
  [sa-preacceptance-policy-view/01](../sa-preacceptance-policy-view/issues/01-sa-preacceptance-control-policy-view-has-no-production-adapter.md)、
  [ve-claims-read-seams/01](../ve-claims-read-seams/issues/01-eligibility-rule-view-needs-a-tenant-dimensioned-read.md)、
  [02](../ve-claims-read-seams/issues/02-claim-evidence-collection-surface-is-unbuilt.md)），
  其余差量各有记录在案的留待依据（实例证据/建模/子域排期），不构成假骨架。

**建议路径**：先收三张可派票（互不撞文件、不占 `assemble.go`/`endpoints.go` 号，可并行），
随后作第二十七轮盘点戳；届时若四口留待与外部标识子域的处置经用户认可维持留待，
八切片即可按「达标或有裁定的显式留待」交宣布。按 T4 第 3 条，宣布本身是用户的决定。

## 七、本轮没做的事

- 没实现任何端口、没动任何生产代码（盘点轮纪律；三张新票 ready-for-agent 待派）。
- 没把四口「登记面留待实例证据」翻案成预造（判据与 ADR-0072/frontline 先例一致，翻案留给
  裁断轮）。
- 没更新棘轮普查 [census-d5e5d20](../production-wiring-ratchet-gate/census-d5e5d20.md)
  （T2 量尺，口径不同属，留给下一次接线批）。
- 没有还原 r25 遗留的「65 对 62 差 3」（对头条无影响，差式可比性已由工具复现校验背书）。
