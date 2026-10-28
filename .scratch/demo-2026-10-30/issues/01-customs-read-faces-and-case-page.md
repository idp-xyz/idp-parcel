# 01 关务读口与 customs-cases 页

Category: enhancement
Status: resolved——2026-10-28 进 main（`dc7830ec8`；全量 162 ok / 0 FAIL，admin-web 822 pass，种子退 0），见文末。此前 in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-demo-cc（代码最后一笔见完成记录）。此前：2026-10-28 tops-20（idp-parcel-91）立并认领，分支 claude-demo-cc
Blocked by: 无

## 现象

审计（`../ui-coverage-audit.md`）：`/customs-cases`、`/customs-declaration-submissions` 只有 POST；申报发送与技术回执、放行结果、关案记录、门禁判断册（交出读数 `ADMITTED` 只在交接登记响应体里）都没有 GET；`customs-cases` 页「关务案件、申报单元、资料快照、提交版本」四签是占位。演示「立案 → 申报 → 发送回执 → 放行 → 门禁放行交出」在管理台上看不到。

## 做什么

后端（`internal/customscompliance` 读侧 + `cmd/parcel-api`，照现有 CC 查阅口写法，操作者查阅面、租户取自认证信封、只读）：

1. 案件列表与单份读回（含申报单元、状态、关案记录若有）。
2. 申报提交版本与发送尝试 / 技术回执（PSB/10 第 1 项 ADR-0342 的发送事实与回执）。
3. 放行结果（外部结果登记与归属、`release_outcome`）。
4. 动作准入判断册读面（按包裹或申报单元列各受守护动作的读数与依据）。

前端（`apps/admin-web` 的 customs-cases 页）：四个占位签接上 1–3；加一签或一区显示 4（按包裹查）。照同页已接线签与 `party` 区页面写法，空态写清是什么。

不做：新的写口；改判断语义。

## 完成判据

- 真库用例：各读口在演示租户形状数据上答出内容、他租户不可见、未授予 403。
- 管理台：演示租户跑完 `declare-one-parcel.sh` + 发送 + 放行 + 交出后，四类结果在页面上可见（组件单测 + 类型检查；能起浏览器就截一次）。

## 完成记录

- 起点 `5d5a4484b`；后端 `b6697ccee`，前端随后一笔。
- 后端四个操作者查阅口（operationsReadFace、租户取自信封、只读、limit 取 Intake）：`GET /customs-case-records`（案件+申报单元+关案记录，`?case=`）、`/customs-submission-records`（提交版本+尝试+发送事实+技术回执，`?version=`）、`/customs-external-result-records`（六层外部结果，放行层带放行事实，`?version=`）、`/customs-action-admission-judgements`（判断册，`?object=`）。读侧 SQL 在 `internal/customscompliance/adapters/postgres/operations_record_views.go`，端口在 `ports/operations_records.go`，不折叠任何层。
- 前端：customs-cases 页四个占位签接真，另加「监管结果与放行」「门禁判断册」两签（判断册按包裹/单元号查）。资料快照签取自提交版本读回（资料引用、角色快照引用、摘要），字段级溯源不在读面。
- 代决（须评审）：1 路径命名（`/customs-*-records`，避开已有 POST 路径）；2 行类型用纯字符串不重建领域对象（查阅面不替写口把关）；3 判断册 `?object=` 按 object_ref 精确匹配，包裹或单元号取决于消费方登记什么；4 未立 ADR、未取 AT 号。
- 自验（含 PG）：`go build`、`go vet`、`gofmt`；`go test -p 1 ./internal/customscompliance/... ./cmd/parcel-api ./internal/architecture/...` 全绿；前端 `tsc --noEmit` 与 `pnpm test`（813）全绿。真库用例：演示租户形状数据答出内容、他租户不可见、空册答空、limit 非正拒；403 与 405 与 5xx 在 http 单测。未起浏览器截图，未跑种子端到端。
- 留给协调方：演示前用 `declare-one-parcel.sh` + 发送 + 放行 + 交出后在页面上目测一次；`/customs-internal-restrictions` 无 GET（审计 2.7）不在本票；机制清点需重生成。

## 评审记录（2026-10-28，隔离子代理，钉 `2d5d9fd0c`）

两轴无阻断。租户隔离逐处核过（主表与各相关子查询、LEFT JOIN 的 ON 都带租户；他租户为空有真库用例）；各层原样上列、不折总状态，封闭值与迁移、`external_result.go` 层名对上；前端六个 interface 与后端 json tag 逐字段一致；路径与 `/exception-case-records` 等同族。判断册 `object_ref` 是声明包裹号（交出门禁按包裹号写入），按包裹号查得到、按申报单元号得空——前端标签与注释原写「按包裹或申报单元查」会误导，推送方改为「按包裹号查」（`70d530089`）。非阻断记入冻结清单：固定 limit 200、无游标，外部结果整表拉回在前端按版本挂，超 200 条时较晚版本会显示「尚无结果」。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-demo-cc` tip `2d5d9fd0c` 作出处；隔离树 cherry-pick 到 main `6bf5f4b0c`，加文案订正一笔；重生成生成物、两项 `-check` 退 0；admin-web `tsc -b` 与 822 项单测通过；全量 162 ok / 0 FAIL；种子一次性库退 0；推确切 SHA `dc7830ec8`。

