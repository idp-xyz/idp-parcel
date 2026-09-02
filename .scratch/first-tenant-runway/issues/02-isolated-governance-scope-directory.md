# 隔离形态的生产归属目录与自身权威串

Category: enhancement
Status: resolved（2026-09-02，MCP-5；形态判定所缺的那份 ADR 同轮落文为 [ADR-0091](../../../docs/adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)）

演示动线三堵墙的**墙二**。取证基线 `c9835bf`。

## 墙

即便绕过墙一直调提交编排，`cmd/parcel-api` 的 `buildSubmissionOrchestration` 把 `ProductionOwnershipAdapterDeps` 的 `Directory` 与 `SelfAuthority` 留空，归属如实答`权威未确定`，提交停在 `OWNERSHIP_UNRESOLVED`。

**这一格的恢复动作不是登记。**`ProductionOwnershipAdapter.governanceScope` 在 `Directory == nil` 或 `SelfAuthority == ""` 时**先于**读治理登记册就返回未配置——往 `pilot_governance.authority_interval` 里登多少行都不会改变答案。这条曾被写反过，勘误记在 `.scratch/syn-wall-door-audit/issues/13-production-ownership-bridge-has-no-assembly-point.md` 的 2026-08-26 评论里。

桥本身已经接了（票 13 `resolved`，`AnswerValidity` 裁为单次提交处理视界、装 1 分钟）。缺的只有这两格。

## 做什么

在**隔离形态**下给出这两样，生产形态一字不改仍留空：

1. **`GovernanceScopeDirectory` 的一个实现。** 它把判断范围折成治理登记册认得的坐标。隔离形态下用合成坐标。
2. **自身权威串。** 隔离形态取 `SYN-` 前缀的合成值。

落点跟随票 01 的形态判定——两票都在 `cmd/parcel-api` 的装配层动手，**装配点占号，串行排**，不要并行改同一处。

## 一处要先想清楚的

票 13 把 `SelfAuthority` 判成实例半边「不代拟坐标」，这在**生产**上我同意。但它的勘误又写明恢复动作是「写一个目录实现并说出自己的权威串」——这听起来更像产品对自己的一次声明，而不是等租户填的取值。

**本票不重开这个定性**，只在隔离形态下给合成值（跟 `SYN-TENANT-01` 同性质，不需要重新定性就能给）。但做票人如果在实现过程中确认「本产品的权威串」根本不依赖任何租户，请单独提出来——那意味着生产形态的这一格也不该空着，而那是另一个决定。

## 必须守住的一格

**答案的来源要变，结论不许被抄近路。** 接上合成目录之后，归属应当真的读了治理登记册并按里面的合成区间答；如果登记册为空它照旧答`权威未确定`。不许写一个直接返回「已确定」的假目录——那就把墙二从「诚实的未配置」变成了「撒谎的已配置」。

## 完成判据

隔离形态下提交编排能越过 `OWNERSHIP_UNRESOLVED`；生产形态下行为一字未变并有测试钉住；登记册为空时仍答`权威未确定`且有用例覆盖；`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库（归属链有真库用例，须 PASS 非 SKIP）。

## 参照

`internal/parcelshipment/adapters/pilotgovernance/production_ownership.go`；`cmd/parcel-api` 的 `buildSubmissionOrchestration` 与 `assemble_submission.go`；[ADR-0063](../../../docs/adr/0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md)、[ADR-0017](../../../docs/adr/0017-admission-gates-judged-by-blocking-cause.md)（按阻断原因判读）；`.scratch/syn-wall-door-audit/issues/` 的票 02 与票 13。

## Comments

- 2026-09-02 MCP-5（用户经通道 5 授权接本票，并在四路范围选项间裁定「一份 ADR 同时裁两格」）：**墙二已降。**

  **先撞上的是形态判定，不是实现。** 票面写「落点跟随票 01 的形态判定」，而票 01 是 `ready-for-human`；照抄 ADR-0078 那套开关又撞 [ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) Decision 四「按环境选择的只有装配点上查阅行的 Intake 一件事」——归属目录不是查阅行的 Intake，是第二维。另核出票 01 那份 ADR 按其原范围（写端点的 Intake）也**不自动覆盖本票**：两条缝各自都要碰 Decision 四。用户裁定合并为一份记录，即 ADR-0091；它把适用面由枚举改为三条入格判据，并明写写面有持久化、ADR-0078 的「零持久化」论证在这一侧不成立，可分辨物改由 `SYN-` 前缀承担（形状同 ADR-0089 的 `FTI/` 标记）。

  **本票不重开的那个定性也没被重开。** 票面「一处要先想清楚的」问的是 `SelfAuthority` 到底是不是实例半边；本轮按票面办，只在隔离形态下给合成值，生产形态两格照旧留空，票 13 的定性一字未动。

  **交付**：`internal/parcelshipment/adapters/pilotgovernance/isolated_governance_scope.go`（`IsolatedGovernanceScopeDirectory`，只交坐标、参数匿名、残缺坐标构造期即拒）、`cmd/parcel-api/assemble_isolated_write.go`（`IDP_PARCEL_ISOLATED_WRITE_TENANT` 三态门、两开关一致性校验、合成坐标与权威串常量）、`buildSubmissionOrchestration` 多一个隔离入参（nil 即生产形态）、`main.go` 的解析与启动日志、`scripts/demo-seeds/data/governance/07-authority-interval-shipment-intake.json` 与 seed.sh 一行。

  **「必须守住的一格」有反证钉着**：三个真库用例互为对照——生产形态（nil）答 `OWNERSHIP_UNRESOLVED`、隔离形态空册仍答 `OWNERSHIP_UNRESOLVED`、隔离形态且册里有匹配区间才走到 `SUBMITTED`。一个直接返回「已确定」的假目录会让第二条变红。期望修订不在用例里照 `revisionFor` 重算，改走调用方真实拿得到的那条路：先提交一次从归属决定里读出修订，再以另一份来源身份提交。

  **未做，且是有意的**：墙一（命令面 Intake）不与本票同批落地。ADR-0091 Consequences 已把这个中间态写明——设了写开关只降墙二，`POST /shipment-requests` 仍答 `403`，因此演示动线脚本本轮**不改**：它描述的行为没有变。

  验证：`gofmt -l` 空、`go build ./...` 与 `go vet ./...` 退 0、`go test -count=1 ./...` **含真库**（同刻探针 `TestFreezeScopesAreInvisibleToEachOther` 得 `PASS` 非 `SKIP`）。首跑有一处 `FAIL` 落在 `internal/platform/outboundcall`，经核是另一会话正把该目录改名为 `internal/platform/outbound/` 的半途态，与本票改动无交集。
