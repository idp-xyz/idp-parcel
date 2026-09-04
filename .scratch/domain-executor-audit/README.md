# domain-executor-audit

`census-9d6063c.md` 是 2026-09-02 那次「有领域模型、无生产执行器」普查的取证快照，锚 `9d6063c`，不随后续提交改写。

当时用来量它的一次性探针（`probe/main.go`）已于 2026-09-04 随票 [mechanism-executor-triage/05](../mechanism-executor-triage/issues/05-type-reachability-ratchet-as-a-second-baseline.md) 退役：同一套类型可达性量法现在是 `internal/architecture/production_type_reachability_ratchet_test.go` 这道门禁，名单冻在同目录的 `production_type_reachability_baseline.txt`。要重量，跑 `go test ./internal/architecture/ -run TypeReachability -count=1`，不要再复活探针——两套量法并存就是第二口径。
