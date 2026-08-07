# Agent 交接：去参考设计权威依赖（自洽文档路径）

状态：路径 **A（supersede ADR）、B（抬升已定案 OPEN 项）、C（誊金样例）已执行完毕，D 的硬标准（`docs/` 无权威性外链指向参考树）已达成**；仍未做的只有**批量抹名**（待产品决定）与**删除参考树**（待人类确认）。本文件是工作摘要，不是新权威规则。规则仍以各 `CONTEXT.md` / ADR / 用例为准。

日期：2026-08-07（A+B+C+D 于同日执行，通道 `idp-mcp-1`）  
上一会话通道：`idp-mcp-4`  
仓库：`idp-parcel`

---

## 1. 本会话要解决的问题

人类提出的问题是：这些红线能不能靠补充新文档或新内容来解决，而不是继续引用那套外部参考设计。

**结论：可以。**

红线禁止的是：

- 把参考树当作本仓第二权威（单一权威）；
- 改写已 Accepted 的 ADR 正文历史（只允许 new / supersede）；
- 伪造证据（改参考设计声明的哈希/文件名去贴仓库现存文件）；
- 未确认参数写死为生产默认；证据层级不诚实（`S` 冒充 `R`/`P`）。

红线**不禁止**：

- 新建或改写**活文档**（CONTEXT、验收矩阵、交接、工作单、README），使每条主张在本仓内自证；
- 用 **supersede** 新 ADR 重述 Decision/Consequences，Links 只指向本仓；
- 把参考设计里仍需要的内容**誊进** `docs/`（金样例输入/期望/单元格坐标等），再删除参考树。

「去掉对参考设计的引用」≠「假装从未吸收过」：决策实质必须落在本仓权威位置；台账可以退役；路径链接可以清掉；名称可以换成中性说法，但**不得**再靠点进参考树才能读懂规则。

产品其后确认执行批量抹名，因此本仓活文档一律以「参考设计」称呼那套外部交付物、以「参考树」称呼其目录，原代号只在两处保留：已被取代的 ADR-0011 正文（红线禁止改写已接受记录），以及仍需按真实路径执行的删除指令。

---

## 2. 上一会话已确认的产品判断

| 判断 | 说明 |
|---|---|
| 参考树定性 | 外部参考输入，不是规格、不是权威；定性见 ADR-0011 |
| 把参考设计当可引用权威 | 事故，不是设计意图 |
| 源价卡权威文件 | `docs/reference/蜴国际-美线UPS-Ground-同行价卡-260729.xlsx` |
| 该文件 SHA-256 | `9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d` |
| Golden 声明的「副本…xlsx」 | 已删除；声明哈希 `22ec1f55…` **永久不可复核** |
| 哈希门禁结论 | **不可闭合 → 长期声明**：等价关系由仓库所有者断言，非哈希证明；保证强度低于哈希匹配 |
| Schema 指针 | `$schema` 指向 `…v1.0.schema.json`，磁盘只有 `…v1.0.1.schema.json`；按后者校验，属参考侧笔误 |
| 佣金 + 追溯影响分析 | **不进首发**（REF-OPEN-03 两处「未识别」只登记、不新建 UC） |

权威门禁正文：[`docs/domain/parcel-pricing/CONTEXT.md`](../domain/parcel-pricing/CONTEXT.md)「Source integrity gate」。  
核实写回：[`docs/design/pp-s03-w01-golden-case-source-evidence-request.md`](./pp-s03-w01-golden-case-source-evidence-request.md)（表内第四列自证，删参考树后仍可复算单元格）。

---

## 3. 吸收台账与 REF-OPEN 现状

台账：[`docs/design/pp-reference-design-absorption-coverage.md`](./pp-reference-design-absorption-coverage.md)

收口条件**机制侧已达成**（六项均有决策或明确延后）：

| 编号 | 状态 | 一句话 |
|---|---|---|
| `REF-OPEN-01` | 已定案 | 「计算目的」首发三值与价格方向一一对应；已进 CONTEXT / GLOSSARY 方向 |
| `REF-OPEN-02` | 机制闭合，业务裁决延后 | 四条 `SRC-DISC-*` 进门禁；裁决前 41 例不得金额验证 |
| `REF-OPEN-03` | 已定案 | 八流程 ↔ UC 核对完成；佣金/追溯影响分析首发不做 |
| `REF-OPEN-04` | 已定案 | **首期不实现 Compiled Pricing Plan / 编译器服务**；发布校验语义在 CONTEXT；内容哈希用 `fingerprint.go`；勿简化成「只是缓存」 |
| `REF-OPEN-05` | 已定案 | 不另写 ADR；不采纳参考侧 API/技术设计是 ADR-0009+0011 的推论，台账记一笔即可 |
| `REF-OPEN-06` | 已定案 | 安全/租户/审计/保留**不当计价范围**；改判跨切面，PN-08 `PAR-GOV-*` |

仍标「待决」但不进 OPEN 清单的七类：Geometry/表达式等「等真实需求」、聚合事务/CQRS「实现时收敛」、Golden §28 映射「跟 OPEN-02」。

**去权威依赖的结果：** `REF-OPEN-04` 与 `REF-OPEN-06` 的完整理由已抬进 `parcel-pricing/CONTEXT.md`，台账这两行降为索引；`REF-OPEN-03` 的两条未识别能力改写为不依赖章节号的描述，以台账为登记处。台账余下的「见 §N」是对照表的行主键，删目录后失去对照对象，属预期——那是对照记录，不是规则依据。

---

## 4. `SRC-DISC-*` 单元格核实结论（业务仍待签字）

已在权威源 xlsx 上只读核实，写在 `PP-S03-W01`：

| ID | 级别 | 核实结论 | 业务还需做什么 |
|---|---|---|---|
| `SRC-DISC-001` | BLOCKING | **疑为术语歧义**：`L8` $5000 违禁品文字条款 vs `L40` Unauthorized Package / 超限 $2025，不是同一费用 | 确认是否两回事 |
| `SRC-DISC-002` | HIGH | **属实**：`L32`/`L36` 等为 FedEx 标签的完整超大费行 | **出具方确认**：粘贴残留还是真实计费 |
| `SRC-DISC-003` | MEDIUM | **不成立**：八折 = 80% | 业务确认后可从阻塞移除 |
| `SRC-DISC-004` | MEDIUM | **不成立**：AHS/OS 基准在 `Q32`–`Q35`，不在 `F1` | 业务确认后可从阻塞移除 |

当前真正硬阻塞金额可用性的是 **`SRC-DISC-002`**（及在签字前仍按原级别对待的 001/003/004）。  
CONTEXT 门禁第四条仍写「尚未裁决」——与 W01 核实表一致：核实 ≠ 业务裁决。

---

## 5. ADR-0011 红线冲突点（已解决）

原冲突：[`ADR-0011`](../adr/0011-parcel-pricing-context-within-idp-parcel.md) 状态为 Accepted，其 Links 把参考树下三份文件列为可点击依据，读者必须打开外部参考树才能读懂决策来由。

已执行的合规路径：新建 [`ADR-0012`](../adr/0012-parcel-pricing-context-within-idp-parcel.md) 取代 0011，自洽重述 Context/Decision/Consequences/Alternatives，Links 只指 CONTEXT-MAP、parcel-pricing CONTEXT、ADR-0009、`PP-S03-W01` 与旧 ADR 本身；Alternatives 里「复制参考设计作为独立平台」改写为「复制外部参考设计为独立计费平台或微服务」，并补入「只删旧 ADR 外链」为何不采用。

ADR-0011 **只改状态行**为 `Superseded by ADR-0012` 并加 `Superseded: 2026-08-07`，正文一字未动，其参考树链接作为历史保留（删目录后成为断链，属预期）。`docs/adr/README.md` 新增「已被取代决策」小节。

现行权威指向已从 0011 改到 0012 的位置：`parcel-pricing/CONTEXT.md` 首段、`PN-07` 交接依据表、吸收台账各处。

---

## 6. 路径选择结果

人类已选 **E→A+B，C 单独排期**（2026-08-07，通道 `idp-mcp-1`）。

| 选项 | 内容 | 现状 |
|---|---|---|
| A | supersede ADR-0011 | **已完成**，见 §5 |
| B | 抬升 REF-OPEN-04/06 等到 CONTEXT/决策索引，退役台账依赖 | **已完成 OPEN-04 与 OPEN-06**；OPEN-03 残留见下 |
| C | 誊金样例（至少 41 `SOURCE_RATE_CARD`）进 `docs/` | **已完成，且扩到全部 136 例**，见[转录本](./pp-golden-case-transcript.md)；价卡抽取另行归档 |
| D | scrub 活文档路径（CONTEXT/matrix/handoff/README） | **硬标准已达成**：无权威性外链，流程依赖已清，过时口径已改。仅剩批量抹名待产品决定 |
| E | 先出执行清单再动手 | 已出清单并获批 |

**`REF-OPEN-03` 已收口。** 其八条流程 ↔ `UC-*` 核对表仍以参考设计章节号为行主键，删目录后无法复核；但可长期保留的残余只有两处「未识别」登记——周期结算的佣金、追溯调价的影响分析。两条已改写为不依赖章节号的能力描述（含说明与应属上下文），并明确**以吸收台账为登记处**：台账因此从「吸收台账」转为历史索引兼这两条的登记处，不整表退役。

选此落点而非 `CONTEXT.md`，是因为它们是范围登记而不是领域语言；写进 `CONTEXT.md` 会让上下文同时承载「不做什么」的排期信息。真到要做时按所属上下文立用例，并把登记从台账移出。

不要擅自删除参考树。

---

## 7. 建议的「去参考设计依赖」执行清单（完成标准）

按序；未完成前一步不要删目录。

1. **金样例落库（硬门槛）** — **已完成**  
   - ✅ 全部 136 例逐字转录至 `docs/reference/golden-cases/`，含四条 `SRC-DISC` 声明；程序生成而非手抄，逐例规范化比对 0 差异。  
   - ✅ 95 例合成/上游一并转录，理由是门禁明写它们可作隔离 `S`，只誊 41 例会让门禁指向不存在的东西。  
   - ✅ 原案例集的 `fixtures`（一张 `WEIGHT_ZONE` 价表 + 25 项附加费抽取）不进 `docs/reference/`，改为归档在 `docs/archive/reference-design-rate-card-extraction-v1.0.1.json`，带「仅追溯、未经核实、不得作价卡数据」抬头。既不把未核实价表放进现行数据目录，也不永久丢掉追溯线索。  
   - 完成标准已达成：不打开参考树也能审这些案例。

2. **重写依赖参考设计案例对比的门禁叙述** — **已完成**  
   - ✅ `PP-S03-W01` 的 `EVD-03` 与 `parcel-pricing/CONTEXT.md` 源完整性门禁改为指向本仓转录本。  
   - 完成标准已达成：两处均无「必须打开参考树 JSON」的步骤。

3. **抬升已定案 OPEN 项** — **已完成**  
   - ✅ `REF-OPEN-04` 三层理由已写入 `parcel-pricing/CONTEXT.md` 新增「首发实现取舍」节，改用本仓自有概念（发布生命周期的「草稿 → 已校验」、「版本内容摘要」、源完整性门禁）表述，无参考设计章节号。  
   - ✅ `REF-OPEN-06` 跨切面边界已写入同文件「Boundaries and ownership」。  
   - ✅ `REF-OPEN-03` 的两条未识别能力改写为不依赖章节号的描述，以台账为登记处。  
   - 完成标准已达成：台账转为历史索引兼两条登记的所在处，其余各项只留索引，唯一住所在权威文档。

4. **Supersede ADR-0011** — **已完成**  
   - 完成标准已达成：ADR-0011 正文与 Accepted 历史保留（仅加状态行），现行权威 [`ADR-0012`](../adr/0012-parcel-pricing-context-within-idp-parcel.md) Links 零参考树路径。

5. **Scrub 活文档** — **已完成**  
   - ✅ 无权威性外链：`docs/` 下唯一指向参考树的可点击链接是已被取代的 ADR-0011 正文里那三条，红线禁止改写已接受记录，故刻意保留为历史；删目录后成为断链，属预期。  
   - ✅ 流程依赖已清（见第 2 项），过时口径已改（`PP-S03` 的「哈希修复后」、CONTEXT「待修复验证资料」）。  
   - ✅ 产品确认后已执行批量抹名：活文档统一称「参考设计」与「参考树」，`FOO-OPEN-*` 改为 `REF-OPEN-*`，两份文件更名为 `pp-reference-design-absorption-coverage.md` 与 `pp-de-reference-authority-agent-handoff.md`。  
   - 原代号仅存于两处：ADR-0011 正文，以及下一项按真实路径给出的删除指令。

6. **删参考树前验证** — 待人类确认  
   - 无活文档可点击进参考树；`go test ./...` 绿；人类确认。  
   - 删除前复跑一次案例保真比对：那是删掉之后再也执行不了的核验。  
   - 目录的真实路径仍是 `foo/`，删除指令必须用真实路径才可执行，这不是抹名遗漏。  
   - **禁止** Agent 在未经人类确认时删除该目录。

---

## 8. 同会话相关、但非本交接主线的工作（避免下一任误开）

若下一任只接「去参考设计权威依赖」，可忽略本节；若同仓续写 PN02 合成契约再读。

已完成（摘要）：

- `SafeHandoffAssessment.ConfirmationReference()`：未确认交接不得播种 OTHER 权威（与 `EffectiveAt()` 纪律对齐）。  
- `SYN-CHAIN-01..03,05..07` 契约测试；CHAIN-04 拆商业/结算两侧；结算 **account** 由 settlement 从依据维度解析，**currency** 来自 commercial 结算政策；反射守卫禁止跨边界 struct 带 `account`。  
- `// Covers:` 回引；S02-AT-09 作业不变迁生产权威。  
- REF-OPEN-04 理由重写；SRC-DISC 核实写入 W01；矩阵 / pn-07 去掉过时「等哈希修复」措辞。

未完成 / 曾挂起（以工作区 todo 与任务包为准，交接时请 `git status` 复核）：

- 推广 `// Covers:`；拆 S01-W04/W05 独立契约文件；CHAIN-04 曾阻塞于任务包与 CONTEXT 所有权——后续是否已定请查当前代码与 CONTEXT。

---

## 9. 红线与写作约定（下一任必须遵守）

来自 [`AGENTS.md`](../../AGENTS.md)：

- 未确认参数 / `BD-*`：可配置或显式未决，不写生产默认。  
- 证据：`S` 只记 `S`；生产 `Go` 只来自登记册 + PN-08。  
- 一决策一处；交接与用例只引用，不复制第二套口径。  
- 改领域语言 → 先 CONTEXT（必要时 MAP/GLOSSARY），再 UC。  
- 难逆转取舍 → 新 ADR 或 supersede，**不改写**已接受 ADR 历史。  
- 敏感实例外置；仓库只登脱敏标识与证据索引。

本主题额外：

- **不修改参考树下任何文件**去「对齐」仓库哈希或 Schema 名。  
- 单元格核实可写进本仓文档；**业务裁决**前不得把 41 例升为金额验收。  
- 断言强度源身份支撑 `P` 时必须人类书面接受该强度。

---

## 10. 关键路径速查

| 用途 | 路径 |
|---|---|
| 开工总则 | `AGENTS.md`、`docs/agents/workflow.md` |
| 计价边界 + 源门禁 | `docs/domain/parcel-pricing/CONTEXT.md` |
| 上下文地图 | `docs/domain/CONTEXT-MAP.md` |
| 现行计价 ADR | `docs/adr/0012-parcel-pricing-context-within-idp-parcel.md` |
| 已被取代的旧计价 ADR（仅历史） | `docs/adr/0011-parcel-pricing-context-within-idp-parcel.md` |
| 吸收台账 | `docs/design/pp-reference-design-absorption-coverage.md` |
| 源证据工作单（含 SRC-DISC 核实表） | `docs/design/pp-s03-w01-golden-case-source-evidence-request.md` |
| PP-S03 交接 | `docs/design/pp-s03-par-set-02-03-evidence-and-synthetic-contract.md` |
| 验收矩阵（证据层级） | `docs/product/PILOT-ACCEPTANCE-MATRIX.md` |
| 权威价卡文件 | `docs/reference/蜴国际-美线UPS-Ground-同行价卡-260729.xlsx` |
| 案例转录本（说明） | `docs/design/pp-golden-case-transcript.md` |
| 案例转录本（数据） | `docs/reference/golden-cases/parcel-pricing-rating-golden-cases-v1.0.1.json` |
| 价卡抽取归档（仅追溯） | `docs/archive/reference-design-rate-card-extraction-v1.0.1.json` |
| 参考树（待删） | 参考树（~28 文件；案例与价卡抽取均已落库，内容上可删） |
| 计价域代码 | `internal/parcelpricing/domain/`（含 `fingerprint.go`） |

文档索引入口：[`docs/README.md`](../README.md)。

---

## 11. 建议下一任开场动作

A + B + C 已完成，剩余工作按下列顺序：

1. 读本文件 §1–§7。  
2. 唯一仍待人类决定的是**要不要批量抹掉活文档里的参考设计字样**。这是称谓问题，不是依赖问题：删目录所需的内容自证已经完成，`docs/` 下已无权威性外链指向参考树（仅存三条在已被取代的 ADR-0011 正文，属刻意保留的历史）。产品若不要求抹名，保持现状即可。  
3. 删参考树只差人类确认。确认前请复跑一次案例保真比对——那是删除后再也无法执行的核验。  
4. 每步改完：更新本交接文状态栏，或按 `/handoff` 另开新交接；在 `docs/README.md` 为新增权威文档补一句入口。

---

## 12. 本交接不包含的内容

- 未删除参考树。  
- 未批量抹掉活文档中其余参考设计字样；那是称谓取舍，待产品决定。  
- 未核实转录案例中的任何金额；转录只保证与原交付一致，不保证与价卡一致。  
- 未核实归档的价卡抽取；它按未经核实原样保存。  
- 未取得 `SRC-DISC-*` 业务签字。  
- 未改变证据层级或试点 `Go/No-Go`。  
- 未改变任何计价语义：ADR-0012 与 CONTEXT 新增内容都是既有决策的自洽重述与抬升，不新增能力范围。
