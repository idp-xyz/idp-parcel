# 04 前后端共享契约夹具——让 TS 响应类型对着后端真答出的 JSON 校验

Category: enhancement
Status: resolved（2026-09-03，MCP-2，`cb1e4d5`；首切片 networkrouting，余下上下文见 Comments）
Blocked by: 01（已 resolved，`d722301`）

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

- 2026-09-03 · MCP-2：**落于 `cb1e4d5`，转 resolved。** 首切片取 `networkrouting` 的 `GET /route-plans`
  两册（`initial-route` / `reassessment`）。

  **两条路里选了第一条**（前端直接读 Go 侧 `testdata/` 那一份）：只有一份文件，不立第二份拷贝
  也就不需要再加一道「两份一致」的门禁；代价是 `node-builtins.d.ts` 补 `node:fs` 的 `readFileSync`
  一格，票面已预期。路径相对 `apps/admin-web`，靠 `scripts/run-tests.mjs` 把 cwd 钉在那里。

  **票面写的 `satisfies <ResponseBody>` 做不到，换了个等价物。** 运行时 `readFileSync` + `JSON.parse`
  读进来的是 `unknown`，编译期对它 `satisfies` 没有意义；而改成 `resolveJsonModule` 导入又要求文件
  在 `rootDir` 内（即复制进前端树，回到第二条路），且 JSON 导入把 `"INITIAL_ROUTES_LISTED"` 拓宽成
  `string`，对 `outcome` 这类字面量联合照样 `satisfies` 不过。于是把**类型的键清单写成值**并让
  编译器替清单作证：`as const satisfies readonly RequiredKeys<T>[]` / `OptionalKeys<T>` 证清单不多列、
  不错格，`Exhaustive<T, Listed>` 证不漏列（漏了哪个键，报错里就写哪个键）。运行时再拿清单核夹具：
  必备键不缺、没有类型不认识的键、值为字符串、每个可缺席键至少出场一次；`outcome` 用
  `'INITIAL_ROUTES_LISTED' satisfies …['outcome']` 钉住封闭集那一格。

  **反证（完成判据第二条），在 detached worktree 检出 `cb1e4d5` 上做**：
  - TS 侧把 `InitialRouteRecord.declaredParcelId` 改成 `declaredParcelID`：`tsc` 红两处——
    `TS2820 '"declaredParcelId"' is not assignable to 'RequiredKeys<InitialRouteRecord>'` 与
    `TS2322 'true' is not assignable to '"declaredParcelID"'`（后者就是 `Exhaustive` 把漏的键名摆出来）。
  - Go 侧把 `initialRouteBody` 的 tag 改成 `declared_parcel_id`：**先红在既有的逐键断言**（`formed["declaredParcelId"]`），
    夹具比对还没轮到——这是真实流程里 Go 作者必然要先跟的那一步；把断言也改过去之后，夹具比对红并
    点名 `testdata\route_plans_initial_route.json`、提示 `-update`；`-update` 写回后前端红：
    `响应体.judgments[0] 缺必备键 declaredParcelId——后端把它改名或去掉了，TS 类型还当它必在`（17/18）。
    三处都 `git checkout` 改回，worktree 干净后拆掉。

  **复核册的替身多了一行已改路（`REROUTED`）**：原测试只有一行失效，`rerouteState` 按设计缺席，前端
  「每个可缺席键至少出场一次」那条会红在夹具没覆盖上——那不是前端错，是夹具没给实例。加行之后
  传输测试顺带多了 `rerouteState` 的转写断言。

  **评审两轴**（基线 `757b036`）：Standards 拿住一处假话（Go 注释「四个可缺席键各有在场与缺席的实例」，
  实际只有 `rerouteState` 兼有缺席实例）与一处重复（两条测试各写一遍读→逐行→字符串→可缺席覆盖），
  均已修：注释改真，逻辑收成 `assertShape` / `assertRows` / `assertOptionalKeysCovered` 三个按形状
  递归的小函数，行数组与嵌套对象在 `KeyShape` 里各占一格。Spec 无缺项。

  **未做且要说清**：票面点名的 `parcelpricing` 这一侧没动——另一会话此刻正在 `pricing-reference-series-operations`
  票组里写 `internal/parcelpricing`（树上有其在途改动），按地盘纪律不进；`partycommercial` 按票面等 MCP-5
  释号。要补时照本切片的形状：包内一份 `contract_fixture_test.go`（或抽到共享测试包）+ 前端一个
  `*.contract.test.ts`，每个上下文一票或一并一票由认领人定。

  **验证**：共享树上 `go test ./internal/networkrouting/adapters/http/ -count=1` 绿、`go vet` 退 0、
  `gofmt -l` 空、`go test ./internal/architecture/ -count=1` 绿（路径门禁认得测试名里的
  `GET /route-plans?register=…`，在端点表上）、`tsc --noEmit` 无输出、`pnpm test` 18/18；detached worktree
  检出 `cb1e4d5` 同结果。**未含 PG 与 -race**：本笔没有 postgres 适配器改动，那一层无从验。提交时树上
  另有 `pricing-reference-series-operations` 票 03 与 `scripts/demo-seeds` 两份 JSON 的在途改动，属另一
  会话，按 pathspec 提交未带走。
