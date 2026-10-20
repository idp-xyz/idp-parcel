# 02 组合根按文件自登记：加一个端点或一条派发路由不再改共享清单

Category: enhancement
Status: ready-for-agent
Blocked by: 无（但须等另一会话 idp-parcel-9b 的票 ps-external-mark-relations/14 进 main 后再基于 origin/main 开工，见「协作」）
地盘：`cmd/parcel-api/`（`endpoints.go` 的 `assembleBusinessEndpoints`、`main.go` 的构造与调用、`endpoints_test.go` 及其它调它的测试）、`cmd/parcel-dispatch/`（`assemble.go` 的 `assembleDispatcher` / `wireDispatcher` 路由表）；新 ADR-0222
出处：用户 2026-10-19 同意「热点清单冲突」基础设施票。近两周提交频次（`git log --since=2026-10-05`）：`cmd/parcel-api/endpoints.go` 40、`cmd/parcel-dispatch/assemble.go` 37、`cmd/parcel-api/main.go` 32、`cmd/parcel-api/endpoints_test.go` 25——每张加端点或加路由的票都改这几处的同一段，重放时反复冲突，且有一类冲突要手补 `if err != nil` 才编得过。

## 现状

- `assembleBusinessEndpoints` 是一个上百个位置参数的函数；加一个端点要同时改它的参数表、函数体、`main.go` 里的构造与按位传参、以及测试里按位传的那一长串未接线替身。
- 派发侧 `assembleDispatcher` / `wireDispatcher` 两个长函数里按序列出每条路由、每个消费者与它的未决哨兵名单；加一条路由改的是同一个函数的同一段。

## 要什么

- **加一个 API 端点 = 新增（或只改）该特性自己的文件。** 不改任何共享参数表、按位调用点、共享测试调用；生产装配与测试装配都从同一个登记处取。
- **加一条派发路由 = 同上。** 路由、消费者、未决哨兵名单、分区与信封门禁要求的东西随该特性文件走。
- 写法由作者定，写进 ADR-0222（号已占）。候选：包级登记（各 `assemble_<特性>.go` 在 `init()` 里向本包登记表追加一个「给定共享依赖 → 产出端点/路由」的构造函数）；或命名字段的依赖结构 + 各文件的 `register<特性>(…)` 由一张**生成的**清单调用。取舍要讲清：可达性与接线门禁（`internal/architecture`）怎么还看得见每条接线；未配置即拒（ADR-0055）与「真渠道就位前没有任何一行真 Intake」怎么守；初始化顺序与错误（构造失败）怎么如实中止进程，不吞。

## 不变式（硬门禁）

- **路由集与行为零变化。** 两个命令各出一份装配前后的路由清单（API：方法 + 路径 + 接入面/Intake 种类；派发：路由键 + 消费者名 + 未决哨兵名单 + 分区键来源），以测试或一次性对照证明前后逐条相同，对照方法与结果写进完成记录。
- `go build ./...`、`go vet ./...`、`gofmt -l` 干净；`./internal/architecture/...` 全绿且**不放宽任何基线**（若某基线按文件路径计数而自登记改了路径，说明原因并改基线口径，不是加例外）。
- 组合根仍在 `cmd/`：不向 `internal/` 引入任何登记表或反射式装配。
- 注释中文、只写为什么；搬动的既有注释原样随代码走，不改写其论证。

## 协作

- 另一会话 idp-parcel-9b 的地盘：`cmd/parcel-api/assemble_reception*.go`、`cmd/parcel-frontline-import`、`cmd/parcel-dispatch/assemble_node_identity_recheck.go`、`internal/nodeoperations/**`。本票不得不动这些文件的**装配接入方式**时，只做机械改写（把它们接进新登记处），不改其内容；动之前协调方会先告知对方。
- 本票落地后，所有在途分支加端点/路由的写法都要换成新形态：完成记录里写一段「新端点/新路由怎么加」的三五行配方，并把它放进 ADR-0222 的 Consequences。

## 验证

`scripts/test-full.sh`（infra-speed/01）跑全量一遍——本票动的是全部真库装配，受影响范围就是全仓；外加上面的路由清单对照。
