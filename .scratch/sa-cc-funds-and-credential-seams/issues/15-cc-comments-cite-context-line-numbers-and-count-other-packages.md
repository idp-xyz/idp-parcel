# CC 注释里的「硬句 NNN」是 CONTEXT 行号、已随 ADR-0137 插句错位；`parcel-customs-register` 头注数着 `application` 包的格数——一笔改口成引文与点名，零行为

Category: chore
Status: ready-for-agent——2026-09-11 14:2x 通道 1 推送方立票并直接转 ready（要裁的为零；收下 sa-cc/10 非作者评审 Standards ① 与 sa-cc/07 步一评审 Standards ① 两条非阻断，扩到同族全部）；取证锚 main `0b027ab8`
Blocked by: 无（硬）。**软阻**：第五波 sa-cc/04（通道 4）/ 05（通道 5）/ 07 步二（通道 2）都在 `internal/customscompliance/**` 与 `cmd/parcel-customs-register/` 动手，本票是同目录大面积注释改动，先于它们进 main 会让三条分支 rebase 时逐文件解注释冲突——**等这三票进 main 后再开工**，推送方广播后转派——**05 已 15:1x 进 main、07 步二已 16:0x 进 main（2026-09-11，通道 1 推送方记），只剩 04**；07 步二评审又点了两处同族（`adapters/http/register_credential_and_duty.go` 头注「同一套八格」与 `writeDutyReconciliationAnswer` 头注「资金事实那三格」、`apps/admin-web/src/pages/customs/api.ts` `dutyRegistrationOutcomeLabels` 注释「资金事实三格」），随本票一并收。05 评审 Spec ② 与 07 步二判断项 ①（CLI 与 HTTP 答复面都未透出 `HandoffReference()`）**不并入本票**——那是答复形改动，本票零行为；归 CC owner 另立一张小票（tasks.md 15:0x 节写的「并入 sa-cc/15」由 16:0x 节更正）

## 缺口（取证于 `0b027ab8`；数本身是论点，故写数并锚 SHA）

- `git grep -n -E '硬句\s*[0-9]+'` 在 `internal/customscompliance/**` **76 处 / 46 文件**，`cmd/parcel-customs-register/` 2 处 / 2 文件（`translate.go` `gateKeyFrom` 头注、`translate_test.go` 一条 `t.Fatalf` 文本），`apps/admin-web/src/pages/customs/` 6 处 / 4 文件（`CustomsCasesPage.tsx` ×2、`CustomsRestrictionsPage.tsx`、`api.ts` ×2、`register-rows.test.ts`）。
- 那些数字是 `docs/domain/customs-compliance/CONTEXT.md` 的**行号**，不是 CONTEXT 自带的编号（CONTEXT 里没有任何「硬句 N」标记）。实测于 `0b027ab8`：第 212 行 =「法定义务人、实际付款方和最终承担费用的客户可以不同，不能互相推导」（代码引 212 ✓）、214 = 三态「不能实现为一组互斥总状态」（✓）、216 =「门禁满足不生成放行，也不能复用于其他动作或监管边界」（✓）；**第 217 行是 ADR-0137 落地时插入的「税费付款门禁怎么读核对」那句**，其后每行下移一行——代码里的「硬句 218」（关闭前逐项盘点全部适用义务 / 任一义务未终结阻止关闭，`case_closure.go` / `obligation_inventory_view.go` / `close_customs_case_test.go` 等）今天指到的 218 行是「监管核定及法定义务由本上下文拥有……」，「硬句 219」（承接项必须指名接收责任方，`query_case_registers.go` / `registrationjson/translate.go` / `case_config_registry_test.go` / admin-web `api.ts` 等）指到的 219 行是义务盘点那句。AGENTS.md「改文档」条预言的失效模式已经发生：指错了，没有任何东西报。
- 跨包计数（AGENTS.md「计数与行号同构」）：`cmd/parcel-customs-register/translate.go` `answer` 头注与 `configurationCall` 头注（「案件配置族的八格、税费付款协作与核对族的十三格」「凭证四个 handler」）、`main.go` `configurationAnswer` 头注（「封闭八格」）与 `dutyReconciliationAnswer` 头注（「封闭十三格」）、`duty_registers_test.go` 文件头注与 `TestDutyReconciliationAnswerCoversEveryOutcome` 头注（「十三格 / 八格」）、`main_test.go` `TestConfigurationAnswerCoversEveryOutcome` 头注（「八格」）——数的是 `internal/customscompliance/application` 的 `CaseConfigurationOutcome` / `DutyReconciliationOutcome` 常量个数；那边加一格是正当改动，改的人不会路过 `cmd/`。sa-cc/07 步一评审 Standards ① 点的就是这一族（「八格」是旧例、「十三格」是新添）。
- 出处：sa-cc/10 非作者评审 Standards ①（`query_duty_collaborations.go` / `query_duty_verifications_test.go` / `CustomsRestrictionsPage.tsx` 三处「硬句 212 / 214」，当时判「沿包内旧例非新造」、归 CC owner 一笔换引文）；sa-cc/07 步一评审 Standards ①（「十三格」四处）。两条都随票记为非阻断，本票收口。

## 做法

1. **行号 → 引文**：每处 `硬句 NNN` 换成该句的一小段原词引文（≤ 20 字，用「」），数字删掉。注释里已经引了原句的（如「CONTEXT 硬句 214 明禁实现为一组互斥总状态」）只删数字、保留引文；`Covers:` 行写成 `Covers: CC CONTEXT「……」`。引文选**该句最不会改的那半**（AGENTS.md：引文至少失配可见）。逐处对着 CONTEXT 当前文本核一遍——凡是原来指错行的（218 / 219 一族），以代码语义为准找到它真正要引的那句，不按今天的行号抄。
2. **跨包计数 → 点名**：「八格 / 十三格 / 四个 handler」改成「案件配置族的格」「协作与核对族的格」「凭证等各 handler」这类不带数的写法；`TestDutyReconciliationAnswerCoversEveryOutcome` / `TestConfigurationAnswerCoversEveryOutcome` 头注写「逐格对着 `application` 的枚举表」即可——用例本身列的表是它自己的断言，不动。
3. **admin-web 六处**同法（TS 注释与 `register-rows.test.ts` 的 `Covers:`）。
4. 一笔或按包分几笔都行；**不顺手改任何非注释行**。`t.Fatalf` 文本里的「硬句 219」属测试失败信息，一并换成引文。

## 红线

- 零行为改动：`git diff` 只许出现注释行、`Covers:` 行、`t.Fatalf` / 错误文本字符串行；生产代码字符串不动（错误文本里若有「硬句 N」也换，但那属信息文本不属行为）。
- 不改 `docs/domain/customs-compliance/CONTEXT.md`——本票不给 CONTEXT 加编号来「修」引用；行号引用的解法是不引行号。
- 不改口径：改口只换指法，不改注释所讲的规则内容；发现注释讲的规则与 CONTEXT 现文不符（不只是行号错），另记一条交 CC owner，不在本票里替它改。

## 完成判据

1. `git grep -n -E '硬句\s*[0-9]+' -- internal/customscompliance cmd/parcel-customs-register apps/admin-web/src/pages/customs` **零**。
2. `git grep -n -E '十三格|八格|四个 handler' -- cmd/parcel-customs-register` 零。
3. `git diff <基线> --stat` 只含上述目录；`git diff <基线> -- '*.go'` 的 `+`/`-` 行全部是 `//` 注释、`Covers:` 或字符串字面量（评审抽验）。
4. `gofmt -l` 空、`go build ./...` / `go vet ./...` 退 0；`go test -count=1` 动过的 Go 包（不带 DSN，编译过即可——真库用例 skip 属预期，票面如实记「真库未跑、无行为改动」）；admin-web `tsc --noEmit` 0、`node scripts/run-tests.mjs` 全 pass。
5. 清点：注释改动不改清点数字，`git status --porcelain -- docs/product/MECHANISM-INVENTORY.md` 为空即可。

## 地盘

`internal/customscompliance/**`（只注释）、`cmd/parcel-customs-register/**`（只注释与测试文本）、`apps/admin-web/src/pages/customs/**`（只注释）。**不动** `docs/**`、任何其他上下文。撞点见 Blocked by 软阻——开工前 `git fetch` 核 sa-cc/04 / 05 / 07 步二 已进 main。

## 参照

AGENTS.md「改文档」（行号与计数同构那段，含 `1dbc680` 抽验）与「写代码注释」；[sa-cc/10](10-cc-credential-collaboration-and-verification-read-faces.md) Comments 非作者评审 Standards ①；[sa-cc/07](07-cc-credential-and-duty-reconciliation-registration-faces.md) Comments 步一评审 Standards ①；同族票 [ve-disc/04](../../ve-disclosure-policy-view/issues/04-ve-comments-cite-context-line-numbers-and-count-methods.md)（VE 半边）、[lc/36](../../label-channel-service-first-release/issues/36-ps-label-final-review-follow-ups-header-comments-and-constructor-guards.md)（PS 头注与构造器项）。

## Comments

- 2026-09-11 14:2x · 通道 1 推送方：立票。**只写票面，未动代码。** 能力边界：数字来自 `git grep` 与 `Select-String` 实测（`0b027ab8`），行号错位核了 212 / 214 / 216 / 217 / 218 / 219 六行；没逐处读 76 处注释各自引的是哪句——实施时逐处核。
