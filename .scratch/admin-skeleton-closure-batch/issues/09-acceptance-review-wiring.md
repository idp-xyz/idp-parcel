# 接受前人工复核页接线：域与决定机制全在，缺的是队列读面与两个命令口

Category: feature
Status: resolved（随 `df51ce0` 入库；MCP-1 收尾交付，MCP-5 复核后收口）
Session: MCP-5（2026-08-31 立票即认领，频道指示「两页骨架完整实现」）→ MCP-1 接手收尾
Blocked by: 无（08 号票同人承接，文件不相交处并行，相交处按序落）

## 事实基线（取证于 `4b35815`）

页面骨架 `apps/admin-web/src/pages/governance/AcceptanceReviewPage.tsx` 三区全 `unconfigured`。
后端这半与其余骨架页不同，**机制几乎是全的**：

- 域：`ShipmentRequest.CompleteManualReview`（authority/reviewer/evidence 三引用必填、重复
  完成拒绝、决定越界后拒绝补录）、`ResumeByManualReview` 等待态、重建保真（rehydration_test
  三条守着）全在 `internal/parcelshipment/domain`。
- 应用：`RejectShipmentRequestHandler`（授权先于一切写动作、撞既有决定交回那一个、释放资金
  控制）已实现**但没有任何 cmd 构造它**；`AdvanceAcceptanceChainHandler` 在
  `cmd/parcel-dispatch` 装配，复核门由信封重投自然过（ADR-0081：完成复核落库后，下一次重投
  在 `FormAcceptanceDecision` 处读到 `ManualReviewCompleted` 即成决定）。
- 读：`shipment_request.snapshot->acceptanceTask.waitingOn`（uint8，`3=MANUAL_REVIEW`）已
  持久化，但没有投影列，查不动「等复核的都有谁」。

缺的三样：**队列读面**（按作用域列出 waitingOn=MANUAL_REVIEW 的已提交委托）、**复核完成的
应用处理器与命令端点**、**主动拒绝的命令端点**。

## 范围（机制半边，先例照抄同批 02–06）

1. 迁移 `migrations/parcel_shipment/`（顺延编号）：`task_waiting_on smallint` 投影列 +
   快照回填 + 部分索引；`Insert`/`Save` 同步写列。
2. `ports.go`：`ManualReviewQueueView`（键 `AuthorizedQueryScope`，同 `ShipmentRequestViews`
   ——运营可见集是装配注入的授权结果，ADR-0078 Decision 一）+ 队列行记录。
3. 应用 `complete_manual_review.go`：找回聚合 → `CompleteManualReview` → `Save`。**不驱链**：
   推进属派发一拍（ADR-0081），复核完成只是把门闩拉开。
4. HTTP：队列查阅端点（GET）+ 复核完成端点（POST）+ 主动拒绝端点（POST）。命令行照红线挂
   **字面量 `UnconfiguredIntake{}`**（ADR-0055；`.scratch/admin-write-faces/01` 抄送的红线：
   隔离准入不得扩到写行），真实编排照常构造注入——差的只是采信那一步。
5. 隔离读准入：`IsolatedOperationsReadIntake` 加队列查阅方法（读行沿用 ADR-0078 开关）。
6. `cmd/parcel-api`：三行端点 + 真编排构造（复核完成、主动拒绝）+ unwired 占位对应项。
7. 页面接真 + `liveIds` 登记。决定按钮打到命令端点，403 `ACCESS_CHANNEL_NOT_CONFIGURED`
   如实呈现「渠道未配置」——不造开发用采信身份。

## 完成判据

- 全仓 `go build`/`go vet`/`gofmt -l`（无输出）绿；`go test ./...` 绿且注明含不含 PG
  （`IDP_PARCEL_POSTGRES_DSN` 设与未设各一遍，PG 侧队列/投影用例真跑）。
- `apps/admin-web` `pnpm build` 绿；页面在隔离读环境列得出等复核委托、决定按钮如实 403。
- 票面回填提交 SHA 与验证结果。

## Comments

- 2026-09-01 · MCP-1 接手收尾。MCP-5 的会话 2026-08-31 晚崩溃，1–6 步代码留在工作树未提交；
  用户 09-01 指示本频道接管，MCP-5 复活后改派票 08 域模型半边（地盘划分已广播）。

  **接手时的实测状态**（基线 `1665fdb`，工作树未提交）：`go build`/`go vet` 绿，
  `go test -count=1 ./...` 四红。其中一红与本票无关——整树被某工具重写成 CRLF，
  `migrations` 的行尾守卫因此报红，`gofmt -l` 同时报 32 个文件。已整树归一为 LF（73 个文件，
  字节级替换，`git diff --stat` 前后同为 22 files / 514+ / 58-，抽查 `git hash-object`
  与索引 blob 同 SHA），守卫与 gofmt 随之转绿。成因不是 `.gitattributes`（`*.go`/`*.sql` 等
  已钉 `eol=lf`），是别的工具写出的 CRLF；`core.autocrlf=true` 来自 system 级配置。

  **本票欠的三处守卫已补**：
  1. `manual_review_completed_handoff.go` 未声明分区主体 →
     `internal/architecture/partition_subject_registry_test.go` 补一行「租户/客户账户/委托」，
     与「委托已提交」同主体是有意的（续办不得越过未投出的提交，ADR-0086 Decision 二），
     票面注明避免下一个人当重复行剪掉。
  2. `OutboxManualReviewCompletedHandoff.HandOffManualReviewCompleted` 缺无事务负向证据 →
     新增 `manual_review_completed_handoff_test.go`，照同包「委托已提交」交接测试逐条对照：
     同事务成立、回滚一并消失、重发同一份、无事务拒、缺完成留痕响亮报错，共五条。
     该口此前一条测试都没有。
  3. `/acceptance-review-queue` 在隔离读启用态返 500 而准入名单没有它 →
     `isolatedReadAdmittedPatterns` 补一行。它与委托查阅共用同一个 Intake 变量，
     漏这一行等于断言「同一个 Intake 会给出两种答案」。

  **第 7 步已做**：`AcceptanceReviewPage.tsx` 接 `GET /acceptance-review-queue` 列表与单份两支，
  决定区打两个命令端点，`liveIds` 补 `acceptance-review` 一行。读面形状与命令草案落
  `apps/admin-web/src/pages/shipment-request/api.ts`（parcel-shipment 传输层的镜像所在处）。

  两点与原票面措辞不同，理由记此：
  - 按钮不叫「复核通过／复核不通过」，改为「记录复核完成」与「主动拒绝委托」。两者不是
    同一动作的两个方向——前者只落留痕、决定由下一轮判断形成（ADR-0086），后者在自己的
    命令事务里当场形成决定且不发续办信封（Decision 三）。原标签会让复核角色以为自己在表决。
  - 命令草案只送 `shipmentRequestId` 与 `reason`。复核人／授权依据／证据引用**故意不送**：
    那三样由已认证操作员身份经 PAR-INT-01 翻译得出，页面填任何值都是伪造采信身份。

  **验证**（`IDP_PARCEL_POSTGRES_DSN` 已设、库可达）：本票涉及的包
  `cmd/parcel-api`、`cmd/parcel-dispatch`、`internal/parcelshipment/adapters/...`、
  `application`、`migrations` 全部 `ok`，**含真库**——新增五条交接用例单跑 `-v` 得 `PASS`
  非 `SKIP`。`gofmt -l` 对本轮改动文件无输出。`apps/admin-web` `npx tsc --noEmit` 无输出、
  `pnpm build` 绿。

  **顺带修掉一处本票造成的注释错位**（由 MCP-3 的只读普查点名，我核实后改）：
  `cmd/parcel-dispatch/assemble.go` 的 `wireDispatcher` 注释里三处序数引用
  （「第二条投向 network-routing」「第五条只接 effective-delivery」「第六条只接
  transport-handover」）在 `1665fdb` 上**全都是对的**——那份枚举从「PS 接受决定」起算。
  本票在枚举头部插入「委托已提交」与「复核已完成」两条后，三处同时错位两位，
  而 `go build`/`go vet`/全仓 `go test` 对此零信号。已全部改为按名字指，并去掉开头的
  「今天有十三类事件」计数（该计数当时也已与映射的 14 个键对不上）。这正是 AGENTS.md
  「计数与行号同构」那条禁的东西，此处留一句成因说明，免得下一个人再写回序数。

  **未做与不由我做的**：本票未提交（用户要求逐批指示后再提交）。全仓 `go test ./...` 此刻
  仍有两红——`TestNoNewProductionFactoryGoesUnwired` 与
  `TestEveryIdentifierOnTheRehydrationSurfaceIsClassified`，均指向 MCP-5 正在写的
  `internal/parcelshipment/domain/label_transaction*.go`（`EstablishLabelTransaction`、
  `RehydrateLabelTransaction`），属票 08 地盘，未动。`docs/adr/README.md` 上那一处改动同时
  含 ADR-0084（票 08）与 ADR-0086（票 09）两行，提交时各取自己那一行、不动邻行。

- 2026-09-01 · MCP-5：**收口。** 上一条写在提交之前，此后本票随 `df51ce0` 入库并已推到
  `origin/main`，但 Status 一直没人翻——MCP-1 在翻之前下线了。我在 `e35d898` 上逐项核过范围
  那七步都在树上（投影列迁移、`ports` 队列口、`complete_manual_review` 应用层、三个 HTTP 口、
  `cmd/parcel-api` 三行端点与真编排、页面接真与 `liveIds` 一行），据此改 resolved。
  上一条列的两红此后也已消：它们指向的是票 08 的 `label_transaction*.go`，两道门禁按各自
  规则登记后转绿，随 `9fbcf24` / `e35d898` 入库。
  **翻状态不是形式**：跟踪器现在只剩一个会话在读，一张写着 in-progress 的已完工票会让下一个
  人（很可能是几天后的我）重新去查它到底做没做。
