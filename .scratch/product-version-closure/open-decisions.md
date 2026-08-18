# 待裁定与附带发现 · #7 最小产品版本正文及持久化

Category: chore
Status: **五项全裁且落地**（收口取证 origin/main `1fff679`，2026-08-18）。本文保留提出时的论证，
不改写原节；落地 SHA 只写在本表与各节末「已落」一行。

配 [design.md](./design.md)。**提出时取证于已提交态 `37495cd`。**

分两半：**D-** 是设计里裁不动、需频道或 owner 拍板的取舍；**F-** 是核设计时撞见的既有件问题，
不阻断本切片，交对应 owner。

| | 裁定 | 挡住的批 | 状态 |
|---|---|---|---|
| **D-1** | 三件均归**规则对象版本**（Intake/Final→kind 4，Cancellation→kind 9） | B6 | **已落** `6e4ccda`（ADR-0058，迁移 0013） |
| **D-2** | **存** `plan_direction` / `binding_conversion` | B3 | **已落** `1b764c3`；绑定补丁 `81707fd`（ADR-0057，迁移 0010/0011） |
| **D-3** | B7 按「**不参与**选择」建表，五维照存 | B7 | **已落** `74a6c35`；夹具消歧 `1fff679`（ADR-0059，迁移 0014） |
| **D-4** | **不进** `ViewRevision` | 无 | **不进**已随 `861280b`（0007）与 B5 兑现；`CommercialAuthority`「已知的收窄」已把族 B 排除出视图适用范围（随 B3 `1b764c3`） |
| **D-5** | 「更正一条更正」合法；两侧同为**只增多条**，取最后一条 | B2 | **已落** `90a90f7`；行锁串行化 `50aed5e`（ADR-0056，迁移 0009） |
| **F-1** | 补租户判定 | 无 | **已落** `1cb4074` |
| **F-2** | 随 B4 加只读端口 | 无 | **已落** `26864d9`（`CommercialPublicationView`） |
| **F-3** | 装载核对，不等即拒装 | 无 | **装载核对已落**（B5 `08964b3` / `5c3d03a`，`ConsistentAcceptanceRulePackage`）；**双处形状未消灭**，长期形态仍交领域 owner |

**B1（产品形态册）不被任何一项挡住**，已落 `cce0cff`——它也是验证整套装载形状的那一批，
实施中据此把「五查」修订为「单查左连接」（design.md §2.3）。

下文各节保留提出时的原始论证；裁定以「**已裁**」小节附在各节末尾，不改写原论证——
裁定是接着那些论证作出的，抹掉它们会让日后读裁定的人看不出它在回答什么。

本文只列草案要点与判据，不代写 ADR 正文——按 AGENTS.md，难逆转取舍要走 ADR，而 ADR 归 owner 落笔。

---

## D-1（阻断）阶段内容声明族的拥有对象

**阻断**：design.md §3.2 整批（B6：三表三口）。不裁就定不下主键，而迁移不可变。

**事实**（三条，各自可复核）：

1. `IntakeQualificationContent`、`FinalRuleContent`、`CancellationAuthorityContent`
   三个类型**都不持有拥有对象**。同族其余成员都持有：`AcceptanceRuleContent` 持 `rulePackage CommercialVersion`，
   `PendingRoutingPermission` 持 `product CommercialVersion`，在途的 `PreAcceptanceControlDeclaration`
   持 `contract CommercialVersion`（**在途未提交**）。
2. `CancellationAuthorityContent` 的注释原文把拥有对象写成「一个已生效**产品或合同**的取消授权目录」——
   两个候选并列，文档层面就未定。`FinalRuleContent` 注释写「一个已生效**规则包**的终局规则声明」，
   同段正文却反复说「此产品下」。
3. 消费侧 `IntakeContentSource` 注释写「声明从哪个规则包版本读、怎么缓存**属装配**」——装配侧也没有答案，
   三口按 `psdomain.SourceIdentity` 取，不按 PC 版本取。

**为什么不能先建表再说**：ADR-0042 的整条纪律是「声明按拥有对象归属……合成一张表会把这条归属抹掉，
而 UC 正是按对象分别判给它们的」。0006 为此把两张表分开，理由写在迁移注释里。拥有对象未定就落主键，
是用一次不可变迁移替这次裁决拍板。

**要裁的三问**：

| 声明 | 候选拥有对象 | 备注 |
|---|---|---|
| 收寄资格（`PAR-COM-16`） | 规则包 / 服务产品 | 注释说规则包；但「允许哪些收寄来源」读起来像产品形态的属性。消费侧 UC-PS-003 |
| 终局规则（`PAR-COM-17`） | 规则包 / 服务产品 | 注释与正文自相矛盾（见事实 2）。消费侧 UC-PS-004 |
| 取消授权目录（`PAR-COM-17`） | 服务产品 / 客户合同 | 注释明列两者。消费侧 UC-PS-006 |

**判据建议**（供裁决参考，不是结论）：沿用 ADR-0042 判待路由许可归产品时用的那条——
看消费侧 UC 把这件事判给谁说了算（三者分别是 UC-PS-003、UC-PS-004、UC-PS-006）。
**三件可以不同归属**；ADR-0042 本身就把校验组判给规则包、把待路由判给产品。
硬要三件同归一处，反而是在抹掉 ADR-0042 想保住的区分。

**代价**：不裁则 B6 停摆，PS 三口继续无生产实现，`service_stage_rules.go` 注释里那句
「日后存储成片时三口同切」继续挂账。

---

## D-2 价格政策册要不要存 `planDirection` 与 `binding_conversion`

**事实**：`NewCommercialPricePolicy` 收 `planDirection`、`conversion` 两个入参，交给 `checkPlanBinding`
判完**即弃**——两者都不在 `CommercialPricePolicy` 结构体上。后果是：**不存这两列，装载口重建不出政策**，
因为构造函数要它们。

**建议**：存。理由不是「给结构体补字段」，是 UC-PC-001 步骤 1「保全来源、来源版本、内容摘要、请求方、
批准依据」的直接适用——`planDirection` 是发布当时 `parcel-pricing` 给出的答复，本上下文没有资格自己查
（同 ADR-0034 把 `PricingPlanStandingLookup` 设为入参而非查询的那条纪律）。把当时的答复连政策一起保全，
装载时原样传回，`checkPlanBinding` 每次装载重跑一遍，`AT-PC-033`「不把 BUY 价卡隐式当 SELL 价卡」
因此在装载面上也守得住。

**否决的替代**：给价格政策开一条 `RehydratePricePolicy` 路绕过 `checkPlanBinding`。它把 AT-PC-033
的守卫降级成只在发布面有效，而装载面恰恰是生产解析每次都要走的那条路。

**为什么要 ADR 而不是径直写代码**：它改的是「登记册存什么」，且引入一类新东西——
**本上下文保全的、由邻接上下文给出的发布期答复**。这在 PC 侧没有先例，值得记一笔，
否则下一个人会把这两列读成「PC 自己推断的方案方向」，而那正是 ADR-0034 否决过的东西。

**阻断**：design.md B3（0008/0009 两册）。B1／B2 不受影响。

---

## D-3 规则包的五维适用性要不要参与选择

**事实**：`RulePackageApplicability` 有五维（服务产品、合同、法人、范围、期间），
而 `ResolveCommercialBasis` 选候选时**完全不看它**——只按 `version.scope` 相等与
`registry.selectionInterval(version)` 命中筛。也就是说今天规则包的适用性由**版本壳的范围**表达，
正文里那五维在解析路径上是死字段。

**两个方向，代价不同**：

- **不参与**（现状延续）：规则包正文纯属点读族，表按 design.md §3.1 的形状建，`ViewRevision` 不变。
  代价是正文里五维继续无人读——那本身是个信号，说明它要么该被删、要么该被用。
- **参与**：五维必须进 `CommercialRegistry` 与 `ViewRevision`（design.md §1 判据），
  候选过滤逻辑改写，`AT-PC-021`「同一范围两个合同同时命中」一路的冲突判定基数随之变化，
  失效检测面同步变宽。

**性质**：这是**领域决定，不是持久化决定**。它决定规则包正文落哪一族，因此挡着 B7，
但不该由持久化设计代拍。

**建议**：交 PC 领域 owner。若短期不裁，按「不参与」建表可行且可逆——反向（点读族改成视图族）
只需新增装载路径，不需要改已落的表。

---

## D-4 在途 0007 声明族要不要进 `ViewRevision`

**在途未提交，不作为现状断言。** 承输入包 §4.2 第 4 点登记的张力，本切片同样裁不动它，原样上交。

在途的 `PreAcceptanceControlDeclaration`（合同 → 要不要接受前控制）今天不参与 `ViewRevision`
派生（登记册无该通道）。本切片给合同正文建表（B5）之后，同一个拥有对象（客户合同版本）
下会有两路内容：范围级的财务控制绑定（正文件）与合同版本级的「要不要」声明（在途 0007）。

- 若两路都**不进**视图（design.md §1 判据下的族 B 归属，本文的建议）：
  `CommercialAuthority` 注释那句「漏在外面会让 ViewRevision 按不完整的内容派生」的**适用范围要重新陈述**——
  它今天读起来像是对所有登记内容说的，实际只对参与选择/采用的内容成立。
- 若要进：需要说明为什么一次与选择无关的声明改动应当把该范围在途解析全判失效。

**建议**：不进，并同笔把那句注释的适用范围写准。**这一句注释的改动落在他人在途件的相邻位置**，
按并行会话规约要先在频道说一声、只改自己那一行。

**已落（不进）**：0007 入 `861280b`，合同正文入 B5；两路都不进 `ViewRevision`。
`CommercialAuthority`「已知的收窄」已把族 B 排除出视图适用范围（随 B3 `1b764c3`）。
原「押后」的注释修正因此已发生，不是本收口新做。

---

## D-5 区间更正册撞键时答覆盖还是答冲突

**阻断**：design.md B2（0011 区间更正册）。

**事实**：本仓的登记落点代数一路是「同键同内容=重放，同键异内容=冲突，绝不覆盖」——
`Register`、`SaveVersion`、`SaveGrant`、`Save`（解析）四处同款，ADR-0031 记着这条。
**区间更正是唯一的例外**：`RegisterValidityCorrection` 撞键时先比 `sameValidityCorrection`，
同更正返回既有视图（重放），**异更正直接 `registry.corrections[key] = correction` 覆盖**并推进 `ViewRevision`，
既不报冲突也不报错。

于是持久化面有两条路，选错哪一条都会让两侧分家：

- **跟内存侧（覆盖）**：`ON CONFLICT ... DO UPDATE`。代价是本册成为发布登记册里**唯一可原地改写的表**，
  而 UC-PC-001 结果语义`已发布`那一格的禁止行为写着「原地编辑正文或追溯改写历史」。
  可辩护的地方在于更正改的不是正文——ADR-0038 正是把它与 `Revise`／`SupersededBy` 分清的那份记录。
- **跟其余四处（冲突）**：`ON CONFLICT DO NOTHING` + 读回比内容。代价是**内存侧与库侧对同一次调用给不同答案**：
  「更正一条更正」在内存登记册上是一次正常覆盖，在库上答`内容冲突`。

**建议**：先问一个前置问题——**「更正一条更正」是不是一件合法的业务事**。
若合法，正解多半不是二选一，而是**更正只增、一个版本可有多条、选用区间取最后一条**，
即把内存侧的 map 也改成有序集合；那样两侧同为只增，`ViewRevision` 也自然覆盖全部更正历史。
若不合法，则内存侧那次覆盖本身是个缺陷，该答冲突。

两种走向都要动领域，因此这不是持久化设计能单方面决定的 → 交 PC 领域 owner，建议记 ADR
（ADR-0038 的 supersede 或补充，不改写已接受正文）。

---

## 附带发现（交 owner，不阻断本切片）

### F-1 `ViewRevision` 的价格政策分支缺租户过滤

`CommercialRegistry.ViewRevision` 派生时，结算政策与产品两支都判
`policy.version.tenant != tenant → continue`，**价格政策那一支只判 `policy.scope != scope`，没有租户条件**。

今天登记册按租户装载，因此触发不到。但 ADR-0040／ADR-0003 要求租户是身份的一部分而非过滤器，
而四册落库、装载口变宽之后，任何一次「按范围装载」的口子都会让它变成可触发的跨租户串味。
三支不一致本身也是个维护面隐患——读的人会以为那是有意的。

**建议**：与另两支对齐，加租户判定。改动一行，属领域包 owner。

**已落** `1cb4074`。

### F-2 `CommercialAuthority` 持有写能力端口，两端口分离的理由已被自己的装配点破掉

`ports.go` 写着两个端口分开的理由是「合并会让只需读的解析持有 `SaveVersion`」，
但 `NewCommercialAuthority(registry ports.PublicationRegistry)` 收的正是那个写侧端口。
今天多持有一个 `SaveVersion`；design.md §2.5 的四个 Save 落地后会变成多持有五个。

**建议**：随本切片 B4 修（加只读端口，`CommercialAuthority` 依赖它）。属「会让旧调用点对不上」那一类，
调用点少（`NewCommercialAuthority` 及其测试），单独 worktree 一次性应用即可，**改前频道占号**。

**已落** `26864d9`：`PublicationRegistry` 内嵌 `CommercialPublicationView`；`NewCommercialAuthority` 只收只读口。

### F-3 合同的规则包引用在两处

`CustomerContract.rulePackage`（正文件字段）与 `CommercialVersion.references[AcceptanceRulePackageObject]`
（版本壳指名引用）可以承载同一个事实。而 `declaredReferences` **不强制**合同必须指名规则包
（`references` 允许为空），所以两处不是「一处派生另一处」，是两处各自可空、可分歧。

本切片不修（属领域形状，非持久化造成），但持久化会把它固化到库里，因此 design.md §3.1 加了一道装载核对：
两处都在场时必须相等，不等即拒绝装载该行。**这只保证不分歧，不消灭双处。**

**建议**：由 PC 领域 owner 决定长期形态——要么规定合同版本必须指名规则包（正文件字段变派生），
要么明确两处各管一件事并写下区别。

**已落（只保证不分歧）**：B5 `08964b3` / 复审 `5c3d03a` 装载走 `ConsistentAcceptanceRulePackage`，
两处都在场且不等则拒装、不折成未配置。**双处形状未消灭**——正文件字段与版本壳指名引用仍各自可空，
长期形态仍交领域 owner。
