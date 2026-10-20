# 16 管理台不认「授权交付方」这一参与方角色：关系列表显示原码，登记表单的角色下拉里没有它

Category: bug
Status: resolved——2026-10-19 进 main（前端切片，在 `main` 上直接做），完成记录见文末。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 的作者子代理认领，照前端切片在 `main` 上直接做。此前 ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立并代决
Blocked by: 无
父票：[10](./10-authorized-delivery-relationship-in-party-commercial.md)
出处：票 10 完成记录「没做」。
地盘：
- apps/admin-web/src/pages/party/**（`presentation` 里的 `partyRoleLabels` 与依赖它的列表、表单、测试）

前端票：照 docs/agents/workflow.md「前端切片」在 main 上直接做。

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「yes」。

## 现象（读码）

票 10 在 PC 参与方关系的封闭角色集里加了 `AUTHORIZED_DELIVERING_PARTY`（授权交付方）。管理台的 `partyRoleLabels` 没有这个键：关系列表里这一角色显示原码，角色筛选与登记表单的下拉里没有它，配置员在页面上登不了授权交付关系。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决）

- `partyRoleLabels` 加这一键，中文显示名取「授权交付方」（与 PC CONTEXT、ADR-0212 的称呼一致）。
- 登记表单选这一角色时，适用范围要按票 10 定的唯一拼法填写：`customerAccount=<客户账户>`，可选再加 `&node=<节点>`（键序固定、值转义）。表单为这一角色给出「客户账户」「节点（可选）」两个输入，由前端拼成那一种拼法，不让配置员手写查询串；拼法放纯函数模块配 node:test。
- 其余角色的表单与列表行为不变。

## 完成判据

- [x] 三道门（Node 22）：`tsc -b --noEmit`、`node scripts/run-tests.mjs`、`vite build` 退 0。
- [x] node:test：显示名、筛选项与表单下拉含新角色；范围拼法纯函数覆盖只给账户、账户加节点、需转义的值、空账户被拒四格，拼出的串与票 10 后端 `TestAuthorizedDeliveryScopeHasOneSpelling` 认的那一种逐字一致。
- [x] 票面完成记录里判据逐条标 ✅ / ◑ / ❌。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的作者子代理，前端切片在 `main` 上直接做）

起点 main `a657bef1`。证据层级 **S**：夹具全为 `SYN-` 合成值，没连真 parcel-api 登过一条授权交付关系。

### 提交

| 提交 | 内容 |
|---|---|
| `5c7c9121` | `partyRoleLabels` 加 `AUTHORIZED_DELIVERING_PARTY: '授权交付方'`；关系列表与表单下拉的用例；票面 in-progress |
| `ba5e5cbd` | 新文件 `authorized-delivery-scope.ts`（`spellAuthorizedDeliveryScope`）与 `authorized-delivery-scope.test.ts` |
| 本笔 | 登记表单接线（草稿两格、载荷、本地问题、组件）与表单用例；本完成记录 |

### 落点

都在 `apps/admin-web/src/pages/party/`：

- `presentation.ts`：`partyRoleLabels` 末尾加一键；头注删掉「五格」这个计数，改称封闭集。关系列表角色列、角色筛选、登记表单下拉、参与方概览的角色排序都从这张词表派生，加这一键即一并认得。
- `authorized-delivery-scope.ts`（新）：`authorizedDeliveringPartyRole`（类型取 `PartyRoleCode`，词表改名时编译不过）、`spellAuthorizedDeliveryScope({ customerAccount, node })`。拼法：`customerAccount=<值>`，节点非空串时再接 `&node=<值>`；值照 Go `url.QueryEscape` 的转义表逐字转——只留 `A–Z a–z 0–9 -_.~`，空格写 `+`，其余按 UTF-8 字节写大写 `%XX`。不用 `URLSearchParams`：它不转 `*`、却把 `~` 写成 `%7E`，拼出来服务端不认。
- `party-relationship-form.ts`：草稿多 `scopeCustomerAccount`、`scopeNode` 两格；`deliveryScopeSpellingOf(draft)` 一处拼，载荷、本地问题与组件的「将送」都从它取；选授权交付方时载荷的 `scope` 取拼好的串，拼不出则键缺席（与修订号编不出同办），`PartyRelationshipRegistrationItem.scope` 因此改成可缺；问题报在 `relationships[0].scope`。其余角色照旧原样带 `scope`。
- `PartyRelationshipRegistrationForm.tsx`：按角色二选一渲染「适用范围 *」那一格。

### 表单形状

- 角色下拉末项「授权交付方 · AUTHORIZED_DELIVERING_PARTY」。
- 选了它，「适用范围 *」一格换成并排两个输入：「客户账户 *」（占位「如 SYN-ACCOUNT-01」）、「节点（可选）」（占位「留空即不收窄到节点」）。格下一句「持有方是交付方，相对方是这个客户账户所属的参与方；范围收到这个客户账户，可再收到一个节点。」，拼得出时接着显「将送 `customerAccount=…`」。
- 拼不出时问题显在这一格下：客户账户空白「客户账户要填：授权交付方的适用范围至少收到一个客户账户」；节点只填了空白「节点只填了空白：不收窄到节点就留空」；含编不成 UTF-8 的孤立代理码元各一句。提交按钮随之禁用。
- 切回别的角色，手填一格连同原值都回来；两格的残值不进载荷、不报问题。

### 代决

- 只填空白的节点按拒处理，不当成没填：服务端对空白节点也拒；悄悄丢掉等于改掉配置员填的东西。
- 首尾空白不裁、照转成 `+`：服务端不裁，与本表单各格「不裁首尾空白」同判。
- 关系列表的「适用范围」列照旧显原串，没有拆成账户与节点：裁定只要显示名，且「其余列表行为不变」。

### 判据逐条

- ✅ 三道门（Node 22.23.3，在 `apps/admin-web` 下）：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 597/597 退 0；`vite build` 退 0（只有既有的 chunk 大小提示）。
- ✅ node:test 显示名、筛选项与表单下拉含新角色：`party-relationship-list.test.ts` 的「授权交付方显中文显示名、按它筛得出行」「筛选与排序选项表」，`party-relationship-form.test.ts` 的「角色选项从 partyRoleLabels 派生」。
- ✅ 拼法纯函数四格：`authorized-delivery-scope.test.ts` 的「只给账户：customerAccount 一键」「账户加节点：customerAccount 在前、node 在后」「需转义的值照 Go 的转义表逐字拼」「空账户、空白节点与编不成 UTF-8 的值被拒」；表单接线另有「授权交付方：适用范围由两格拼，拼不出不送」「其余角色的适用范围照旧原样带」。
- ✅ 与后端 `TestAuthorizedDeliveryScopeHasOneSpelling` 逐字一致：前三格期望串是那个用例的原样。另用仓外探针经 `go test -overlay` 挂进 `internal/partycommercial/domain`（探针源与 overlay 都在仓外，未落仓；实测于 `ba5e5cbd`）：前端用例里拼出的六串，`ParseAuthorizedDeliveryScope` 全收、读回原值、`Reference()` 拼回逐字相同；`URLSearchParams` 那种写法（裸 `!'()*`、`%7E`）与空白账户、空白节点都被拒。后端原用例同一次跑也过。
- ✅ 票面完成记录逐条标注：本节。

### 事件路径（DOM 探针）

`apps/admin-web/scripts/dom-probe.mjs` 跑仓外探针，渲染 `PartyRelationshipRegistrationForm`，十一项全 ok：下拉有「授权交付方 · AUTHORIZED_DELIVERING_PARTY」；未选角色时是手填一格；change 选它后换成两格；账户空时报问题、提交按钮禁用；input 填 `A&B=C` 与 `N/1 2` 后显「将送 customerAccount=A%26B%3DC&node=N%2F1+2」、问题消失；填上关系标识后按钮可点，点下去截住的请求打到 `/commercial-party-relationship-registrations`，载荷 `role` 为 `AUTHORIZED_DELIVERING_PARTY`、`scope` 为上面那一串；另起一份表单选它再切回「客户」，手填一格回来。

### 未验

- 没在真浏览器里看，也没连 parcel-api 登一条授权交付关系走到 `REGISTERED`、再在列表里看到「授权交付方」那一行；列表显示名只由 `labelOf` 的用例证。
- CI：见 push 后的 run，不在本笔里。

### 评审

diff 只碰 `pages/party/**` 业务页，没碰 `templates/`、`shell/`、`components/` 共享面，按 workflow「前端切片」第 5 条自查判据，不另要评审。
