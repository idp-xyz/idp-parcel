# 14 管理台各列表页改游标翻页、筛选与检索下推

Category: enhancement
Status: in-progress——parcel-shipment 一笔（委托与渠道择优各页，分支 `cursor/crpr-14a-admin-shipment` tip `2ab4958c9`）已随 `098767b1e` 进 main；其余上下文的后端都已进 main，页面待接
Blocked by: 逐上下文跟随 02–13——某上下文的后端子票进 main 之后，才接该上下文的页面
地盘：`apps/admin-web/src/pages/**` 里消费 02–13 各册的 api 层与列表页；`apps/admin-web/README.md`「列表页上列通则」只在需要时改一句。
出处：ADR-0144 决定四、八；首批 [05](../../catalogue-read-pagination/issues/05-first-two-pages-push-down.md) 的接法（`pages/catalogue-query.ts`、
`pages/use-catalogue-cursor.ts`、模板 `pagination` 的 `mode: 'cursor'`）。

## 做什么

1. 各 api 层按 ADR-0144 带上游标、筛选维与 `q`，收答复的 `page`；列表页接游标翻页条（模板现成的 `mode: 'cursor'`，取数走 `use-catalogue-cursor`）。
2. 检索词沿地址上的 `?q=` 原样下推；已迁读口那几页撤掉客户端筛选，头注改写成现状。
3. 带计数与 `truncated` 的队列页：积压数照旧示出，「仅显示前 N 条」一类提示改由翻页条承担。
4. 答复 400 带理由散文时原样示出（沿既有 `problemNote`）。

按上下文分笔，每笔只接后端已进 main 的那几册：shipment-request（含 channel-selection）、visibility、pricing、operations（node 与 transport）、network（路线三页）、
settlement、customs、collection、party（客户账户之外）、governance、operator-access 与 work-devices。

## 后端子票交来的提醒

- **collection（10 的非作者评审提出）**：`CodLedgerPage` 今天的客户端检索会命中客户显示名，`/collection-subledgers` 的 `q` 只覆盖本册的标识列——客户名称在
  party-commercial，不在 collection-remittance 的表里。照「做什么」把检索下推到 `q` 之后，按客户名称搜会落空。接这一页前先定：保留按名称的页内补充匹配并在页上写明只对本页生效，
  或另立后端票给读面组合上名称列；不要不声不响地丢掉这项能力。
- **按客户名称检索的一般接法**：运单列表（14a 交回时撤掉了这项检索）与 COD 台账同属此类——客户名称在 party-commercial。读口声明了 `customerAccountId`
  筛选维的册，页面可以先按名称查客户账户目录（`/commercial-customer-accounts?q=`）取账户号，再以 `customerAccountId` 收窄；这是一个「选客户」控件，不是
  `q` 的一部分，要不要做、做在哪几页另行定。
- **network（07 交回）**：`route-plans.contract.test.ts` 读的两份后端契约夹具多了 `page`，管理台类型不认这个键，那条用例会红。**07 进 main 时须同批补上
  管理台的类型与那条用例的比对助手**（07 的非作者评审指出：`assertShape` 只认字符串叶子，而 `page.size`、`page.total` 是数字、`page.next` 可为
  `null`），不能让 main 上的前端用例红着等这一步。另：复核页与建议页今天在页内按 `reassessedAt`、`suggestedAt` 排，读口缺省序是 `recordedAt`
  （建议答复体为此新添了这一格），接这两页时以读口的次序为准。
- **pricing（04 交回）**：价卡状态是页面现算的，不是登记册上的列，服务端不能按它筛；要筛得先在读面导出这一列。`seriesId` 精确筛选维为「只看这一条序列」开，
  `q` 的包含匹配会带进同前缀的兄弟序列。
- **operations（05 交回）**：订舱状态在读口里派生，库里没有这一列，没有放成筛选维。
- **customs（09 交回）**：解释规则、口岸目录、申报路径目录三册同键多版改为旧版在前（决胜键随首列同向），这两页按答复次序上列，次序变化在页上看得见。
- **operator-access（13 交回）**：`/operators` 的 `q` 只覆盖主体标识与绑定依据，不含授予标识与能力面；管理台今天在页内也按这两项检索。接这一页前先定：只搜主体与依据，
  还是回 13 给 `q` 加子表的 `EXISTS`。
- **parcel-shipment（14a 交回）**：运单列表撤掉了按运单号、客户、件数的页内排序与状态计数——运单号可缺、件数在快照里，都不能作可排维（spec「可排维与行标识的列必须非空」
  「可排维、筛选维从小放」），状态计数在截断的一页上本来就不是真数。面单交易的渠道面单号检索，02 已补进 `q`，页面应保留这项检索。

## 不做

- 不动模板与外壳（`templates/*`、`shell/*`）；不改未迁读口的页面行为。

## 完成判据

- 每笔：三道门（tsc / run-tests / vite build）退 0；改到的 api 层有 node:test 钉住查询串的拼法与 `page` 的接收。
- 每个上下文至少一页在演示种子上「检索 → 翻页 → 换检索回第一页」走通（dom-probe 或浏览器，做不到如实写）。

## 完成记录 · parcel-shipment 一笔（含 channel-selection；2026-11-06，分支 `cursor/crpr-14a-admin-shipment`，基 `d6b3f64a5`）

基是 02 的分支 tip（`cursor/catalogue-pagination-rollout`）。02 还没进 main，本笔就在分支上做，等 02 落地后由推送方摘到 main。diff 只在
`apps/admin-web/src/pages/shipment-request/**`、`apps/admin-web/src/pages/channel-selection/**` 与本票面；没动 `templates/*`、`shell/*`、
`components/*`，也没动未迁读口的页面。

**落点**（笔 `c96b5cbda`；面单交易的检索提示回改在 `32704949c`）

| 页 | api 函数 | 下推什么 | 翻页条 |
|---|---|---|---|
| 运单列表 | `listShipmentRequestViews` | `q`（地址上的 `?q=`，停稳后）；状态胶囊 → `state`；表头「提交时间」旧→新 → `sort=submittedAt`（新→旧是缺省序，不带） | 工具条右端 |
| 接受前复核 | `listAcceptanceReviewQueue` | 只翻页 | 页底 |
| 授权处置 | `listAuthorizedDispositionQueue` | 只翻页 | 页底 |
| 释放拒收处理 | `listControlReleaseRefusals` | 只翻页；页头「待处理 N 笔」是整册积压 | 页底 |
| 受理后处置请求（货物处置上半） | `listPostIntakeDispositionRequests` | 只翻页；「待处置 N 票」是整册积压 | 队列下沿 |
| 待人工处理项（货物处置下半） | `listPostIntakeManualItems`；按包裹读回另立有界查找 `findPostIntakeManualItemsOfParcel` | 只翻页；「待办 N 项」是整册积压 | 待办清单之后 |
| 面单交易 | `listLabelTransactions` | `q` | 模板 `pagination` |
| 渠道择优并列冲突 | `listTiedChannelSelectionDecisions`，条件由 `tiedChannelSelectionConditions` 拼 | 选择器 `view=tied`，收窄时加 `scope`、`mapping`，不算检索条件 | 模板 `pagination` |
| 取消先后冲突 | — | 管理台没有这一页，本笔不新建 | — |

共用的改动：`shipment-request/api.ts` 的 `ApiResult` 改用 `catalogue-api` 那一份，4xx 带上 `detail`；新文件 `shipment-request/cursor-pager.tsx`
（`CursorPager`、`CursorPagerBar`、`pastFirstPage`）——`ReviewFlowTemplate` 与 `WorkbenchPageTemplate` 没有翻页槽，页面照 `ListPageTemplate`
那条翻页条的标记自摆一条，字句与按钮可否取模板导出的 `cursorPageSummary`、`cursorPagerControls`；运单列表的查询条件由 `shipmentListConditions`
一处拼；`refusalQueueViewState`、`dispositionQueueViewState`、`requestQueueViewState` 多收 `narrowed`。按运单号、按一组标识、按标识取单份
这些有界查找一字不变。

**判断项**

1. **运单列表撤掉的**：运单号、客户、件数三列的页内排序——读口能排的只有 `submittedAt` 与 `shipmentRequestId`，运单号列显的是单据号，
   按委托标识排对不上屏上的次序，所以不给排；状态胶囊上的计数与命令头指标——一页里数出来的不是各状态的总数，按状态的总数读口不给；
   按客户名称检索——名称从参与方册转写，读口 `q` 只覆盖委托号、单据号、来源请求键。读口另有 `customerAccountId` 一维，本页没有对应的
   筛选条，不另加。头注已改写成现状。
2. **面单交易**：渠道面单号检索由后端 `q` 覆盖（后端 `2ffc962a0`），检索框提示保持「搜包裹号 / 面单交易 / 渠道面单号」，检索词照旧
   下推、页内不再筛；`transactionResult` 一维本页没有筛选条，不另加。`page` 与 `total` 数的是交易，摘要因此分开报「本页 N 笔交易,摊开 M 行」。
3. **没有检索框与筛选条的页**（复核、授权处置、释放拒收、货物处置两份清单）只接翻页，读口收的 `q` 与筛选维不接，也不为此新加控件；
   票面「撤客户端筛选」在这几页无可撤。头注各写一句。
4. **翻过第一页答零行**：队列里的行处理掉之后重取当前页（游标不动）常会碰到，这不是册为空。上面那几页把 `pastFirstPage` 当 `narrowed`
   交给呈现态，留在就绪态，翻页条跟着留下好往回翻；待办清单那一行另说「这一页往后没有待办了,往前翻看看。」
5. **渠道择优的商业范围与映射**照 ADR-0144 当选择器带，不进 `catalogueConditionsNarrow`；它们在条件签名里，换收窄对象回第一页。
6. **`truncated`** 留在答复类型里（答复照旧带），页面一律读 `page`。
7. **400 的理由散文**：复核、授权处置、释放拒收、货物处置上半、面单交易、渠道择优经 `catalogueViewState` 接在问题码说明之后；运单列表
   自有呈现态，照同一做法接在 `problemNote` 之后；待办清单的错误行同样接上。

**完成判据**（作者自验，钉 `c96b5cbda`，WSL node v22.23.3）

- ✅ 三道门：`tsc -b --noEmit` 退 0；`scripts/run-tests.mjs` 1227 pass / 0 fail，退 0；`vite build` 退 0（只有既有的 vendor 块超 500 kB 提示）。
  面单交易回改之后在 `32704949c` 上重跑，结果相同。
- ✅ api 层 node:test：新 `shipment-request/catalogue-reads.test.ts` 钉运单列表与其余列表读口「不带参数即裸路径，带检索、状态、排序、游标
  一并下推」、筛选维按参数名逐值下推、有界查找的请求一字不变、`page` 原样交出、400 的理由散文进 `detail`（没给就没有这一格）；新
  `channel-selection/tied-decisions-read.test.ts` 钉选择器与检索词、游标同进一个查询串，按决定号取单份不变，`page` 原样交出。队列与运单
  列表的纯逻辑用例跟着改（整册积压数、`narrowed`、`shipmentListConditions`）。
- ✅ 运单列表在演示种子上走通「检索 → 翻页 → 换检索回第一页」（dom-probe 接真栈，32 ok / 0 fail，证据 `S`）。种子里没有运单，第二页又要
  多于一页大小（200）的行，所以在灌好的 `SYN-TENANT-01` 上经 `POST /v1/shipment-requests` 补交 205 票 `SYN-ACCESS-REQUEST-PAGE-001…205`
  （全部 201、已提交）：
  - 栈：`d6b3f64a5` 构建的 `parcel-api` 加一次性 `postgres:16.14`，`scripts/demo-seeds/seed.sh` 灌入退 0。页面挂在 `scripts/dom-probe.mjs`
    那一套 happy-dom 里，fetch 转给真 `parcel-api`；探针源在仓外。
  - 首屏发裸路径，「第 1 / 2 页 · 共 205 条」，200 行。逐字敲 `PAGE` 只发停稳后的一问 `?q=PAGE`，地址带上 `q=PAGE`。下一页带检索词与
    游标：第二页 5 行，与第一页不相交，合起来 205 个不同运单，末页「下一页」不能按。上一页回第一页，不带游标，行同第一次。
  - 在第二页换检索词 `PAGE-007`，不带旧游标，回第一页「第 1 / 1 页 · 共 1 条」，恰是那一笔；改回 `PAGE` 落第一页。零命中示「当前筛选
    条件下没有匹配的运单」，不报「还没有运单」。
  - 101 个字的检索词答 400，页面示「这次查询没有被接受」并原样接上「检索词 q 超过 100 个字符」。状态胶囊「待受理」下推 `state=SUBMITTED`，
    换胶囊不带旧游标，零命中留在就绪态。表头旧→新下推 `sort=submittedAt`，首行是最早那一笔。
  - 同栈 curl：`state=FOO`、集外的键、坏游标各答 400 带 detail。走完拆栈。
- ✅ 释放拒收处理页的翻页（dom-probe，替身传输，12 ok / 0 fail）：第一页「第 1 / 2 页 · 共 3 条」，页头只报「待处理 3 笔」、不再写「先列出」；
  下一页带答复的游标；第二页答零行仍留着翻页条，不报「没有积压」，页头随答复报「待处理 2 笔」，上一页能按；上一页回第一页、不带游标。

**未验与风险**

- 浏览器没走：`AuthGate` 要真登录，本笔只用 dom-probe。
- 要操作员凭据的那几页（复核、授权处置、释放拒收、货物处置、面单交易、渠道择优）没在真栈上走：种子里这几册没有行，本笔也没起操作员
  凭据。它们靠 api 用例与上面那条替身走查；「每个上下文至少一页」由运单列表满足。
- 模板缺口：`ReviewFlowTemplate` 与 `WorkbenchPageTemplate` 没有翻页槽，`cursor-pager.tsx` 照抄了 `ListPageTemplate` 那条翻页条的标记，
  模板补槽之后该删。归模板地盘，本笔不改模板。
- 积压在两次读之间变少时，翻页条会显「第 2 / 1 页」（`cursorPageSummary` 的字句，替身走查里见到）。归模板。
- `ListPageTemplate` 的两页（面单交易、渠道择优）照首批接法：不带检索时翻过第一页答零行落空态，翻页条随之不显。面单交易只增不减，碰不到；
  并列清单在翻页之间被人裁掉一批时可能碰到，重进页面即回第一页。
- 决定、登记之后重取当前页，清单先回加载态再出行（`useCatalogueCursor` 的 `retry` 如此）；旧版重取期间旧行留在屏上。
- 运单列表不再能按客户名称检索：读口 `q` 不覆盖这一列，要补归后端声明。
- 面单交易按渠道面单号检索没在真栈上走：页面只是照旧把检索词下推到 `q`，命中由后端 `2ffc962a0` 的 `TestLabelTransactionsKeywordReachesTheChannelParcelIdentifier` 证明。
- 运单列表 400 时接在前面的问题码说明取 `presentation.ts` 的 `problemCodeNotes`，那一句面向命令（「请求构造不出命令,……请检查填写内容后再提交。」），
  理由散文跟在后面；`catalogue-view` 的那一句面向查询。本笔没改。
- 工作台（`Workbench.tsx`）的待办计数不带参数调这几个读口，复核与授权处置两项数的是第一页的条目，不是整册积压。迁移前后一样，不在本笔地盘。
- `go test ./internal/architecture/...` 本机没跑（本笔只动前端）；全量 `scripts/test-full.sh` 归推送方在重放后的 tip 上跑。

**评审**：只碰 `pages/**` 与本票面，不碰 `templates/*`、`shell/*`、`components/*`，按 workflow「前端切片」评审那一步不要 Spec 轴评审，判据自查如上。
