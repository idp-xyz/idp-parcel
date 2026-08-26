# VE 六类目录查阅面与导航条目——登记通道齐备、库里零行、无页可归

Category: enhancement
Status: resolved
Owner: MCP-1（2026-08-26 认领；ve-claims-read-seams/01、/02 已先行收口，资格与证据两缝接真不影响本票读面）

自[接线前沿盘点](../report.md)批 B 的干净那半。与票 01 无文件重叠，可完全并行；只有末段 `cmd/parcel-api` 装配要等票 01 释号（见 [spec](../spec.md) 地盘与占号）。

## 事实（实读代码与演示库取证，锚 `7ce41e4`）

1. `parcel-ve-register` 已能登记六类目录：里程碑映射、分诊规则、通知策略、索赔资格、索赔授权、披露策略。**六个子命令都在，不是计划**。
2. 演示库里这六类**全部零行**——`scripts/demo-seeds/seed.sh` 只调用计价、网络、关务、商业四个 CLI，没调用它。灌一次就有数据。
3. 六类目录表**都带租户维**（实测：`triage_rule_version` 与 `claim_authorization_catalogue` 都以 `tenant_id` 打头做主键）。因此 `ADR-0077` 「租户在读口方法签名上」与 `ADR-0078` 的隔离读准入形状**逐字成立**，不需要先裁键形——这正是本票与票 03 分开的原因。
4. VE 已有 `adapters/http` 包，含 `isolated_read_intake.go` 与 `unconfigured_intake.go`，追踪投影查阅端点是现成的同族样板。
5. `adapters/postgres` 里 `rule_catalog.go`、`claim_eligibility.go`、`notification_policy.go`、`disclosure_policy.go`、`catalog_registration.go` **都只有写口**，没有列读方法。
6. **这六类今天无页可归**：导航主数据区一个 VE 条目都没有，追踪异常区那三页装的是案件不是目录。

## 要做什么

- 种子：新建 `scripts/demo-seeds/seeds/visibility/`，六类各发一版合成 `SYN-` 目录；`seed.sh` 加调用（**占号文件**，见 spec）。
- `internal/visibilityexception/ports/` 加伴生列表读端口，租户在签名上，**不拓宽既有登记写口**（理由与来历见 `parcelpricing/ports/catalogue_read.go` 的文件注释）。
- `adapters/postgres/` 加读适配器；父子册（版本 + 条目）一条语句取回，不分两次——`ReadExecutor` 不保证同一快照，分两次会拼出从未同时存在的父子状态。
- `adapters/http/` 加查询处理器，准入复用 VE 现有的隔离读 Intake，不新立一路。
- `cmd/parcel-api` 装配（**等票 01 释号**）。
- `apps/admin-web`：导航主数据区加条目、`moduleInfoById` 补出处、新页面、`page-registry.tsx` 的 `pageById` 与 `liveIds` 各加行（**只改自己那行，不动邻行**）。

## 两处要在票面先答的

**一、六类拆几页。** 六类都是主数据，但性质不齐：里程碑映射与分诊规则是**判断规则**，通知策略与披露策略是**对外披露口径**，索赔资格与索赔授权是**索赔前置**。全塞一页会得到一个六页签的巨面，拆六页又会让主数据区一次多出六个条目。倾向按上面三组拆三页，但这是本票内的裁决，写进实现时的页面文件注释。

**二、导航分区。** 按导航文件自己的分区口径（主数据区集中「身份、目录与登记式对象」的查阅入口），这六类归主数据区。但追踪异常区那三页（异常分诊、异常案件、索赔与追偿）与它们同源不同性——目录是规则，那三页是案件。**不要把目录塞进那三页**：案件页今天是空的（被三堵墙拦），塞进去会让人以为墙降了。

## 完成标准

- 六类各答 `200` + 非空册；未启用隔离读准入时答 `403`，与既有查阅端点同签名。
- 真库测试钉住：租户隔离、空册如实答空不折成未配置、父子同快照、`limit` 非正即拒。
- 全仓 `go test -count=1 ./...` 绿（**含真库**——单跑一个真库用例看 `-v` 下是 `PASS` 不是 `SKIP`）。
- 页面层取证：WSL 起 vite + Edge 无头取 DOM，新页见 `SYN` 数据、无未配置码。
- `seed.sh --reset` 复灌零报错（六类 CLI 各自幂等）。

## 收口（2026-08-26 · MCP-1）

`8f1c8e7` 读面（端口 `ports/catalogue_read.go`、真库适配器、`/visibility-catalogues?kind=` 处理器、`cmd/parcel-api` 装配），`202e15b` 种子第六步（六类七笔，含一份显式空名单授权），`708b10f` 管理台三页与导航。

**票面两问的裁决**：①六类按性质拆三页——判断规则（里程碑映射+分诊规则）、对外披露口径（通知策略+披露策略）、索赔前置（索赔资格+索赔授权），各页两页签，理由写在各页文件头注释；②三条目全归主数据区（排序注释补「→ 追踪异常」），不塞追踪异常区那三张案件页。端点形状循 `/commercial-policies`：单端点 `?kind=` 封闭六格分派，准入复用 VE 既有 `OperationsTrackingIntake`，不新立一路。

**取证**

1. 端点：六 kind 各答 `200` + 非空 `catalogues`（隔离读实例 `:19081`，`CLAIM_AUTHORIZATION` 两行——一行两申请人、一行显式空名单）；未配置 Intake 时 403 由 `endpoints_test.go` 的 unwired 探针与处理器测试钉住。
2. 真库测试九条全 `PASS` 非 `SKIP`（单跑 `TestListMilestoneMappingsReturnsVersionsWithEntriesNewestFirst`、`TestCatalogueListsAreTenantScoped` 各 0.2s+ 实跑）；父子同快照靠 `json_agg` 相关子查询一语句取回（票 07 笛卡尔积教训，六册各聚各的）。
3. `go test -count=1 ./...`：94 包全绿。唯一红点是工作树里**未提交**的 `internal/settlementaccounting/adapters/partycommercial/`（票 sa-preacceptance-policy-view/01 的半成品，MCP-1 自己的，5 用例红），与本票三笔提交无关，接下来在那张票内收拾。
4. `seed.sh --reset` 复灌零报错，末行「五上下文全部落库」。
5. 页面层：重起 vite（inotify 注记照旧）后 Edge 无头取 DOM——三页首屏 + 三个第二页签（`dom-dump-tab.mjs`）。判断规则页 4 条映射 + 4 条分诊（四个结果词各在场）；披露页通知策略 1 条、披露策略两客户四维（`展示:SYN-VE-CONTENT-…`×3、`待确认`×3、`不披露`×2）；索赔页资格 1 份、授权 2 份（`显式空名单:不授权任何申请人代提` 在场）。全部产物零未配置码。

**页面层一处刻意分文件**：目录读走共享 `catalogue-api` 传输（与 party/customs 目录页同款五格判读），追踪投影查阅仍走 `visibility/api.ts` 自己的出口——两族读面语义不同（登记态目录 vs 派生态投影），分文件即分口径，`api.ts` 头注已改「唯一出口」措辞。
