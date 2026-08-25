# 04 集成收口(MCP-7,blocked on 03)

Category: enhancement
Status: resolved

地盘、阻塞边与纪律见 ../spec.md。开工条件:票 03 报出 SHA(已满足:ef6e154)。

接手声明:原派 MCP-7;票 03 由 1 号按用户 2026-08-25 队列指示接手并收口,本票
同一理由续接,占号广播已随票 03 释号一并发出(2026-08-25)。

## 活

1. **提交态验证**:临时 worktree 按票 03 的 SHA 干净检出(node_modules junction 借主树的做法沿用你票 01 的配方),pnpm build 绿;后端侧全仓 go test -count=1(真库用例 -v 出示 PASS,若 02 已在其 SHA 出示过且其后无 .go/.sql 变动,可引用其证据并注明)。
2. **规格收口**:本规格 Status: resolved,四票状态核对;报 1 号汇总(SHA 链 + 验证种别:绿是含真库的绿还是未设 DSN 的绿,按 parallel-sessions.md「报绿说清哪种绿」)。

## 完成标准

提交态 build 与测试证据齐全,规格收口,1 号收到汇总。

## 决议

- 提交态验证(2026-08-25 10:5X,临时 worktree 干净检出 ef6e154,node_modules 以
  junction 借主树——票 01 配方):`pnpm build`(tsc -b && vite build)绿,2689 模块;
  vendor 块体积告警系上游摇树问题,票 01 收口已记,非本轮引入。验证树按纪律拆除:
  卸 junction(rmdir 不递归目标)→ 删本次 build 产物 dist → worktree remove 不加
  --force → prune → Test-Path 无壳 → worktree list 复核登记已摘,主树 node_modules
  完好。
- 后端侧引用票 02 证据并注明:git log --name-only 267cb44..ef6e154 仅 6 份前端文件
  + 2 份 scratch md,零 .go/.sql 变动,故票 02 在 267cb44 出示的「含真库的绿」
  (全仓 go test -count=1 全 ok、ListCurrent 真库用例 -v PASS 非 SKIP)对本链仍然
  有效,本票未重跑。
- 规格收口:四票全 resolved(01 c6c7914/ADR-0076、02 267cb44、03 ef6e154、04 本票),
  spec Status: resolved。
- SHA 链:c6c7914 → 267cb44 → ef6e154(簿记 be793b8 及本收口)。绿的种别:前端为
  提交态 build 绿;后端为票 02 的含真库绿(本票以零 .go/.sql 变动引用,未重跑)。
- 1 号即本施工通道,汇总经 IDP 队列 reply 呈报。
