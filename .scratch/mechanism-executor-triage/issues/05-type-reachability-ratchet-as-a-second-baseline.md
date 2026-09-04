# 05 把类型可达性探针做成第二道棘轮：自己的基线文件，探针退役

Category: chore
Status: resolved——`16fc63e`（MCP-2，2026-09-04）；首版基线 63 条、探针已退役，验证见文末 Comments
Blocked by: 无（票 04 已裁）

## 要建什么

按票 [04](./04-should-the-ratchet-cover-the-new-family.md) 的裁决：**不动** `production_wiring_ratchet_test.go`
与 `production_wiring_baseline.txt`，在 `internal/architecture/` 下**另立**一道按类型可达性量的棘轮：

1. **量法**照探针 [`.scratch/domain-executor-audit/probe/main.go`](../../domain-executor-audit/probe/main.go)：
   从生产代码（非 `_test.go`）指名的种子出发，沿字段类型与方法签名闭包，闭包外的 `internal/<上下文>/domain`
   导出类型即「零生产消费者」。判据与函数名棘轮互补——它看得见 `New*` 构造的聚合根（`ProductChannelMapping`
   那一类）与 `NewDecimal` 那种命名方向相反的影子。
2. **基线文件** `production_type_reachability_baseline.txt`：冻住首次运行的名单，头部照函数名棘轮基线的写法记
   「在哪个 SHA 的干净检出上量得 N」与每条的理由行；**只许减不许增**，新增条目要在理由行写明它等谁
   （owner 已裁过的写法见函数名棘轮基线对 MCP-6 两条新增的记法）。
3. **首次基线要先去噪**：探针报的 122 是上界，票 04 点名了一类噪声——`Resolution` 那种只由未导出函数产出、
   导出了却只在包内用的类型，那是该改小写的设计瑕疵而非缺执行器。**不要把它们冻进基线**：要么先改小写
   （各上下文地盘，逐个在频道占号），要么在基线里单列一节「待改小写」并写明理由。基线里的每一行都要
   有人能答「它在等什么」。
4. **探针退役**：测试落地后删 `.scratch/domain-executor-audit/probe/`，目录 README 留一句指向本测试。

## 红线

- 不改函数名棘轮的判据与基线（可比性）。
- 不在测试里放任何产品判断：它只报名单变化，不判「等租户」还是「烂了」（与函数名棘轮头注同一立场）。
- 基线数字一律带 SHA 与「在干净检出上量」的注记（`docs/agents/parallel-sessions.md`「数一份共享文件里的条目」节）。

## 验证

`go test ./internal/architecture/ -run TypeReachability -count=1` 在干净检出上绿；人为把一个已接线的领域类型
从生产调用点上摘掉，测试必红并指名该类型；把基线里一条对应的类型接上线后，测试提示「基线可以剪」（与函数名
棘轮同一形状）。`gofmt -l` 空、`go build`/`go vet` 退 0。

## 边界

只加测试与基线文件，不接任何线、不改任何领域类型（改小写属各上下文自己的票）。

## Comments

- 2026-09-04 · MCP-2：**落于 `16fc63e`，转 resolved。** 实现基线 `37bea80`（认领时 HEAD），期间 HEAD 走到
  `08e62ec`（MCP-1 的 label-channel/10 第三层），未碰本票地盘。

  **做了什么。** `internal/architecture/production_type_reachability_ratchet_test.go` 三个用例（只许变短 /
  基线不许烂 / 门禁真能红），`production_type_reachability_baseline.txt` 首版 **63 条**（在 `16fc63e` 的
  detached 干净检出上以 `go test -run TypeReachability -count=1` 全绿并重数同得 63）。量法照探针，另多走
  三格且都只让名单变短：生产范围含 `cmd/`（装配点是真消费者，探针只扫 `internal/`）、导出常量/变量的声明
  类型算边（含 iota 组隐式继承）、未导出中间类型在图上。探针目录已删，`.scratch/domain-executor-audit/README.md`
  指向本门禁。函数名棘轮的判据与基线一字未动（`git diff 37bea80 16fc63e -- internal/architecture/production_wiring_*`
  为空）。

  **去噪怎么做的。** 63 条没有一条是零引用死码——全部至少经一个导出入口可取得，只是那个入口本身零生产
  调用。所以理由行统一写「经 X 取得，X 在等谁」，分三类：一类 X 在函数名基线（同一缺口的类型侧影子，
  随 X 接线出名单）；二类 X 是 `New*`（函数名棘轮网外那一格，理由在这边写全）；三类 X 只有包内调用
  （函数名棘轮按包内裸标识符也算引用所以不响）。**三类单列「待改小写候选」节**：票 04 点名的 `Resolution`
  与同形的 `ComplianceJudgment` 一族五条；改小写还是立接线票归各上下文所有者，门禁不裁。

  **票面「验证」三条各有证据。** (1) 干净检出全绿见上。(2) 在 detached worktree 里给
  `internal/parcelpricing/domain/decimal.go` 追加 `type ZZProbeOrphan struct{}` → 只许变短那条红，报文点名
  `internal/parcelpricing/domain ZZProbeOrphan`。(3) 给 `internal/partycommercial/application/register_party_identity.go`
  追加 `var _ domain.Resolution` → 基线不许烂那条红，报文点名 `internal/partycommercial/domain Resolution`
  并给「剪掉这一行，这是好消息」。两处改动均已复原，worktree `git status --untracked-files=all` 为空后拆除。

  **一处失手要记下。** 第一次跑变异用例时用 `[System.IO.File]::WriteAllText` 配相对路径，.NET 按进程
  工作目录解析而不按 `Push-Location`，**两处变异写进了共享树而不是 worktree**；随即用同一份原文写回，
  `git diff --numstat` 为空、无残留字样（`register_party_identity.go` 的 ` M` 是此前就有的 CRLF-only）。
  处方：.NET 文件 API 一律绝对路径。这一格与 workflow.md「本机环境」同族，值得补一条，归那份文档的下一次改动。

  **两轴评审**（基线 `37bea80`，subagent 鉴权故障改为串行自评）：Standards 拿住基线分组标题里的同文件
  内计数（剪条目会静默变错，已去掉）与 `readTypeReachabilityBaseline` 同形（不动旧门禁是红线，保留并注明）；
  Spec 拿住「待改小写」未按票面单列成节（已改）与头注未写明比探针多走的三格（已补）。

  **验证**：`gofmt -l internal/architecture/` 空、`go vet ./internal/architecture/` 退 0、
  `go test ./internal/architecture/ -count=1` 绿（共享树与 `16fc63e` 干净检出各一遍）。**本笔不含 .sql、
  不含 PG 用例、未跑 -race**——纯测试与文本文件，那两层无从验。
