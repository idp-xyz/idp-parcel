# ADR-0101：运营操作者面的登记载荷形状由产品定义，属机制半边——「渠道原始载荷 → 登记快照」的翻译只在客户渠道上属渠道契约；价卡首例：产品发布版本化导入模板，导入形成持久化草稿，校验与发布共用一份规范化摘要，批准是独立的操作者动作，审批职责规则缺省朝拦

Status: Accepted（2026-09-03，用户经 IDP 队列通道 4 授权本会话「作为业务和系统专家自决，先按建议出 ADR」，与 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 同一次授权、同一批裁决。裁决能力边界：读过 [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 全文与票 [admin-write-faces/01](../../.scratch/admin-write-faces/issues/01-registry-configuration-has-no-admin-write-face.md) 文末「不逐字段建」那条评论、parcel-pricing `CONTEXT.md`「定价方案与价表版本」生命周期与「不采纳的工件形态」两节、参考蓝图 V1.2 第 38 节、`internal/parcelpricing/domain` 的 `PriceCardRegistration` 与方案快照、管理台价卡页与其 `api.ts`、`cmd/parcel-pricing-register`、`scripts/demo-seeds/seedgen`；未重读 parcel-pricing `CONTEXT.md` 其余各节、[计价规则模型最终设计](../design/pp-pricing-rule-model-final-design.md)全文与其他登记册的 `CONTEXT.md`——本记录因此只裁**载荷形状归谁定**与**价卡生命周期中草稿、已校验、已批准、已发布各态的持久化载体**，不改任何一条领域不变量，不裁其他登记册的具体形态）
Date: 2026-09-03

## Context

管理台价卡页的「登记价卡」签收的是登记快照 JSON 本体。票 admin-write-faces/01 的交付评论把理由写得很清楚：「ADR-0085 Decision 三把『渠道原始载荷 → 登记快照』的翻译划给渠道接入契约、随 `PAR-INT-01` 提供，现在把它拆成字段就是替租户拟那份还没有的契约」；`api.ts` 的注释同样写着「请求体形状此刻没有契约」。首租户实施的[需求映射](../../.scratch/tenant-implementation-01/requirement-mapping.md)据此把「Excel → 登记批次的转换工具」登记为「待建（实施工具）」。

用户在通道 4 对此的质疑与对 CLI 的质疑是同一句话：一个独立软件产品不能这样。本记录同意，且理由与 ADR-0100 同构——**那句「替租户拟契约」的论证用在了错误的对象上。**

**客户渠道的载荷形状确实不是产品的。** 客户系统走 API、标准文件还是门户，各自的报文长什么样，是客户那一侧的既成事实；ADR-0055 与 ADR-0072 否决「替它拟表」是对的，本记录一字不改。

**操作者面的载荷形状只有一个来源——产品。** 运营配置员面对的是产品自己的界面，他往里填什么、传什么文件，形状由界面决定，而界面是产品的。产品不定义它，就没有人定义它；「等租户」等来的不是一份契约，而是每个租户各一张自己格式的 Excel，每张都要工程师手翻成 JSON——那正是需求映射里那行「实施工具」的实况，也正是 ADR-0085 Alternatives 否决「写面长期只走 CLI」时点名的病：「把一项产品能力错登记为一项运维动作」。

**参考蓝图早已把它当产品功能。** [最终解决方案 V1.2](../reference/reference-design/国际小包计费与结算平台最终解决方案_V1.2_审定版.md)第 38 节「价卡导入与治理」写有导入流程（上传原始文件 → 保存原始文件和哈希 → 识别表头与单位 → 映射价表家族 → 生成草稿 → 标记歧义 → 人工确认 → 校验 → 审批 → 发布）、阻止发布的情况、生命周期与「已发布版本不可编辑」；其交付物清单列有《价卡导入模板与校验规范》。parcel-pricing `CONTEXT.md` 已把生命周期吸收为「草稿 → 已校验 → 已批准 → 已发布 → 已退役或已替代」，并在「不采纳的工件形态」里写明发布期校验「已由生命周期承载，不需要另造工件来承载」。

**今天的实现把草稿、已校验、已批准、已发布压成一次 POST。** `PriceCardRegistration` 一次携带方案、源文件身份、方向授权引用与发布批准责任方，登记成功即进版本清单——也就是直接到「已发布」。草稿、已校验、已批准三态没有任何持久化载体，于是「录入者今天上传、批准者明天批准」在结构上做不到，批准责任方只能是请求体里的一个字符串，谁填的、凭什么填，登记册答不上来。

**校验语义不缺，缺的是翻译与回显。** CONTEXT「草稿 → 已校验」要求的「结构、依赖、区间、单位、币种和规则语义通过检查」，今天由领域构造门承担：`NewPricingPlanVersion`、`NewRateTableVersion` 及其下各构造函数拒绝立不住的方案，`RehydratePriceCardRegistration` 按规范化版本号重验快照。一个导入器需要新写的是「模板的一行 → 构造函数的入参」这段翻译，和「构造门拒了哪一行、为什么」的逐行回显；它不需要、也不该另造一套校验规则。

## Decision

**一、运营操作者面的登记载荷形状由产品定义，属机制半边。** ADR-0085 Decision 三「表单页组装登记载荷」原句不变；票 admin-write-faces/01 评论中「不逐字段建」的理由——「渠道原始载荷 → 登记快照」的翻译属渠道接入契约——**适用场景收窄为客户渠道载荷**。管理台各登记签收什么形状（逐字段表单、模板导入、JSON 快照镜像）由产品按「登记频次 × 操作者角色 × 载荷结构」裁，实施票逐册定；JSON 快照签保留为受控批量口的在线镜像，**不是运营配置员的主路径**。

**二、价卡首例：产品发布版本化导入模板。** 模板（Excel/CSV）由产品定义并版本化，模板版本随规范化版本号（[ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)）演进；模板的列与领域构造函数的入参一一对应，模板不引入领域里没有的概念——它只是 [ADR-0015](./0015-closed-grammar-open-vocabulary-for-multi-carrier-rating.md) 封闭文法的表格投影，开放词汇（费用代码、区域名、承运商）照旧由租户填。模板是产品契约，租户按模板填；租户自有格式到模板的转换属实施服务，不进产品。模板规范作为 parcel-pricing 的设计文档随实施票产出，按 AGENTS 「新增权威文档」条在 [docs/README](../README.md) 登入口。

**三、导入形成持久化草稿，归 parcel-pricing。** 一次导入在 parcel-pricing 立一份**草稿**：原始文件身份（名与 SHA-256，沿 `SourceFileIdentity`，真实文件本体按 [ADR-0008](./0008-self-hosted-minio-evidence-store.md) 外置）、解析出的方案快照、校验结果（通过，或逐行问题清单）、录入操作者（取自 ADR-0100 的 `OperatorEnvelope`，不自报）、时间。草稿册是可执行价卡版本所有者自己的册——版本归谁，版本的草稿就归谁。CONTEXT 生命周期前两态由此有载体：**草稿**是已解析而未过构造门的，**已校验**是过了构造门且规范化摘要已算出的。草稿不进版本清单，任何评价读不到它。

**四、校验与发布共用一份摘要。** 「已校验」那一刻算出的规范化快照与版本内容摘要，就是发布时交给登记用例的那一份；发布**不重解析、不重算**。预览页显示的摘要与登记册记下的摘要必须逐字节相等——否则会长出「预览通过、登记却答内容冲突」这一格，而它与真实的内容冲突在答复上不可分辨。形状同 [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)「重建只校验不重算」。

**五、批准是独立的操作者动作，发布走既有登记用例。** 批准者身份取自 `OperatorEnvelope`，与录入者身份一起记在草稿上，「已校验 → 已批准」由它推进。发布 = 把已批准草稿交给既有的 `RegisterPriceCard` 用例：登记快照即草稿正文，`publicationApprover` 即批准者主体，答案代数（`RECORDED` / `ALREADY_REGISTERED` / `CONTENT_CONFLICT` / `CANONICALIZATION_DIFFERS` / `NOT_ACCEPTED` / `UNDECIDED`）一格不改。受控 CLI 不经草稿，照旧直接消费登记快照——那是受控批量口的定位（ADR-0085 Decision 一），其治理在系统外的受控流程里。

**六、审批职责规则是租户治理参数，缺省朝拦。** 录入者与批准者须否为不同主体、批准者须持哪一格授予，属租户的治理规则，按[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)增一行登记其实例。产品保证的是**两身份都被记录且可比**；规则未登记时，批准门答**未配置、不放行**——分界句同 [ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)：读一个空登记册并如实答未配置不是默认实现。不写死「默认必须双人」，也不写死「默认单人可批」——两者都是替租户定治理规则，AGENTS 红线「未确认参数保持可配置或显式未决，不写死为生产默认」两个方向都禁。

**七、不做的，逐条写明。**

- 不把蓝图第 38 节的回测、影响分析、毛利倒挂阈值、黄金样例作为发布门一并做进来。它们各自是否进产品按[计价规则模型最终设计](../design/pp-pricing-rule-model-final-design.md)的范围判据逐项裁——那份判据存在的目的就是防止产品长成第二套通用计费平台，本记录不替它开口子。
- 不定义客户 API 渠道的载荷形状，那一半照旧等 `PAR-INT-01`。
- 不给已发布版本任何编辑口，CONTEXT 原句「不得修改原版本的正文」照旧；草稿可改可弃，发布后的更正走新版本。
- 不在本记录裁其他登记册的具体形态。

**八、推广原则。** 网络、关务、商业、VE、代收各登记册按 Decision 一自裁：结构简单、低频的册可以直接逐字段表单；矩阵型、高频、需双人治理的册走「模板导入 → 草稿 → 批准 → 发布」；每册在自己的实施票里写明选了哪一形与理由，本记录只钉原则与价卡首例。

## Consequences

- parcel-pricing 增草稿册（迁移模块）、导入用例（解析模板 → 领域构造 → 摘要 → 存草稿）、批准用例、发布用例（交既有 `RegisterPriceCard`），以及草稿查阅读口；对应 HTTP 端点全部挂 ADR-0100 的操作者 Intake，按 ADR-0085 两阶段纪律接线。
- 管理台价卡页：「登记价卡」签改为「导入价卡」（上传 → 校验结果与摘要预览 → 存为草稿）；新增「草稿」签（列表、批准、发布，录入者与批准者两列并排可见）；「价卡目录」签不变；JSON 快照签退为高级口，不出现在运营配置员的主导航路径上。
- 新增 parcel-pricing 设计文档《价卡导入模板与校验规范》，在 docs/README 登入口；模板版本号进文档头部，与规范化版本号的对应关系写在那里而不写在代码注释里。
- 参数登记册增一行审批职责规则（实例半边）；`PAR-SET-02/03`（价卡实例内容）的状态不因本记录改变——模板是产品的，卡上的数仍是租户的。
- 首租户实施的需求映射里「Excel → 登记批次的转换工具 待建（实施工具）」一行由其所有者按本记录改口径为产品功能；本记录不代改 `.scratch/` 文件。
- `scripts/demo-seeds/seedgen` 与 seed 路径不变：合成数据经领域构造函数折装、走 CLI，不经草稿。
- 票 admin-write-faces/01 评论中「不逐字段建」那条的理由被本记录收窄，票面由其所有者补一条指回本记录的评论；`api.ts` 中「请求体形状此刻没有契约」一段随导入端点落地改写。

## Alternatives considered

- **维持只收登记快照 JSON，等 `PAR-INT-01`。** 否决：理由用错了对象，见 Context；代价是每个租户每张卡都要工程师手翻，产品能力被登记成实施工具。
- **逐字段表单作为主路径。** 否决（仅作为主路径）：价卡是分区 × 重量段 × 费用项的矩阵，一张卡上百到上千格，逐格录入既慢又是金额保真度的敌人——CONTEXT 源内容差异条目已记录过一张卡因人工录入而整卡降为「不作为生产金额来源」的先例。表单留给低频、结构简单的册（Decision 八）。
- **无持久化草稿，预览后一键登记。** 否决：它把「已校验 → 已批准」压回请求体里的一个字符串，录入与批准必然同人同刻，任何审批职责规则都无法成立；CONTEXT 生命周期里就只剩「已发布」有载体。
- **草稿放在管理台前端状态里。** 否决：批准者换会话、换人就看不见；录入者身份只能由前端自报，而后端不采信自报身份是 ADR-0100 Decision 二的红线。
- **写死双人审批（或写死单人可批）为默认。** 否决：两者都是替租户定治理规则；红线禁的是「写死为生产默认」，不分方向。缺省朝拦是唯一不替租户做决定的取值。
- **把蓝图第 38 节全套门禁一起做。** 否决：那是第二套通用计费平台的入口，计价规则模型最终设计的范围判据专为挡它而立；逐项另裁。
- **导入器放在 `internal/platform/` 或独立进程。** 否决：导入器调用的是 parcel-pricing 的领域构造门，翻译职责与领域所有权在同一侧才守得住「不另造校验规则」；platform 承载的是无业务判断的技术件。

## Links

- [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：录入者与批准者身份的出处（`OperatorEnvelope`）；本记录 Decision 三、五、六依赖它
- [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)：在线写面属产品能力、CLI 为受控批量口、写表单属机制半边；本记录收窄票 01 评论对其 Decision 三的引申，不停用其任何一条
- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：规范化版本号——模板版本与之同步演进的依据
- [ADR-0015](./0015-closed-grammar-open-vocabulary-for-multi-carrier-rating.md)：封闭文法与开放词汇——模板只是文法的表格投影、不引入新概念的依据
- [ADR-0028](./0028-aggregate-rehydration-is-a-separate-door-that-validates-without-recomputing.md)：只校验不重算——校验与发布共用一份摘要的形状来源
- [ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md)：「读空册如实答未配置不是默认实现」——审批职责规则缺省朝拦的分界句
- [ADR-0008](./0008-self-hosted-minio-evidence-store.md)：原始价卡文件本体外置，草稿只登名与哈希
- [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)：「定价方案与价表版本」生命周期与「不采纳的工件形态」——本记录给前者的草稿、已校验、已批准、已发布配载体，遵守后者不另造校验工件
- [计价规则模型最终设计](../design/pp-pricing-rule-model-final-design.md)：范围判据——蓝图第 38 节其余门禁逐项另裁的依据
- [最终解决方案 V1.2 第 38 节](../reference/reference-design/国际小包计费与结算平台最终解决方案_V1.2_审定版.md)：参考蓝图对导入与治理的目标描述（参考材料，不是本仓权威）
- 票 [admin-write-faces/01](../../.scratch/admin-write-faces/issues/01-registry-configuration-has-no-admin-write-face.md)：「不逐字段建」评论的出处，本记录收窄其理由的适用场景
- [首租户需求映射](../../.scratch/tenant-implementation-01/requirement-mapping.md)：「Excel → 登记批次的转换工具 待建（实施工具）」一行，本记录改其口径为产品功能
- `internal/parcelpricing/domain`：`PriceCardRegistration`、`SourceFileIdentity`、`RehydratePriceCardRegistration` 与方案快照——本记录 Decision 三、四、五在其上加草稿载体，不改它们
- 来源：IDP 队列通道 4 的质疑与授权（2026-09-03）
