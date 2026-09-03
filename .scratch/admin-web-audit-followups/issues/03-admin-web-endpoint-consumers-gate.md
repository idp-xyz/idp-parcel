# 03 门禁：管理台发出的每条路径都在 parcel-api 端点表上

Category: enhancement
Status: ready-for-agent
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
