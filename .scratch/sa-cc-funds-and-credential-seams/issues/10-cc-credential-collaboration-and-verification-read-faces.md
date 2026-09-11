# 凭证 / 税费付款协作 / 税费付款核对三册没有读面：`/customs-*` 四个查阅口都不覆盖它们，写签没有读签可跟

Category: enhancement
Status: resolved——2026-09-11 10:5x 通道 1 推送方重放进 main（非作者评审 ← 通道 2 两轴 0 阻断；main 上 SHA 与分支 SHA 对照见 Comments「进 main 记录」；与后继四票同批，lc/30 因评审阻断另批）；此前 resolved——2026-09-10 22:4x 通道 5（task-7e1d15e1）落分支 `mcp5-sacc10`（代码 tip `0017f49b`，清点 `6b731816`），等非作者评审与进 main（进 main 记录由推送方补）；此前 in-progress——2026-09-10 21:5x 通道 5 按通道 1 派单 task-7e1d15e1 认领，分支 `mcp5-sacc10` 基远端 main `9ddbafcf`；此前 ready-for-agent——2026-09-10 17:5x 通道 5 按通道 1 派单 task-a93cb825 写入裁决：三个读签各挂同族登记册读面已在的页——凭证进 `customs-cases`、协作与核对进 `customs-restrictions`，不新开页（见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 17:3x 通道 5 立票（task-9a2ff746；票 [07](07-cc-credential-and-duty-reconciliation-registration-faces.md)「要裁的」3 裁「读面另立」时点名），只写票面未动代码；取证锚 `66cad4c4`
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

## 完成记录

（通道 5，task-7e1d15e1，2026-09-10 21:5x–22:4x；分支 `mcp5-sacc10` 基远端 main `9ddbafcf`，六笔全部已推 origin。）

| 笔 | SHA | 内容 |
|---|---|---|
| 1 | `444d10fe` | 票面 Status → in-progress |
| 2 | `3dba4fd9` | 做法 1：`ports` 三个伴生列表读口 `CredentialCatalogueRead` / `DutyCollaborationCatalogueRead` / `DutyVerificationCatalogueRead`（行：`CredentialCatalogueEntry` = 领域凭证 + 登记时间；协作事项直接交回 `DutyPaymentCollaboration`；核对复用 `DutyVerificationRecord`）；`adapters/postgres` 新增 `CredentialCatalogue`、`DutyReconciliationCatalogue`（一型两口）；真库用例 14 例 |
| 3 | `3654d711` | 做法 2：`adapters/http` 新增 `query_credentials.go` / `query_duty_collaborations.go` / `query_duty_verifications.go`，各立入口 GET `/customs-credentials` / `/customs-duty-collaborations` / `/customs-duty-verifications`，响应形封闭；契约测试 15 例 |
| 4 | `0d8cd298` | 做法 3：`cmd/parcel-api` 五文件接线（纯加 67 行、不动邻行；占号→推→释号） |
| 5 | `0017f49b` | 做法 4：管理台凭证签进 `CustomsCasesPage`（紧挨就绪与授权）、协作与核对两签进 `CustomsRestrictionsPage`（挨着放行门禁核对）；`register-rows.ts` 纯函数 + 6 例测试 |
| 6 | `6b731816` | 做法 5：机制清点重生成（干净 detached 检出；CC 接入面端点 10 → 13，端口 384 → 387） |

**验证**（均在隔离树 `mcp5-sacc10` 上）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 全仓 0；`go test -p 1 -count=1`（带 DSN）CC 包组 + 反向依赖（`cmd/parcel-api`、`cmd/parcel-customs-register`、`cmd/parcel-dispatch`、PS / VE 的 `adapters/customscompliance`）+ `internal/architecture` 全 ok / 0 FAIL；CC `adapters/postgres` 整包 `-v` 0 SKIP、新增 14 例全 PASS；`cmd/parcel-api` `-v` 72 PASS / 0 SKIP；admin-web `tsc --noEmit` 0、`run-tests` 220 pass / 0 fail（含新 6 例）。未跑全量（推送方那一跑是唯一真值）。

**完成判据对照**：1 三口真库各三例（空 / 有行逐格 + 跨租户 / limit）+ 构造期拒 nil；http 各五例（405 / 未配置 403 / 空册空数组且不采信自报参数 / 有行逐键 / 读不回 5xx），核对那例断言键集恰好九键 ✓。2 三个查阅口在 `endpoints.go` 在册；`internal/architecture` 绿（管理台路径门禁含三条新路径）✓。3 三读签可见；空册走 `catalogueViewState` 空态，文案各说各话（凭证未登记 → 门禁停「凭证未登记」不读作不适用；无协作事项 → 核对未决不读作无需付款；无核对版本 → 门禁那一道未决不读作已付）✓。4 清点重生成 ✓。

**两处照领域形状而非票面字面的列**（不算裁决，记下免得评审再找）：凭证签没有单独「版本」列——`RegulatoryCredential` 一身份一版、无版本维（0014 自注），换期限或额度是另一张凭证；「额度依据」落成「次数额度」一列，缺席显「来源未提供」。协作事项签没有单独「核对入口」列——核对册按（范围，税费引用）回指本册，两册对读即得，本册对象上没有那个字段，不虚构。

**红线核**：三轴三列无合成列、核对签摘要只报版本数（不数「几版悬着」）；「待关联」不在两册读口里代算；`internal/customscompliance/application/**` 零改动、三册写侧形零改动；夹具全 `SYN-` 合成串。

## Comments

- 2026-09-10 · 通道 5（task-9a2ff746）：立票，未动代码。能力边界：读过票 07 全文、`ports.go` 三册写侧接口名与 `GateConditionCatalogueRead` 头注、`adapters/http` 四个 `query_*.go` 文件名；没读三册的行字段与管理台 `pages/customs/` 的页面文件——列什么字段开工时以 `ports` 行类型为准。
- 2026-09-10 · 通道 5（task-7e1d15e1）：实施完工，见「完成记录」。非作者评审与进 main 记录由推送方 / 评审通道补。
- **评审 ← 通道 2 · 钉 `49ef9d5f`（代码 tip `0017f49b`，基线 `9ddbafcf`）· 2026-09-11 10:5x**（task-09f4b036，非作者，隔离树 `%TEMP%\idp-review-sacc10`，不带 DSN；由推送方代落）。评审侧实测：`gofmt -l` 空；`go vet ./internal/customscompliance/... ./cmd/parcel-api/...` 0；`go test -count=1` http / ports / `cmd/parcel-api` / `internal/architecture` 全 ok、0 FAIL；CC `adapters/postgres` 九例无 DSN skip → 未验；admin-web `tsc` / `run-tests` 未跑 → 未验（按派单只读 diff）。评审者起的两个 `/code-review` 隔离 pass 截止前未回报，本报为评审通道自做两轴。
  - **Standards**：**阻断 无**。**非阻断 3**：① 跨文件引用带行号——`adapters/http/query_duty_collaborations.go` `dutyCollaborationBody` 注释「CONTEXT 硬句 212」、`query_duty_verifications_test.go` Covers 注「CONTEXT 硬句 214」、`CustomsRestrictionsPage.tsx` `collaborationColumns` 注释「硬句 212」；核过 CONTEXT.md 该两行确为被引句，即行号；按 AGENTS.md「改文档」「Go 注释里的跨文件引用同受此约束……不用行号」。包内 `ports.go` / `domain/*` 旧例同写法，属沿旧例非新造；改法换引文即可。② 判断项（Duplicated Code）：`CredentialsTable` / `DutyCollaborationsTable` / `DutyVerificationsTable` 与既有 `ReleaseGatesTable` 取数 / 过滤 / retry 同形，页内四份；仓内先例如此，不阻断。③ 判断项：`register-rows.ts` `collaborationBasis` 末分支对集外 kind 回落 `duty ?? noPayBasis`，与同函数注释「不借字段」及 `labelOf`「坏数据露出来」口径不一；影响仅坏数据行。**无发现（核过）**：夹具全 `SYN-*` / `TENANT-1` / `tenant-a|b` / `digest-*`，币种仅 ISO 测试码 `XTS`，无真实凭证类型 / 程序 / 付款条件；注释全中文；三读口形照 `GateConditionCatalogueRead`（租户在签名、`limit<1` 拒、空册 `make(…,0)` 答空、点读口未拓宽）；`NewCredentialCatalogue` / `NewDutyReconciliationCatalogue` 构造期拒 nil；`dutyVerificationBody` 九键三轴逐键、无 status 合成列，`presentation.ts` 三词表分立；`omitempty` 有 `query_case_registers.go` 同包先例；`outcome` 常量封闭（ADR-0022）。
  - **Spec**：**阻断 无**。**非阻断 3**：① 完成判据 1「真库各一例（有 / 无 / 跨租户）」与判据 3「`tsc --noEmit` 0 / `run-tests` 全 pass」评审侧未验（无 DSN / 未跑 admin-web）；用例本身在：`credential_catalogue_test.go` 4 例、`duty_reconciliation_catalogue_test.go` 5 例，含空 / 有行逐格 / 跨租户 / limit / nil，`register-rows.test.ts` 6 例。② 票面未要的呈现：`CredentialsTable` filterSummary「其中来源未提供次数额度 N 版」、`DutyCollaborationsTable`「其中明确无需付款 N 份」——按单字段计数，非三态折叠、非「待关联」，不违红线，但属票面「做法 4」列表之外的摘要（轻微）。③ 做法 4 协作事项「税费义务依据」落成 `kind` + `basis` 两列，另带 `obligor` / `formedAt`（对象自有字段）——多于票面字面，不违语义。**无发现（核过）**：作者自报三处站得住——`domain.RegulatoryCredential` 七字段无版本维（票面「版本」列无对象可映）；`domain.DutyPaymentCollaboration` 八字段无核对引用（「核对入口」列不虚构）；核对签 filterSummary 只报 `verifications.length`。裁决落点：凭证签 `CustomsCasesPage` `credentials` Tab 紧挨 `preconditions`；协作 / 核对 `CustomsRestrictionsPage` `duty-collaborations` / `duty-verifications` Tab 紧挨 `gates`；无新页、无新路由。端点 `/customs-credentials` / `/customs-duty-collaborations` / `/customs-duty-verifications` 各立入口，`endpoints.go` 在册、`isolated_read_test.go` 放行 +3、`endpoints_test.go` 探针 +3，不并进 `registry` 分派。`application/**`、`domain/**`、`migrations/**` 零改动；`ports.go` 纯加，写侧接口未动。空态 `catalogueViewState` 文案各说各话、不显默认。
  - **结论**：Standards 0 阻断 / 3 非阻断（最重：行号引用）；Spec 0 阻断 / 3 非阻断（最重：真库与 admin-web 评审侧未验——推送方全量已补验，见下条）。可合入；行号引用建议合入前一笔换引文或留作跟进。
- **进 main 记录（通道 1 推送方，2026-09-11 10:5x）**：通道 1 前会话 09-10 23:0x 在 `%TEMP%\idp-replay-batch` 把三链（sa-cc/10 六笔 → 后继四票 → lc/30 八笔）叠到 main `e8fe7bf9`（tip `affdedcd`），随后中断——未验、未派评审、未簿记。本会话核过内容（sa-cc/10 文件对分支 tip 只差清点；四票 .md 对 `mcp4-followups` 零差）后沿用那份重放；lc/30 评审出一条 Spec 阻断回作者修，故三链拆两批，**本批 = sa-cc/10 六笔 + 后继四票**，tip 取 `72a247ff` 另建 `%TEMP%\idp-replay-sacc10`。SHA 对照：`444d10fe→264eeeeb`、`3dba4fd9→630d427d`、`3654d711→df78a173`、`0d8cd298→32311c76`、`0017f49b→e6132fce`、`49ef9d5f→24d6a376`；作者清点笔 `6b731816` 不带，tip 重生成 **`99ceb975`**（CC 生产 78→83 / 测试 78→83、接入面端点 112→115、端口声明 384→387，与作者那份口径同）。后继四票 `585af1c5→9a77be16`、`17370740→bcb676d0`、`2240d826→6e31da11`、`3520dd1a→72a247ff`（纯 .md，推送方自审：四笔只动 `.scratch/**`，Status 与 Blocked by 如报，相对链接逐一在）。**验证钉 `99ceb975`**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./...` **107 ok / 0 FAIL / 15 无测试 / 0 cached**（10:51:10→10:53:15，125 s）；`-v` 探针 CC `adapters/postgres -run 'Credential|DutyReconciliation|DutyPayment'` PASS 10 / SKIP 0、`cmd/parcel-api` PASS 70 / SKIP 0（评审侧「未验」两处在此补齐）；admin-web `tsc --noEmit` 0、`run-tests` 220 / 220。簿记一笔在其上（本票 Status + 评审 + 本条、spec 10 行、票 07 Blocked by 句、tasks.md），纯 .md 自审；`ls-remote` 核 `e8fe7bf9` 未动 → ff → `push <sha>:main`。分支 `mcp5-sacc10@49ef9d5f` 作封存出处、改名 `merged/`、远端删。**推送方对评审的处置**：两轴 0 阻断，非阻断六条随票记、不挡合入。Standards ① 行号引用三处是 AGENTS.md 明令的写法，作者未改、推送方不代改作者代码——归 CC owner 一笔换引文（可并入 [14](14-adopt-digest-header-and-duty-reconciliation-handler-rejects-nil.md) 那类小改票，或 sa-cc 下一号）；Standards ③ `collaborationBasis` 末分支回落与 Spec ② / ③ 呈现多于票面，随票记，不立票。
