# 12 送站交接单：作业事实登记开立、归入、关闭；客户渠道与运营决定两路确认绑定；范围成立时自投门铃重核

Category: enhancement
Status: ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by:
- [08](./08-pre-advice-and-handover-note-model-and-window-register.md) 进 main：交接单的领域与册。
- [10](./10-authorized-delivery-relationship-in-party-commercial.md) 进 main：授权交付判断与 NO 侧适配器。
- [11](./11-identity-check-judges-the-verifiable-delivery-scope.md) 进 main：门铃触发的重核要带上范围的判断。
- 首方客户渠道 Intake 已由 [first-party-api/00](../../first-party-api/issues/00-first-party-customer-intake.md) 进 main（`38916b00`），客户渠道确认口照它的形状挂，不设阻塞边；与 [09](./09-pre-advice-submission-on-the-first-party-customer-channel.md)、first-party-api/01 同改客户面能力的枚举与 CHECK，谁后进谁 rebase。
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定三、六、九；NO CONTEXT 生命周期「送站交接单」；[UC-NO-006](../../../docs/application/node-operations/UC-NO-006-ESTABLISH-A-VERIFIABLE-DELIVERY-SCOPE.md) 与 `AT-NO-071` 至 `AT-NO-075`。
地盘：
- `internal/accessidentity`：作业事实登记的事实类型加开立、归入、关闭；运营决定加决定种类「送站交接确认」；客户面能力加「送站交接确认」；各自测试
- `migrations/access_identity/`：上面几处 CHECK，取当时下一个空号（另一会话正在连续取号，不写死号；可并成一份）
- `internal/nodeoperations/application`：开立、归入、关闭、两路确认、范围成立与门铃的编排，及测试
- `internal/nodeoperations/adapters/http`：作业事实登记三口、运营决定一口、客户面一口，及测试
- `internal/nodeoperations/adapters/postgres`：门铃，照 `identity_recheck_doorbell.go` 的做法；及测试
- 重核消费者认新门铃：`internal/nodeoperations/adapters/inbox` 与 `cmd/parcel-dispatch`（路由表一行；装配进既有的身份重核装配）
- `internal/nodeoperations/adapters/partycommercial`：票 10 的适配器装进 `cmd/parcel-api`
- `cmd/parcel-api`：端点表、装配、真库用例新文件
- `internal/architecture/partition_subject_registry_test.go`：门铃若用新分区键，加一行
- `scripts/demo-seeds`：运营决定与作业事实登记的授予、准入范围对照、治理种子的生产权威区间（照 no-reception-identity/06 的先例），合成货主系统的「送站交接确认」授予
- `docs/product/MECHANISM-INVENTORY.md`（重生成）、`.scratch/at-coverage-inventory.md`

## 做什么

1. **开立、归入、关闭**（ADR-0212 决定三，「作业事实登记」能力面）。
   - 事实身份由设备签发（ADR-0023），重放照该能力面的既有代数。
   - 开立：节点、交付方线索、声称的客户账户（至少一个）、开立时刻。
   - 归入：交接单须开放；作业实物须在同一节点有当前收寄，当前有效版本待识别、不带关联、不记身份冲突。不合条件答交接单状态不符。
   - 关闭冻结成员。
2. **运营决定确认**（「运营决定」能力面，决定种类字面量本票定，建议 `HANDOVER_NOTE_CONFIRMATION`）。
   - 载荷：交接单、客户账户、交付方参与方、依据引用。
   - 客户账户须在声称之列。
   - 经票 10 的适配器问授权交付判断：是即落确认；否答不在授权交付关系内；未配置答未配置；读不动不形成答案。
   - 运营决定只能确认，不能否认。不限于已关闭的交接单。
3. **客户渠道结论**（首方客户渠道族，客户面能力字面量本票定，建议 `HANDOVER_NOTE_CONFIRMATION`，照「一口一词」）。
   - 客户账户取自信封，须在声称之列；不在的与查无此单同答。
   - 只对已关闭的交接单，结论是确认或否认。
   - 来源请求键照首方族的推导口，从接入请求标识取。
   - 生产写口，准入范围在铸造侧判（ADR-0139 决定八）。
4. **结论代数**：同结论重投答已有结果；异结论答已有结论，并回显在册结论。
5. **范围成立与门铃**（决定六）。
   - 最后一个声称的客户账户得到确认时，同一笔事务给本上下文自投门铃（租户，交接单）；重核对象是名下当前有效版本仍待识别、不带关联、不记身份冲突的作业实物。
   - 向范围已成立的交接单归入一件时，同一笔自投（租户，那一版判断版本），只重核那一件。
   - 用既有的身份重核 inbox 消费者接，判法走票 11 的同一份身份判断。
   - 任一绑定被否认，不交门铃。
6. **答复分格**。信封格：客户面照 ADR-0139 决定四，操作者面照 ADR-0151 决定三与 ADR-0100 决定四。业务格照 UC-NO-006 结果语义契约，线上名本票定、写进完成记录。
7. **逐口核**（ADR-0151 决定三）：运营决定口凭哪条代数，写进完成记录。

## 不做

- 管理台的交接单页与确认按钮（票 13）。
- 客户查看交接单明细的客户面读口。
- 一线过渡导入模板里的交接单行（另票，若需要）。
- 运营决定否认绑定。

## 完成判据

- [ ] 编排用例 `AT-NO-071`：开立、收寄带它、归入、关闭；关闭后归入、归入不合条件的实物，都答交接单状态不符；归入不改写来源。
- [ ] 编排用例 `AT-NO-072`：授权交付判断答是、否、未配置，以及交付方是本人，依次答绑定已确认、不在授权交付关系内、未配置、绑定已确认；读不动不形成答案。
- [ ] 编排与 HTTP 用例 `AT-NO-073`：客户渠道对未关闭的答状态不符；已关闭、声称了它的答确认或否认；没声称它的与查无此单同答；同结论重投答已有结果，异结论答已有结论。
- [ ] 编排用例 `AT-NO-074`：声称两个客户账户只确认其一时不成立、不交门铃；另一个否认后不再成立，成员可以归入另一张交接单。
- [ ] 真库用例 `AT-NO-075`（`cmd/parcel-api` 与 `cmd/parcel-dispatch`，经生产装配）：没带依据而落待识别的同号实物，归入交接单 → 运营决定确认 → 门铃 → 重核追加关联版，业务时间取原收寄时刻、不另起控制起点，意图一封；名下已关联或记冲突的实物不受影响。修前先红。
- [ ] 信封各格：作业事实登记、运营决定、客户面三族各自的未配置、令牌无效、未授予、不在准入范围；别族令牌一律拒。
- [ ] 演示实走（`S`）：不带预报送一件，落待识别 → 开交接单并归入 → 运营决定确认 → 识别。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/accessidentity/...`、`internal/nodeoperations/...`、`internal/partycommercial/...`、`cmd/parcel-api`、`cmd/parcel-dispatch`、`cmd/parcel-access-register`、`migrations/...`、`scripts/demo-seeds/...`、`internal/architecture/...` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

L
