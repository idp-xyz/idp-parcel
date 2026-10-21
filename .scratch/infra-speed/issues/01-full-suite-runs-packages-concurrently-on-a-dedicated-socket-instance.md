# 01 本机全量包并发：专用一次性实例，经 Unix socket 连

Category: enhancement
Status: resolved——2026-10-19，见文末「完成记录」
Blocked by: 无
地盘：`scripts/test-full.sh`（新）、`docs/agents/parallel-sessions.md`「全量只跑一次」节
出处：用户 2026-10-19 对「每次需要很久，科学吗」的答复「需要」（同意先做全量提速与热点清单两张基础设施票）；`parallel-sessions.md` 「再往下省时间要动的是夹具」那段自己留的「再回头量 `-p 1` 能不能放……这是一张票的量」

## 要什么

推送方进 main 前那一跑含 DSN 的全量，从包串行改成包并发，且不牺牲稳定性与证据口径。

## 完成记录

- **先量，再改。** 实测于 `fec2a2a1`，数字与解释写在 `parallel-sessions.md` 新段「`-p 1` 量过之后」，此处不复述。要点：串行 248 秒；共用 55432 的 TCP 包并发约 60 秒，但连跑第三遍有包在建连时 `connection reset by peer`（端口区间窄 + Docker 端口代理 + `TIME_WAIT`）；专用实例走 socket 包并发 45 秒，连跑稳定。
- **`scripts/test-full.sh`**：起一只与 `compose.yaml` 同镜像、同 tmpfs、不关 `fsync` 的一次性实例，socket 目录挂宿主临时目录的子目录（镜像入口会把它 chown 给容器内 uid 999），等「init process complete」那行再 `pg_isready`（避开初始化时的临时实例），默认 `-p 8`，收尾先 `docker stop` 让 PostgreSQL 自己删 socket 与锁文件。实测端到端 46 秒、153 ok，退出后无残留容器与临时目录。
- **CI 不动，但它 Test 步注释里有一句随本票变假**：「本机验证口径一直是 `-p 1`（AGENTS / parallel-sessions），CI 不该比本机并发」。本票原想一并改掉，推送时远端拒收（本机凭据缺 `workflow` 权限，不能改 `.github/workflows/`），按纪律不绕，改动已撤出本票。**2026-10-20 已补**（用户把这件交给本会话办，凭据补上 `workflow` 权限后与票 08 同一分支推上去）：把那句换成「本机全量 2026-10-19 起改走 `scripts/test-full.sh` 的专用实例包并发；那边放开凭的是本机实测（无 -race、本机的 CPU 与端口条件），搬不到 runner 上，这里的 -p 1 只凭上面这几笔账」。
- **代决**（用户授权「你作为业务和系统专家自决」）：本机全量放开 `-p 1`、改走专用实例。**越权风险点**：`parallel-sessions.md` 原写「放不放以 `ci.yml` 那种带 run 号的实测为据」，本票的依据是本机实测而非 CI run——放开的也只是本机，CI 照旧，口径一致；若 owner 认为本机也须等 CI 侧实测，回退只是推送方换回旧命令，脚本可留作子集用。
- **验证**：`scripts/test-full.sh`（全量）退 0、153 ok；子集模式（两个真库包）退 0；残留检查零。
