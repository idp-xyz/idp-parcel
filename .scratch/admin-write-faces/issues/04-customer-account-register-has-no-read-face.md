# 04 货主客户账户册有写口无读面——身份三级里唯一看不见的一级

Category: enhancement
Status: ready-for-agent（落点要先裁一句，见「要裁什么」）
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
