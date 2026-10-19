# 78 ADR 索引里 ADR-0206、0207、0209、0210 也各登了两行

Category: bug
Status: needs-triage——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md) 落 ADR-0206 前向指针时所见。
Blocked by: 无
出处：票 59 完成记录「偏离与待裁」。同类的 ADR-0205 由票 74 立（在分支 `claude-dcg68` 上，尚未进 main）。

## 现象

在 `2c6311f1` 上，`docs/adr/README.md` 里 ADR-0205、0206、0207、0209、0210 各有两行条目，链接与标题相同，摘要措辞不尽相同。命令：`grep -o "^- \[ADR-0[0-9]*" docs/adr/README.md | sort | uniq -d`。

票 59 给 ADR-0206 写「挂法已答」时，两行都追加了同一段后缀，不让其中一行无声变旧；没有删任何一行。

## 三类

机制（文档索引卫生）。规模 XS。

## 最小的诚实修法

与票 74 合并处理：逐对核两行，留摘要更全、合「一决策一处定义」的那一行，删另一行，不重排其他行；删前确认留下的那行没有漏掉被删那行独有的内容。`docs/adr/README.md` 是跨票热点，进 main 时单独一笔提交，避开同时在途、往 README 追加行的分支。

## 要裁的

无。
