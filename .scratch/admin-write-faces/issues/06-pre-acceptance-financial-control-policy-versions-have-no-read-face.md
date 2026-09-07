# 06 接受前财务控制策略版本发布得出来、管理台看不见

Category: enhancement
Status: resolved——读面落地：`?kind=PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 第九册（分支 `mcp6-awf06`，基线 `95182b9d`：`3106cc3c` 后端、`a560d6d8` 管理台、`d6d35bee` 清点；2026-09-07，MCP-6 接 MCP-3 crash 后续做；完成记录在文末，含出处与验证强度；main 上的 SHA 待 MCP-1 重放后对照）。此前 blocked：取证已由 report.md A 组代做（只有壳）；MCP-3 2026-09-04 裁**②**：不按「只有壳」收口，正文表是缺口本体，另立 [party-commercial-context-gaps/07](../../party-commercial-context-gaps/issues/07-pre-acceptance-financial-control-policy-has-no-content-table.md)（已 resolved，ADR-0115）
Blocked by: 无（party-commercial-context-gaps/07 已 resolved）

## 裁决（MCP-3，2026-09-04，owner 授权自决）

**取②，不取①。** 票面完成判据第二条允许写「这一类版本只有壳、列壳没有信息量」并把它记成**长期事实**——那句话与
PC CONTEXT 正面冲突：词条明写策略「定义适用范围、共同通过条件和失败处置」，Rules 明写「合同要求组合控制时，策略必须
明确每项控制的适用范围、判断顺序、共同通过条件和失败或补偿责任」。壳不是这一类版本的形态，是**机制半边还没做**（与
`party-commercial-context-gaps/03` 补信用政策/供应商协议正文、ADR-0104 补客户服务规则正文同一形）。把机制缺口写成长期事实，
下一个读票 03 提示句的人会以为它不必再建。

取证（report.md A 组，锚 `08e62ec`）本票认可不重做：`migrations/party_commercial/` 无策略正文表；`0007` 是合同级声明
（答「要不要」，头注自己写「策略版本回答『控制怎么做』」）；SA `LoadControlPolicy` 读闭合 + 声明，不读策略正文。

**本票因此转 blocked，不收口也不做**：读面列的是正文，正文不在就没有可列的列面；等 pc-gaps/07 落表后，本票按 ADR-0077
通例在 `?kind=` 分派上加一格、与既有七册同形，那一步才是本票自己的活。**票 03 那句「今天没有册可看」的提示句不改**——它今天
仍是真话，改的时机是读面落地那一刻，与本票同笔。

**能力边界**：读过本票、PC CONTEXT「接受前财务控制策略」词条与 Rules 三句、`0007` 迁移全文、report.md A 组该条；没读
`pre_acceptance_control_policy.go` 全文与 ADR-0044/0054/0079 正文——裁的是「壳是不是长期事实」这一问，不裁正文形状（归 pc-gaps/07）。

## 缺口

发布口对象类别 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`（`CommercialObjectKind` 第 5 类）的
版本可以发布成功，但管理台没有任何一本册列它：「商业规则与策略」页的接受前财务控制册列的是
**挂在客户合同版本下的声明**（`0007`，`object_kind=2`，答「这份合同要不要控制」），不是策略
版本本身（答「控制怎么做」）。发布之后操作者在管理台找不到自己刚发的那一版，只能靠它被
解析选中时间接看见。

与票 [04](./04-customer-account-register-has-no-read-face.md) 当初记的 `customer-account`
「有写面无读面」同族。发现于票 [03](./03-publication-write-face-blocked-by-two-misaligned-closed-sets.md)
取证（2026-09-03，锚 `c93abba`）。

## 先取证

- 这一类版本今天在库里有没有正文表（`migrations/party_commercial/` 里哪一份），还是只有
  `commercial_version` 上的版本壳？没有正文表的话读面能列的只有壳。
- 结算政策解析（ADR-0044）实际采用的控制策略版本从哪张表读——那条读路径就是读面该转写的列面。

## 完成判据

要么读面上多一本册（沿 `?kind=` 分派加一格，与既有七册同形、ADR-0077 通例），要么票面写明
「这一类版本只有壳、列壳没有信息量」并把它记进票 03 那句「今天没有册可看」的提示里作为长期
事实。两条都算完成；不算完成的是继续让它发布得出来而看不见。

## 边界

不动发布口、不动领域；`cmd/parcel-api/endpoints.go` 若要加行按共享接线文件纪律占号。

## 完成记录（2026-09-07，MCP-6；分支 `mcp6-awf06`，基线 `95182b9d`，不推——MCP-1 重放进 main）

**取的是完成判据第一条**：读面上多一本册，沿 `?kind=` 分派加一格，与既有各册同形（ADR-0077 通例）。第二条（写成长期事实）按 MCP-3 2026-09-04 的裁决不取，正文表已随 pc-gaps/07 落地。

| 笔 | SHA | 内容 |
|---|---|---|
| A | `3106cc3c` | 后端第九册：`ports.CommercialPolicyCatalogueRead.ListPreAcceptanceFinancialControlPolicies` + 行体 `PreAcceptanceFinancialControlPolicyRow` / `PreAcceptanceControlItemRow`；postgres `OperationsCatalogue` 按第 5 类版本壳驱动、0024 父表左连接、子表按判断顺序聚成 json 同一条语句取回；HTTP `kindPreAcceptanceFinancialControlPolicy` 进两处 switch，行体 `contentRegistered` + `content{jointPassCondition, registeredAt, controls[{control, chargeScope, order, onFailure, responsibility}]}`（控制项键与受控 CLI 批文同词）；`unwiredCommercialCatalogue` 跟随一个方法。测试：HTTP 两例（第九册行体三态与分派只走策略册；`PRE_ACCEPTANCE_CONTROL` 仍只列合同声明）+ 分派表加一行；PG 一例（壳有/无正文、别类壳与他租户不进本册、limit 非正拒、空册无 error） |
| B | `a560d6d8` | 管理台：`CommercialPolicyKind` 加格、两个 Record、本册列向（控制项合成一栏带顺序）、三个封闭集标签、册名「接受前财务控制策略」、来源提示句；票 03 那句「那一类版本今天没有册可看」按本票边界同笔改成指向本册；`policy-rows.test.ts` 三态与合栏转写 |
| C | `d6d35bee` | 机制清点在 `a560d6d8` 干净检出重生成：partycommercial 测试文件 86→88，合计 761→763；生产文件面、端口声明数、端点数不变 |

**出处**：A、B 的实现是 MCP-3 会话的在途产出（分支 `mcp3-pcgaps07` 工作副本，11 件，mtime 11:37–11:45，会话 12:4x crash），MCP-1 封存为 `chore(salvage)` `8dc99b9f`（不进 main）。本会话按 parallel-sessions「接手别人在途产出先写自己第一片 red」：先写 HTTP 与 PG 两片 red（`go vet` 报 undefined 确认红），再读 `8dc99b9f` 对判据——版本壳驱动 / 正文左连接 / `HasContent` 不拿 `Controls` 兼作 / 控制项按判断顺序 / kind 名与分派 / JSON 键——全部对上，沿用其实现；只改两处：端口行字段名照领域访问器（`Kind` / `FailureDisposition`，对方是 `Control` / `OnFailure`），对方两份同判据用例并入我的两片不留两份。B 一字未改。

**验收对照**：读面上多一本册 ✓（A + B）；与既有各册同形 ✓（版本壳 + `contentRegistered` + `content` 节，照 `CUSTOMER_SERVICE_RULE` 那册）；不动发布口、不动领域 ✓；`cmd/parcel-api/endpoints.go` 未加行（`?kind=` 复用既有端点）✓；`unwired_orchestration.go` 只动 `unwiredCommercialCatalogue` 那一个 hunk ✓。

**验证强度**（detached 干净检出 `d6d35bee`，含 DSN，门禁容器 `127.0.0.1:55432`）：`gofmt -l .` 零输出；`go build ./...`、`go vet ./...` 退 0；`go test -p 1 -count=1 ./...` 99 包 ok / `FAIL` 裸子串零命中 / 退 0。探针一正一反：`internal/partycommercial/adapters/postgres` DSN 已设 `--- PASS` 221 / `--- SKIP` 0（53s），未设 `--- PASS` 2 / `--- SKIP` 170（0.025s）；单跑本票那例 `PreAcceptanceFinancialControlPolicyCatalogue` 有 DSN PASS（0.32s）、无 DSN SKIP。`internal/partycommercial/adapters/http` 90 PASS、`internal/architecture` 157 PASS 两种设置同数。`apps/admin-web`：`node node_modules/typescript/bin/tsc --noEmit` 退 0、`node scripts/run-tests.mjs` 74 pass / 0 fail（node_modules 是指向主树的目录联接，未跑 install）。清点：`d6d35bee` 干净检出重跑生成器零差。未跑 `-race`（本机走不了，见 workflow.md 本机环境）。

**要 MCP-1 落的装配行**：无（复用 `/commercial-policies` 端点与既有商业目录适配器）。

**顺带量到、不在本票**：管理台 `CommercialPolicyKind` 没有 `CUSTOMER_SERVICE_RULE` 那格——后端第八册（pc-gaps/05，0023）落了，`apps/admin-web/src` 里 `CUSTOMER_SERVICE_RULE` 零命中，`policyKindLabels` 也没有它；pc-gaps/05 票面只写了「目录读面一格」（后端）。是与本票同形的「发布得出来、管理台看不见」，归 admin-web PC 页地盘，要不要立票交 MCP-1。

## Comments

- 2026-09-07 · MCP-6：收口。三笔为 A 代码 / B 管理台 / C 清点；封存笔 `8dc99b9f` 的证据句已移进 A 的提交信，重放时跳过它。
