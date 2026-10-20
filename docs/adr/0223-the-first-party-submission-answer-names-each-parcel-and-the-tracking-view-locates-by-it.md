# ADR-0223：首方提交答复逐件带回包裹号——建成委托的答复带 `parcels`，每件配客户包裹参考；号从委托读，重放读回已签的、不重签；首方契约对客户追踪视图承诺的查询标识就是这个号

Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/101，协调方代决先走「答复带回号」这条路。补 [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 的首方契约：ADR-0140 Decision 八把端点字段留给各上下文 owner 与其用例，本记录定提交答复的一格与客户追踪视图对外承诺的查询标识。ADR-0140 正文不改，只在其 Status 行与 Links 加前向指针；不停用任何已接受记录的条款。越权风险点见文末，待 PS owner 复核，第 3 点同时待 VE owner 会签。）
Date: 2026-10-19

## Context

- 现象（钉 `79ad6122`，只记 `S`）：`GET /customer-tracking-view?parcel=<…>` 只认包裹的内部标识（`DPCL-…`）。客户系统经首方渠道能拿到的标识只有自己的客户包裹参考与委托号——`POST /shipment-requests` 答 `201 SUBMITTED` 只带 `shipmentRequestId`。票 96 的演示改用操作者渠道 `GET /tracking-projections` 读出包裹号，真实客户系统做不到这一步。
- 读码（钉 `4cf203fb`）：
  - 号在答复形成时已经在委托上。首方接单入口 `CustomerSubmissionIntake` 译命令时就按草案 `parcels[]` 的次序现签声明包裹的内部标识，建单把它们立成首个提交版本的成员。客户包裹件号按同一次序与之配对，随委托落下（ADR-0184 决定四）。
  - 号以后不变。PS CONTEXT 写明「声明包裹在委托接受时进入固定成员基线，并保持永久内部标识」。受控补充形成的新提交版本只许成员集合等值（domain 的 `supersessionMembers`）；成员重组走关联新委托，那是另一次提交、另一份答复。
  - VE 客户视图库以（租户，客户账户，包裹）为键，包裹就是这个号。[UC-VE-008](../application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md) 查询标识一行列了三种，「包裹内部引用」是其一。
  - 接单入口每次都现签一组新号。重放在来源身份上被认出，命令里那组号走不到建单（`mintInternalIdentities` 的注释）；并发建单撞「已存在」（ADR-0031）也一样。
  - ADR-0140 Decision 一的端点描述符与 `tools/api-contract` 还没落，`docs/api/generated/` 不存在。首方契约今天只以处理器的响应结构体存在，没有生成物要重生成。
- 票面列了两条路，不互斥：
  - 答复带回逐件的号，客户系统自己记映射；
  - 按客户参考号直接定位视图，VE 要读 PS 的标识映射，或在派生视图时把客户参考随视图存下。
- 协调方代决先走第一条。若号在答复形成时还不存在，就改成在首方渠道提供按委托号读回逐件号的读面。读码表明号已在，所以走答复带回，不另开读面。

## Decision

**一、建成委托的提交答复带 `parcels`。**

- **在场条件**：与 `shipmentRequestId` 同在同缺，即 `SUBMITTED`，以及指名了原委托的 `EXISTING_RESULT`。没有建成委托的答复整键不出现，不给空数组。这类答复包括：接入冲突、输入未受理、三种归属挡下、两种关联拒绝，以及重放一份从未建过单的输入。
- **形状**：每件一个对象。
  - `parcelId`：这件声明包裹的内部标识。
  - `customerParcelReference`：客户在这份委托里为这件声明的包裹件号，与草案 `parcels[]` 那一格同名。没声明件号的那一件整键不出现，不拿空串或内部标识顶替。首方口每件必有件号，缺席只出自不经首方口的提交。
- **次序**：首个提交版本的成员次序，即草案 `parcels[]` 的次序；受控补充之后重放也不变。契约是按 `customerParcelReference` 配对，次序是附带承诺。
- 答复只回显这份委托自己声明的东西，不涉别的客户，`UC-PS-001`「不得泄露其他客户的存在、业务量或内容」不受影响。

**二、号从委托读，不从命令读。** 建成、重放、并发建单撞「已存在」，三条路都从那份委托读成员与件号。重放答复与首答逐件相同；接单入口这一次现签的号不出现，不重签，也不落库。不加迁移。

**三、首方契约对客户追踪视图承诺的查询标识是这个号。** 客户系统拿 `parcels[].parcelId` 作 `GET /customer-tracking-view?parcel=` 的值。

- 标识只定位、不是凭证：VE 仍按令牌绑定的客户账户核归属（UC-VE-008），别的账户的同号与不存在同答 `VIEW_NOT_FOUND`，AT-VE-151 不变。
- 视图在委托`已接受`、初始路由投影形成之后才派生。在那之前，客户查自己的号也答 `VIEW_NOT_FOUND`，与不存在不分；VE 的答复代数不因本记录变。

**四、不做的，逐条写明。**

- 不按客户包裹参考或版本化外部标识直接定位客户视图（UC-VE-008 另两种查询标识）。那要 VE 读 PS 的标识映射，或随视图存客户参考，归 VE、PS owner；去向票 demo-chain-gaps/119。
- 不另开按委托号读回逐件号的首方读面。号在答复形成时已在。答复丢失时，按 UC-PS-001「调用方必须能够通过稳定请求关联查询原结果」：同一接入请求标识原样重发，答 `EXISTING_RESULT`，带回同一组号。
- 不改操作者族各读口，不改 VE。
- 不加 `/v1`，不改问题体。那是 ADR-0140 Decision 二、三的实施票。
- 不先造描述符。ADR-0140 Decision 一的描述符立起时，提交端点的描述符照本记录写进 `parcels`。

## Consequences

- 代码：
  - `application.SubmitShipmentRequestResult` 增 `Parcels()`，元素是 `SubmittedParcel`（内部标识，加可缺席的客户件号）；三条指名委托的路都经同一个 `naming`。
  - `shipmenthttp` 的 `submitResponse` 增 `parcels`。
  - 无迁移、无新端点，装配不动。
- 用例：[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 增 `AT-PS-101`–`105`，步骤 10 引本记录；UC-VE-008 查询标识一行引本记录。
- 演示：[演示动线](../design/synthetic-demo-journey-script.md)「客户追踪视图」一节改为从首方提交答复取号。`scripts/demo-seeds/submit-one-shipment.sh` 断言答复带回草案那一件，并打出号。
- 管理台不受影响：它按名读 `outcome` 与 `shipmentRequestId`，多一键不改它的读法。

## Alternatives considered

- **在首方渠道开一个按委托号读回逐件号的读面**（协调方给的备选）。否决：号在答复形成时已在，多一个读口是同一事实的第二条路；答复丢失靠原样重发拿回，UC-PS-001 已定。
- **按客户参考号直接定位视图。** 不否决，推后：票 119。
- **号放进答复，但从命令读。** 否决：重放会交回一组从没落库的号（决定二）。
- **`parcels` 用对象表（客户参考 → 号）。** 否决：没声明件号的那一件没有键，次序也丢了。
- **字段叫 `parcel`，与 VE 查询参数同名。** 否决：PS 自己的读口都叫 `parcelId`（委托详情的 `declaredParcels[].parcelId`），同一个上下文里同一个东西用一个名字。

## 越权风险点

1. **答复多一键是契约变更**（PS owner）。首方 API 尚未发布兼容性基线（ADR-0140 Decision 五），今天加键不破坏任何集成方；首个基线发布后，这一格进基线。
2. **次序是附带承诺**（PS owner）。若将来成员次序有业务含义（例如多件委托的件序号），另立。
3. **接受前查视图答 `VIEW_NOT_FOUND`，与不存在不分**（VE owner 会签）。客户系统拿到号立刻查会落这一格，要过一会儿再查；它分不出「还没派生」与「号不对」。要分开得在 VE 的答复代数里加一格，那会给枚举开口，归 VE owner；本记录不改。
4. **号在接受前就交给客户**（PS owner）。接受前的声明包裹不是正式包裹（PS CONTEXT「外部标识关联只在被标识对象成为正式对象时形成」一条），号此时只是一枚永久内部标识。委托被拒绝或撤回时，这枚号永远查不到视图。答复的 `outcome` 仍是 `SUBMITTED`，不是接受；客户系统若把拿到号读成已受理，是读错了。
5. **不经首方口的提交**（PS owner）。没声明件号的件只带 `parcelId`，调用方只能按次序对。这类渠道的回执契约属 `PAR-INT-01` 的适配，不在本记录。

## Links

- [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)：首方契约的生产与版本；Decision 八「不在本记录定任何端点的字段」——本记录是其中一处落点
- [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)：决定四（客户查阅面 `/customer-tracking-view` 的首方 Intake）、决定五（首方 API 的契约是客户面端点的单一权威）
- [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md)：决定四，客户包裹件号与内部标识的配对
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：状态码与 `outcome` 不变，`parcels` 只随答案在场
- [ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)：并发建单撞「已存在」按重放重答
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：`AT-PS-101`–`105`
- [UC-VE-008](../application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)：查询标识与不可枚举
- [PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)：声明包裹的永久内部标识；外部标识关联随接受形成
- 票 `.scratch/demo-chain-gaps/issues/101-customer-tracking-view-locates-by-the-internal-parcel-id-only.md`；后续票 `119`
