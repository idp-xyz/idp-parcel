# 规范化版本报出口零消费者：版本已随摘要前缀同行，真渠道 Intake 也不需要它

Category: chore
Status: resolved——2026-09-09 17:4x，MCP-6（task 2a4dfaba，通道 1 派；分支 `mcp6-wbr02` 基 origin/main `3c9a41bd`，笔 `e38bc18d` / `eecd05d4`）：`CurrentPayloadCanonicalizationVersion` 删去，唯一测试引用改钉 `PSC-1:` 字面量（没有只为它写的测试），`CanonicalizeSubmissionPayload` 注释记「届时重新导出版本出口」，基线行剪掉、PS 组理由行改历史注、头注记成因第一种并钉 SHA 两法同得 4→3；完成记录见 Comments；main 上的 SHA 由推送方补。此前 in-progress（17:1x 认领）；此前 ready-for-agent——二选一由通道 1 代裁（2026-09-09，用户授权自决）：**取 (b) 删**。理由：基线名单是「有生产代码、无生产调用方」的清单，一个明知
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

本票不改代码、不改基线；PSC-2 何时引入不归本票（寄收件范围与服务要求拿到自己的领域模型那天，见 `payloadCanonicalizationVersion` 的注释）。（立票时的边界；落地笔见 Comments。）

## Comments

- 2026-09-09 17:4x · MCP-6（task 2a4dfaba，通道 1 派；分支 `mcp6-wbr02` 基 origin/main `3c9a41bd`）：
  **完成记录，转 resolved（取 (b) 删，成因第一种）。** 逐笔：`e38bc18d` 认领（只改票面 Status）；`eecd05d4` 删函数 + 钉测试 + 剪基线。
  完成判据「取 (b) 删」逐项：① 删 `domain.CurrentPayloadCanonicalizationVersion` 与其注释（`payload_canonicalization.go`）；
  `payloadCanonicalizationVersion` 常量留在包内，`CanonicalizeSubmissionPayload` 继续用它。**没有只为它写的测试**：全仓唯一测试引用是
  `TestCanonicalizeSubmissionPayloadIsStableAndCarriesItsVersion` 用它取前缀，那条证的是「摘要携带产生它的规范化版本」（ADR-0014），
  属摘要函数不属出口，所以不删、改钉 `PSC-1:` 字面量并注明理由（版本一换这里就该红，是引入 PSC-2 那笔要看见的信号，不是要绕开的耦合）。
  ② `CanonicalizeSubmissionPayload` 注释「引入 PSC-2 的那笔工作按前缀取版本再分支」旁加「届时重新导出版本出口」，并写明为什么今天不留
  一个只等将来的导出（无行号无计数）。③ 剪基线行；PS 组理由行改成历史注，「接它的仍是真渠道那笔工作」那句按票面要求不再保留；头注记
  成因第一种。**取证两法**：`git grep` 全仓非测试 `.go` 只此一处声明、零调用；删后 `go build ./...` 与 `go vet ./...` 退 0（vet 连测试
  一起编）。**记数带「在哪量的」**：量在隔离 worktree（动手前 `git status -- internal/architecture/` 为空），本笔单独作用于父提交
  `e38bc18d`（其基线 blob `49aaff84` 与 `3c9a41bd` 的逐字节相同）上，两法（UTF-8 逐行滤非空非注释；字节层数行首非 `#`）同得剪前 4、
  剪后 3（parcel-shipment 2→1），`git grep -c '^internal/'` 在 `e38bc18d` / `eecd05d4` 上同得 4 / 3；只对该检出成立——main 于 17:3x
  已含 wbr/01 那一剪（`62e19b1b`，同法数得 3），本笔重放到它之后的数由推送方在链 tip 重数，这里不写。剪前
  `TestWiringBaselineHasNoStaleEntry` 按预期红并点名此条，剪后 `internal/architecture` 绿。**不动的**：`source_submission.go` /
  `ClassifySourceSubmission`、PSC-2、ADR-0091 那句「仍无调用点，留在名单上」（已接受 ADR 的历史陈述，不改写）、spec.md 状态行（批收口时
  改一次）；`.scratch` 下其它引用此名的旧票面与盘点是历史取证，不动。**验证（作者层，隔离 worktree）**：`gofmt -l .` 空；`go build ./...`
  / `go vet ./...` 退 0；`go test -count=1` PS domain + `go list` 反查的 22 个反向依赖（非 cmd 19 个无 DSN 跑，`adapters/postgres` 无 DSN
  即跳过、只作编译证）+ `internal/architecture/...` 全 ok；反向依赖里 `cmd/parcel-api` / `cmd/parcel-commercial` / `cmd/parcel-dispatch`
  带 DSN `-p 1 -count=1 -v`：284 PASS / 0 SKIP / 0 FAIL（13.9 s / 3.8 s / 27.3 s），跑在通道 1 17:3x 关窗之后。
