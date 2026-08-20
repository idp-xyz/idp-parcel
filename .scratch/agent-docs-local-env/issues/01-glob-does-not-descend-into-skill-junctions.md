# Glob 不下降进技能 junction，列目录判断「技能装没装」会得到假阴性

Category: enhancement
Status: resolved

## 现象

`C:\Users\topsx\.cursor\skills` **本身是普通目录**（`Attributes = Directory`，`LinkType` 为空），
它下面的 37 个条目**每一个都是 junction**，各自指向 `D:\tops\idp-skills\skills\<分类>\<名字>`。

在这个布局上，几种探法给出的答案不一致：

| 探法 | 结果 |
|---|---|
| `Get-ChildItem -Force C:\Users\topsx\.cursor\skills` | 37 个条目，**正常列出** |
| `Test-Path ...\skills\implement\SKILL.md` | `True` |
| Read（绝对路径） | 读到全文 |
| **Cursor `Glob` 工具**，`implement/SKILL.md` @ `...\skills` | **`0 files found`** |

Glob 走的目录遍历不下降进 reparse point，因此看不见 junction 里的任何文件；而它**报空不报错**，
调用方拿到的是「这里没有」而不是「这里我进不去」。同一个 Glob 工具在真实目录上正常——
`...\.cursor\projects\d-tops-idp-parcel\mcps` 同样在工作区外，它列得出 15 个 JSON，所以这不是
工作区范围限制，就是 junction 那一格。

## 后果

判断某个技能装没装时，列目录得到空结果会被读成「没装」，进而去重装或改走别的路径。这个假阴性
和已记在文档里的另一个假阴性叠在一起就更难拆：`disable-model-invocation: true` 的技能本来就不
出现在 agent 可用列表里，两条合起来，一个装好好的技能可以在**列表里看不见、列目录也搜不到**，
只有绝对路径 Read 才证明它在。

## 要写什么

`docs/agents/workflow.md` 的 **本机环境 › 当前宿主：Windows + WSL** 一节，**并进已有的那条目录联接
条目**（开头是「`~/.cursor/skills` 下 37 个条目是指向 `D:\tops\idp-skills` 克隆的目录联接」），
不要另起一条。junction 是宿主相关的，放「与宿主无关」那节不对。

要落的事实与措辞要点：

- 点名 **Cursor 的 `Glob` 工具**，不要泛泛写「列目录」——`Get-ChildItem` 在父目录上是好的，
  写成「列目录都不行」是错的，会让下一个人放弃一条可用探法。
- 说清**报空不报错**，这才是它危险的地方。
- 给出可用探法：判断技能装没装一律**绝对路径 Read**（或 `Test-Path`），不靠 Glob。
- 与「与宿主无关」那节里 `disable-model-invocation` 那条的关系点一句（两个假阴性会叠加），
  但**不要重述**那条的内容，它已经写全了。

## 边界

- 只改 `docs/agents/workflow.md` 一个文件，一处。不动 AGENTS.md、不动别的节。
- 不重复文档里已有的事实：junction 指向哪、37 这个数、改技能要回上游改、
  `disable-model-invocation` 看不见不等于没装——**这些都已经在了**。
- 跨文件引用用符号名或引文，不用行号。

## Comments

- 2026-08-20 MCP-1：起因是 MCP-4 报「Glob 不穿越 junction」。协调岗复核后**病因判断要更正**：
  它以为 `~/.cursor/skills` 整个是 junction、且 `Get-ChildItem -Force` 也穿不过去，实测两条都不对
  ——父目录是普通目录，`Get-ChildItem -Force` 列得出全部 37 个。真正成立的只有「Glob 不下降进
  子目录那一层 junction 且报空不报错」。它另一条「看不见不等于没装」属实，但 `workflow.md`
  早已写过，不必再写。按复核后的事实立票。
- 2026-08-20 MCP-1：MCP-2 按票面写入 `workflow.md`（`b3c3fc3`），票 resolved。
