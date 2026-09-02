# 结算登记册与 CONTEXT 硬句的三处出入（写侧待裁：补列还是改文档）

Category: chore
Status: resolved——三票已裁并已实施（ADR-0087，`63b6e19` 落文、`aeeb709` 落码），子票全 resolved；收口取证见文末

## 这三处是怎么被看见的

不是专门审出来的，是做[票 admin-skeleton-closure-batch/04](../admin-skeleton-closure-batch/issues/04-settlement-accounting-http-read-faces.md)
的**对栏裁定**时撞出来的：那张票要把 `settlement-accounting` 四张管理台页的栏目与登记册逐格对上，
对不上的栏逐栏定「撤栏／保留并标未登记／换栏」。对到一半发现，**对不上的栏几乎全是册级缺席**
——不是「这一行的实例还没到」，是登记册上就没有那一维。

这个区别要紧：四页接完仍是空册（写入方是事务链，事务链在接入渠道墙后面），而**册级缺席的那些栏，
等到册子有行了也不会长出来**。所以它们不是接线问题，是登记册与 CONTEXT 之间的出入。

票 04 已按「册级缺席即撤栏」把页面那一侧处置完了，**读面没有为任何一栏造值**。本目录三张票管的是
另一侧：登记册要不要补上这些维。

## 取证锚点

三张票的全部举证锚 **`35564ad`**（该 SHA 的 `.go` 与 `.sql` 与已验绿的 `16e2ddb` 逐字节相同，
两者只差一份票面 Markdown）。表结构引 `migrations/settlement_accounting/` 下各文件，
CONTEXT 引 `docs/domain/settlement-accounting/CONTEXT.md`。

**举证一律引符号名不引行号**：这些文件正在被多条线改，行号写下去当场就在腐。

## 为什么单开一个目录

不塞进 `admin-skeleton-closure-batch`：那是**管理台骨架页收口批**，做的是机制半边的读面与接线；
这三处是**写侧要不要补列**，性质不同，读面一侧已经在票 04 里收口了。混进去会让那张批的完成判据
变得依赖三个写侧裁决。

## 三票一览

| 票 | 出入 | 一句话 |
|---|---|---|
| [01](issues/01-customer-charge-does-not-fix-the-eight-confirmation-facts.md) | `customer_charge` 少七项 | CONTEXT 要求每条确认费用固定八项，册上直接成列的只有一项 |
| [02](issues/02-customer-charge-has-no-version-chain-while-supplier-cost-does.md) | 客户费用无版本链 | 同一条「不覆盖历史结果」，供应商侧有整条版本链，客户侧一行都没有 |
| [03](issues/03-operating-result-components-have-no-role-dimension.md) | 组成项无角色维 | 「审核应付与贷项按各自借贷方向分别计入一次」在册上验证不了 |

## 边界（三票共用）

- **本目录不改表、不建列、不写迁移、不动领域模型。** 三票都只写到「差什么、补与不补各自的连带」
  为止，改不改由人裁。
- 三处都**不属票 04 的范围**（那张票零设计裁决、只建读面），也不构成它的阻断——票 04 的读面按册上
  实有的内容照实转写，册子补不补列都成立，补了再加栏即可。
- 补列会动已施加的迁移，须按本仓惯例**新开序号文件重建约束**（先例：`pilot_governance` 0002、
  `visibility_exception` 0003、本模块 0012 撤销 0008 的例外支），不得改写已施加的迁移。
- 三张表当前**全部 0 行**（写入方在接入渠道墙后面），所以补列不需要数据清洗——这一点对三票都成立，
  是现在裁比以后裁便宜的唯一理由。

## 收口（2026-09-02 MCP-3 核）

三处出入**都按「补」裁并已实施**，本 spec 上文「三票均待人裁」「本目录不改表、不建列、不写迁移」
两句自 2026-09-01 起描述的是裁决**之前**的状态。裁决落
[ADR-0087](../../docs/adr/0087-settlement-registers-carry-the-facts-their-hard-sentences-require-checking.md)
（`63b6e19` 落文，`aeeb709` 落码，三条决定一并裁）。

逐票核对（判据是 [issue-tracker](../../docs/agents/issue-tracker.md) 的「Complete a parent」：
子票全 resolved 才收父票）：

| 票 | 状态 | 落地物 |
|---|---|---|
| 01 确认费用七项事实 | resolved | `ConfirmedChargeFacts` 类型 + 迁移 `0014_customer_charge_confirmation_facts.sql`（同在或同缺的耦合 CHECK、封闭收付方向）+ 端口 `ConfirmedChargeFactsView` |
| 02 客户费用版本链 | resolved | 迁移 `0015_customer_charge_adjustment_register.sql` 立 `charge_adjustment` + `RecordChargeAdjustmentHandler`（UC-SA-002）+ 唯一创建用例门 |
| 03 组成项角色维 | resolved | `ComponentRole` 封闭八值按口径分组 + `verifyPayableAndCredit` 写侧复验 |

**收的是本目录，不是这三件事的全部。** 三张票的 Comment 各自记了「未做且有意未做」，都在本目录
范围之外，别当成随本 spec 一起收掉了：

- 三个用例的**生产装配未做**——`ConfirmChargeHandler`、`RecordChargeAdjustmentHandler` 至今只在
  测试里装配，未进 `cmd/parcel-api`。
- `ConfirmedChargeFactsView` **无生产实现**，那是实例半边：本仓无租户因而无册可读，不造默认值。
- 票 04 撤掉的经营页四栏、`customer_statement.adjustment_lines` 的角色退回、费用页调整栏——
  册子现在给得出了，加栏属读面那一片。

这三条不是欠账遗漏，是本 spec 开头「本目录只写到差什么为止」那条边界的正当结果。
