# 一线作业过渡的受控批量导入 CLI——模板 → 既有命令用例，硬期限守卫，来源标记

Category: enhancement
Status: in-progress——本轮范围（收寄子命令、期限守卫、模板与说明、真库往返用例）已全部落主线；余下的是被别处阻断的两格，见下
Blocked by: 无（本票自身不被阻断；两处**内容缺口**各有独立票，见「阻断在别处的两格」）

[ADR-0089](../../../docs/adr/0089-frontline-transition-controlled-import-with-structural-sunset.md) 的机制半边。
决定归 ADR，本票只记实现与验证；四类现场事实的逐格判定归
[映射表](../fact-to-usecase-mapping.md)，两份都不复述 ADR 的决定。

## 已落主线

`cmd/parcel-frontline-import`，子命令今天只有 `intake`（收寄）。

- **期限守卫** `guardStructuralSunset`：期限写成源码常量 `structuralSunsetLiteral`，守卫是 `run`
  的第一条语句，时钟经参数注入。三个用例钉住它——期限时刻本身放行、晚一纳秒即拒、同一
  时刻换时区表示不改判断；一个用例同时给坏 DSN 与自称豁免的环境变量，仍是启动拒绝且标准
  输出一行都没有，「没有绕过口」这句因此是验过的而不只是写着的。
- **模板译装** `decodeIntakeTemplate`：封闭列集（多一列拒、少一列拒、重复拒）、BOM 容忍、
  列序不限、一份文件一个批次、行事实号批内唯一；三态各自的必填与禁填逐格核清——`REFUSED`
  行给了 `evidenceRef` 是拒不是忽略，因为既有记录形状没有地方放它，静默丢掉会让内勤以为
  证据登进去了。
- **来源标记** `intakeSourceID` / `intakeEvidence`：`FTI/<模板版本>/…` 前缀落在 `SourceID` 与
  `Evidence` 两格。来源身份与录入者无关（同一张纸单被两人各录一次是重放不是两条事实），
  录入者落在证据引用里。
- **逐行推进** `importIntake`：一行一笔事务，部分成功是常态——整批一笔会让一行的冲突回滚掉
  其他行已合法形成的事实（UC-NO-002「部分收寄不回滚其他实物已合法形成的事实」）。
  `intakeRowDisposition` 把 UC-NO-002 的六格结果译成四格去向，不增不减；退出码里未决压过
  被拒，因为「重跑同一文件」在还有未决时仍是必要动作，而已落地的行重跑答重放、无副作用。
- **真库往返** `vertical_test.go`（照登记 CLI 家族形状）：`REFUSED` / `SCAN_ONLY` 两支真落库
  并读得回、`RECEIVED` 行在身份核对缝不通时**一行都不落**、重跑答重放、同一 `factRef` 换业务
  时间答来源冲突且原行记录时刻与摘要都不变。第二个用例换能解析的替身跑同一份模板，证
  `buildIntakeImporter` 的 `identity` 参数就是 PS 侧到位后唯一要换的那一格，并在真库上钉住
  证据引用带录入者、业务时间来自模板而记录时刻来自时钟——两者若同源，导入的事实就再也
  说不出现场什么时候发生的。
- **模板人读说明** [template.md](../template.md)：逐列格式、三态各自的必填与禁填、导入后四种
  去向各自要做什么。示例值全为合成值；`cmd/parcel-frontline-import/testdata/intake-v1.csv` 是
  机器侧的同一份形状，两边换列必须同时换 `intakeTemplateVersion`。**待 MCP-1 转客户**。

## 阻断在别处的两格

两格都**不阻断本票**，但决定了本口今天能导什么：

1. **`RECEIVED` 行运行期必然未决。** `ports.ParcelIdentityView` 全仓无生产实现，本口接显式
   未配置替身 `unconfiguredParcelIdentityView`（同 `cmd/parcel-api` 装配点的处置），开工前把
   这一点打印出来。`REFUSED` / `SCAN_ONLY` 两支不经身份核对，照常落库。恢复动作在 PS 侧，
   见 `.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md`；
   本口要换的只有 `buildIntakeImporter` 的 `identity` 参数那一格。
2. ~~**集运子命令本期不建。**~~ **已解阻（2026-09-02）**：
   [no-consolidation-fact-provenance/01](../../no-consolidation-fact-provenance/issues/01-consolidation-commands-carry-no-source-executor-evidence-or-business-time.md)
   已落主线，集运六口现在收 `domain.WorkFactSource`（来源身份、执行方、证据、业务发生时间），
   来源身份兼幂等键，业务时间不再取 `Clock.Now()`。本票加子命令的前提已备齐，备料仍在
   映射表「集运：输入逐格」节。

换单与称重两类无既有用例可接，按 ADR-0089 细则⑤ 如实记缺口、不造用例，见映射表。

## 未办

本票本轮范围已清。余下两件都不在本票地盘，各自解阻后再回来加子命令，见上「阻断在别处
的两格」。

## 完成判据

- `gofmt -l` 对改过文件无输出；`go build ./...`、`go vet ./...`、`go test -count=1 ./...` 绿并注明含不含真库。
- 期限守卫有拨钟用例（已办）。
- 真库往返用例贴 PASS 非 SKIP 的证据行（已办，取证见 Comments）。
- 模板说明存在且示例行无真实实例值。

## Comments

- 2026-09-02 MCP-1：立票并同笔落地。本票的实现是 11:47 崩溃现场 `mcp5-frontline-import@b0d4430`
  的封存件，经复核（`go build` / `go vet` 退 0、六个用例全 PASS）后按集成候选取回主树，
  只补了 `sunset_test.go` 的 `gofmt`。ADR-0089 先于代码落文（`32b78d7`），否则「一线过渡选 B」
  这个决定会只存在于代码注释里。
- 2026-09-02 MCP-1：补真库往返用例。按探针纪律一正一反各取一次证——**DSN 未设：6 PASS / 2 SKIP；
  DSN 已设（门禁容器 55432）：`TestFrontlineImportVerticalOnRealPostgres` 与
  `TestFrontlineImportLandsReceivedRowOnceIdentityResolves` 各 PASS**。反向那次真的数出了 2 个 SKIP，
  「这两个用例确实连了库」才算验过，而不是只看到一行 `ok`。
- 2026-09-02 MCP-1：**集运那一格的阻断解除**（`98e1752`，三笔 `f9c04da` / `3f304c9` / `98e1752`）。
  集运六口补上了来源事实那一层，形状与本 CLI 收寄口用的 `ReceptionKey` 同源，因此加子命令时
  模板那几列不必另谈一套词。要注意的是导入来源标记落点：收寄口走的是 `SourceID` 加证据引用，
  集运口同样由 `WorkFactSource` 的来源身份承担，节点作业查阅页的「开启来源」「封装来源」两列
  已能把导入与扫描两条路分开显示——这正是本票当初报出边界 B 的那个诉求。
