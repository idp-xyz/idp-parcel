# 合规限制页收放行门禁：目录与认定两表

Category: enhancement
Status: resolved
Owner: MCP-1（2026-08-27 接续断线前的在途件收口；票 05 已先行收口，两票串行不并行的约束照守）

自[票 04 的导航裁决](./04-registered-but-unreadable-rows-need-a-nav-ruling.md)。这一格连裁都不算裁——`customs-restrictions` 在 `moduleInfoById` 里的主责句结尾原文就是「放行门禁核对」，与 CONTEXT 116 的术语条同词。页面早认领过，只是没实现读面。

## 事实（实读代码，锚 `dc611a3`）

1. `gate_condition_catalog` / `gate_condition_finding`（0008）都带 `tenant_id`；主键含 `(scope_ref, action, boundary_ref)`，`action` 是封闭四值（`OUTBOUND_RELEASE`、`LOADING_DEPARTURE`、`CROSS_CUSTOMS_MOVEMENT`、`FINAL_DELIVERY`）——0008 自注：动作在主键里不是附属列，因为门禁判断绑定动作与边界、不能复用于其他动作（CONTEXT 硬句 216）。
2. 点读适配器 `GateConditionView.LoadPreconditionFindings` 已在，按 `(tenant, scope, action, boundary)` 取一份认定集，交回领域对象——为编排而设，不是目录上列。
3. 种子灌过一份门禁目录与一条已满足的门禁认定（`08-gate-catalog.json` / `09-gate-finding-met.json`）。

## 要做什么

与票 05 同形（同一个上下文、同一批共享文件），因此**两票串行、不要并行派**：

- `ports` 加伴生列表读端口，不拓宽 `LoadPreconditionFindings`。
- `adapters/postgres` 列表适配器，父子一条语句取回。
- `adapters/http` 查询处理器，复用既有两种 Intake。
- `cmd/parcel-api` 装配 + 放行面枚举加行。
- `CustomsRestrictionsPage` 接真 + `liveIds` 加一行。

## 两条形状约束

1. **目录未登记与登记了空清单必须分得开**，且两者含义与关闭义务那一格**相反**——0008 自注原话：门禁目录未登记 → 未决，没有清单的门禁判断无从复核；而登记了却空清单是「此动作在此边界本就不受门禁」的如实答案（领域折叠为「不适用」）。页面不能把这两态说成同一句话，也不能照抄票 05 的说法。
2. **认定的封闭三值刻意没有「未知」格**（0008 自注：判断不出来的前置条件根本不该进折叠）。读回集外取值即坏数据，上抛，不折成第四格。

页面上门禁按 `(scope, action, boundary)` 三元组成组显示；动作用 CONTEXT 原词的中文化说法，封闭四值进词表（照 `pages/party/presentation.ts` 的形状）。

## 完成标准

同票 05：`200` + 非空册、未启用准入答 `403`、真库测试钉住租户隔离与两态可分辨、全仓测试绿（含真库）、页面层 DOM 取证。

## 收口（2026-08-27 · MCP-1）

四笔提交：`cd23768` 读面（端口 `GateConditionCatalogueRead`、真库适配器、`/customs-gate-conditions` 处理器），`56af09a` `cmd/parcel-api` 装配与隔离读放行面两份枚举各加一行，`be26d97` 种子第二份门禁，`fb1fb01` 合规限制页门禁签接真与 `liveIds`。

**票面两条形状约束的落法**。①两态分得开：空 `findings` 是「此动作在此边界本就不受门禁」在场的一格，整份目录缺席才是「未登记 → 未决」；页面行内写「本就不受门禁」、空册文案写「未登记 ≠ 不受门禁」，与 `ClosureObligationsTable` 那对逐句不同，没有照抄票 05。②认定封闭三值不折第四格：集外取值在真库读口 `preconditionStateOf` 上抛，传输层与页面都不设「未知」，词表也不补——`labelOf` 对集外取值原样回显。

**另两处本票内的决定**。端点各立入口而不并进 `/customs-case-registers` 的 `registry` 分派：那三册归 customs-cases 一张页面，门禁两表归 customs-restrictions，分派对应「一页里的页签」、各立入口对应「各自独立的页」（判据同票 05 引的 `/commercial-customer-contracts` 先例）。页面刻意不设「门禁判断」列：五值结论（`FoldGateConclusion`）是门禁编排的判断语义，查阅面转述登记册本身，在页面折一次就是第二处判断权威。三元键（范围·动作·边界）三列并列而不折成一个「核对标识」，是 CONTEXT 硬句 216 的表形。原 `ReleaseGateRow` 骨架同笔删除——它有 `verdict` 与 `releaseResultRef` 两列，而登记册按裁决就不交这两样。

**取证**

1. 端点（隔离读实例 `:19081`，种子租户）：`200` + 两份门禁——`CROSS_CUSTOMS_MOVEMENT` 带一条 `MET` 认定，`FINAL_DELIVERY` 带空 `findings`。同实例 `POST` 答 `405`；另起未设 `IDP_PARCEL_ISOLATED_READ_TENANT` 的实例（`:19082`），本端点与 `/customs-case-registers` 同答 `403`（共用同一 `CatalogueQueryIntake` 变量，两行一起换值）。未配置格另由 `endpoints_test.go` 的 unwired 探针钉 `403` + `ACCESS_CHANNEL_NOT_CONFIGURED`。
2. 真库测试三条全 `PASS` 非 `SKIP`（反向探针：设 DSN 3 `PASS` 0 `SKIP`，清空 DSN 转 0 `PASS` 3 `SKIP`）。钉住空册答空列表、两态可分辨（零认定目录在场 vs 未登记动作缺席）、跨租户不可见、认定三值逐格如实、父子同快照一语句取回、三维键序稳定、`limit` 非正即拒且截断不静默全量。
3. `go test -count=1 ./...` 含真库：退 `0`、80 包 `ok`、零 `FAIL`；`go vet` 净，`gofmt -l internal/ cmd/ migrations/` 空。
4. `seed.sh --reset` 复灌零报错，两笔 `gate-catalog: REGISTERED` 各自幂等，末行「五上下文全部落库」。
5. 页面层：tmux 托管重起 vite 后 Edge 无头取 DOM。门禁签（现为默认签）见两份门禁：动作中文「跨关务区域移动」「交付」、边界 `SYN-PROC-CN-EXPORT`、前置条件 `SYN-PRE-EXPORT-DECL-RELEASED` 认定「满足」，以及空清单那份的「本就不受门禁 / 未登记任何前置条件」。汇总行「门禁 2 份 · 前置条件认定 1 项 · 其中不受门禁 1 份」。反例三条均 0 命中：无未配置码、无五值结论词「部分满足」、无旧骨架列「核对标识」。

**取证环境踩到一处，已另笔修**（`253f2ff`）：`.gitattributes` 给 .go/.md/.sql 等各钉了 `eol=lf` 却漏掉 `.sh`，而本机 `core.autocrlf=true`，于是本目录下的取证脚本检出即 CRLF，`bash` 在 `set -uo pipefail\r` 那行就死。**这个坏法在取证环节特别难认**：vite 起不来时 Edge 取回 Chromium 自己的错误页，`--dump-dom` 照样落盘、字节数照样有、退出码照样是 `0`，只有 needle 全 miss——与「页面没接上」逐条同形，本轮先误判了一次。另一半原因是 vite 用 `nohup` 起在 `wsl -- bash` 会话里，`wsl.exe` 一退它就死；改由 tmux 托管。
