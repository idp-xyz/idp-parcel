# 凭证登记与税费付款协作三方法有编排无入口：`RegisterCredential` / `FormCollaboration` / `ReceiveFundsFact` / `VerifyPayment` 在 `cmd/` 零调用方，租户既登不了凭证也交不进核对

Category: enhancement
Status: in-progress——2026-09-11 11:2x 通道 2 按通道 1 派单 task-e04c2d5a 认领；步一（CLI）在分支 `mcp2-sacc07-cli` 基远端 main `2c7326ef`，步二（端点 + 管理台写签）另开分支 `mcp2-sacc07-web`、等 sa-cc/14 进 main 后基新 main；此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）写入裁决：取 B（三册 CLI + 端点 + 管理台，同族一致）、`external-funds-fact` 人工口去掉（[ADR-0137](../../../docs/adr/0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md) 决定四）、读面另立 [10](10-cc-credential-collaboration-and-verification-read-faces.md)（见「要裁的」下「裁决」）。**步一 CLI 可开工**；步二端点 + 管理台等 10。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
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
