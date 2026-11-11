# 03 提交链路：首方 API 收货物明细与平台号，落库，重放与冲突，接受基线保留

Category: enhancement
Status: ready-for-agent
Blocked by: 02

依据：ADR-0373 决定四、五、七、八；ADR-0374 决定二、三、四；UC-PS-001 输入语义契约「服务请求」「声明包裹」「申报原始资料」三行，`AT-PS-181` 至 `AT-PS-183`、`AT-PS-186` 至 `AT-PS-188`；ADR-0140（首方契约）。

## 要做的

- 首方提交口 `/v1/shipment-requests` 收委托级货物明细、平台货件段与逐包裹的平台箱标号、装箱分配。建议形状如下，按 ADR-0140 的契约生成器定稿：

  ```
  goods: { contentCategory, contentCategoryNote,
           lines: [ { descriptions:[{lang,text}], quantity, unit, unitValue, lineValue, currency,
                      netWeight, netWeightUnit, commodityCodes:[{side:EXPORT|IMPORT, code}],
                      originCountry, sku, salesOrder:{platform, orderNumber, orderLine} } ] }
  platformShipment: { platform, shipmentId, referenceId }
  parcels[]: { …既有字段…, platformBoxLabel, packing:[{ line, quantity }] }
  ```

  包裹上原来的扁平货物字段（`goodsDescription`、`quantity`、`declaredValue`、`currency`、`originCountry`）去掉；首方 API 眼下没有生产客户端（`PAR-INT-13` 待提供），按 ADR-0140 的版本规则判断能不能在 v1 里直接改，不能就开新的主版本。
- 接入口给所有在场的申报格记来源标记「本单申报」。
- 形状错映射成 `400 MALFORMED_REQUEST`，带出错的位置。
- 提交版本、接受基线与客户原始资料版本保留货物明细（迁移放在 `migrations/parcel_shipment/`，编号接现有最大号）；平台三种号按外部标识关联登记，客户核对与歧义规则照旧。
- 重放与冲突照既有规则：同一来源请求身份与同一摘要是重放，内容不同是冲突；货物明细与平台号是 `PSC-1` 上的可缺键（ADR-0373 决定八），旧载荷的摘要不变。
- 外部标识加种类：平台箱标号（标识包裹，照客户包裹件号的做法——随首次提交声明在委托上，接受时形成关联，`已提交`期间算歧义信号，拒绝或撤回后不再算，适用范围是平台、恒范围相容）；平台货件号与平台参考号（标识委托，`ExternalIdentifierRelation` 的被标识对象要扩到委托）。ADR-0374 决定四；节点作业那一侧的歧义判断照 ADR-0195/0211 不另立判法。02 登进架构棘轮基线的几行随本票接上而出名单。
- 随产品带一份「FBA 头程」接单规则包参考配置（平台货件号、箱标号、装箱分配报全必填），租户经普通登记采用（ADR-0147）；必填执行器若尚未落地，参考配置先写成文件、执行器另立票（owner 复核建议 17）。
- 提交答复不加字段，逐件照旧带回包裹号（ADR-0223）。

## 完成判据

- 列出的 AT 逐条有测试：单元、HTTP 适配器与真库。
- 一票 50 箱的 FBA 货件真库提交通过，委托一份、包裹 50 件，货件号与参考号关联委托、箱标号关联各包裹（`AT-PS-188`）。
- `PSC-1` 时代保全的来源照旧能重放（迁移不改旧行）。
- `go vet`、受影响包测试，以及 `scripts/test-full.sh`（进 main 前由推送方跑）。

## 不做的

查阅视图在 04，管理台在 05，修订在 06，关务读口在 07。
