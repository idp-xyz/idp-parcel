# 关务与贸易合规专项开发基线

状态：关务控制域专项基线；`UC-CC-001..012`、客户资料入口 `UC-PS-002` 及下游接收 `UC-NO-001`、`UC-TF-001`、`UC-SA-001`、`UC-VE-001` 的整体边界已核对；承运商外部监管舱单责任和应用入口已经闭合，剩余真实试点参数和专项分层按本文管理

本文评估现有十二个关务应用用例是否足以指导关务控制域专项开发，并把该专项的生产、回放/模拟和后续增强分开。它不是 `idp-parcel` 的产品总体首发基线；总体主线见[国际小包网络运营首发产品基线与开发主线](./PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)。本文不重新定义关务领域规则，不代替[关务与贸易合规上下文](../domain/customs-compliance/CONTEXT.md)、[首发试点范围](./PILOT-SCOPE.md)或[参数登记册](./PILOT-PARAMETER-REGISTER.md)。

文件路径暂保留 `CUSTOMS-FIRST-RELEASE-DEVELOPMENT-BASELINE.md` 以避免历史链接断裂；文件标题和产品入口已明确其专项性质。

## 评估结论

`UC-CC-001..012` 已覆盖关务主业务链的核心判断：建案、组建申报单元、形成正式资料、评估就绪、提交授权、冻结并提交、接收承运商外部监管舱单引用、接收外部监管结果、形成后续申报动作、协作监管执行、核对税费付款和逐动作放行门禁、管理内部合规限制，以及逐义务关闭、受控重开和建立后续案件。

当前可以进入首发开发准备，但还不能把现有用例直接当作完整生产 backlog。案件终结、节点监管执行、监管运输处置、实际代垫/客户回收、关务异常协调/客户披露及承运商外部监管舱单接收已经形成应用契约。稳定业务入口已经闭合，剩余阻塞不在于继续增加“清关状态”或空用例，而在于真实程序实例和证据尚未填充：

- 出口、进口的真实责任承运商、来源契约、外部身份/版本、范围、变更关系和后续结果语义，以及其他登记册参数。

十二个现有用例没有需要删除的完整重复。`UC-CC-012` 负责接受和关联承运商外部监管舱单引用，`UC-CC-006` 负责把关联本产品原提交或该外部引用的响应转换为分层监管事实，`UC-CC-007..009` 负责消费事实形成后续判断，`UC-CC-010` 负责案件生命周期终结与关闭后续办，`UC-CC-011` 负责内部限制的唯一形成与解除入口；这不是重复。`UC-CC-003` 唯一拥有申报就绪规则，`UC-CC-005` 只验证本产品原提交判断仍有效并处理提交专属门禁，不得另建一套就绪算法，也不得用于承运商监管舱单。

## 现有用例完整性

| 业务段 | 当前权威用例 | 当前结论 | 首发处理 |
|---|---|---|---|
| 建立稳定案件 | [UC-CC-001](../application/customs-compliance/UC-CC-001-ESTABLISH-CUSTOMS-CASE.md) | 建案边界完整；关闭、重开和后续案件由 `UC-CC-010` 独立拥有 | 保留现有用例；与 `UC-CC-010` 共同形成案件生命周期闭环 |
| 申报单元与正式资料 | [UC-CC-002](../application/customs-compliance/UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md) + [UC-PS-002](../application/parcel-shipment/UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md) | 客户原始资料版本入口与关务正式资料边界完整；关务只读消费来源版本 | 首发受控入口 `P`，真实字段、阶段和授权按参数启用 |
| 申报就绪 | [UC-CC-003](../application/customs-compliance/UC-CC-003-ASSESS-DECLARATION-READINESS.md) | 七类业务门禁完整，并明确为唯一就绪规则所有者 | `P` 生产主链 |
| 提交授权 | [UC-CC-004](../application/customs-compliance/UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md) | 人工/自动授权和失效边界完整 | 首发只启用已确认的人工授权路径 |
| 冻结与提交 | [UC-CC-005](../application/customs-compliance/UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md) | 提交快照、权限、占用、防重和待确认边界完整 | `P` 生产主链；不重复实现就绪规则 |
| 承运商外部监管舱单 | [UC-CC-012](../application/customs-compliance/UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md) | 出口、进口均由承运商形成并提交；本产品只形成外部引用、版本关系、明确范围关联和当前采用判断 | 出口、进口各至少一个真实引用 `P`；复杂冲突、待关联和变更分支 `R/S` |
| 外部结果 | [UC-CC-006](../application/customs-compliance/UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md) | 来源保全、两类互斥结果目标、结果分层、本产品原提交查询和迟到事实完整 | 正常结果 `P`；复杂冲突和迟到分支 `R/S` |
| 后续申报动作 | [UC-CC-007](../application/customs-compliance/UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md) | 补充、更正、撤销、重报和替代关系边界完整 | 锚点程序正常补充/更正按需 `P`；复杂替代 `R/S` |
| 查验、扣留与处置 | [UC-CC-008](../application/customs-compliance/UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md) + [UC-NO-001](../application/node-operations/UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md) + [UC-TF-001](../application/transport-fulfillment/UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md) | 监管决定、执行上下文承接、节点任务/运输准备、执行事实和关务核对分离；现场动作与真实移动分别承接 | 最小查验协作 `R/S`，真实线路自然发生时保留生产证据 |
| 税费、付款、回收与门禁 | [UC-CC-009](../application/customs-compliance/UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md) + [UC-SA-001](../application/settlement-accounting/UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md) | 监管、资金、实际代垫、客户回收和放行所有权已分离；付款核对采用三个正交维度；回收本金与服务费分离；门禁绑定拟执行动作 | 仅在锚点程序适用时进入 `P`，复杂付款分配、退回与撤销 `R/S` |
| 关闭、重开与后续案件 | [UC-CC-010](../application/customs-compliance/UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md) | 逐义务关闭核对、有效责任承接、按原关闭责任来源受控重开和独立后续案件边界完整 | 正常关闭 `P`；受控重开与后续案件 `R/S`，自然发生时保留生产证据 |
| 内部合规限制 | [UC-CC-011](../application/customs-compliance/UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md) | 限制形成、范围化覆盖、原来源解除、当前适用性及下游消费边界完整；不镜像外部监管结果 | 首发启用已登记来源和人工/规则决定；复杂重叠、迟到和批量分支 `R/S` |

`P` 表示首发生产证据，`R/S` 表示脱敏历史回放、正式测试环境或隔离模拟。具体证据要求以[验收矩阵](./PILOT-ACCEPTANCE-MATRIX.md)为准。

## 应用闭环完成与剩余缺口

### 1. 关务案件闭环已形成

[UC-CC-010](../application/customs-compliance/UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md) 已负责：

- 正常关闭前逐项确认当前申报、阻断性限制、监管处置、税费和其他义务已经终结，或剩余责任已由有权接收方对明确范围有效承接。
- 保存关闭核对、逐项关闭依据、关闭决定、关闭责任来源、适用范围、决定方、授权和生效时间。
- 对仍属于原监管程序的迟到事实，依据使当前关闭期成立的关闭决定、受影响依据项、原关闭责任来源及当前授权形成受控重开决定。
- 对独立补税、稽核、申诉或后续义务建立关联后续案件，不强行重开原案。
- 不回退已经发生的节点作业、运输移动、交接或交付事实。

首发生产必须实现正常关闭和未结责任盘点。受控重开与后续案件至少形成 `R/S` 骨架，真实发生时按权威事实处理。真实义务目录、有效责任承接、关闭/重开权限和分类依据通过 `PAR-CUS-06` 管理。

### 2. 内部合规限制已形成

`UC-CC-011` 已形成，负责：

- 由明确责任来源对对象、受限动作、范围、依据、开始时间和解除条件形成内部限制。
- 仅允许来源责任方形成部分或全部解除，人工备注、异常关闭、商业批准或外部放行不能代替解除。
- 向申报就绪和逐动作放行门禁提供当前有效判断，不维护一个可手工修改的 `blocked` 字段。
- 保存限制、更正、解除和迟到事实的版本与适用时间，不覆盖历史。

`UC-CC-003`、`UC-CC-009` 和 `UC-CC-010` 消费该用例的范围化当前判断；`UC-CC-006` 可以独立接收并保全外部事实，必要时只把已接受事实作为 `UC-CC-011` 的限制依据输入。任何消费用例都不得自行形成或解除内部限制。

### 3. 客户原始资料补充与更正已形成

[UC-PS-002](../application/parcel-shipment/UC-PS-002-AMEND-CUSTOMER-SOURCE-DATA.md) 已由 `parcel-shipment` 拥有接受后客户原始声明的新版本、原因、权限、业务时间和当前采用判断。它不改变接受基线、成员、客户/法人/合同/产品、节点实测、正式申报或已发生事实。

未提交范围由 `UC-CC-002` 只读消费新来源版本并形成新的正式资料，`UC-CC-003` 重新评估就绪；已提交范围由 `UC-CC-007` 分类补充、更正、撤销重报或替代。案件关闭后到达的资料版本由 `UC-CC-010` 保全和分类，不自动重开。首发入口、允许字段、阶段、授权、并发和迟到语义通过 `PAR-COM-13` 与 `PAR-INT-01` 确认，不能依赖开发或数据库人工改值。

### 4. 下游接收与回接

以下能力不能只停留在领域所有权说明；节点、运输、结算和异常披露入口均已形成：

| 交接 | 责任上下文 | 最低闭环 |
|---|---|---|
| 查验、扣留或处置协作 | `node-operations` / `transport-fulfillment` | [UC-NO-001](../application/node-operations/UC-NO-001-ACCEPT-AND-EXECUTE-CUSTOMS-NODE-COLLABORATION.md) 承接现场动作；[UC-TF-001](../application/transport-fulfillment/UC-TF-001-ACCEPT-AND-FULFILL-REGULATORY-TRANSPORT-DISPOSITION.md) 只承接真实移动；两者记录实际事实并回接稳定引用，不把任务或到达冒充监管结果 |
| 实际代垫与客户回收 | `settlement-accounting` | [UC-SA-001](../application/settlement-accounting/UC-SA-001-ASSESS-ACTUAL-ADVANCE-AND-FORM-CUSTOMER-RECOVERY.md) 接收付款方、外部资金事实、关务税费/核对和合同责任，先确认实际代垫，再独立形成、不形成或调整客户代垫回收 |
| 关务异常与客户披露 | `visibility-exception` | [UC-VE-001](../application/visibility-exception/UC-VE-001-COORDINATE-CUSTOMS-EXCEPTIONS-AND-CUSTOMER-DISCLOSURE.md) 只消费已接受关务事实，分别形成信号、案件、处置请求、逐客户披露决定和消息意图；消息提交、接受、送达和客户确认不互相冒充，也不改变关务义务 |
| 退运或其他运输处置 | `transport-fulfillment` | 已并入 `UC-TF-001`：只依据当前有效且范围明确的移动授权建立独立监管旅程，不回退原运输旅程 |

锚点线路首发没有实际采用的分支可以只做 `R/S` 或明确由现行外部流程承担，但必须登记权威责任方、交接接受结果和回接证据，不能留成无人拥有的“后续处理”。

### 5. 监管舱单责任

首发出口和进口监管舱单已经确认均由责任承运商在外部形成并提交。[UC-CC-012](../application/customs-compliance/UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md) 负责接收并关联外部身份、来源版本、范围、形成/提交事实和更正/撤销/替代关系：

- 本产品不建立本地监管舱单草稿、资料编辑、就绪、授权、提交、查询或再次发送流程，也不代表承运商更正、撤销或替代。
- `UC-CC-006` 只在 `UC-CC-012` 已接受的外部引用上接收后续监管结果；承运商报告“已提交”不等于监管机构已接收或业务受理。
- 运输总单、运输舱单、外部监管舱单引用和实际装载保持独立，任何共同标识或成员关系都不能合并其身份和事实层。
- `PAR-CUS-01/02/03/05` 和 `PAR-INT-03` 继续登记真实责任承运商、来源、外部身份/版本、范围、变更关系、权限、复杂验证证据和后续结果语义。

这项责任决定和稳定应用入口已经闭合；剩余缺口属于真实试点参数，不再以“谁形成监管舱单”阻塞领域分析。

## 关务专项开发切片

以下切片只描述 `PN-05` 关务控制域专项工作流。它们可以与产品主线的委托、节点、运输、追踪和结算切片并行准备，但任何一个关务切片完成都不代表产品总体首发完成。

| 顺序 | 切片 | 生产范围 | 完成标准 |
|---|---|---|---|
| 0 | [真实参数取证与生产语义准入](../design/customs-slice-0-business-development-handoff.md) | 参数登记册中渠道或国家专属详细设计前的 31 项精确门槛参数，以及退运、税费/代垫、异常/通知等实际启用分支的附加参数 | 对同一业务对象、事实类型、业务身份和适用范围只有一个权威形成方；不同上下文分别拥有各自事实或判断；已确认责任落实到真实实例，未决项只阻断对应生产分支，不阻止稳定骨架开发 |
| 1 | [建案、资料和内部限制](../design/customs-slice-1-business-development-handoff.md) | `UC-CC-001/002`、`UC-PS-002`、内部合规限制最小闭环 | 案件、客户来源版本、单元、正式资料和限制可追溯；缺口有责任入口，不允许人工改状态绕过 |
| 2 | [人工就绪、授权和提交](../design/customs-slice-2-business-development-handoff.md) | `UC-CC-003/004/005` 连续工作流 | 只用一套就绪规则；授权、提交版本、尝试和外部关联分别留痕；超时不盲目重发 |
| 3 | [外部舱单、正常结果、逐动作门禁与正常关闭](../design/customs-slice-3-business-development-handoff.md) | `UC-CC-012` 出口/进口外部监管舱单引用、`UC-CC-006` 正常结果、适用的 `UC-CC-009` 门禁、`UC-CC-010` 正常关闭 | 外部舱单引用不生成本地提交；承运商提交事实、技术回执、监管接收/受理、放行和门禁不合并；节点/运输只消费动作匹配结果；案件逐义务无未解释责任或具备有效承接 |
| 4 | [适用税费与结算交接](../design/customs-slice-4-business-development-handoff.md) | 锚点程序确有税费时启用 `UC-CC-009` 和 `UC-SA-001` | 监管税费、外部资金、实际代垫、客户回收、服务费和放行分别形成；付款覆盖不制造放行，回收不冒充已收款 |
| 5 | [最小异常验证](../design/customs-slice-5-business-development-handoff.md) | 查询待确认、原案内补充/更正、`UC-CC-008` 监管执行协作、`UC-NO-001` 最小节点协作、适用的 `UC-TF-001` 监管运输、`UC-CC-010` 迟到事实分类、受控重开与后续案件，以及 `UC-VE-001` 关务异常协调和逐客户披露 | 自然发生使用 `P`，复杂或未自然发生分支使用 `R/S`；证明不重复执行、不覆盖历史、不越权解除、不复活旧授权、不回退既有事实，正常监管过程不被自动异常化，消息结果不冒充客户确认 |
| 6 | [撤销、重报与替代关系](../design/customs-slice-6-business-development-handoff.md) | `UC-CC-007` 的 `AT-CC-191..207`、`AT-CC-215`；跨客户只验证后续动作范围隔离 | 18 项验收唯一归属；撤销与重报独立，顺序门禁由唯一就绪规则判断，拟替代不被技术结果提前升级，历史和非交集范围不被覆盖；默认使用 `R/S`，只有真实程序、责任和证据齐备时进入生产 backlog |

切片 0 不是技术调研阶段，而是生产业务语义准入。切片 1 至 5 可以并行准备代码骨架；切片 6 只在其前置链稳定后准备复杂分支骨架。任何未取得真实来源、规则或责任依据的分支都必须保持显式未配置，不能写入默认国家、口岸、角色、阈值、顺序或 SLA。多执行方处置、多付款分配、监管税费退回与资金撤销分别保留在其权威用例的后续产品 backlog，不再合并进切片 6。

## 用例级开发交付清单

本清单是开发与测试的追踪入口，不重新定义用例中的结果语义。每项交付必须以链接用例的完整输入、结果、权限、并发和验收示例为准；表内概述不能替代用例正文。

真实参数未就绪不阻止开发稳定对象、业务结果、显式未决、未受理、技术未形成和测试骨架，但必须阻止对应生产判断或外部动作。开发不得用默认国家、角色、渠道、状态码、范围、时限或管理员权限使未配置分支“先跑起来”。

| 用例与验收范围 | 开发必须交付的业务能力 | 必须守住的分层或负向结果 | 主要生产门禁 |
|---|---|---|---|
| [UC-CC-001](../application/customs-compliance/UC-CC-001-ESTABLISH-CUSTOMS-CASE.md)，`AT-CC-001..022` | 按固定监管程序和责任范围建立稳定关务案件，保存包裹关联及初始角色资格快照 | 案件、申报单元、资料、提交和监管结果分离；条件不足、技术未形成、未受理和不适用分别返回 | `PAR-CUS-01..04`、`PAR-NET-02/03` 及真实客户、法人、产品和合同版本 |
| [UC-CC-002](../application/customs-compliance/UC-CC-002-FORM-DECLARATION-UNIT-AND-DATA.md)，`AT-CC-023..047` | 形成申报单元、成员范围、正式资料快照和字段级来源追溯 | 客户原始资料、实测、关务判断和正式资料不得覆盖对平；资料未决不得用默认字段补齐 | `PAR-CUS-01..05` 及当前客户、法人、产品和合同依据 |
| [UC-CC-003](../application/customs-compliance/UC-CC-003-ASSESS-DECLARATION-READINESS.md)，`AT-CC-048..077` | 作为唯一就绪规则入口，对指定单元、资料、动作和适用时点逐门禁判断 | 已就绪、未就绪、判断未决、不再就绪和技术未形成分离；批量部分结果不产生单元“部分就绪” | `PAR-CUS-01..05`、`PAR-NET-02/03` 及适用内部限制判断 |
| [UC-CC-004](../application/customs-compliance/UC-CC-004-AUTHORIZE-DECLARATION-SUBMISSION.md)，`AT-CC-078..106` | 对明确目标、渠道和拟提交动作形成人工授权决定，并保存请求方、决定方和依据 | 已授权、明确拒绝、待人工决定、条件未决、授权失效和技术未形成分离；等待或无权限不是拒绝 | `PAR-CUS-03/04`；首发未登记自动政策时不得自动授权 |
| [UC-CC-005](../application/customs-compliance/UC-CC-005-FREEZE-SUBMISSION-AND-INITIATE-ATTEMPT.md)，`AT-CC-107..139` | 重新核验门禁，冻结不可覆盖提交版本，形成授权使用、凭证关系、提交尝试和发送意图 | 已登记待发送、已发起和结果待确认分离；超时不得盲目重发、换身份或重复消费授权 | `PAR-CUS-01..05` 中的真实渠道、账号、外部关联、权限和凭证规则 |
| [UC-CC-006](../application/customs-compliance/UC-CC-006-RECEIVE-AND-RECONCILE-EXTERNAL-RESULTS.md)，`AT-CC-140..180` | 保全来源，将结果关联到互斥的本产品原提交或 `UC-CC-012` 外部舱单引用，并形成分层监管事实 | 待关联、解释未决、来源冲突、结果冲突、技术未形成及各结果层分离；原提交查询和安全再次发送只适用于本产品提交 | `PAR-CUS-01..06` 的来源权威、代码/层次、范围、时间、更正和查询语义 |
| [UC-CC-007](../application/customs-compliance/UC-CC-007-MANAGE-POST-SUBMISSION-ACTIONS-AND-REPLACEMENT.md)，`AT-CC-181..220` | 分类原案内补充、更正、撤销、重报及替代目标，保存拟替代和有效替代关系 | 客户资料变化不自动形成后续申报动作；新版本和关系追加形成，不覆盖原提交、原结果或已发生事实 | `PAR-CUS-01..06` 的动作资格、顺序、身份保持、权限和关系生效语义 |
| [UC-CC-008](../application/customs-compliance/UC-CC-008-COORDINATE-INSPECTION-DETENTION-AND-RECONCILE-DISPOSITION.md)，`AT-CC-221..257` | 建立范围化监管协作事项，接收节点或运输承接与执行事实，并核对处置覆盖 | 监管决定、协作事项、承接、任务/准备、执行事实和关务核对分离；监管决定执行要求冲突、执行交接冲突、协作请求冲突和执行事实冲突不得互换，且部分执行、失败、差异、冲突和证据不足不得冒充完成 | `PAR-CUS-01..05`、`PAR-NET-04/11` 中的真实动作、执行方、路径、权限和证据规则 |
| [UC-CC-009](../application/customs-compliance/UC-CC-009-RECONCILE-TAX-PAYMENT-RECOVERY-AND-RELEASE-GATE.md)，`AT-CC-258..295` | 接收税费和外部资金事实，按范围核对付款，并对明确拟执行动作形成放行门禁判断和结算交接 | 税费、资金、覆盖/差额/有效性、实际代垫、客户回收、动作门禁和监管放行分别形成；付款覆盖不制造放行 | `PAR-CUS-01..05`、`PAR-INT-05`、`PAR-SET-01/08`；不适用税费时须有明确依据 |
| [UC-CC-010](../application/customs-compliance/UC-CC-010-CLOSE-REOPEN-AND-ESTABLISH-FOLLOW-UP-CASE.md)，`AT-CC-296..338` | 逐义务形成关闭核对和关闭决定，对迟到事实分类为受控重开、后续案件或无需改变生命周期 | 可关闭不等于已关闭；重开尊重原关闭责任来源，不撤销原关闭或回退物流、资金和结算事实 | `PAR-CUS-06` 的义务目录、责任承接、关闭责任来源、分类依据和权限 |
| [UC-CC-011](../application/customs-compliance/UC-CC-011-MANAGE-INTERNAL-COMPLIANCE-RESTRICTIONS.md)，`AT-CC-339..375` | 由责任来源对明确对象、范围和动作形成内部限制，并由相应来源形成部分或全部解除 | 单项限制解除不等于当前动作无其他限制；外部放行、异常关闭、商业批准或条件满足不自动解除 | `PAR-CUS-07` 的来源、对象/动作词表、监管边界、形成/解除权限和规则版本 |
| [UC-CC-012](../application/customs-compliance/UC-CC-012-RECEIVE-AND-RELATE-EXTERNAL-REGULATORY-MANIFEST.md)，`AT-CC-376..410` | 接收承运商外部监管舱单身份、来源版本、范围和变更关系，形成外部引用、关联和当前采用判断 | 不创建本地监管舱单或提交尝试；运输舱单、外部引用和实际装载分离；承运商已提交不等于监管接收或受理 | `PAR-CUS-01/02/03/05`、`PAR-INT-03` 的责任承运商、来源、身份/版本、范围、权限和结果语义 |

单个用例只有在其验收范围全部可重复执行，并且所有未就绪生产参数都能稳定返回未配置、未决、未受理或不适用而不触发越权业务结果时，才达到开发完成。通过单条正常样例、页面可操作或接口返回成功不能替代该标准。

## 共享实现约束

关务及其下游接收用例都需要请求身份、幂等、并发、未受理、业务未决、技术未形成、审计和发布恢复，但这些机制不应在每个用例各自发明一套实现。开发应共享稳定机制，同时保留每个用例自己的业务结果和所有权：

- 相同请求与相同业务内容返回已有结果；相同请求身份携带不同内容形成冲突。
- 业务负向判断、权威依据未决和技术处理未形成分别表达。
- 来源事实、有效性判断和当前派生结果追加保存，不按最后到达覆盖。
- 跨客户、法人、程序、案件和范围的权限与最小披露统一执行。
- 已形成业务结果与发布意图一起可恢复；重试发布不能重复形成业务事实。

共享机制不能合并监管决定、关务判断、物理执行、外部资金、运营结算或异常协调的业务语义。

## 生产分支启用阻塞清单

- 锚点出口和进口程序、申报渠道、报关服务方、账号及授权范围。
- 出口、进口的真实监管舱单责任承运商、来源契约、外部身份/版本、适用范围、更正/撤销/替代关系及后续结果语义。
- 内部合规限制的责任来源、适用动作和解除授权。
- `PAR-COM-13` 中客户原始资料补充/更正的生产入口、允许字段、阶段、授权、并发和迟到语义。
- 查验、扣留、处置和退运在 `UC-NO-001/UC-TF-001` 中使用的真实执行方、路径、授权与回接证据。
- 锚点程序是否产生税费、是否要求付款、付款是否阻断哪些拟执行动作。
- 唯一财务系统边界提供的外部资金结果层，以及 `PAR-SET-08` 中实际代垫和客户回收的成立、调整条件。
- `PAR-CUS-06` 中的正常关闭义务目录、有效责任承接、逐项关闭责任来源、受控重开决定和后续案件建立权限。
- `PAR-VIS-04..07` 与 `PAR-INT-06` 中的关务异常条件、分诊和发作期、协调响应、最小披露、通知义务、授权及消息结果层。

上述值统一进入[参数登记册](./PILOT-PARAMETER-REGISTER.md)，取证和局部准入顺序见[关务切片 0 交接](../design/customs-slice-0-business-development-handoff.md)。在它们确认前，可以实现领域对象、输入/结果语义、显式未决和测试骨架，但不能宣称对应生产流程已经完成。

## 下一执行顺序

1. 先按切片 0 的 [`CC-S0-W01` 证据请求工作单](../design/customs-slice-0-w01-evidence-request.md)核验锚点客户、责任法人、产品合同、成熟线路和出口/进口适用区域；候选脱敏标识不能代替真实证据。
2. 再按 [`CC-S0-W02` 来源证据工作单](../design/customs-slice-0-w02-source-evidence-request.md)核验程序、责任承运商、来源、外部身份/版本、范围、变更关系和结果语义；按 [`CC-S0-W03` 权限证据工作单](../design/customs-slice-0-w03-roles-permissions-evidence-request.md)核验 `PAR-CUS-03/04`，并执行 `CC-S0-W08` 的 `PAR-CUS-05` 验证证据。
3. 并行完成 `PAR-COM-13`、`PAR-SET-08`、`PAR-VIS-04..07`、`PAR-INT-06` 及实际启用分支的其他参数，再逐分支决定进入 `P`、保持 `R/S` 或明确本期不适用。

关务稳定业务分析已经形成从客户来源输入、判断、承运商外部监管舱单引用、外部交互、物理/结算/异常披露交接到案件终结的完整开发闭环。真实参数填充完成后，才能把对应分支转为生产 backlog 并进入渠道或国家专属详细设计。
