# 03 CC 放行门禁核对编排无装配

Category: enhancement
Status: in-progress——2026-10-28 会话 tops-20（idp-parcel-91）立并认领（用户「现在就接线」），分支 claude-lw-cc
Blocked by: 无

## 做什么

把 `VerifyReleaseGateHandler` 接成生产入口（触发方照 UC-CC-009），折不出结论答 `UNDECIDED`+`GATE_CONDITION_VIEW_UNAVAILABLE`（code/34）；生产装配真库用例。

## 完成判据

- 生产路径能做放行门禁核对并落册、发意图；折不出结论时不落册（真库用例）。
- 演示动线里「立案 → 放行结果 → 交出门禁从未决变放行」这一格能走到（psb/05「预走」记的收口重走顺序），或写明还差什么。
