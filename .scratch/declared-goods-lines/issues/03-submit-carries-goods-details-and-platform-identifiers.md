# 03 提交链路：首方 API 收货物明细与平台号，落库，重放与冲突，接受基线保留

Category: enhancement
Status: resolved——2026-11-12 进 main
Blocked by: 02（作者一侧已提交 `1cd0528bc`，未进 main；按用户「提交，然后开工」叠在其上做，分支 `feat/goods-details-submit`，工作树 `idp-parcel-goods-domain`，基线 `1cd0528bc`）

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

## 完成记录（作者一侧，2026-11-10）

- 提交：分支 `feat/goods-details-submit` 上的 `bdab5a273`（叠在 02 的 `1cd0528bc` 之上；工作树 `idp-parcel-goods-domain`），未推送、未进 main。
- 落点：
  - 首方提交口与补充口收委托级 `goods`、`platformShipment` 与逐箱 `platformBoxLabel`、`packing`；包裹上旧的扁平货物字段去掉。货物明细对着客户包裹参考造一次进摘要，再经 `GoodsDetails.WithPackingParcels` 挪到成员上进命令；装箱分配错指到 `parcels[].packing`。补充口拒收箱标号。
  - 命令、聚合（提交版本带货物明细，委托带平台声明，建单、补充、重建对着成员核）、委托快照两段可缺 JSON（零迁移）；读回经 `RehydrateGoodsDetails`、`RehydratePlatformShipment`、`RehydratePlatformBoxLabel`，不再过接入的门。
  - 外部标识加「平台箱标号」一类：接受时形成关联（分配方与适用范围是销售平台），`已提交`期间算歧义信号且不按客户账户收窄，撤下时响声明撤除门铃；迁移 `0037` 放宽类型封闭集、加部分 GIN 索引；存不进册的读回箱标号不形成关联。
  - 管理台最小适配（`goods-draft.ts`）；首方契约文档补货物明细一段、形状规则与破坏性移除的后果。
- 判据：`AT-PS-181`、`AT-PS-182`、`AT-PS-183`、`AT-PS-186`、`AT-PS-187`、`AT-PS-188` 的接入、聚合与真库半边各有测试 ✅；一票 50 箱真库往返、接受后按任一箱标号找到那一件 ✅；同一载荷两次提交、现签成员不同而摘要相同 ✅；旧摘要不变（02 钉的常量照旧过）✅。
- 未做：平台货件号与平台参考号挂委托的关联——没有消费方，ADR-0374 决定七把按货件号查的读口划在另定；`AT-PS-189`（仓配平台备货）等仓配实现切片。
- 验证（含 PG，`IDP_PARCEL_POSTGRES_DSN` 指 55432 门禁库）：`go build ./...`、`gofmt`、`go vet` 通过；`./internal/parcelshipment/...`、`./internal/architecture/...`、`./migrations/...`、`./internal/platform/migrate/...`、`./cmd/...`、节点作业、关务、运输履约、全程追踪与异常、路由各包全部 ok，持久化包真跑不跳过。管理台三道门：`tsc` 0、`run-tests` 1319 过 0 败、`vite build` 0。全量 `scripts/test-full.sh` 未跑，留给推送方。
- 评审：`/code-review` 三轮，基线 `1cd0528bc`。
  - 第一轮（high）9 条：坏快照答成客户输入错、读回平台号又过入册门、注释说反、装箱分配错不指位置、货物明细解析两遍、契约形状规则与位置词、缺重放摘要与 50 箱测试，全改；「补充口不映射货物形状错」驳回，理由写在 `FormNewSubmissionVersionCommand.DeclaredGoods` 注释：摘要那一遍与命令这一遍出自同一份草案与包裹次序，前者过了后者必过，成员集合变了先撞 `ErrSubmissionBoundaryChanged`，走到它只能是装配错误。
  - 第二轮（medium）9 条：新重建工厂没进重建面名单（架构门红）、撤下门铃漏箱标号、歧义查询新支无索引、箱标号的信号错按客户账户收窄、货物读回过接入门、读回超界号写册会回滚、旧形重放后果没写、换挂不拒非单射、读回重复核分配，全改。
  - 第三轮（low）：无可处理发现。
- 留作记录、未改：客户包裹件号读回后同样可能超过入册门再写册，属既有代码，不在本票范围。
- 进 main 时同笔改：UC-PS-001「给开发的交接」货物明细与平台号两行、UC-PS-002 交接一行按实现现状改写（规则与实现的差距只写在 UC 交接）。
## 进 main 记录

- 2026-11-12 随 declared-goods-lines 链进 main，main 尖 `c5001b3de`；重放、改号（分支 0378、0380 落地为 ADR-0382、ADR-0383）、生成物与门禁的完整记录见 [25 号票](./25-an-incomplete-declaration-for-the-estimate-waits-for-customer-supplement.md)「进 main 记录」。
