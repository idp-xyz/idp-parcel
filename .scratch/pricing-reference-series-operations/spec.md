# 计价参考序列（汇率 / 燃油）的运营形态：绑定标识、复核进在用、评价时解析

Category: enhancement
Status: in-progress——01 resolved（`7042a38`），02 resolved（`1b09c2d`），03 可认领，04–05 ready-for-agent（按阻塞边顺序），06–07 draft

依据：[ADR-0099](../../docs/adr/0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)；术语与生命周期已落 [parcel-pricing CONTEXT](../../docs/domain/parcel-pricing/CONTEXT.md)「计价参考序列 / 序列版本复核 / 在用序列版本」与「计价参考序列版本」。本目录只引用，不复述第二套口径。

## 起因

用户（2026-09-03，通道 3）问「现在支持用户手工汇率的维护方式吗」→「头部企业怎么管」→「运营便利上你的专业建议」→「你自决吧」。对照行业做法后核出的结构性事实（实测于 `0d492b8`）：

- 价卡把**序列版本**（含摘要）绑进方案清单与内容摘要，`resolveSeries` 要求逐格相等。序列每出一版，引用它的每张卡都要重登。这条不改，任何登记表单、自动喂价都进不了评价。
- 序列版本登记之后没有任何状态；`PAR-SET-11` 指名的「复核责任方」没有机制落点。
- 评价用例没有接序列解析口（票 08 评论原话「消费面不在本票」）。

## 取证锚点

全部举证锚 `0d492b8`；该 SHA 上 `go build ./...` 绿（本会话未验 PG）。共享树上同时压着 MCP-5 在 `partycommercial` 的未提交改动，本目录不触碰。

## 票一览与阻塞边

| 票 | 一句话 | 层 | 阻塞于 |
|---|---|---|---|
| [01](issues/01-bind-series-identity-and-compose-evaluation-manifest.md) | 价卡绑序列标识；评价清单 = 方案清单 + 解析到的序列版本；`PPC-4`；复核值对象与在用选择纯函数 | domain | 无 |
| [02](issues/02-review-register-and-in-force-resolver-persistence.md) | 复核追加表（0004）、复核写口、在用解析读口及真库实现 | ports + postgres | 01 |
| [03](issues/03-review-use-case-and-evaluation-resolves-in-force.md) | 复核用例；评价用例形成前解析在用版本补齐取值；CLI 增复核种类；seedgen 重跑 | application + cmd | 02 |
| [04](issues/04-review-endpoint-and-admin-structured-form.md) | 复核端点（未配置格）；管理台复核动作 / 逐字段登记表单 / 更正动作 | http + admin-web | 03 |
| [05](issues/05-coverage-horizon-and-blocked-evaluations-read-face.md) | 覆盖地平线与被挂起评价联动的读面 | ports + postgres + admin-web | 03 |
| [06](issues/06-source-connector-framework-and-cfets-connector.md) | 来源连接器框架 + 首个连接器（CFETS 中间价）；免复核声明格 | adapters | 03；**draft**：出网方式待定 |
| [07](issues/07-customs-valuation-rate-series-kind.md) | 海关计税汇率作为第三种序列种类？消费方是谁？ | 跨上下文 | **draft**：待裁 |

## 边界

- 不改 `PAR-SET-11`（实例半边）；不写任何真实汇率、来源、阈值；SYN 夹具只记 `S`。
- 不拓宽 `ReferenceSeriesRegister` 接口（会拆写侧替身，`ports/catalogue_read.go` 记过）。
- 不碰 `partycommercial` / `party_commercial` / `cmd/parcel-commercial`（MCP-5 地盘）。
- 加点规则、汇兑损益、锁汇不在本目录（ADR-0099 决定七）。

## 完成判据

- 一张已发布价卡在序列出新版本、复核通过后**不重登**即可在新评价里用到新取值；重放旧评价仍读旧版本。
- 未复核版本进不了评价；评价原因文字能区分「无版本 / 未复核 / 期次缺口」。
- 全仓 `go build` / `go vet` / `go test -count=1` 绿，真库用例 `-v` 下 `PASS` 非 `SKIP`。
