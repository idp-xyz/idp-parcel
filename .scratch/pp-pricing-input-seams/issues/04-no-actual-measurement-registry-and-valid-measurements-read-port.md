# `node-operations` 实际测量登记册（追加不覆盖）+「仍有效的实际测量」只读口：NO 今天连登记册都没有，`parcel-pricing` 造快照的「实重 / 尺寸」实测那一源指不到

Category: enhancement
Status: draft——**2026-09-15 18:4x 通道 4 按通道 1 房务单裁决（task-2d1616ed「四-1 pp-seams/04：立，你立」）立票**：只转记 [02](02-billable-weight-actual-measurement-and-declared-dimensions-read-port.md) 裁决 1 / 2 / 4 / 6 与 [spec](../spec.md) 子票表 04 行、「不在本目录」NO 登记入口一句已写定的口径，不自造新裁决；02 裁决 1 没写定的列在「待裁」节，归 NO owner。只写票面未动代码；取证锚 main `0f433022`（`git ls-files migrations/node_operations` 仍止于 `0004`）
Blocked by: 无（02 裁决 1 拆出；PS 半边 02 已进 main `c657a5e7`，两半互不阻）

**用词**：本票只写「实际测量」（NO 的词，原始量：重量 / 尺寸 / 数量的测量事实）与「仍有效的实际测量」；**不写**「当前有效实测」当本票产出——那是派生结果，02 裁决 1 定它不由本票派生。「计价重量」归 PP、「客户 / 供应商计费重量」归 SA（GLOSSARY 已定，02「用词」节同）。

## 从哪里拆出来

02 原是「实重 / 尺寸两源都没有读口」一票，2026-09-14 22:1x 通道 1 代裁（NO·PS owner 口径）裁决 1 把它拆两半：PS 半边（申报重量 / 尺寸只读口，事实已在库上、差一只口）留在 02，2026-09-15 11:5x 进 main；NO 半边（实际测量登记册 + 仍有效实测只读口，今天连登记册都没有）拆成本票。拆的理由（02 裁决 1 原句）：「两半地盘、owner、体量都不同……合在一票会让小的等大的」。spec 子票表 04 行与「不在本目录」节各留一句给本票，本票是那两句的去处。

## 缺口（转记 02「缺口」NO 半边，钉 `db480695`；逐条可重跑）

- `git grep -n -i -E 'measure|weight|dimension' -- internal/nodeoperations/ports/ internal/nodeoperations/domain/ migrations/node_operations/` → 零命中（exit 1）。
- `git ls-files migrations/node_operations` → `0001_reception` / `0002_collaboration_execution_consolidation` / `0003_consolidation_source_provenance` / `0004_parcel_containment_indexes`；没有测量表（本票立票时于 `0f433022` 复核仍是这四份）。
- `git ls-files internal/nodeoperations/domain | grep -v _test` → 节点收寄 / 实物控制 / 集运单元 / 集运作业事实 / 关务协作 / 查询作用域，没有测量对象。
- `git grep -n 'registry=measurement' -- internal/nodeoperations/adapters/http/query_node_operations_records_test.go` → 用例头注「测量与交接证据两区在存储上没有登记册，它们的名字也在封闭集之外：没有表就没有读法」——NO 自己已经如实记了。
- NO `CONTEXT.md`「实际测量」「当前有效实测」两词条与生命周期「当前有效实测和当前位置」在代码上零落地。
- PP 侧要什么：`internal/parcelpricing/application/form_evaluation_from_request.go` `missingInputReadPorts` 第二条点名 `node-operations / parcel-shipment: no read port for measured or declared actual weight and dimensions (pricing weight)`；PS 那半已由 02 的 `ports.DeclaredMeasurementView` 接上，NO 这半仍空。

## 语言从哪里来（转记 02）

- NO `CONTEXT.md`「实际测量」：「节点对明确作业实物在特定时间、位置和作业依据下取得的重量、尺寸、数量或其他物理测量事实。实际测量不可覆盖；当前有效实测依据有效性、对象范围和业务规则派生。`parcel-pricing` 依据计算目的和已解析商业依据形成计价重量，`settlement-accounting` 再形成客户或供应商计费重量的财务采用」。
- NO `CONTEXT.md` 规则：「每次实际测量必须保留对象、测量项、结果、单位、发生时间、位置、来源和作业依据」；「当前有效实测按明确业务规则从仍有效的实际测量派生。客户声明、监管申报、客户计费重量和供应商计费重量均不得覆盖实际测量，节点也不形成最终计费重量」。
- GLOSSARY「当前有效实测」（所有者 NO；避免使用：最后一次称重、客户申报重量、计费重量）。
- `docs/domain/CONTEXT-MAP.md` `settlement-accounting → parcel-pricing` 那条边：「……节点作业 / 小包托运的实重尺寸……那一侧立」。

## 形（02 裁决 1 已定，照写；作者开工不重裁）

1. **领域对象**：照 NO `CONTEXT.md`「实际测量」规则逐字段——对象、测量项、结果、单位、发生时间、位置、来源、作业依据；**追加不覆盖**（「实际测量不可覆盖」）。
2. **登记册 store（写口）+ 迁移 `migrations/node_operations/` 新序号**（02 裁决 1 写 `0005`；`0001`–`0004` 不改；开工时以 `git ls-files migrations/node_operations` 为准取下一号）。
3. **只读口**：按（租户，对象身份——正式包裹身份或集运单元，各答自己的测量）答**仍有效的实际测量清单**（原始量：数值 + 单位 + 发生时间 + 来源引用）。
4. **不派生「当前有效实测」**：「当前有效实测按明确业务规则从仍有效的实际测量派生」那条规则里的「明确业务规则」（按来源 / 位置 / 设备定证明力）是租户的、属实例半边，机制不种「最近一次」之类默认；消费方拿到恰一条就用，多于一条且无已登记规则 → 在**消费方那侧**停「输入不可得：实测多于一条」。
5. **成员是集运单元时**（02 裁决 4）：本口按对象身份答该对象自己的测量（整袋重是单元自己的测量）；展开到成员逐件归 PP 消费侧（spec「不在本目录」）。
6. **两源并存按谁**（02 裁决 2）：不在本票——机制规则写进 PP `CONTEXT.md`「计价输入快照」、落 PP 消费侧票；**本票的口与 02 的 PS 口各答各的、不知道对方存在**。
7. **登记入口不做**（spec「不在本目录」）：测量怎么进登记册（节点作业事件 / 设备 / 人工）是登记入口题，本票只立登记册与只读口，入口另票归 NO，票面写明。

## 红线

- NO 不形成计价重量或计费重量、不替 PP 挑「用哪个重量」（NO `CONTEXT.md`「节点也不形成最终计费重量」）。
- 提供方只开只读口，不是写侧登记册的再导出：只读口方法集不含 `Save`，不暴露整条登记记录（spec「边界」）；PP 不 import NO `application`（ADR-0025），`internal/architecture` 边界门禁是判据。
- 追加不覆盖：同对象同测量项再登一条**不改前条**；「仍有效」的记法见待裁 1，裁前不预设「后登的顶掉先登的」。
- 未确认参数保持可配置或显式未决：当前有效实测的派生规则、测量项的封闭集、单位对表，任何一个由裁决或实例定，不预拟（spec「边界」）。
- 真实实测属实例半边；夹具全 `SYN-`，不写默认重量 / 默认尺寸。

## 完成判据（转记 02 判据 1 + 裁决 6「判据 1 移到 04」；待裁项定后写实；可 grep）

1. `git grep -n -E 'type \w+ interface' -- internal/nodeoperations/ports/` 多出两只：实际测量登记册 store（写口）与「仍有效的实际测量」只读口；只读口方法集不含 `Save`。
2. `migrations/node_operations/` 新序号迁移一份，`0001`–`0004` 零 diff；表字段覆盖「形」第 1 条八项。
3. 真库用例：按（租户，对象身份）取仍有效实际测量——一正（同对象两条都在清单里、原样两条，不合一）、一缺（无测量答空 / found=false，不造默认）、一隔离（他租户同对象身份答无）；追加不覆盖有用例钉住（再登一条不改前条）。
4. `internal/architecture` 边界门禁绿；`internal/parcelpricing/**`、`internal/parcelshipment/**`、`internal/transportfulfillment/**` 零 diff。
5. `docs/product/MECHANISM-INVENTORY.md` 干净检出重生成：NO 端口声明 +2、迁移 +1（数字以重生成为准）。
6. 待裁 1 若裁成「仍有效」要在 NO `CONTEXT.md` 长一句 → 随本票同笔落，只加不改既有硬句。

## 地盘

`internal/nodeoperations/{domain,ports,adapters/postgres}`、`migrations/node_operations/`（新序号）；`docs/domain/node-operations/CONTEXT.md` 仅当待裁 1 要长词条。不动 `cmd/parcel-api`（本票无在线入口）、`internal/parcelpricing/**`、`internal/parcelshipment/**`、`internal/transportfulfillment/**`、`internal/settlementaccounting/**`。

## 不在本票

- **登记入口**（测量怎么进登记册）——另票归 NO（spec「不在本目录」）。
- **两源并存按谁**——PP `CONTEXT.md` + PP 消费侧票（02 裁决 2）。
- **成员展开**（集运单元 → 成员逐件）与**单位对表**（NO 单位 → PP `WeightUnit` / `LengthUnit` 封闭集）——PP 消费侧（spec「不在本目录」）。
- **「当前有效实测」的派生**——实例半边规则，机制只交清单（02 裁决 1）。

## 待裁（02 裁决 1 未写定的部分；归 NO owner，裁前不动代码）

1. **「仍有效」怎么记。** 追加不覆盖之下，一条实际测量怎样变成不再有效——是登记一条指回原条的「作废 / 撤回」事实，还是实际测量从不失效、「仍有效」只是全部清单？NO `CONTEXT.md`「当前有效实测依据有效性、对象范围和业务规则派生」把「有效性」列为一维，但没说它记在哪、由谁改。决定只读口过滤什么、登记册要不要多一格。
2. **对象身份在登记册里的形。** 「正式包裹身份或集运单元」两种身份是裸引用一列（照 01 裁决「裸引用原样不带种类」），还是引用 + 种类两列？决定只读口的键与真库用例的夹具形。
3. **测量项是封闭集还是开放引用。** NO `CONTEXT.md` 写「重量、尺寸、数量或其他物理测量事实」——「其他」是留开放，还是首版封闭为重量 / 尺寸两项、其余等真需求？
4. **单位怎么带。** 自由引用串照 PS `MeasurementUnitReference`（02 裁决 5：对表归 PP 消费侧），还是 NO 自有封闭集？02 的先例是自由串原样交。

## 参照

[02](02-billable-weight-actual-measurement-and-declared-dimensions-read-port.md)「缺口」NO 半边、「语言从哪里来」、裁决 1 / 2 / 4 / 6、完成记录「逐条对裁决」1；[01](01-tf-charge-occurrence-member-object-read-view.md) 裁决（裸引用）；[spec](../spec.md)「用词」「边界」「不在本目录」NO 登记入口一句、子票表 04 行；NO `CONTEXT.md`「实际测量」「当前有效实测」与「测量、位置、状况与核对」规则节；GLOSSARY「当前有效实测」「计价重量」；`internal/nodeoperations/adapters/http/query_node_operations_records_test.go`（NO 自记「没有登记册」）；`internal/parcelpricing/application/form_evaluation_from_request.go`（`missingInputReadPorts`）；ADR-0025。

## Comments

- 2026-09-15 18:4x · 通道 4（task-2d1616ed，通道 1 房务单裁「四-1 pp-seams/04：立，你立」）：立票 ← 通道 4 · **能力边界：只读 02 / spec（01 裁决只经 spec 子票表 01 行），未读 NO 代码与 NO `CONTEXT.md` 正文**（本票引的 NO CONTEXT 句子全部转自 02「语言从哪里来」，未回原文核）；唯一新量的一件是 `git ls-files migrations/node_operations` 于 `0f433022` 仍止于 `0004`。「形」节七条全是 02 裁决 1 / 2 / 4 与 spec 既有句的转记，无一条是本票新裁；「完成判据」在 02 判据 1 之上按裁决 1「多于一条原样交」与红线补了三格可 grep 的写实（同对象两条原样、他租户答无、再登不改前条），是判据写实不是新裁，owner 不认可删格即可；「待裁」四条是 02 裁决 1 字面没覆盖、作者开工必碰的，列出不裁。spec 子票表 04 行「票面由 NO 半边作者按 02 裁决 1 自立」一句由此变旧，归推送方簿记时改。
