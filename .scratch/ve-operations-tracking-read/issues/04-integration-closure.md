# 04 集成收口(MCP-7,blocked on 03)

Category: enhancement
Status: open

地盘、阻塞边与纪律见 ../spec.md。开工条件:票 03 报出 SHA。

## 活

1. **提交态验证**:临时 worktree 按票 03 的 SHA 干净检出(node_modules junction 借主树的做法沿用你票 01 的配方),pnpm build 绿;后端侧全仓 go test -count=1(真库用例 -v 出示 PASS,若 02 已在其 SHA 出示过且其后无 .go/.sql 变动,可引用其证据并注明)。
2. **规格收口**:本规格 Status: resolved,四票状态核对;报 1 号汇总(SHA 链 + 验证种别:绿是含真库的绿还是未设 DSN 的绿,按 parallel-sessions.md「报绿说清哪种绿」)。

## 完成标准

提交态 build 与测试证据齐全,规格收口,1 号收到汇总。
