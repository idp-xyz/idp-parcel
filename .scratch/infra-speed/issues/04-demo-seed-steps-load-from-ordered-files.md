# 04 演示种子按序加载步骤文件，`seed.sh` 不再是共享清单

Category: enhancement
Status: ready-for-agent
Blocked by: 无
地盘：`scripts/demo-seeds/seed.sh`、`scripts/demo-seeds/steps/`（新）、`scripts/demo-seeds/README.md` 里讲「怎么加一条种子」的那段
出处：用户 2026-10-19 同意「热点清单冲突」基础设施票。近两周 `scripts/demo-seeds/seed.sh` 提交 48 次：每张加种子的票都往同一个脚本的同一节追加命令与注释。

## 要什么

- `seed.sh` 只留前导（参数、编译登记 CLI、施加迁移），之后按文件名字典序 `source` `scripts/demo-seeds/steps/*.sh`。
- 现有命令按原节拆进步骤文件，**顺序与原脚本逐条相同**，每条命令上方的注释原样随它走。文件名三位数前缀、按节留空档（如 `100-commercial.sh`、`200-pricing.sh`……），让以后的票能插新文件而不改旧文件；节内有跨上下文先后依赖的（例如某条必须在另一上下文某条之后），按原脚本顺序切成多个文件，不重排。
- README 里讲加种子的那段改成：新种子优先新增步骤文件；必须插进既有序列中间时取空档号；不改别的文件。

## 不变式（硬门禁）

- **调用序列逐条相同**：把 `$BIN/*` 换成只记录参数的桩，分别跑改前与改后的脚本，两份调用记录逐字节相同。比对方法与结果写进完成记录。
- 真库一遍：一次性库上 `seed.sh` 退 0（建库方法见下）。
- 只做机械拆分：不改任何种子数据、不改任何命令参数、不删不改注释文字。

## 验证

调用序列比对 + 一次性库灌一遍：`docker exec idp-parcel-postgres-gate psql -U parcel -d postgres -qc "CREATE DATABASE <名>"`，DSN 换库名跑 `seed.sh`，用完 `DROP DATABASE <名> WITH (FORCE)`。不动 Go，不必跑全量。
