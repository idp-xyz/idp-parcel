# templates 目录盲覆盖事故封存（2026-08-24）

## 经过（证据，不是结论）

- 10:27 MCP-2 把「UI阶段1C 三页面模板」派给 7 号（task-1b88252c）。
- 10:29 用户告知「7 号通道不存在，改派 6 号」；MCP-2 将 7 号任务标 failed 并改派 6 号（task-12112b38）。
- 实际上 7 号存在且已消费原任务。7 号与 6 号随后并行写同一目录 `apps/admin-web/src/templates/`，互不知情。
- 按 mtime 与 import 关系核实：最终盘面是 6 号的自洽整套（三模板 + state-slot.tsx + types.ts + demo.ts + index.ts，全部互引）；7 号的三个模板文件被 6 号后写覆盖（两者从未提交，无从恢复），仅存两个无引用孤儿。

## 本目录内容

7 号交付中幸存的两个原件，封存原样、一字未改：

- `view-state.tsx`（mtime 10:38:21）—— 7 号的四态闸 ViewStateGate 设计
- `shared.tsx`（mtime 10:38:46）—— 7 号的共享类型与状态徽章辅助（含与 6 号 types.ts 同名异形的 AuditEntry）

从 `apps/admin-web/src/templates/` 移除的理由：与 6 号在位整套功能重叠（双套四态闸、AuditEntry 同名异形），违反单一权威；两文件无任何引用方。

## 教训指向

docs/agents/parallel-sessions.md「先分地盘，这一步只有人能做」与「盲覆盖：报告防不住它」。本次事故的根因是把「通道是否存在」当成了无需取证的背景设定——改派前用 list_sessions 查一次就能拦住。
