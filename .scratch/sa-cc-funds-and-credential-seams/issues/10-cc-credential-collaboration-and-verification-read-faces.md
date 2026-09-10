# 凭证 / 税费付款协作 / 税费付款核对三册没有读面：`/customs-*` 四个查阅口都不覆盖它们，写签没有读签可跟

Category: enhancement
Status: ready-for-agent——2026-09-10 17:5x 通道 5 按通道 1 派单 task-a93cb825 写入裁决：三个读签各挂同族登记册读面已在的页——凭证进 `customs-cases`、协作与核对进 `customs-restrictions`，不新开页（见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 17:3x 通道 5 立票（task-9a2ff746；票 [07](07-cc-credential-and-duty-reconciliation-registration-faces.md)「要裁的」3 裁「读面另立」时点名），只写票面未动代码；取证锚 `66cad4c4`
Blocked by: 无（[07](07-cc-credential-and-duty-reconciliation-registration-faces.md) 步二 Blocked by 本票；本票不等 07）

## 缺口（取证于 `66cad4c4`）

- `internal/customscompliance/adapters/http` 的查阅口只有 `query_case_registers.go` / `query_compliance_rules.go` / `query_gate_conditions.go` / `query_ports_paths.go` 四个；`cmd/parcel-api/endpoints.go` 关务查阅口对应四个（票 07 缺口节）。
- 三册的写侧登记面已在 `ports`：`CredentialRegistry` / `CredentialView`（凭证）、`DutyCollaborationStore`（协作事项）、`DutyVerificationStore` + `DutyVerificationKey` / `DutyVerificationRecord`（付款核对）——都是按键写读，没有伴生的租户上列读口（形照 `GateConditionCatalogueRead` 那种「伴生列表读口」，ADR-0077 决定一 / 五）。
- 管理台 `apps/admin-web/src/pages/customs/` 今天两页（`customs-cases` / `customs-restrictions`）+ 口岸路径页，没有凭证、协作、核对任何一册的列。
- 伞票 awf/07「写签跟着读签走」：票 07 步二的三个登记签要挂在读签旁，没有读面就没处挂。

## 语言从哪里来

- CC `CONTEXT.md`：本上下文拥有「监管凭证及其适用性和使用关系……监管核定税费、税费付款协作事项、税费付款核对、放行门禁核对」；「监管凭证」词条（身份、版本、签发机构、持有人、辖区、商品、程序、线路、有效期、次数或额度）；「税费付款协作事项」「税费付款核对」词条。
- ADR-0077 决定一 / 五：登记册的伴生列表读口按租户上列、不拓宽点读口；两个调用面各答各的问题。
- ADR-0137 决定三：付款核对三态「分别表达」——读面上列三态原值，不折总状态。

## 做法

1. `ports` 三个伴生列表读口（形照 `GateConditionCatalogueRead`）：`CredentialCatalogueRead.ListCredentials`、`DutyCollaborationCatalogueRead.ListDutyCollaborations`、`DutyVerificationCatalogueRead.ListDutyVerifications`；租户在签名上、`limit` 非正拒、空册答空列表；postgres 实现 + 真库各一例（有 / 无 / 跨租户）。
2. `adapters/http` 三个 `query_*.go`（或并进一个 `query_duty_registers.go` 两册 + `query_credentials.go` 一册——见「要裁的」）；响应形封闭（ADR-0022），三态逐键原值；契约测试各一例。
3. `cmd/parcel-api/endpoints.go` 加查阅口（共享接线文件，动前占号）；`cmd/parcel-api/unwired_orchestration.go` 若有读面桩集合随之加法。
4. 管理台 `pages/customs/` 加读签（落点按「裁决」）：**凭证签进 `CustomsCasesPage.tsx`**（就绪与授权签旁——凭证是就绪门禁第 4 道的依据，awpwf/05 让该页接了就绪 / 授权 / 关闭）：身份 / 版本 / 签发方 / 持有人 / 有效期 / 额度依据；**协作事项签与付款核对签进 `CustomsRestrictionsPage.tsx`**（放行门禁目录与认定两表旁——该页 `moduleInfoById` 主责句本就是「监管核定税费与放行门禁核对」，awpwf/06 让它接了门禁两表）：协作事项列（范围 / 税费义务依据 / 付款要求来源 / 责任交接目标 / 核对入口）、付款核对列（三态原值 + 范围 + 核对版本 + 资金事实引用）；「未登记」如实显空态，不显默认。端点各立入口不并进 `/customs-case-registers` 的 `registry` 分派（照 awpwf/06 的判据：分派对应一页里的页签，各立入口对应各自独立的页——三册分落两页，各立三个入口）。
5. `internal/architecture` 管理台路径门禁绿；机制清点 tip 重生成（接入面查阅口 +3）。

## 红线

- 只读、只列：不在读面里折三态成总状态（CC CONTEXT）、不推「待关联」以外的派生（mech/07 CC-c「待关联派生——没有核对引用它的事实就是待关联」照原口径）。
- 不写任何真实凭证类型 / 程序 / 付款条件取值；夹具全是合成串。
- 不动三册写侧登记面的形；不动 `internal/customscompliance/application/**`。
- 跨客户隔离照 UC-CC-003「任一客户查询不得获知其他客户的存在……」——按租户上列，不按客户过滤给客户看。

## 完成判据

1. 三个伴生读口真库各一例（有 / 无 / 跨租户）；http 契约各一例「空册空列表、有行逐键」。
2. `cmd/parcel-api/endpoints.go` 三个查阅口在册；`internal/architecture` 绿。
3. 管理台三个读签可见，未登记显如实空态；`tsc --noEmit` 0 / `run-tests` 全 pass。
4. 清点 tip 重生成。

## 地盘

`internal/customscompliance/ports/ports.go`（三个读口接口 + 行类型，只加不改）、`internal/customscompliance/adapters/postgres/`（新文件）、`internal/customscompliance/adapters/http/`（新 `query_*.go`）、`cmd/parcel-api/endpoints.go` 一段（占号）、`cmd/parcel-api/unwired_orchestration.go` 若需、`apps/admin-web/src/pages/customs/`。

## 要裁的

1. **三个读签挂哪一页**：(甲) `customs-cases` 页加三签（案件配置四册已在那页，凭证 / 协作 / 核对与案件同族）；(乙) 新开 `customs-duties` 页放协作 + 核对两签、凭证进 `customs-cases`；(丙) 三签全新开一页。呈现落点，A 类，推送方可裁；先例：awf/06 / 21 政策页加一册、admin-web-page-wiring-frontier/04 的归属裁决。

### 裁决

（通道 1 推送方代裁 2026-09-10 17:5x，口径「各自挂在它同族登记册读面已在的页，没有同族页才新立；不为一册新开一页」；通道 5 核三册同族页并写入；task-a93cb825。）

- **凭证册 → `customs-cases`（导航「关务案件与申报」）。** 同族页是就绪依据所在页：[awpwf/05](../../admin-web-page-wiring-frontier/issues/05-customs-cases-takes-readiness-authority-and-closure.md) 让该页接了就绪判断、提交授权、关闭核对三类；凭证是 UC-CC-003 就绪门禁第 4 道的依据，ADR-0137 决定一的凭证门禁判断册日后也挂这里。
- **付款核对册 → `customs-restrictions`（导航「合规限制与监管税费」）。** 同族页是税费 / 放行门禁条件所在页：该页 `moduleInfoById` 主责句写「关务限制及解除、监管核定税费与放行门禁核对」，[awpwf/06](../../admin-web-page-wiring-frontier/issues/06-customs-restrictions-takes-release-gate-conditions.md) 让它接了门禁目录与认定两表；核对是 ADR-0137 决定三里「税费付款」那道门禁读的东西，与门禁两表同页一眼对得上。
- **协作事项册 → `customs-restrictions`，随它的消费方。** 协作事项的消费方是税费付款核对（`VerifyPayment` 在协作事项未形成时答未决，票 05 做法 2），核对在哪它在哪；`moduleInfoById` 那句的「监管核定税费」本就涵盖协作事项（CC CONTEXT「税费付款协作事项」词条：依据已接受监管核定税费形成）。
- 三册都有同族页，**不取乙（新 `customs-duties` 页）、不取丙**。口径：伞票 awf/07「写签跟着读签走」+ 不为一册新开一页（awf/06 / 21 都是往既有页加册）。票 07 步二的三个写签由此也分落两页。

## 参照

票 [07](07-cc-credential-and-duty-reconciliation-registration-faces.md)「要裁的」3 与「裁决」；ADR-0077 决定一 / 五、ADR-0022、ADR-0137 决定三；`internal/customscompliance/ports/ports.go` 的 `GateConditionCatalogueRead` 头注（伴生列表读口先例）；[awf/06](../../admin-write-faces/issues/06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) / [awf/21](../../admin-write-faces/issues/21-customer-service-rule-register-read-face.md)（读面另立先例）；[admin-web-page-wiring-frontier/04](../../admin-web-page-wiring-frontier/issues/04-registered-but-unreadable-rows-need-a-nav-ruling.md)（页面归属裁决先例）。

## Comments

- 2026-09-10 · 通道 5（task-9a2ff746）：立票，未动代码。能力边界：读过票 07 全文、`ports.go` 三册写侧接口名与 `GateConditionCatalogueRead` 头注、`adapters/http` 四个 `query_*.go` 文件名；没读三册的行字段与管理台 `pages/customs/` 的页面文件——列什么字段开工时以 `ports` 行类型为准。
