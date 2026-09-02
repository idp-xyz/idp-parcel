# 隔离形态的生产归属目录与自身权威串

Category: enhancement
Status: ready-for-agent

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
