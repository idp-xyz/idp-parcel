# 13 管理台读面：送站预报与送站交接单的查阅，待识别实物显示交付范围核对态，交接单详情带「确认绑定」动作

Category: enhancement
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（09、11、12 已进 main），分支 `claude-psxm13`，走 Go 那条路。此前 ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by:
- [09](./09-pre-advice-submission-on-the-first-party-customer-channel.md) 进 main：预报要先有来路，读面才有东西可看。
- [11](./11-identity-check-judges-the-verifiable-delivery-scope.md) 进 main：交付范围核对态落册之后，复核目录才带得出它。
- [12](./12-handover-notes-are-opened-filled-closed-and-confirmed.md) 进 main：交接单与「送站交接确认」运营决定口由它落。
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定五（核对态是答复与复核目录里的信息格）；[UC-NO-006](../../../docs/application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md) 结果语义契约与「权限、隔离与审计」；UC-NO-002 结果语义契约之后讲交付范围核对态的那一段。
地盘：
- `internal/nodeoperations/adapters/http`：运营查阅读的预报列表与详情、交接单列表与详情（新文件），及测试
- `internal/nodeoperations/ports/catalogue_read.go`、`adapters/postgres/review_catalogue.go`：复核目录带交付范围核对态、未证原因与依据引用；预报与交接单的查阅读端口与实现
- `cmd/parcel-api`：端点表、装配
- `apps/admin-web/src/pages/operations/**`：预报页、交接单页、待识别实物页的核对态一栏、交接单详情的「确认绑定」动作
- `.scratch/at-coverage-inventory.md`

## 做什么

1. **查阅读口**（运营查阅读能力面）。
   - 预报：按节点与预计送达日列，详情给号码集合、客户账户、撤回与否。
   - 交接单：按节点与开立日列，详情给声称的客户账户、每个账户的绑定结论与途径、成员与各自形成方式、是否关闭、范围是否成立。
   - 跨租户与查无同答。
2. **复核目录**。待识别实物多带三格：交付范围核对态（已证、未证附原因、未配置）、依据引用、此刻归入的交接单（若有）。管理台待识别实物页显示这几格，让人分得出「缺范围」「范围成立但冲突」「零候选」与「未核对」。
3. **确认绑定动作**。交接单详情对每个待确认的客户账户给「确认」按钮：选交付方参与方（读 PC 的参与方目录）、填依据引用，提交到票 12 的运营决定口。答复各格照实呈现，「未配置」「不在授权交付关系内」写明恢复动作。
4. **走 Go 那条路**：随本票分支、非作者评审与推送方全量，不走前端切片的 main 直做（同 no-reception-identity/06 的先例）。admin-web 三道门照 [workflow 本机环境](../../../docs/agents/workflow.md#本机环境) 的口径。

## 不做

- 交接单的开立与归入（作业事实登记，属一线作业端）。
- 客户面读口。
- 预报有效时窗与授权交付关系的登记页（各自登记写面已在票 08、10；管理台页另立，若需要）。

## 完成判据

- [ ] 读口用例：预报与交接单的列表与详情，跨租户与查无同答；未授予运营查阅读答其格。
- [ ] 复核目录用例：交付范围核对态、未证原因、依据引用与归入的交接单都带出。
- [ ] admin-web 三道门退 0；核对态判读、确认动作的载荷拼法与各格答复的呈现有用例（落在判读纯函数上，照 identity-disposition 的先例）。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`internal/architecture/...` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

M
