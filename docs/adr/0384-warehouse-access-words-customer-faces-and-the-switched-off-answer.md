# ADR-0384：仓配的客户面能力、运营决定与作业范围用一套词；登记仓不另开能力面；开关关着时的答法留给落端点的那张票

Status: Accepted（2026-11-18，Cursor 会话代决：用户要把仓配文档做完，原话「你能继续完整完成吗」。票 [warehouse-fulfillment/11](../../.scratch/warehouse-fulfillment/issues/11-write-the-customer-faces-and-the-frontline-warehouse-work-contract.md)。代接入身份 owner 与仓配履约 owner 决，越权风险点见文末，待接入身份 owner 复核。用户没有逐条确认。部分停用 [ADR-0310](./0310-first-party-customer-command-five-faces-launch-with-direct-customer-one-to-one-and-command-prefixed-request-keys.md) 决定一「五个保留前缀」里的「五个」。回答 [ADR-0375](./0375-warehouse-fulfillment-is-an-optional-capability-on-its-own-pn-09-slice-off-per-deployment-unless-switched-on-and-unconfigured-per-tenant-until-registered.md) 越权风险点 5 与决定四留给实施票的那一格：风险点 5 本记录取「不另开」；决定四的两种做法只写明各碰哪一句，不选定。本记录不动 `internal/**`。）<!-- 索引注记 -->｜**判据**：词按口各立，不并；作业范围加「仓」，一台设备仍只登记一个范围；从载荷收货主的只限新建或还没有货主的对象；开关关着时两种答法都不得是 404，选哪一种归第一张落端点的实施票。

Date: 2026-11-18

## Context

票 07 至 10 的用例把客户面能力、运营决定种类、作业范围种类与作业事实类型写成暂名，并写明定名归本票。`internal/accessidentity` 的作业范围种类今天只有 `NODE` 与 `OFFSITE`，一台设备只登记在一个范围上，请求的范围与登记不同即答 `ErrDeviceNotInUse`。节点与仓同址、共用手持机时，这一台设备进不了两个范围。

ADR-0375 越权风险点 5：登记册配置写与运营查阅读两族不分上下文，持有它们的操作者就能登记、停用任何一个仓。ADR-0375 写「第 6、7 两点在任何生产部署打开开关之前要有答案」，不含第 5 点。一个仓都没登记的租户，只有登记仓那一口照常受理。

ADR-0375 决定四：开关关着时仓配端点不提供业务答复。怎么答归第一张落端点的实施票：沿用「接入渠道未配置」，或按恢复动作另立一格。两种都不得折成 404。本票把两种做法各碰到的既有句子写明，不代替那张票选定。

ADR-0151 决定二写身份格只从信封与授权答复来，不从请求体收。用例里有几口是新建对象，或给还没有货主的对象指名货主，没有在册对象可以取。

ADR-0310 决定一把首方族的保留前缀定为五个。仓配的货主写口若裸用接入请求标识，会与委托提交落在同一个判重空间。

## 候选与反方

- **客户面按业务对象并成少数几项能力。** 反方：提交、告知更新与撤回的授予不同，并了就无法只给其中一口。否决。
- **同一种运营决定下的各个动作共用一个去重空间。** 反方：代录提交与代录告知更新是两笔决定，共用会让后一笔被前一笔的决定标识挡住。否决。授予仍是一种，去重按决定标识或按被指名的对象。
- **作业范围加一种「节点兼仓」。** 反方：一次请求就要同时判两套作业，`ErrDeviceNotInUse` 的不变量被拆开。否决。同址登记两台（可以是同一台物理机分两次登记，不能一次请求进两个范围）。
- **为仓配另开登记能力面，并把它定为打开开关之前的门。** 反方：ADR-0375 没有把风险点 5 放进打开开关之前。另开是以后的收紧，不是现在的门。否决。
- **开关关着时答 404。** ADR-0375 已经否决。两种候选都保持 403，见决定七。

## Decision

### 一、客户面能力一词一口

下列词进首方客户渠道的能力词表。中文是用例里的名字，右边是册上的字面量。读口共用「仓配查阅」，不按对象再拆。

| 用例里的名字 | 字面量 | 口 |
|---|---|---|
| 商品维护 | `GOODS_MAINTENANCE` | 货主维护商品 |
| 入库预报提交 | `INBOUND_PRE_ADVICE_SUBMISSION` | 提交入库预报 |
| 入库预报告知更新 | `INBOUND_PRE_ADVICE_ARRIVAL_NOTICE` | 告知更新 |
| 入库预报撤回 | `INBOUND_PRE_ADVICE_WITHDRAWAL` | 撤回入库预报 |
| 收货差异确认 | `RECEIPT_DISCREPANCY_CONCLUSION` | 对一份差异作结论 |
| 出库请求提交 | `OUTBOUND_REQUEST_SUBMISSION` | 提交出库请求 |
| 出库截单 | `OUTBOUND_CANCELLATION` | 截单 |
| 自备面单补交 | `OWNER_LABEL_SUPPLEMENT` | 补交自备面单 |
| 退货预报提交 | `RETURN_PRE_ADVICE_SUBMISSION` | 提交退货预报 |
| 回库处置 | `RETURN_DISPOSITION` | 货主提交回库处置 |
| 加工指令提交 | `PROCESSING_INSTRUCTION_SUBMISSION` | 提交加工指令 |
| 加工指令撤回 | `PROCESSING_INSTRUCTION_WITHDRAWAL` | 撤回自己提交的加工指令 |
| 商品换标指令提交 | `RELABEL_INSTRUCTION_SUBMISSION` | 提交商品换标指令 |
| 商品换标指令撤回 | `RELABEL_INSTRUCTION_WITHDRAWAL` | 撤回自己提交的商品换标指令 |
| 增值服务提交 | `VALUE_ADDED_SERVICE_SUBMISSION` | 提交不关联库存的服务 |
| 仓配查阅 | `WAREHOUSE_READ` | 下面「客户面读什么」的全部读口 |

代录、认领、处置、完结、建账导入、冻结、盘点、库存调整、加工与换标的运营发起，都不在客户面。待认领到货与建账导入单不上客户面。

客户面读什么：仓，商品与仓测，入库预报（含现行告知、累计实收与单据号），逐次收货，收货差异，出库请求，加工指令，商品换标指令，开箱记录，在库量、各项占用的数量、可用量、库龄与流水，退货预报，增值服务的提交与完成。库存读口首版不带库位、冻结的原因与操作者。盘点、库存差异、库存调整、待认领到货与建账导入不上客户面。销毁的流水另带货主授权的依据引用。

由收货事实转来的待认领到货，在原入库预报的读口上只标「另有到货待认领」，不带那件到货的内容。退件转来的不标在任何一张预报上。

告知更新是整组替换：没带的到货线索或货主参考号就是清空。成功答复回显现行那一版。现行的是后到的那一版，代录可以盖过货主刚交的；下发给作业端的也是这一版。

开箱形成的收货差异与收货确认形成的是同一种。结论口指名差异，不指名预报。开箱那一种在开箱记录上读，结论仍走收货差异确认这一口，途径一并带出。

### 二、运营决定一种一种授予，去重不按种

下列每种一个字面量，进 `operator_grant_decision_kind` 那道封闭检查。同一种底下带决定标识的动作，各用自己的决定标识，不共用一个去重空间。不带决定标识的，重放身份取被指名的对象。

| 种类 | 字面量 | 带决定标识的 | 重放取被指名对象的 |
|---|---|---|---|
| 商品代录 | `GOODS_RECORD_ON_BEHALF` | 代录修订 | — |
| 入库预报代录 | `INBOUND_PRE_ADVICE_RECORD_ON_BEHALF` | 代录提交、代录告知更新 | 代录撤回 |
| 收货差异结论代录 | `RECEIPT_DISCREPANCY_RECORD_ON_BEHALF` | — | 代录结论 |
| 入库预报完结 | `INBOUND_PRE_ADVICE_CLOSE` | — | 完结 |
| 待认领到货认领 | `UNCLAIMED_ARRIVAL_CLAIM` | 认领 | 撤销认领 |
| 待认领到货处置 | `UNCLAIMED_ARRIVAL_DISPOSITION` | 处置 | — |
| 建账导入提交 | `STOCK_IMPORT_SUBMISSION` | 提交 | — |
| 出库请求代录 | `OUTBOUND_REQUEST_RECORD_ON_BEHALF` | 代录新建 | 代录截单、代录补交面单 |
| 退货代录 | `RETURN_RECORD_ON_BEHALF` | 代录预报、代录处置 | — |
| 次品转良品 | `CONDITION_RESTORE` | 决定 | — |
| 加工指令 | `PROCESSING_INSTRUCTION` | 运营发起 | 运营作废 |
| 商品换标指令 | `RELABEL_INSTRUCTION` | 运营发起 | 运营作废 |
| 库存冻结 | `STOCK_FREEZE` | 冻结 | 解冻 |
| 盘点 | `STOCK_COUNT` | 创建 | 开始、完成、作废 |
| 库存调整提交 | `STOCK_ADJUSTMENT_SUBMISSION` | 提交 | — |
| 库存调整批准 | `STOCK_ADJUSTMENT_APPROVAL` | — | 批准、驳回 |
| 增值服务完成 | `VALUE_ADDED_SERVICE_COMPLETION` | 完成 | — |

批准职责规则是登记册配置写的对象，对象身份是租户，不进上表。登记册配置写已有的在线口与受控批量口照旧消费同一登记用例，批准职责规则走这一面，不新开批量口。运营决定首版只有在线口。建账导入的提交与演示种子都走这个在线口，不另做受控批量口。

### 三、作业范围加「仓」，一台设备仍只一个范围

作业范围种类加 `WAREHOUSE`（仓）。字面量进 `WorkScopeKind` 的封闭集。一台设备仍然只登记在一个作业范围上。节点与仓同址时登记两台：同一台物理机可以分两次登记成两台设备，一次请求不能同时落在两个范围。请求的范围与这台设备登记的不同，仍答 `ErrDeviceNotInUse`。

作业事实类型，字面量即词：

`RECEIPT_CONFIRMATION` 收货确认，`WAREHOUSE_MEASUREMENT` 仓测登记，`UNCLAIMED_ARRIVAL` 待认领到货登记，`INTERNAL_MOVE` 库内移动登记（上架、移库、拣选位补货与回库都是这一种），`FORM_CONVERSION` 形态转换登记，`CONDITION_CONVERSION` 状态转换登记，`SHORTAGE` 短少登记，`PROCESSING_EXECUTION` 加工执行，`RELABEL_EXECUTION` 商品换标执行，`UNBOXING` 开箱，`COUNT_ENTRY` 盘点清点，`PICK` 拣货，`OUTBOUND_CHECK` 出库复核，`PACK` 打包，`OUTBOUND_CONFIRMATION` 出库确认，`PICKUP_HANDOVER` 自提交付，`RETURN_RECEIPT` 退货入库。

### 四、从载荷收货主客户账户的口

下列口的货主客户账户是业务内容，向 `party-commercial` 核在不在本租户。查不到与越权探针同答。这些口在本记录接受之前不开真渠道。

商品代录，入库预报的代录提交，待认领到货的认领，建账导入的提交，出库请求的代录新建，退货预报的代录，库内各作业事实的行，次品转良品，运营发起的加工指令与商品换标指令，开箱，冻结，盘点的范围与清点行，库存调整的提交，不关联库存的增值服务的代录提交。

指名在册对象的代录（预报的告知更新与撤回、差异的结论、截单、补交面单、回库处置所指的那一件）货主取自对象。载荷里再带货主即坏载荷。

作业事实登记面上的操作者与运营决定面上的操作者是同一个操作者标识，批准职责规则可以把他们比。设备身份不是操作者，不拿来当提交人。

### 五、登记仓不另开能力面

仓、库区、库位、库位类型、包材、待认领到货的处置规则与批准职责规则，仍走登记册配置写。持有这一面的操作者能登记、也能停用任何一个仓。本记录不把「另开能力面」定为打开仓配开关的前提。要收紧，另立记录。

### 六、仓配货主写口各取命令前缀

来源请求键 = 前缀 + 去两端空白的接入请求标识。前缀：

`goods:`、`inbound-pre-advice:`、`inbound-arrival-notice:`、`inbound-withdrawal:`、`receipt-discrepancy:`、`outbound-request:`、`outbound-cancellation:`、`owner-label:`、`return-pre-advice:`、`return-disposition:`、`processing-instruction:`、`processing-withdrawal:`、`relabel-instruction:`、`relabel-withdrawal:`、`value-added-service:`。

这些前缀并入首方族保留前缀表，与 ADR-0310 原有的五个一起，提交口与送站预报口都不得拿来开头。仓配今天没有已存的来源请求键。实施票落地前再核一次已存的提交标识：有标识恰以前缀开头的，加上前缀之后那一笔不能原样重放，实施票列出来，不静默改键。

部分停用 ADR-0310 决定一「不得以五个保留前缀之一开头」里的「五个」。原五口的前缀与键算法不变。

### 七、开关关着时两种答法，本记录不选

两种都不折成 404，状态码都是 403，都不是业务 `outcome`。第一张落仓配端点的实施票选定一种，并在它自己的记录里向 ADR-0055 与 ADR-0222 挂指针。

- **沿用「接入渠道未配置」。** 碰到 ADR-0055 决定三「状态码取 403，不取 404」：那一格的恢复动作是去配置接入渠道，开关没开不是这件事。也碰到 ADR-0222「只有声明了那一面的行答复离开接入渠道未配置」：渠道打开而开关仍关时，闭合用例会把这一格看成已经离开未配置，从而把「没有业务答复」读成「有业务答复」。
- **另立一格，例如能力未启用。** 不改 ADR-0055 决定三那一格的恢复动作，只是旁边多一格，同样 403、同样不得是 404。碰到 ADR-0222 同一句：离开「接入渠道未配置」之后还要再认这一格，落在这一格仍然不是业务答复。闭合用例要加这一判，只加在仓配端点上。

### 八、准入词

各写口判准入范围时，能力词取决定一或决定二的字面量，事实类型词取决定三的作业事实类型。没有作业事实的运营决定与客户面命令，事实类型词就用该能力的字面量。登记册配置写沿用该面已有的能力词，不新造。这是 ADR-0180 决定四「词表归产品策略」在仓配上的一份。对象范围对照仍是租户取值，没登记答不在准入范围。某个租户怎样经 PN-08 启用仓配，仍是 ADR-0375 越权风险点 7，本记录不定。

## 对已接受记录的作用

- ADR-0310 决定一「不得以五个保留前缀之一开头」里的「五个」，扩成首方族保留前缀表（决定六）。原五口不变。
- ADR-0375 越权风险点 5 由决定五回答，无条款停用。决定四的选定仍归实施票；决定七只写明两种做法各碰哪一句。

## Consequences

- 词进 `internal/accessidentity` 与迁移里的封闭检查，归接入身份的实施票，不在本记录里改代码。每加一种都要重建那道检查；本记录一次列全，不分批。
- 首方契约的仓配一节只引用本记录的词与用例的结果，不另立口径。
- 一线作业端下发哪一版告知，取现行那一版，含代录盖过货主的那一版。

## 越权风险点

1. 登记仓不另开能力面（决定五）：持有登记册配置写的人能停用任何一个仓。归接入身份 owner。本记录不把它做成打开开关的门。
2. 从载荷收货主的口（决定四）在本记录接受之前不开真渠道。查不到与越权探针同答，仍可能把「没有这个货主」说成同一种失败。归接入身份 owner。
3. 开关关着的答法未选（决定七）。归第一张落端点的实施票，以及 `parcel-api` owner。
4. 一台设备不能兼节点与仓（决定三）。同址要两台登记。归接入身份 owner 与仓配履约 owner。

## Links

- [ADR-0375](./0375-warehouse-fulfillment-is-an-optional-capability-on-its-own-pn-09-slice-off-per-deployment-unless-switched-on-and-unconfigured-per-tenant-until-registered.md)：决定四、越权风险点 5
- [ADR-0310](./0310-first-party-customer-command-five-faces-launch-with-direct-customer-one-to-one-and-command-prefixed-request-keys.md)：决定一的保留前缀
- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：决定二，身份格不从请求体收
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：决定三，403 不取 404
- [ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)：闭合用例那句「只有声明了那一面的行答复离开接入渠道未配置」
- [ADR-0180](./0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)：决定四，词表归产品策略
- 票 [warehouse-fulfillment/11](../../.scratch/warehouse-fulfillment/issues/11-write-the-customer-faces-and-the-frontline-warehouse-work-contract.md)
