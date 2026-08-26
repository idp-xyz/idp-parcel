# 04 演示动线:产品经理视角的合成 S 端到端

Category: feature
Status: resolved(脚本 2b37b30,勘误与页面层取证 73a789e)
Owner: MCP-1(队列通道 1)
Blocked by: 无——范围经 2026-08-26 裁决收敛(见下方 MCP-1 评论)。读面半边全解除
(master-data-wiring/06,07,08 与本 feature 票 05 均 resolved,读面准入按 ADR-0078 落地,
`IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01` 启用);委托侧事实链半边的三堵墙
(取证见下方 MCP-2 评论)移出本票范围:动线在委托步如实停墙并指名重启条件,不等审计票 01。

把「产品就绪 = 可演示」具体化为一条动线:租户的物流产品经理用合成 S 数据走
「建服务产品 → 配价卡与线路 → 一单委托从提交到终局 → 追踪与对账」,管理台全链可看。

## v1 / v2 分界(评估已裁,开工前先读)

- **v1 读为主**:全链业务事实由登记 CLI 灌成套合成 S 种子(产品、价卡、委托、收寄、
  履约、投影、费用),页面负责如实展示;写动作(下单等)由 CLI 侧演示。理由:生产
  路径 403 是刻意的(ADR-0055),隔离环境的合成接入渠道也被 ADR-0055 Decision 五
  两项机制未决拦着(载荷规范化摘要、准入范围装配)。
  【2026-08-26 补】v1 的「页面如实展示」经勘察证实自身还差一道读面准入——已裁定
  走装配注入放行(ADR-0078),实现在票 05;两项写侧未决对运营查阅面的豁免由该 ADR
  显式作出,本条理由句对写路径依然成立。
  【2026-08-26 再补·上一句已过时】上面点名的「两项机制未决」今天**都已闭合**:载荷
  规范化摘要是审计票 14(resolved),准入范围装配归审计票 11 与 13(均 resolved)。因此
  拦住隔离环境合成接入渠道的**不再是机制缺件,而是一次未做的裁决**——ADR-0072 维持了
  ADR-0055 对运行时渠道登记表的否决,把登记册形状留待 `PAR-INT-01` 证据。ADR-0078 已
  为读面示范过绕开这条否决的形状(不立册,按装配注入放行,`SYN-` 前缀门禁);写侧要不要
  照办是新的一裁,不在本票。
- **v2 可点击**:上述两项闭合后,在隔离环境按 S 级登记合成接入渠道,演示动线的写
  路径才能从页面发起。v2 需要一次明确裁决(小 ADR 或票内决定),本票不做。

## 要求(2026-08-26 范围裁决后)

- 种子要「成套成故事」——**主数据侧**:同一合成租户下,产品/价卡/线路/海关口径
  相互引用,能讲通「一个产品从定义到可售」的前半生;不是每表孤立几行。票 08 的
  种子包是基础,缺口在本票补。委托/收寄/履约/投影/费用五类种子**移出本票**
  (三堵墙取证见下方 MCP-2 评论),墙开(审计票 01 重启条件到位)后另立票补后半生。
- `SYN-ACCOUNT-01` 约定保留给后续票:委托侧事实链种子的客户账户用它(隔离读面
  准入注入的可见账户,见 cmd/parcel-api 的 isolatedReadCustomerAccount);换账户或
  加账户先扩那处常量,否则灌入的委托行在 /shipment-request-views 上不可见。本票
  今天写不出委托行,该条不构成本票工作量。
- 页面如实标注演示态(demo/S),不冒充生产;不 mock、不绕 Intake。
- 动线脚本(哪个角色、按什么顺序、每步看哪页、讲哪句)落一份文档,位置开工时定
  (docs/design 或随管理台文档)。**动线在委托那一步如实停住**:指名三堵墙各自的
  重启条件(审计票 01 的 PAR-INT-01 证据、审计票 13 的装配点留白、审计票 04 的
  0007 写入方),与 ADR-0017「墙本身是正确的」同调。

## 完成标准(随范围裁决同步)

动线的主数据段在本机隔离环境可完整走通并可复灌(灌种子 → 设
`IDP_PARCEL_ISOLATED_READ_TENANT` 起 API → 读端点/页面见数据);委托段如实停墙
有实证(写端点 403 与错误码);脚本文档落位;自己提交,票面记 resolved + SHA + 哪种绿。

## Comments

- 2026-08-26 · MCP-2:按「继续」勘察本票可开工性(只读,未改代码),基线 `7056c05`。
  **结论:票面「要求」第一条的委托侧事实链种子今天灌不进去,「阻塞已全解除」只对读面
  半边成立。** 三堵墙彼此独立,任一堵单独就足以拦住;三堵都不是「没人做」,而是已裁定的
  显式留待。

  **一、库内现状(演示库 `idp-parcel-postgres-gate`,实测)。** 委托侧逐张 0 行:
  `parcel_shipment.source_submission` / `shipment_request` / `final_outcome`、
  `node_operations.reception`、`transport_fulfillment.effective_delivery`、
  `visibility_exception.customer_view` / `tracking_projection_current`、
  `settlement_accounting.customer_charge`。主数据侧 `network_routing.line_version` 有 1 行
  (票 08 种子),但 `network_routing.network_definition` 同样 0 行——第三堵墙就在那。

  **二、墙一:委托侧没有入库通道。** 票 08 种子包的四条通道全是主数据登记 CLI
  (`parcel-commercial` / `parcel-pricing-register` / `parcel-network-register` /
  `parcel-customs-register`),委托侧一条都没有。`parcel-dispatch` 不是入口:`Loop.Run`
  只反复调 `DispatchOnce` 转投已在 Outbox 里的信封,没有事实可转。HTTP 那侧的写端点在
  `assembleBusinessEndpoints` 里全装 `UnconfiguredIntake{}`,ADR-0078 的放行面只枚举运营
  查阅八行、把写端点显式排除(票 05 的 `isolatedReadAdmittedPatterns` 两向都有测试信号)。
  能造真通道的审计票
  [01](../../syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md)
  现为 `needs-info`:ADR-0072 维持了 ADR-0055 对运行时渠道登记表的否决,重启条件是
  `PAR-INT-01` 最低证据到位,而本仓尚无租户。

  **三、墙二:绕过通道也过不了生产归属。** 即便写一个 CLI 直调
  `buildSubmissionOrchestration` 那套编排,`parcel-api` 装配点把 `Directory` 与
  `SelfAuthority` 留空(审计票
  [13](../../syn-wall-door-audit/issues/13-production-ownership-bridge-has-no-assembly-point.md)
  有意为之),而 `ProductionOwnershipAdapter.governanceScope` 在这两样任一缺席时**先于读
  登记册**就返回未配置,归属答`权威未确定`,提交停在 `OWNERSHIP_UNRESOLVED`。

  **附带勘误(未改,留给裁定)**:`buildSubmissionOrchestration` 头注与审计票 13 都写
  「恢复动作从『写代码』变成『登记一条权威区间』」。这句对治理桥三个读口成立,对
  `Directory` 不成立——`Directory == nil` 时 `governanceScope` 早退,那条区间根本不会被
  读到,恢复动作仍是「写一个目录实现」。`pilot_governance.authority_interval` 眼下也是
  0 行,两头都不在。

  **四、墙三:再往下还有一堵。** 即便前两堵都过、委托成`已接受`,路由腿会停在
  `RouteEvidenceNotConfigured`:三个证据视图只读 0007 的 `network_routing.network_definition`,
  而审计票
  [04](../../syn-wall-door-audit/issues/04-network-definition-register-no-writer-no-resolver.md)
  的「本票不降墙」一节明文把 0007 的写入方排除在交付范围外,全仓 INSERT 只在
  `network_definition_test.go`。没有路由就没有投影、没有费用——票面要的「投影、费用」
  两格在委托能提交之后依然拿不到。

  **五、还能交付什么。** 票面四件要求里,「动线脚本文档」那件不依赖委托侧数据。可交付的
  形状是:主数据七页走真数据(票 07 已接线、票 05 已放行),走到委托那一步**如实停住并
  指名墙与重启条件**——这与 ADR-0017「墙本身是正确的」同调,不是降级交付。不可交付的是
  「要求」第一条的委托/收寄/履约/投影/费用五类种子;第二条的 `SYN-ACCOUNT-01` 约定本身
  成立,但今天没有任何东西能写出带该账户的委托行。

  **要裁的一件**:把本票收敛成「主数据可走 + 委托侧如实停墙」的动线(本轮即可做),还是
  保持原范围继续等审计票 01 的重启条件(`PAR-INT-01` 证据,无租户即无限期)。两条都不需要
  新 ADR,第一条只改本票范围。

  本笔只动 `.md`,故无构建信号可报。

- 2026-08-26 · MCP-1:**范围裁决并认领。** 用户经队列通道 1 委托本会话裁断并令开工,
  裁定**收敛范围**(上方「要裁的一件」第一条):动线 = 主数据真数据可走 + 委托步如实
  停墙并指名重启条件;不等审计票 01(PAR-INT-01 无租户即无限期)。理由:演示目标是
  「产品就绪=可演示」,在墙前如实停住并指名重启条件与 ADR-0017「墙本身是正确的」同调,
  是诚实演示不是降级;保持原范围会把本票挂进无限期。不需要新 ADR——读面放行与写侧
  禁区 ADR-0078 已定,本裁只改本票范围。票头 Status/Owner、「要求」「完成标准」两节
  同笔按裁决改写,原文以 git 史为准(基线 627d339)。

- 2026-08-26 · MCP-1(收口):**resolved。** 交付两笔:脚本文档 `2b37b30`
  (docs/design/synthetic-demo-journey-script.md + docs/README 入口),勘误与页面层取证
  `73a789e`。**并行撞车如实记**:2b37b30 由另一会话在本票认领笔(9213acf,12:43 已推送)
  之后 12:49 落于本地——内容与本会话同期独立取证的端点层事实完全一致,故整体采纳、
  不另立第二份;其「页面层无法取证(链接农场失效/需 PAT/连不上 github)」一节定因错误,
  已在 73a789e 勘误(node_modules 是 WSL 侧安装,从 WSL 起 vite 即可),该会话遗留的
  parcel-api-journey.exe(:19080)已清停。
  **验证种别(S,本轮实测)**:①复灌——seed.sh --reset 干净重灌全程零报错(37 行登记
  全落);②启用态——IDP_PARCEL_ISOLATED_READ_TENANT=SYN-TENANT-01 起 parcel-api,
  ADR-0078 启动日志在场,放行面 19 条参数化请求全 200(行数见脚本「取证」表),
  POST /shipment-requests 与 /customer-tracking-view 均 403 ACCESS_CHANNEL_NOT_CONFIGURED;
  ③页面层——WSL 起 vite(代理指 API),Edge 无头逐页 DOM 取证十页:七主数据页见 SYN-
  数据行、委托查阅与追踪投影诚实空态(LISTED/PROJECTIONS_LISTED,0 行)、工作台就绪度
  11/23/1,十页无一处未配置码。未设变量态与非 SYN- 前缀拒启态本轮未重跑,沿用票 05
  收口验证与 2b37b30 的三态实测记录。
  **「页面如实标注演示态」的落点**:数据在带 SYN- 前缀(PN-02 合成口径)、启用放行必写
  启动日志(ADR-0078)、脚本文档全文标注 S 与生产库禁令;页面组件不加横幅——合成性是
  环境属性而非页面属性(page-registry 的 demo 档是模块属性,七页属接线档),加横幅反而
  把「页面已接真端点」说成「页面是演示件」。
  **零代码改动**:本票全程只动 docs 与 .scratch,.go/.ts/.sql 零改,故无构建信号可报;
  委托侧的墙与重启条件照脚本第 5 步记录,本票不降墙。
