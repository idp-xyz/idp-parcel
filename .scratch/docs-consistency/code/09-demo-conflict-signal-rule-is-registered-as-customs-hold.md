# 09 演示租户把冲突信号规则登成 `CUSTOMS_HOLD`：一次交接更正分叉在演示动线上变成「关务扣留」并可对客披露

Category: bug
Status: resolved——2026-10-26 进 main（见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-code09（基于 main `47c589855`）。此前 ready-for-agent——R2（上线后尽快）：错在随产品发布的参考配置（`SYN-TENANT-01` 种子），不是机制；真实租户自己登冲突信号规则，但照抄参考配置的会继承同一个错。演示动线上一次替代链分叉就会对客说成关务扣留——对客答错，限于演示租户与照抄者。绕法：重登一条中性类型的冲突信号规则。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-16（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，种子未改）。

## 现象

冲突信号只由投影派生在替代链分叉裁不了时形成，来源可以是运输交接、交付等任何事实，不是关务事实；种子却把它的信号类型登成 `CUSTOMS_HOLD`。同一类型在分诊、异常披露两份种子里另有去处，于是这一格在演示上会挂到既有关务案件上、并对 `SYN-ACCOUNT-01` 可披露为关务扣留。

## 证据

- 文档：VE CONTEXT「关务相关异常只能消费 `customs-compliance` 已经接受且范围明确的事实或判断……查验、扣留、税费或内部限制等正常程序事实也只有命中已登记异常或客户影响规则时才形成异常信号。」
- 配置与代码：
  - `scripts/demo-seeds/data/visibility/09-conflict-signal-rule-v1.json`：`"signalKind": "CUSTOMS_HOLD"`。
  - 冲突信号的唯一形成处：`application.DeriveProjectionHandler` 的 `raiseConflict`。
  - `02-triage-rules-v1.json` 里 `CUSTOMS_HOLD` 走 `ATTACH_TO_EXISTING`；`08-exception-disclosure-rules-v1.json` 对 `SYN-ACCOUNT-01` 可披露（内容 `SYN-VE-CONTENT-CUSTOMS-HOLD-V1`）。
  - `scripts/demo-seeds/README.md` 与票 seed-completeness/02 的组成表把这一行写成了「冲突信号规则一租户一条，钉在 `CUSTOMS_HOLD`」。

## 三类

产品策略（随产品发布的参考配置）。机制不动；租户取值不动（真实租户自登）。

## 规模

S：改种子三份与 README 一段；若信号类型是封闭词表，加一个中性类型词属机制，一并做。

## 最小的诚实修法

冲突信号登一个中性的来源冲突类类型；分诊规则给它一条去处（例如新立案件或交运营复核），异常披露规则对它默认不披露；同步 `scripts/demo-seeds/README.md`「对外怎么说」一段。

## 要裁的

- 信号类型词：VE 现有封闭集里有没有可用的中性类型；没有就加一格（与票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md) 要立的「关务进展待确认」信号区分开）。作者可代决。

## 完成记录（2026-10-26，Claude Code 子代理，分支 claude-code09）

三类只动产品策略（演示租户的参考配置）；机制不动、无迁移、租户取值不动。

### 落点

- `scripts/demo-seeds/data/visibility/09-conflict-signal-rule-v1.json`：`signalKind` `CUSTOMS_HOLD` → `SOURCE_FACT_CONFLICT`，`confidence` `CARRIER_CONFIRMED` → `SOURCE_CONFIRMED`；版本号 `SYN-VE-CONFLICT-V1` 不变。
- `02-triage-rules-v1.json`：加一条 `SOURCE_FACT_CONFLICT` / `SOURCE_CONFIRMED` → `MANUAL_REVIEW`。
- `08-exception-disclosure-rules-v1.json`：加 `SYN-ACCOUNT-01`、`SYN-ACCOUNT-02` 各一条 `SOURCE_FACT_CONFLICT` / `SOURCE_CONFIRMED`，不披露、不自动发布。
- `scripts/demo-seeds/steps/300-visibility.sh`：那两条命令上方的注释随之改。
- `scripts/demo-seeds/README.md`：用法段补一句同号换内容要 `--reset`；「对外怎么说」一段改写冲突信号规则那几句。
- `cmd/parcel-ve-register/demo_conflict_signal_seed_test.go`（新）：`TestDemoConflictSignalSeedIsANeutralSourceConflictReviewedAndNotDisclosed`，把种子原件经生产登记链灌进真库，再用生产读口（`vepg.ConflictSignalRules`、`vepg.TriageRules`、`vepg.ExceptionDisclosureRules`）判冲突信号的类型、分诊去处与两户披露。
- 票 [seed-completeness/02](../../seed-completeness/issues/02-customs-ve-collection.md)：故事一句与组成表两行加前向注记，原记录不改写。

### 代决

1. **信号类型不是封闭词表，不加格。** `domain.ExceptionSignalKindReference` 是开放引用（其注释：「信号目录属实例参数」），各表 `signal_kind` 只有非空 CHECK；`PAR-VIS-04` 的通用异常类型目录归票 [product-strategy-boundary/11](../../product-strategy-boundary/issues/11-ve-eta-triage-disclosure-methods-and-notification-connector.md)，尚未立。没有封闭集可挑，也就没有机制格要加：无迁移、无 HTTP 词表、admin-web 判断规则页原样示码，ADR-0025 全函数纪律不触发。所以不需要动任何已接受 ADR，也不取迁移号。
2. **类型词取 `SOURCE_FACT_CONFLICT`（迁移 0025 头注里 `PAR-VIS-04`「事实冲突待确认」那一类）。** 冲突信号由 `DeriveProjectionHandler.raiseConflict` 在本上下文对已接受来源事实裁决不了（替代链分叉）时形成，判断方是本上下文、来源不限。票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md) 要立的「关务进展待确认」是关务所有者判出的同层冲突、限关务进展一维——判断方与维度都不同。本词不含 `CUSTOMS`、`PROGRESS`，不撞名；README 引 ADR-0281 决定九写明那一类另走一路、不用这一格，并写明今天关务同层冲突不形成信号，不撞义。
3. **可信度一并改 `SOURCE_CONFIRMED`。** 票面只点名类型。但分诊与披露都按（类型、可信度）取条目，新加的条目必须带一个可信度；照抄 `CARRIER_CONFIRMED` 会把「承运确认」写进两份新条目，而分叉的各方可以是交接、关务更正或任何源的事实。分叉关系本身由源上下文给出（各后继自己指名前身），所以记「源上下文确认」。演示租户的出发门禁阻断信号规则已用同一个词。
4. **分诊去处：人工复核，明登条目。** UC-VE-004「资料不足、可能重复或关联不明必须人工分诊」：一处分叉说明的是哪一支当前有效尚无从确认，而且一个来源的更正问题可能波及成批包裹——CONTEXT 对来源系统整体中断要求「优先按共同原因建立来源级信号或案件……不为每个包裹重复建立相同处置案件」，同一取向。自动建案还要求「高可信、高影响」，分叉在影响上未知；替代链分叉的冲突信号今天又没有恢复边界（UC-VE-004 交接已记，归票 docs-consistency/code/30），自动建出的案件没有信号侧的收口。分诊读口对已配目录里没有条目的信号同样答人工复核，仍然明登：参考配置要能在判断规则页上看到冲突信号的去处，而不是靠「没命中」落进那一格。
5. **披露：两户都明登不披露。** 披露读口对没有条目的信号答未配置，披露决定停在未决——那不是不披露（`ExceptionDisclosureRules.RuleForSignal` 的注释与 CONTEXT「披露条件不成立或授权不足时分别形成暂不披露或待授权结果」）。冲突本身是信息待确认，客户视图本就只由当前有效投影派生、分叉各方不参与派生，对客再披露一条「来源事实冲突」只会暴露内部数据状况。
6. **版本号不换，同号换内容，已灌库用 `--reset`。** 冲突信号规则一租户一条、键只有租户（0025），换哪个版本号都落不进已灌的库，换号只会暗示一条不存在的换版路径；分诊与披露两本册若另立 V2，演示故事要凭空多一个生效区间，而背后没有业务事件。先例是 README 已记的 `SYN-VE-MAP-V1` 与价格规则 v1。
7. **`CUSTOMS_HOLD` 的分诊与披露条目保留。** 它们本身不错，是关务扣留一类的参考去处；本票只把冲突信号挪出这一类。演示租户登的信号规则今天都不形成它们，README 如实写了。
8. **测试不点 AT 号。** 用例钉的是演示种子的参考配置，不是 `AT-VE-063`、`AT-VE-099` 的机制；点上会把两行从盘点的④挪进①，夸大机制覆盖。也没有新立 AT 号，`.scratch/at-coverage-inventory.md` 不动。

### 越权风险点

1. 可信度改了（代决 3），票面只点名类型；若评审认为可信度归另一张票，回退那一格要连同分诊、披露里新加的三条一起改。
2. 类型词是演示租户的参考配置；票 product-strategy-boundary/11 日后立通用异常类型目录时，可能要对齐这个词，届时同号再改要 `--reset`。
3. 分诊取人工复核而不是自动建案（代决 4），票面两种都列为例子。
4. 用例判「不借别类类型」靠专用断言：冲突规则的类型在分诊与披露两本当前版里只挂在它自己那一个可信度下。冲突规则若借用一个在两本里只登过同一可信度的别类类型，这道断言分不出，只剩分诊与披露两道断言按去处拦；`CUSTOMS` 子串只是命名习惯上的可读提示，不承重。
5. README 那句「演示租户登的信号规则都不形成这两类」说的是种子此刻的内容；日后有种子登了形成 `CUSTOMS_HOLD` 或 `ETA_GAP` 的规则，这句要随之改。
6. 改了一张已 done 的票（seed-completeness/02）：只加前向注记，原记录原样。

### 改动面（供与 demo-chain-gaps/135 对照）

只动上面「落点」各文件。没有碰 `RaiseSignalCommand`、`raiseConflict` 或任何 `internal/` 代码，没有改冲突信号的期次与恢复边界逻辑，没有迁移。

### 验证（证据 S）

- 先红：用例在原种子上红在类型断言（`冲突信号类型 = CUSTOMS_HOLD`）；改后 `-count=3` 绿（含 PG）。
- 变异七处全杀：类型改回 `CUSTOMS_HOLD`；类型与出发门禁共用；删分诊条目（靠没命中落人工复核）；分诊改自动建案；账户 01 可披露；删账户 02 披露条目；可信度改回 `CARRIER_CONFIRMED`（条目失配）。每次从备份还原，`cmp` 核过字节相同。
- `go build ./...`、`go vet ./...` 退 0；`gofmt -l` 空。
- `scripts/test-full.sh -count=1 ./cmd/parcel-ve-register/... ./scripts/demo-seeds/... ./internal/architecture/... ./migrations/... ./cmd/parcel-dispatch/...` 退 0（含 PG；后者的用例读同一种子目录）。改动包无反向依赖（`go list -test` 算）。
- 种子：一次性库 c09seed 干净灌退 0，库里冲突信号规则一行 `SOURCE_FACT_CONFLICT/SOURCE_CONFIRMED/SYN-VE-CONFLICT-V1`、分诊一条人工复核、两户不披露两条。先按 origin/main 灌（临时 detached worktree），再从本分支：三条登记命令各答 `REFUSED VERSION_NOT_OVERWRITABLE`、退出码 2，库里仍是 `CUSTOMS_HOLD`；不带 `--reset` 重跑 `seed.sh` 先停在网络目录（退出码 3，README 早有记）；`seed.sh --reset` 退 0，新行落库。用后删库、删 worktree。
- rebase 到 origin/main `820952abc`（无冲突；其间 main 未动本票所涉各目录）后重跑：build、vet、gofmt，新用例 `-count=3`，上面那组包 `-count=1`，一次性库干净灌，全部退 0。

### 评审与收尾（2026-10-26）

隔离评审结论可合入，0 阻断 0 N，另提三条信息，已在同一分支新提交里收掉：

1. 评审的两文件变异（冲突规则改为 `DELIVERY_FAILED/HEURISTIC`，两条披露同改）在上一版用例下存活（复测确认）。用例加专用断言：冲突规则的类型在分诊与披露两本当前版里只挂在冲突规则那一个可信度下。`CUSTOMS` 子串断言挪到末尾、头注写明不承重。复跑变异：评审变异被专用断言杀；类型改回 `CUSTOMS_HOLD`（可信度留 `SOURCE_CONFIRMED`）被专用断言杀；整份回到 main（`CUSTOMS_HOLD/CARRIER_CONFIRMED`）被分诊断言杀（答 `ATTACH_TO_EXISTING`）；拿掉 `CUSTOMS` 断言再跑这两种回退照样被杀；先前七处照旧全杀。种子与用例每次从备份还原，`cmp` 核过。
2. README「对外怎么说」里关务同层冲突那句改引 ADR-0281 决定九，并写明今天关务同层冲突到 visibility-exception 入账不派生、不形成信号（`adapters/customscompliance/derive_on_external_result.go` 遇 `record.LayerConflict` 直接返回）。
3. README 的中文释义对齐迁移 0025 头注原词「事实冲突待确认」；类型词 `SOURCE_FACT_CONFLICT` 不变。代决 2 的释义同改。

收尾前 rebase 到 origin/main `a01755e11`（无冲突）。其间 demo-chain-gaps/135、136 进了 main：信号发作期改按因果条件定位，UC-VE-004 把替代链分叉冲突信号缺恢复边界改归 docs-consistency/code/30。据此把代决 4 的出处改指 code/30，并删掉用例头注里「发作期按（对象、类型）定位、借用类型会并进别类发作期」那半句（已不成立）；其余代决不受影响。在这个 tip 上重跑 `go build ./... && go vet ./...` 与 `scripts/test-full.sh ./cmd/parcel-ve-register/... ./scripts/demo-seeds/...`（含 PG），新用例 `-count=3`，一次性库干净灌一次，全部退 0。之后 main 又进到 `47c589855`（未动本票所涉各目录），再 rebase 无冲突，build、vet、上面那组包与新用例 `-count=3` 重跑退 0。

## 进 main 记录（2026-10-26，协调方）

- 分支 `claude-code09` 由作者 rebase 到 origin/main `47c589855` 后快进，tip `7f7ef1ad8`。只改参考配置（种子三份、README、步骤注释）与一条种子真库用例，没有动 `internal/` 代码，没有迁移。
- 隔离评审可合入，0 阻断 0 N；三条信息在收尾轮收掉：用例加「专用类型」断言（评审的借用别类类型的两文件变异由此被杀，`CUSTOMS` 子串断言不再承重）；README 改引 ADR-0281 决定九并补「今天关务同层冲突不形成信号」；中文释义对齐迁移 0025 头注原词。收尾 diff 与处方一致，协调方核过后未再起第二轮评审。
- 与另一会话的 demo-chain-gaps/135、136（VE 出发门禁信号与冲突信号恢复边界）改动面不相交；协调方进 main 前与对方核过逐文件清单。
- 协调方单独提交重生成的机制清点；ADR 索引与 AT 计数无变化；检查均退 0。
- 全量 `scripts/test-full.sh` 退 0、158 个包 ok；`go build ./...`、`go vet ./...` 干净；演示种子在一次性库上灌通，用后删库。
- 去向：无新票。信号类型词 `SOURCE_FACT_CONFLICT` 已被演示种子占用，票 demo-chain-gaps/124 定「关务进展待确认」的类型词时须避开（已告知该票所在会话）。
