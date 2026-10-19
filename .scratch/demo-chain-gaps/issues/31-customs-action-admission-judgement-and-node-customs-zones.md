# 31 关务动作准入判断与节点关务区域

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dcg31。2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md) 与 [ADR-0206](../../../docs/adr/0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md)。
Blocked by: 无
地盘：CC 新 `application/judge_action_admission.go`；ports 新文件（对象→范围/程序、节点区域）；`domain` 新 `node_customs_zone.go`；postgres 新册 + 迁移 CC 新号；CC http 登记口；`cmd/parcel-customs-register` 新 kind；`cmd/parcel-api` 装配 + `endpoints.go` 行；种子 customs 四个节点区域；ADR-0206、CC CONTEXT、UC-CC、CONTEXT-MAP。
出处：[05](./05-movement-facts-are-unlinked-and-departure-gate-is-self-reported.md)；方案「05 移动事实与出发门禁」D05-2、D05-3、D05-4、D05-5，「跨票通则」X3，「需要的 ADR」ADR-0206，「已不成立或已修」05。

## 做什么

- CC 自有「动作准入判断」查询，请求驱动；输入与三源折叠照方案 D05-3 与「需要的 ADR」ADR-0206。
- 适用动作由 CC 按节点关务区域定，未登记节点的答法照方案 D05-5；不采纳「每次出发都问两种」。
- 新登记册「节点关务区域」，值属租户取值，`parcel-customs-register` 加 kind；演示种子登 SYN 节点区域。
- 对象到范围与程序由 CC 解析，解析不到答未决；演示停在 scope 一级（方案 D05-4）。
- 判断记录可重放，不入队（方案 X3）；现有 `VerifyReleaseGate` 与 `gate-verification.recorded` 不变。
- 起草 ADR-0206；CC CONTEXT、UC-CC 准入判断、CONTEXT-MAP 的 TF→CC 拉取边照方案「随票同步的文档」。
- 进 main 的令牌次序与波次见方案「并行车道」。

## 完成判据

- 按区域定适用动作。
- 三源折叠成封闭答复，门禁满足不答放行。
- 节点未登记答 `未配置`。
- 判断记录可重放、不入队。
- 真库测试覆盖放行 / 阻断 / 不受门禁 / 未决 / 未配置。
- 演示：SHA→SZX 只适用 `LOADING_DEPARTURE`；SZX→SIN 另加 `CROSS_CUSTOMS_MOVEMENT`。

## 规模

L

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg31`）

### 接票时核实

- 出处 05 的现象逐条成立。`NewVerifyReleaseGateHandler` 在 `cmd/` 零引用；限制管理 `ManageRestrictionHandler` 同样没有生产入口。CC 没有任何东西说一个网络节点在哪个关务区域。
- CC 已有的「动作准入」只折限制一源（`domain.JudgeActionAdmissibility`），按「此刻」判，不按动作时点。
- 演示门禁目录只登了 `SYN-SCOPE-01` 上出口程序的 `CROSS_CUSTOMS_MOVEMENT`（认定满足）与 `FINAL_DELIVERY`（空清单）。演示包裹由 `submit-one-shipment.sh` 按时间戳现取标识，不在任何关务案件与申报单元里（逐件建案等 psb/18）。

### 代决（用户授权「你作为业务和系统专家自决……解决所有的问题」）

权威落在 ADR-0206。这里只记方案没裁、由本票自决的几处，以及与方案字面的偏离：

1. **范围即申报单元。** 对象→范围按 CC 自己的申报单元与案件册解析：当前单元（被替代的不算）即门禁与放行所说的「明确申报范围」，程序取单元行的。在案而未成单元的，单独记下那一案的程序。限制另按对象自身读，因为内部限制可以直接落在包裹上。
2. **细化 D05-4「解析不到答未决」。** 对象不在任何案件与单元里时：
   - 跨区移动答未决（缺案可依）；
   - 同区动作答不受门禁。

   方案字面是一律未决，那样演示与所有无案的境内段每一步都读成未决，与票 32 的演示判据「SHA→SZX 读数不受门禁」自相矛盾。在案未成单元仍答未决。
3. **D05-5「该对象有跨区门禁目录行」的落法。** 某当前单元登了跨区那一道的目录、且那一道判下来不是不受门禁，或有动作时点有效的限制约束跨区移动，才答未配置。只登了空清单的跨区目录不算。
4. **三源折法的细格**（方案只说「门禁满足不答放行；外部放行不解内部限制」）：
   - 全部放行才放行；
   - 只有部分或附条件放行答未决，与 ADR-0197 一致，部分放行不等于放行；
   - 门禁满足而没有放行答阻断；
   - 门禁未满足、部分满足、冲突答阻断，但已有全部放行时答未决；
   - 同层冲突的放行不算；
   - 限制与放行都按动作时点读，放行取 CC 保存的业务发生或适用时间。
5. **压制次序。** 阻断 > 未配置 > 未决 > 放行 > 不受门禁。依赖读不动不进五格：应用层答`读不动`（`ActionAdmissionUnreadable`）并指名哪一口，供票 32 译成它的「读不动」格。
6. **门禁读法共用。** 从 `VerifyReleaseGate.Handle` 抽成 `assessReleaseGate`，两条编排共用，核对编排答复一格不变（既有用例原样绿）。准入判断不调核对编排：那会写门禁核对册、入队没有订阅者的 `gate-verification.recorded`。
7. **判断记录落库。** 按内容派生标识 `AADJ-…`（前缀进前缀门禁），只建立不更新、不入队；重放命中同一行，判断时刻取首次。方案只写「可重放」，没说记不记；准入判断针对明确对象和动作，事后要答得出「当时凭什么」，所以记。
8. **意图三种。** 出发（起讫必填）、交出（去向可缺，给了才问跨区）、交付。交出映射为出库 `OUTBOUND_RELEASE`。接收不受门禁，所以没有「接收」意图。
9. **登记面命名与入口。** 端点 `/customs-node-zone-registrations` 与 CLI `node-zone` 同词，挂操作者渠道登记册 Intake。参数登记册立 `PAR-CUS-08`，并把首发判据句的 `PAR-CUS-01` 至 `07` 改到 `08`。

### 越权风险点（全文见 ADR-0206「越权风险点」）

1. 限制今天没有生产写入，阻断只可能来自门禁或外部结果（CC owner）。
2. 区域粒度按节点，节点内监管区与非监管区装不下（CC owner、租户）。
3. 不在册对象的同区动作答不受门禁：要求建案而未建案的包裹，境内段放过（CC owner）。这是对方案 D05-4 的细化。
4. 演示包裹不在册，演示动线上不会出现放行；放行、阻断、未配置只在真库用例里出现（演示租户）。
5. 门禁目录以申报单元为范围，租户要按单元登目录（CC owner、租户）。
6. 部分与附条件放行答未决（CC owner）。
7. 门禁未满足而已全部放行答未决（CC owner）。
8. 对象只认包裹；袋与集运单元由消费方展开（TF owner，票 32）。
9. 交出映射为出库、给了去向才问跨区；交接登记的挂法等 peer（TF owner）。
10. 门禁目录没有历史区间，离线补登时读的是此刻的目录（CC owner）。
11. 参数登记册首发判据从 `PAR-CUS-07` 延到 `08`：新增一项上线前须确认的租户取值（产品 owner）。

### 做了什么

- 领域：
  - `domain/node_customs_zone.go`：节点与区域引用、动作意图三种、`DetermineGuardedActions`。
  - `domain/action_admission.go`：五格、依据种类、`JudgeActionAdmission`。
  - `domain/regulatory_restriction.go`：加 `InForceAt` 与 `JudgeActionAdmissibilityAt`，与原判断共用一份折法。
- 应用：`application/judge_action_admission.go`、`register_node_customs_zone.go`；`verify_release_gate.go` 抽出 `assessReleaseGate`。
- 端口：新文件 `ports/action_admission.go`，含区域写读两口、对象范围、限制按范围、放行读口、判断册、判断标识派生。
- 持久化：迁移 `migrations/customs_compliance/0024_node_customs_zone_and_action_admission.sql`，含区域册（排他约束）、判断册（词表 CHECK）、`release_outcome` 按范围索引。postgres 适配器四个新文件。
- 登记面：
  - `registrationjson/node_customs_zone.go`；
  - customshttp `register_node_customs_zone.go`；操作者登记册 Intake 与未配置 Intake 各补一口；
  - `cmd/parcel-customs-register` 新命令 `node-zone`。
- 装配：新文件 `cmd/parcel-api/assemble_customs_admission.go`，含区域登记事务壳与准入判断事务壳（自带事务，供票 32 的消费侧端口直接接）。`endpoints.go` 与 `main.go` 只追加一参一行，`endpoints_test.go` 追加探针一行与占位一参。
- 种子：`scripts/demo-seeds/data/customs/27–30-node-zone-*.json` 四个节点区域；`seed.sh` 关务一节末尾追加四行；README 补册数与数据故事一段。
- 文档：
  - ADR-0206；`docs/adr/README.md` 追加一行；
  - CC CONTEXT 两个词条、「动作准入判断」一节、拥有一项；
  - UC-CC-013（`AT-CC-411`–`425`），`docs/application/README.md` 一行；
  - CONTEXT-MAP 补 TF→CC 拉取一条与 CC 拥有一项；
  - GLOSSARY 两条；
  - `PAR-CUS-08`。

### 用例

- 领域（替身无关）`action_admission_test.go`、`node_customs_zone_test.go`：
  - SHA→SZX 只适用装载出发，SZX→SIN 另加跨区，终点未登记跨区待定；
  - 交出、交付各自的动作；读数缺或多是编程错误；
  - 不在册对象同区不受门禁、跨区未决；
  - 门禁满足仅全部放行才放行，动作时点之后的放行不算，部分 / 附条件未决；
  - 门禁未满足阻断，已全部放行则未决；
  - 限制压过放行、按动作时点判；
  - 门禁各停点归未配置或未决；在案未成单元未决；
  - 跨区待定只在对象带跨区门时答未配置；整次答复取最重；缺门禁读数报错。
- 应用（替身）`judge_action_admission_test.go`：
  - 记一次、重放回同一份；来源一变换标识另记；
  - 门禁停点译成读数；各依赖读不动停在读不动并指名；
  - 未受理；构造门拒 nil。
- 真库 `adapters/postgres/action_admission_test.go`：
  - 演示四节点区域下 SHA→SZX / SZX→SIN 的适用动作；
  - 放行、阻断（未取得放行、限制压过放行）、不受门禁、未决（不在册跨区、只有部分放行且同层冲突的全部放行不算）、未配置（目录未登记、终点未登记区域）；
  - 重放回同一行且 Outbox 不增；判断册词表 CHECK；两只写口无事务负向证据。
- 真库 `node_customs_zone_registry_test.go`：登记、重放、同起点换区域冲突、换版落终点、起点落进已闭合区间冲突、按时点读回。
- 真库 `cmd/parcel-customs-register/node_zone_test.go`：读演示种子文件原样灌入并重放，再用生产准入编排钉演示判据。
- 真库 `cmd/parcel-api/assemble_customs_admission_test.go`：登记口挂操作者渠道（无令牌 401、自报租户 400）；生产装配两格对真库走通，准入判断自带事务、重放回同一份。
- HTTP 与译装：`register_node_customs_zone_test.go`、`registrationjson/node_customs_zone_test.go`。

### 验证

- 于 `932f03c4`：`go build ./...`、`go vet ./...`、`gofmt -l`（全部已跟踪 Go 文件）干净。
- `IDP_PARCEL_POSTGRES_DSN=… go test -p 1 -count=1` 跑过下列包，全绿：
  - `./internal/customscompliance/...`、`./cmd/parcel-api/`、`./cmd/parcel-customs-register/`；
  - `./internal/architecture/`（接线、可达性、前缀、PBC-08 门禁）、`./migrations/`、`./scripts/demo-seeds/...`；
  - 另跑 NR、PS、SA 三个消费 CC 的适配器包，以及 `./cmd/parcel-dispatch/` 中 `Customs|ExternalResult` 用例。
- 一次性库 `dcg31_seed` 上 `seed.sh` 全程退 0，四个节点区域在册，用完已 DROP。
- 未跑全量，未重生成机制清点。

### 偏离与待裁

- 偏离方案字面一处：D05-4「解析不到答未决」细化为「跨区未决、同区不受门禁」（见代决 2）。
- 准入判断今天在 `cmd/parcel-api` 只装好、没有生产消费者。消费者是票 32 的 `DepartureGateView` 消费侧适配器：接 `customsAdmission.actionAdmission`，把 `ActionAdmissionUnreadable` 译成它的「读不动」，未配置格怎样落进它的五格读数由票 32 定。
- 交接（外包干线）与有效交付的门禁按交出、交付意图读本判断，挂法等 peer，见方案「等 peer 的」。
- 管理台没有节点关务区域的查阅与登记页（`apps/admin-web` 是前端切片），读面端点也没开。管理台要列这本册时，另立读口（`ports` 加一口目录上列、`/customs-ports-paths` 那样的查阅端点）。
- 中间提交 `1923c740` 的 CLI 用例读的种子文件在 `c531879e` 才入库，单拣前一笔时该用例会红；分支头一致。
- 进 main 时：ADR 号、AT 号（`AT-CC-411`–`425`）、`PAR-CUS-08`、迁移号 `0024` 若被占，全仓搜替。热点文件（`endpoints.go`、`main.go`、`endpoints_test.go`、`seed.sh`、README、`docs/adr/README.md`）都只追加，按方案「并行车道」装配令牌与种子令牌的次序 rebase。

