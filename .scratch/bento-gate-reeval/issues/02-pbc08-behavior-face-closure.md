# PBC-08 行为面收口：补 30 方法真缺证据，53 行归属定案，审计脚本门禁化

Category: enhancement
Status: resolved

接手声明(2026-08-25 MCP-1):用户经队列指示清空全部未结票,本票为唯一 ready-for-agent
实现票,由 1 号领办。地盘注意:CC 与 PP 的 adapters/postgres 目录与 MCP-2 在途 7 页批
重叠,本票只新增测试文件与脚本,占号广播随开工发出。

## 决议(2026-08-25,MCP-1)

- 落库 SHA:ccbf79e(12 文件:常驻门禁 internal/architecture/pbc08_behavior_evidence_gate_test.go
  + 十包 transaction_guard_test.go / enqueue_once_test.go 共 11 份负向测试)。简报三处
  随后笔更新(PBC-08 记为通过、闸门表证据格、行为面段)。
- **交付物 3 先行(脚本门禁化)**:评估结论=可并入且已并入。审计以纯标准库 go/ast 重写为
  internal/architecture 常驻门禁,与本包既有门禁同构(含「门禁能红」自证测试,四条归属
  路径各有合成用例)。归属按接收者变量类型解析,四路:直接构造 New<T>、元组夹具按签名
  结果类型位置映射、结构夹具按字段类型、未导出写方法记账到同类型导出调用方——前两路
  正是票 01 实证的「捆绑夹具 new*Stores + 包内非唯一 Save/Replace」假 MISSING 的修法。
  原 audit-pbc08.ps1 不再落盘(一次性脚本被常驻门禁取代,原件仍以 git show 2399ecd 调取);
  「零假阳」以门禁绿为证:首跑红单逐条核对后全为真缺,既有约 128 条证据全部被认出
  (PS 仅余 1 缺、NO 清零——票 01 第二节所列 53 行待归属的包全部自动归属成功)。
- **交付物 2(定待归属,把「84 缺」修成真实清单)**:门禁在 44eeaa6 首跑红单 68 方法/十包
  (同树原脚本口径报 116 缺,变量级归属自动清 48 条假缺):CC 23、VE 19、NR 8、SA 7、
  PC 4、PP 3、PS/PG/TF/outboxintent 各 1。较票 01 基线(49a2ab0)的真缺 30 多出的部分
  全部来自其后新落的登记册/目录写口(CC 五本案件配置登记册、VE CatalogRegistrar 六目录、
  NR 网络目录七族+自动改路事实、PP 价卡与参考序列、PC 声明发布四正文、PS 解析键、
  PG/VE 通道留痕)与两条基线漏点(SA StatementDisputes.Replace、SupplierExpectedCosts.Save)。
- **交付物 1(补真缺)**:68 口全数补齐,票 01 列名的 30 方法尽在其中。守卫先于入参解读的
  写口传零值;守卫前有入参门的格(留痕完整性、CC 三处封闭枚举、NR 行完整性)传最小有效值
  让请求走到守卫。TF ResultVersions 的守卫在未导出 next 里,证据按门禁口径落在两个导出
  签发口上(真实调用路径)。
- **EnqueueOnce 口径(票面边界点名的未决)**:裁定自带第一手证明、不留豁免清单——
  internal/platform/outboxintent/enqueue_once_test.go 直接断言无事务被拒,间接覆盖
  (每个 Outbox handoff 负向块取道它)降为冗余而非依据。
- 验证(提交态,临时 worktree 检出 ccbf79e,DSN 已设):gofmt 零输出;go build / go vet
  零信号;全仓 go test -p 1 -count=1 零 FAIL(78 包 ok,285 秒);新守卫用例单跑 -v 为
  PASS 非 SKIP(TestRemainingSettlementWritesRefuseToRunOutsideATransaction 出示)。此为
  「含真库的绿」。验证树查净后按纪律拆除(无 --force)。
- 边界重申:本票完成**不解除** Bento 闸门——九项 PBC 证据已齐,登记 B-06 是另一步闸门
  动作,按票 01 裁定另行报批;简报闸门表结论格照旧「保持阻断」。
- 树上 MCP-2 在途 7 页批文件与三份死现场改动均未带走(提交信有记)。

来源：[票 01](./01-reeval-verdict-bento-gate-stays-blocked.md)「差什么」第 1、2 条，
2026-08-21 MCP-3 受用户委托裁断开票。证据基线与三分类清单以票 01 正文为准，原件以
`git show 2399ecd:.scratch/bento-gate-reeval/<file>` 调取。

## 交付物

1. **补真缺**：票 01 列名的 30 方法 / 22 类型（customscompliance 12、visibilityexception 12、
   parcelpricing 1、settlementaccounting 特名 5）逐个补「引用 `ErrTransactionRequired` 且调用
   该方法」的负向证据块。简报明文该性质「随适配器逐个成立，没有静态门禁能替它把关」。
2. **定待归属**：按 `evidence-blocks.txt`（`2399ecd`）完成 53 行人工归属，把审计输出的
   「84 缺」修成真实清单；已实证的假 MISSING 形状（捆绑夹具 `new*Stores` + 包内非唯一
   `Save`/`Replace`）见票 01 第二节。
3. **脚本门禁化**：改进 `audit-pbc08.ps1` 的归属逻辑（识别捆绑夹具），使其可复跑、零假阳后，
   评估并入 `internal/architecture` 常驻门禁的可行性；并不进去也要把可复跑版脚本与口径
   落盘（不再是一次性脚本）。

## 边界

- 只补测试与脚本，不改任何生产写方法的行为。
- `outboxintent.EnqueueOnce` 自包证明要不要求，属口径问题，本票按票 01 的记载与行动 2 一并
  定口径并写明理由，不默认。
- 本票完成不解除 Bento 闸门：它只收 PBC-08 行为面一项；闸门解除按票 01 裁定走九项全过 +
  `B-06` 登记。

## 参照

票 01（三分类清单与证据索引）；ADR-0026（先证后闸）；`docs/design/` 持久化简报的
「每新增一个持久化适配器都要自带这条证明」。

## Comments

- 2026-08-21 MCP-3：随票 01 收口裁定开票。
