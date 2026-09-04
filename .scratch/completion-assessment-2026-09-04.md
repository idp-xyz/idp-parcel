# 项目完成情况客观评估（2026-09-04）

Category: chore
Status: 取证快照，不随后续提交改写

只读评估，代码改动只有随本文同批的一处 CI 超时（见「远端 CI」节）。评估框架取自[首发开发主线](../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)自身的判据：**产品就绪 = 八个 PN 切片的机制半边全部达标**（骨架完整、受控案例可复算、参数显式未配置），**试点就绪**属租户里程碑、不在开发方可达路线图内。上一份同形快照是 [2026-08-20](./completion-assessment-2026-08-20.md)（盘于 `c133e46`）；本文对着同一框架定位现状，并给出两端实测的差量。

## 取证基线

- 盘于 `dfd1725b`（远端 `main` 同值，22:3x `ls-remote` 核过），2026-09-04 22:4x。
- 共享树上有约 70 处 ` M`，`git diff --numstat` 对其中绝大多数为空（CRLF 幻影，成因见 `docs/agents/parallel-sessions.md`「一处会误导的信号」）；无未提交的 `.go`/`.sql` 真改动，因此下述测试证据准确反映已提交代码。
- 计数分两套口径，不混用：**机制半边清点**的数取自生成物 [`MECHANISM-INVENTORY.md`](../docs/product/MECHANISM-INVENTORY.md)（`ae7b4c8a` 在干净检出上重生成，CI 有门守它与代码一致）；**文件级**的数用 `git ls-files` 在 `dfd1725b` 上现数，与 08-20 那份同口径以便作差。

## 结论

八个 PN 切片自 2026-08-28 起全部「达标」（PN-02/03/06/07 带认可的显式留待），产品就绪里程碑已宣告。此后两轮审计把差量重新打开：09-02 裁定「零生产调用点的导出领域工厂」计入机制差量（当时 32 条），今天剩 9 条；端口精确口径缺 12，其中 7 口是已认可的留待实例证据。票务 224 张关掉 199。**作为可演示的软件产品已经成立；作为能承接一个真实租户的产品，差的不是代码，是租户、真渠道与若干项归用户的裁决。** 远端 CI 自 08-12 后没有一次 success，成因两段（账务停摆、超时），是本文唯一建议立刻动手的工程项。

## 硬证据（本轮实测，钉 `dfd1725b`）

| 项 | 结果 | 口径 |
|---|---|---|
| `gofmt -l` / `go build ./...` / `go vet ./...` | 零信号 | 隔离 detached 检出 `ae7b4c8a`（与 `dfd1725b` 只差一笔 `.md`） |
| `go test -p 1 -count=1 ./...` **含 PG 门禁** | **96 包全部 ok，0 FAIL**，7m58s | `postgres:16.14` 门禁容器 `127.0.0.1:55432`，DSN 已设；探针 `TestReferenceSeriesCanonicalizationDiffersIsAnswered` 带 DSN `PASS`、不带 `SKIP`；**本机未加 `-race`** |
| `apps/admin-web` | 仓内 `tsc --noEmit` 退 0；`run-tests.mjs` 66/66 | 同一检出，`node_modules` 走 junction |
| 生产 Go 文件 / 测试文件 | 835 / 808 | `internal`+`cmd`，按 `_test.go` 分（08-20 同口径：442 / 444） |
| 全仓 `.go` | 1665（测试 820） | `git ls-files '*.go'`，含 `tools/`、`scripts/` |
| SQL 迁移 | 11 模块 134 份 | 清点报告（08-20：75） |
| postgres 适配器生产文件 | 229（其中 Outbox 投递 51） | 清点报告（08-20：141） |
| 应用编排文件 | 106，分布在 11 个上下文 | 清点报告（08-20：59） |
| HTTP 适配器生产文件 / 接入面端点 | 101 / 96 | 清点报告 |
| 消费适配器 / 直投路由表 | 26 个生产文件 / 17 条 | 清点报告（08-20 路由 12 条） |
| 跨上下文消费缝 | 18 组 49 个生产文件 | 清点报告 |
| 端口 | 声明 336；基线口径缺 15；**精确口径缺 12** | 清点报告 |
| 生产接线棘轮基线 | **9 条**（PS 2、PC 3、PP 4） | `internal/architecture/production_wiring_baseline.txt` 非注释行；09-02 为 32、09-03 为 25 |
| ADR / UC | 111 篇 / `UC-*.md` 50 份（含 `UC-PS-001` 配套商业简报，与 08-20 同数） | `docs/adr/0*.md`、`docs/application/*/UC-*.md` |
| 管理台页面 | 登记 39 页全部 `live`（对真端点），另 1 页演示 | `apps/admin-web/src/page-registry.tsx` 的 `liveIds` / `demoIds` |
| 实施票 | **224 张，关 199（88.8%）**：resolved 195、done 2、superseded/handed-off 2；未关 25：draft 12、in-progress 6、ready-for-agent 1、blocked 2、needs-info 4 | `.scratch/*/issues/*.md` 的 `Status:` 行 |
| 父 spec | 20 份：resolved 13、in-progress 6、superseded 1 | `.scratch/*/spec.md` |

### 三条产品就绪判据对照

- **骨架完整**：满足，但量具有已知盲区。生产代码零 `TODO`/`panic` 的判据仍成立；09-02 裁决指出这三样证据对「从未开过端口的规则」在构造上全盲，于是把棘轮基线的零调用点工厂计入差量——今天剩 9 条。
- **受控案例可复算**：本机满足（含真库 96 包全绿）；**远端 CI 的持续保证三周不成立**，见下节。
- **参数显式未配置**：满足且被刻意维持。生产代码无业务阈值、金额、费率常量；今天进 main 的金额取整策略（ADR-0107）仍是「未声明→`AMOUNT_PRECISION_UNDECLARED`」的形状，不给默认位数。

## 远端 CI：自 2026-08-12 后无一次 success

`gh run list` 实查：最后一次 `success` 是 2026-08-12T20:16Z（`1a175a24`）。之后两段成因：

1. **08-20 → 08-31 账务停摆**：run 注解原文「The job was not started because recent account payments have failed or your spending limit needs to be increased」，基线文档 08-28 已记为运行事实加注。
2. **09-04 账务已恢复，job 能起，但被 `timeout-minutes: 10` 掐掉**：run 33883842149（`dfd1725b`）跑到 10m20s 报「exceeded the maximum execution time」。全仓 `go test -race` 带真库门禁已超 10 分（本机不带 `-race`、`-p 1` 就要将近 8 分）。同日更早几次 run 因 `cancel-in-progress` 被后续推送取消，没有一次跑完。

处置：随本文同批把 `timeout-minutes` 提到 30（`.github/workflows/ci.yml`，注释写明依据）。上限只防挂死；再超就该拆 job。**在它变绿之前，「CI 守着」这句在文档里成立、在远端不成立**，可复算证据以各轮本机真库实跑为准。

## 快照后增量（`c133e46` → `dfd1725b`，十五天）

同口径两端实测：生产 Go 文件 442→835、测试 444→808；迁移 75→134；postgres 适配器 141→229；编排 59→106；ADR 67→111；路由表 12→17；实施票从「三票在办 + 十票待分诊」到 224 张关 199。

内容上这十五天做的是：08-28 宣告产品就绪；09-02 把棘轮 32 条判入差量并逐条取证，随后 `tf-unwired-seven` 八票、`tf-segment-lifecycle-closure` 十票（08 今日入 main）、`label-channel-service-first-release` 二十四票（`01`–`19`、`21`、`23` resolved）、管理台写面 26 个登记口接进端点表、`collection-remittance` 首次有生产代码、`frontline-transition-import` 受控导入 CLI（ADR-0089，硬期限 2026-12-31）、计价参考序列运营形态（pricing 01–04、07–10）与今天的规范化换号 PPC-4→PPC-5。ADR-0088 把尾程面单渠道转售判进首发服务形态，是这期间唯一改产品范围的决定。

## 剩余机制缺口（真缺口，非实例墙）

1. **端口精确口径缺 12**（清点报告原名单）：
   - 认可留待实例证据 7 口：SA `BuyEvaluationView`、`ClaimAmountRuleView`、`ConfirmedChargeFactsView`、`ContractResponsibilityView`、`SupplierAuditAuthorityView`、`SupplierPayableAccountView`（登记面形状等实例证据）；VE `NotificationChannelGateway`（等真实渠道凭证）。
   - 等第一家真源 2 口：TF `TrackingSource`（虚高：名字出现过无人实现，随 label-channel/20 落）；PS `LabelChannelGateway`（合成替身已有，真渠道适配器等渠道）。
   - 建模未决 3 口：PS `LabelValidityRuleView`、`SourceDataAmendmentAuthorizer`、`SourceDataRuleDeclaration`（`BD-PS-009` / `PAR-COM-13`）。
2. **棘轮基线 9 条**零生产调用点的导出工厂：PS `AssessSafeHandoff`、`CurrentPayloadCanonicalizationVersion`；PC `ResolveCreditPolicy`、`ManualReviewRequirementFor`、`ValidateBeforeDecision`；PP `ReplayPricingEvaluation`、`MarshalPricingPlanSnapshot`、`RehydratePricingPlanSnapshot`、`ParseCanonical`。
3. **未关票 25 张**，按性质分五类，逐张归一处不重数（明细与派工见 [`tasks.md`](./tasks.md) 21:5x 节）：
   - 可直接实施 6：pricing/06（MCP-5 在收口）、shape-gaps/01–03 票级与 amount-precision/02 余项（MCP-3 在做）、pricing/05 05b。
   - 先建模或裁再动 10（多半各要一篇 ADR）：tf/09、tf/10、tf-carrier-master-document-register/01、pc-gaps/07、admin-write-faces/07 伞票、label-channel/24、pilot-governance/01、auto-reroute-demo-reachability/01、first-tenant-runway/10、ve-claims/04（其中三问归用户，见下）。
   - 等第一家真源 2：label-channel/20、22。
   - 阻塞于另一票 2：admin-write-faces/06（等 pc-gaps/07 正文表）、frontline-transition-import/01 余格（`RECEIVED` 行身份核对缝，等 PS 侧）。
   - 归用户 5：needs-info 四张（ps-external-mark-relations/01、ve-008/04、nr-route-evidence-views/01、syn-wall-door-audit/01）与 first-tenant-runway/03（`PAR-NET-14` 实例值）。票数之外还有两件同样归用户：tenant-implementation-01 三份仓外动作（不是实施票）、ve-claims/04 交用户裁的三件（起算事实源与业务日历归谁、Notice 来源、票 04 四问）。
4. **裁决债务**：ADR-0101–0111 十一篇是近两日 agent 受托代裁（「owner 授权自决」口径），其中 ADR-0109（跨上下文归属）与 ADR-0111（改 PP CONTEXT 硬句）两处标「需用户复核」，不认可走 supersede 不改历史。上面第 3 类十张票会再产一批。**这些是「替用户先裁一版」，不是「完成」**。
5. **工程残局**：`git worktree list` 里 6 棵别人的树待核拆（`idp-parcel-mcp2-tf02`、`idp-parcel-mcp4-bk`、`idp-parcel-mcp6-fti`、`idp-parcel-mcp6-d4ps` 内容已入 main，`D:/tops/idp-ppclean` 分支与 main 零差；`D:/tops/idp-tf03` 在 TEMP 之外、留给用户定）；今日会话崩溃四轮，靠「每小步立刻提交」与分支封存救回。

## 实例半边墙（按设计如实存在，不计缺口）

尚无租户。参数登记册所有实例项待提供：Intake 认证（`PAR-INT-01`）、网络定义与规则值（`PAR-NET-14`）、商业参数（`PAR-COM-13/14/15`）、真实渠道凭证、汇率出网能力。纵向闭环停在 `ROUTING_APPLICABILITY_UNAVAILABLE` → `ROUTE_EVIDENCE_NOT_CONFIGURED` 一类哨兵上，全部拒绝默认值；仓内禁出网代码（pricing/06 的 CFETS 段因此留 draft）。作业端（一线扫描）不在开发主线，过渡走受控导入 CLI，硬期限内置。**接通不等于能干活**——这正是基线要求的诚实形态。

## 总评

机制半边完成度以「产品就绪已宣告 + 审计重开的差量」计约九成以上：12 口端口缺里 7 口是认可留待、2 口等真源、3 口建模未决；棘轮 9 条；十张建模票是模型层缺口不是骨架缺口。质量证据扎实：含真库 96 包全绿、零假实现、生产与测试文件约 1:1、无业务常量、管理台 39 页全部对真端点。**距开发方可达的最高点，剩的是有限且可枚举的机制工作量**——但其中一半要先经过用户的裁决。

三处该马上做的：① 让远端 CI 重新变绿（本批已改超时，下一次推送见分晓）；② 复核 ADR-0109/0111 两处越权点与其后要出的 ADR 节奏；③ 回答上面「归用户」那一格——五张票加两件票外事项，它们任何通道都完成不了。

## 本文不包含

- 不重跑 `-race`（本机 Windows 侧无 gcc），CI 绿后以它为准。
- 不评估 `docs/archive/` 与关务专项工作单的历史覆盖。
- 不裁断任何在办票；去留以各票与用户裁定为准。
