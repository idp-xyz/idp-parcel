# 保价与声明价值作为可销售服务选项

Category: enhancement
Status: draft
Blocked by: 01, 02, 03

取证基线 `c9835bf`。

## 缺口

`声明价值`、`保价`、`保额`、`赔付限额`、`DeclaredValue` 在全仓（排除 `docs/reference`、`docs/archive`、`docs/prd`）**只有一处命中**，在[竞品公开事实台账](../../../docs/product/COMPETITOR-FACT-LEDGER.md)里，是引 Shippo 的公开事实。

现有的**保险追偿**不是它。那是出事之后运营企业向供应商、实际承运商或保险方主张的责任链，住在 `visibility-exception`（追偿事项、责任结论）与 `settlement-accounting`（应追偿金额、认可金额）。它回答「谁赔我们」，不回答「客户下单时买不买保价、保多少、上限多少、保费怎么算」。

## 为什么它该进

按[能力范围判据](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md#能力范围判据)：「必须能指出一类真实存在的承运或渠道商业模式需要它，且该模式在目标客户群里是常规形态而非边缘情形。」

指得出：跨境出口小包按声明价值加收保费、约定赔付上限，是小包产品的标准条款行，不是某一家的特例；Shippo 与 Easyship 都把保险作为独立变现轨道（台账 `S1` 已核实其按声明价值加运费的比例计费形态，引用时连核对日期一起引）。判据说它该进。

## 做什么

**先定形态，再谈落点。** 至少要答清这几件，它们决定对象归谁：

- 声明价值是**客户申报的事实**还是**经运营企业采纳的判断**？两者的版本化与更正路径不同。
- 赔付上限由**服务产品版本**携带，还是由**客户合同**携带，还是两者各有一层（产品给默认上限、合同可收紧）？
- 保价成立的时点是**委托接受**还是**正式承诺**？它影响取消与处置分支下保费的归属。
- 出险时它与既有的**客户索赔项**、**客户赔付**、**保险追偿**如何接——保价是赔付责任的**依据之一**，不是第四条并行的金额链。这一格接错会造出第二套赔付口径。

计价那一头的钩子已经在了：`internal/parcelpricing/domain/feature_condition.go` 的服务选项是**多值**类别特征（注释：「一个包裹可以同时带签名与……」），保费作为一个可被条件读取的服务选项能进现有规则模型，不需要新的价表族。

按形态答案落在 `party-commercial`（服务产品与合同侧）与 `parcel-shipment`（委托侧）的 `CONTEXT.md`，再改引用它们的 `UC-*`；若判为难逆转取舍则先走 ADR。

## 必须守住的一格

**任何费率、比例、上限金额一律不填。** 保费比例是渠道与保险方的商业条款，属实例半边，等真实价卡与协议。本票只做形态——判据本身就写着「形态决定支持什么结构，实例值永远等真实价卡」。

## 为什么排在演示之后

`Blocked by: 01, 02, 03` 不是技术依赖，是**优先级依赖**。保价是第一次商务会面必被问到的东西，但演示动线走不通的时候，多一个能力也没人看得见。三堵墙降完再做它。

做票人若判断本票可与三墙并行而不抢地盘，可提出解除阻塞边——它不碰 `cmd/parcel-api` 装配点，也不碰 `networkrouting`。

## 完成判据

形态四问各有答案并落进对应 `CONTEXT.md`；受影响的 `UC-*` 同笔更新；与既有索赔/赔付/追偿三条链的边界写明白，不新增第四条金额链；无任何费率或金额取值；`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿。

## 参照

`docs/domain/party-commercial/CONTEXT.md`、`docs/domain/parcel-shipment/CONTEXT.md`、`docs/domain/visibility-exception/CONTEXT.md`（索赔与追偿）、`docs/domain/settlement-accounting/CONTEXT.md`（赔付与应追偿金额）；`internal/parcelpricing/domain/feature_condition.go`；`UC-VE-007`、`UC-SA-007`；台账 `S1`。
