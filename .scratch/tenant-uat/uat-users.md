# IDP-DEMO 验收要用的人

这是验收环境里「谁、能做什么、看得见什么」的唯一一份。人不另造角色。能力面、运营决定、作业范围、数据可见范围都按人一笔一笔授，登记在这一张表上。别的文件只引用这里，不另写一份名单。

Tops 持有全部授予，只用来搭数据和给人授权，不拿他走业务验收。用他走，看不出权限边界。口令都是 `changeme`。

客户、仓两列是数据可见范围，不写进能力面。没写就是这一类什么都看不见；「全部」在册上写成 `*`。作业事实上的仓是另一笔，只决定在哪个仓登作业，不代替看得见的仓。委托列表按客户切，仓册按仓切。库存、商品、入库这些仓内页还没接上同一份范围，接上之前仍是整租户。

| 人 | 邮箱 | 现在 | 能力面 | 客户 | 仓 | 用来验什么 |
|---|---|---|---|---|---|---|
| Tops | tops@idp.xyz | 已在册 | 五格全有，71 种决定，7 个作业范围 | 全部 | 全部 | 搭数据、给别人授权。不作为业务岗位 |
| 陈屿 | chenyu@idp.xyz | 已在册。登记册配置写已撤销 | 主数据与运营查阅读、操作者接入管理 | 无 | 无 | 登记别人、授和撤。不能改价、不能受理 |
| 林培 | linpei@idp.xyz | 已在册 | 主数据与运营查阅读、登记册配置写 | 全部 | 全部 | 公司、客户、产品、价格、线路、规则。不能给人授权 |
| 阅岚 | yuelan@idp.xyz | 已在册 | 只有主数据与运营查阅读 | 全部 | 全部 | 各页只能看。写按钮和决定按钮都不该出现 |
| 苏晚 | suwan@idp.xyz | 已在册 | 查阅，加下面客服受理那 13 种决定 | 只有 `IDP-ACCOUNT-QINGHE` | 无 | 受理、拒绝、撤回之后的处置、身份冲突 |
| 顾川 | guchuan@idp.xyz | 已在册 | 查阅，加下面调度那 23 种决定 | 全部 | 无 | 委托运输、订舱、舱位、改路、派送重问 |
| 沈澄 | shencheng@idp.xyz | 已在册 | 查阅，加下面关务那 9 种决定 | 全部 | 无 | 建案、申报、放行、限制 |
| 周衡 | zhouheng@idp.xyz | 已在册 | 查阅，加下面财务那 9 种决定 | 全部 | 无 | 对账、供应商账单、费用确认、资金事实采用 |
| 马泊 | mapo@idp.xyz | 已在册 | 查阅，加下面仓配那 17 种决定，外加作业事实：仓 `IDP-WH-PVG` | 全部 | 只有 `IDP-WH-PVG` | 入仓、波次、库存、退货。现场登记作业只在这个仓 |

陈屿身上的登记册配置写已撤销，这一格在林培身上。

库存调整的提交和批准现在都在马泊身上。产品允许拆成两个人。验收要专门看「提交的人不能自己批准」时，再加一个仓配复核，不在这 9 人里。

作业范围先只给马泊一个仓。网点、场外的作业事实等线路建好、真有那个网点标识再授，不先造一串还没有的范围。

## 一票货还要 4 个系统账号

这些不是操作者，不进管理台顶栏。发行方是集成客户端那一只（验收环境 `:5558`），权按客户面能力或外部事实类型授。

| 系统 | 绑到 | 用来验什么 |
|---|---|---|
| 青禾货主系统 | 客户账户 `IDP-ACCOUNT-QINGHE` | 客户自己下单、预报、查轨迹 |
| 海屿航运 | 参与方 `IDP-PARTY-HAIYU` | 干线承运商回订舱 |
| 报关行 | 之后要登的报关服务商 | 回传关务技术回执 |
| 银行 | 之后要登的资金来源 | 入账，供财务采用 |

## 运营决定归谁

### 客服受理（13）

- MANUAL_REVIEW_COMPLETION
- ACTIVE_REJECTION
- AUTHORIZED_DISPOSITION
- CONTROLLED_CLOSURE
- CONTROLLED_REOPENING
- IDENTITY_CONFLICT_DISPOSITION
- PENDING_IDENTITY_RECHECK
- POST_INTAKE_MANUAL_ITEM_HANDLING
- POST_INTAKE_SERVICE_DISPOSITION
- CONTROL_RELEASE_REFUSAL_HANDLING
- MANUAL_MEASUREMENT_ENTRY
- CONDITION_RESTORE
- DELIVERY_HANDOVER_CONFIRMATION

### 调度（23）

- LOAD_ASSIGNMENT
- PARTICIPATION_TERMINATION
- SEGMENT_CLOSURE
- FULFILLMENT_SEGMENT_COMMISSION_ADOPTION
- FULFILLMENT_SEGMENT_COMMISSION_ADOPTION_WITHDRAWAL
- TRANSPORT_COMMISSION_SUBMISSION
- TRANSPORT_COMMISSION_CANCELLATION
- TRANSPORT_COMMISSION_AGREEMENT_REVISION
- BOOKING_REQUEST_SUBMISSION
- BOOKING_REQUEST_CANCELLATION
- CAPACITY_RESERVATION
- CAPACITY_RELEASE
- CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT
- AUTHORIZED_REROUTE
- ALTERNATE_JOURNEY_START
- EFFECTIVE_TIME_JUDGMENT
- DISPATCH_TASK_REGISTRATION
- DELIVERY_DISPATCH_REASK
- DELIVERY_ATTEMPT_RESULT_CONFIRMATION
- PICKUP_ATTEMPT_RESULT_CONFIRMATION
- NODE_COLLABORATION_ACCEPTANCE
- TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION
- TRANSPORT_CHARGE_OCCURRENCE_INVALIDATION

### 关务（9）

- CUSTOMS_CASE_ESTABLISHMENT
- CUSTOMS_CASE_CLOSURE
- CUSTOMS_DECLARATION_SUBMISSION
- CUSTOMS_SUBMISSION_AUTHORIZATION
- REGULATORY_CREDENTIAL_REGISTRATION
- REGULATORY_DISPOSITION_ACCEPTANCE
- RELEASE_GATE_VERIFICATION
- INTERNAL_RESTRICTION_ESTABLISHMENT
- INTERNAL_RESTRICTION_DECISION

### 财务（9）

- STATEMENT_PUBLICATION
- STATEMENT_DISPUTE_REGISTRATION
- STATEMENT_DISPUTE_RESOLUTION
- SUPPLIER_BILL_RECEPTION
- SUPPLIER_BILL_AUDIT
- SUPPLIER_BILL_DISPUTE_DECISION
- CHARGE_CONFIRMATION
- CHARGE_ADJUSTMENT_REGISTRATION
- EXTERNAL_FUNDS_FACT_ADOPTION

### 仓配（17）

- WAVE_RELEASE
- STOCK_IMPORT_SUBMISSION
- STOCK_COUNT
- STOCK_FREEZE
- STOCK_ADJUSTMENT_SUBMISSION
- STOCK_ADJUSTMENT_APPROVAL
- INBOUND_PRE_ADVICE_RECORD_ON_BEHALF
- INBOUND_PRE_ADVICE_CLOSE
- UNCLAIMED_ARRIVAL_CLAIM
- UNCLAIMED_ARRIVAL_DISPOSITION
- GOODS_RECORD_ON_BEHALF
- RECEIPT_DISCREPANCY_RECORD_ON_BEHALF
- OUTBOUND_REQUEST_RECORD_ON_BEHALF
- OUTBOUND_CONFIRMATION_CORRECTION
- RETURN_RECORD_ON_BEHALF
- PROCESSING_INSTRUCTION
- RELABEL_INSTRUCTION

合计 71，与产品词表的运营决定种类逐一对应，没有遗漏。上面五个岗位各拿自己那一组，组与组不重叠。Tops 另持全部。
