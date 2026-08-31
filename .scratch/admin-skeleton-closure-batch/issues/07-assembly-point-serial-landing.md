# 批务：装配点串行落地与批面收口

Category: chore
Status: resolved——MCP-1（批收口注记见 Comments；MCP-3/4/5 会话 crash 后 02/03/05/06 票面代结同笔）
Blocked by: 02, 03, 04, 05, 06（各票阶段一交活即可逐笔落地，不必等齐）

## 为什么要有这一票

票 02–06 都要往 `cmd/parcel-api` 的装配四件（`endpoints.go`、`assemble_isolated_read.go`、
`main.go`、`unwired_orchestration.go`）加行。本仓 2026-08-13 就是在这类共享接线文件上连断远端
构建两次：一次 `add` 卷带（暂存时树里混着另一会话未提交的接线，远端在不存在的嵌入目录上编译
失败），一次提交竞态（两个会话相隔 35 秒各自提交同一对文件，后者的暂存快照取自前者落地之前，
`git commit` 不做合并，直接把先落地那份接线剥掉了）。

**逐块核防的是「卷走别人的」，防不了「盖掉别人刚提交的」——后者只有占号能防**，因为竞态窗口
在暂存与提交之间，任何对树的检查都看不见别人 35 秒后才落地的提交。

所以本票**永久占号装配四件**：各线不自改，交活给 MCP-1 串行落地。这也用上了「验证可以外包」
——验的是一个不动的对象（已验 SHA），不必由推的人亲自验。

## 做什么

对每一份交活：

1. **收活**：核对该线交出的**已验 SHA**、要装配的端点行、以及「自上一个已验 SHA 以来动没动过
   `.go`/`.sql`」这一报。
2. **落装配**：把端点接进 `endpoints.go` 装配表，隔离读放行面与 `unwired` 占位读口同笔跟上
   ——**四处漏一处各有测试点名**（装配漏挂答 404、启用态仍 403、放行面失配），这是既有纪律，
   照 `/collection-subledgers` 那一笔的形状办。
3. **验提交态**：临时 worktree 检出该 SHA（验的是提交态，不含任何人未提交改动），跑
   `go build`、`go vet`、`go test -count=1 ./...`，DSN 设好并**单跑一个门禁用例看 `PASS` 还是
   `SKIP`** 以证明这一跑含真库。拆树**不加 `--force`**。
4. **推已验 SHA**：`git push origin <已验 SHA>:main`，不推会动的分支名。推之前**贴着 push**
   跑一次 `git log --oneline <上次已推 SHA>..<X>` 看这一笔底下压着谁的提交——有别人的就先问。
   远端状态一律走 `git ls-remote`，不读本地缓存的 `origin/main`。
5. **广播**：在频道播「某端点已装配、SHA 为某某」，该线据此进入阶段二（页接真 + `liveIds`）。

## 收口

全部落地后：核对 `liveIds` 与实际发请求的页两向一致（既无「登了 live 却不发请求」，也无「发
请求却没登」——后者正是 `65b6cf2` 之前 `cod-ledger` 那一笔的形状）；跑一遍前端调用路径与
`endpoints.go` 装配表的逐条对齐；更新本批 spec 的 Status。

## 完成判据

票 02–06 全部 resolved；工作台「页面骨架」一档降至 1（仅 `label-transactions`，其建模属票 08）；
`liveIds` 两向零漂移；含真库全仓绿（注明）；远端 `main` 与本地一致（`ls-remote` 证）。

## Comments

### 批收口（MCP-1，2026-08-31）

装配落地按到达序逐笔完成：治理 `/governance-registers`、计价 `/pricing-evaluations`、网络
`/route-plans`、结算四口、作业 `/node-operations-records`、履约 `/transport-fulfillment-records`、
VE 案件侧三口（`1f5d7ae`）。三线会话（MCP-3/4/5）先后 crash，余量（票 05/06 阶段二落地与
02/03/05/06 票面状态）由 MCP-1 审查后代结——06 阶段二审查证据在该票 Comments。

收口两审：

1. **`liveIds` 两向一致**：35 个 live id 逐一对到发真请求的页（`service-areas` 经
   `/network-catalog?family=…` 供数）；反向无「发请求却没登」——不发请求的仅
   `acceptance-review` 与 `label-transactions`，均如实不登。
2. **前端调用路径与 `endpoints.go` 逐条对齐**：管理台 30 条调用路径全部在装配表；装配表多出的
   六条（`/node-operations/receptions`、`/transport-fulfillment/deliveries` 与
   `/delivery-proof-corrections`、`/customer-tracking-view`、`/claims`、`/customs/external-results`）
   为命令面或客户面端点，不属管理台页，零漂移成立。

提交态验证（`255db1e`）：`go build ./...`、`go vet ./...`、`gofmt -l` 零信号；含真库全仓
`go test -p 1 -count=1 ./...` 绿（DSN 55432 实跑；门禁单跑
`TestCaseReviewListsSignalEpisodesWithConclusions` 见 `PASS` 非 `SKIP`）；`apps/admin-web`
`pnpm build`（`tsc -b` + vite）提交态绿。

工作台骨架档收至 **2**：`label-transactions`（建模属票 08，draft 待裁）与 `acceptance-review`
（批面范围裁定明写不入批——`internal/parcelshipment` 开批时有他会话在途改动）。完成判据句
「降至 1」与范围裁定冲突，以范围裁定为准；`acceptance-review` 接线另立票时再降。

推送：本笔簿记为批尾，`git ls-remote` 复核远端后推 `origin/main`（推送结果以远端为证）。