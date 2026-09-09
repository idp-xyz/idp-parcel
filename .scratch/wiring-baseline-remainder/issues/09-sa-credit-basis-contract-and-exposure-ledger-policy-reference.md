# SA 收 ADR-0127 的 contract 段：`Deps.CreditBasis` mandatory、暴露账本行持久化政策引用；PS 夹具先补 `CreditBasisView` 替身

Category: enhancement
Status: in-progress——19:4x 通道 3 接续（接续单 task-de74738a-f485-422e-8e85-7a78d474d1ad；前一任 18:4x 自领后 18:5x 中断，树上留一份未提交的 PS 夹具
测试 +62/−19、mtime 18:47:46），分支 mcp3-wbr09 基 d5a35960。接手对照：先自列第 1 步判据（三格替身 / 接口断言 / 三处 `Deps` 构造全接 / 账期夹具额度与状况
同值以保住既有用例的读法 / 不改既有断言），再读 diff——逐条对上，PS 包 build/vet/test/gofmt 绿，原样接着用。
2026-09-09 通道 1 代裁立票（用户授权自决）：wbr/03 完成记录「未落三件」之②，ADR-0127 决定五写明「contract 段——依赖 mandatory、
nil 在构造期拒——随 PS 那份夹具补上 `CreditBasisView` 替身的那笔一起落，票面记为 SA 后续项」；Consequences 写明「暴露账本行上不持久化政策引用（要 SA 迁移，
随 contract 段另立）」。两半同票、先 PS 夹具后 SA contract，一个通道做完，PS 那份测试文件改前占号
Blocked by: 无

## 条目

- `internal/settlementaccounting/application/apply_pre_acceptance_control.go`：`ApplyPreAcceptanceControlDeps.CreditBasis` 为 nil 时 `exposeCredit` 沿旧路（额度取
  登记状况、结果不带政策引用）——ADR-0127 决定五的 expand 段，留它只因 `internal/parcelshipment/adapters/settlementaccounting` 的测试夹具直接构造这份 `Deps`。
- SA 暴露账本行今天不持久化「实际采用的政策」引用；`settlement-accounting` CONTEXT 要求「每项信用暴露保存实际采用的政策与范围」，`AT-SA-171` 的政策半边今天只在
  形成时的结果上成立、落库即丢。

## 做法（顺序固定）

1. **PS 夹具（PS 地盘，只动测试）**：`internal/parcelshipment/adapters/settlementaccounting` 里构造 `ApplyPreAcceptanceControlDeps` 的夹具补一只 `CreditBasisView` 替身
   （found=true 带一份 `CreditBasis` / found=false / error 三格可配，照 `PreAcceptanceControlPolicyView` 替身的写法）；单独一笔，提交说明写明是为 SA contract 段铺路。
2. **SA contract 段**：`Deps.CreditBasis` 在构造期拒 nil（与其它 mandatory 依赖同一处、同一错误形状）；`exposeCredit` 删旧路分支——额度一律取信用依据；
   `cmd/parcel-dispatch` 装配已接真适配器（ADR-0127 同笔），零改动应能编过——编不过就是有第三个构造点，找出来一并接。
3. **SA 迁移**：暴露账本行加政策引用列（对象 + 版本两格，与 `CreditBasis` 里的政策版本引用同形；可空只为存量行，新写入非空由应用层保证、CHECK 不追溯）；
   `adapters/postgres` 写入 / 读回带它；`operational_position.go` 读面若列暴露明细则带出政策引用列。
4. 测试：SA 应用层 nil 拒 / 有依据取政策额度 / found=false 落 `CREDIT_BASIS_NOT_CONFIGURED`（既有）；PG 真库写读往返带政策引用；PS 夹具那侧既有用例全绿。
5. `AT-SA-171` 在验收对照里标「政策半边落库成立」；wbr/03 完成记录「未落三件」之② 回指本票。

## 完成判据

`Deps.CreditBasis == nil` 构造期拒；暴露账本行落库带政策引用并可读回；含 DSN 跑 `./internal/settlementaccounting/...`、`./internal/parcelshipment/adapters/settlementaccounting/...`、
`./migrations/`，反向依赖含 `cmd/*` 的包带 DSN 跑；gofmt / vet 0。

## 边界

不动 PC；不动比例额度基数（那是 [10](./10-credit-ratio-base-is-declared-on-the-credit-policy-content.md)）；PS 侧只动那份夹具；不改 ADR-0127 正文。
