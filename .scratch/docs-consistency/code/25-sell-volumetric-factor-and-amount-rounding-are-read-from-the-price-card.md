# 25 PP SELL 方向的体积系数与金额取整：文档归商业价格政策，代码与演示种子只读价卡

Category: bug
Status: ready-for-agent——R2（上线后尽快）：SELL 评价已在生产路径上；PC 价格政策口径对 SELL 必填体积系数，PP 却只读卡上那一份，两处各自合法，合同政策上声明的那个静默不起作用。实施时两处登同值即可绕开（可靠，但全靠人），所以列 R2；租户若按合同政策调系数而不改卡，计费重量与金额就与预期不符。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无——2026-10-21 文档自洽 07 已裁（[ADR-0260](../../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md)）：**认价卡**。原 Blocked by：docs-consistency/07 先裁「SELL 评价的计价条款（金额取整、体积系数、汇率口径）从哪一份工件读」——即 audit/pc-pp.md H5 的 B 路与 audit/cross.md B2-02、B2-03 同一裁。
出处：[audit/pc-pp.md](../audit/pc-pp.md) H5（审计归 C，并注明 owner 若认定 SELL 卡自带系数与取整就是首发形态则改归 B）；同根的 [audit/cross.md](../audit/cross.md) B2-02（金额取整，B）、B2-03（汇率口径，B）在本票登记、不另立。审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，PC、PP 代码与种子在此期间未改。

## 现象

PP CONTEXT、PC CONTEXT 与 ADR-0107 都说销售方向的体积系数与金额取整属商业价格政策；PP 代码不分方向，一律读价卡上的系数与取整，`internal/parcelpricing` 里没有任何地方读 PC 价格政策口径。PC 价格政策口径只有税务、体积、汇率三格，没有金额取整。

## 证据

- 文档与决定：
  - PP CONTEXT「体积系数」：「采购方向的系数属价卡内容，销售方向的系数属商业价格政策」；「金额取整策略」：「采购方向的策略属价卡内容，销售方向的属商业价格政策」。
  - PC CONTEXT「商业价格政策版本」：「……以及销售方向的体积系数……`parcel-pricing` 按声明的口径解析取值」。
  - ADR-0107 决定一（同上，与体积系数两侧归属同构）。
  - 相反一侧（cross B2-02）：ADR-0190 三类把「价卡的金额取整声明」列为租户取值；ADR-0198 决定二「卡没声明金额取整，请求记`待判断`」；ADR-0179 决定三选 SELL 卡「不走 PC 价格政策的定价方案绑定」。
- 代码与种子：
  - PC `internal/partycommercial/domain/volumetric_caliber.go` 的 `NewVolumetricCaliber`：SELL 必须声明系数引用，BUY / INTERNAL 不得声明。
  - PP `domain.PricingPlanVersion` 不分方向携带卡自己的 `VolumetricFactor` 与金额取整；`domain/input.go` 的 `deriveVolumetricWeight` 只读 `policy.VolumetricFactor()`。
  - `scripts/demo-seeds/data/pricing/price-card-cn-sg.json`（`direction: SELL`）自带 `weightPolicy.volumetricFactor`（除数 5000）与 `amountRounding`（`HALF_UP`）。
  - PC `domain.PricePolicyCaliber` 只有 tax、volumetric、fx 三格。

## 三类

机制：SELL 评价从裁定的那一份工件取计价条款，另一处拒收或不再要求。产品策略：哪一份是权威（待 07 裁）。租户取值：系数、取整、汇率口径的具体值。

## 规模

M（裁为价卡：撤掉 PC 的 SELL 体积口径必填，改 PC 校验与登记；裁为商业价格政策：PP 的 SELL 评价读 PC 口径，SELL 卡拒收这几格，按 ADR-0179 决定三「两处同一笔改」一并改选卡，L）。

## 最小的诚实修法

两种裁向都要做到「同一格只有一处合法声明」：

- 认价卡：PC 的 SELL 体积口径改为不要求（或撤掉），已登的行如实标「不参与计价」；PP CONTEXT 与 ADR-0107 由 07 改。
- 认商业价格政策：PP 的 SELL 评价从 PC 口径取系数、取整与汇率口径；SELL 卡上出现这几格即拒收；PC 口径补金额取整一格。

## 要裁的

全部在 Blocked by 那一裁里。

## 裁向（2026-10-21，文档自洽 07）

[ADR-0260](../../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md) 取「认价卡」：体积系数与金额取整不分方向是价卡内容，换算用卡绑定的汇率序列（汇率口径的声明仍在商业价格政策上，由序列取值登记的口径引用指名）；按范围选卡不读价格政策的定价方案绑定。PP 不改。照此实施：

1. PC：`domain.NewVolumetricCaliber` 对三个方向一律不收系数引用（`SELL` 不再必填，带了即拒，判法与今天的 `BUY`、`INTERNAL` 相同）；受控批文与管理台表单的价格政策口径去掉体积一格；已发布的口径行原样留在册上，读面（价格政策目录）把那一格标「不参与计价」。若价格政策正文的规范化文档形状因此变化，照 ADR-0014 判换不换号。
2. PC：`FxCaliber` 注释里「消费方据以停在未决，不得读成零加点」那半句（缺加点一格时的读法）改为 ADR-0260 决定三的口径（评价不读政策正文、不执行取值时点规则、不加点）；`VolumetricCaliber` 注释里「销售方向没有承运商卡，只能由商业政策声明」那条理由与它的测试头注随第 1 条改写。
3. 种子：`scripts/demo-seeds/data/commercial/publish-batch.json` 的 `SYN-PRICE-RULE-CN-SG` 口径去掉 `volumetricFactor`，摘要随之重算；种子卡 `price-card-cn-sg.json` 不动。
4. NR：路由策略版本的比较价格政策一格（`ports.RouteStrategyDefinitionVersion.ComparisonPricePolicy` 一路，迁移与 `adapters/postgres/network_catalog.go` 同改）不再作换算依据——删掉，或保留为历史列并在登记口拒收新值；ADR-0148 决定四那一句已由 ADR-0260 部分停用。
5. 补用例：`AT-PC-062`（UC-PC-001，PC 侧拒收）；`AT-PP-013`（UC-PP-001）今天的行为已对，补 Covers 点名即可。在 `.scratch/at-coverage-inventory.md` 把 `AT-PC-062` 从④移入①、`AT-PP-013` 从②移入①。

