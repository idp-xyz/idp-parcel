# 04 演示种子按序加载步骤文件，`seed.sh` 不再是共享清单

Category: enhancement
Status: resolved——2026-10-19 进 main（全量钉 `a34f8852`：`scripts/test-full.sh` 156 ok / 0 FAIL；02、03、04 同批，见文末「进 main 记录」）。此前 in-progress——完工待进 main，分支 claude-is04
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

## 完成记录

作者子代理，worktree `idp-parcel-is04`，分支 `claude-is04`，基点 `f91f29a4`（其后 origin/main 前进到 `d815e385`，`scripts/demo-seeds/` 未变）。

### 做了什么

- **`seed.sh`**：前导（参数、编译登记 CLI、施加迁移）逐字节未动；其后改为加载 `scripts/demo-seeds/steps/*.sh`——按 `LC_ALL=C sort` 排序后逐个 `source` 进同一 shell，末行「种子灌入完成」留在 `seed.sh`。另加两道守卫：`steps/` 下一个文件也没有时退 1；文件名不是「三位数前缀 + 连字符」时退 1。
- **`steps/`**：原脚本以 `echo "== … =="` 起头的每一节一个文件，节内命令、续行与注释原样搬入，节间空行不搬。编号 100 起按 50 递增：`100-commercial`、`150-pricing`、`200-network`、`250-customs`、`300-visibility`、`350-collection`、`400-governance`、`450-access`、`500-settlement`、`550-transport-schedule-templates`、`600-transport-capacity-pool`、`650-node-operations-pre-advice-window`、`700-transport-sha-szx-express`、`750-transport-szx-sin-express-sin-lm-shuttle`（均 `.sh`，取证于 `ce00a6ae`）。运输三节被节点作业一节隔开，照原序各成一个文件，不合并。
- **`compare-seed-calls.sh`**（`1e9cf2b4`）：调用序列比对，用法与原理见脚本头注与 README「加一条种子」。
- **README** 新起「加一条种子」一节（放在「用法」之后）。

### 代决与越权风险点

用户授权原话「你作为业务和系统专家自决……解决所有的问题」。以下逐条代决，待 owner 复核。

1. **拆分粒度只到原节，节内不再切。** 商业那一节里混着消费方（parcel-shipment）的解析键登记，照票面「节内有跨上下文先后依赖的……切成多个文件」字面可切。不切的理由：解析键那段注释以「上一行的发布批里……」指向发布命令，切开后「上一行」落到另一个文件里，而本票不许改注释文字。**越权风险点**：以后有票要在商业节内部插命令时，得先把 `100-commercial.sh` 在插入点切开（README 已写切法）；若 owner 认为应现在就切，切法是机械的，比对脚本能证。
2. **编号 100 起按 50 递增。** 票面举例 `100-commercial.sh`、`200-pricing.sh` 按 100 递增，照搬的话十四节排不进三位数。**越权风险点**：节间空档 49 个号；000–099 留给须先于商业发布的步骤，750 之后留给新节。owner 若要别的步长，改名是纯机械的。
3. **排序钉 C 序，同号不报错。** 两张并行的票取同一个号时按全名的 C 序先后跑，不当作错误：并行票看不见对方的种子，不该互相依赖，报错只会把「去冲突」变回「合并时冲突」。**越权风险点**：两张同号票如果其实有依赖，加载器发现不了；README 写明「真有先后，后者另取更大的号」，靠作者守。
4. **加载器多一道命名守卫**（无三位数前缀即退 1）。票面没要求。理由：C 序下无前缀文件排到所有数字之后，能跑，但位置不是作者选的。**越权风险点**：这是 `seed.sh` 新增的失败方式，只对命名起作用，现有文件全部满足。
5. **桩的接法是 PATH 上垫假 `go`，不给 `seed.sh` 加测试开关。** 假 `go` 把每个包编成只记「程序名 + 全部参数」的桩，`seed.sh` 一字不改即可被比，任何一版（含没有比对脚本的旧版）都能当基准。比对另外也比 `seed.sh` 的标准输出（各节标题），比票面要求严。**越权风险点**：假 `go` 只认 `go build -o <目录>/ <包>…` 一种形状，`seed.sh` 改编法时比对脚本要跟着改——它会报错退出，不会静默比两份空记录。
6. **README 原本没有讲加种子的段落**（票面地盘写的是「那段」），本票新起一节。步骤文件不带 shebang、不设可执行位，是被 `source` 的片段。**越权风险点**：位置与措辞 owner 可调。README「组成」表若干行写着「`seed.sh` 不复述这些取值」，命令与注释现在在 `steps/` 下；`seed.sh` 仍是整体入口，这句话仍成立，且不在本票地盘，未改。仓内其他文档（含已接受 ADR-0217「`seed.sh` 一行」）同理未改。

### 验证方法与结果

- **改前基准**：比对脚本对 `origin/main` 与尚未改动的工作树跑一遍，四项相同。调用记录每种模式 180 行（`go build` 一行、`migrate` 一行，其余是登记 CLI）；sha256 前 16 位：不带参数 `9ab224ed8ce5cfa7`、`--reset` `6160013941feb501`；标准输出 `cd6493f4ad7604ed` / `7d325cf6a3fff770`。四份记录另存为文件，留给下面的独立复核。
- **改后比对**：`scripts/demo-seeds/compare-seed-calls.sh origin/main HEAD`（HEAD = `ce00a6ae`）退 0，四项相同，sha256 与改前逐一相同。
- **独立复核**（不经比对脚本的比较逻辑）：自己垫同一个假 `go`，在 `LC_ALL` 取 `C`、`en_US.UTF-8`、`zh_CN.UTF-8` 时各跑两种模式，与改前存下的记录 `cmp`，六次都相同。
- **只搬不改**：步骤文件按序以空行相接，与原 `seed.sh` 从第一节标题到末行之前的正文 `cmp` 相同；`seed.sh` 前导与原版 `cmp` 相同。
- **反例**：把 `150-pricing.sh` 两条命令对调，比对脚本报「不同」、退 1 并给出 diff；放一个 `zz-noprefix.sh`，加载守卫报错退 1。两处都已复原。
- **真库**：在 55432 上 `CREATE DATABASE is04_seed`，`seed.sh` 退 0（各节标题全出，末行「种子灌入完成」），再跑 `--reset` 一遍也退 0；抽查 `pilot_governance.authority_interval` 36 行；用完 `DROP DATABASE is04_seed WITH (FORCE)`。
- `go build ./...`、`go vet ./...` 干净，`gofmt -l` 空。本票不动 Go，未跑全量（照票面）。

### 偏离与待裁

- **进 main 时若有冲突**：若这期间 main 上另有票往旧 `seed.sh` 某节追加了命令，合并会在 `seed.sh` 冲突。处理：把那几行连同注释搬进对应的步骤文件，或按 README 新起一个取空档号的文件；再跑 `compare-seed-calls.sh <那张票进 main 后的 main> <合并结果>`，应当相同。
- 票面没要求、本票多做了两件：比对时也比标准输出，加载器加了命名守卫（代决 4、5）。
- `docs/product/MECHANISM-INVENTORY.md` 未重生成，留给进 main 时统一做；它的生成不读 `seed.sh`。

## 进 main 记录

2026-10-19 协调方合入，02、03、04 同批：合入时 seed.sh 自本票基点未被别票改过；`compare-seed-calls.sh origin/main HEAD` 四项相同，一次性库实灌退 0。叠合后按新步骤重生成 ADR 索引、重算 AT 盘点计数、重生成机制清点，各自单独成笔；`adr-index -check` 与 `at-coverage-counts -check` 退 0；在干净树上对 `a34f8852` 跑 `scripts/test-full.sh` 全量，156 ok、0 FAIL。
