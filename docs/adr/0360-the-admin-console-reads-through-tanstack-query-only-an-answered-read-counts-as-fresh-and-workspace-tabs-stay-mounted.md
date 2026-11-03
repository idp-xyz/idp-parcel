# ADR-0360：管理台取数层用 TanStack Query——答案按「答到了才算新鲜」缓存，键以绑定租户打头、写后按键作废；工作区标签保活，离开满新鲜期回来按新开页重挂

Status: Accepted（2026-11-03，用户在 Claude Code 会话中与会话商定：起因是用户报公司主体换行、换标签时闪一下，会话比对 idp-prism 的做法后提出取数层、新鲜期、标签保活与渐进迁移，用户认可。无专票，本记录即落点。不停用任何已接受 ADR；改的是前端票面落下的壳层取舍，见「对既有票面的作用」。）<!-- 索引注记 -->｜**取法**：`@tanstack/react-query` v5，`main.tsx` 一个 `QueryClient`，查询函数照旧答 `ApiResult` 不抛；只有 `outcome` 算新鲜，新鲜期 30 秒与标签规则共用一个常量，其余答案即刻过期，不自动重试、不随窗口聚焦重读；键依次为绑定租户、资源、参数，写口答到后作废受影响的键；标签保活，新鲜期内回来原样、过了重挂；参与方先迁并删手写缓存，其余域随改随迁
Date: 2026-11-03

## Context

取证于 main `4e2a28f1b`（`apps/admin-web`，React 19 + Vite）。

- **没有共用的取数层。** 页面各自在 `useEffect` 里取数：`pages/party/register-list.ts` 的 `useRegisterList`、`pages/party/PublicationFormFields.tsx` 的 `useLoaded`，其余多是页内现写。「什么时候再读一次」每页各定一份。
- **防闪烁的手写缓存在长。** 为止住公司主体换行、换标签的闪烁，先后加了 `pages/party/legal-entity-read-cache.ts`（按版本缓存、同读并一、邻行预取、注册号类型目录另缓一份、写后 `invalidateLegalEntityReads`），`useRegisterList` 的 `remember` 选项（跨挂载记住上一份答案），`pages/use-refdata-names.ts` 的模块级存储。这几样正是查询库的本职：缓存、同读并一、先显旧答案后台再读、预取、写后作废。每多一页闪就多一份手写，各带一套新鲜规则。
- **闪烁另一半来自壳层。** `Layout.tsx` 给 `EditorGroup` 传 `preserveInactiveTabContent={false}`，切走即卸载，回来是新挂载：本地状态清空、答案从空开始重读。`.scratch/workspace-tab-view-state` 把已接入页的呈现状态存回来了，答案仍重读，空→有那一帧还在。
- **姊妹产品不闪。** `idpxyz/idp-prism` 用同一组 `@idpxyz/*` 界面包与同一个 `EditorGroup` 工作区，取数用 `@tanstack/react-query` v5：`staleTime: 30_000`、`refetchOnWindowFocus: false`、只对可重试的错误重试，`preserveInactiveTabContent` 开。
- **本仓的读口不抛。** 每个读答 `ApiResult`（`pages/catalogue-api.ts`）：`outcome`，或 `unconfigured` / `loginRequired` / `notGranted` / `callerProblem` / `noAnswer` / `transport`。代码里已有红线：一次读不到不能当成没有（`pages/shipment-request/shipment-ref.ts`「读口没形成答案,不能当成「没有」」）。查询库把「没抛」读成成功、按成功缓存——原样套上，一次 `transport` 会被当成新鲜答案留满新鲜期。
- **租户。** 演示租户 `SYN-TENANT-01` 按真实租户对待（ADR-0150）；管理台的绑定租户由 `pages/operator-binding.ts` 的 `boundTenantOf` 读出。缓存不带租户，换绑之后就会把上一个租户的答案显给下一个。

## Decision

**一、取数层用 `@tanstack/react-query` v5。** `main.tsx` 建唯一一个 `QueryClient`，包在应用根上。查询函数照旧答 `ApiResult`、不抛：非 `outcome` 是要显给人看的答案（未配置、要登录、无授权、没连上），不是异常，页面照今天的分支画它。（机制）

**二、只有答到了才算新鲜。** `outcome` 在新鲜期内算新的，不再问；其余答案留在缓存里也即刻过期，下次用到就再问，从不显成「没有记录」。重读在途时照旧显上一份 `outcome`；上一份不是 `outcome` 的，在途时按加载画（按了重试要看得见反应）——与 `registerListReducer` 同一语义。新答案到了就替换，哪怕这次是读不到：页面如实显这次的失败，不拿旧的 `outcome` 顶。新鲜期 30 秒，定为一个常量（`query/freshness.ts` 的 `answerFreshForMs`），查询层的 `staleTime` 与决定四的标签规则都用它：两处各定一个，人会碰到「这页刚刷过、那页回来却是半分钟前的」。`refetchOnWindowFocus` 关；自动重试关——非 `outcome` 是答案不是异常，再问由人（刷新键、续办）或下一次用到触发。（产品策略；新鲜期与不随窗口聚焦重读与 idp-prism 同值，重试不同：idp-prism 对可重试的错误重试一次）

**三、键以绑定租户打头，写后按键作废。** 查询键依次是绑定租户 id、资源、参数，换租户就换了一整片键，不会串读。登记、发布等写口答到之后作废受影响的键（`invalidateQueries`），不再手传 `refreshKey` / `retry` 去逼页面重读。（机制）

**四、工作区标签保活，离开满新鲜期回来重挂。** `EditorGroup` 的 `preserveInactiveTabContent` 开。新鲜期内回到一张标签，原样；离开满新鲜期再回来，这张标签按新开页重挂——重挂即重读，与保活之前每次回来的结果相同，已迁到查询层的页先显缓存的答案、后台再读。判据是 `shell/workspace-state.ts` 的 `returnNeedsFreshMount`。保活随附：

- 藏着的页仍听得到每次地址变化，只认落在自己这张标签名下的（`hashBelongsToTab`；页面经 `templates/use-own-address-change.ts` 的 `useOwnAddressChange` 听）。否则列表标签听到自己那张详情标签的地址，会在背后翻成详情。
- 检查器只由活动标签提供。
- 列表的多选集活到那张列表页被重挂为止：新鲜期内进详情再回来仍在；离开满新鲜期回来重挂即丢。
- 工作台仍不是标签（不给它一个假的「×」）。没有活动标签时，把工作台画进标签内容区：`@idpxyz/ui-workspace` 的保活模式在还有标签时不显 `emptyStateContent`。这是上游的缺口，正修归 idp-ui；本仓的画法是绕行，上游补上后撤掉。

（机制；「新鲜期内回来原样」是产品策略）

**五、渐进迁移。** 参与方域（`pages/party/**`）先迁，迁完删掉它的手写缓存（`legal-entity-read-cache.ts`、`useRegisterList` 的 `remember`）；其余域下次有人改到时再迁。不一次重写。（开发顺序，不属三类）

本记录没有租户取值。

## Alternatives considered

- **每页继续手写缓存。** 否决：新鲜规则有几页就有几份，同读并一、写后作废各写一遍；闪烁修好一页、换一页又冒出来。
- **SWR。** 否决：idp-prism 已统一在 TanStack，两个产品共用界面包与工作区，取数层分两家就是两套心智；写后作废与写口一侧的支持也弱于 TanStack 的 `invalidateQueries` / `useMutation`。
- **标签照旧卸载。** 否决：每次回来都重建、重读，正是闪烁的源头；呈现状态能存回来（workspace-tab-view-state 做过），空→有那一帧存不回来。

## 对既有票面的作用

不停用任何已接受 ADR。改前端票面落下的壳层取舍；票面是历史记录，不改写，后来者以本记录为准：

- `.scratch/admin-web-workspace-form/issues/01-shell-editor-group-tabs-status-bar.md` 给 `Layout.tsx` 定 `preserveInactiveTabContent` 关，理由是「每张页各自 fetch，留着会攒请求；myshop-web 演示数据无此顾虑，我们有」：改为开。那时的顾虑由决定二、四接住——藏着的页不随窗口聚焦重读，只听自己的地址，离开满新鲜期回来才重读；取证时各页也没有轮询。
- `.scratch/admin-web-workspace-form/issues/06-list-detail-roundtrip-keeps-page-state.md` 否了「列表标签保活」，理由是「保活的非活动页仍挂着全局 hashchange 监听，会把别的标签的地址当成自己的」：决定四「只认落在自己这张标签名下的」那条接住这一条——页面不再直接挂监听。同票定的「多选集进详情即丢」（票 04 随之改口）改为决定四「列表的多选集活到那张列表页被重挂为止」那条：它丢的理由是「跨标签留着，反而会把重取后已经变了的行留在选择集里」，而新鲜期内列表没有重读，行没变；一过新鲜期就重挂，选择集照旧丢。
- `.scratch/workspace-tab-view-state/spec.md`「保留该卸载策略，工作区按标签保存已接入的临时呈现状态，回到标签时恢复；业务答案仍重新读取。」：卸载策略不再保留。新鲜期内回来连答案一起原样；离开满新鲜期回来重挂，按标签存的呈现状态照旧恢复。

## Consequences

- 打包体积增约 13 kB（gzip）。
- 迁移期两种取数方式并存，读一页要先看它在哪一边。
- 藏着的页常驻内存，开着的标签越多占得越多。
- 新鲜期内快速切回，看到的答案最多是 30 秒前的；刷新键照旧在。
- 代码与注释里按「非活动标签卸载」写的说法随实现改口。
- 上游跟进：`@idpxyz/ui-workspace` 保活模式下的空状态归 idp-ui 立票；补上后撤本仓的绕行。
- 代码改动全在 `apps/admin-web/**`，按 workflow「前端切片」落；本记录照「进 main 的步骤」，带非作者评审。

## Links

- [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：演示租户按真实租户对待，决定三的键以绑定租户打头
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类，各决定所标类别的判据
- [workflow.md「前端切片：一人在 main 上直接做」](../agents/workflow.md#前端切片一人在-main-上直接做)
- 票 [admin-web-workspace-form/01](../../.scratch/admin-web-workspace-form/issues/01-shell-editor-group-tabs-status-bar.md)：标签卸载的原取舍，决定四改为保活
- 票 [admin-web-workspace-form/06](../../.scratch/admin-web-workspace-form/issues/06-list-detail-roundtrip-keeps-page-state.md)：否了列表标签保活、定了多选集进详情即丢，决定四接住与改口
- 规格 [workspace-tab-view-state](../../.scratch/workspace-tab-view-state/spec.md)：按标签存回呈现状态，重挂时照旧用
- `idpxyz/idp-prism`：同一组界面包上的 TanStack Query 先例
