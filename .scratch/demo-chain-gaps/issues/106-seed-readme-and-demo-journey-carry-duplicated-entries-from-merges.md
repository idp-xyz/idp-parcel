# 106 种子 README 组成表与演示动线里有合并留下的重复条目，旧的那一条已经不对

Category: bug
Status: needs-triage——2026-10-19 Claude Code 子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权），出自 [98](./98-demo-walk-doc-and-seed-corrections-from-the-8197fa2f-walk.md) 往这两份文档追加时所见。
Blocked by: 无
地盘：`scripts/demo-seeds/README.md` 组成表；`docs/design/synthetic-demo-journey-script.md` 第 5 步的补记链。
出处：票 98 完成记录「偏离与待裁」。同类的 ADR 索引重复行由票 [74](./74-adr-readme-lists-adr-0205-twice.md)、[78](./78-adr-readme-lists-several-recent-adrs-twice.md) 处理过。

## 现象（钉 `5f0aa66d`）

种子 README 组成表有三对重复行，每对首列相同、内容一新一旧：

- `data/access/` 两行。较旧那一行说集成客户端族只有「合成报关服务商与合成银行各一台」，首方客户端的授予「此刻只有提交」；较新那一行已有合成干线承运商、送站预报两格与送站交接确认。
- `data/transport/` 两行（周期安排 `01`–`03`）。较新那一行改成点名 `01`–`03`、去掉份数（票 18 评审修补）。
- `data/transport/05…`、`06…` 两行（上海→深圳快线）。较新那一行末尾写明 ADR-0201 补记、演示脚本与 `seed.sh` 都指到这里。

演示动线第 5 步有两条「2026-10-19 补记（票 `demo-chain-gaps/65`…）」：前一条说没有另做一次性库实走；后一条是评审后的版本，多了 `ONWARD_SAME_AS_RELEASING` 停点与一次性库实走证据。

查法：

- `grep -n '^| \`data/' scripts/demo-seeds/README.md` 看首列重复；
- `grep -n '补记（票 \`demo-chain-gaps/65\`' docs/design/synthetic-demo-journey-script.md` 出两行。

读的人照旧的那一条读，就会得出过时的结论。

## 三类

机制（文档卫生）。规模 XS。

## 最小的诚实修法

- README 每对留较新、合「一决策一处定义」的那一行，删另一行，不重排其他行；删前确认留下的那行没有漏掉被删那行独有的内容。
- 动线的 65 补记留后一条，删前一条：两条是同一张票的同一则补记，前一条被评审版取代，删它不算改写带实走证据的旧补记——带实走证据的是后一条。
- 两份文件都是跨票热点（README 只追加的约定、动线补记链），进 main 时单独一笔提交，避开同时在途、往这两处追加的分支。

## 要裁的

无。
