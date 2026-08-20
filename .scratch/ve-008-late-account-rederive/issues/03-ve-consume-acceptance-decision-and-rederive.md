# VE 消费接受决定，对该委托名下包裹补派生客户视图

Category: enhancement
Status: resolved
Blocked by: ~~01, 02~~（已全部解除：01=`004e338`、02=`11057dc`，均在 `origin/main`）

~~**未解除阻塞前不要派工、不要开工。** 01 未定命名就写代码 = 绕过领域语言红线；
02 未落地就写消费者 = 手里没有包裹清单。~~ 2026-08-20 两个前置均已合入，阻塞解除。

## 形状（取证已复核于 `0f05304`）

`nrinbox.AcceptedDecisionEventType` **已在装配路由表内**，当前挂单一消费者 `routed`
（NR 初始路由）。本票是给**同一 `EventType` 挂第二个消费者**走 `dispatch.FanOut`，
与收寄 / 揽收 / 交付三类现有形状同形。

**因此不需要新登记事件类型**，ADR-0049 要挡的「无订阅者即显式失败卡分区」在这条路上不触发。

FanOut 顺序按既有铁律：**先 VE 后 PS/NR**，避免投影卡在实例墙上。

## 要做什么

1. VE 侧消费适配器：收到接受决定 → 按（租户 + 委托）取声明包裹清单（02 号票的口）→
   对每件包裹读当前投影 → 有投影则重走客户视图派生。
2. `assemble.go` 把该 `EventType` 从单一消费者改为 FanOut(VE, NR)。
3. 成员循环按 ADR-0066「多对象信封在消费侧按成员拆分」，先例是 `7d3d35c`（CONS-PROJ-DECL-A）。

## 已知形状细节（不阻塞，但要如实处理）

- **信封覆盖接受与拒绝两种走向**（`State` 字段），VE 侧**只应对接受态动作**。
- **账户维必须仍走 PS 反查口，不得改用信封自报的 `CustomerAccountID`。**
  载荷确实带 `CustomerAccountID`，但它回答的是「**这份委托**属于谁」，而客户视图需要的是
  「这件**包裹**此刻唯一归谁」。两者只在无歧义时重合。

  反查口买的不止是查出账户，还有 ADR-0060 的第三格：同一租户下两份当前已接受委托同时声明
  同一包裹时交 `psdomain.ErrAmbiguousParcelTarget`（`current_accepted_parcel_target.go`），
  VE 侧译成 `ErrAmbiguousCustomerAccount`（`customer_account_lookup.go`）落未决。
  `ports.go` 明写「不按时间或行序任选」。

  **绕开它的后果是跨账户泄露**：包裹 P 同时在 R1（C1，已接受）与 R2（C2，已接受）的声明清单里，
  R2 的接受决定到达，成员循环走到 P，若取信封里的 C2 就派生了视图——全程没人发现 R1 也声明了 P，
  而 C2 拿到的可能是属于 C1 的包裹轨迹。这正是 AT-VE-152「不任选候选」与 ADR-0060
  「机制拒绝自动采认」要挡的事。

  **信封的 `CustomerAccountID` 改作一致性校验用**：反查口是账户维的权威，信封值只作比对。
  **不匹配一律硬失败。** 已取证「反查到了且账户不同」这一格在基线上**结构上不可达**，
  实现时只需重验两道闸仍在，不必再纠结跳过与否：

  - 已接受的委托**撤不了也换不了代**：`WithdrawByCustomer`、`FormNewSubmissionVersion`、
    `RejectByAuthority` 开头都是 `if request.decisionFormed`，而接受本身就把该标志置真
    （`withdrawal.go` 的注释写明三者「共用同一个 `decisionFormed` 闸门」）。
  - 已接受后唯一开着的 `AmendCustomerSourceData` **不动成员也不动账户**：它只往
    `sourceDataVersions` 追加，且以 `ErrParcelOutsideAcceptanceBaseline` 挡住越过接受基线的范围。
  - 于是走完剩下的路：命中行就是本委托时账户必然相等（账户属来源身份四维，随委托不变）；
    命中行是另一份委托时，本委托仍已接受且仍声明该包裹 → 查询命中两行 →
    `ErrAmbiguousParcelTarget`，**根本走不到比对**。

  ⚠️ 该论证只覆盖**领域转换**。绕过领域直写库的迁移或人工 SQL 不在其内——但那本就是不变量已破，
  同样归硬失败，结论不变。
- **常态下本消费者空转**：正常序（先接受委托、后包裹流转）时按包裹读不到当前投影，什么也不做。
  只有在「末次源事实已入账、其后才建立委托」的倒序里才真正干活。
- **NR 腿撞实例墙不变**：该 FanOut 的 NR 腿本来就撞 `ROUTE_EVIDENCE_NOT_CONFIGURED`。按 01 号
  外部评审票的分格修复（`a097d7f`，仅全路未决才记 `consumer_undecided`），VE 腿成功不改变 NR 腿的
  未决记账，但 VE 腿硬失败会把整格升成硬失败——**这是正确行为**，既有 assemble 级断言要如实更新，
  不要为了让旧断言绿而改行为。

## 首发不覆盖（用户拍板 Q2，写进票面而不是默默漏掉）

账户维取得回还有第二种成因：**委托早已接受、包裹经新提交版本才成为其成员**。那条链走 ADR-0066
的提交版本形成，不是接受决定。**首发不接**，因为它需要 PS 侧给出成员差集语义，牵动面大出一截。
列为已知未覆盖。

## 硬约束

- `assemble.go` 是共享接线。**本票占用它期间不得再派第二张碰它的票。**
- 不发明生产默认；实例墙保持诚实。中文注释。
- 隔离 worktree 从 `origin/main` 长出。只 `git add` 本票文件。
- 跑测试按包路径，不要 `-run "关键字"`。

## Comments

- 2026-08-20 MCP-1：用户拍板路线 a + Q2「首发只接 acceptance-decision.formed」后立票，置 draft 待前置。
- 2026-08-20 MCP-1：前置 01/02 均已合入 `origin/main`（tip=`11057dc`），置 ready-for-agent 并派 MCP-4。
  02 号票落地口：`ports.CurrentDeclaredParcelsView.FindCurrentDeclaredParcels`（按租户+委托答
  清单/found/err，零行答未找到）。本票占用 `assemble.go` 期间不再派第二张碰它的票。
- 2026-08-20 MCP-1：**MCP-4 会话死于提交之前**，实现整套只存在于隔离树工作区文件里。协调岗
  已在 `ve008-accept-rederive` 上提交保命，SHA **`a771bc3`**（父 `11057dc`，8 文件 1280 增 18 删）。
  已复核实现守住票面账户维硬约束：走 `veports.ParcelCustomerAccountView`，信封 `CustomerAccountID`
  只作比对、不匹配交 `ErrCustomerAccountMismatch` 硬失败，`ErrAmbiguousCustomerAccount` 原样上抛
  归未决；`assemble.go` 接 `FanOut(VE, NR)` 先 VE 后 NR。
  ~~**`a771bc3` 未过集成门禁**——只跑过 gofmt / build / vet，含 PG 全量套件未跑，不得当已验证 SHA 推 main。~~
- 2026-08-20 MCP-1：**已验绿并合入，置 resolved**。detached 验证树跑全门禁：`gofmt` 无输出、
  `go build ./...` = 0、`go vet ./...` = 0、`git diff --check` = 0、设 DSN 后
  `go test -p 1 -count=1 ./...` **TEST_EXIT=0（含 PG，约 220 秒，全包 ok 无 FAIL）**。
  `git push origin a771bc3:main` 快进成功（`11057dc..a771bc3`），验证树已拆。
  票面预告的「VE 腿硬失败把整格升成硬失败、既有 assemble 级断言要如实更新」没有触发额外红——
  `cmd/parcel-dispatch` 与 `internal/platform/dispatch` 均一次过，说明断言在实现里已一并改到位。
  `assemble.go` 占号就此释放。
