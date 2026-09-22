# 项目完成情况审查快照（2026-09-22）

状态：取证快照，钉 `e0d3f89d`（= 远端 `main`，2026-09-21 12:29），不随后续提交改写

只读审查，未改仓库任何源码或文档（本文与 [docs/README.md](../README.md) 的一条入口除外）。评估框架沿用[首发开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)自身判据：**产品就绪 = 八个 PN 切片的机制半边全部达标**（骨架完整、受控案例可复算、参数显式未配置），**试点就绪**属租户里程碑、不在开发方可达路线图内。上一份同形快照是 `.scratch/completion-assessment-2026-09-04.md`（盘于 `dfd1725b`；`.scratch` 按用户 09-22 裁定将删，该文件若不再在，差量以本文括号内的 09-04 值为准）。

可由代码算出的数一律以[机制半边清点](../product/MECHANISM-INVENTORY.md)在本 SHA 上的那份为准；本文只把它们与文件级现数并排以便作差。**本文里的每一个数都锚在 `e0d3f89d`，只作此刻取证，不作别人的基准。**

## 取证基线

- 代码与文档取证于 `e0d3f89d`；审查期间另一会话（通道 3）在共享树上提交了 `44e8a355`（docs-only：项目总览 HTML 自 `.scratch/` 迁入 `docs/overview/`、README 登记一节），未推。本文所有代码数字不含它，文档数字也不含它。
- 共享树上 74 处 ` M`，其中 72 处 `git diff --numstat` 为空——CRLF 幻影（含 30 份 `.go`、3 份 `.sql`），成因见 `docs/agents/parallel-sessions.md`「一处会误导的信号」。真改动只有两处未提交：`.cursor/rules/idp-mcp-messenger.mdc`（删「点名」一节）与 `docs/adr/README.md`（加 ADR-0143 入口）；未跟踪两件：`docs/adr/0143-*.md`（Proposed，2026-09-21）与 `docs/ux/commercial-party.md`（1853 行外来 UX 文档，归属未定）。
- 本次实测在一台此前无 Go / Node 的 Linux 宿主上完成：Go 1.26.5（与 `go.mod` toolchain 同版）装于 `~/.local/go`，Node 22.23.2 + pnpm 10.32.1 装于 `~/.local/node`；门禁库按 `compose.yaml` 起 `postgres:16.14`，跑完 `docker compose down -v`。工作副本与干净检出（detached worktree @ `e0d3f89d`）各跑一遍，两边不同的地方逐条写明。

## 结论

八个 PN 切片自 2026-08-28 起全部「达标」（PN-02/03/06/07 带认可的显式留待），产品就绪里程碑成立。09-02 审计重开的差量此后基本收尽：生产接线棘轮 32 → 25 → 9 → **1**；端口精确口径缺 12 → **7**，且 7 口全部有归类。远端 CI 从 09-04 的「08-12 后零 success」恢复到 **`main` 最近 25 个 push run 全 success**。票务 374 张关 358。**作为可演示、可进入商务洽谈的软件产品，机制半边已经成立并被加固；作为能承接一个真实租户的产品，差的仍不是代码，是租户、真渠道与一批归用户的裁决——而卡住「演示走到价值」的那一条，也是裁决。**

## 硬证据（本轮实测）

| 项 | 结果 | 口径 |
|---|---|---|
| `go build ./...` / `go vet ./...` | 0 / 0 | 共享树，Go 1.26.5 |
| `gofmt -l .` | 工作副本 30 文件；**干净检出 0** | 30 个全部是 CRLF 幻影：提交 blob 经 `gofmt -d` 零差 |
| 清点工具 `go vet` / `go test` + 重生成 `MECHANISM-INVENTORY.md` | ok；`git status --porcelain` 零行 | 与 CI「Mechanism inventory is current」同口径 |
| `scripts/ci/test-shards.sh check` | 131 包，4 片无重叠、无遗漏、无空片 | — |
| `go test -race -count=1 -p 1 ./...` 带 `IDP_PARCEL_POSTGRES_DSN` | **114 ok + 1 FAIL；干净检出补跑该包 ok → 115/115**，0 `DATA RACE`，5m43s | 唯一 FAIL 是 `migrations` 的 `TestEmbeddedMigrationAssetsCarryNoCarriageReturnOrBOM`：工作副本三份 `.sql` 带 CRLF（`parcel_shipment/0008`、`party_commercial/0015`、`0016`），提交 blob 零 `\r`；它守的正是这一格 |
| `apps/admin-web`（干净检出） | `pnpm install --frozen-lockfile` ok；`tsc -b --noEmit` 0；`run-tests.mjs` **368 pass / 0 fail**；`vite build` ok | 三道门与 CI `admin-web` job 同口径 |
| 远端 CI | `main` 最近 25 个 push run **25/25 success**（含 `e0d3f89d` run #1019）；累计 1008 run | GitHub Actions API 实查 |

### 三条产品就绪判据对照

- **骨架完整**：满足。生产代码零 `TODO`/`FIXME`/`panic(` 的判据仍成立；09-02 裁决点名的量具盲区（零调用点的导出工厂）已从 32 条收到 1 条。
- **受控案例可复算**：满足，且远端与本机双重成立——09-04 那份写「CI 守着这句在远端不成立」，本轮 25/25 绿，本机 `-race` + 真库全绿。
- **参数显式未配置**：满足且被刻意维持。参数登记册 65 行全部 `待提供`；直接依赖仍是三个；生产代码无业务阈值、金额、费率常量。

## 规模与差量（`git ls-files` @ `e0d3f89d`；括号内为 09-04 @ `dfd1725b` 同口径值）

- 生产 Go 1063（835）/ 测试 Go 1039（808），`internal` + `cmd` 按 `_test.go` 分；全仓 `.go` 2124
- SQL 迁移 174（134），11 模块；ADR 142（111）；`UC-*.md` 50（50，含 `UC-PS-001` 配套商业简报）；`CONTEXT.md` 10
- 清点报告：业务上下文 12；应用编排 123（106）；postgres 适配器 269（229，其中 Outbox 投递 57）；http 适配器文件 134；接入面端点 126（96）；消费适配器 33（26）；直投路由 24（17）；跨上下文消费缝 25 组 74 文件（18 组 49）
- 端口声明 413（336）；基线口径缺 14（15）；**精确口径缺 7（12）**
- 生产接线棘轮基线 **1**（9；09-02 为 32、09-03 为 25）
- 管理台 `liveIds` **42** 页 live（39）+ 1 页 demo；`apps/admin-web/src` 246 份 ts/tsx；测试 368（66）
- 实施票 **374 张，关 358（95.7%）**：resolved 347、已进 main 8、done 2、superseded 1；未关 16：draft 8、needs-info 6、blocked 2（09-04：224 张关 199 = 88.8%，未关 25）
- 父 spec 28：resolved 19、in-progress 7、draft 1、superseded 1

## 剩余机制缺口（真缺口，非实例墙）

1. **端口精确口径缺 7**（清点报告原名单）：
   - 认可留待实例证据 4 口：SA `ClaimAmountRuleView`、`ConfirmedChargeFactsView`、`SupplierAuditAuthorityView`、`SupplierPayableAccountView`（登记面形状）。
   - 等第一家真源 2 口：TF `TrackingSource`（虚高：名字出现过无人实现；`label-channel-service-first-release/20` draft）；VE `NotificationChannelGateway`（等真实渠道凭证；[ADR-0142](../adr/0142-first-party-webhooks-are-a-product-owned-outbound-channel.md) 草案把首方 webhook 定为它在机器接收方上的第一份生产实现）。
   - 在办 1 口：PP `PricingInputResolver`（09-04 之后新开的缝，`pp-pricing-input-seams` spec in-progress、票 04 draft）。
2. **棘轮基线 1 条**：PS `ParseDeliveryPlaceReference` 零生产调用点。
3. **未关票 16 张**：
   - draft 8：`admin-web-workspace-form/01`（多标签壳，判断项归用户）、`/02`（右侧检查器，壳层段 Blocked by 01）；`first-party-api/01–03`（全部 Blocked by ADR-0139 接受）；`label-channel-service-first-release/20`、`/22`（等第一家真源）；`pp-pricing-input-seams/04`。
   - needs-info 6：全部归用户，见下节。
   - blocked 2：`auto-reroute-demo-reachability/02` 与 `first-tenant-runway/03`——**同一根因**：网络解析层不在，唯一的证据视图实现对已登记范围上抛 `ErrNetworkDefinitionUnresolvable`，初始路由停在 `ROUTE_EVIDENCE_UNAVAILABLE`，没有计划就没有复核、更没有受控改路。

## 归用户的决定（任何通道都完成不了）

- **7 篇 Proposed ADR**：[0139](../adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 首方客户 API 渠道族、[0140](../adr/0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 契约生成与 `/v1`、[0141](../adr/0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md) 集成方沙箱、[0142](../adr/0142-first-party-webhooks-are-a-product-owned-outbound-channel.md) 首方 webhook——这四篇是用户 09-16「软件公司不该等真实客户才做 internal/public API」那句的产物，接受后 `first-party-api` 三票才激活；0143 对账单号走平台编号（09-21，文件未跟踪）；[0071](../adr/0071-catalogue-views-carry-tenant-in-the-method-signature.md)；`label-channel/39` 产出的 PS 终局重派生 ADR（归 PS owner）。
- **6 篇已接受 ADR 的越权风险点无 owner 复核记录**：0128–0133；另 `sa-preacceptance-policy-view/02` 票内一组、`agent-docs-local-env/02` 一案。清单与摘录见 `scripts/owner-review-queue.ps1` 的产物。
- **6 张 needs-info**：`nr-route-evidence-views/01`（把 `PAR-NET-14` 的机制半边从其规则取值切开——**这是让合成演示动线越过第五步「委托」走到「路由选出」的钥匙**，上节 blocked 两票都等它）；`syn-wall-door-audit/01`（接入渠道册与第一个真 Intake）；`ps-external-mark-relations/01`（外部标识关系子域零模型）；`ve-008-late-account-rederive/04`（运维回放端点等 `PAR-INT-01`）；`admin-web-group-legal-entities/04`（目录读口分页/排序/筛选契约，一决策一处定义要先出 ADR）；`/05`（法人业务属性，改的是 PC 领域语言，要先改 CONTEXT）。
- `admin-web-workspace-form` 判断项 1–3（多标签 / 分栏 / `ActivityBar`）；`docs/ux/commercial-party.md` 的归属；收费模型采哪几层。

## 实例半边墙（按设计如实存在，不计缺口）

尚无租户。[参数登记册](../product/PILOT-PARAMETER-REGISTER.md) 65 行全部 `待提供`：Intake 认证（`PAR-INT-01`）、网络定义与规则值（`PAR-NET-14`）、商业参数（`PAR-COM-13/14/15`）、真实渠道凭证、汇率出网能力。首个租户候选（08-31）的实施材料四件已备在 `tenant-implementation-01`，检查单 A/C/D/F/G 组是用户→客户的动作。纵向闭环仍停在 `ROUTING_APPLICABILITY_UNAVAILABLE` → `ROUTE_EVIDENCE_NOT_CONFIGURED` 一类哨兵上，全部拒绝默认值。**接通不等于能干活**——这正是基线要求的诚实形态。

## 工程卫生

- **共享树 72 处 CRLF 幻影**在 Linux 宿主上会让 `gofmt -l` 与 `migrations` EOL 守卫本地变红；内容零差，逐文件 `git checkout -- <path>` 即归一。本轮未动，因为这棵树有并行会话在写；归一那一笔应由推送方在空窗做。
- `docs/agents/workflow.md`「当前宿主：Windows + WSL」一节对这台 Linux 宿主已作废（该节自己写了换宿主整节删）；`~/.local/go` 与 `~/.local/node` 不在默认 PATH。
- HEAD 领先远端一笔（`44e8a355`，通道 3，docs-only），待推送方带走。

## 本文不包含

- 不裁断任何在办票或 Proposed ADR；去留以各票与用户裁定为准。
- 不评估 `docs/archive/` 与关务专项工作单的历史覆盖。
- 不重跑 `apps/admin-web` 的浏览器级探针（`scripts/dom-probe.mjs`），只跑了三道门。
