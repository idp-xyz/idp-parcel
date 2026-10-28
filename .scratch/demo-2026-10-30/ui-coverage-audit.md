# 周五演示：管理台覆盖表（只读审计，2026-10-28，基于 main 52d483cfe 的代码）

约定：页面 id 即 `#/<id>`（`http://localhost:5173/#/shipment-request-inquiry`）。`live` = 在 `apps/admin-web/src/page-registry.tsx` 的 `liveIds` 里。
所有 `liveIds` 之外的页只有 `template-preview`（演示）；其余 47 个导航页全部 live。**live 不等于演示时有东西看**：下表「读到什么」是读页面组件与它调的端点后的判断，没有开浏览器跑过。
触发方式记号：【页】管理台页面上的操作；【脚本】`scripts/demo-seeds/*.sh`；【API】只能 curl（证据里有载荷，但没有现成脚本）；【自动】dispatch 或系统自动。

## 块 1　下单到妥投主链

| # | 动线步骤 | 触发方式 | 结果在哪看（页面 id / 路由 / live） | 缺口 |
|---|---|---|---|---|
| 1.1 | 提交委托 | 【脚本】`submit-one-shipment.sh`（首方客户令牌）。管理台 `shipment-request`「提交服务请求」页发的是同一个 `POST /v1/shipment-requests`，但带操作者令牌 | `shipment-request-inquiry`（委托查阅，live）。列表与详情抽屉读 `GET /shipment-request-views`；**必须起 `IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01`**，否则整页是「未配置」 | **提交页点了必 401** `CUSTOMER_CLIENT_CREDENTIAL_REJECTED`（票 first-party-api/05 冻结）。页面没有这个码的专门文案，只显示「调用方问题 HTTP 401」 |
| 1.2 | 受理 → 已接受（接受判断） | 【自动】dispatch。需要人工时走 `acceptance-review`、`authorized-disposition` 两页（live，读 `isolatedReadFace` 队列） | `shipment-request-inquiry` 详情抽屉：声明包裹、接受判断任务、接受/拒绝决定 | 正常路径两个队列页是空的（只有等待态才有行） |
| 1.3 | 选渠道/面单择优 | 【自动】 | `channel-selection-decisions`、`label-transactions`（live，只读） | 演示动线没走到这条，页面里有没有行取决于种子，未核 |
| 1.4 | 初始路由 `ROUTE_FORMED` | 【自动】 | `route-plans`（live）「初始路由」册：托运申请、判断结论、计划版本、计划适用性 | **不显示计划的各履约段**（票 demo-chain-gaps/72 冻结）；「路由有哪四段」只能口头讲或看 API |
| 1.5 | 运输准备 `OPPORTUNITY_PREPARED` | 【自动】 | 无页面。`transport-preparation` 册后端有，页面没有分区（票 45 冻结） | 缺读面，R3 冻结 |
| 1.6 | 送站预报 | 【脚本】`submit-one-pre-advice.sh`（首方客户令牌） | `node-operations-review`（live）「送站预报」区，读 `GET /node-operations-delivery-pre-advices` | 无 |
| 1.7 | 收寄 `INTAKE_FORMED`（含交付范围核对态） | 【脚本】`receive-one-delivery.sh`（配置员令牌 + 设备三头） | `node-operations-review`「收寄」区（reception 册）；「送站交接单」区 | 无 |
| 1.8 | 自营段：登班次、两池预占、装载分配、交出、出发、交进口岸 | 【API】全部 curl，**没有脚本**（载荷散在 `synthetic-demo-journey-script.md` 补记与 psb/05 逐段） | `transport-fulfillment-review`（live）：班次、运力池（含已预占/已释放）、权威交接结果、交接范围汇总 | 装载分配、承运段、自营段本身没有读册；这一页只看得到班次/池/交接结果。一线作业按 ADR-0021 本就不进管理台 |
| 1.9 | 干线：委托、订舱、承运答复、交给干线承运商、段采用 | 【API】操作者令牌 + 承运商集成客户端令牌，**无脚本** | `transport-fulfillment-review`「订舱」区（状态词 `CONFIRMED`、答复）与「答复挡回」区；「总单」区 | 运输委托与段采用没有读册，页面只看到订舱 |
| 1.10 | 干线成本 BUY 自动形成 | 【自动】 | `charges-billing`（live）「供应商预期成本」册（CNY 8500，协议 `…/v3`） | 无 |
| 1.11 | 末端：尾程交接、派送任务、派送尝试、有效交付 | 【API】作业事实各口，无脚本 | `transport-fulfillment-review`「权威交接结果」「交付证明」区；外部轨迹有效时间判断用 `effective-time-judgment`（live，有写面） | 派送任务、派送尝试无读册；待确认尝试结果册（`pending-attempt-result`）与派送待办册后端有、页面没有 |
| 1.12 | PS 终局 `EFFECTIVE_DELIVERY` + 追踪投影 | 【自动】 | `tracking-projection`（live，`GET /tracking-projections`）：最新标准里程碑、投影版本、替代关系；抽屉里有各条事实的业务/有效/接收三个时间 | 无 |
| 1.13 | 客户轨迹视图（客户看到的那份） | 【脚本/API】`GET /v1/customer-tracking-view?parcel=…`，首方客户令牌（`customer-token.sh`） | **管理台没有**。`tracking-projection` 是运营内部投影，不是客户视图（披露/脱敏口径不同；ETA 对客是 `PENDING_CONFIRMATION`） | 要讲「客户看到什么」只能在终端；`disclosure-policies` 页（live）能看披露策略册当旁证 |

## 块 2　关务跨境

| # | 动线步骤 | 触发方式 | 结果在哪看 | 缺口 |
|---|---|---|---|---|
| 2.1 | 立案、授权确认、申报提交 | 【脚本】`declare-one-parcel.sh <包裹号>`（配置员令牌；**一库一件**，写死申报单元，同库第二件包裹答 `SCOPE_CONFLICT`） | `customs-cases`（live）「就绪与授权」签：`GET /customs-case-registers?registry=readiness` 与 `submission-authority`（授权确认能看到）；「关闭义务」签 | **「关务案件」「申报单元」「资料快照」「提交版本」四签是占位**（页面明说「对象族列表端点尚未建立」，不发请求）。后端也没有对应 GET（`/customs-cases`、`/customs-declaration-submissions` 只有 POST） |
| 2.2 | 申报发送 + 技术回执 | 【自动】dispatch 经通道发给合成服务商（须先 `go run ./scripts/demo-seeds/declaration-broker`）；回执【脚本】`send-one-declaration.sh <提交版本号>` | **无页面**。提交尝试、技术回执、监管接收、业务受理列只在上面那个占位的「提交版本」签里，没有读口 | 后端读口 + 前端都缺。只能看脚本末行输出（尝试 `REGISTERED_PENDING_SEND`，技术判断 `ACKNOWLEDGED`） |
| 2.3 | 放行结果 `RELEASE_RESULT/FULL` | 【API】报关服务商集成客户端令牌打 `POST /customs/external-results`，**无脚本** | **无页面**，无 GET 读口 | 同上，后端读口 + 前端都缺 |
| 2.4 | 放行门禁核对 `MET` | 【API】`POST /customs-release-gate-verifications` | `customs-restrictions`（live）「门禁核对」签读 `GET /customs-gate-conditions`：门禁目录 + 前置条件认定（`MET/UNMET/CONFLICTING`） | 能看目录与前置认定；**核对这一次动作的记录本身是否显示，没有核实**。演示单元的门禁行 launch-wiring/08 已补进种子 |
| 2.5 | 门禁控制交出（交给干线承运商读数 `ADMITTED`） | 【API】只在交接登记那次的响应体里（`gateReading`） | **无页面**。`action_admission_judgement` 没有 GET，运输履约查阅页的交接结果区不含读数列 | 后端读口 + 前端都缺。讲法：终端展示 `HANDOVER_REGISTERED` 响应 |
| 2.6 | 关案 | 【API】`POST /customs-case-closures`；要先按案号受控 CLI 补义务清单，否则答 `UNDECIDED/OBLIGATION_INVENTORY_UNCONFIGURED`（停点二，R2） | `customs-cases`「关闭义务」签（只显示义务清单，不显示关案记录） | 关案记录无读口 |
| 2.7 | 内部合规限制建立/解除（拦截） | 【API】`POST /customs-internal-restrictions`，缺 `standing` 答 `NOT_ACCEPTED` 不带原因 | `customs-restrictions`「合规限制」签存在，但后端 `GET /customs-internal-restrictions` 答 405，**没读口**，签读不到东西 | 看不到限制本身，只能从后续交接响应的读数 `BLOCKED` 看出 |
| 2.8 | 税费付款协作/核对 | 手工登记 【页】（`customs-restrictions` 的登记签，`duty-collaboration`、`duty-payment-verification`） | 同页「税费付款协作」「税费付款核对」签（live，读 `/customs-duty-collaborations`、`/customs-duty-verifications`） | 无 |
| 2.9 | 监管凭证登记 | 【页】`customs-cases`「登记监管凭证」签（走 `customs-regulatory-credential-manual-registrations`，登记人栏可见） | 同页「监管凭证」签 | 无 |
| 2.10 | 合规规则库、口岸与申报路径 | 种子 | `compliance-rules`、`customs-ports-paths`（live） | 无，这是关务块里最稳的页 |

## 块 3　计价与结算

| # | 动线步骤 | 触发方式 | 结果在哪看 | 缺口 |
|---|---|---|---|---|
| 3.1 | 价卡、参考序列（燃油/汇率） | 种子；页上还有登记/导入/草稿面板 | `price-card-catalog`、`reference-series`（live） | 无 |
| 3.2 | 运营试算 | 【页】`pricing-estimate`（live）`POST /pricing-estimates`（走查阅准入，不落库、无「保存」按钮） | 同页逐卡并列的评价 | 需要手填重量/路线等输入；演示前先自己试一遍 |
| 3.3 | 估价（SELL 预估客户费用 CNY 9600） | 【自动】收寄后形成 | `pricing-evaluation`（价格评价，live，`GET /pricing-evaluations`）；`charges-billing`「客户费用」册阶段 `ESTIMATED` | 无 |
| 3.4 | 估价冻结 / 信用暴露 | 【自动】接受时 `FRZ-xxxx`、`EXP-xxxx` | **无页面**。冻结、暴露、余额都没有读端点（`/settlement-*` 只有费用、对账单、资金核销、经营结果四个 GET）。能看到的只有规则：`commercial-policies`（live）选「接受前资金控制」类 | 后端读口 + 前端都缺。效果（余额不够被拒 `AVAILABLE_BALANCE_INSUFFICIENT`）在 `shipment-request-inquiry` 里表现为委托 `REJECTED`，拒绝原因展示未核 |
| 3.5 | 费用确认（客户费用 `ESTIMATED→CONFIRMED`） | 【自动】终局后；运营补拍口 `POST /settlement-charge-confirmations`【API】 | `charges-billing`「客户费用」册：阶段、确认依据、七项固定字段 | 无（补拍口无页面，但自动路径就够） |
| 3.6 | 对账单发布 | 【API】`POST /settlement-statement-publications`（配置员令牌，列费用+报合计），无脚本 | `reconciliation`（对账单，live）「客户对账单」册：账户、期间、合计、行数、争议 | **没有发布按钮**。自动截单在上海 18:00 |
| 3.7 | 供应商账单接收 | 【API】`POST /settlement-supplier-bill-receptions` | `reconciliation`「供应商账单接收」册：匹配数、`auditAuthorityConfigured` | 没有录入页 |
| 3.8 | 审核应付 `PAYABLE_FORMED` | 【API】`POST /settlement-supplier-bill-audits` | **无页面、无读端点**（对账单页明说「主张不是审核应付」） | 后端读口缺。只能展示终端响应 |
| 3.9 | 收付款核销 / 外部资金事实 | 【页】`settlement-application`（live）「采用资金事实」手工签；银行客户端【API】；账户映射只有受控 CLI（`parcel-settlement-register funds-mapping`） | 同页资金事实列表（已核销/未核销/映射/核销明细） | 映射无页面入口 |
| 3.10 | 经营结果 / 毛利 | — | `operating-metrics`（live） | **经营结果册恒空**（要成本分摊，冻结）。页面会明说，但别点 |
| 3.11 | 商业价格/结算政策 | 种子 | `commercial-policies` 选「价格政策」「结算政策」两类是**空册**（`COMMERCIAL_POLICIES_LISTED` 空数组，是有答案的空） | 讲清「空态≠未配置」，别当事故 |

## 块 4　异常处理

| # | 动线步骤 | 触发方式 | 结果在哪看 | 缺口 |
|---|---|---|---|---|
| 4.1 | 决定前撤回 | 【脚本】`walk-customer-command-faces.sh`（首方客户令牌；委托 A 补充+撤回）。管理台 `shipment-request`「决定前撤回」签发 `POST /v1/shipment-requests/withdrawals`，同样是客户渠道口 | `shipment-request-inquiry` 看委托状态 | **页上撤回必 401**，同 1.1 |
| 4.2 | 接受后取消 | 【脚本】同上（委托 B 的 `PCN-1`）。管理台 `cancel-parcel`（导航名「取消与收寄后处置」，live）打 `POST /v1/shipment-requests/parcel-cancellations`，也是客户渠道口 | `shipment-request-inquiry`；`tracking-projection` | **页上取消必 401**。注意：这页名字写「收寄后处置」，但**页里没有收寄后处置决定（退运）的入口**，只有取消三种走向 |
| 4.3 | 取消先后冲突 | — | **无页面**：`GET /cancellation-order-conflicts` 后端有，前端没引用 | 前端缺（小） |
| 4.4 | 收寄后处置决定（退运 `RETURN`） | 【API】`POST /post-intake-disposition-decisions`（配置员令牌，授予已入种子） | **无页面**：`GET /post-intake-disposition-requests` 后端有，前端没引用 | 前端缺 |
| 4.5 | TF 退运旅程开出 | 【自动】 | 无页面（退运旅程无读册；`alternate_journey` 只在库里） | 读口缺，R3 |
| 4.6 | 待人工处理清单（NR、NO 两项）及登记 `HANDLED/UNABLE` | 【API】`GET /post-intake-disposition-manual-items?parcel=`、`POST /post-intake-disposition-manual-item-results` | **无页面**（freeze 清单明记「没有管理台页，R2」） | 前端缺，端点齐全 |
| 4.7 | 拦截（内部合规限制 BLOCKED） | 见 2.7 | 无读口 | 同 2.7 |
| 4.8 | 身份冲突处置、人工重核 | 【页】`node-operations-review` 的 IdentityDispositionPanel（`POST /node-operations/identity-dispositions`、`identity-rechecks`） | 同页「待识别实物」区 | 需要先造出待识别实物（收寄不带预报依据，`receive-one-delivery.sh` 不设 `IDP_PARCEL_PRE_ADVICE_ID` 即得 `PENDING_IDENTIFICATION`），这条是现成的可演示异常 |
| 4.9 | 异常分诊、异常案件、索赔追偿 | 客户索赔【脚本】`walk-customer-command-faces.sh`（`VCL-1`） | `exception-triage`、`exception-cases`、`claims-recovery`（live） | 主链动线不产生异常信号，**前两页多半是空态**；索赔页可能有一行，未核 |
| 4.10 | 改路建议/决定 | 【页】`route-plans` 改路视图（`POST /route-reroute-decisions`） | 同页「路由复核」册 | 要有重新评估触发才有行，动线没走 |

## 页面是否需要补（见回报）

### A. 只缺前端（端点已在 `cmd/parcel-api`）

1. **收寄后处置与待人工处理清单页**（中）。放在「委托受理」区或并进 `cancel-parcel` 作新签。
   - 读：`GET /post-intake-disposition-requests`（待办请求，含 `pendingCount`、`truncated`）、`GET /post-intake-disposition-manual-items?parcel=`（NR/NO 两项带结果与处理人）。
   - 写：`POST /post-intake-disposition-decisions`（`direction: RETURN`，操作者决定面）、`POST /post-intake-disposition-manual-item-results`（`HANDLED` / `UNABLE`，重放答 `200 ALREADY_REGISTERED`）。
   - 只做两个读签（小）也能讲清「清单列出、状态可见」，决定与登记仍用 curl；带两个表单就是中。形状见 `internal/parcelshipment/adapters/http/query_post_intake_disposition_requests.go`。
2. **取消先后冲突册**（小，可选）：`GET /cancellation-order-conflicts`，只读列表。

### B. 端点也缺（需后端读口 + 前端，不属于「只缺前端」）

- 关务：案件/申报单元/提交版本（含提交尝试、技术回执）/放行结果/关案记录/内部合规限制/动作准入读数，**全部无 GET**。`customs-cases` 页四个占位签就是为这些预留的。中到大，且要走 ADR-0017 准入闸门口径。
- 结算：冻结、信用暴露、运营余额、审核应付，**无 GET**。
- 若周五前不做，这两块只能用终端响应讲（见回报「避开」）。

### C. 可选（有现成页看结果，可不补）

- 对账单发布、供应商账单接收、审核应付的操作页（小到中，四个端点在）：用【API】触发，在 `reconciliation` 看结果即可替代；审核应付没有读口。
