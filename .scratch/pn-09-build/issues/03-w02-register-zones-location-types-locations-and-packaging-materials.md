# 03 `PN09-W02` 第二部分：库区、库位类型、库位与包材

Category: enhancement
Status: in-progress（2026-11-19 认领，分支 `cursor/wf-w02-registers`）
Blocked by: 02

依据：[UC-WF-001](../../../docs/application/warehouse-fulfillment/UC-WF-001-REGISTER-WAREHOUSES-ZONES-LOCATIONS-AND-PACKAGING-MATERIALS.md)；[仓配履约 CONTEXT](../../../docs/domain/warehouse-fulfillment/CONTEXT.md)「主数据登记」；现行做法见 [dox 仓配履约](../../../dox/contexts/warehouse-fulfillment.md)「已实现：仓登记册」与票 [02](./02-w02-register-and-read-warehouses.md)。

用户 2026-11-19 说「继续」。分工同前：拆包、任务说明与验收由主会话做，代码与测试由 Sonnet 5.5 子代理写。

## 做法

四册都照仓那一册已经在 main 上的路加：领域、端口、应用层、迁移、PostgreSQL 适配器、报文译装、HTTP、`cmd/parcel-api` 装配。接入身份的适配器、开关关着时的处理器、问题体的形状都用现成的，不另写。

分三段交，每段是一条完整的路，做完一件就提交并推送：

1. **库区**。
2. **库位类型与包材**。
3. **库位**。

三段都做完、评审过、全量真库测试通过，才进 main。

## 四册共同的规则

与仓那一册相同，不重述。只列新增的：

- 四册的登记结果字面量与仓相同：`REGISTERED`、`ALREADY_REGISTERED`、`CONTENT_CONFLICT`、`NOT_ACCEPTED`。结果代数尽量共用一套类型，不抄四份。把仓那一册改到共用类型上可以做，对外的字面量与答复体不得变。
- `NOT_ACCEPTED` 新增两种原因：`REFERENCE_NOT_FOUND`（指名的对象在本租户查不到，带 `field` 指出哪一格）、`REFERENCE_DEACTIVATED`（新指名的对象已停用，带 `field`）。
- 新增一种结果 `NOT_CONFIGURED`：登记库位类型或包材时，本租户一个仓都没登记。状态码 `200`，不落册。登记仓之后原样重交照常受理。
- 判定次序：先按（身份，修订号）判重放与冲突；库位类型与包材接着看本租户有没有登记过仓；再核修订连续、不可改的格、所指的对象、内容。前面一格不成立就答前面那一格。
- 别的租户的对象一律按查不到答，不泄露是否存在。
- 开关关着时，这十二个端点都答 `403 WAREHOUSE_FULFILLMENT_NOT_ENABLED`。登记处的行都带 `warehouseFulfillment` 那一格。
- 每册三个端点：登记（登记册配置写面）、当前修订的分页列表、一个对象的修订史（后两个是主数据与运营查阅读面）。

## 各册

会话代决的字面量与取舍都可以推翻。

### 库区

- 身份（租户，仓编码，库区编码）。内容：名称、库区用途、是否停用。
- 库区用途是产品内置的封闭六种，字面量：`RECEIVING_STAGING`（收货暂存）、`STORAGE`（存储）、`PICKING`（拣选）、`DEFECTIVE`（次品）、`RETURN_STAGING`（退货暂存）、`OUTBOUND_STAGING`（待出库）。集外的词是坏载荷。
- 用途登记后不改：改了答 `IMMUTABLE_FIELD`，`field` 是 `usage`。
- 仓须已登记（停用的仓也算已登记）：查不到答 `REFERENCE_NOT_FOUND`，`field` 是 `warehouseCode`。
- 端点：`POST /warehouse-fulfillment-zone-registrations`，`GET /warehouse-fulfillment-zones`（可按 `warehouseCode`、`deactivated` 筛），`GET /warehouse-fulfillment-zone-revisions?warehouseCode=&zoneCode=`。

### 库位类型

- 身份（租户，库位类型编码），各仓共用。内容：名称、是否停用。产品不内置任何库位类型。
- 端点：`POST /warehouse-fulfillment-location-type-registrations`，`GET /warehouse-fulfillment-location-types`，`GET /warehouse-fulfillment-location-type-revisions?locationTypeCode=`。

### 包材

- 身份（租户，包材编码），各仓共用。内容：名称、外廓尺寸（长、宽、高与一个长度单位）、重量（数值与一个重量单位）、是否停用。
- 单位只收内置计量单位目录里的码。尺寸单位须是长度类，重量单位须是重量类。不在目录或类别不对答 `CONTENT_INVALID`，`field` 指出是 `dimensions.unit` 还是 `weight.unit`。
- 数值写法只认 `12` 或 `12.50` 这种十进制数，不带符号、指数与千分位，整数部分至多 9 位，小数部分至多 6 位。写法不对是坏载荷。数值为零答 `CONTENT_INVALID`。
- 数值按数值比，不按写法比：`12.5` 与 `12.50` 是同一内容。库里按精确十进制存，登记时不折算单位。回显去掉尾零的写法。
- 端点：`POST /warehouse-fulfillment-packaging-material-registrations`，`GET /warehouse-fulfillment-packaging-materials`，`GET /warehouse-fulfillment-packaging-material-revisions?packagingMaterialCode=`。

### 库位

- 身份（租户，仓编码，库位编码）：库位编码在仓内唯一。内容：所属库区、库位类型（可缺）、优先级（可缺，整数，数字越大越优先；不给就是没有，不取默认）、是否停用。
- 所属库区登记后不改：改了答 `IMMUTABLE_FIELD`，`field` 是 `zoneCode`。
- 仓查不到答 `REFERENCE_NOT_FOUND`，`field` 是 `warehouseCode`。所属库区须已登记且属于同一个仓，否则答 `REFERENCE_NOT_FOUND`，`field` 是 `zoneCode`。
- 带库位类型的，那个类型须已登记，否则 `REFERENCE_NOT_FOUND`，`field` 是 `locationTypeCode`。新指名的类型已停用答 `REFERENCE_DEACTIVATED`。沿用上一笔修订里的同一个类型、而它后来被停用了，照常受理。
- 不核库区与仓是否停用：停用只挡此后的移入目标，不挡登记。
- 端点：`POST /warehouse-fulfillment-location-registrations`，`GET /warehouse-fulfillment-locations`（可按 `warehouseCode`、`zoneCode`、`deactivated` 筛），`GET /warehouse-fulfillment-location-revisions?warehouseCode=&locationCode=`。

## 验收

UC-WF-001 的 `AT-WF-001` 至 `AT-WF-004`、`AT-WF-009`、`AT-WF-010`、`AT-WF-012` 里涉及这四册的部分。用例头注写 `Covers: AT-WF-…（哪一册的部分）`。

每张实施票的两条：开关关着时其他上下文的答复不变；本票不消费、也不交出任何信封。

## 不在本票

- 受控批量口与登记 CLI；演示种子；管理台页面。
- 货主的客户系统查阅仓。
- 停用的库区与库位「不再作移入目标」（`AT-WF-007`、`AT-WF-008`）：那是入库与库内用例的事，还没有实现。
- 向一线作业端下发库区与库位。
