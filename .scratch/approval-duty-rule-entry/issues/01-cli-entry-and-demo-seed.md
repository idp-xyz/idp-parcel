# 01 register-approval-duty-rule 子命令与演示种子

Category: enhancement
Status: resolved——2026-11-04 进 main（本笔提交）
父票：[spec](../spec.md)

## 要做的

1. `cmd/parcel-commercial/register_approval_duty_rule.go`：翻译 + 交 `ApprovalDutyRules.SaveApprovalDutyRule`，退出码照本 CLI 的三格。
2. 用例：翻译（必填、可缺、未知键）；对库（首次、重放、冲突不覆盖）。
3. 种子 `scripts/demo-seeds/data/commercial/register-approval-duty-rule.json` 与 `steps/100-commercial.sh` 一步。
4. ADR-0362；`PAR-COM-18` 状态格改写。
5. 对演示库跑一次子命令，然后在管理台走通「存为待批准 → 批准 → 发布」。

## 完成判据

- `go build`、`go vet`、`go test ./cmd/parcel-commercial/`（含带库用例）退 0；三份生成物重生成。
- 演示租户：审批与发布页右栏批准答已批准、发布答已发布，版本出现在目录。

## 完成记录（2026-11-04，直接在 `main` 上做）

- 落点：`cmd/parcel-commercial/register_approval_duty_rule.go`（翻译 + 交 `ApprovalDutyRules.SaveApprovalDutyRule`，退出码三格照本 CLI）、`main.go` 分派；用例两条（翻译；对库首次 / 重放 / 冲突不覆盖，在一次性 Postgres 上 PASS）；种子 `register-approval-duty-rule.json` 与 `100-commercial.sh` 一步；ADR-0362；`PAR-COM-18` 状态格改写。
- ✅ `go build ./...`、`go vet ./cmd/...`、`go test ./cmd/parcel-commercial/` 退 0；机制清点与 ADR 索引在 tip 上重生成（单独成笔）。
- ✅ 对演示库：子命令答 `SAVED`（录入者与批准者可为同一主体；不要求等级），重放答 `ALREADY_REGISTERED`。随后在管理台（land 构建旁路实例 19081 + vite 5175）走通：服务产品页「发新版本」给 `SYN-PROD-CN-SG-EXPRESS` 存 v2 为待批准（`DRAFT_SUBMITTED`）→ 审批与发布页右栏「批准」答 `DRAFT_APPROVED`、栏底换成「发布」→ 确认框后答 `DRAFT_PUBLISHED`（`PUBLISHED_EFFECTIVE`，交付条件声明 `SAVED`），栏显已发布与批准 / 发布时刻；对象页 `EXPRESS v2` 历史两版，点 v1 换版。演示库因此多了 EXPRESS v2（真实业务写入，留着）。
- 顺手：审批页钉在表尾的那一行原先显旧状态（栏里已发布、行上还写待批准），改标「已离开列表」（`PendingPublicationsPage.tsx`，`pages/**` 自审）。
- 评审：非作者评审第一轮 BLOCK（ADR-0126 决定五被接上却没挂部分停用指针；端口注释仍写旧规则；种子 README 没加行；测试助手重复），四处照改后复核 APPROVE。证据 S。
