# VE 六条：步骤在 UC 里、执行器不在编排里——接线

Category: enhancement
Status: in-progress——MCP-6（2026-09-04，基线 `3fff246`）
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
