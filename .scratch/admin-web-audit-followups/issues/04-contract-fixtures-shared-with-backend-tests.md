# 04 前后端共享契约夹具——让 TS 响应类型对着后端真答出的 JSON 校验

Category: enhancement
Status: ready-for-agent
Blocked by: 01

## 缺什么

前端 `*ResponseBody` / `*Record` 类型全部手写；后端 JSON 形状由各 `query_*_test.go` 的断言钉住。
两侧之间没有共享物，字段改名、加格（票 02 的 `CREDIT_POLICY`、在途的价格政策 `caliber`）都要靠
人工发现。仓里没有 OpenAPI，也不该为此临时造一份第二口径。

## 形状

最小做法：后端测试把它断言过的响应体**原样落盘**到 `internal/<ctx>/adapters/http/testdata/*.json`
（Go 侧一个 `golden` 写法即可），前端测试读同一份文件做 `satisfies <ResponseBody>` 的编译期校验
加一次运行时判别（`kind` / `outcome` 在封闭集内）。

## 阻塞与地盘

- 后端半边落在各上下文的 `adapters/http`，其中 `internal/partycommercial/` 此刻是 MCP-5 地盘。
  本票先做**不在他人地盘**的上下文（`parcelpricing`、`networkrouting`），商业侧待释号。
- 前端半边要 `node:fs` 读 `.json`——票 01 的最小声明要补 `readFileSync` 一格，或改为把夹具
  复制进 `apps/admin-web/src/test/fixtures/` 由 Go 侧门禁比对两份一致。两条路由实施票裁。

## 完成判据

- 至少一个上下文的列表响应有共享夹具，改任一侧字段名两侧都红。

## Comments
