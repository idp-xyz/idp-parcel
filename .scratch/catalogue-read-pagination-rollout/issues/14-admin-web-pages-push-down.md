# 14 管理台各列表页改游标翻页、筛选与检索下推

Category: enhancement
Status: ready-for-agent
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

- **collection（10 的非作者评审提出）**：`CodLedgerPage` 今天的客户端检索会命中客户显示名，`/cod-subledgers` 的 `q` 只覆盖本册的标识列——客户名称在
  party-commercial，不在 collection-remittance 的表里。照「做什么」把检索下推到 `q` 之后，按客户名称搜会落空。接这一页前先定：保留按名称的页内补充匹配并在页上写明只对本页生效，
  或另立后端票给读面组合上名称列；不要不声不响地丢掉这项能力。
- **按客户名称检索的一般接法**：运单列表（14a 交回时撤掉了这项检索）与 COD 台账同属此类——客户名称在 party-commercial。读口声明了 `customerAccountId`
  筛选维的册，页面可以先按名称查客户账户目录（`/commercial-customer-accounts?q=`）取账户号，再以 `customerAccountId` 收窄；这是一个「选客户」控件，不是
  `q` 的一部分，要不要做、做在哪几页另行定。
- **network（07 交回）**：`route-plans.contract.test.ts` 读的两份后端契约夹具多了 `page`，管理台类型不认这个键，那条用例会红。**07 进 main 时须同批补上
  管理台的类型**，不能让 main 上的前端用例红着等这一步。另：复核页与建议页今天在页内按 `reassessedAt`、`suggestedAt` 排，读口缺省序是 `recordedAt`
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
