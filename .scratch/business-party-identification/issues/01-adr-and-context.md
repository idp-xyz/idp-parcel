# 01 ADR-0367、CONTEXT、UC-PC-001

Category: docs
Status: resolved——2026-11-05 进 main
父票：[spec](../spec.md)

## 要做的

立 ADR-0367；改 party-commercial CONTEXT「业务参与方」词条，加参与方身份诸句；UC-PC-001 加「往来单位识别信息补充」与交接一段；重生成 ADR 索引与机制清单。

## 完成判据

- `go run ./tools/adr-index -check` 退 0。
- 改现行规则，进 main 前要一份非作者评审。

## 完成记录（2026-11-05）

- 落点：ADR-0367（五条决定，越权风险点待 PC owner 复核）；party-commercial CONTEXT「业务参与方」词条补识别信息一句，Rules 参与方身份诸句后加识别号、一号一主、外部编码与客户角色判读四条；UC-PC-001 加「往来单位识别信息补充」一节，交接补实现落点一段；ADR 索引重生成。
- ✅ `go run ./tools/adr-index -check` 退 0。
- 评审：与票 02–04 同一份非作者评审。
