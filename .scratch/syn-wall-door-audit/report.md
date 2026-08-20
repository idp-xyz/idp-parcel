# 墙-门对照清单:合成租户 S 级走通审计

基线:`49a2ab0`(origin/main tip,worktree `syn-wall-door-audit`)
证据等级:S(隔离纸面走查——静态审计符号、迁移与装配点;未运行进程,未触碰共享树)
票源:MCP-1 派票 SYN-WALL-DOOR-AUDIT(2026-08-20 11:12,重派 12:15)

> **采纳注记(2026-08-20 MCP-1,自封存分支 `a958029` 采纳,用户批复「按建议办」)**:
> 本报告与十张票录于 `49a2ab0`,作者会话死后经封存分支抢救入主线;采纳时**未逐票对当前
> main 复核**。任何一票开工前,先按票内符号与迁移对当时的 main tip 重核四件(仓储/装载口/
> 写入方/登记口)——死会话票面状态可能冻在过时时刻,今日已有两例教训。

## 问题与判据

各「诚实实例墙」的 `*_NOT_CONFIGURED` 语义对读者暗示「可配而未配」,但有些墙今天想配也没处配——机制的门本身没建。本清单假想明天签下一个租户,按业务链逐墙走,对每堵墙核实四件:

1. **配置仓储**——存这份实例配置的表/迁移在不在;
2. **装载口**——判断代码读它的读口(SELECT 适配器)在不在;
3. **写入方**——非测试代码里有没有 INSERT 这张表的仓储方法;
4. **登记口**——进程外有没有受控路径(端点/用例接线/env)能把配置放进去。

判定三级:

- **有门**:四件齐且接线,租户上线流程今天就能配;
- **半门**:仓储+装载口在,写入方或登记口缺——形状已定,缺注入路径;
- **无门**:仓储或装载口本身不存在,或墙后还缺判断机制(如解析层)——想配也没处配。

按 ADR-0017 的口径:**墙(哨兵)本身全部正确**——实例留空并拒绝默认值是红线要求,不是缺陷;本清单登记的缺件全部属机制半边(门),不是催实例值。

## 总表

| # | 链位 | 墙(哨兵/错误码) | 哨兵所在符号 | 仓储 | 装载口 | 写入方 | 登记口 | 判定 | 票 |
|---|---|---|---|---|---|---|---|---|---|
| W01 | 接入认证 | 无独立哨兵,并入 W02(未配置即拒先于身份) | `UnconfiguredIntake` 各上下文 `adapters/http` | ✗ | ✗ | ✗ | ✗ | 无门 | 01 |
| W02 | 接入渠道 | `ACCESS_CHANNEL_NOT_CONFIGURED`(403) | `codeAccessChannelNotConfigured` / `ErrAccessChannelNotConfigured`,PS/NO/TF/VE/CC 五处 `adapters/http/unconfigured_intake.go`;装配点 `assembleBusinessEndpoints`(`cmd/parcel-api/endpoints.go`) | ✗ | ✗ | ✗ | ✗ | 无门 | 01 |
| W03 | 建单·生产归属 | `OWNERSHIP_UNRESOLVED` / `ADMISSION_PAUSED` / `AUTHORITY_UNRESOLVED` | `SubmitOutcome`(`parcelshipment/application/submit_shipment_request.go`)、`parcelshipment/domain/production_ownership.go` | ✓(pilot_governance) | ✗(PS 侧) | ✓(`governance_records.go`) | 用例有未接线 | 无门(桥缺) | 02 |
| W04 | 接受·商业解析键与锚点 | `COMMERCIAL_RESOLUTION_KEY_NOT_CONFIGURED`(消费方键)、`ANCHOR_POLICY_NOT_CONFIGURED`(提供方锚点策略) | `parcelshipment/adapters/partycommercial/commercial_basis.go`;`partycommercial/domain/commercial_resolution.go` | ✓ | ✓ | 解析✓ 发布✗ | ✗ | 半门 | 03 |
| W05 | 接受·接单内容与 asOf 策略 | `PC-ACCEPTANCE_CONTENT_NOT_CONFIGURED`、`NOT_CONFIGURED`(JudgmentAsOf)、`REACHABILITY_AS_OF_NOT_CONFIGURED`、`FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED` | 同上适配器;`parcelshipment/ports.JudgmentAsOfOutcome`;`parcelshipment/application/judgment_continuation.go` | ✓(0005/0006) | ✓ | ✗ | ✗ | 半门 | 03 |
| W06 | 接受·决定授权规则 | `REJECTION/WITHDRAWAL/SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED`、`RULES_NOT_CONFIGURED` | `judgment_continuation.go`;`parcelshipment/ports.AuthorizationOutcome` | ✓(0003) | ✓ | ✓(`authorization_grant.go`) | ✗ | 半门 | 03 |
| W07 | 接受·前置财务控制 | `CONTROL_POLICY_NOT_CONFIGURED`、`CONTROL_SCOPE_NOT_CONFIGURED`、`CONTROL_AMOUNT_NOT_CONFIGURED` | `settlementaccounting/application/apply_pre_acceptance_control.go`;`parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go` | ✓(0007) | ✓ | ✗ | ✗ | 半门+两缝未接 | 03、07 |
| W08 | 接受·可达性目的 | `REACHABILITY_PURPOSE_NOT_CONFIGURED` | `parcelshipment/adapters/networkrouting/reachability.go`(装配期 `ServicePurpose` 零值) | ✓(0008 service_product_form) | ✓ | ✓ | ✗(接受链未装配) | 半门 | 03 |
| W09 | 路由·网络定义登记册 | `NETWORK_EVIDENCE_NOT_CONFIGURED`、`ROUTE_EVIDENCE_NOT_CONFIGURED`、`REASSESS_EVIDENCE_NOT_CONFIGURED`;登记了也 `ErrNetworkDefinitionUnresolvable` | `networkrouting/application` 三用例;`networkrouting/adapters/postgres/network_definition.go` | ✓(0007) | ✓ | ✗(注释自证) | ✗ | 无门(写入方+解析层双缺) | 04 |
| W10 | 路由·自动改路事实目录 | 装配点 `AutoReroute: nil`(显式未配置) | `cmd/parcel-dispatch/assemble.go`(`networkIntakeConsumer`) | ✗ | ✗ | ✗ | ✗ | 无门 | 05 |
| W11 | 收寄/揽收·采用资格 | `ELIGIBILITY_UNDECIDED`;资格视图未配置;硬资格证据 `UnconfiguredIntakeQualificationEvidence` | `parcelshipment/application/adopt_network_intake.go`;`pspartycommercial.NewServiceStageRulesAdapter` 装配(`assemble.go`) | ✓(0013) | ✓ | ✗ | ✗ | 半门(声明)+无门(证据口) | 03、10 |
| W12 | 交付→终局·终局规则 | `FINAL_UNDECIDED`;PAR-COM-17 终局声明无行 | `parcelshipment/application/form_parcel_final.go`;装配注释(`adoptEffectiveDeliveryConsumer`) | ✓(0013) | ✓ | ✗ | ✗ | 半门 | 03 |
| W13 | 关务·案件配置面 | `DECLARATION_UNDECIDED`、`RESULT_UNDECIDED`(配置视图空册) | `customscompliance/application/submit_declaration.go`、`receive_external_result.go`;五只读视图 `readiness_view / submission_authority_view / interpretation_rule_view / obligation_inventory_view / gate_condition_view` | ✓(0006/0007/0008) | ✓ | ✗ | ✗ | 半门 | 06 |
| W14 | 计价·价卡 | 评价`不可计价`/`待判断`(无适用价卡) | `parcelpricing/domain`(`plan.go`/`rate_table.go` 纯内存);`ports` 仅 `EvaluationStore`/`EvaluationHandoff` | ✗ | ✗ | ✗ | ✗ | 无门 | 07 |
| W15 | 计价·参考序列 | `REFERENCE_SERIES_UNRESOLVED`、`EXCHANGE_RATE_UNRESOLVED` | `parcelpricing/domain/evaluation.go`(序列为评价入参,无登记册) | ✗ | ✗ | ✗ | ✗ | 无门 | 08 |
| W16 | 投影·里程碑映射 | `MAPPING_NOT_CONFIGURED`(投影落未归类) | `visibilityexception/application/derive_projection.go`;装载口 `NewMilestoneMappings`(`rule_catalog.go`) | ✓(0009+0014,键已按裁定改类型维) | ✓ | ✗ | ✗ | 半门 | 09 |
| W17 | 客户视图·披露策略 | 空册→四维全部待确认 | `tenantBoundCustomerViewDerive`(`assemble.go`);`NewDisclosurePolicies`(`disclosure_policy.go`) | ✓(0012) | ✓ | ✗ | ✗ | 半门 | 09 |
| W18 | 异常·分诊/通知/索赔资格 | `TRIAGE_RULES_NOT_CONFIGURED`、`NOTIFICATION_POLICY_NOT_CONFIGURED`、`ELIGIBILITY_CATALOGUE_NOT_CONFIGURED` | `raise_signal.go` / `notify_customer.go` / `handle_claim.go`;装载口 `NewTriageRules` / `NewNotificationPolicies` / `NewClaimEligibilityRules` | ✓(0009/0010/0011) | ✓ | ✗ | ✗ | 半门 | 09 |

## 有门对照组(证明判据能分辨)

| 项 | 门 | 说明 |
|---|---|---|
| 调度器部署形态 | env 必填 | `IDP_PARCEL_POSTGRES_DSN`、`IDP_PARCEL_ROUTE_SERVICE_PURPOSE`、投递超时/批量/租约五项,`settingsFromEnv` 拒空不给默认——服务目的这个实例参数在调度器一侧**有门**(env),在 PS 接受链一侧无(W08) |
| 治理登记 | 应用用例 | `pilotgovernance/application` 的 `record_stage_review`(候选版本集/阶段评审/权威区间)与 `govern_incident`(暂停/恢复/接管)+ `governance_records.go`/`incident_records.go` 写入方齐;缺的只是进程级入口与 PS 桥(W03,票 02) |
| 运行时业务事实 | 事件驱动写入方 | 全链业务事实(来源保全、收寄、揽收、交付、判断、评价、投影、视图)各自有 INSERT 写入方并已接调度器——墙不挡事实,只挡实例配置 |

## 逐墙细节

### W01/W02 接入认证与接入渠道(无门)

八个业务端点(`/shipment-requests`、`…/withdrawals`、`/node-operations/receptions`、`/transport-fulfillment/deliveries`、`…/delivery-proof-corrections`、`/customer-tracking-view`、`/claims`、`/customs/external-results`)全部装着 `UnconfiguredIntake{}` + `unwired*` 编排桩。按 ADR-0055,「空登记册就是装配点本身」:今天没有渠道登记册这张表,没有任何一行真渠道 Intake,配置一个渠道的唯一方式是改 `assembleBusinessEndpoints` 的代码。认证同理——路由层只有 RequestID/Recoverer 中间件,凭据验证与来源信封铸造按红线归真渠道 Intake(ADR-0003 禁采信自报身份),该机制件整体缺席。PAR-INT-01 的实例值今天**没有落点**。→ 票 01。

### W03 生产归属(无门,桥缺)

`ports.ProductionOwnershipAuthority` 在全库只有接口定义,无任何生产适配器。治理侧(pilot-governance)有 `authority_interval` 表、INSERT 写入方与 `record_stage_review` 登记用例(PAR-GOV-03 的机制半边基本齐),但:①无进程入口接治理用例;②无 PS→pilot-governance 的桥接适配器把权威区间译成归属决定。合成租户即便种好治理记录,提交链在 `DecideProductionOwnership` 一步仍无实现可调。→ 票 02。

### W04–W08 接受链五墙(半门,同根)

商业解析、接单内容、asOf 策略、授权规则、前置财务控制声明、服务产品形态,六类配置的表(party_commercial 0002–0014)与只读装载口全部就位,`resolve_commercial_basis` 解析用例也真——但**发布侧没有用例**:六张声明表(`as_of_policy_declaration`、`acceptance_content_declaration`、`pre_acceptance_control_declaration`、`customer_contract_content`、`stage_content_declaration`、`acceptance_rule_package`)在非测试代码里零 INSERT;`commercial_version`/`commercial_resolution`/`authorization_grant`/`service_product_form`/`price_policy`/`settlement_policy` 有仓储级写入方但无登记用例、无进程入口。当前唯一可执行的填充路径是测试内隔离种子(`cmd/parcel-dispatch/syn_pc_seed_test.go`,SYN-RES-01,S 级)。→ 票 03。

两条缝单列:控制金额应由估价形成(计价缝,依赖 W14/票 07);控制作用域应从已解析结算政策来(ADR-0044 缝,票 03 范围内注明)。

### W09 网络定义登记册(无门,双缺)

`network_routing.network_definition` 表(0007)与读口 `NewNetworkDefinitions` 在,但读口注释自证:「今天本表没有写入方,因此生产上恒答`未配置`」;且登记了也走 `ErrNetworkDefinitionUnresolvable`——解析层(候选生成/过滤/排序,PAR-NET-14 机制半边)不存在。缺两件:登记口+写入方;解析层。→ 票 04。

### W10 自动改路事实目录(无门)

`ReassessRouteDeps.AutoReroute` 在装配点显式给 nil:四条件事实目录无生产实现,改路评估整段不做,连「为什么没自动」都说不出。→ 票 05。

### W11/W12 采用资格与终局规则(半门+无门)

声明面同 W04–W08(stage_content 无写入方,票 03)。另一件独立缺件:硬资格证据口(ADR-0063)只有 `UnconfiguredIntakeQualificationEvidence` 这个「诚实无门」实现,资格证据(如清关预检)今天没有任何来源可登。→ 票 10。

### W13 关务配置面(半门)

就绪/提交授权/解释规则/关闭义务/门禁条件五类配置,表(0006/0007/0008)与五个 `*_view.go` 只读装载口齐,零写入方。PAR-CUS-01..07 的实例值没有登记路径。→ 票 06。

### W14/W15 计价双无门

`parcel-pricing` 的端口只有评价存取与交接;价卡(`PricingPlanVersion`/`RateTableVersion`)与参考序列(燃油/汇率,ADR-0013 归属已裁)是纯内存领域对象,**无表、无仓储、无装载口、无登记口**。三个价表族(`WEIGHT_ZONE`/`FIRST_CONTINUE`/`UNIT_PRICE`,PPC-2)机制已实现,但一份真价卡今天没有任何地方可放。`evaluate_pricing` 用例未接任何进程。这是全链「机制最齐、门最缺」的一段。→ 票 07、08。

### W16–W18 VE 四目录(半门)

里程碑映射(0009+0014)、分诊规则(0009)、通知策略(0010)、索赔资格(0011)、披露策略(0012)——装载口全部就位且按租户现绑(装配点注释明确「空册即如实说等」),写入方全部缺席。映射目录的键结构问题已按 2026-08-19 裁定改到类型维(0014),填得满了,但仍没有往里填的口。→ 票 09。

## 与在途票的关系

- 外部评审 01(FanOut 未决盖硬失败,MCP-5 在修)、02(启动无 DB 就绪检查)、04(claim_item 无修订 CAS):平台/健壮性缺陷,与本清单的「门」正交,不重复立票。
- 外部评审 03(账户迟绑无客户视图重派生):W17 相关的已立票,本清单引用不重复。
- `.scratch/ve-milestone-mapping-key` 01(映射键结构,MAP-KIND in-progress):其机制裁定已落 0014,本清单 W16 只补「写入方缺失」这一件。

## 链尾说明

票面链止于投影/客户视图。计价之后的结算链(费用确认条件、分摊规则等)未逐墙走;抽查见 `settlement_accounting.charge_confirmation_condition` 有仓储级写入方无登记用例,与 W04–W08 同模式,留待后续票面。

## 汇总

走墙 18 堵:**无门 6**(W01/W02 合一扇、W03、W09、W10、W14、W15,另 W11 证据口半边无门)、**半门 11**、有门 0(对照组之外主链上没有一堵墙今天能从进程外配置)。机制票底稿 10 张,见 `issues/`。
