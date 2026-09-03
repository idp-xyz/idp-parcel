# 03 门禁：管理台发出的每条路径都在 parcel-api 端点表上

Category: enhancement
Status: resolved（2026-09-03，MCP-2 撰写、MCP-1 接手收口，`2a9a76a`）
Blocked by: 无

## 缺什么

审查在 `0d492b8` 上逐条比对过：前端 `apps/admin-web/src/pages/**` 里所有 `'/xxx'` 路径字面量都
在 `cmd/parcel-api/endpoints.go` 的 `[]httpapi.BusinessEndpoint` 表上（0 悬空）。但这是一次人工
比对，下一次端点改名、前端多写一条，没有任何东西会红——与 `production_wiring_baseline` 要治的
「未接线看着像已接线」同形，只是方向反过来：**调用看着像有人接**。

## 落哪

`internal/architecture/` 已有一族走全树、解析源码的门禁（`boundaries_test.go`、
`production_wiring_ratchet_test.go` 等）。新增一道 `admin_web_endpoint_consumers_gate_test.go`：

- 以 `go/ast` 取 `cmd/parcel-api` 生产文件里 `[]httpapi.BusinessEndpoint` 字面量的 `Pattern`
  （认法照 `tools/mechanism-inventory/wiringcensus.go`：按类型认字面量，不按函数名）。
- 扫 `apps/admin-web/src/**/*.ts` 与 `*.tsx`，正则取以 `/` 开头、出现在引号或模板串里的路径
  字面量，去掉 `?` 之后的查询串；排除 `/api`、`/oidc`、`/auth/callback` 这三条前端侧约定
  （`vite.config.ts` 与 `oidc.ts` 拥有它们）。
- 断言：前端集合 ⊆ 端点表集合。差集非空时逐条报「前端在 <file> 发向 <path>，端点表无此行」。

**不反向断言**（端点表有、前端无）：六个渠道/一线/外部集成入口按设计不在管理台，反向会永远红。

## 为什么不放前端

前端侧读 `.go` 文件要 `node:fs`，而票 01 的底座刻意不为 Node 内建写替身声明超出必要；Go 这边
`go/ast` 是现成的，且这道门禁与其余门禁同跑同红。

## 完成判据

- `go test ./internal/architecture/ -run AdminWebEndpointConsumers -count=1` 绿。
- 反证一次：临时把某条前端路径改错，门禁红并点名文件与路径；改回。

## Comments

- 2026-09-03 · MCP-1：**落于 `2a9a76a`，转 resolved。** 门禁由 MCP-2 按「落哪」一节写成，三条测试：
  主门禁（前端 ⊆ 端点表）、识别谓词对前端实际写法逐种认对（含必须不认的：`/oidc`、`/auth/callback`、
  `/api`、正则、带大写或下划线的串）、合成装配源与前端片段证明门禁判得出红。MCP-2 会话在 gofmt
  与反证那一步中断（转录停在 13:45:15），我接手做验证与收口，代码一行未改。

  **完成判据第一条**：写的是 `-run AdminWebEndpointConsumers`，而三条测试名里没有这个串（它们叫
  `TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable` 等，按本包「测试名是一句话」的惯例起的）；
  实际用 `go test ./internal/architecture/ -run AdminWeb -count=1`，三条 `PASS`。判据那条正则是票面
  先于测试名写下的，以测试名为准，不为凑正则改名。

  **完成判据第二条（反证）**：在 detached 临时 worktree 检出 `2a9a76a`，把
  `apps/admin-web/src/pages/collection/api.ts` 里 `'/collection-subledgers'` 改成单数，门禁红并点名：
  `apps/admin-web/src/pages/collection/api.ts：管理台发向 /collection-subledger，parcel-api 端点表无此行`；
  `git checkout` 改回后再跑绿。反证在 worktree 里做而不在共享树上做，是因为第一次做时踩了一个坑，
  写在这里免得下一个人重踩：**PowerShell 的 `Set-Location` 不改 .NET 进程的当前目录**，
  `[IO.File]::WriteAllText` 收到相对路径时写的是进程启动目录——于是「在 worktree 里改一条路径」
  实际改到了共享树上那份 `api.ts`，而 worktree 里的门禁照样绿。共享树上那份已当场 `git checkout`
  改回（前后各查过 `git diff HEAD` 为空）。给 .NET IO 一律传绝对路径。

  **验证**：共享树上 `gofmt -l internal/architecture` 空、`go vet` 退 0、三条测试 `-count=1` 绿；
  detached worktree 检出 `2a9a76a`：`gofmt -l .` 空、`go build ./...` 与 `go vet ./...` 退 0、
  `go test -p 1 -count=1 ./...` 零 FAIL（DSN 指向门禁容器，PG 用例真跑）。
