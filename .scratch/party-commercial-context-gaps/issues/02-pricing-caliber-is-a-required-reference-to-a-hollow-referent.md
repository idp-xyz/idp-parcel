# 计价口径：引用是强制的，被引的那份商业价格政策却没有口径列

Category: chore
Status: draft
Blocked by: 无

## 先纠一条容易得出的错判

本票的初稿结论原本是「计价在自选口径、两个上下文之间没有接缝」。**取证之后这两句都不成立**，
记在这里是因为下一个人很可能重走同一条路：`internal/parcelpricing` 对 `internal/partycommercial`
确实是**零 import**，从这一条推出「没有接缝」是自然的，而它错了。

**这两个上下文之间的缝是用版本引用接的，不是用 import 接的**，零 import 恰恰是上下文边界
生效的样子，不是遗漏。缝在 `parcelpricing` 那侧**已经建好并且守着**（见下），漏的在另一头。

## CONTEXT 要求什么

`docs/domain/party-commercial/CONTEXT.md`「商业价格政策版本」词条，硬句原文：

> 它还声明计价所需的商业口径：汇率的牌价类型、取值时点规则和加点规则，以及销售方向的体积系数。
> 这些是商业约定而非承运商规则，`parcel-pricing` 按声明的口径解析取值，不自行选定口径。

同一节另有一条税务硬句：

> 商业价格规则必须声明含税、未税或税务不适用，并引用适用税务分类；法定税码、税率、税额和
> 发票结果仍由财税系统拥有。

Boundaries 一节也把税务分类依据点名给了本上下文（「……接受与主动拒绝授权、税务分类依据、
信用政策……的商业版本」）。

## `parcel-pricing` 那侧：已经建好，而且拒绝裸值（取证 `9d6063c`）

这一半做得很实，四层都在：

- **领域**：`ReferenceSeriesValue` 带一个可选的 `quoteBasis`。`valid()` 里两道——`quoteBasis`
  在场时其 `kind` 必须是 `ArtifactCommercialPolicy`，且 `kind == ReferenceSeriesExchangeRate`
  时 `quoteBasis` **不得缺席**。构造函数分两个：`NewReferenceSeriesValue` 与
  `NewQuotedReferenceSeriesValue`。
- 注释把理由写在那里：「燃油费率不需要口径依据：它的折扣系数写在卡上。汇率需要，而卡对它
  没有发言权。」以及「不接受未声明口径的裸汇率。一个没有口径的数字事后无从争辩，因为没人
  说得出它本该是哪个汇率。」
- **库**：`TestReferenceSeriesCheckConstraintsRejectImpossibleRows` 证库内 CHECK 同样挡住
  「一行裸汇率」与「只有口径 ID 没有版本」。
- **快照**：`evaluation_snapshot.go` 把 `quoteBasis` 写进评价快照并原样装回，重放不丢口径。
- **读面**：目录读面与 HTTP 行体都把 `quoteBasisId` / `quoteBasisVersion` 成对透出，燃油行
  不长出这两个键。

所以「计价自选口径」是假的：**它结构上就跑不出一个没有口径引用的汇率取值。**

## 缺口在被引的那一头

`quoteBasis` 指向一份商业价格政策版本。而
`migrations/party_commercial/0010_commercial_price_policy.sql` 建的表，列是：

    tenant_id, object_kind, object_id, version_label,
    direction, plan_ref, plan_direction, binding_conversion, policy_scope_ref,
    effective_starts_at, effective_ends_at, registered_at

对着 CONTEXT 那句逐项点：

| CONTEXT 要求政策声明的口径 | 册上 |
|---|---|
| 汇率的牌价类型 | **无列** |
| 汇率的取值时点规则 | **无列** |
| 汇率的加点规则 | **无列** |
| 销售方向的体积系数 | **无列** |
| 含税 / 未税 / 税务不适用 | **无列** |
| 适用税务分类引用 | **无列** |

领域侧 `CommercialPricePolicy` 同样没有这六项。**含税/未税/税务分类在全仓 Go 代码里零实现**
——只在 CONTEXT 里存在。

于是这条缝的形状是：**引用是强制的，被引的那份东西是空的。** `quoteBasis` 钉住了"哪份政策
声明了口径"，任何人顺着它去问"那口径是什么"，政策答不出来。合成夹具里的 `SYN-PRC-FX-POLICY`
是一个只有身份没有正文的引用，它证明了缝接得上，没证明缝那头有东西。

## 为什么这比"少六列"更要紧

**这三项直接改卖价。** 牌价类型（中行现汇卖出 / 央行中间价 / 平台自定）、取值时点（下单日 /
出账日 / 账期结束日）与加点规则，任意一项换口径都会改变客户实际被收的钱，而按 CONTEXT 这三项
是**商业约定**、必须由合同侧声明。今天这三项落在哪里？**落在给出 `ReferenceSeriesValue` 的
那个人手上**——序列登记时填进去的那个数字本身。口径没被声明，只是被那个数默认了。

`parcel-pricing` 已经把这件事挡在了它管得着的边界上（不接受裸汇率），**它挡不住的是引用指向
一份空政策**——那需要另一头有内容可校。这是本仓反复记的那个形状的又一面：**接错看着像接对**
（缝在、引用在、测试绿，而被引的一头是空的）。

**体积系数这一项还多一层不对称。** `parcelpricing` 的 `VolumetricFactor` 注释写着「卡上写明了
它——L4 写的是『体积磅 = 英寸计的长 × 宽 × 高 / 250』——所以本包从不自带任何系数」。对**采购
方向**这是对的，系数就在承运商价卡上。但 CONTEXT 说的是**销售方向的体积系数**是商业约定；
`plan.go` 里没有任何方向概念，也没有汇率那样的口径引用要求。所以汇率有守卫、体积系数没有，
两者在 CONTEXT 里是并列的一句话。

## 补与不补，各自的连带

### 若补

- **先裁形状再裁列**：六项是全部平铺成列，还是分成「汇率口径」「体积口径」「税务口径」三组
  各自成表/成 JSONB。牌价类型与取值时点规则都该入封闭词表（本模块已有先例：
  `commercial_price_policy_direction_closed`、`..._conversion_closed`），加点规则是数值+口径，
  形状与前两者不同。
- **新开迁移序号**，不改写 `0010`（`party_commercial` 当前最大 `0017`）。表 0 行，无清洗。
- **约束**：SELL 方向政策是否必须声明体积系数？含税/未税三值与税务分类引用是否「同在或同缺」？
  这两问要一并裁，否则补了列等于没补。
- **消费侧不需要新增 import**：`parcelpricing` 已经能拿到 `quoteBasis` 指向的政策身份，缺的是
  一条按该引用取回口径正文的路。这条路走端口还是走登记时冻结进序列，须裁——**后者更贴本仓
  已有纪律**（评价重放不现取）。
- 读面连带：管理台价格政策页可加口径栏。

### 若不补

- 需要有人明说：口径由序列登记方在登记时自行负责，`quoteBasis` 只作为**留痕**而非**校验依据**。
  若如此，CONTEXT 那句「`parcel-pricing` 按声明的口径解析取值，不自行选定口径」就该跟着改
  ——它现在读起来像是有一份可解析的声明。
- 税务那句同理：要么补，要么明说首发不表达含税/未税。
- 代价是**「已声明口径」这件事在系统里只有一个引用、没有内容**，而它看起来像是有的。

## 边界

本票**不改表、不建列、不写迁移、不动两侧领域模型**。特别地，**不动 `parcel-pricing`**：那一侧
按 CONTEXT 已经做对了，本票的缺口全在 `party-commercial` 这头。
