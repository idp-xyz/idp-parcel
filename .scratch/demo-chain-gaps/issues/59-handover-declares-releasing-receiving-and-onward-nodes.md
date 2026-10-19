# 59 权威交接声明交出、接收、去向节点

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `b6e8b7f9`：152 ok / 0 FAIL；59、28 同批，见文末「进 main 记录」）。此前 in-progress——完工待评审与进 main，分支 claude-dcg59（隔离评审 0 阻断，已按评审修订并 rebase 到 `de9e3ad9`；见文末「完成记录」「评审记录」）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [handover-dispatch-plan](../handover-dispatch-plan.md)。
Blocked by: 无（TF 规范化令牌排在 [21](./21-load-assignment-allocates-against-capacity-reservations.md) 之后）
地盘：TF `domain/transport_handover.go`、`handover_rehydration.go`、`payload_canonicalization.go`（交接出新形状版本）；`application/register_transport_handover.go`（命令三格，更正承继）；`adapters/postgres/transport_handover_registry.go` + TF 迁移（认领时现取）；tfhttp `register_transport_handover.go`、`query_transport_fulfillment_records.go`（交接册列三格）；`cmd/parcel-api` 新真库用例文件；文档 ADR-0213、TF CONTEXT「权威运输交接」词条一句、UC-TF-005 输入与 AT、`docs/design/synthetic-demo-journey-script.md`。
出处：[04](./04-handover-does-not-move-node-control-or-trigger-reassessment.md)；方案「复核」04「票面漏掉的真阻断」，「代决」C1，「需要的 ADR」ADR-0213。

## 做什么

- 交接登记加三个可选声明：交出节点、接收节点、去向节点；保存、摘要、更正与缺席的答法照方案 C1。
- 文档：起草 ADR-0213（要点见方案「需要的 ADR」，C13、C14 两条由 [65](./65-handover-records-an-outbound-release-gate-reading.md)、[66](./66-effective-delivery-records-a-final-delivery-gate-reading.md) 落，ADR 里写成决定并标明后继票）。
- 进 main 的令牌次序与波次见方案「波次与令牌」。

## 完成判据

- 三格缺席照登，答复不变。
- 同内容重放答 `EXISTING_VERSION`，含新形状之前入库的行。
- 同键换节点答 `SOURCE_CONFLICT`。
- 更正承继前版的节点声明。
- 查阅口 `registry=transport-handover` 回显三格。
- 真库测试。
- 演示：SHA 收寄后 `POST /transport-fulfillment/handovers` 带 `releasingNode=SYN-NODE-SHA-HUB` 答 201 `HANDOVER_REGISTERED`；原样重发答 200 `EXISTING_VERSION`；改 `receivingNode` 重发答 200 `SOURCE_CONFLICT`；查阅口列出三格。

## 规模

M

## 完成记录（2026-10-19，作者子代理，分支 `claude-dcg59`，原基 `2c6311f1`，评审修订时 rebase 到 `de9e3ad9`）

### 做了什么

- **领域**：`domain.HandoverNodeDeclarations`（交出、接收、去向三格，各自可缺）挂上交接判断，读口 `Nodes` / `ReleasingNode` / `ReceivingNode` / `OnwardNode`。裁决不限制三格。`Correct` 照前版承继，`RehydrateTransportHandoverSpec` 带三格。
- **摘要**：至少声明一格时首登摘要写新形状 `TFC-2`（三格都写，没声明的写 null）；一格都不声明仍写 `TFC-1`，字节不变。交接口改用 `domain.CompareStoredHandoverDigest` 比对，其余各口仍用 `CompareStoredDigest`。
- **编排**：`RegisterTransportHandoverCommand` 加三格。空白读作没有声明；存不下的值（含 NUL）答未受理。
- **存储**：TF 迁移 `0039` 给交接登记册加三列可空，空白串 CHECK 拒。登记册的各条读路（按键、按前版找后继、按范围）都带回三列。
- **查阅册**：`TransportHandoverCatalogueRow` 加三列，`registry=transport-handover` 逐格透出，没声明的键不出现。
- **线格式**：首登载荷加 `releasingNode` / `receivingNode` / `onwardNode` 三个可缺键，答复回显在册那一版。更正载荷不收这三键（闭合解码，带了答 400）。
- **不动的**：交接登记信封（仍只带键，消费方按键读回），所以票 60 的派送发起读交接不受影响。
- **文档**：
  - 起草 ADR-0213（决定一至三本票落，决定四、五写成决定并标明后继票 65、66）。
  - ADR-0206 的 Status 行与 Links 加「挂法已答」前向指针。
  - ADR-0014 的 Status 行与 Links 加部分停用指针。
  - `docs/adr/README.md`：新增 ADR-0213 一行；ADR-0014、ADR-0206 各一行只在行尾追加。原基上 ADR-0206 登了两行，两行都接了后缀；rebase 时 main 的 `3a790071` 已合成一行，后缀只接在留下的那一行上。
  - TF CONTEXT「权威运输交接结果」词条补一句。
  - UC-TF-005 输入契约加「交接节点声明」一行，验收表立 `AT-TF-216`–`222`。
  - AT 盘点点名 7 行加追记；演示动线脚本补记一条。
- **立票**：[78](./78-adr-readme-lists-several-recent-adrs-twice.md)，ADR 索引里 0206、0207、0209、0210 各登两行；rebase 后发现 main 已合掉，已标 resolved。[79](./79-admin-web-handover-register-shows-declared-nodes.md)，管理台交接册显示三格节点，出自评审。

### 提交

rebase 到 `de9e3ad9` 之后：`ded7e55c` 领域与编排 · `87100c95` 落册（迁移 0039、各条读路、查阅册） · `25eb8021` 线格式 · `c8973441` 生产装配真库用例 · `b972e9b2` ADR-0213 与前向指针、索引 · `19f8464d` CONTEXT、UC-TF-005、AT 盘点 · `b0b364da` 演示动线补记 · `028422b6` 立票 78 · `be22bb33` 完成记录。评审修订的提交见「评审记录」。

### 代决与越权风险点

方案 C1 已裁的部分（声明、不透明、不核目录与设备范围、进摘要、更正承继、缺席是答案）照做。下面是方案没裁、本票自决的细节，越权风险点逐条列出，待 TF owner 复核。

1. **旧形行上补报节点答冲突**，不按旧形重算认作重放。
   - 越权风险点：这部分停用了 ADR-0014 决定里「规范化版本不同不是冲突，也不得据此判定重放失败」一句，范围只限 TF 交接首登。它也是对方案 C1「旧行按旧版比」字面的收窄：只要本次没声明节点，旧行仍按旧形比、照答重放。
   - 越权风险点：节点进文法之前登下的交接永远补不上节点。
   - 理由：旧形里没有节点一格，按旧形重算会把声明静默吞掉。PC 不肯把 `PCC-2` 的条件按 `PCC-1` 静默丢掉，同一个理由。
2. **按内容分版，不整口换号。** 不声明节点的首登仍写 `TFC-1`，先例是 PC 的 `PCC-2`（ADR-0209）。
   - 越权风险点：交接口两种形状长期并存，比对多一道 `CompareStoredHandoverDigest`。
3. **更正载荷收到节点键答 400**，不是照收照弃。
   - 越权风险点：节点声明错了没有更正面，只能人工处置，而处置面未立。
   - 理由：更正承继前版；照收照弃会让设备以为改了节点。
4. **三格不随裁决限制。** 拒收与待确认同样可以声明节点。
   - 越权风险点：消费方要自己按裁决过滤。方案 C5、C6、C8 已规定只认`已交接`。
5. **答复与查阅册里没声明的键不出现**（与 `basis`、`corrects` 同一写法）。
   - 越权风险点：老版本服务的答复里也没有这些键，调用方只能凭服务版本区分「没声明」和「不认识这一格」。
6. **空白读作没有声明，库面 CHECK 拒空白串**，「没有声明」只有 NULL 一种写法。存不下的值答未受理，不进`未决`。
7. **ADR-0206 在 README 里的两行重复条目都追加同一段后缀**，不删任何一行。删重复行另立票 78。（rebase 后已无此事：main 的 `3a790071` 把两行合成一行，后缀只接在那一行上，票 78 已标 resolved。）

ADR-0213 文末另有越权风险点：声明错了没人核、节点与参与方两套指称、存量行补不上节点、没有改节点的面、没声明交出节点的交出不问 CC、交付门禁 at 的取法、去向没声明时跨区不列。

### 用例

`AT-TF-216`–`222`（UC-TF-005），Covers 点名分布在下列五个测试文件，逐条去向见 `.scratch/at-coverage-inventory.md`：

- `internal/transportfulfillment/domain/handover_node_declarations_test.go`
- `internal/transportfulfillment/application/register_transport_handover_nodes_test.go`
- `internal/transportfulfillment/adapters/postgres/transport_handover_node_declarations_test.go`
- `internal/transportfulfillment/adapters/http/handover_node_declarations_test.go`
- `cmd/parcel-api/handover_node_declarations_test.go`

`TFC-2` 的金样摘要另用 `sha256sum` 对同一份文档独立算过一遍，与测试里的定值相符。

### 验证（实测，只记 `S`）

- 构建与静态检查：`go build ./...`；`go vet ./internal/transportfulfillment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/`；`gofmt -l internal cmd`，三者都干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1 ./internal/transportfulfillment/... ./internal/architecture/` 全 ok。
- 同一 DSN，`-run 'Handover|ControlFact|WorkFact|Participation|TransportFulfillment|Endpoint'` 跑 `./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/visibilityexception/adapters/...`，全 ok。
- 没跑全量，也没重生成机制清点。
- **演示**：在一次性库 `dcg59_walk` 上 `seed.sh` 灌入退 0，起 `parcel-api`（操作者渠道三件加治理权威串），用 Dex 令牌依次取证：
  - SHA 枢纽设备收寄合成标识 `SYN-PARCEL-DCG59-01`，答 `200` `PENDING_IDENTIFICATION`。合成号不在关联册，属预期。
  - 华东场外设备交接首登，带 `releasingNode=SYN-NODE-SHA-HUB`，答 `201` `HANDOVER_REGISTERED` 并回显。
  - 原样重发答 `200` `EXISTING_VERSION`，回显在册的声明。
  - 加 `receivingNode` 重发答 `200` `SOURCE_CONFLICT`。
  - 另一件三格都声明的交接答 `201`。
  - `registry=transport-handover` 列出两件，节点格各照声明；库面两行摘要都是 `TFC-2`。
  - 不带令牌答 `401`。
  - 走完已停进程、`DROP DATABASE … WITH (FORCE)`。

### 偏离与待裁

- **代决 1**：偏离方案 C1「旧行按旧版比」字面，并部分停用 ADR-0014 一句，待 TF owner 复核。若 owner 改裁「补报节点按旧形认作重放」，只改 `CompareStoredHandoverDigest` 里 `TFC-1`/无版本那一支和 ADR-0213 决定三那一句，存量与新行都不用迁移。
- **合并提示**（已在评审修订时 rebase 解掉，留作记录）：
  - 与 `claude-dcg60` 冲突在 `.scratch/at-coverage-inventory.md`、`docs/adr/README.md`。
  - 与 `claude-dcg64` 冲突在 `docs/adr/README.md`、`docs/design/synthetic-demo-journey-script.md`。
  - 都是同一锚点两边各追加行，按「只追加」两边都留即可。
  - 代码与 `claude-dcg60`、`dcg64`、`dcg68`、`dcg28` 无冲突。那几条分支都不调用本票改了签名的 `CanonicalizeHandoverPayload`。
- **工作树并行**：本票先后有两个作者实例写同一个工作树，协调方已停掉原实例。`ee102f18`（rebase 后为 `ded7e55c`）由原实例提交，内容就是本实现的领域与编排那一半；逐文件核过，工作树里只有一套实现，没有重复或残留。

## 评审记录（2026-10-19，作者子代理，分支 `claude-dcg59`）

隔离评审 0 阻断，非阻断 7 项，逐项去向如下。

### 修了的

1. **ADR-0213 决定六第一条与方案 C5、票 61、62 不一致**（评审建议进 main 前修）。原写法是 NO 的转出与 HANDOVER_IN「分别取交出节点与接收节点」，读起来像转出挂在交出节点上。按 C5，`已交接`就转出实物此刻在身的那份控制，声明的交出节点只拿来比对。这条也与同节「本记录只定三格的形状与语义」自相矛盾。
   - 改法：两条都只指去处，即 NO 的转出与 HANDOVER_IN 怎样读三格由 ADR-0214 定，NR 的复核怎样取位置由 ADR-0215 定，与 README 那一行同一写法。另加一句本记录自己的边界：不把哪一格定为控制转移的前提，没声明交出节点不等于控制不转出。
   - 没有照评审的第一种写法（「转出不以交出节点为条件，声明的交出节点只作比对」）原样写进去。那是 NO 的规则，归 ADR-0214。ADR-0213 接受后不改正文，在这里写一份，等于同一决策两处定义。
   - 同节末句补「或与自家记录不符时」。
2. **ADR-0213 Consequences「登记册三条读路都带回这三列」是跨文件计数**（评审建议进 main 前修）。改成「登记册的各条读路都带回这三列」。同类计数顺手一并去掉：
   - 真库往返用例的注释；
   - 本票完成记录「存储」一条与提交清单；
   - 完成记录里「越权风险点七条」。
3. **分支落后 main**（评审建议进 main 前修）。rebase 到 `origin/main` `de9e3ad9`，三处冲突两边都留：
   - `.scratch/at-coverage-inventory.md`：AT 表与追记段各一处。票 60 的 `AT-TF-191`–`200` 在前，本票 `AT-TF-216`–`222` 在后。
   - `docs/adr/README.md`：
     - ADR-0206：main 的 `3a790071` 已把两行合成一行，后缀接在留下的那一行上。
     - 0213 一行排在 0216、0217 之前，三行按号升序。0216、0217 原有的先后不动。
   - `docs/design/synthetic-demo-journey-script.md`：票 64、60 两段在前，本票一段在后。
   rebase 后 `go build ./...` 通过。README 里按 `uniq -d` 查，没有重复行。
4. **README 里 ADR-0213 一行写「NO、NR、CC owner 会签」，读起来像会签已经完成**。改成「待 NO、NR、CC owner 会签」，与 ADR 正文 Status 一致。
5. **票 78「出处」过时**。改成票 74 已随 A 波进 main。rebase 时还发现 main 的 `3a790071` 已经把 0206、0207、0209、0210 各两行合成一行，票 78 要做的事 main 上已经做完，所以连同 Status 一并标 resolved，并补「关闭记录」。
6. **`TestABlankHandoverNodeIsRefusedByTheDatabase` 只断 err != nil**。
   - 改成用 `pgconn.PgError` 断 SQLSTATE `23514`，约束名为 `transport_handover_nodes_not_blank`。
   - 原来只证交出节点一列，现在三列逐列各证一遍。
   - 改的过程中见到一个边界：CHECK 照 TF 既有迁移用 `btrim`，只削空格，制表符串过得了库面。领域的 `TrimSpace` 范围更宽，挡在库前，所以领域收下的值一定过得了 CHECK（评审已核）。直插用例只取空格与空串，测试注释写明了原因。迁移不改，与 TF 既有写法保持一致。

### 另立票的

7. **管理台交接册不显示三格节点**。diff 全在 `apps/admin-web/**`，按 workflow.md「前端切片」在 main 上直接做，立票 [79](./79-admin-web-handover-register-shows-declared-nodes.md)，Blocked by 本票。本票不带。

### 修订提交

- `e5e11f5a`：ADR-0213 决定六与读路计数，README 会签措辞。
- `62cf4ddd`：真库空白节点用例断到 SQLSTATE 与约束名，往返用例注释去掉计数。
- `bae60f71`：票 78 标 resolved。
- `b5967922`：立票 79。
- 本笔：完成记录随 rebase 更新，加本节。

### 修订后验证（实测，只记 `S`）

- `go build ./...`、`go vet ./internal/transportfulfillment/... ./cmd/parcel-api/ ./cmd/parcel-dispatch/`、`gofmt -l internal cmd` 都干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1 ./internal/transportfulfillment/... ./internal/architecture/` 全 ok。
- 同一 DSN，`-run 'Handover|ControlFact|WorkFact|Participation|TransportFulfillment|Endpoint|Dispatch|Delivery'` 跑 `./cmd/parcel-api/ ./cmd/parcel-dispatch/ ./internal/visibilityexception/adapters/...`，全 ok。rebase 后这里已含票 60 的派送发起用例。
- 用 `-v` 逐条确认以下用例都是 PASS，不是 SKIP：
  - `TestABlankHandoverNodeIsRefusedByTheDatabase` 的三个子用例；
  - `TestHandoverNodeDeclarationsRoundTripThroughEveryReadPath`；
  - `cmd/parcel-api` 的两条节点真库用例。
- 演示种子本票没有改，评审已在合并树上灌过一遍、退 0，这次没有重灌。
- 没跑全量，也没重生成机制清点。

## 进 main 记录（2026-10-19）

- 59 与 28 合成一批，按 59 → 28 叠到 main 上，整批快进推送，与本记录同一次推送。只有 UC-TF-005 的追加段要合，两边都留。
- 合批种子在一次性新库上 `seed.sh` 灌通，退 0，用完删库。
- 全量 `go test -p 1 -count=1 ./...`（带 55432 DSN）在干净游离检出上钉 `b6e8b7f9`：152 包 ok，0 FAIL，0 SKIP。机制清点在同一 tip 上重生成。
