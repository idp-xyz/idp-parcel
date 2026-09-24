# 01 演示库上隔离写路径的委托提交恒答 `ADMISSION_PAUSED`：种子注释「准入控制为 OPEN」在保守规则下已不成立

Category: bug
Status: resolved——2026-09-24 通道 4 交付（分支 `mcp4-dia01`，代码 tip `d777afab`、含清点的 tip `7071091e`，基 `71bdbf71`；派单 `task-e7f42ddf` ← 通道 3），待非作者评审与推送方重放；分支已推 `origin/mcp4-dia01`
Blocked by: 无
地盘：`cmd/parcel-governance-register`（新增阶段评审登记子命令）、pilot-governance 应用层里阶段评审的入口（若 CLI 需要新接）、`scripts/demo-seeds`（`seed.sh`
与 `data/governance/`）。碰 Go，走[并行会话](../../../docs/agents/parallel-sessions.md)那条路。
出处：2026-09-24 通道 3 备真浏览器验收环境时实测（独立库 `idp_verify`，代码钉 `04898af1`）。

## 现象

隔离读、写两个开关都开（`SYN-TENANT-01`），演示种子全量灌入之后，`POST /shipment-requests` 答 `ADMISSION_PAUSED`，
`productionOwnership.suspensionReference` 为 `SYN-GOV-SUS-0002`——种子 `06-suspension-pricing-scope.json`，范围 `SYN-PILOT-SCOPE/pricing@v1`。
委托受理维的范围却是 `SYN-PILOT-SCOPE/shipment-intake@v1`（种子 `07-authority-interval-shipment-intake.json`）。演示库上因此造不出任何委托，
委托查阅、检查器、列表 → 详情往返在演示环境里都没有数据可验。

## 为什么

pilot-governance postgres 适配器 `Suspensions.FindUnresumedSuspension` 头注：答不出覆盖关系时**保守答暂停**
（`domain.AdmissionSuspendedByUnreadableScopeRelation`）；只有 `scope_version_relation` 里登了「互不相干」的边，别处的暂停才不及于所问版本。
种子从没登过这条边，所以任何一笔未恢复的暂停都会拦住委托受理。而 `seed.sh` 在委托受理那一行上方的注释仍写「本行的试点范围版本与上面两笔暂停
各不相同，因此不受它们影响，准入控制为 OPEN」——那是保守规则落地之前的口径。

种子登不上那条边，是因为治理 CLI 今天只开了 `authority-interval` / `suspend` / `resume` 三类；范围版本关系随阶段评审登记
（`application/record_stage_review.go`），CLI 没有入口。

## 裁决（用户 2026-09-24 授权通道 3 自决）

按保守规则的本意修：给治理 CLI 开阶段评审登记口（种子注释里「阶段评审与接管第二批」本就要开的那一类），种子为 `shipment-intake@v1` 与两笔暂停
各自的范围各登一条「互不相干」关系；`seed.sh` 那句注释改成现状。

不取「种子把 0002 也恢复掉」：那会让治理页失去「暂停中」的演示格，而且只是把这次撞上的那一笔挪开——下一笔没恢复的暂停照样拦。

## 完成判据

- 干净库上 `seed.sh` 之后，隔离读写都开，`POST /shipment-requests` 答 `SUBMITTED`；治理页上仍有一笔暂停中。
- CLI 新子命令有用例；`seed.sh --reset` 重灌全程零报错。

## 旁证

验收环境里临时在 `idp_verify` 为 0002 登了一笔恢复，才造出三条委托；那笔恢复只在隔离库里，不进种子。

## 完成记录（2026-09-24，分支 `mcp4-dia01`，基 `71bdbf71`）

作者：通道 4（派单 `task-e7f42ddf` ← 通道 3）。落点五笔，按次序：

- `6dedd097` 命令词段：`domain.ChannelCommand` 加第二批的 `stage-review`；迁移 `pilot_governance/0007` 按 0006 头注的约定放宽
  `channel_execution_command_closed`（约束名不变、只增不改）。
- `6306d3aa` 应用段：`application.FixCandidateSetHandler`——评审所引候选版本组进册的那一步。
- `50f3315d` CLI 段：`parcel-governance-register stage-review`（翻译、执行、答复转写），装配补候选组 / 决定 / 覆盖关系三库。
- `d777afab` 种子段：`data/governance/08-stage-review-shipment-intake.json`；`seed.sh` 治理段注释改成现状，07 之后加一行。
- `7071091e` 在分支干净检出上重生成机制清点。

对完成判据：

- ✅ 干净库上 `seed.sh` 之后隔离读写都开，`POST /shipment-requests` 答 `SUBMITTED`；治理页上仍有一笔暂停中。一次性库
  `idp_mcp4_dia01check`（已删）上起 `parcel-api`（隔离读写 `SYN-TENANT-01`）：答复 `outcome: SUBMITTED`、`admissionControl: OPEN`；
  `GET /governance-registers?register=suspension` 列 `SYN-GOV-SUS-0001` 与 `SYN-GOV-SUS-0002`，`register=resumption` 只有 0001——0002 仍在
  暂停中。反证：在 `main` `ef9b8225` 的临时检出上用原版种子灌另一个一次性库，同一请求答 `ADMISSION_PAUSED`（`PAUSED/SYN-GOV-SUS-0002`）。
- ✅ CLI 新子命令有用例：`stage_review_test.go`——翻译逐字段、七种拒收、两段答案全格、真库纵向（登关系前保守拦
  `SUSPENDED_BY_UNREADABLE_SCOPE_RELATION` → 登后 `NOT_SUSPENDED` 而暂停对其自身范围照旧 `SUSPENDED_BY_NAMED_SCOPE`、重放答
  `EXISTING_DECISION` 留第二痕、同标识异内容的候选组答治理冲突、评审未受理时候选组随整笔事务撤回且不留痕）；应用层
  `fix_candidate_set_test.go` 与领域命令词往返另有用例。
- ✅ `seed.sh --reset` 重灌全程零报错：同一一次性库上干净灌与 `--reset` 重灌都退 0，都走到 `stage-review: RECORDED` 与「种子灌入完成」
  （脚本 `set -euo pipefail`，任何一条 CLI 非零即中止）。

裁决四条都落了：CLI 开阶段评审登记口；种子为 `shipment-intake@v1` 与 `routing@v3`、`pricing@v1` 各登一条互不相干；`seed.sh` 那句注释
改成现状；没有把 0002 恢复掉。

门禁（钉 `7071091e`，worktree 无未提交改动）：`gofmt -l .` 0 个文件；`go build ./...` / `go vet ./...` 0；带 DSN `go test -count=1 -p 1`
跑改动包及其反向依赖共 11 包：10 ok、1 无测试文件（`pilotgovernance/ports`），415 条用例 pass、0 fail、0 skip。迁移 0007 按字节 0 个 CR、
无 BOM。全量由推送方跑。

判断项（票面没定、由本票取的，留给评审）：

1. 评审要引已在册的候选版本组，而生产路径此前没有任何地方形成它：`stage-review` 输入随带该次评审固定的候选组，由
   `FixCandidateSetHandler` 进册（同内容重放、同标识异内容答 `CONTENT_CONFLICT`——组不可扩张），与评审同一笔事务；评审没落成即整笔
   撤回，不留没有评审的组。仍只开票面裁决的一个子命令，没有另开候选组登记口。
2. 评审的范围版本与候选组引用都取自输入里那份候选组，输入不另给第二份，免得两处写得对不上。
3. 退出码沿本入口约定：落定 / 已有决定 0；候选组同标识异内容、权威冲突、覆盖关系冲突、决定已落而续办引用非空 2；未受理 1；未决 3。
4. 种子评审取 `SHADOW_RUN` 上 `GO`、不授予区间：委托受理的权威区间仍由 07 经 `authority-interval` 登记，同一区间再授一次会被冲突预检
   拦下；决定时点 2025-12-28、生效 2026-01-01，与 07 的区间起点对齐。
5. 地盘补两格：`internal/pilotgovernance/domain/channel_command.go`（命令词封闭集在领域）与 `migrations/pilot_governance/0007`（放宽留痕表
   CHECK，不放宽就记不下痕、命令答未决）；开工时已经 `report_task` 报备通道 3。
6. 阶段评审与接管都还没有读面（治理读面只有权威区间、暂停、恢复三册），stage-admission 页那两格照旧说明未开；管理台不在本票。
7. 旁见：`scripts/demo-seeds/seed.sh` 在仓库里登记为 `100644`，README 写的 `./scripts/demo-seeds/seed.sh` 在 Linux 上答 Permission denied；
   本票验证一律用 `bash scripts/demo-seeds/seed.sh`。文件模式不在本票地盘，未动，可另立票。
