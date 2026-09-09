# 规范化版本报出口零消费者：版本已随摘要前缀同行，真渠道 Intake 也不需要它

Category: chore
Status: in-progress——2026-09-09 17:1x，MCP-6（task 2a4dfaba，通道 1 派；分支 `mcp6-wbr02` 基 origin/main `3c9a41bd`）按「取 (b) 删」落地。此前 ready-for-agent——二选一由通道 1 代裁（2026-09-09，用户授权自决）：**取 (b) 删**。理由：基线名单是「有生产代码、无生产调用方」的清单，一个明知
今天没有分支可走的导出函数留在名单上，等于让名单替它记「将来会有人用」——那是注释的活，不是名单的活；PSC-2 那笔工作本来就要读这个文件，
`payload_canonicalization.go` 里那句「届时重新导出版本出口」就是提醒。完成判据照下「取 (b) 删」一条逐字做。此前 draft——只读取证（MCP-6，锚 `2efef58e`），PS 地盘归 MCP-2
Blocked by: 无（二选一都是 PS 地盘内的小改，不等任何上游）

## 条目

`internal/parcelshipment/domain CurrentPayloadCanonicalizationVersion`（`payload_canonicalization.go`）。基线理由行：「版本报出口留在名单上：至今零生产调用点，接它的仍是真渠道那笔工作」。

## 它是什么

`CurrentPayloadCanonicalizationVersion()` 返回常量 `payloadCanonicalizationVersion`（此刻 `PSC-1`），注释「按其他取值记录的摘要，本构建无法重算」。同文件 `CanonicalizeSubmissionPayload` 产出的摘要串**自带版本前缀**（`PSC-1:<sha256>`），其注释写明「前缀让版本随既有存储同行，零迁移」，并把「跨版本的比较纪律（版本不同不是冲突、回放按原版本重新规范化）」留给「引入 PSC-2 的那笔工作按前缀取版本再分支」。

## 今天的样子

- 摘要函数已接：隔离 Intake（`adapters/http/isolated_write_intake.go`）调 `CanonicalizeSubmissionPayload`；ADR-0091 把摘要函数剪出了名单。
- 版本出口全仓非测试零引用。`PayloadDigest` 上没有按前缀取版本的方法；没有任何比较两枚摘要的代码按版本分支——今天只有一个版本，那条分支没有代码可走。
- **基线那句「接它的仍是真渠道那笔工作」取证不支持**：真渠道 Intake 与隔离 Intake 走同一个 `CanonicalizeSubmissionPayload`，版本已经在摘要串里，Intake 没有理由再单独问一次版本。PAR-INT-01 供的是渠道字段词表，与版本出口无关。

## 该有的调用方

**引入 PSC-2 那笔工作里的重放分类分支**：比较存量摘要与新算摘要之前先按前缀取版本，版本不同即「不是冲突、按原版本重新规范化」（ADR-0014），那一步要问「本构建支持哪一版 / 能不能按记录的那一版重算」——这才是这个出口的用处。另一个可能的消费方是治理读面（报出当前规范化版本供核对），今天没有这样的读面。

**哪张 UC 哪一步**：`UC-PS-001` 应用流程步 2「接入适配器 / 产品接入处理——按当前接入规则识别首次提交、重复、重试或内容冲突」，也就是结果行「已有结果」（相同逻辑请求身份、相同规范化内容摘要）与「接入冲突」（相同身份、摘要不一致）之间那道分类；「一致性、幂等与并发」一节把判据写成硬句。按前缀取版本再分支就落在这一步里：版本相同才比摘要，版本不同不是冲突。今天代码里这一步是 `domain/source_submission.go` 的 `ClassifySourceSubmission`（同身份同摘要 `SourceReplay`、同身份不同摘要 `SourceConflict`），比的是整串摘要，没有取版本那一格——因为只有一版；PSC-2 引入时它就是要长出版本分支的那个函数，届时才有人问版本出口。

## 三分

两选一，交 PS 地盘裁，**不建议维持现状那句「等真渠道」**：

- **(b) 删**：函数体一行，PSC-2 落地时随分支一起回来；基线少一条，`payloadCanonicalizationVersion` 常量留在包内继续被 `CanonicalizeSubmissionPayload` 用。代价：PSC-2 那天要记得重新导出——那笔工作本来就要读这个文件，漏掉的概率低。
- **(c) 留待**：理由行改写为「调用方是引入 PSC-2 时的重放分类分支（按前缀取版本再分支，ADR-0014），那一层随 PSC-2 落；真渠道 Intake 不是它的调用方」。代价：一条明知今天没有分支可走的条目继续躺在名单里。

两边都比现状诚实。若取 (b)，`decimal.go` 那段「同文件已有正主时别再开只做转发的构造器」的教训在这里不适用——它不是转发，是零消费者。

## 能不能归到已认可的留待

不能，也不需要：它缺的不是实例值，是第二个规范化版本。

## 完成判据（二选一，落地那笔连理由行一起改；MCP-1 2026-09-07 裁）

- **取 (b) 删**：删 `CurrentPayloadCanonicalizationVersion` 与只为它写的测试，`payloadCanonicalizationVersion` 常量留在包内；`payload_canonicalization.go` 那段「引入 PSC-2 的那笔工作按前缀取版本再分支」旁加一句「届时重新导出版本出口」；剪基线行，头注记一句成因第一种（已删），在自己那笔的干净检出上两法同得记数、钉 SHA。
- **取 (c) 留**：条目不动，理由行整段换成：

  > 规范化版本报出口。**调用方是引入 PSC-2 那笔工作里的重放分类分支**——`ClassifySourceSubmission` 按前缀取版本再分支（版本相同才比摘要，版本不同不是冲突，ADR-0014；UC-PS-001 步 2），那一层随 PSC-2 落地那天这一条出名单。真渠道 Intake 不是它的调用方：版本已随 `PSC-1:` 前缀在摘要串里同行。

  两条路都不许保留今天那句「接它的仍是真渠道那笔工作」。

## 边界

本票不改代码、不改基线；PSC-2 何时引入不归本票（寄收件范围与服务要求拿到自己的领域模型那天，见 `payloadCanonicalizationVersion` 的注释）。
