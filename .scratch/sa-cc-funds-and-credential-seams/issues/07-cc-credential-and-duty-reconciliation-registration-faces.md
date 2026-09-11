# 凭证登记与税费付款协作三方法有编排无入口：`RegisterCredential` / `FormCollaboration` / `ReceiveFundsFact` / `VerifyPayment` 在 `cmd/` 零调用方，租户既登不了凭证也交不进核对

Category: enhancement
Status: in-progress——**步二开工，2026-09-11 14:1x**（通道 2 按通道 1 派单 task-6a228ee9 接手；分支 `mcp2-sacc07-web` 已从 `a1b19433` 快进到 `51ca1270` = 远端 main，步一的 `registrationjson` 三册译装因此可直接复用；树上那份未提交的 `adapters/http/register_credential_and_duty_test.go` red 测试读过后**沿用**——它对三端点的断言与本包 `register_configuration.go` 的转写口径及 ADR-0022 一致，缺的只是实现，重写只会再抄一遍同样的断言）；**步一（CLI）已进 main，2026-09-11 13:4x**（分支 `mcp2-sacc07-cli` 代码 tip `40f3d6d3`；main 重放 tip `3ec86aaa`、清点 `fa386a29`；非作者评审 ← 通道 1 推送方自跑两轴 0 阻断，见 Comments）；**步二（端点 + 管理台写签）在途**：分支 `mcp2-sacc07-web` 基 `a1b19433`，通道 2 在做（13:3x 树上只有一份未提交的 `adapters/http` red 测试），步二若复用步一的 `registrationjson` 译装先 rebase 到含 `3ec86aaa` 的 main；此前 2026-09-11 11:2x 通道 2 按通道 1 派单 task-e04c2d5a 认领；步一（CLI）在分支 `mcp2-sacc07-cli` 基远端 main `2c7326ef`，步二（端点 + 管理台写签）另开分支 `mcp2-sacc07-web`、等 sa-cc/14 进 main 后基新 main；此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）写入裁决：取 B（三册 CLI + 端点 + 管理台，同族一致）、`external-funds-fact` 人工口去掉（[ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定四）、读面另立 [10](10-cc-credential-collaboration-and-verification-read-faces.md)（见「要裁的」下「裁决」）。**步一 CLI 可开工**；步二端点 + 管理台等 10。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 步二 Blocked by [10](10-cc-credential-collaboration-and-verification-read-faces.md)（写签跟着读签走）——**10 已进 main（2026-09-11 10:5x，重放 tip `99ceb975`，见票 10「进 main 记录」）**，步二不再被它阻；步二的三个写签各挂 10 落的读签旁（凭证 → `CustomsCasesPage` 凭证签；协作 / 核对 → `CustomsRestrictionsPage` 两签）。步一不阻。资金事实那一口由 [02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md) + [03](03-cc-inbox-consumer-receives-external-funds-fact.md) 的信封驱动，本票不再含它

## 缺口（取证于 `3f485e97`）

- `git grep -E 'RegisterCredential|DutyPaymentReconciliation|ReceiveFundsFact|VerifyPayment|FormCollaboration' -- cmd/` **零**。
- `cmd/parcel-api/endpoints.go` 关务只有：`/customs/external-results`、`/customs-interpretation-rule-registrations`、`/customs-gate-catalog-registrations`、`/customs-candidate-port-registrations`、`/customs-declaration-path-registrations`、`/customs-case-requirement-registrations` 与四个查阅口。
- `cmd/parcel-customs-register` 存在，头注「八本册子十二个命令」（就绪判断 / 提交授权各带撤销、解释规则、关闭义务与门禁前置条件各目录 + 明细、建案要求规则、口岸目录、申报路径目录），子命令式而非 `-kind`；凭证、协作、资金事实、付款核对四者不在其中（`git show HEAD:cmd/parcel-customs-register/main.go | Select-String -Pattern 'credential|duty'` 零）。
- mech/07「没做」第 5 条：「三组的在线登记面/端点与 admin 写面」。

## 语言从哪里来

- CC `CONTEXT.md`：本上下文拥有「监管凭证及其适用性和使用关系……监管核定税费、税费付款协作事项、税费付款核对、放行门禁核对」。
- ADR-0085 决定一：「CLI 不退场——CLI 与端点消费同一登记用例，是同一能力的受控批量口与在线口」；决定四：其余上下文的取舍「登记频次 × 操作者角色」由实施票逐册裁。
- awf/05 那张票的 owner 裁决记录：「判据以 ADR-0085 决定四为准，不以票 01 裁决二那句『有 CLI 先例才进端点表』为准」。

## 做法（一张两步，照 awf/05 的形；步二按裁决取舍）

**步一 · CLI**（A、B 共有）：`cmd/parcel-customs-register` 加三个子命令——`regulatory-credential`（→ `RegisterCredential`）、`duty-collaboration`（→ `FormCollaboration`）、`duty-payment-verification`（→ `VerifyPayment`）；~~`external-funds-fact`（→ `ReceiveFundsFact`，运维补录口）~~ 按 ADR-0137 决定四去掉（资金事实进 CC 只经 SA 采用信封，见「裁决」2）；输入沿 `-input` JSON，未知字段拒，退出码沿既有格；命令族列在一处（照 `parcel-network-register` 的 `supportedKinds` 教训：用法文本与未知命令的错误文本不各抄一遍）。

**步二 · 在线登记端点 + 管理台**（仅 B；裁 B 后必做，等 [10](10-cc-credential-collaboration-and-verification-read-faces.md) 的读面）：`internal/customscompliance/adapters/http` 增三个 `*Registrar` 接口 + 处理器 + 封闭响应形（ADR-0022），装配以字面量 `UnconfiguredIntake{}` 起步，装配行进 `cmd/parcel-api/endpoints.go`（共享接线文件，动前占号）；管理台 `pages/customs/` 加登记签——**写签跟着读签走**：凭证、协作、核对三册今天有没有读面先核（`customs-case-registers` / `customs-gate-conditions` / `customs-ports-paths` 三个查阅口不覆盖它们），没有读面的册先不铺写签，另记一条。

## 红线

- 只建入口，不写任何真实凭证 / 程序 / 付款条件（`PAR-CUS-01..07` 待提供）；合成值只记 `S`。
- 不新增覆盖语义：四个编排各自的答案代数（`已存在` / `内容冲突` / 未决各格）原样转写成退出码与状态码，不在入口层重判。
- 端点若做，写准入不另立形（ADR-0085 决定二）；隔离 demo 里如实答未配置。
- 三轴与关联依据（`VerifyPayment` 的入参）由登记方交进来——真实程序的关联规则属实例半边，入口不代判。

## 完成判据

1. CLI 单测：三个子命令各一条绿路径 + 治理两格（重放 / 冲突）+ 受理门拒绝 + 依赖故障退出码；不含真库（写口的真库用例在 `adapters/postgres` 已有，本票零改动那一层）。
2. 步二（已裁 B）：http 单测只收 POST、未配置 403、三态响应；`cmd/parcel-api` 装配用例真库一正一反；`internal/architecture` 管理台路径门禁绿；管理台写签挂在 10 落的读签旁。
3. `git grep -E 'RegisterCredential|FormCollaboration|VerifyPayment' -- cmd/` 各至少一处非测试命中；`ReceiveFundsFact` 的非测试调用点归 [03](03-cc-inbox-consumer-receives-external-funds-fact.md)（dispatch 路由），不在本票。
4. 清点 tip 重生成（接入面端点数如裁）。

## 地盘

`cmd/parcel-customs-register/`（步一）；`internal/customscompliance/adapters/http/`、`cmd/parcel-api/endpoints.go` 一段、`cmd/parcel-api/assemble_customs*.go`、`apps/admin-web/src/pages/customs/`（步二）。不动 `internal/customscompliance/application/**`。

## 要裁的

1. **A 只补 CLI / B CLI + 端点 + 管理台**：按 ADR-0085 决定四「登记频次 × 操作者角色」——凭证是低频·合规角色，协作 / 核对是逐申报范围的高频·作业角色；两组可能裁法不同，本票允许拆成「凭证 A、协作核对 B」。归 CC owner。
2. **`external-funds-fact` 要不要开人工入口**：[02](02-sa-external-funds-fact-adoption-hands-off-an-envelope.md)+[03](03-cc-inbox-consumer-receives-external-funds-fact.md) 落地后它是信封驱动；人工补录口保留还是去掉——保留就得答「补录的事实与 SA 采用的事实撞键怎么办」（编排今天答 `已存在` / `内容冲突`，够不够）。归 CC owner。
3. **读面**：凭证 / 协作 / 核对三册今天无查阅端点；写签跟读签走，则读面是本票步二的前置还是另立票。归 CC owner。

### 裁决

（通道 5 写入，2026-09-10 17:2x；task-b941ce87 分类：1 为 C（拿不准，或 B）、2 为 B、3 为 A；task-9a2ff746 落笔。1 由用户 17:0x 授权按「机制半边现在做」口径定，越权点 CC owner 复核；2 落 [ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定四，越权点 SA owner 复核；3 由通道 1 推送方裁。）

- **1 → B：三册都做 CLI + 端点 + 管理台，口径「同族一致」；步一 CLI 先行。** ADR-0085 决定四的判据「登记频次 × 操作者角色」是租户运营事实，今天无租户只能是产品假设；按用户授权的读法「C 类中的产品流程按『机制半边现在做』口径定」，机制半边把入口做齐、不替租户猜哪册低频（同 ve-disc/02-1 的裁法）。凭证与协作 / 核对不分开裁——分开等于用假设的频次裁掉一册的在线口。越权点：CC owner 复核「三册同族一致」是否与凭证册的合规角色口径相容。
- **2 → 去掉 `external-funds-fact` 人工补录口**（ADR-0137 决定四）。SA CONTEXT 与 CC CONTEXT 同句「银行、支付或财务系统拥有实际付款……外部资金事实更正」、采用在 SA（UC-SA-001「同一外部资金事实通过回调、文件或人工核实重复到达，只能被采用一次」已把人工核实算进 SA 那一口）；CC 另开人工入口是第二铸造点，「撞键怎么办」就是两个铸造点的冲突本身。人工更正登在 SA，再经 02 的信封到 03 的消费者。**只定口径，02 / 03 进 main 前不动代码**；CLI 子命令由此为三个。越权点：SA owner 复核「单一采用口」（ADR-0137 越权风险点 6）。
- **3 → 读面另立 [10](10-cc-credential-collaboration-and-verification-read-faces.md)「凭证 / 协作 / 核对三册读面」；本票步二 Blocked by 10，步一不阻。** 伞票 awf/07「写签跟着读签走」口径已定，剩下只是拆票顺序；awf/06 / 21 读面另立的先例可照。

## 参照

[awf/05](../../admin-write-faces/issues/05-auto-reroute-facts-has-no-registration-entry.md)（一张两步、A/B 裁法、判据出处）；ADR-0085；[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md)「没做」第 5 条；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-8 ⑤；`cc-case-requirement-rule-registry`（CC 最近一次登记面从 CLI 到端点的先例，已 resolved）。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
- 2026-09-11 11:4x–11:57 · 通道 2（task-e04c2d5a）· **步一完工**（完工报经队列到通道 1，票面未写完成记录，此条由推送方按分支内容代记）：分支 `mcp2-sacc07-cli` 三笔——`c4e4c70c` 票面 in-progress；`5621b0b5` `internal/customscompliance/adapters/registrationjson/translate.go` 加 `RegulatoryCredentialFromJSON` / `DutyCollaborationFromJSON` / `DutyPaymentVerificationFromJSON` 与 `dutyObligationKindFrom` / `dutyCoverageFrom` / `dutyDeltaFrom` / `dutyFactValidityFrom` 四个封闭词表解析器；`40f3d6d3` `cmd/parcel-customs-register/` 三个子命令 `regulatory-credential` / `duty-collaboration` / `duty-payment-verification`（`translate.go` `allCommands` 一处列全、`answer` 接口让两族用例各自的结果代数各成一型；`main.go` `buildRegistrar` 装 `NewRegisterCredentialHandler` 与 `NewDutyPaymentReconciliationHandler`（接 sa-cc/14 新签名的错误）、`systemClock`、`dutyReconciliationAnswer` 按恢复动作归四格；新 `duty_registers_test.go`）。作者报：分支 rebase 到 `a1b19433`（含 14）后带 DSN `cmd/parcel-customs-register` 35 PASS / 0 SKIP。
- **评审 ← 通道 1 推送方自跑 · 钉 `40f3d6d3` · 基线 `a1b19433` · 2026-09-11 13:4x**（派给通道 6 的 task-8543732a 12:05 起 pending 未认领、12:30 逾时，按「到时未交，推送方改派或自跑」自跑；隔离子代理再次鉴权错，两轴由通道 1 会话直读 diff；通道 1 非作者。评审侧实测在重放树：`gofmt -l` 空、`go build` / `go vet` 0、带 DSN 全量见「进 main 记录」）。
  - **Standards**：**阻断 无**。**非阻断 1**：① 跨包计数——`translate.go` `answer` 头注与 `configurationCall` 头注（「案件配置族的八格、税费付款协作与核对族的十三格」「凭证四个 handler」）、`main.go` `dutyReconciliationAnswer` 头注（「封闭十三格」）、`duty_registers_test.go` 文件头注与 `TestDutyReconciliationAnswerCoversEveryOutcome` 头注（「十三格 / 八格」）数的是 `application` 包 `DutyReconciliationOutcome` / `CaseConfigurationOutcome` 的常量个数，AGENTS.md「写代码注释」明令不数别处的东西；「八格」是本包旧例（`configurationAnswer` 头注、`main_test.go`）沿用，新增的是「十三格」。与 lc/34「六个」/ lc/27「三路 / 六口」/ sa-cc/14「四口」同族，归 CC owner 一笔改口，不挡合入。**无发现（核过）**：译装层错误文本「封闭二值 / 三值 / 四值」数的是同一处 `switch` 自己的 case，本包内不算；`kind` 缺席放行到领域、由编排答 `DUTY_OBLIGATION_BASIS_ABSENT`，是「不新增覆盖语义」红线的正读，头注讲清了为什么不在译装拒；打错的词在译装指名拒；`decodeStrict` 拒未知字段；用法文本与未知命令错误文本同出 `allCommands`；合成值全 `SYN-*`，无真实凭证 / 程序 / 付款条件；注释全中文，无行号引用（`0016 自注` 引的是迁移文件号，`UC-CC-009 步 4` 是用例自身编号）；`main.go` 头注把旧「八本册子十二个命令」的计数删了、改指 `allCommands`。
  - **Spec**：**阻断 无**。**非阻断 1**：① 核对子命令没有一条经 `execute` 到编排 `DutyReconciliationNotAccepted` 的用例——三轴集外与引用空白都在译装层拒成用法错（`translate_test.go` 空 `fundsRef`、`coverage=ALL`、`delta=MISSING`、`validity` 空四例），编排层的 NOT_ACCEPTED 从 CLI 可能触发不到；映射本身由 `TestDutyReconciliationAnswerCoversEveryOutcome` 逐格钉住（十三值 + Invalid → 3），不挡。**无发现（核过）**：判据 1——凭证：绿路径（七件原样到册、`uses` 缺席落「来源未提供」）/ 重放 0 `EXISTING` / 换有效期 2 `CONTENT_CONFLICT` 且册面不动 / 有效期倒置 1 `NOT_ACCEPTED` / 写口故障 3；协作：核定格与无需付款格（空税费引用落键）两条绿 / 重放 / 换目标 2 / 矛盾输入 1 / 义务依据缺席 3 `DUTY_OBLIGATION_BASIS_ABSENT` / 写口故障 3 指名；核对：两道前置齐 0 且三轴、依据、三维键原样、核对时间取时钟 / 重放 0 / 换轴不是冲突是新版本追加两版并存（该族无「内容冲突」格，本口不造——正是红线「原样转写」）/ 无依据 1 `FUNDS_FACT_PENDING_ASSOCIATION` / 资金事实未接收 3 / 协作未形成 3 / 写口故障 3 指名；全部替身册，不含真库。判据 3——`git grep -E 'RegisterCredential|FormCollaboration|VerifyPayment' -- cmd/ ':!*_test.go'`：`main.go` `NewRegisterCredentialHandler` / `translate.go` `FormCollaboration` / `VerifyPayment` 各命中；`ReceiveFundsFact` / `external-funds-fact` 在 `cmd/parcel-customs-register` 零命中（只在 `cmd/parcel-dispatch` 03 那处）。范围——diff 只 6 件，`application/**` 与 postgres 写口零改动；`buildRegistrar` 除接 14 新签名外新增的是本票步一本来要装的两个 handler 与三个适配器，不是票面之外的行为；资金事实三格留在 `dutyReconciliationAnswer` 表上但无命令能交回，头注写明理由（一族一张表）。
  - **汇总**：Standards 0 阻断 / 1 非阻断；Spec 0 阻断 / 1 非阻断。可合入。
- **进 main 记录（步一；通道 1 推送方，2026-09-11 13:4x）**：`%TEMP%\idp-replay-sacc07` detached `2dcffbb3`（= 远端 main），`cherry-pick a1b19433..mcp2-sacc07-cli` 三笔零冲突（分支动的 6 件与 main 自 `a1b19433` 起的 17 笔零文件重叠，按「无重叠才直接重放」不劳作者重验）；本票文件对分支 tip 零差。SHA 对照：`c4e4c70c→a5c6523f` / `5621b0b5→138edb11` / `40f3d6d3→3ec86aaa`；清点在 tip 重生成 → **`fa386a29`**（`cmd/` 测试 82→83，其余不变）。**验证钉 `fa386a29`**：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；13:31 占号，带 DSN `go test -p 1 -count=1 ./...` **107 ok / 0 FAIL / 15 无测试 / 0 cached**（13:31:33→13:33:41，128 s）；`-v` 探针 `cmd/parcel-customs-register` + `registrationjson` PASS 35 / SKIP 0、CC `adapters/postgres -run 'Credential|DutyReconciliation|DutyPayment|Collaboration'` PASS 13 / SKIP 0；13:3x 释号。簿记一笔在其上（本票 Status + 步一完工代记 + 评审 + 本条、sa-cc spec 07 行、tasks.md），纯 .md 自审；`ls-remote` 核 `2dcffbb3` 未动 → `merge --ff-only` → `push <sha>:main`。分支 `mcp2-sacc07-cli@40f3d6d3` 作封存出处、改名 `merged/`、远端删；`D:/tops/idp-parcel-mcp2-sacc07-cli` 比内容后拆。**推送方对评审的处置**：两轴 0 阻断；两条非阻断随票记——Standards ① 归 CC owner 与 sa-cc/10 三处行号引用合一笔改口，Spec ① 不立票（映射已钉）。**步二**（通道 2，`mcp2-sacc07-web`）不在本条：树上 13:3x 只有一份未提交的 `internal/customscompliance/adapters/http/register_credential_and_duty_test.go`（red，引用尚不存在的 `NewRegisterRegulatoryCredentialEndpoint`），零提交、远端无此分支——不拆、不动；复用 `registrationjson` 的话先 rebase 到含 `3ec86aaa` 的 main。
