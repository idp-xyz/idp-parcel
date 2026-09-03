# 04 货主客户账户册有写口无读面——身份三级里唯一看不见的一级

Category: enhancement
Status: in-progress——MCP-6（2026-09-03）。落点已裁：**客户与合同页（`party-contracts`）加「客户账户」签**，不并进业务参与方页、不新开页，依据见 Comments 首条
Blocked by: 无。它**阻塞**票 [02](./02-remaining-registries-take-online-registration-faces.md)
的 `customer-account` 登记签，以及 `identity-deactivation` 的第三格

## 缺什么

货主客户账户是 ADR-0003 三级边界的第三级（运营集团租户—责任法人—货主客户账户）。它有登记
用例、有受控 CLI、有在线写口 `/commercial-customer-account-registrations`（已在端点表），
**唯独管理台没有任何一页读它的册**。

取证于 `0d07866`：`apps/admin-web/src` 全树里 `customerAccountId` 只作外键出现在委托列表与
详情、路由计划页、受理复核页；没有 `CustomerAccountRecord` 这样的行形状，`pages/party/api.ts`
里也没有与 `listBusinessParties` / `listGroupLegalEntities` 并列的 `listCustomerAccounts`。

## 它是从哪里漏掉的

不是哪条判据把它排除了，是没人数到它——形状与票 02 里 `case-requirement` 被数漏那一格相同。

[`admin-remainder-mechanism-batch/01`](../../admin-remainder-mechanism-batch/issues/01-party-identity-lifecycle.md)
的标题是「集团法人**与货主客户账户**的登记机制」，「做什么」第一条也写着三层结构照 ADR-0003。
但它 2026-08-28 的补格裁定处理的是**业务参与方身份本体册**，实施交付的读面是法人册、参与方
身份册、关系册。客户账户那一册在标题里、在 ADR 里，不在交付里，而那张票已 `resolved`。

**不要去改那张票**：它自己那一格（身份本体册不可见）确实补完了，判据与测试都成立。本票承接
的是它标题覆盖、范围未及的第三级。

## 后果两处

1. 身份三级里唯一登记成功却无处可看的一级。`admin-remainder-mechanism-batch/01` 补格裁定
   写过一句判据，逐字适用于此：**身份本体不可见等于那个生命周期不可观察**。
2. 直接卡住票 02 两格。`customer-account` 登记签因此本批不摆（退无可退，摆哪都是登进去之后
   没有任何页面能证实它生效）；`identity-deactivation` 已裁摆业务参与方页，但快照里
   `CUSTOMER_ACCOUNT` 那一种身份停用之后，结果同样哪都看不见。

## 要裁什么

**落哪一页**，两个候选，判据是页面所有权语言（`navigation.ts` 的 `moduleInfo` 原句）而不是
「哪页还有空位」：

1. **并进业务参与方页第三签**——该页今天两签（参与方身份 / 参与方关系），所有权语言说的是
   「角色中立的业务参与方身份」。要先确认货主客户账户算不算它说的那种身份：它是**租户的客户**
   的账户，与业务参与方是不是同一层，这句得从 `CONTEXT.md` 取原话，不能凭页面还有位置就并。
2. **新开一页**——若上一条答否，则它需要自己的落脚点。新开要动 `page-registry.tsx` /
   `navigation.ts` / `liveIds`（第三类共享接线文件，MCP-3 统一加行）。

先答第一问再动手。`admin-web-page-wiring-frontier/04` 那张裁决票的教训在这里成立：**先接后想
会得到一批按登记表结构长出来的页面**，那是让库表形状决定产品形状。

## 做什么

形照 `admin-remainder-mechanism-batch/01` 的「补格实施」那一节，逐件对应：

1. `ports` 加客户账户行形状与列表读法。**注意它有没有 `HasPartyName` 那一格**——身份本体册
   没有（名称在本册行上），法人册与关系册有（要左连接）。客户账户属哪一种，按它自己的表定，
   不照抄。
2. `adapters/postgres`：`DISTINCT ON` 取最新修订，status 的 `CASE` 与另两册**逐字相同**
   （同一个 `domain.IdentityLifecycle`，判据不该有第二种写法）。
3. `adapters/http` 端点 + `cmd/parcel-api` 端点表、探针、隔离读放行表、unwired 占位各加一行
   （读行入放行表，写行不入——两侧的判据见 ADR-0078 与票 02 红线）。
4. 页面：按上面裁定的落点。

## 完成判据

- 生命周期三格各有实例可显，钉法照
  `TestBusinessPartyCatalogueShowsAllThreeLifecycleCells`：未来生效（`REGISTERED`）、
  已过生效时点（`EFFECTIVE`）、带停用两件（`DEACTIVATED`），外加跨租户零行与 limit 非正拒。
- 全仓 `gofmt -l` 无输出、`go build`、`go vet` 绿、`go test -count=1 ./...` 绿（注明含不含
  真库）；`apps/admin-web` 的 `tsc --noEmit` 无输出（`pnpm build` 照票 02 已定的措辞如实记）。
- **落地后回票 02 收两格**：`customer-account` 登记签摆上，`identity-deactivation` 的
  `CUSTOMER_ACCOUNT` 结果有处可看。回填时在票 02 记一条，不在本票另立第二套口径。

## 参照

[ADR-0003](../../../docs/adr/0003-group-tenant-legal-entity-customer-account.md)、
[ADR-0085](../../../docs/adr/0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)；
[`admin-remainder-mechanism-batch/01`](../../admin-remainder-mechanism-batch/issues/01-party-identity-lifecycle.md)
的补格裁定与补格实施两节（本票的形状出处）。

## Comments

- 2026-09-03 MCP-6：认领。**开工前重核（于 `13c87bd`）**：`apps/admin-web/src` 全树仍无
  `listCustomerAccounts` / `CustomerAccountRecord`（票面结论成立）；`/commercial-customer-account-registrations`
  在 `cmd/parcel-api/endpoints.go` 端点表上（写口成立）；`customer_account_registration` 表在
  `0015`，列为 `account_id / customer_party_id / basis_ref / effective_from / deactivated_at /
  deactivation_basis`，**无状态列**（状态按装载时点导出，与另两册同）。

  **落点裁定：客户与合同页（`party-contracts`）加「客户账户」签。** 判据照票面——页面所有权
  语言原句，不是哪页有空位。

  **一、不并进业务参与方页。** 该页 `moduleInfoById['business-parties'].source` 原句：「角色中立
  的业务参与方身份，以及……参与方关系与渠道账号的持有人」。PC CONTEXT「货主客户账户」词条：
  「运营集团租户内**面向一个货主客户**建立的业务隔离与商业关系边界。它不是运营法人、经营组织
  或登录用户」；Rules：「责任法人和货主客户都使用稳定业务参与方身份；货主客户账户必须明确
  关联其客户参与方……参与方、账户、法人和合同标识不能互相替代」。账户**引用**一个参与方身份，
  定义里就带着角色（货主客户），所以它不是那页说的角色中立的身份本体。先例同向：责任法人
  同样「同时具有业务参与方身份」（CONTEXT），却落在自己的「集团与法人」页而不是业务参与方页
  的一签——三级边界的第二级没有并进去，第三级也不该并。

  **二、票面漏了第三个候选，而它正是被所有权语言点名的那页。** `moduleInfoById['party-contracts']`
  （标题「客户与合同」）的 `source` 原句：「**客户账户**、客户合同与供应商商业协议的独立版本
  生命周期」——客户账户是第一项，页名里的「客户」就是它；页面 description 也自认「货主客户账户
  与责任法人尚无登记面，不上列」，即登记面出现后它就该上这页。CONTEXT「客户合同版本」词条：
  「运营企业责任法人与明确货主客户账户在一个适用期间内接受的商务和服务条件」——合同的相对方
  正是账户，同页两签是「合同 ↔ 它的相对方」的自然邻接。

  **三、因此不必新开页**，`page-registry.tsx` / `navigation.ts` / `liveIds` 三个共享接线文件一个
  不动。顺带记一处：`party-contracts` 的 `source` 仍点名「供应商商业协议」，而那册已另有
  `supplier-agreements` 页——那半句过期，属 navigation.ts（第三类文件、非本票必需），本票不改，
  记在这里供后续清理。

  **行形状**：`customer_party_id` 是对参与方册的引用，名称不在本册行上——所以本册**有**
  `partyNameKnown` 那一格（同法人册，左连接参与方册），不同于身份本体册。状态导出用同一个
  `domain.IdentityLifecycle`，`CASE` 与另两册逐字同。

  **登记签一并摆**：票 02 已为 `customer-account` 备好端点、标题与快照提示，只是「无读面故不摆」；
  读签落本页，按「写签跟着读签走」登记签也摆本页。`presentation.ts` 里两句「本册今天没有读面」
  「今天还没有的客户账户页」随之改成实际去处。

  **做法与顺序**：PC 侧三层（`ports` / `adapters/postgres` / `adapters/http`）所在目录此刻是 MCP-2
  的活现场（PCG-02/03 在途，`ports.go`、`operations_catalogue.go`、`adapters/http/*.go` 均未提交），
  MCP-5 裁定等它释号。本票代码因此在隔离 worktree 分支上做：只加新文件、不改 MCP-2 正在写的
  文件；`cmd/parcel-api` 那几行（端点表、探针、放行表、unwired 占位）待释号后占号再加。以已验
  SHA 交集成。
