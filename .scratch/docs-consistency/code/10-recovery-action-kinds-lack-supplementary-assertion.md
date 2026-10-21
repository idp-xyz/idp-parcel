# 10 VE 追偿动作种类是封闭二值，缺「补充主张」

Category: enhancement
Status: ready-for-agent——R3（不急）：追偿动作经索赔编排（`application/handle_claim.go`，装配在 `cmd/parcel-api/assemble_claims.go`）已有命令面，但首发追偿以受控模拟为主；缺的是一种动作，不是答错，需要补充主张时可先登为又一份正式主张并在依据里写明。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-17（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象与证据

- 文档：VE CONTEXT Language「追偿通知或主张动作」：「预先通知、正式主张和补充主张可以是不同动作」；UC-VE-007 主流程第 8 步「分别准备预先通知、正式主张或补充主张」，输入语义「预先通知、正式主张或补充主张的独立动作」。
- 代码：`internal/visibilityexception/domain/recovery_matter.go` 的 `RecoveryActionKind` 只有 `PreliminaryNotice`、`FormalAssertion`，`valid()` 只认这两个。

## 三类

机制。无产品策略；补充主张的时限、对象等取值归合同与租户，未登记答未配置。

## 规模

S：枚举加一格，存储 CHECK 与读回随改；补充主张须回指它补充的那份正式主张。

## 最小的诚实修法

加「补充主张」：必须回指同一追偿事项下一份已形成的正式主张；不改写被补充那一份。

## 要裁的

无。不做的话，修订票 docs-consistency/04 在 UC-VE-007 注明首发只支持两种动作。
