# 101 客户追踪视图只认正式包裹号，客户系统经首方渠道拿不到这个号

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg101（完成记录与评审记录见文末；隔离评审 0 阻断，非阻断已按评审记录处置）。此前 ready-for-agent——2026-10-19 分诊为 A（挡演示/上线），批次见 [triage-2026-10-19](../triage-2026-10-19.md)。此前 needs-triage——2026-10-19 Claude Code 子代理立（票 [96](./96-customer-tracking-view-has-no-first-party-customer-intake.md) 实现时发现）。
Blocked by: 无
地盘：VE 查阅口的定位（`internal/visibilityexception/adapters/http` 的 `CustomerTrackingViewIntake` 与查询键），或 PS 首方提交答复带回包裹号（`internal/parcelshipment/adapters/http`）；二选一或都做，看裁决。
出处：[UC-VE-008](../../../docs/application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)「输入和结果语义」查询标识一行（「包裹内部引用、授权客户参考号或版本化外部标识；标识只用于定位候选，不是访问凭证」）与主流程步骤 2；[ADR-0139](../../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四、五。

## 现象（钉 `79ad6122`，只记 `S`）

`GET /customer-tracking-view?parcel=<…>` 只认正式包裹号（`DPCL-…`）：VE 客户视图库以（租户，客户账户，包裹）为键，包裹就是这个号。客户系统经首方渠道能拿到的标识只有自己的客户包裹参考（`customerParcelReference`）和委托号——`POST /shipment-requests` 答 `201 SUBMITTED` 时只带 `shipmentRequestId`，不带包裹号。首方 API 上没有一条路让客户系统得知 `DPCL-…`。票 96 的演示是用操作者渠道 `GET /tracking-projections` 读出包裹号再交给客户令牌查的；真实客户系统做不到这一步。

## 三类

- 机制：查询标识按 UC-VE-008 三种之一定位候选——授权客户参考号要在本客户账户名下解析到唯一包裹，零匹配与「别的账户名下有」同答 `VIEW_NOT_FOUND`（AT-VE-151 不可枚举），多匹配走 AT-VE-152 的冲突格；版本化外部标识同理，谱系归 PS。或者：PS 首方提交答复带回逐包裹的正式包裹号，客户系统自己记映射。
- 产品策略：首方 API 对外承诺哪一种查询标识（ADR-0140 契约的一部分）。
- 租户取值：无。

## 要裁的

- 两条路不互斥。答复带包裹号改的是 PS 的答复形状（ADR-0140 契约），按客户参考定位要 VE 读 PS 的标识映射（跨上下文读口，或 VE 在派生视图时把客户参考随视图存下）。哪条先做、VE 是否存客户参考，归 VE / PS owner。

## 完成判据

- 客户系统只凭首方 API 上拿得到的标识，就能查到本账户包裹的视图；别的账户的同名参考不泄露存在性。
- 演示动线改走客户自己的标识，不再经操作者渠道读包裹号。

## 规模

M

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg101`）

授权：用户原话「你作为业务和系统专家自决……解决所有的问题」，并同意只做分诊 A 组。协调方代决走「客户系统经首方渠道拿得到正式包裹号」这条路；答复带回与按委托号读回的读面二者择一，由本票按读码事实定。

### 做了什么

- **ADR-0223**（`8f007d59`）：[首方提交答复逐件带回包裹号，客户追踪视图按它定位](../../../docs/adr/0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md)。它补 ADR-0140 的首方契约；ADR-0140 正文不改，只在 Status 行与 Links 加前向指针。`docs/adr/README.md` 已接受列表末尾加一行。
- **代码**（`37c6b339`）：
  - `application.SubmitShipmentRequestResult` 增 `Parcels()`，元素是 `SubmittedParcel`（内部标识，加可缺席的客户件号）。建成、重放、并发建单撞「已存在」都经 `naming`，从委托读。
  - `shipmenthttp` 的提交答复增 `parcels`。
  - 无迁移、无新端点，装配不动；`cmd/parcel-api` 的生产文件没改，只在 `assemble_customer_client_test.go` 加断言。
- **用例与演示**（`03e9d0e8`）：
  - UC-PS-001 增 `AT-PS-101`–`105`，步骤 10 引 ADR-0223；UC-VE-008 查询标识一行引 ADR-0223。
  - 演示动线「客户追踪视图」一节与 `scripts/demo-seeds/README.md`「客户追踪视图」那一条改为从首方提交答复取号，不再经操作者渠道读。
  - `submit-one-shipment.sh` 断言答复带回草案那一件、与委托详情的声明包裹对上，末行打出包裹号。`seed.sh` 没动。
- **立票 119**（`ca329599`）：[按客户自己的包裹参考直接定位视图](./119-customer-tracking-view-locates-by-the-customers-own-parcel-reference.md)，协调方代决分诊为 C。
- **实走记录**（`2dc54a39`）：记在演示动线同一节。
- 号段 120 没用上。

### 代决

1. **走答复带回，不开读面。** 读码表明号在答复形成时已在委托上：
   - 首方接单入口译命令时按草案次序现签内部标识，建单立成首个提交版本的成员；
   - 接受不换号（PS CONTEXT「保持永久内部标识」）；真实拆分或合并另起新号、原号留作历史身份（GLOSSARY「包裹内部标识」），见越权风险点 6；
   - 受控补充只许成员集合等值，成员重组是另一次提交。
   所以不需要「受理后才签」那条备选。
2. **形状。** `parcels[]`，每件 `parcelId` 加 `customerParcelReference`。
   - 后者与草案那一格同名，客户系统按同一个名字配对。
   - 前者沿用 PS 委托详情 `declaredParcels[].parcelId` 的叫法，不随 VE 查询参数叫 `parcel`。
3. **在场条件。** 与 `shipmentRequestId` 同在同缺；没建成委托的答复整键不出现，不给空数组。
4. **号从委托读，不从命令读。** 接单入口每次都现签新号；从命令读，重放会交回一组从没落库的号。
5. **次序取首个提交版本。** 受控补充后成员次序可以换，取当前版本会让重放在补充前后次序不同。契约是按 `customerParcelReference` 配对，次序是附带承诺。
6. **没声明件号的件整键缺席。** 不拿空串或内部标识顶替客户参考。首方口每件必有件号，缺席只出自不经首方口的提交。
7. **按客户参考直接定位不做。** 去向票 119（C，要 VE、PS owner 裁定）。
8. **OpenAPI 不重生成。** 首方契约没有生成物：ADR-0140 的描述符与 `tools/api-contract` 还没落，`docs/api/generated/` 不存在。描述符立起时，提交端点照 ADR-0223 写进 `parcels`。

### 越权风险点

详述在 ADR-0223 文末，这里逐条点名：

1. 答复多一键是契约变更（PS owner）。首个兼容性基线发布前加键不破坏任何集成方。
2. 次序是附带承诺（PS owner）。
3. 接受前查视图答 `VIEW_NOT_FOUND`，与不存在不分（VE owner 会签）。客户系统分不出「还没派生」与「号不对」。
4. 号在接受前就交给客户（PS owner）。委托被拒绝或撤回时，这枚号永远查不到视图；`outcome` 仍是 `SUBMITTED`，不是接受。
5. 不经首方口的提交没有件号（PS owner），调用方只能按次序对。
6. 内部标识成了客户可见的契约键，而它只在同一物理服务件存续期间不变（PS owner；后继展示一半 VE owner 会签）。GLOSSARY 写内部标识「不要求」展示给客户，不是「不得」；真实拆分或合并后客户手里的号成历史身份，后继展示靠 VE 身份谱系（`AT-VE-162`）。
7. 决定三替 VE 许了诺（VE owner 会签）：VE 查阅口长期按这个号定位，UC-VE-008 查询标识一行随之加了一句。

### 用例

UC-PS-001 `AT-PS-101`–`105`，各自的 `Covers:` 落在：

- `internal/parcelshipment/application/submit_shipment_request_parcels_test.go`：应用层各条；
- `internal/parcelshipment/adapters/http/submit_shipment_request_parcels_test.go`：传输层键名、整键缺席、重放逐字节相同；
- `cmd/parcel-api/assemble_customer_client_test.go`：真库，真首方接单入口，`DPCL-` 号即落库成员，重放同号。

### 验证

- `go build ./...`、`go vet ./...`、`gofmt -l` 干净。
- `scripts/test-full.sh ./internal/parcelshipment/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./tests/bentocontract/... ./internal/architecture/...` 全绿；`tools/mechanism-inventory` 模块 `go test ./...` 绿。没跑全量，没重生成 `docs/product/MECHANISM-INVENTORY.md`。
- 变异核对：把次序改成取当前提交版本，`AT-PS-105` 红。
- 实走（`ca329599`，只记 `S`）：
  - 环境：一次性库 `dcg101_demo`，`seed.sh` 退 0；私有 Dex 发首方令牌。
  - 提交：`submit-one-shipment.sh` 答 `201 SUBMITTED`，带回 `DPCL-BMPF4LQNQEGK24IQLKSW4L6HHA` 配客户参考；原样重发答 `200 EXISTING_RESULT`，同号。
  - 查视图：委托`已接受`后，拿答复里的号查客户追踪视图，答 `200 CURRENT_VIEW`；拿客户参考查答 `VIEW_NOT_FOUND`。
  - 收尾：已停进程、删容器，`DROP DATABASE … WITH (FORCE)`。

## 评审记录（2026-10-19，隔离评审；修补者为作者子代理，分支 `claude-dcg101`）

阻断 0 项。非阻断里标「进 main 前修」的，在分支上修了其中三项；MECHANISM-INVENTORY 一项按并行会话约定留给进 main 那一步。其余三项：VE 会签、件号两端空白两项顺手修了，合并冲突一项落文字。修订提交 `f9559b88`（ADR）、`a38afb46`（注释）、`694e6fd9`（票 119 与分诊表）。

### 已修

1. **「号以后不变」只引了 PS CONTEXT 一句**：漏了「包裹内部标识在同一物理服务件持续存在期间保持不变」，也漏了 GLOSSARY「包裹内部标识」一条的两句：「真实拆分或合并后的新物理服务件使用新的内部标识」「内部标识不要求展示给客户或外部渠道，也不等于外部标识」。
   - ADR-0223 Context 那一条改为「号以后不变，真实拆分或合并除外」，并补引上面两处原句。另起一条「号交给客户不越 GLOSSARY」：「不要求」不是「不得」；号交给首方客户之后仍是内部标识，不进外部标识关联册。
   - 越权风险点增 6：内部标识成了客户可见的契约键，又只在同一物理服务件存续期间不变。拆分合并后客户手里的号成历史身份，后继展示靠 VE 按身份谱系（UC-VE-008 `AT-VE-162`），本记录不核也不加。PS owner 复核，后继展示一半由 VE owner 会签。
   - `SubmittedParcel` 注释补「真实拆分或合并除外」。Links 补 GLOSSARY、`AT-VE-162` 与 PS CONTEXT 那一句。
   - 完成记录「代决」1 与「越权风险点」同步。
2. **对别处的计数**（AGENTS.md「改文档」：计数与行号同构）：
   - ADR 决定一不再写「三种归属挡下、两种关联拒绝」，改为「判据只看 `shipmentRequestId` 在不在，不逐个结果列举」，后面例举结果名，不带数。
   - Context 改为「「包裹内部引用」是其中之一」。决定二、Consequences、README 条目去掉「三条路」。决定四「UC-VE-008 另两种查询标识」改为点名授权客户参考号与版本化外部标识。「票面列了两条路」「先走第一条」改为按名指。
   - `naming` 注释去掉「三条路」。
   - 票面：README「第 9 条」改为按条目名「客户追踪视图」指；「落在三处」「应用层五条」去掉数。
   - 票 119：「列了三种」「只承诺第一种」「第二种查询标识」改为点名；「决定四第一条」改为引那一条的原句。
3. **票 119 的 Status 与 Blocked by**：
   - Status 改为 `needs-triage`，C 的分组写进分诊表，票面写明以分诊表为准。照分诊表的约定，非 A 组票面状态不动。
   - Blocked by 改为「用户决定（VE、PS owner：…）」，并补上票面「要裁的」第二问（客户参考在一个账户里是否唯一）。
   - 分诊表在「49、57、58、76」那一行之后追记 `119 | C` 一行，注明是票 101 实施时追记。核过 `claude-dcg103`、`claude-dcg105` 与 main 自分诊以来都没动这份文件，不会撞。完成记录原越权风险点 6（「分诊表没加 119 那一行，留给协调方」）随之撤下。
4. **决定三的 VE 会签只覆盖第 3 点**（评审没标进 main 前修；改的是 Status 行与风险点，趁 ADR 进 main 前改最便宜，就修了）：
   - Status 行改为「决定三（连同 UC-VE-008 查询标识一行的改动）与第 3、6、7 点同时待 VE owner 会签」。
   - 越权风险点增 7：决定三替 VE 许了诺。VE 将来换查询键，就是首方契约的破坏性变更（ADR-0140 Decision 五）。
   - README 条目改为「代 PS owner 裁定、VE owner 会签决定三」。
5. **件号回显的是去过两端空白的值**（评审没标进 main 前修，顺手修）：
   - 决定一「形状」加一条：回显的是委托上的声明值，不是请求的原样字节。接单入口已去掉单元格两端空白，号码字符一个不动，客户系统按自己送出的值去两端空白后配对。依据是 ADR-0184「越权风险点 2 的事实更正」定的入口纪律：委托只有这一个声明值。
   - `submittedParcelView` 注释与 README 条目同补。
   - 行为不改。改成回送原样字节，就得在委托之外另存一份请求原文，成了第二个声明值。

### 没采纳（在分支上）

- **`docs/product/MECHANISM-INVENTORY.md` 在分支末端过时**（parcelshipment 测试数随本分支两份新 `_test.go` 增加）：分支上不重生成。
  - 本票的派单要求不重生成；`docs/agents/parallel-sessions.md` 也约定由推送方在合并末端兜底重生成。
  - main 自 merge-base 以来也改过这份文件，在分支上生成的版本进 main 时照样要再生成一次。
  - 处置：协调方进 main 时在干净检出的合并末端重生成，随合并落下。

### 落文字、不改

- **与并行分支的合并冲突**：`git merge-tree` 显示，`claude-is03`（infra-speed/03 改写各 ADR 的 Status 行、把 README 索引改成生成）在 ADR-0140 与 `docs/adr/README.md` 上都冲突，`claude-is02`（ADR-0222）在 README 上冲突；与 origin/main 合并干净。
  - 后进 main 的一方解冲突。
  - is03 若先进、README 索引成了生成物，ADR-0223 那一行重跑生成器得到，不手补。ADR-0140 Status 行末的「补充」前向指针按 is03 的新样式重放。
- **复验**（修订后，钉 `694e6fd9`）：`go build ./...`、`go vet ./...`、`gofmt -l` 干净。`scripts/test-full.sh ./internal/parcelshipment/... ./cmd/parcel-api/... ./cmd/parcel-dispatch/... ./tests/bentocontract/... ./internal/architecture/...` 全绿。本轮只改注释与文档，没动行为，演示种子与 `submit-one-shipment.sh` 没动，不重走。
