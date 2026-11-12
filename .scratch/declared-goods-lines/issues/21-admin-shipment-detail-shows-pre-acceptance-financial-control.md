# 21 管理台运单详情显示「受理前财务控制」

Category: enhancement
Status: in-progress
Blocked by: 19、20（均已提交，未进 main；同在分支 `feat/goods-details-admin`）

依据：[ADR-0379](../../../docs/adr/0379-the-pre-acceptance-estimate-trail-records-product-weight-and-charge-lines-and-is-read-on-the-shipment-detail.md) 决定四。

## 要做的

- 运单详情页加一块「受理前财务控制」，放在「受理进度」之后：
  - 结论：取委托详情的 `financialControl`（票 20），四种都显示：已冻结资金、已占用信用、受限、不适用（合同约定无财务控制）。受限项逐条写原因（可用余额不足、可用额度不足、逾期等，代码译成中文；没认出的照原样显示代码）。
  - 估价：取 `GET /settlement-pre-acceptance-estimates?shipmentRequest=`（票 19），逐提交版本一段：
    - 合计与币种；
    - 价卡（方案与版本，链到价卡详情）与服务产品；
    - 逐件包裹的计价重量与费用行；
    - `detailRecorded:false` 时写「这份估价形成于补记之前，没有逐件明细」。
  - 占用（可能两笔）：资金冻结或信用占用，状态用结算的词（已冻结或已占用、已释放、已扣费，部分扣费另写已扣金额）、金额与时刻。费用行的金额是小数原样，不走最小币单位的格式化。
  - 一句「受理前估价，不是实际费用；实际费用见费用页」。
- 估价没形成时（没有留痕），把受理任务 `lastAttemptReason` 上实有的停因码译成中文显示：今天到得了的是小包托运的待判原因（`FINANCIAL_CONTROL_*` 一类）；结算的 `SA-` 细原因今天到不了任务上（票 22），也先备好译法。认不出的码原样显示。「受理进度」里的停因同用这套译法。
- 金额按最小币单位与币种格式化，沿用费用页的格式化函数。

## 完成判据

三道门通过；纯函数（结论与原因译法、估价段落的组装、停因译法、金额格式）各有单测，覆盖通过、受限、两版估价、旧行无明细、没有留痕几种。

## 不做的

后端（票 19、20）；对客户显示。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/admin-financial-control-block` 上的 `9da274b53` 与评审修订 `34d9fc2b4`（基线 `a73304a3e`），全部改动在 `apps/admin-web/`，未推送、未进 main。后端（票 19、20）不在这棵树上，照钉死的形状写，没联调。
- 拣入：在 `feat/goods-details-admin` 上是 `6318c872f` 与评审修订 `ccbc0aff4`，叠在票 19、20 之后，`TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable` 在合并头上通过。
- 与票面不同的几处：
  - **停因不在任务上。** 票面与 ADR-0379 以为 `SA-` 停因记在受理任务的 `lastAttemptReason` 上；实测于 `a73304a3e`，任务上记的是小包托运自己的未决原因（`FINANCIAL_CONTROL_NOT_FORMED`、`FINANCIAL_CONTROL_NOT_CONFIGURED` 等），结算的细原因只折进续办引用的摘要，读不回；`未形成`那一停还会回滚重投，这次尝试可能不落库。做法：两族都译——任务上记的未决原因，与结算的 `SA-<未形成原因>` 及两种拒收（补到任务上之后这一族先认得；票 22 若带来别的码族，到时再补）；词表与 Go 源两个方向的对得上由契约测试核；「没有留痕」那一句按两族任一认出的财务控制停因写。把结算原因补到任务上另立（票 22）。
  - **估价读口的形状按评审后重钉的那一份。** `occupations` 是数组（组合策略可同时冻结与占用信用，空数组即没有占用）；状态按种类各一套（冻结 `HELD`、信用 `RECORDED`，各有 `RELEASED`、`CONVERTED`）；合计、逐件小计与占用金额是最小币单位整数串，费用行金额是主单位十进制串、逐行带币种。照票 19 现行的那份写；开工时拿到的是改钉前的样例。
  - **`outcome` 没钉具体的词。** 读口答复的 `outcome` 只说「随兄弟按键读口的惯例」，页面收成可选的串，不据它分支，只读 `estimates`。
  - **价卡链到价格表，不是某一版的详情。** 价格表没有按地址直接打开某一版的入口；链接把方案标识交作检索词（`#/price-card-catalog?q=<方案>`），筛出这个方案的各版本。
  - 有旧版本的估价而当前版本没有时，另说一句「当前版本的估价没形成（或还没有估价、或合同约定不做），下面是此前版本的估价」，免得把旧版本读成现在的。
  - 逐版本按试算时点新的在前（票面只说逐版本一段，没说次序）。
- 落点：
  - 新文件 `pages/shipment-request/financial-control.ts`：`financialControlConclusionOf`（已冻结资金、已占用信用、受限、不适用（合同约定无财务控制），认不得的照原码）、`restrictionReasonText`（可用余额不足、可用额度不足、账户逾期）、`controlItemText`（从 `recorded-judgments.tsx` 挪来，复核页输出不变；运单详情传 `withReason` 带上受限原因）。
  - 新文件 `pages/shipment-request/stall-reasons.ts`：`stallReasonOf`，译小包托运的未决原因与结算的 `SA-` 停因，标明哪些停在财务控制；认不得的照原码，`SA-` 前缀的仍算财务控制。运单详情的「受理进度」与复核页的「上次没推进」都用它，原码在各自的技术详情；与用语词典重合的码读成同一句。
  - 新文件 `pages/shipment-request/stall-reasons.contract.test.ts`：读 Go 源里的未决原因、结算未形成原因、施加那一路的两种拒收与两本账的限制原因码，两个方向核词表（Go 有而词表没有、词表有而 Go 已没有，都红并点名）。
  - 新文件 `pages/shipment-request/pre-acceptance-estimate.ts`：`estimateSectionOf` / `estimateSectionsOf`（逐提交版本一段：合计、价卡 `方案@版本`、服务产品、估价方式、逐件计价重量与费用行、旧行那一句、占用、技术行）、`occupationViewOf`（资金冻结中、信用占用中、已释放、已扣费，部分转扣费写「部分已扣费」与已扣多少，冻结、释放、扣费各时刻）、`normalisedEstimateOf`（数组格是 null 时先收成数组，Go 的 nil 切片会这样编）、`estimateBlockStateOf`（带上结论：在读、未配置、拒绝、调用方问题、没答或连不上、没有留痕、有留痕）。没有占用只说「没有占用」，当前版本且结论受限才加「控制受限」；结论不适用时说「合同约定无财务控制，不做估价」。
  - 新文件 `pages/settlement/money.ts`：费用页的 `formatMoney` 挪成纯 `.ts` 以便单测，`ops-ui.tsx` 照旧转出；加 `isPositiveMinorAmount`。
  - `pages/settlement/api.ts`：估价读口的类型与 `getPreAcceptanceEstimates`；`pages/shipment-request/api.ts`：`ShipmentRequestDetail.financialControl?`，`ReviewFinancialControlRecord` 补 `jointPassCondition?`、`occupationFormed?`。
  - `pages/party/ref-links.ts`：多一种引用 `price-card`。
  - 组件：`PreAcceptanceFinancialControl.tsx`（结论、估价与占用两半，估价一半自有加载、出错、重试与空态）、`use-pre-acceptance-estimates.ts`（各读各的，结算那一口读不到不让运单详情空掉）、`field-row.tsx`（详情页的键值行挪出来共用）。
  - `ShipmentRequestDetailPage.tsx`：「受理进度」之后加「受理前财务控制」一块；「最近处理结果」改用 `stallReasonOf` 的中文，原码进技术详情「最近处理原因码」。
- 判据：
  - 结论与原因译法：四种结论、没判过、认不得的结论；三种限制原因与认不得的原因；复核页那一行输出不变——单测 ✅。
  - 估价段落组装：一版的各格、两版按试算时点新的在前且只有当前版本标当前（两种输入次序、时点读不出各一）、旧行无明细（逐件只剩小计）、没有占用（当前版本且受限才说控制受限）、冻结与信用占用同时在场、认不得的码照原样、数组格为 null 不抛错——单测 ✅。
  - 占用写法：冻结中、信用占用中、已释放、已扣费、部分已扣费（冻结与信用各一）、扣过一部分再释放——单测 ✅。
  - 停因译法：任务上今天记的两格、`SA-` 的没有在用价卡、价卡适用冲突、输入不可得、财务控制以外的原因、认不得的码——单测 ✅。没有留痕时结论不适用说合同约定不做、按两族各说出停因、停在别处只说还没有；当前版本没有估价另说一句——单测 ✅。词表与 Go 源两个方向对得上、与用语词典重合的码读成同一句——契约测试与单测 ✅。
  - 金额格式：千分位不换算、币种空、缺席、非整数串原样；费用行照原样带小数——单测 ✅。
  - 读口按委托标识取、标识经地址编码——单测 ✅。
  - 三道门通过 ✅（见验证）。
- 未做：
  - 浏览器里没走过：这台机器上起不了带后端的页面，也没法看真实的版面。`dom-probe` 要先往仓外装 happy-dom，这次没装；改用仓外探针（vite 自带的 esbuild 束成 CJS，`renderToStaticMarkup`，估价那一问换成桩）渲了八种状态（在读且没判过、在读一行带 `role="status"`、受限且没有留痕带停因、两版含旧行、连不上带 `role="alert"`、需要登录带重新登录、数组格为 null、不适用），关键字句都在。事件路径（重试、展开其余几件）没证。
  - 联调等票 19、20；任务上的结算停因等票 22。
- 验证（`34d9fc2b4`）：`apps/admin-web` 下 `node node_modules/typescript/bin/tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 1391 过 0 败（基线 1358；`9da274b53` 时 1385）；`node_modules/.bin/vite build` 退 0。契约测试删掉一个词条时会红并点名那个码（在测试产物上改一次实测）。
  - Go 门：`TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable` 在票 19 的端点挂上 `cmd/parcel-api` 端点表之前红（管理台多了 `/settlement-pre-acceptance-estimates`）。本票与票 19、20 一起落，不单独进 main。
- 评审（非作者，2026-11-11）：阻断一条，是落地次序：上面那条 Go 门要等票 19，随 19、20 一起落。应改与小意见都在 `34d9fc2b4` 改了——估价数组为 null 时先收数组并加单测；结论传进估价判读（不适用、没有占用只在当前版本且受限时说控制受限）；失败态改用 `SectionError` 与 `OperatorRefusalNote`，在读一行 `role="status"`；加契约测试对着 Go 源核词表；占用按标识作键；包裹号表头取 `objectNouns`；补已释放带已扣金额与版本次序的单测、版本按试算时点新的在前；复核页同用停因译法；删去列调用方、叙述改动的注释；等待运营登记那一停没保存改成描述句；计价重量一格的注释照「未记」改。
