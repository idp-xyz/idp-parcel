# 18 外部承运凭证登记册：凭证指向哪个载运对象，今天没人答

Category: enhancement
Status: resolved——MCP-2（2026-09-04；认领基线 `9e3c1a5`，实现 `024cb5f`，完成记录见文末）
Blocked by: 无（`16` 已 resolved）

## 缺口

票 `16` 的收编执行器要把一条素材认领为「就明确外部承运凭证所指对象」的事实（TF CONTEXT
「外部承运轨迹事实」生命周期节：「素材带有发生时间**且对象、凭证可关联** → 事实成立」）。它通过
`ports.ExternalCarrierCredentialResolver` 问「这份凭证此刻指向哪个载运对象」，而全仓**没有任何生产实现**
——机制清点（`e697c9a`）「缺」名单已如实列入。执行器对此答`未决`（`TrackingCredentialRegistryUnconfigured`），
不留痕：那是本上下文自己的缺口，不是源的缺陷。

TF CONTEXT「外部承运凭证」词条早已定义了要登记的东西：分配方、真实标识对象、适用范围、版本、替代关系；
CONTEXT-MAP 把「外部承运凭证的身份和版本」列在 TF 拥有清单里。**缺的是那本登记册的代码与写面。**

## 做什么

1. TF 领域新立「外部承运凭证」聚合（或值对象＋登记册），按 CONTEXT 词条五件事建模；作废、失效、替代只改变
   适用关系，不删历史凭证（CONTEXT 规则节）。
2. `ports` 加登记册仓储；`adapters/postgres` 落表与迁移（`transport_fulfillment/0012`），真库实跑。
3. 以该登记册实现 `ports.ExternalCarrierCredentialResolver`：登记过 → `CredentialResolved` 并交回对象；
   没登记过 → `CredentialUnknown`（执行器据此留痕）。**`CredentialRegistryUnconfigured` 只在装配处没接
   登记册时出现**，接上之后这一格不再由本实现产出。
4. 登记写面（管理台或 `cmd/parcel-*-register` 形状）随实施票按 ADR-0101「登记频次 × 操作者角色 × 载荷结构」裁。

## 红线

- 凭证不解释为包裹的当前运单号（CONTEXT 词条）；「真实标识对象」是登记出来的，不从单号格式推断。
- 不填任何承运商的凭证格式、校验规则（实例半边）。
- 不动 `16` 落的执行器分格：`未知`留痕、`未配置`未决两格的语义由端口注释已定。

## 完成判据

登记册有实现与真库测试；`ExternalCarrierCredentialResolver` 有生产实现且机制清点「缺」名单里该口消失；
`16` 的执行器用例矩阵里「凭证不认识→留痕」一格能在真实现上复现。`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

票 `16` 完成记录「刻意留下的三格」第 2 条；TF CONTEXT「外部承运凭证」词条与规则节；
`internal/transportfulfillment/ports/external_tracking_fact.go` 的 `ExternalCarrierCredentialResolver`。

## 完成记录（2026-09-04，MCP-2）

两段会话接力：通道 2 上一段会话 11:55 认领，在共享树上写完领域三件（`ExternalCarrierCredential` 聚合与
三条改变适用关系的门、`RehydrateExternalCarrierCredential` 重建门、用例）、`ports.ExternalCarrierCredentialRegistry`
与迁移 `0012`，12:22 中断于 postgres 适配器开写之前；本会话从中断点续做，实现落 `024cb5f`（一笔，含在途件），
机制清点与本记录随下一笔。

**做什么逐条**：

1. 聚合按 CONTEXT 词条五件事建模；作废、失效、替代各成新版本回指前版并落定适用终点，原版本一字不动 ✓
2. `adapters/postgres.ExternalCarrierCredentials`：一版一行只插不改，「当前版」按回指派生（表上没有 current 列）；
   迁移 `0012` 的 CHECK 逐条镜像重建门（适用中不回指、改变必有终点与前版、替代者只随已替代出现）；真库实跑 ✓
3. 同一只类型实现 `ExternalCarrierCredentialResolver`：按「适用状态 × 适用范围盖住此刻 × 标识的是载运对象」
   三道答 `RESOLVED` / `UNKNOWN`；**从不产出 `REGISTRY_UNCONFIGURED`**，那一格只剩装配处没接登记册一种来路。
   范围那一道是在票面之外多守的一格：首版可带预先声明的终点，到期而没人来登失效版本时仍拦得住 ✓
4. 写面按 ADR-0101 决定八自裁：低频、五件事、一行一版 → **逐字段表单候选**，不走模板导入与草稿。落的是
   在线登记口两个（首登 / 改变适用关系），照 ADR-0085 两阶段：`CredentialIntake` + 端点构造函数、
   `UnconfiguredIntake` 两口同堵、端点表挂字面量、生产装配接真库（`buildExternalCarrierCredentialRegistration`）。
   路径 `/transport-fulfillment-external-carrier-credential-{registrations,applicability-changes}`；管理台页面
   归 admin-write-faces 那一族另立，本票不落 ✓

**完成判据逐条**：登记册有实现与真库测试 ✓；`ExternalCarrierCredentialResolver` 有生产实现、机制清点两口径
「缺」名单里该口消失（`docs/product/MECHANISM-INVENTORY.md` 在 `024cb5f` 干净检出上重生成） ✓；票 `16`
执行器矩阵「凭证不认识→留痕」在真登记册上复现——`TestTheAdoptionExecutorLeavesMaterialUnadoptedWhenTheRealRegistryDoesNotKnowTheCredential`
同时证反面：登记后同一素材被认领、对象取自登记册而非凭证字符串 ✓；`024cb5f` 干净 detached worktree 上
`gofmt -l internal cmd tools` 空、`go build ./...` 与 `go vet ./...` 退 0、`go test -count=1 ./...` 94 包 ok / 0 FAIL，
**DSN 已设、本机 PG 门禁容器实跑，真库用例 `-v` 下为 PASS 不是 SKIP** ✓

**登记编排的结果代数**（`RegisterExternalCarrierCredentialHandler`）：`CREDENTIAL_REGISTERED` / `APPLICABILITY_CHANGED`
（201）；`EXISTING_VERSION`（同键同内容重放）、`CONTENT_CONFLICT`（同键异内容，保留原版本）、
`CREDENTIAL_ALREADY_REGISTERED`（同一凭证拒立第二个首版——两个不回指前版的版本会让「当前版」成为两个答案）、
`CREDENTIAL_NOT_REGISTERED`、`NO_LONGER_APPLICABLE`、`INPUT_NOT_ACCEPTED`、`REGISTRATION_UNDECIDED`（200）。
版本由登记方指名不在这里铸：它是分配方那一侧这份凭证的版本身份，同一版本重放要能被认出来。

**刻意留下的**（不是欠账，各有归处）：

1. 收编执行器的生产装配点仍不存在（票 `16` 完成记录第 1 格：随第一家真源的拉取节拍票立）。解析口的生产
   实现今天由 `cmd/parcel-api` 的登记口装配构造并在真库用例里被执行器用到；到那一步在节拍的装配点把同一只
   `ExternalCarrierCredentials` 按解析口注入即可，本票不替它预留。
2. 迁移号：`0012` 已占；tf-segment-lifecycle-closure/02 与 `19` 若落表从开工那刻重取（此刻为 `0013` 起），
   ADR-0103 Consequences 里「`0012` 起」那句到 tf/02 开工时一并改。
3. `0012` 落在既有 `transport_fulfillment` 模块目录，`migrations/migrations.go` / `plan.go` 不动。

**验证口径**：本票的两道棘轮（函数名、类型可达性）在 `024cb5f` 上对 TF 零新增——`RegisterExternalCarrierCredential`
经登记编排、`RehydrateExternalCarrierCredential` 与两个 `Parse*` 经 postgres 适配器、`ExternalCarrierCredentialSpec`
经编排签名各有生产调用路径；基线两份文件本票未动。
