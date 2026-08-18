# 最小产品版本正文及持久化 · 设计

Category: feature
Status: **机制半边已落** origin/main `1fff679`（2026-08-18）。五项裁定均已落地；实例半边仍空，
不宣称真实可配置完成。裁定原文与逐项落法见
[open-decisions.md](./open-decisions.md)；落地清点见 §8。

## 进度

取证于已提交态 `1fff679`（`git log --oneline` + 对应迁移/ADR 文件在该 SHA 均在）。
下表 SHA 是合入 `origin/main` 后的提交，不是各隔离 worktree 的 tip。

| 批 | 落地 | 迁移 / ADR |
|---|---|---|
| F-1 `ViewRevision` 价格政策补租户判定 | **已落** `1cb4074` | 无新表 |
| 0007 接受前控制声明（B5 前置，原「在途」） | **已落** `861280b` | 0007；D-4 不进 `ViewRevision` |
| B1 服务产品形态册 | **已落** `cce0cff` | 0008 |
| B2 区间更正册 | **已落** `90a90f7`；行锁串行化 `50aed5e` | 0009；[ADR-0056](../../docs/adr/0056-validity-correction-append-only-serialized-by-version-row.md) |
| B5 合同正文父子表 | **已落** `08964b3`；复审补丁 `5c3d03a`（租户身份闭包 + 单查左连接） | 0012；F-3 装载核对同批 |
| B3 价格与结算政策册 | **已落** `1b764c3`；绑定补丁 `81707fd`（写入前重跑 `NewCommercialPricePolicy`，0010 CHECK 镜像矩阵） | 0010 / 0011；[ADR-0057](../../docs/adr/0057-price-policy-preserves-publish-time-adjacent-replies.md) |
| B4 只读端口收窄（F-2） | **已落** `26864d9` | 无新表；`CommercialPublicationView` |
| B6 阶段内容声明族 | **已落** `6e4ccda` | 0013；[ADR-0058](../../docs/adr/0058-stage-content-owned-by-rule-objects.md) |
| B7 规则包正文 | **已落** `74a6c35`；夹具消歧 `1fff679` | 0014；[ADR-0059](../../docs/adr/0059-rule-package-applicability-stored-not-selected.md) |

**一处设计在实施中被修订**：§2.3 原写「一次范围装载五查」，B1 落地时改为**单查左连接**。
理由与原设计的缺陷都记在 §2.3，不抹掉原方案。

治评审 [`docs/review/081701.md`](../../docs/review/081701.md) 第 7 条「产品版本未闭合」。输入包见
[design-input.md](./design-input.md)（取证快照，不随实施改写）。

设计落笔时取证 `37495cd` / `93165cd`；0007 当时在途未提交。收口时 0007 已入 `861280b`，
下文凡写「在途未提交」的 0007 均以该 SHA 为准，不再是现状。

**地盘**：设计正文仍是本包；本收口另改 [`docs/review/081701.md`](../../docs/review/081701.md)
处置标账第 7 行。不动产品基线、不动 `internal/` / `migrations/`。§2 起的 SQL 与 Go 签名
是设计稿；对应实现以 `1fff679` 上的迁移与端口为准，号段以 §8 为准（落地顺序与草案不完全同号）。

---

## 0. 本文管什么

评审第 7 条在代码上的形状（输入包 §2.4 一句话汇总）是：九类商业对象里只有版本壳、解析闭包、授权治理册与
三样声明有持久化面；**五通道登记册里四通道 + 正文件六种共十项没有落库面，其登记方法在非测试代码中零调用。**

本文对这十项逐项定：**归哪一族、模式长什么样、由哪个装载口交出、未配置怎么说、本切片做不做。**

不管的：正文内容（实例半边，按红线留空并拒绝默认值）、跨上下文消费方的编排改动（属各消费方的票）、
真实迁移文件与 Go 代码（本文只出设计稿）。

---

## 1. 分线：判据是「进不进 `ViewRevision`」，不是「是不是正文」

十项不是同一类东西。把它们按「正文件 vs 登记通道」分，是按**今天有没有内存通道**分——那是历史，不是判据。
按落地位置分族要用一条能一直用下去的判据。

判据取 `ViewRevision`。它是提交前失效检测的唯一依据：`ValidateClosureBeforeDecision` 按原键重解，
比的就是 `resolutionID`，而 `resolutionID` 由 `ViewRevision` 与各采用版本派生。于是：

- **凡参与「选哪个候选」或「采用时附带什么」的内容，必须进 `ViewRevision`。** 漏掉它，
  `CommercialAuthority` 注释里那句就会成真——「漏在外面会让 ViewRevision 按不完整的内容派生，
  从而在视图其实已经变了的时候答『还是同一个视图』」。
- **凡「唯一选出之后才读」的内容，绝不能进 `ViewRevision`。** 进了，一次与选择毫无关系的声明改动
  会把该范围全部在途解析判成`已失效`。这是 ADR-0042 决定三「塞进解析结果，声明就有机会参与决定
  它自己被谁采用」在持久化面上的同一条纪律：那条管的是读取时机，这条管的是失效基数。

按这条判据，十项分三族：

| 族 | 成员 | 装载方式 | 进 `ViewRevision` |
|---|---|---|---|
| **A 权威视图族** | 价格政策、结算政策、产品形态、区间更正 | 随范围整册装载，进 `CommercialRegistry` | 是（今天内存侧已经进了） |
| **B 按拥有对象点读族** | 合同正文、规则包正文、阶段内容声明族 | 按已唯一选出的对象点读，独立只读端口 | 否 |
| **C 本切片不建表** | 产品—渠道映射、协议正文、信用政策正文 | — | — |

族 A 的四项判进来是因为**它们已经在里面了**：`ResolveCommercialClosure` 直接消费
`registry.policies`／`settlementPolicies`／`serviceProductOf`／`selectionInterval`，`ViewRevision`
也已经为四者各派生一段。它们缺的只有落库面，语义一格都不用动——这是本切片风险最低的一半。

族 B 判出去要单说，因为它反直觉：合同正文与规则包正文是**正文**，看起来比声明更「权威」。但今天
`ResolveCommercialClosure` 一个字节都不读它们——选候选只看 `version.scope` 与 `selectionInterval`，
确认指名引用只看 `version.references`。把它们塞进登记册不是「补全视图」，是**扩大失效基数**：
改一条与选择无关的费用范围绑定，会让该范围所有在途解析重解。要让它们参与选择是另一个决定
（见 open-decisions D-3），且那是领域决定不是持久化决定。

族 C 的判据不是族 B 的判据，第 5 节单说——**它与 ADR-0053 的「不建表」也不同源**，那一条的理由本文不适用。

---

## 2. 族 A：四登记通道的持久化面

### 2.1 一册一表，行不复制版本壳

既有两种先例，本设计取后者：

- `authorization_grant`（0003）**把整份 `versionDocument` 嵌进自己的快照**，重建走
  `document.Version.version()` → `NewAuthorityGrant`。它当时有充分理由：0001 的 `object_kind` CHECK 是 1..7，
  第九类根本进不去版本册，注释自己写着「不并进 commercial_version……日后授权规则走发布登记册属另票」。
- `acceptance_rule_content`／`pending_routing_permission`（0006）**只存自己的字段，按版本四元键挂在拥有对象上**。

`c1866c2`（迁移 0004）把 kind CHECK 放宽到 1..9 之后，0003 那个理由已经消失，四册没有任何理由再走它。
**族 A 四表一律只存自己的字段，按 `(tenant_id, object_kind, object_id, version_label)` 外键回
`commercial_version`。**

理由是单一权威，且这里的代价具体可说：版本壳复制两份，两份就能不一致——一个版本在 `commercial_version`
里已经 `RETIRED`，而政策行内嵌的快照仍写着 `EFFECTIVE`。那不是理论风险，正是 `ViewRevision`
存在的目的所要防的东西，而复制会让它在**登记册自己内部**被绕过。

连带一条：**族 A 四表都不存 `scope_ref`。** 范围是版本壳上的事实，装载查询 join `commercial_version`
并按 `scope_ref` 过滤即可。存第二份会让「政策行说 A 范围、版本行说 B 范围」这种行存得下来。

（注意区分：`CommercialPricePolicy` 自己的 `scope` 字段与 `SettlementPolicy` 的六维
`applicability` **不是**版本范围，它们是政策正文的一部分，必须存——见下表。）

### 2.2 四张表

迁移号自 **0008** 起，前提是在途 0007 先落。若 0007 未落则整体前移一位；**号段由落地顺序定，本文不锁死。**

#### 0008 `commercial_price_policy`（ADR-0034）

| 列 | 类型 | 说明 |
|---|---|---|
| `tenant_id` / `object_kind` / `object_id` / `version_label` | 主键四元组 | `object_kind` CHECK = 6（`PriceRuleObject`） |
| `direction` | text | 镜像 `PriceDirection`：`BUY` / `SELL` / `INTERNAL` |
| `plan_ref` | text | `PricingPlanReference` |
| `plan_direction` | text | **发布当时 `parcel-pricing` 对该方案自身方向的答复**，同封闭集 |
| `binding_conversion` | text | 镜像 `PlanBindingConversion`：`NONE` / `FROZEN_BUY_EVALUATION` |
| `policy_scope_ref` | text | 政策自己的适用范围，不等于版本范围 |
| `effective_starts_at` / `effective_ends_at` | timestamptz / 可空 | 政策自己的有效区间 |
| `registered_at` | timestamptz | 默认 `now()` |

`plan_direction` 与 `binding_conversion` 这两列要专门交代，因为它们**不在 `CommercialPricePolicy`
结构体上**：`NewCommercialPricePolicy` 收下它们、交给 `checkPlanBinding` 判完就丢弃。后果是
**不存这两列就重建不出政策**——装载口拿不出构造函数要的入参。

存它们不是给结构体补字段，是 UC-PC-001 步骤 1「保全来源、来源版本、内容摘要、请求方、批准依据」
的直接适用：`plan_direction` 是发布当时 PP 给出的答复，本上下文没有资格自己查（同
`PricingPlanStandingLookup` 是入参而非查询的那条纪律）。把当时的答复连同政策一起保全，装载时原样传回，
`checkPlanBinding` 会在每次装载时重跑一遍——AT-PC-033 那条「不把 BUY 价卡隐式当 SELL 价卡」因此
在装载面上也守得住，而不只在发布面。

这一条改的是「登记册存什么」，因此建议记 ADR（open-decisions **D-2**）。

约束：`direction` / `binding_conversion` 各一条封闭集 CHECK；`effective_ends_at IS NULL OR > starts_at`；
四元键与两个引用列非空白；外键回 `commercial_version` 四元组。

#### 0009 `commercial_settlement_policy`（ADR-0044）

主键四元组同上，`object_kind` CHECK = 7（`SettlementPolicyObject`）。

`method` 镜像 `SettlementMethod` 封闭二值 `PREPAID` / `TERMS`——**第三值有意不留**，
「客户级默认」在领域里被排除过一次，库上再开一格就是把它请回来。

`SettlementApplicability` 六维全部平铺成列：`legal_entity_ref`、`counterparty_ref`、`contract_label`、
`charge_scope_ref`、`currency_code`，加区间两列。六维平铺不进 jsonb，因为
「同一精确范围两法命中即冲突」这条判定要按维度比对；埋进 jsonb 之后每次冲突判定都要先解一次文档，
而那正是最不该出错的一步。

#### 0010 `service_product_form`（ADR-0050）

主键四元组，`object_kind` CHECK = 1（`ServiceProductObject`）。唯一正文列 `form`，
CHECK 镜像 `ServiceProductForm` 今日封闭集——**只有 `NETWORK_SERVICE` 一值**。

面单渠道形态按 `PAR-COM-12` 本期不适用，领域里就有意没列，库上同样不列。ADR-0050 已经把这条写死过一次：
「把适配器写成常量`要求`就是一个默认值……适配器必须读一份真声明再翻译」——一张只能存一个值的表，
读出来的仍然是一份真声明；预先列上第二个值才是替租户拟。

#### 0011 `commercial_validity_correction`（ADR-0038）

行按版本四元组指向**被更正的原版本**，`object_kind` 不钉死（九类都可能被更正）。正文列：
`corrected_starts_at`、`corrected_ends_at`（可空）、`correction_ref`、`corrected_at`。

本表是 ADR-0038 那句「有效性更正是登记册上的独立事实，不改写原 `CommercialVersion` 键下的正文、批准与原区间」
的落库形态：**原区间留在 `commercial_version` 一字不动，更正区间只在本表。**

~~一个版本至多一条更正，主键即四元组~~——本文初稿照内存侧
`map[commercialVersionKey]ValidityCorrection` 的形状这么定，**D-5 裁定后不成立**：一个版本可有
多条更正。四元组因此只是外键，不是主键。

被 D-5 推翻的正是下面这段分析，原样保留因为裁定是接着它作出的：

> **落点代数不能照搬另外三册**，这是本册唯一的特殊处：`RegisterValidityCorrection` 撞键时，
> 同更正重放不推进视图，**异更正直接覆盖**（`registry.corrections[key] = correction`）并推进视图，
> 不报冲突。而版本册的 `Register` 对同键异内容答 `RegistrationConflict`——两者是不同代数。
> 若持久化面照抄另外三册的 `ON CONFLICT DO NOTHING` + 读回比内容，一次「更正一条更正」在库上
> 会答`内容冲突`，而在内存登记册上是一次正常的覆盖，两侧就此分家。

**D-5 已裁（MCP-1）**：「更正一条更正」合法，正解取第三条路——**两侧同为只增：一个版本可有多条
更正，选用区间取登记顺序上的最后一条**；内存册那次覆盖按缺陷修，随 B2 同笔。

于是本表的主键不是四元组：四元组下要能放多条，需要一个登记序轴。这一改动落在 B2，届时按裁定
落法定序键（若「取最后」需要新排序键或并发语义决定，按裁定升级写 ADR）。

### 2.3 装载：单查左连接（B1 实施时修订，原为「五查」）

**原方案**：`LoadForScope` 一查扩成五查（先版本册、再四册），四册各自 join `commercial_version`
按 `(tenant_id, scope_ref)` 过滤，重建时按四元键在刚建好的 registry 上 `Lookup` 取版本壳。

**B1 落地时改成一条左连接语句。** 原方案有一个当时没看见的缺陷：

> 五查是五条独立语句。`bentopg.DB.ReadExecutor` 交回的是 `Querier`，**不保证多条语句同处一个
> 快照**（是否在事务里取决于 ctx 有没有带）。两次读之间若有写入落地，装出来的登记册会是一半
> 旧一半新，而 `ViewRevision` 由各通道内容共同派生——于是它会派生出一个**从未存在过的中间状态**
> 的修订。那比读到旧数据更糟：旧数据至少对应过某个真实时刻，这个修订谁都没见过，却会被拿去
> 判定在途解析是否失效。

一条语句天然免疫这一类。改法可行是因为**四册都以版本四元组为主键、与版本一一对应**，左连接后
仍是每个版本一行，不放大结果集：

```sql
SELECT version.snapshot, product.form   -- 其余三册按同样方式续接列
  FROM party_commercial.commercial_version AS version
  LEFT JOIN party_commercial.service_product_form AS product
         ON product.tenant_id = version.tenant_id AND product.object_kind = version.object_kind
        AND product.object_id = version.object_id AND product.version_label = version.version_label
 WHERE version.tenant_id = $1 AND version.scope_ref = $2
 ORDER BY version.object_kind, version.object_id, version.version_label
```

连带好处：`Lookup` 回指那一步没有了——版本壳与它的正文件在同一行里，挂不错。原方案第 4 条
（`Lookup` 取不到即 error）随之消失，它防的那类不一致在单查下结构上不可能发生。

逐行重建仍是两条纪律，与原方案一致：

1. 正文件列为 NULL → 该版本没登记这一册，**不是缺陷**（族 A 不设未配置格，见第 4 节）；
2. 版本状态非 `EFFECTIVE` → **跳过该正文件**（见 2.4），版本本身照常入册；
3. 正文件取值领域不认 → **整次装载 error 上抛**，不跳过。

第 3 条与第 2 条方向相反是有意的。第 2 条是「这一行不在本次视图内」，第 3 条是
「这一行不该长成这样」——那是坏数据，跟 `commercial_publication.go` 里
「快照不是本适配器写下的形状」是同一种硬拒。把坏行悄悄跳过，登记册会少一份正文件，
而解析会因此得出一个看起来完全正常的`唯一已解析`，只是正文件不可观察——那与
「这个对象根本没登记正文件」在调用方那里分不开，一次坏数据会伪装成一格合法的缺席。

### 2.4 版本非生效时，行怎么办

四册的 `New*` 一律要求 `version.status == CommercialVersionEffective`（九个正文件构造函数无一例外）。
而 `commercial_version` 存 2..6 五种状态。于是必然出现「政策行在册，但它的版本已经 `RETIRED`」。

**决定：行永不删（不可覆盖），装载时不进登记册。**

这不是静默丢失，理由要说清：`ResolveCommercialBasis` 选候选时本来就 `version.status != CommercialVersionEffective → continue`，
所以一个已收尾版本的正文件即使装进来也永远选不中；而版本状态本身**在 `ViewRevision` 里**
（版本那一段派生含 `version.status.String()`），因此 `EFFECTIVE → RETIRED` 这次转变照样推动修订、
照样触发在途解析重解。跳过正文件行不会让任何一次失效检测漏掉。

反过来若为此放宽 `New*` 的生效前置（加一条 `Rehydrate*` 路），就等于给九个正文件的共同不变量开一个只有
适配器走得到的口子。那属难逆转取舍，按 AGENTS.md 要走 ADR；本设计不需要它，因此不开。

### 2.5 端口扩展形状

`ports.go` 的 `PublicationRegistry` 注释已经预告过本切片：「本口先只承载版本册；有效性更正册（ADR-0038）
与价格/结算政策册（ADR-0034/0044）的持久化面另票补，端口届时扩展而不是在这里预开空方法」。按它扩，
再补 ADR-0050 之后多出来的产品册（该注释写「三册」，产品册是第四册，注释未及更新）。

**写侧：四个具名 Save，不开通用口。**

```go
SavePricePolicy(ctx, policy domain.CommercialPricePolicy,
    planDirection domain.PriceDirection,
    conversion domain.PlanBindingConversion) (PolicySaveOutcome, error)
SaveSettlementPolicy(ctx, policy domain.SettlementPolicy) (PolicySaveOutcome, error)
SaveServiceProduct(ctx, product domain.ServiceProduct) (PolicySaveOutcome, error)
SaveValidityCorrection(ctx, correction domain.ValidityCorrection) (PolicySaveOutcome, error)
```

四个方法各收一个具名领域类型。前三个的落点代数复用既有三值形状（`已保存` / `已登记`（重放）/
`内容冲突`，ADR-0031 同款）；**第四个不一定**——区间更正在内存侧是后写覆盖而非冲突，代数待 D-5 裁定，
届时可能要为它单开一个落点类型。

四个具名方法正是 UC-PC-001 交接要的「按业务对象提供语义化命令与 Repository」。它反对的是
`SaveContent(kind, payload)` 那种「可以修改任意商业表」的口，具名方法不落在那个禁令里。

`SavePricePolicy` 多两个入参，因为结构体不带它们（见 2.2）。签名上看得见比塞进一个 spec 结构体好：
调用方必须显式交出 PP 的答复，忘了就编不过。

**读侧：把 `CommercialAuthority` 收窄到只读端口。**

这一项不是新增，是修一处已经失效的隔离。`ports.go` 写着两个端口分开的理由是
「合并会让只需读的解析持有 SaveVersion」，但 `NewCommercialAuthority(registry ports.PublicationRegistry)`
收的正是那个写侧端口——**理由已经被自己的装配点破掉了**。今天它多持有一个 `SaveVersion`；
四个 Save 落地后会变成多持有五个。

建议同批加一个只读端口（`LoadForScope` 单方法），`CommercialAuthority` 依赖它，`PublicationRegistry`
内嵌或实现它。这是「会让旧调用点对不上」那一类改动，按并行会话规约要先在频道占号；
调用点少（`NewCommercialAuthority` 与它的测试），走单独 worktree 一次性应用比三步法划算。

---

## 3. 族 B：按拥有对象点读的模式与装载口

族 B 照 0005／0006 已经立起来的范式走：**表按拥有对象的版本四元键，`object_kind` 用 CHECK 钉死，
封闭枚举镜像成 CHECK，「查无此行」= 未配置，读口只读、不带 Save。**

在途 0007（合同 → 接受前控制要不要）是这张图上最新的一例（**在途未提交**）。族 B 各表与它并列，不并表。

### 3.1 合同正文：父行 + 绑定子表（0012）

`CustomerContract` 是 `version + rulePackage + bindings`，而 `NewCustomerContract`
**允许 bindings 为空**。于是这里正撞上 ADR-0052 Decision 四第二条：

> 空册与「登记了一个空集合」是两回事。前者是未配置；后者是租户明确声明……登记册必须能区分这两者，
> 不能靠「查出零行」推断。

- 「合同正文未登记」→ 无父行。
- 「合同已登记，但没对任何费用范围作约定」→ 有父行、零子行。`FinancialControlFor` 对任何范围答「不存在」，
  而那是合同正文里那句「未绑定的范围答『不存在』而不是『不适用』」要的答案。

两者靠一张表分不开，因此**必须父子两表**——与 0006 拆
`acceptance_rule_content` / `acceptance_rule_check_group` 同一个理由，同一个形状。

`customer_contract_content`（父）：四元键 + `object_kind` CHECK = 2 + `rule_package_id` + `declared_at`。
`customer_contract_control_binding`（子）：父键 + `charge_scope_ref` 进主键（同一范围两条结构性挡住）+
`policy_id`（可空）+ `inapplicability_basis`（可空）+ 外键级联删除。

子表要害一条 CHECK：**`policy_id` 与 `inapplicability_basis` 恰有一个非空。**

```sql
CHECK ((policy_id IS NOT NULL AND inapplicability_basis IS NULL)
    OR (policy_id IS NULL AND inapplicability_basis IS NOT NULL))
```

它镜像 `FinancialControlBinding` 那条「要么适用一份指名的策略，要么显式不适用并记录依据，
零值两者都不是」。两列都空的行读回来就是一次零值绑定，而零值绑定「读不成允许通过」这句话
只在领域里成立——库上放行一行两空，`NewAppliedFinancialControl` 与 `NewInapplicableFinancialControl`
都构造不出它，装载只能整次 error。用 CHECK 在写入时挡住，比在装载时炸掉好。

（写法遵 0003 立下的那条纪律：可空列先 `IS NULL` / `IS NOT NULL`，不让任何比较式单独以 NULL 决定约束。）

**`rule_package_id` 是一处真张力，必须写明。** 同一个事实可能在两处：正文件的 `rulePackage` 列，
与版本壳的 `version.references[AcceptanceRulePackageObject]`。`declaredReferences` 已逐字核过——
**它不强制合同必须指名规则包**，`references` 允许为空。所以「不建这一列、装载时从 `version.references` 取」
这条省列的路走不通：一份没有指名引用的合同版本合法存在，而 `NewCustomerContract` 必须拿到规则包引用。

因此列要建，同时装载时加一道核对：**若 `version.ReferenceTo(AcceptanceRulePackageObject)` 在场，
必须与本列相等，不等即拒绝装载该行**（属 2.3 第 4 条那种硬拒）。这不消灭双处，只保证两处不会
悄悄分歧。双处本身是领域已有的形状，不是本切片造成的，也不在本切片修（见 open-decisions F-3）。

### 3.2 阶段内容声明族：**拥有对象未裁定，本切片阻断**

PS 侧 `service_stage_rules.go` 的注释已经承诺过它：「存储问题属于整个阶段内容声明族
（Intake/Final/Cancellation 三口一盘棋）……日后存储成片时三口同切」。本切片本该切它，但切不动，原因是
**主键定不下来**：

1. 三个内容类型 `IntakeQualificationContent`、`FinalRuleContent`、`CancellationAuthorityContent`
   **都不携带拥有对象**。这与族 B 其余成员正相反——`AcceptanceRuleContent` 持有 `rulePackage CommercialVersion`，
   `PendingRoutingPermission` 持有 `product CommercialVersion`，在途的 `PreAcceptanceControlDeclaration`
   持有 `contract CommercialVersion`。三件一个都没有。
2. `CancellationAuthorityContent` 的注释把拥有对象写成
   「一个已生效**产品或合同**的取消授权目录」——**文档里就是未定的**。
   `FinalRuleContent` 注释写「一个已生效**规则包**的终局规则声明」，正文却反复说「此产品下」。
3. 消费侧三口（`IntakeContentSource` 等）按 `psdomain.SourceIdentity` 取，注释明说
   「声明从哪个规则包版本读、怎么缓存属装配」——**装配也没答**。

ADR-0042 的整条纪律是「声明按拥有对象归属，合成一张表会把这条归属抹掉」。拥有对象没裁定就建表，
等于用一次迁移把它拍死，而迁移不可变。这正是 ADR-0053 那条判据在此处的正当适用面：
**该等的不是租户参数，是一次归属裁决。**

三件的模式其余部分已经可以写死，等裁决落地即可套：
`intake_qualification_content`（父：允许来源子表 + 资格引用子表）、`final_rule_content`
（责任结果 → 终局类型，四值封闭集进主键）、`cancellation_authority_content`
（请求方格 → 规则引用，二值封闭集进主键）。三者都是「至少一行」，与 0006 同样**库上守不住**
（SQL 表达不了「子表至少一行」），由装载时过 `New*` 兜——`New*` 对零行一律答
`Err*NotConfigured`，与「无父行 = 未配置」殊途同归，这一点可接受。

**D-1 已裁（MCP-1）**：三件的拥有对象都是**规则对象版本**，不是产品或合同——后两者是**采用方**
（经解析闭包接受时固定），UC 与适配器注释里的「按产品/合同」全是采用层话语，不是归属层话语。
逐件：

| 声明 | 拥有对象 | 裁定依据 |
|---|---|---|
| 收寄资格 | `ACCEPTANCE_RULE_PACKAGE` 版本（kind=4） | 类型注释与适配器协作者注释两处一致 |
| 终局规则 | `ACCEPTANCE_RULE_PACKAGE` 版本（kind=4） | 类型注释记规则包；UC-PS-004「接受时固定…终局规则版本」与接单规则包采用语义吻合 |
| 取消授权目录 | `AUTHORIZATION_RULE` 版本（kind=9） | UC-PS-006 交接表明记 PC 提供「授权规则」；九类封闭集恰有此格；CONTEXT 给授权规则独立身份「不合并为大配置」 |

于是 B6 三表主键 = （租户，拥有规则版本四元组［`object_kind` 入 CHECK］，声明格）。同笔要写 ADR
记归属裁定与证据，并修两处注释：`CancellationAuthorityContent` 的「已生效产品或合同」（归属与
采用两层混写）与 `FinalRuleContent` 正文的「此产品下」（采用后效果话语）。CONTEXT 若需补归属句
同笔。

裁定方声明的边界：读过 `service_stage_content.go`／`service_stage_rules.go` 全文、UC-PS-004/006
相关行、PC CONTEXT 词条；**未读** 0005/0006 既有声明迁移与 UC-PS-003 正文。裁的是归属方向；
实现中若与 0005/0006 既有族形状冲突，报回再仲裁。

### 3.3 规则包正文（0013，视 D-3 而定）

`AcceptanceRulePackage` = `version + applicability(五维) + rules(五分区)`。模式本身直白：
父行存五维适用性，子表逐条 `(category, rule_reference)`，`category` CHECK 镜像 `RuleCategory` 五值封闭集
（`MINIMUM_INGRESS_IDENTITY` / `SHIPMENT_INVARIANT` / `PRODUCT_AND_CONTRACT_DOCUMENT` /
`REGULATORY_SOURCE_DOCUMENT` / `CROSS_FIELD_CONDITION`）。`AssembledRule`
「结构上没有地方放阈值」，表上同样只有分类与引用两列，这一条自然成立。

卡住的不是模式，是**族归属**：`RulePackageApplicability` 的五维（产品、合同、法人、范围、期间）
是**选择信息**——它说的正是「哪些包适用」。今天 `ResolveCommercialBasis` 完全不看它，只按
`version.scope` 与选用区间选包。所以：

- 若五维**不参与**选择 → 规则包正文纯属族 B 点读，本表按 3.1 的形状建即可。
- 若五维**参与**选择 → 它必须进 `CommercialRegistry` 与 `ViewRevision`（第 1 节判据），
  候选过滤逻辑要改，`AT-PC-021`「同一范围两个合同同时命中」一路的语义随之变化。

这是领域决定，不是持久化决定，且方向不同则表的族归属不同。

**D-3 已裁（MCP-1）**：B7 按「**不参与**」建表——如实反映当前解析行为。五维字段**照存**
（存内容 ≠ 参与选择）；「要不要参与」列为显式未决交领域建模，日后改判走新迁移，现在不预支。
规则包正文因此确定落族 B 点读。

---

## 4. 未配置格

家族先例三处：ADR-0052（网络证据，`configured bool` 第三格）、ADR-0054（接受前控制策略视图「未配置」格）、
ADR-0055（业务端点 Intake「未配置即拒」）。本切片新增的每个装载口都照这三格走，一格不合并。

| 装载口 | 在场 | 未配置 | 读取失败 |
|---|---|---|---|
| 族 A 四册（经 `LoadForScope`） | 行在册且版本生效 → 进 `CommercialRegistry` | **不设独立格**，见下 | error 上抛，编排译`权威不可读` |
| 合同正文点读口 | 父行在场 → `CustomerContract` | 无父行 → `found=false` | error，等依赖恢复 |
| 阶段内容三口 | 父行在场 → 对应 content | 无父行 → `found=false` | error |
| 规则包正文点读口 | 父行在场 → `AcceptanceRulePackage` | 无父行 → `found=false` | error |

**族 A 为什么不设独立的「未配置」格，要专门讲。** 它是上表四个装载口里唯一的例外，
而例外容易被后来人当成漏掉的一格补上。

ADR-0034 已经裁过这一格该长什么样：「`CommercialRegistry` 可登记 `CommercialPricePolicy`；
只登记版本、不登记政策时，计价目的下的价格规则会落到`无适用依据`——**这是有意的**」。
ADR-0050 对产品册裁的是另一半：「产品缺席不使解析从`唯一解析`退化为`无适用依据`」——形态不可观察，
但解析照常成立。

也就是说，**四册的「没登记」已经各自有一个裁定过的下游答案**，而且两个方向还不一样。
在装载口上再加一个 `configured bool`，等于把已经分好的两种答案又压回一格，让调用方重新去分。
族 A 的诚实表达是「登记册交回它实际有的东西」，缺席由领域按各自已裁的规则译。

**「空册 vs 登记了空集合」逐表判定**（ADR-0052 Decision 四第二条）：

| 表 | 零行的含义 | 是否需要与「空集合」分开 |
|---|---|---|
| 族 A 四表 | 该版本没有登记这一册的内容 | 否——四册各自只有「有/没有」两态，没有「登记了一个空政策」这种东西 |
| `customer_contract_control_binding` | **要分**：零子行 + 有父行 = 合同没约定任何范围；无父行 = 合同正文未登记 | **是**，靠父子两表分（3.1） |
| 规则包规则子表 | 领域要求至少一行，零子行 = 坏数据 | 否，但零行要 error 不要静默 |
| 阶段内容三族子表 | 同上，`New*` 一律答 `Err*NotConfigured` | 否 |

一条通则：**凡领域允许「显式声明空」的，必须父子分表；凡领域要求「至少一行」的，零行走装载 error。**
两者在库上长得一样（都是零子行），只有配上父行的在场与否才分得开。

---

## 5. 族 C：本切片不建表的三种，以及判据不是 ADR-0053

产品—渠道映射、协议正文、信用政策正文，本切片**不建表**。判据只有一条：

> 三者今天既无消费方也无写入方——非测试代码里没有任何一处读它、也没有任何一处写它（输入包 §2.4 逐项核过）。
> 一张没有调用点的表，它的模式没有任何东西能证伪。

这条判据必须与 ADR-0053 的「不建表」**明确分开**，否则会被读成同一条理由而误用：

- ADR-0053 不建表，是因为**字段会是替租户拟的**——「没有一份真实网络定义在手，列出来的字段是替租户拟的，
  而模式一旦落库就按校验和固定」。
- 族 C 不建表，**不是这个理由**。三者的字段一个都不用拟：`ProductChannelMapping`、`SupplierAgreement`、
  `CreditPolicy` 都是已确认的领域类型，字段与封闭集现成。同理，本文族 A、B 建表也**不受 ADR-0053 约束**——
  模式取自已确认的领域类型，不是替租户拟的字段。**两处的「等」等的是不同的东西**：
  ADR-0053 等的是真实定义，族 C 等的是消费方。

本仓的既有做法与这条判据一致：0003 落地时 PS `active_rejection.go` 在场，0005／0006 落地时
PS `CommercialBasisAdapter` 在场。三张表都是随消费方一批落的，没有一张是先建了等人来用。

族 C 三者的落地时机，按各自消费方到位的那一票带走。信用政策另有一句现成的登记在案：迁移 0004 的注释
自认「信用政策尚无存储口」——那句话在本切片之后仍然成立，本文不改它。

### 5.1 还有三处根本不在这十项里，原因各不相同

评审第 7 条罗列的东西比这十项宽。三处点名的缺口本切片一格都不碰，但要说清各自卡在哪一层，
否则下一轮会有人把它们当成本切片漏掉的活：

| 缺口 | 卡在哪一层 | 出处 |
|---|---|---|
| 接受前财务控制策略**正文**（封闭九类的第 5 类） | **领域类型不存在**——比族 C 还早一步。`PreAcceptanceFinancialControlPolicyObject` 只作为枚举值在场 | ADR-0054 自认「`party-commercial` 侧连领域类型都还没有」，且明写「本记录不解决提供方表面」 |
| 客户服务规则版本（评审点名的「轨迹、赔付」商业半边） | **CONTEXT 有语言、代码无形状**——封闭九类里没有这一类，加它要先动 `CommercialObjectKind` | PC CONTEXT 有该词条；九类枚举无 |
| 网络使用资格 | **PC 语言里还没有这个词**，且明确**不是**服务产品形态（「它有自己的版本与有效期间，而形态是产品版本的内在属性」） | ADR-0050 Open question 一，点名为具名提供方缺口 |

三处的共同点是**先要领域建模，不是先要一张表**——顺序反了就会出现「表比领域类型先落地」，
而那张表的列只能凭空拟。三处都该走 `/domain-modeling` 进 PC 语言，之后才谈得上归族。
本切片把它们原样上交，不代拟。

---

## 6. 实施顺序

分批的判据是**每一批自己能验绿、且每一批都有消费方能证明它对**。

| 批 | 内容 | 前置 | 消费方证据 |
|---|---|---|---|
| **B1** | 0010 产品形态册 + `SaveServiceProduct` + `LoadForScope` 装第二册 | 无 | NR `eligibility.go` 走 `AdoptedBasis.ServiceProduct()`；今天生产闭包里形态必缺，落地后 `ErrServiceProductUnavailable` 分支才可能消失 |
| **B2** | 0011 区间更正册 + `SaveValidityCorrection` | B1（同一装载口）；**D-5 已裁** | ADR-0038「接纳更正必须使该范围的 `ViewRevision` 变化」可在真库上验 |
| **B3** | 0008／0009 价格与结算政策册 + 两个 Save | B1；**D-2 已裁** | PS `CommercialBasisAdapter` 的 `settlementTermsFor` 读 `AdoptedSettlementPolicy` |
| **B4** | 只读端口收窄（2.5 读侧） | B1..B3 落定 | 无新消费方，是隔离修复；**需频道占号** |
| **B5** | 0012 合同正文父子表 + 点读口 | 在途 0007 先落，避免同一拥有对象两笔迁移撞号 | SA 侧 PC 适配器（今天不存在，属另票）；在途件是它的姊妹口 |
| **B6** | 阶段内容声明族三表三口 | **D-1 已裁** | PS 三口换真，注释承诺的「三口同切」 |
| **B7** | 0013 规则包正文 | **D-3 已裁** | 视 D-3 方向定 |

B1 先行有个具体理由：产品册是四册里**唯一已经有真实消费路径**的一册（NR 翻译表 + ADR-0050 的
`ErrServiceProductUnavailable` 分支），因此它能最早证明「五查装载 + 回指版本壳」这套装载形状是对的。
形状一旦验过，B2／B3 就只是照抄。

每批的验证按并行会话规约：真库用例必须实跑（`-v` 下看 `PASS` 不是 `SKIP`），
全仓 `go test -count=1 ./...`，推之前在临时 worktree 上验已提交态并**推那个已验 SHA**。

草案号段（上表 B1=0010、B3=0008／0009、B7=0013）是落笔时的预估。落地顺序把 0007 收编后，
实际号段见 §8；本文不回头改上表，以免把设计论证里的引用一并改乱。

---

## 7. 对既有件的影响

| 件 | 影响 |
|---|---|
| `CommercialAuthority` 的「已知的收窄」注释 | 四册落地后逐条失效，随各批同笔改。该注释写「三册」，产品册是漏掉的第四册——改的时候一并补 |
| `PublicationRegistry` 的「本口先只承载版本册」注释 | 同上，随扩展改写 |
| 迁移 0004 注释「信用政策尚无存储口」 | 本切片不动它，仍成立（族 C） |
| PS `CommercialBasisAdapter` | 解析候选与 `ViewRevision` 内容基数变宽，`AT-PC-026` 一路的提交前失效检测行为随之变化——属**语义变化**，要有用例证明变宽后仍只在该失效时失效 |
| NR `eligibility.go` / `form.go` | 翻译表不动（全函数、默认报错不吸收）；`ErrServiceProductUnavailable` 从「必然」变为「可能」 |
| 在途 0007 与合同正文 | 拥有对象同为合同版本，两路并存不并表（语义不同：绑定=范围级适用哪份策略；声明=合同版本级要不要）。`ViewRevision` 归属见 open-decisions D-4。0007 已入 `861280b`，不再是在途件 |

---

## 附：与输入包的三处出入

输入包取证扎实，逐条复核后有三处要更正或补充，记在这里免得下一个人按原文写代码：

1. **`CommercialPricePolicy` 的构造签名比结构体宽。** 输入包 §2.1 第 6 行记为
   「version+direction+plan+scope+effective」，那是结构体字段。`NewCommercialPricePolicy` 实际还收
   `planDirection` 与 `conversion` 两个入参，判完即弃。这不是笔误层面的出入——**它直接决定价格政策册要多两列**，
   否则装载口重建不出政策（见 2.2 与 D-2）。

2. **阶段内容声明族的拥有对象在文档里就是未定的，不只是「没建模」。** 输入包 §2.4 把三口记为
   「读口是 PS 适配器内部协作者接口」，属实；但更要紧的一层是
   `CancellationAuthorityContent` 的注释原文写着「已生效**产品或合同**」——两个候选并列。
   这使它成为本切片唯一的**结构性阻断**（3.2 / D-1），而不是一件可以照着建表的活。

3. **`ViewRevision` 的价格政策分支缺租户过滤。** 输入包未及此。`ViewRevision` 派生时，结算政策与产品两支
   都判 `policy.version.tenant != tenant → continue`，价格政策那一支只判 `policy.scope != scope`，
   **没有租户条件**。今天登记册按租户装载因而触发不到，但四册落库、装载口变宽之后，
   任何一次「按范围装载」的口子都会让它变成可触发的跨租户串味。属既有件的问题，不在本切片地盘，
   记为 F-1 交 owner。**收口：F-1 已落 `1cb4074`。**

---

## 8. 收口清点（取证 `1fff679`）

输入包 §2.4 的十项是「领域件在位、无持久化面」。本切片只做机制半边：表、端口、装载口、未配置格。
**没有一份真实租户正文**；PAR-COM-* 实例行仍空，不能把「可发布、可装载的结构」说成「真实可配置完成」。

| 输入包项 | 族 | `1fff679` 上的落点 | 不是漏项？ |
|---|---|---|---|
| `ServiceProduct` 形态 | A | 0008 + `SaveServiceProduct`；随 `LoadForScope` 进册（`cce0cff`） | 已落 |
| `ValidityCorrection` | A | 0009 + `SaveValidityCorrection`；只增多条、行锁串行化（`90a90f7` / `50aed5e`，ADR-0056） | 已落 |
| `CommercialPricePolicy` | A | 0010 + `SavePricePolicy`（含发布期 `plan_direction` / `binding_conversion`）；绑定补丁 `81707fd`（ADR-0057） | 已落 |
| `SettlementPolicy` | A | 0011 + `SaveSettlementPolicy`（`1b764c3`） | 已落 |
| `CustomerContract` 正文 | B | 0012 父子表 + `CustomerContractContentView`（`08964b3` / `5c3d03a`） | 已落 |
| `AcceptanceRulePackage` 正文 | B | 0014 父子表 + `AcceptanceRulePackageContentView`（`74a6c35` / `1fff679`，ADR-0059）；五维照存不参与选择 | 已落 |
| 阶段内容三件 | B | 0013 三表三口（`6e4ccda`，ADR-0058）；PS 侧换真 | 已落 |
| `ProductChannelMapping` | C | 无表 | **本切片明确不建**（无消费方，不是漏批） |
| `SupplierAgreement` 正文 | C | 无表 | **本切片明确不建** |
| `CreditPolicy` 正文 | C | 无表；0004 注释「信用政策尚无存储口」在 `1fff679` 仍在 | **本切片明确不建** |

另两项不在这十项里、本切片也不代拟：接受前财务控制**策略正文**（领域类型仍不存在）；客户服务规则版本（CONTEXT 有语言、九类枚举无）。0007 是合同版本级「要不要」声明，已落 `861280b`，属族 B 点读、不进 `ViewRevision`。

读侧收窄：`CommercialAuthority` 只依赖 `CommercialPublicationView`（`26864d9`，F-2）。
装载口：`LoadForScope` 单查左连接装版本壳 + 族 A 四册。族 B 各走独立点读口。
F-3：装载时壳与正文件规则包引用都在场且不等 → 拒装；**双处形状未消灭**。
