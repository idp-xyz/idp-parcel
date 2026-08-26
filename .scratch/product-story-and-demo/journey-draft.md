# 演示动线筹备草案(票 04 的开工执行清单)

Category: draft
Status: 筹备半边产物,只读勘察,不是权威文档
取证时点:2026-08-25,HEAD `8cb43e4`(全文所有「当前」「共几个」类断言均锚此 SHA;执行半边开工时按票 07/08 收口后的树重验,不要把本文当那时的事实读)
落位追记(2026-08-26):执行半边已收口——权威动线脚本落
[docs/design/synthetic-demo-journey-script.md](../../docs/design/synthetic-demo-journey-script.md)
(2b37b30 落位,页面层取证与环境定因勘误随票 04 收口笔);本文只余取证史料价值,
断言一律以那份文档与当时代码为准。

本文是 [04-demo-journey](./issues/04-demo-journey.md) 开工时的执行清单,由 WSL 队列频道 7 按 2026-08-25 18:02 调度轮产出。票 04 维持 draft,执行半边候 master-data-wiring/07、08 收口。本文只报勘察事实与脚本草案,不替执行半边做任何裁决;与票面、ADR、CONTEXT 冲突时本文错。

两条勘察结论先行,全文其余内容都建在它们上面:

1. **`PAR-INT-01` 未配置下,`cmd/parcel-api` 的业务端点一律 403,主数据查阅入口也不例外。** `assembleBusinessEndpoints`(cmd/parcel-api/endpoints.go)的端点表在取证 SHA 上共 17 行,每行都装配各上下文的 `UnconfiguredIntake`,对一切请求答 403 + `ACCESS_CHANNEL_NOT_CONFIGURED`(ADR-0055;403 是诚实答案不是缺陷)。查阅读面也在 Intake 之后——作用域必须来自认证授权结果,采信自报租户会穿透 ADR-0003(见 `PricingCatalogueIntake` 的接口注释,ADR-0077 Decision 三同款)。生产代码里唯一的 Intake 实现就是未配置版;放行版只存在于传输层测试替身(如 `grantedCatalogueIntake`)。**因此七页的「数据态」在当前装配下不可达**:灌再多种子,页面拿到的仍是未配置态。这直接顶着票 07「(灌种子后)数据态」与票 08「七页可见种子数据」的完成标准,是执行半边开工要先解的第一件事(见第三部分末尾)。
2. **登记 CLI 只覆盖主数据与规则册;动线要的业务事实(委托、收寄、履约、投影、费用)当前无灌入口。** 六个登记口(计价、网络、关务、商业、VE、治理)灌的全是版本化登记册;能写业务事实的只有 parcel-api 的 POST 端点,而它们全在 403 后面。唯一先例是 `cmd/parcel-dispatch` 的合成测试夹具(SYN-V0 / SYN-PC-SEED,应用层+真仓储直写,只在 `go test` 里可运行)。明细见第二部分。

---

## 一、动线脚本草案(产品经理视角,合成 S,v1 读为主)

**环境前置**(引 compose.yaml 与各 CLI 约定,不复述细节):本机 `docker compose up -d`(postgres:16.14,127.0.0.1:55432,tmpfs——容器一停数据即空,复灌是常态而非例外);迁移由迁移作业施加(各 CLI 不自行迁移);`IDP_PARCEL_POSTGRES_DSN` 设向本机库;票 08 种子已灌;`cmd/parcel-api` 起着;admin-web dev 起着(vite 把 `/api` 代理到 parcel-api,无任何旁路或 mock)。

**角色与讲法约定**:主讲角色是**租户的物流产品经理**(本仓开发的是卖给物流企业的软件,产品经理是租户方角色);第 4、5 步分别切**客服/运营**与**结算/财务**视角。v1 读为主是票面已裁的分界:写动作由 CLI 侧演示,页面负责如实展示;页面如实标注演示态(demo/S),不冒充生产。每步「如实态」一栏写的是取证 SHA 上的可观察结果,执行时以当时的树为准。

### 第 0 步 · 开场:工作台

- **角色**:产品经理。
- **看哪页**:工作台(`workbench`,导航「总览」区;不在 page-registry 登记,由 Layout 直接渲染,反向读取登记处派生就绪度总览)。
- **讲哪句**:「管理台版图按业务价值链分区:受理→商业配置→计价→网络路由→作业履约→关务→追踪异常→结算核算。每个模块的就绪度从接线登记处派生——接线、骨架、演示三档如实分档,没有装出来的页面。演示数据一律是隔离合成 S,系统里没有任何真实企业。」
- **如实态**:工作台总览可看,分档真实。

### 第 1 步 · 建服务产品

- **角色**:产品经理。
- **CLI 侧动作**:`parcel-commercial publish -input <发布批 JSON>`——发布服务产品(`service-product`)及其引用的接单规则包(`acceptance-rule-package`)等商业权威依据;发布批逐项独立成败、重复返回原结果、冲突绝不覆盖。
- **看哪页**:服务产品与渠道(`service-products`,主数据区;票 07 范围)。
- **讲哪句**:「服务产品按不可覆盖版本受控发布——同键异内容登不进去,这不是报错是治理答案。页面只列版本壳,产品—渠道映射与授权不上列并如实说明(接线裁决如此,不是没做完)。」
- **如实态**:页面组件与未配置态在封存增量里已具备;数据态候读面准入裁决(结论 1)。

### 第 2 步 · 配价卡与线路(及其余主数据)

- **角色**:产品经理。
- **CLI 侧动作**(对应票 08 的四个登记口):
  - 价卡与序列:`parcel-pricing-register -kind price-card|reference-series -file <登记快照 JSON>`;
  - 网络:`parcel-network-register -kind node/connection/line/service-area/service-calendar/availability-adjustment/route-strategy -file <登记行 JSON>`(登的是版本骨架;地理覆盖等列还不存在,PAR-NET-14);
  - 合规规则:`parcel-customs-register interpretation-rule -input <JSON>`(及其余命令,见第二部分);
  - 商业策略:`parcel-commercial publish`(`price-rule`、`settlement-policy` 等类别)。
- **看哪页**(均票 07 范围,主数据区):价卡目录(`price-card-catalog`)、计价参考序列(`reference-series`)、网络目录(`network-catalog`)、服务区域与覆盖(`service-areas`)、合规规则库(`compliance-rules`)、商业规则与策略(`commercial-policies`)。
- **讲哪句**:「渠道与规则碎片化是这行的第一痛:每家渠道一套计费、一套轨迹口径,产品经理靠 Excel 管产品。这里价卡、序列、网络目录、合规规则、商业策略全部按版本受控登记,统一领域语言贯穿。网络目录与服务区域共用一个查询入口按族分派;服务区域的覆盖关系今天如实答『尚不存在』——没有的列不装有。」
- **如实态**:同第 1 步——页面具备、未配置态可演示,数据态候准入裁决。另注意 `parcel-network-register` 的文件注释明说:(租户+服务目的)网络定义登记册不在该口,三个证据视图照答未配置——SYN-V0 里初始路由停在 `ROUTE_EVIDENCE_NOT_CONFIGURED` 正源于此,讲解时不要把「目录已灌」说成「路由证据已配」。

### 第 3 步 · 一单委托从提交到终局

- **角色**:产品经理(下单侧叙事),可切客服视角讲查询作用域。
- **CLI 侧动作**:**当前无灌入口**(结论 2)。生产写路径 `POST /shipment-requests` 的 403 是刻意的;隔离环境合成接入渠道被 ADR-0055 Decision 五两项机制未决拦着(载荷规范化摘要、准入范围装配),那是 v2 的闸,本票不碰。
- **看哪页**:提交与撤回(`shipment-request`,已接线基线页)、委托查阅(`shipment-request-inquiry`,已接线基线页)、取消与收寄后处置(`cancel-parcel`,已接线基线页)。
- **讲哪句**:「这一步的第一讲点恰是这个 403:接入渠道没登记,系统就如实说未配置,不放任何『开发默认值』进来——上线那天不需要先排雷(PRODUCT-STORY『没有假默认值』)。委托的一生:提交成立→接受判断(判断不齐就诚实停在未决,不猜)→接受→收寄→履约→终局;每个判断带版本清单可回放。」链路走到哪有测试实证:SYN-V0(cmd/parcel-dispatch/synthetic_v0_test.go)从应用编排穿真仓储与 Outbox,经生产 Dispatcher 投到路由消费者,停在 `ROUTE_EVIDENCE_NOT_CONFIGURED` 的诚实未决——它不是全链闭环,讲的时候不冒充。
- **如实态**:两个查阅页在业务事实可灌之前没有数据态;演示只能讲未配置态本身+CLI/测试侧的链路实证。执行半边若要页面上看到「一单的一生」,先解第二部分的缺口。

### 第 4 步 · 追踪

- **角色**:客服/运营。
- **CLI 侧动作**:里程碑映射等五类规则走 `parcel-ve-register`(`milestone-mapping` 等六命令);投影本身**不是登记对象**——由 `cmd/parcel-dispatch`(常驻派生循环,环境变量定节拍)从库内业务事实派生,事实进不去投影就无从派生。
- **看哪页**:全程追踪(`tracking-projection`,已接线基线页)。
- **讲哪句**:「每家承运商一套状态码是第三痛。这里里程碑映射版本化登记,内部投影与客户视图分层,披露按客户合同分级;映射不到的事实如实标未归类,不硬凑时间线。」
- **如实态**:同第 3 步,受业务事实缺口牵连。

### 第 5 步 · 对账

- **角色**:结算/财务。
- **CLI 侧动作**:无——settlement-accounting 在 `cmd/` 下没有任何入口(结论 2)。
- **看哪页**:对账单(`reconciliation`)、费用与计费(`charges-billing`)——两页均为骨架占位(`UnwiredModule`,不在 liveIds),页面如实写明未接线与出处。
- **讲哪句**:「对账靠人肉是第五痛。机制上:供应商账单到达不等于应付,匹配与审核分步;客户对账单按截单快照发布密封;真实收付显式映射后才核销(settlement-accounting CONTEXT 口径)。今天这两页是诚实占位——版图先立,页面按闸门逐个补,占位页也标着主责上下文与文档出处。」
- **如实态**:v1 此步以版图与机制讲解为主,无页面数据;不属票 07/08 范围,是否补接线由后续票裁。

---

## 二、种子缺口清单

票 08 只灌主数据(产品/价卡/参考序列/网络目录/合规规则/商业策略,经四个登记 CLI)。下表盘点动线还需要的业务事实与每项的灌入事实。「入口」一栏只报勘察到的,查不到的如实标**当前无灌入口**,不发明机制;要不要立新工具票、还是把动线收敛到现有面,归执行半边与调度裁。

### 已有灌入口的(主数据与规则册)

| 对象 | 入口(cmd/) | 命令/种类 | 输入 | 要点 |
|---|---|---|---|---|
| 价卡、参考序列 | parcel-pricing-register | `-kind price-card` / `reference-series` | `-file` 登记快照 JSON(领域折装产物) | 重建门入库前拒;退出码 0/1/2/3(2=治理答案) |
| 网络目录七族 | parcel-network-register | `-kind node/connection/line/service-area/service-calendar/availability-adjustment/route-strategy` | `-file` 登记行 JSON,未知字段拒 | 登版本骨架;0007 网络定义登记册**不在此口**,证据视图照答未配置 |
| 关务配置面六册 | parcel-customs-register | `readiness-register/readiness-revoke/authority-grant/authority-revoke/interpretation-rule/obligation-catalog/obligation-item/gate-catalog/gate-finding/case-requirement`(封闭十命令) | `-input` JSON,未知字段拒 | 撤销是状态推进不是删除;同键异内容绝不覆盖 |
| 商业九类权威依据 | parcel-commercial | `publish`(类别封闭集:`service-product/customer-contract/supplier-agreement/acceptance-rule-package/pre-acceptance-financial-control-policy/price-rule/settlement-policy/credit-policy/authorization-rule`);另有 `register-resolution-key` | `-input` 发布批 JSON | 逐项独立成败、重复重放、冲突不覆盖;退出码 0/1/2 |
| VE 五类规则七表 | parcel-ve-register | `milestone-mapping/triage-rules/notification-policy/claim-eligibility/claim-authorization/disclosure-policy` | `-input` JSON | 版本不可覆盖,无幂等重放格;执行者身份双轨留痕 |
| 治理三类 | parcel-governance-register | `authority-interval/suspend/resume` | `-input` JSON | 票 12 首批;阶段评审与接管不在此口 |

以上六口共同纪律:DSN 一律取 `IDP_PARCEL_POSTGRES_DSN`(未设即拒,不猜连接串);不自行迁移;不内置任何生产默认。种子命名走 SYN- 前缀(对齐 PN-02 合成任务包),证据层级只记 S。

### 当前无灌入口的(业务事实)

| 动线要的事实 | 唯一在库路径 | 该路径现状 | 既有先例(只在测试里可运行) |
|---|---|---|---|
| 委托(提交/撤回/取消) | `POST /shipment-requests` 及两个子路径 | 403(未配置 Intake) | SYN-V0 夹具经 PS 应用编排+真仓储直写(cmd/parcel-dispatch/synthetic_v0_test.go);SYN-PC-SEED 配套种商业闭包与收寄资格(syn_pc_seed_test.go) |
| 收寄(节点收寄登记) | `POST /node-operations/receptions` | 403 | node_intake_projection_test 等 dispatch 投影测试夹具 |
| 履约(交付登记、POD 更正) | `POST /transport-fulfillment/deliveries`、`.../delivery-proof-corrections` | 403 | intake_then_delivery_projection_test、effective_delivery_* 测试夹具 |
| 追踪投影 | 非登记对象:由 cmd/parcel-dispatch 从库内事实派生 | 循环可跑,但上游事实进不去 | 同上各投影测试 |
| 索赔 | `POST /claims` | 403 | 无 CLI |
| 关务外部结果 | `POST /customs/external-results` | 403 | 无 CLI(parcel-customs-register 只灌配置册,不灌案件事实) |
| 费用/计费/对账(settlement-accounting) | 无任何入口(cmd/ 下无 settlement 进程,parcel-api 无其端点) | — | 无 |

**对票面「全链业务事实由登记 CLI 灌成套合成 S 种子(…委托、收寄、履约、投影、费用)」这句的如实核对**:在取证 SHA 上,后五类没有登记 CLI。执行半边开工时二选一(或另裁):①立新票给业务事实开受控灌入口(形状之辩不在本文);②把 v1 动线收敛为「主数据六类页面见数据态(候准入裁决)+业务链以 CLI/测试实证与未配置态讲解」。本文不预判。

---

## 三、当前可用面盘点

### page-registry 的接线事实(锚 `8cb43e4`)

- **已验收基线**(封存前一版,即 `8cb43e4^` 的 `liveIds`,与 PRODUCT-STORY「今天能演示什么」一致,共四项):委托提交(`shipment-request`)、委托查阅(`shipment-request-inquiry`)、逐包裹取消(`cancel-parcel`)、运营追踪投影(`tracking-projection`)。
- **HEAD 现状**:`8cb43e4` 是对原主(MCP-6)未提交增量的**封存提交,非集成候选**(提交信如此,mtime 止于 15:15:02)。它把七页加进了 `liveIds`(合计 11 项)并带着七页组件、api 与词表(13 份文件)。票 07 仍 in-progress,已改派 WSL 队列频道 3 续工;**这七项转为已验收事实要等票 07 收口**,在那之前不要把 HEAD 的 liveIds 读成「七页已接线完毕」。
- **票 07 完成后新增的七项**:价卡目录(`price-card-catalog`)、计价参考序列(`reference-series`)、网络目录(`network-catalog`)、服务区域与覆盖(`service-areas`)、合规规则库(`compliance-rules`)、服务产品与渠道(`service-products`)、商业规则与策略(`commercial-policies`)。
- **演示档**:模板预览(`template-preview`,隔离合成 S,与业务区分开列)。
- **动线沿途会路过的骨架页**(诚实占位,UnwiredModule):接受前人工复核、面单交易、价格评价、路由计划与改路、节点作业查阅、运输履约查阅、关务案件与申报、异常分诊/案件/索赔追偿、费用与计费、对账单、收付款核销、经营核算、代收分户账、阶段决定与暂停恢复等——完整清单以 navigation.ts 与 page-registry 为准,此处不复述第二份。

### `PAR-INT-01` 未配置下的 403 清单(引 ADR-0055,403 是诚实答案)

`PAR-INT-01`(客户生产委托接入渠道)在参数登记册中为**待提供**。取证 SHA 上 `assembleBusinessEndpoints` 端点表的全部路径——命令面与查阅面、业务事实与主数据目录,无一例外——对一切请求答 403 + `ACCESS_CHANNEL_NOT_CONFIGURED`:

```
/shipment-requests            /shipment-requests/withdrawals   /shipment-requests/parcel-cancellations
/shipment-request-views       /node-operations/receptions      /transport-fulfillment/deliveries
/transport-fulfillment/delivery-proof-corrections              /customer-tracking-view
/tracking-projections         /claims                          /customs/external-results
/pricing-price-cards          /pricing-reference-series        /network-catalog(?family= 两页共用)
/customs-compliance-rules     /commercial-service-products     /commercial-policies
```

例外只有进程自身的 `/healthz` 与 `/version`。前端把 403+该码译成头等的「未配置态」(pages/catalogue-api.ts 的结果代数),四个已接线基线页与七个在途页同款——所以「起环境→开页面→看到未配置态」本身就是可演示的诚实,不是坏状态。

### 对执行半边的含义(开工清单)

1. **先裁「隔离环境读面如何达成数据态」——它是 v1 的前置,不是 v2 的。** 票面 v1/v2 分界把「合成接入渠道从页面发起**写**」划给 v2;但取证事实是**读**面同样在未配置 Intake 后面(结论 1),而票 07「数据态」、票 08「七页可见种子数据」都假设读得到。三者顶在一起,需要一次明确裁决(票内决定或小 ADR:读面准入按什么机制在隔离环境放行、S 级如何标注、与 ADR-0055 Decision 五两项未决的关系)。本文只指出顶着,不代裁。
2. **候票 07(频道 3)、票 08(频道 5)收口**,以收口 SHA 重验本文第三部分的接线事实与第二部分的 CLI 清单。
3. **业务事实灌入口缺口**按第二部分结论处理(新工具票或动线收敛),裁定后动线脚本第 3–5 步的「如实态」栏重写。
4. **脚本文档落位**:票面要求动线脚本落一份文档、位置开工时定(docs/design 或随管理台文档)。本文是 .scratch 草案,不占那个位置;届时按「改文档」纪律新增并在 docs/README.md 补索引行。
5. **复灌验证**:tmpfs 库一停即空,完成标准里「可复灌」天然要求种子脚本幂等或可重放——各登记口的退出码语义(0 含幂等重放/2 治理答案不可覆盖)在脚本里要分开对待,ve-register 无重放格,复灌脚本对它要么换版本号要么容忍退出码 2。
