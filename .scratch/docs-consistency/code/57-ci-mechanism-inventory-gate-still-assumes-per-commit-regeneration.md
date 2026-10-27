# 57 CI 的机制清点门仍按各笔自己重生成的旧规矩挂在 PR 上，与生成物的现行规矩相反

Category: chore
Status: in-progress——完工待评审与进 main，分支 claude-tool（见文末「完成记录」）。原 ready-for-agent——R3（不急）：今天不经 PR 落 main，这道门只在 main 推送上起作用，结论照旧对；一旦走 parallel-sessions「落 main 的下一形态」开 PR，每张新增文件或迁移的票都会按设计在这一门上红。2026-10-21 文档自洽 10（10b）立（用户「你作为业务和系统专家自决」授权），分支 claude-dc10b。
Blocked by: 无
出处：[audit/cross.md](../audit/cross.md) AG·M1（归 B，10b 代决）。取证于 main `4e7874ba0`。

## 现象

- `.github/workflows/ci.yml` 的 `Mechanism inventory is current` 一步没有 `if:`，PR 与 main 推送都跑；它下面那段注释写「ADR 索引与 AT 盘点计数也是生成物，但约定与上一步相反」，接着说那两件由推送方在 main 的 tip 上重生成、单独成笔。
- 10b 照各票派单「进 main 时统一做」的实际做法裁定三件生成物同一条规矩：票不在自己的笔上重生成，推送方在 `main` 的 tip 上重生成、各单独成笔（[parallel-sessions「生成物不占号」](../../../docs/agents/parallel-sessions.md#生成物不占号但只在干净检出上生成)、[workflow「进 main 的步骤」](../../../docs/agents/workflow.md#进-main-的步骤)）。按这条规矩，机制清点那一门挂在 PR 上就会按设计红，红就不再是信号——与 ADR 索引那一门的注释说的是同一个理由。

## 三类

机制（CI 配置与注释）。无产品策略、无租户取值。

## 规模

S。

## 最小的诚实修法

1. `Mechanism inventory is current` 一步加与下一步相同的 `if: github.event_name == 'push' && github.ref == 'refs/heads/main'`。
2. 下一步的注释「约定与上一步相反」改为「与上一步同一条规矩」，指向 parallel-sessions「生成物不占号」。
3. 上一步自己的注释（「清点报告因此不可能……落后于代码」）补一句：它守的是推送方漏了重生成那一步。

## 要裁的

无（规矩已由 10b 裁定，见票 `../issues/10-cross-cutting-docs.md`「10b 完成记录」）。

## 完成记录（2026-10-27，作者会话，分支 claude-tool，基 origin/main `f6a4a5298`）

### 做了什么

- `.github/workflows/ci.yml` static job：
  - `Mechanism inventory is current` 加 `if: github.event_name == 'push' && github.ref == 'refs/heads/main'`，与下一步同条件；命令不动（`docs/agents/workflow.md`「进 main 的步骤」里推送方重生成那一条指向这一步取机制清点的重生成命令）。票面第 1 步。
  - 两步的注释合并成同一条规矩：三件生成物票不在自己的笔上重生成，推送方在 main 的 tip 上重生成；「约定与上一步相反」那句删了。清点那一步补了「它守的是推送方漏了重生成那一步，所以不挂 PR」。票面第 2、3 步。
  - 新加一步 `Generated artifacts are well-formed`，`if: github.event_name == 'pull_request'`，跑 `bash scripts/ci/generated-artifacts-pr.sh`。这是票面最小修法之外的一步，理由见代决 1。
- 新增 `scripts/ci/generated-artifacts-pr.sh`（PR 上的生成物门，本机同一条命令可跑，不改工作区）：
  - `adr-index -check`、`at-coverage-counts -check`：退 0 一致，退 1 过期（放行，打印一行），退 2 及以上红并原样打印工具的报错。
  - 机制清点没有 `-check`：生成到临时文件；生成器非零退出，或报告里带「包加载报错」，红；与提交的不同只打印一行。
  - 两个 `-check` 工具先 `go build` 成二进制再跑。
- `docs/agents/parallel-sessions.md`「生成物不占号」：「在 tip 上重跑」补成「在 main 推送的 tip 上」，节末加一段说明 CI 的分工（要求新鲜的两道只在 main 推送，PR 上是脚本、退 1 放行、退 2 照红）。

### 门的行为（本地模拟）

模拟办法：`git archive HEAD` 解到私有目录并 `git init` 成一次性仓库；每个场景在临时分支上提交一个状态，用一段 python 读 `ci.yml`、按事件求值各步的 `if`、对未跳过的步用 `bash -e` 跑 `run:`（`GITHUB_WORKSPACE` 指向该仓库），退出码取各步的并集。两条路径各跑一遍：

| 场景 | PR | main 推送 |
|---|---|---|
| 全新鲜 | 绿 | 绿 |
| 新增一个生产 `.go` 文件（只有机制清点过期） | 绿 | 红 |
| 改一份 ADR 的 H1（只有 ADR 索引过期） | 绿 | 红 |
| 改总览计数（只有 AT 计数过期） | 绿 | 红 |
| 提交里删了 `MECHANISM-INVENTORY.md` | 绿 | 红 |
| 典型的一张票：新文件 + ADR 标题 + 计数都没重生成 | 绿 | 红 |
| 在生成区之外改 `docs/adr/README.md` | 绿 | 绿 |
| ADR 索引的结束标记没了（退 2） | 红 | 红 |
| AT 分桶表缺一行（退 2，报文列出缺哪个与补法） | 红 | 红 |
| 生产文件语法错（端口清点包加载报错） | 红 | 红 |

main 推送路径上 `go run` 把退出码压成 1，日志里能看到工具自己的报错和 `exit status 2`，红绿一致；PR 路径靠脚本里的二进制分出 1 与 2。

### 代决

1. **不只加 `if`，PR 上补一道「格式可解析、不缺行」的门。** 只加 `if` 之后，PR 上机制清点、ADR 索引、AT 计数一概不看：ADR 生成区标记被改坏、ADR 读不进、UC 立了号而分桶表缺行（要作者补行的那类，退 2）、清点工具自己跑不动，都要到 main 推送才红，而那时已经落了 main。这几类重生成救不了，必须作者在落 main 之前修，所以留在 PR 上；只有「过期」放行。
2. **退 1 放行，退 2 照红。** 与 `at-coverage-counts` 头注和作者共用规矩一致（退 2 = 新立 AT 号缺行必须补，退 1 = 只是总览过期可以）。
3. **机制清点在 PR 上的等价物。** 它没有 `-check`，且 main 上的做法是原地重生成再 `git status`，PR 上不能那样（会把工作区弄脏，且过期要放行）。改为生成到临时文件，只看「生成器退出码」与「报告里有没有包加载报错」，过期不红。包加载报错原先在 PR 上靠「与提交的不一致」间接拦着，放行过期之后要显式留这一条，否则这是被放松掉的一个有用检查。
4. **脚本编二进制，不走 `go run`。** 实测 `go run` 把子进程的 `os.Exit(2)` 压成 1（打印 `exit status 2` 但自己退 1）。若 PR 脚本用 `go run`，退 1 与退 2 分不出，缺行会被当成过期放行；变异里这一条被杀（见验证）。main 路径的两步命令保持不动，只在注释里写明它们只当红绿用。
5. **main 路径的两步不改成调脚本。** `workflow.md`「进 main 的步骤」让推送方照 `ci.yml`「Mechanism inventory is current」一步取重生成命令，搬走就得改那份文档；两步本来就对，不动。
6. **不加「PR 不得改生成物」的门。** 作者共用规矩是不提交生成物，但 parallel-sessions「落 main 的下一形态」写的是机制清点、ADR 索引与 AT 盘点计数在落地那一笔的 tip 上重生成、随 merge 一起落，PR 分支上出现生成物改动是合法形态，拦它会卡掉那条路。

### 越权风险点

- PR 上第一次检查 ADR 与 AT 的退 2。若某张 PR 开在一个自己已经退 2 的 main 上（标记坏了、缺行），PR 也红；这是对的（要补行），但红会传染给无关的 PR，直到 main 上补好。
- 「包加载报错」这个判据是清点报告里 `tools/mechanism-inventory/main.go` 的一句字面文案；该文案改了，PR 上这一条无声失效，没有测试守这个串。已在脚本头注里把判据写明，改文案的人 grep 得到。
- 脚本和两个 `-check` 工具的退出码约定（1 = 过期，其余 = 必须修）绑在一起；工具若新增别的「过期类」退出码，要同步改脚本。`markedregion.ErrStale` 是目前唯一退 1 的来源。
- `docs/agents/parallel-sessions.md` 是共享手写文档，本笔动了一处句子和一段，别的作者同时改「生成物不占号」一节会冲突。
- `.github/workflows/ci.yml` 没法在本机真跑 GitHub 的表达式求值；模拟里的 `if` 求值是我写的一个只认本仓现有三种表达式的小解释器，语法正确性靠 YAML 解析与这份对账，不等于 Actions 的实际行为，首个 PR 的 run 才是最终确认。

### 验证

- `go build ./... && go vet ./...` 绿；`gofmt -l .` 无输出；`cd tools/mechanism-inventory && go vet ./... && go test ./...` 绿。
- `TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh ./tools/... ./internal/architecture/...`：`tools/adr-index`、`tools/at-coverage-counts`、`tools/internal/markedregion`、`internal/architecture` 全 ok。
- `python3 -c 'import yaml; yaml.safe_load(open(".github/workflows/ci.yml"))'` 通过，static job 各步的 `name` / `if` 如上。
- 在本树实跑 `bash scripts/ci/generated-artifacts-pr.sh` 退 0、三行「与提交的一致」、`git status` 不新增改动。
- 变异（在上面的一次性仓库里，每个变异独立提交到临时分支，场景集按下表重跑）：
  - 脚本把退 1 当红 → 「只 ADR 索引过期」「只 AT 计数过期」两个 PR 场景由绿变红，被杀。
  - 脚本把退 2 当绿 → 标记坏、缺行两个 PR 场景由红变绿，被杀。
  - 脚本去掉包加载报错的判据 → 语法错场景 PR 由红变绿，被杀。
  - `ci.yml` 去掉 `Mechanism inventory is current` 的 `if` → PR 上新增文件的场景由绿变红，被杀。
  - `ci.yml` 把 PR 步的 `if` 改成 `push` → PR 上标记坏、缺行场景由红变绿，被杀。
  - 脚本里改用 `go run ... -check` → 标记坏、缺行两个 PR 场景由红变绿（退 2 被压成 1 当过期），被杀；这一条同时证明了代决 4。

### 剩余项

无阻塞项。上面几条越权风险点留给复核；首个带 PR 的推送要看一次 Actions 里两种事件各自跑的步骤是否如表。
