# idp-parcel 文档

本目录是 `idp-parcel` 独立项目的产品与领域设计文档入口。除 `archive` 下明确标记的历史材料外，这里的文档只记录已经确认的决策；尚未确认的信息必须明确标记为“待确认”，不得用假设补齐。

## 工作方式入口

- 仓库根 [AGENTS.md](../AGENTS.md)：开发与 Agent 的开工顺序、红线、改文档规则与 skills 路由；不替代本目录下的产品/领域权威定义。
- [Agent skills 接线](./agents/)：issue tracker（本地 `.scratch/`）、triage 标签、领域文档消费约定；供 `/triage`、`/to-spec`、`/to-tickets`、`/implement`、`/wayfinder` 读取。

## 权威文档职责

- [产品基线](./product/PRODUCT-BASELINE.md)：定义产品定位、独立项目边界以及已经确认的长期领域原则，是判断产品是否偏离既定方向的权威依据。
- [国际小包网络运营首发产品基线与开发主线](./product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：定义产品核心闭环、关务控制域的位置、首发纵向切片和端到端开发准入，是产品级首发开发入口。
- [首发试点范围](./product/PILOT-SCOPE.md)：定义首发试点已经确认的范围原则、明确排除项和待确认参数，是判断首发是否越界的权威依据。
- [首发试点验收场景矩阵](./product/PILOT-ACCEPTANCE-MATRIX.md)：把试点范围和领域场景转化为带证据层级、验收门槛、责任与待填参数的执行清单，是试点证据规划和最终验收判定的权威入口。
- [首发试点参数与证据登记册](./product/PILOT-PARAMETER-REGISTER.md)：集中登记真实客户、线路、伙伴、合同、规模和治理参数及其证据引用；只记录实例状态，不重新定义领域规则。
- [关务与贸易合规专项开发基线](./product/CUSTOMS-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：评估 `UC-CC-001..012` 的控制域闭环，登记真实参数阻塞项，并把关务专项生产、回放/模拟和后续增强分层；不代表产品总体首发主线。
- [领域上下文地图](./domain/CONTEXT-MAP.md)：定义十个限界上下文的所有权、非所有权和协作关系，是领域边界的唯一权威来源。
- [参与方与商业上下文](./domain/party-commercial/CONTEXT.md)：定义参与方、客户账户、服务产品、客户合同、渠道关系与账号授权，是该上下文语言、规则和生命周期的权威来源。
- [小包计价上下文](./domain/parcel-pricing/CONTEXT.md)：定义版本化价卡、费率表、计价策略、纯价格评价、解释和回放；不拥有合同、来源事实或费用账务。
- [小包托运上下文](./domain/parcel-shipment/CONTEXT.md)：定义委托接受、包裹身份与谱系、面单交易、外部标识、取消和完成机制，是该上下文语言、规则和生命周期的权威来源。
- [网络与路由上下文](./domain/network-routing/CONTEXT.md)：定义网络拓扑、服务区域、可达性、路由计划、路由指令和受控改路，是该上下文语言、规则和生命周期的权威来源。
- [节点作业上下文](./domain/node-operations/CONTEXT.md)：定义节点收寄、实物控制、待识别实物、测量、位置、集运单元、封签、作业任务、监管协作承接、节点执行事实、装卸和场内核对，是该上下文语言、规则和生命周期的权威来源。
- [运输履约上下文](./domain/transport-fulfillment/CONTEXT.md)：定义实际履约段、班次、容量、运输委托、订舱、装载分配、监管运输处置承接、运输交接、实际承运、揽派尝试、交付证明、运输单证和运输收费发生项，是该上下文语言、规则和生命周期的权威来源。
- [关务与贸易合规上下文](./domain/customs-compliance/CONTEXT.md)：定义稳定关务案件、申报单元、角色与资格快照、正式申报和提交版本、承运商外部监管舱单引用及关联判断、合规判断、内部限制及其解除、监管税费及外部监管事实，是该上下文语言、规则和生命周期的权威来源。
- [全程追踪与异常上下文](./domain/visibility-exception/CONTEXT.md)：定义内部追踪投影、客户普通全程追踪视图、标准里程碑、ETA、异常信号与案件、处置协调、客户可见异常、客户披露决定、责任证据、客户索赔三类期限及追偿外部动作/响应，是该上下文语言、规则和生命周期的权威来源。
- [结算与经营核算上下文](./domain/settlement-accounting/CONTEXT.md)：定义价格评价的财务采用、费用与调整类型唯一所有权、实际代垫判断、客户代垫回收、客户赔付义务、应追偿/认可金额、结算账户、供应商预期成本与审核应付、对账、真实收付映射、核销、成本分摊和经营指标，是物流运营结算语言、规则和生命周期的权威来源。
- [代收与清分边界](./domain/CONTEXT-MAP.md)：定义代收本金、渠道在途、清分、应付客户和汇付的上下文边界。该上下文当前仅由上下文地图定义；首发已确认不包含 COD，因此不创建空的 `CONTEXT.md`。
- [统一领域语言](./domain/GLOSSARY.md)：定义跨上下文必须保持一致的核心术语及需要避免的混用。
- [核心领域场景](./domain/SCENARIOS.md)：用端到端场景验证对象身份、业务规则和跨上下文边界。
- [架构决策记录](./adr/README.md)：解释已经接受且难以逆转的架构选择及其代价。
- [应用用例](./application/README.md)：把已确认领域规则组织成面向开发的输入、结果、应用编排、失败边界和验收条件；当前已覆盖 PN-02 商业权威/接单/撤回、PN-03 接受后路由/收寄/节点作业、PN-04 常规运输/逐包裹取消/终局、PN-06 内部投影/普通客户追踪/异常索赔和 PN-07 通用运营结算，并保留关务专项协作。真实参数、联合验证和代码实现仍待推进；它不重新定义领域规则或真实试点参数。
- [Go 首个消费者切片实施决策简报](./design/parcel-go-first-consumer-slice-decision-brief.md)：固定 Parcel 的 Go 模块化单体、PostgreSQL/显式 SQL、Bento 技术边界和 `UC-PS-001` 首个可编码子切片；未确认业务参数不得因此成为生产默认值。
- [`PN-02` 真实参数取证与开发交接](./design/pn-02-real-parameter-evidence-and-development-handoff.md)：把商业权威发布与唯一解析、锚点商业范围、生产接入与归属、接单/决定前撤回、财务策略、接受前可达性和联合准入组织成五个工作包，并明确稳定骨架、`R/S` 配置验证和生产分支的不同门槛。
- [`PN02-W01` 锚点商业与服务范围证据工作单](./design/pn-02-w01-anchor-commercial-scope-evidence-request.md)：把客户、责任法人、服务产品、客户合同和成熟线路作为独立对象交叉核验；只登记脱敏证据索引，不保存真实值。
- [`PN02-W02` 生产接入与生产归属证据工作单](./design/pn-02-w02-ingress-production-ownership-evidence-request.md)：核验来源请求身份、三类生产归属、安全交接、暂停恢复和回退责任；不把入口连通或技术回执当成生产准入。
- [`PN02-W03` 接单规则、决定授权与接受前财务控制证据工作单](./design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md)：核验规则包、五类时间、人工复核与主动拒绝授权，以及结算模式、账户、价格和财务控制/补偿边界。
- [`PN02-W04` 接受前逻辑可达性证据工作单](./design/pn-02-w04-pre-acceptance-logical-reachability-evidence-request.md)：核验服务区域、网络拓扑、日历与截单、候选资格、三值证据完整性、判断时点和失效重判；不创建路由计划或履约资源。
- [`PN02-W05` 联合验证与生产分支准入工作单](./design/pn-02-w05-joint-verification-and-production-branch-admission.md)：把 W01 至 W04、验收矩阵和技术门槛固定到同一能力范围，形成稳定骨架、`R/S` 或提交 PN-08 的生产候选结论；不自行批准真实客户流量。
- [`PN02-S01` 合成商业与财务控制开发交接](./design/pn-02-synthetic-commercial-and-financial-control-development-handoff.md)：在无真实数据阶段以 `SYN-COM-01..04` 验证预付/账期唯一解析、范围冲突、无适用依据和依赖未决；只形成隔离 `S`，不改变真实参数或生产准入。
- [`PN02-S02` 合成来源保全与生产归属开发交接](./design/pn-02-synthetic-ingress-and-production-ownership-development-handoff.md)：验证来源保全、重复/冲突、三类归属判定、安全交接、暂停恢复和建单门禁；当前不创建生产归属端口、`已提交`聚合或持久化写入。
- [`PN02-SYN` 合成业务契约开发任务包](./design/pn-02-synthetic-business-contract-development-task-pack.md)：把 `S01-W01..W05` 与 `S02-W01..W06` 汇总为开发任务、联合契约检查和当前可编码/禁止生产实现边界；所有结果只记为隔离 `S`。
- [`PN-03` 网络收寄与节点作业开发交接](./design/pn-03-network-intake-and-node-operations-development-handoff.md)：把接受后初始路由、真实收寄模式、客户送站、场外揽收、有效网络收寄、正式承诺、收寄/实测后路由复核、节点集运与联合验证组织成 W01 至 W08；当前生产结论保持 `No-Go / 待参数化`。
- [`PN-04` 运输履约与包裹终局开发交接](./design/pn-04-transport-fulfillment-and-parcel-finalization-development-handoff.md)：把常规运输机会、班次、容量、运输委托、订舱、收费发生项、权威交接、实际履约、派送、POD、逐包裹取消/收寄后处置、替代/退运旅程与终局组织成 W01 至 W08；当前生产结论保持 `No-Go / 待参数化`，不以监管运输专项替代常规主链。
- [`PN-06` 追踪、异常与客户披露开发交接](./design/pn-06-visibility-and-exception-development-handoff.md)：把已接受事实、内部投影、普通客户全程追踪视图、ETA、可见性缺口、异常分诊、案件、客户披露、索赔三类期限、追偿外部动作/响应、关务专项和联合验证组织成 W01 至 W08；通用业务编排已形成，真实参数和联合证据仍待提供，当前生产结论保持 `No-Go / 待参数化`。
- [`PN-07` 运营结算与经营核算开发交接](./design/pn-07-operational-settlement-and-accounting-development-handoff.md)：把计量、客户/供应商计价、运输发生项到预期成本、调整唯一所有权、接受前财务控制、对账单、供应商账单、真实收付映射/核销、成本分摊、经营结果、赔付/追偿金额和关务代垫组织成 W01 至 W09；通用业务编排已形成，真实参数和联合账期证据仍待提供，当前生产结论保持 `No-Go / 待参数化`。
- [`PP-S03` 证据与合成契约开发交接](./design/pp-s03-par-set-02-03-evidence-and-synthetic-contract.md)：在无真实 SELL/BUY 参数时固定隔离 `S` 的计价验收、费用代码交接和缺映射 `PENDING` 边界；不创建结算对象或解除生产门禁。
- [`PN-08` 端到端试点与阶段准入开发交接](./design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md)：把同版本候选证据、历史回放、影子运行、限量生产、唯一权威、暂停恢复、对象级接管、真实主链/账期、在途盘点和正式 `Go/No-Go` 组织成 W01 至 W09；它是产品级治理编排，不新增限界上下文或全局业务状态机，当前生产结论保持 `No-Go / 待参数化`。
- [关务切片 0 真实参数取证与生产语义准入交接](./design/customs-slice-0-business-development-handoff.md)：把锚点范围、出口/进口程序、责任承运商与来源、权限、执行/资金/异常责任及验证证据组织成有依赖的取证工作包，并逐生产分支决定只做稳定骨架、使用 `R/S` 验证或准入 `P`。
- [`CC-S0-W01` 锚点范围首批证据请求工作单](./design/customs-slice-0-w01-evidence-request.md)：把客户、责任法人、服务产品、合同、成熟线路及出口/进口区域的真实证据要求、交叉核验、登记册动作和开发阻断整理成业务方可直接提交的工作单；不保存真实值或第二套参数状态。
- [`CC-S0-W02` 程序、承运商与权威来源证据工作单](./design/customs-slice-0-w02-source-evidence-request.md)：分别核验本产品出口/进口申报、承运商出口/进口外部监管舱单和适用面单渠道，固定责任承运商、代理/渠道、账号、传输来源与监管结果来源的边界。
- [`CC-S0-W03` 关务角色、逐动作权限与规则证据工作单](./design/customs-slice-0-w03-roles-permissions-evidence-request.md)：把角色资格、委派、账号授权、逐动作权限、自动提交政策和监管凭证证据交给开发可执行的权限与规则门禁；不授予节点、运输、付款或承运商舱单实际执行权限。
- [关务切片 1 业务开发交接](./design/customs-slice-1-business-development-handoff.md)：把客户资料版本、关务建案、申报单元与正式资料、内部合规限制组织成开发工作包、依赖顺序、参数门禁和完成定义；它只派生引用权威领域与用例规则，不定义接口或存储结构。
- [关务切片 2 业务开发交接](./design/customs-slice-2-business-development-handoff.md)：把唯一申报就绪规则、失效与重新评估、逐次人工授权、不可覆盖提交版本和提交尝试组织成连续但不合并所有权的开发工作包。
- [关务切片 3 业务开发交接](./design/customs-slice-3-business-development-handoff.md)：把承运商外部监管舱单引用、本产品原提交的正常分层结果、逐动作放行门禁和正常案件关闭组织成双入口开发工作包，并明确切片 4、5 的后续边界。
- [关务切片 4 业务开发交接](./design/customs-slice-4-business-development-handoff.md)：把适用税费、外部资金事实引用、三维付款核对、实际代垫判断、客户代垫回收和追加调整组织成跨关务与结算的开发工作包；真实锚点程序无税费时不得启用生产分支。
- [关务切片 5 业务开发交接](./design/customs-slice-5-business-development-handoff.md)：把原提交查询、原案内补充/更正、监管执行协作、节点与运输承接、关闭后续办、关务异常协调和逐客户披露组织成跨上下文开发工作包，并把复杂撤销重报留给切片 6。
- [关务切片 6 业务开发交接](./design/customs-slice-6-business-development-handoff.md)：把复杂撤销、重报、替代层次、顺序门禁、拟替代/有效替代和迟到双申报保护组织成 18 项唯一验收工作；多执行方、多付款和税费/资金增强继续保持独立 backlog。

产品基线与试点范围承担不同职责：产品基线说明长期必须保持什么，试点范围说明首期实际验证什么。试点文档不得改写产品基线；如真实试点证明基线需要改变，应先形成新的明确决策，再更新相应权威文档。

`idp-parcel` 拥有独立的产品、领域模型、业务数据库、部署和发布边界。其他项目的文档不能成为本项目领域定义的权威来源。

## 历史材料

- [领域发现问答记录](./archive/DOMAIN-DISCOVERY-QA.md)：保留领域发现与决策形成过程，包含候选方案、阶段性建议和未回填确认，不是当前领域规则的权威来源。
- [领域发现问答工作副本](./archive/DOMAIN-DISCOVERY-QA-WORKING-COPY.md)：保留后续访谈期间产生的重复或未完成记录，仅供追溯，不是当前领域规则的权威来源。

## 产品主线阅读顺序

1. 阅读本页，了解文档职责和决策记录原则。
2. 阅读[产品基线](./product/PRODUCT-BASELINE.md)，理解产品边界与长期核心原则。
3. 阅读[国际小包网络运营首发产品基线与开发主线](./product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)，理解产品主链、核心能力和关务控制域位置。
4. 阅读[首发试点范围](./product/PILOT-SCOPE.md)，理解首期验证目标和仍待确认的实施参数。
5. 阅读[首发试点验收场景矩阵](./product/PILOT-ACCEPTANCE-MATRIX.md)，理解每项首发能力采用的证据层级、验收门槛和执行记录要求。
6. 阅读[首发试点参数与证据登记册](./product/PILOT-PARAMETER-REGISTER.md)，填充真实试点实例及其证据引用。
7. 阅读[领域上下文地图](./domain/CONTEXT-MAP.md)和[统一领域语言](./domain/GLOSSARY.md)，理解所有权和核心对象区别。
8. 依次阅读[参与方与商业](./domain/party-commercial/CONTEXT.md)、[小包托运](./domain/parcel-shipment/CONTEXT.md)、[网络与路由](./domain/network-routing/CONTEXT.md)、[节点作业](./domain/node-operations/CONTEXT.md)和[运输履约](./domain/transport-fulfillment/CONTEXT.md)，理解接单到交付的主链边界。
9. 阅读[小包计价](./domain/parcel-pricing/CONTEXT.md)，理解商业绑定、可执行价卡、纯评价和结算金额责任之间的边界。
10. 阅读[关务与贸易合规上下文](./domain/customs-compliance/CONTEXT.md)，理解关务控制域及其与物理执行和运输事实的边界。
11. 依次阅读[全程追踪与异常](./domain/visibility-exception/CONTEXT.md)和[结算与经营核算](./domain/settlement-accounting/CONTEXT.md)，理解追踪、异常、计价采用和运营结算边界。
12. 后续产品版本如果确认包含 COD，阅读[领域上下文地图](./domain/CONTEXT-MAP.md)中的 `collection-remittance` 边界；详细上下文只在真实范围确认后创建。
13. 阅读[核心领域场景](./domain/SCENARIOS.md)，检验这些边界如何共同完成实际业务。
14. 阅读[架构决策记录](./adr/README.md)，理解长期约束背后的取舍。
15. 阅读[应用用例](./application/README.md)，落实 PN-02 至 PN-07 已形成的通用应用编排，并识别各切片的真实参数、联合验证和代码实现缺口。
16. 阅读[`PN-02` 真实参数取证与开发交接](./design/pn-02-real-parameter-evidence-and-development-handoff.md)。当前无真实数据时，先按 [`PN02-SYN`](./design/pn-02-synthetic-business-contract-development-task-pack.md) 执行 `S02-W01..W06` 与 `S01-W01..W05`；它汇总 [`PN02-S02`](./design/pn-02-synthetic-ingress-and-production-ownership-development-handoff.md) 和 [`PN02-S01`](./design/pn-02-synthetic-commercial-and-financial-control-development-handoff.md)，只形成隔离 `S`，不解除生产阻断。取得真实资料后，依次按 [`PN02-W01`](./design/pn-02-w01-anchor-commercial-scope-evidence-request.md)、[`PN02-W02`](./design/pn-02-w02-ingress-production-ownership-evidence-request.md)、[`PN02-W03`](./design/pn-02-w03-acceptance-rules-and-financial-control-evidence-request.md) 和 [`PN02-W04`](./design/pn-02-w04-pre-acceptance-logical-reachability-evidence-request.md) 提交业务证据，再按 [`PN02-W05`](./design/pn-02-w05-joint-verification-and-production-branch-admission.md) 形成同版本联合验证与生产候选证据，并提交 PN-08 阶段评审。
17. 阅读[`PN-03` 网络收寄与节点作业开发交接](./design/pn-03-network-intake-and-node-operations-development-handoff.md)，按 W01 至 W08 推进接受后路由、两类真实收寄来源、正式承诺、路由复核和节点作业，并只把同版本联合验证结果提交 PN-08 评审。
18. 阅读[`PN-04` 运输履约与包裹终局开发交接](./design/pn-04-transport-fulfillment-and-parcel-finalization-development-handoff.md)，按 W01 至 W08 推进班次、容量、交接、实际履约、派送和逐包裹终局，并只把同版本联合验证结果提交 PN-08 评审。
19. 阅读[`PN-06` 追踪、异常与客户披露开发交接](./design/pn-06-visibility-and-exception-development-handoff.md)，按 W01 至 W08 推进追踪投影、ETA/缺口、异常案件和客户披露，并只把同版本联合验证结果提交 PN-08 评审。
20. 阅读[`PN-07` 运营结算与经营核算开发交接](./design/pn-07-operational-settlement-and-accounting-development-handoff.md)，按 W01 至 W09 推进计价、财务控制、客户/供应商结算、核销、分摊和金额结算，并只把同版本联合账期结果提交 PN-08 评审。
21. 阅读[`PN-08` 端到端试点与阶段准入开发交接](./design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md)，按 W01 至 W09 汇总各切片候选，依次形成回放、影子、限量生产和正式验收的实际证据与 `Go/No-Go`；不要把阶段治理实现成全局运输状态。
22. 阅读[Go 首个消费者切片实施决策简报](./design/parcel-go-first-consumer-slice-decision-brief.md)，理解产品主线下的技术代码基线、Bento 消费者合同和首个可编码子切片。

## PN-05 关务专项阅读顺序

只有开展 PN-05 关务控制域取证、开发或验收时，才需要按本节继续阅读；这些材料不是产品总体开发顺序。

1. 阅读[关务与贸易合规专项开发基线](./product/CUSTOMS-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)，理解关务用例完整性、证据门槛和专项切片。
2. 阅读[关务切片 0 真实参数取证与生产语义准入交接](./design/customs-slice-0-business-development-handoff.md)，按锚点范围、权威来源、责任权限和证据依赖推进专项核验。
3. 依次完成[`CC-S0-W01`](./design/customs-slice-0-w01-evidence-request.md)、[`CC-S0-W02`](./design/customs-slice-0-w02-source-evidence-request.md)和[`CC-S0-W03`](./design/customs-slice-0-w03-roles-permissions-evidence-request.md)证据工作单；其余工作包按切片 0 交接和参数登记册推进。
4. 依次阅读[关务切片 1](./design/customs-slice-1-business-development-handoff.md)、[切片 2](./design/customs-slice-2-business-development-handoff.md)、[切片 3](./design/customs-slice-3-business-development-handoff.md)、[切片 4](./design/customs-slice-4-business-development-handoff.md)、[切片 5](./design/customs-slice-5-business-development-handoff.md)和[切片 6](./design/customs-slice-6-business-development-handoff.md)，按各自范围拆分专项任务。

## 维护原则

- 一个决策只保留一个权威定义，其他文档通过相对链接引用，不复制形成第二套口径。
- 已确认决策与待确认事项必须分开表达。
- 共享文档中的真实客户、线路及其他敏感实例使用稳定脱敏标识；真实映射和受限证据保存在自建 MinIO S3 的 `idp-parcel/pilot` 逻辑命名空间中，项目仓库只登记可审计的证据索引。
- 试点范围、临时实现和渠道限制不得被写成长期领域事实。
- 文档使用本项目自己的语言、标识和边界，不继承其他项目的领域对象或生命周期。
- 限界上下文不等于微服务、进程、数据库或团队；技术边界需要另行决策。
- 各上下文的 `CONTEXT.md` 按确认顺序创建，不建立只有标题的空文档。
- 应用用例只拥有业务编排、输入输出语义和验收映射；领域定义、业务不变量和生命周期必须引用相应权威文档，不能复制成第二套规则。
- 后续新增领域、流程、规则、ADR 或集成文档时，应在本页补充入口和权威职责。
