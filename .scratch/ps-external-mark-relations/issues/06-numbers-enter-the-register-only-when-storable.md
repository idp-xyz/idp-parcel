# 06 外部标识号码进册前没有「存得下」的门：超长号码撞索引上限答 500，NUL 撞库面答 500；按号核对遇到不可能在册的号也答错误

Category: bug
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 立并代决、认领，分支 `claude-psxm06`
Blocked by: 无
父票：[05](./05-a-unique-hit-in-someone-elses-number-space-is-not-enough.md)
出处：no-reception-source/03 完成记录「发现」（PS 关联册按号码建的 btree 索引有长度上限，号码进册前无长度门）；本会话复核。
地盘：
- internal/parcelshipment/domain/external_identifier.go 及其测试（`NewExternalIdentifierNumber`）
- internal/parcelshipment/adapters/postgres/external_identifier.go 及其测试（按号核对读口）
- 往外部标识关联册写号的各条入口及其测试（提交 Intake 的客户包裹件号、面单渠道结果的渠道包裹件号——先 grep 全部写入方）
- cmd/parcel-api 下新起真库用例文件
- docs/adr/0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md（文末追加复核记录）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「需要」，即立票并接着做。

## 现象（读码，未实测）

- `NewExternalIdentifierNumber` 只拒全空白。号码原样落进 `parcel_shipment.external_identifier_relation`，表上按号码建了 btree 索引；Postgres btree 索引行有长度上限（约 2.7KB），超长号码在写入那一刻报错，整笔事务回滚、入口答 500——重发同样的号永远一样。
- 号码带 NUL 时 Postgres 的 text 存不下，同样答 500。
- 读的一侧：NO 收寄经 `FindParcelsByExternalIdentifier` 拿设备原样上报的号来核对。一个不可能在册的号（超长）若在读口报错而不是答「零候选」，就会被 NO 当成「身份读不动」：no-reception-source/05 之后落「待识别 · 未核对」并自投重核门铃，而门铃重投每次都撞同一个错，永远续不上。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制，不涉租户取值）

1. **存得下的门定在领域构造处一处**：`NewExternalIdentifierNumber` 拒 NUL，拒 UTF-8 字节长度超过 512 的号。上限是「存得下、索引得了」的机制界，不是业务口径——远低于索引行上限，远高于任何现行条码与运单号的长度，与本仓意图信封 Subject 的上限同量级；租户真实号码长度不需要也不允许在这里配置。号码本身其余字符照 ADR-0184 一个不动（不折算、不去内部空白、不拒其他控制字符）。
2. **写入方因此答坏输入，不答 500**：提交 Intake 的客户包裹件号越界答 400 坏报文并点名该格；面单渠道结果里的渠道包裹件号越界照该路现有的坏结果答法（先查清），不让整笔接受或结果登记以 500 回滚。
3. **按号核对读口把「不可能在册的号」答零候选**：号过不了构造门时，`FindParcelsByExternalIdentifier` 直接答零候选、无歧义信号，不报错——这样的号确定不在册，答「查无此号」是真话；报错会被 NO 读成身份读不动、落进永远续不上的重核。
4. 难逆转的那一半（号码的存得下界）在 ADR-0184 文末追加复核记录，决定原文不动。

越权风险点：代 PS owner 定了 512 字节这一界。owner 复核时看有没有渠道的号码可能更长（例如把整段二维码内容当号码交来）。

## 完成判据

- [ ] 领域：NUL、513 字节答构造错误；512 字节与带回车换行的号照收（逐字）。
- [ ] 提交 Intake：越界的客户包裹件号答 400 点名该格，委托不落；面单渠道结果照该路坏结果答法，不 500。各一条用例，修前先红（真库用例至少一条经生产装配）。
- [ ] 读口：越界号答零候选、无信号、不报错；经生产收寄装配真库用例：设备送来越界号，答待识别（已核对），不落未核对、不响门铃。
- [ ] ADR-0184 文末复核记录；`git diff` 该文件只有新增行。
- [ ] Go 门禁：`gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0；带 DSN 跑 `internal/parcelshipment/...`、`internal/nodeoperations/...`、`cmd/parcel-api`、`internal/architecture` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。
