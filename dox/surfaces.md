# 入口总表

对照 main `d118b4f83`（2026-11-12），由代码读出：管理台页面取自 `apps/admin-web/src/navigation.ts`，接口取自 `cmd/parcel-api/endpoints_*.go` 的登记处。这一页是清单，不受行数上限约束；页面或接口增减时照代码重列。

它回答「产品有哪些能操作的地方」。某一页背后的能力今天能不能用，看所属上下文文件的末节与 [scenarios.md](./scenarios.md)。

## 管理台：54 页

**工作台**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 工作台 | 登录后的起始页。 | 管理台自身 |
| 最近打开 | 这台电脑上最近打开过的运单、客户等对象，点一下回到它。 | 管理台自身 |
| 保存的视图 | 存下来的筛选条件，一键回到常用的列表视图。 | 管理台自身 |

**运单**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 运单 | 查看全部运单：谁下的单、几件、现在到哪一步。 | parcel-shipment |
| 物流动态 | 每件包裹现在到哪一步、最近一条动态是什么。 | visibility-exception |
| 新建与撤回 | 新建一张运单，或在受理决定形成前撤回它。 | parcel-shipment |
| 新建平台货件 | 送往电商平台仓的一个平台货件建一张运单：平台货件号、平台仓、每箱的箱标号与装箱。 | parcel-shipment |
| 待受理复核 | 需要人工判断才能受理的运单，在这里完成复核。 | parcel-shipment |
| 受限运单处置 | 被规则拦下的运单，由有权限的人决定去向：拒绝，或退回客户补充。 | parcel-shipment |
| 资金释放待处理 | 预付冻结没能自动释放的运单，核对后在结算侧释放，或标记已处理。 | parcel-shipment |
| 取消运单 | 客户或运营要求取消某件包裹时，在这里发起。 | parcel-shipment |
| 货物处置 | 已收寄后客户要求停止服务的包裹，决定退运、改派或继续，并跟进待办。 | parcel-shipment |
| 面单 | 向渠道购买面单的每一笔记录及其结果。 | parcel-shipment |
| 渠道择优 | 多个渠道并列、需要人工定夺的运单。 | parcel-shipment |
| 路线规划 | 每件包裹的运输路线怎么定、改过几次。 | network-routing |
| 网点作业 | 网点收寄、集运与交接的作业记录。 | node-operations |
| 运输班次 | 班次、舱位与承运商交接的实际情况。 | transport-fulfillment |
| 外部轨迹时间确认 | 承运商回传的轨迹从何时起算，由我们确认。 | transport-fulfillment |

**客户**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 客户与合同 | 客户账户、合同及其版本：谁签了什么、什么时候生效。 | party-commercial |
| 商业规则 | 接单、价格、结算、信用与授权等规则的各个版本。 | party-commercial |

**合作伙伴**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 往来单位 | 所有与我们有往来的单位——客户、承运商、代理、清关行、渠道——以及彼此之间的关系。 | party-commercial |
| 供应商协议 | 与承运商、代理等供应商签的协议及其版本。 | party-commercial |
| 渠道产品 | 渠道平台提供的产品目录，以及我们的服务产品与它们的对应关系。 | party-commercial |

**产品与价格**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 服务产品 | 我们对外销售的服务产品及其版本与形态。 | party-commercial |
| 价格表 | 对客户与对供应商的价格表版本：导入、批准与发布。 | parcel-pricing |
| 汇率与燃油费率 | 计价要引用的汇率、燃油费率等外部数据，按期登记。 | parcel-pricing |
| 计费记录 | 每一次计价的结果与依据，用于对账与争议复核。 | parcel-pricing |
| 计价试算 | 按价格表试算一件包裹的费用，不写入任何记录。 | parcel-pricing |

**线路与运力**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 网络与网点 | 网点、连接、线路与服务日历的登记。 | network-routing |
| 服务区域 | 各网点覆盖哪些区域。 | network-routing |
| 口岸与申报路径 | 可用的口岸与申报路径。 | customs-compliance |

**清关**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 清关单 | 每件包裹的清关单、报关批次与海关回执。 | customs-compliance |
| 合规限制与税费 | 合规限制及其解除、监管核定税费、放行检查。 | customs-compliance |
| 合规规则 | 禁限运、归类、申报价值等合规判断规则的版本。 | customs-compliance |

**异常与索赔**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 异常处理 | 需要人工分诊或协调处置的异常信号。 | visibility-exception |
| 异常案件 | 已立案的异常：谁在处理、处理到哪一步。 | visibility-exception |
| 索赔与追偿 | 客户索赔，以及向供应商或保险的追偿。 | visibility-exception |
| 索赔资格与授权 | 哪些合同覆盖哪些索赔，谁可以代客户提出。 | visibility-exception |

**账务**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 客户费用 | 每张运单的费用明细、计费重量与调整。 | settlement-accounting |
| 对账单 | 发给客户的对账单，以及客户对单上费用提出的争议。 | settlement-accounting |
| 供应商账单与应付 | 登记供应商发来的账单、逐行审核，审核通过的金额形成应付。 | settlement-accounting |
| 收付款核销 | 把实际收到或付出的钱核到对账单与账户上。 | settlement-accounting |
| 代收货款 | 替客户代收的货款、回汇批次与差异。 | collection-remittance |
| 经营核算 | 按口径派生的经营毛利与经营损失。 | settlement-accounting |

**设置**

| 页面 | 用来做什么 | 上下文 |
|---|---|---|
| 公司主体 | 我们自己签约、开票、结算用的公司。 | party-commercial |
| 追踪规则 | 承运商回传的事件如何对应到标准状态、什么情况自动立案。 | visibility-exception |
| 通知与披露 | 向客户披露什么、通知的时限与渠道。 | visibility-exception |
| 单据编号 | 运单、对账单、异常案件等各类单据的号怎么出：此刻的格式与样例号，改用新格式，登记单据编号时区。 | 平台（单据编号） |
| 扫描设备 | 网点与场外作业使用的扫描设备登记。 | access identity |
| 操作者与授予 | 本公司能登录管理台的操作者，以及每个人持有哪几格授予。 | access identity |
| 审批与发布 | 等待批准的合同、价格与规则发布稿。 | party-commercial |
| 基础数据 | 国家和地区、币种、计量单位的标准目录；国家和币种另可登记本公司启用哪些。 | platform/refdata |
| 试点治理 | 产品实例的阶段决定、暂停与恢复，只给管理员。 | pilotgovernance |
| 演示 | 页面模板的演示页，不是业务功能。 | 管理台自身 |

## 界面用词与领域词

管理台用一线的词，`dox/contexts/` 与代码用领域词。同一个东西，两个叫法：

| 管理台上叫 | 领域里叫 |
|---|---|
| 运单、运单号 | 委托、委托的单据号 |
| 客户单号 | 来源请求键 |
| 包裹号、客户件号、跟踪号 | 包裹内部标识、客户包裹件号、渠道包裹件号 |
| 往来单位 | 业务参与方 |
| 公司主体 | 责任法人 |
| 价格表 | 价卡（定价方案与价表） |
| 计费记录 | 价格评价 |
| 网点 | 物流节点 |
| 清关单 | 关务案件 |
| 物流动态 | 全程追踪投影 |
| 平台货件 | 平台货件（一个平台货件一张运单） |

完整词典在 `apps/admin-web/src/vocabulary/`。

## 接口：254 个，按接入面分

谁能调哪一类接口，由接入面定。每一面的授予在[接入与身份](./contexts/access-identity.md)里登记。

| 接入面 | 谁用 | 数量 |
|---|---|---|
| 客户接口 | 货主客户自己的系统。绑定一个客户账户，路径在 `/v1` 下 | 11 |
| 集成客户端 | 外部系统回传事实：报关服务商、承运商、银行或财务系统 | 6 |
| 作业事实登记 | 一线作业端用登记过的设备登事实：收寄、交接、揽收、派送、交付 | 18 |
| 运营决定 | 操作员作出要留名的决定：复核、处置、裁决、确认、审核 | 54 |
| 登记册配置写 | 操作员登记主数据与规则：合同、价卡、网络、目录 | 75 |
| 主数据与运营查阅读 | 操作员查阅 | 83 |
| 操作者与授予管理 | 管理员管谁能登录、持有哪几格授予 | 4 |
| 隔离读 | 受理复核与授权处置的队列读口 | 3 |

### 客户接口

| 方法 | 路径 | 做什么 |
|---|---|---|
| POST | `/v1/claims` | 提出索赔 |
| GET | `/v1/customer-shipment-tracking` | 按委托读各包裹的追踪 |
| GET | `/v1/customer-tracking-view` | 读一件包裹的客户全程追踪视图 |
| POST | `/v1/node-operations/delivery-handover-conclusions` | 确认或否认送站交接单 |
| POST | `/v1/node-operations/delivery-pre-advice-withdrawals` | 撤回送站预报 |
| POST | `/v1/node-operations/delivery-pre-advices` | 送站预报 |
| POST | `/v1/shipment-requests` | 提交委托（含平台货件） |
| POST | `/v1/shipment-requests/parcel-cancellations` | 取消包裹 |
| POST | `/v1/shipment-requests/source-data-amendments` | 修订客户原始资料 |
| POST | `/v1/shipment-requests/supplements` | 受控补充：交来新提交版本 |
| POST | `/v1/shipment-requests/withdrawals` | 决定前撤回整份委托 |

### 集成客户端接口

| 方法 | 路径 | 做什么 |
|---|---|---|
| POST | `/customs-regulatory-credential-registrations` | 登记监管凭证 |
| POST | `/customs/external-results` | 报关服务商回传外部监管结果 |
| POST | `/customs/technical-receipts` | 报关服务商回传技术回执 |
| POST | `/settlement-external-funds-fact-correction-registrations` | 更正已回传的收付款事实 |
| POST | `/settlement-external-funds-fact-registrations` | 财务系统或银行回传收付款事实 |
| POST | `/transport-fulfillment/booking-answers` | 承运方答复订舱 |

### 操作员与作业端接口

其余各面的接口太多，不在这里逐条列；登记处就是完整清单。按路径前缀的数量：
`transport` 50、`commercial` 43、`customs` 31、`pricing` 22、`settlement` 19、`node` 16、`visibility` 9、`network` 8、`shipment` 6、`reference` 5、`route` 5、`operator` 5、`document` 4、`post` 4、`cancellation` 2、`exception` 2、`control` 2、`claims` 1、`channel` 1、`collection` 1、`governance` 1、`label` 1、`work` 1、`operators` 1、`acceptance` 1、`authorized` 1、`tracking` 1。

## 离线登记命令

运营在数据库网络内手跑，不监听端口。逐个说明见 [tenant.md](./tenant.md)「每一项登在哪个口」。
