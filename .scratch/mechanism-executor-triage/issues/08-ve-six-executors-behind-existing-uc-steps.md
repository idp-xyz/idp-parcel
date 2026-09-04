# VE 六条：步骤在 UC 里、执行器不在编排里——接线

Category: enhancement
Status: resolved——四组四笔 `10adcb3` / `3ca94fc` / `7058d92` / `a18cdf7` + VE-c 补刀 `4de7aba`（2026-09-04，MCP-6；清点重生成 `5894320` 与 `4de7aba` 同笔；见文末「完成记录」）
Blocked by: 无（组内 `PrepareDisclosure` 依赖同票的 `SubmitEvidence` 先接）

由[票 03](./03-fourteen-that-only-tests-ever-call.md) `## Answer` 立出，按 [spec「处置裁决」](../spec.md) 第 1 条。举证在票 03，此处只列改动对象与完成判据。

## 四组，每组一个编排文件，可各成一笔

**VE-a `UC-VE-006` 前半：形成披露决定** — `internal/visibilityexception/application/notify_customer.go` 的命令把 `domain.DisclosureDecision` 当**输入**，用例从「决定已经有了」开始，而没有任何生产代码形成它。新用例文件接 `DecideDisclosure`：输入是发作期、客户账户、披露策略（`ports.DisclosurePolicyView.AssessDisclosure`，策略登记已在，`PAR-VIS-09`），产出三态结论与内容引用；`notify_customer.go` 的输入改为引用这个产物而不是裸接一个决定。`UC-VE-001` 步 7（关务披露）同一函数、同一分层规则，另一个入口。

**VE-b `UC-VE-004` 建案支** — `raise_signal.go` 调 `OpenEpisode` + `ConcludeTriage`，分诊结论为「建案」时没有人建案。接 `EstablishCase`：固定根对象、初始影响范围、责任团队，主状态待响应（`AT-VE-062`）；资料不足/可能重复走人工分诊不建案（`AT-VE-063`），同一连续期重复命中不重复建案（`AT-VE-064`）。

**VE-c `UC-VE-007` 证据两支** — `handle_claim.go` 有索赔项与追偿，没有证据项。接：
- `SubmitEvidence`：证据到达形成证据项，评价起点固定为`已收到`，不由调用方指定（`AT-VE-113`「收到不等于内容成立」）。
- `PrepareDisclosure`：形成证据披露版本，披露范围必备、脱敏指纹不得与原件相同（`AT-VE-132`、`AT-VE-147` 受控复用）。依赖前者。

**VE-d `UC-VE-002`→`UC-VE-004` 冲突支** — `derive_projection.go` 分叉时把双方都留在场（`AT-VE-043`）但不裁决也不发信号；`derive_projection_test.go` 里 `TestAForkedSupersessionKeepsBothSuccessorsInTheProjection` 的注释写着「接 RaiseConflictSignal 是另一张票」——那张票就是这一组。接：
- `ResolveByBusinessTime`：按业务时间排全序；同刻即无法裁决，不硬凑（其余维度——因果、权威范围——各有自己的裁决函数，本组不混判、也不新写）。
- `RaiseConflictSignal`：只对未裁决的判断形成异常信号，携带全部保留事实引用，投影保持信息待确认。信号进 `UC-VE-004` 分诊。
- 接完把那条测试注释改掉——它引的「另一张票」有了。

## 完成判据

- 四组各自：对应 UC 步骤/AT 在编排里有一条真实路径走到该领域函数；`git grep -w <符号> -- 'internal/*.go' ':(exclude)*_test.go'` 在 application 层至少命中一处调用。
- 每组带真库用例与应用层测试；先写 red 再读既有编排。
- `production_wiring_baseline.txt` 名单按接线结果剪，在干净检出上重数并带 SHA，不加宽棘轮。
- 不写任何披露策略取值、异常规则阈值、责任团队实例；全部走登记面（`PAR-VIS-*`）。
- `gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿（含真库，`-v` 下看 `PASS`）。

## 地盘

`internal/visibilityexception/{application,ports,adapters/postgres}`；新迁移走 `migrations/visibility_exception/` 新号并按「同笔提交」纪律带 `migrations.go`（先占号）。`cmd/parcel-api` 端点表若加行另报。

## 完成记录（2026-09-04，MCP-6）

四组四笔，各自在 detached worktree 检出父提交 + 本笔文件上验过（gofmt -l 空、`go build ./...` 与 `go vet ./...` 退 0、`internal/architecture` 与 VE 全包 `go test -count=1` ok、DSN 已设真库用例 `-v` 下 PASS），提交前 `git diff HEAD -- internal/architecture/` 只有本笔 hunk：

| 组 | 提交 | 接上的领域函数 | 生产调用方 | 迁移 |
|---|---|---|---|---|
| VE-b | `10adcb3` | `EstablishCase` | `application/raise_signal.go` 的 `establishCaseFor`（分诊走向为自动建案时，案件随发作期与结论同一记录落库） | `0022` 分诊条目加 `responsible_team`（自动建案必带、其余必不带） |
| VE-a | `3ca94fc` | `DecideDisclosure` | 新用例 `application/decide_disclosure.go`；`notify_customer.go` 改为引用决定登记册里的产物 | `0023` 异常披露规则目录（版本+条目）与披露决定登记册 |
| VE-c | `7058d92` | `SubmitEvidence`、`PrepareDisclosure` | 新编排 `application/manage_evidence.go`（票 03 允许「或新文件」；与索赔编排分立是因为两边依赖无一重合） | `0024` 证据项与证据披露版本 |
| VE-d | `a18cdf7` | `ResolveByBusinessTime`、`RaiseConflictSignal` | `application/derive_projection.go` 的 `judgeForks` / `raiseConflict`，信号直调 `RaiseSignalHandler` 进 `UC-VE-004`；`derive_projection_test` 那句「另一张票」已改 | `0025` 冲突信号规则（一租户一条） |

随后 `5894320` 在 `a18cdf7` 干净检出上重生成机制清点；`4de7aba`（VE-c 补刀）把证据披露版本的行身份补上披露范围（迁移 `0026`；一个版本是「范围 + 脱敏版本」这一对，同一脱敏内容对另一相对方是另一个版本，7058d92 会把它静默读成已有），并同笔重生成清点。在 `a18cdf7` 干净检出上的终验：`go test -count=1 -v ./internal/visibilityexception/... ./internal/architecture/ ./cmd/... ./migrations/ ./internal/platform/migrate/` 全 ok，`--- PASS` 1281 / `--- SKIP` 0 / `--- FAIL` 0（DSN 已设）。

棘轮基线：函数名基线 VE 六条全部剪掉（各笔在自己父提交的干净内容上两法同得：23→22、16→15、15→13、13→11），类型基线 VE 十三型全部剪掉（60→56、34→28、28→25）；VE 组/段清空，注文留着提醒下一个加条目的人。

**与票面的偏差与留待（不在本票范围，各自另立）**：

- 三个新登记面（异常披露规则 `ExceptionDisclosureRuleRegistry`、冲突信号规则 `ConflictSignalRuleRegistry`、分诊条目的团队维已随既有 `RegisterTriageRules` 走 CLI/JSON）中，前两个只立了写入口（postgres，真库测试）与读口，**CLI（`parcel-ve-register`）与在线登记口未接**——单立端口是为了不拆 `CatalogRegistry` 的三处替身与受控 CLI 桩。要接线时形状照 `RegisterNotificationPolicy` 那一族。
- `DecideDisclosureHandler` / `NotifyCustomerHandler` / `ManageEvidenceHandler` 仍无 `cmd/` 装配（与票 03 取证时 `RaiseSignalHandler` 同一状态：有应用层调用方，尚未生产可达）。`cmd/parcel-dispatch/assemble.go` 已随 VE-d 给投影派生接上 `RaiseSignalHandler`（MCP-1 放行），所以信号 → 分诊 → 自动建案这一条在派发进程里今天是真路径。
- 待授权 → 披露的授权入口（形成新一版决定）、证据评价（`Appraise`）的编排入口、异常信号发作期上登记保留事实引用的登记格，三件都在票面之外。
- 影响范围与预计客户影响两维今天没有可查的登记维，异常披露规则条目键只到（客户 + 类型 + 可信度）；等它们有形状再扩键。
- `cmd/parcel-api` 端点表本票未加行。
