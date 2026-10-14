# 21 作业设备册查阅页：只看已登记的行

Category: enhancement
Status: 完工，待评审与重放——2026-10-14 15:39 通道 2。此前 2026-10-14 15:13 通道 2 认领（用户指定 ← 通道 1），分支 mcp2-oc21，基 a3187272
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：`internal/accessidentity` 的设备册查阅行与其 postgres 读口、`internal/accessidentity/adapters/http` 的只读查阅处理器、`apps/admin-web` 的作业设备册页。不改 `cmd/parcel-api/endpoints.go` 与 `assemble_*.go`。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定二与越权风险点 2；[10](./10-device-register-and-operation-fact-capability-face.md) 的设备册（机制半边已进 main）。

## 做什么

1. 设备册增一个按租户列出已登记设备的读口。每行只有设备标识、作业范围、登记还是停用；停用了的带停用时刻。不出型号、序列号、使用人、健康状态。
2. 一个只读的 HTTP 查阅端点，走操作者渠道。租户只取自信封，不采信请求。能力面在判断项里定。没配发行方答未配置；没有查阅读授予答未授予。空册答空列表，不答未配置。
3. 管理台一页，只读这些行。先例是价卡目录页。词用册上的原词（登记、停用、作业范围），不自造译法。

## 不做

不做设备管理，不扫描现场，不发设备标识，不校验设备健康，不改登记口的语义。不做 operator-channel/10 余下半边（作业事实换口、撤隔离放行）。不改 `internal/transportfulfillment` 与 `internal/nodeoperations` 的 `work_fact_intake.go`。

## 完成判据

- 读口按租户交回行，别的租户的设备不在其中；停用行带停用时刻，登记行不带；行上没有型号、序列号、使用人、健康状态，也没有依据。
- 查阅端点：发行方未配置答未配置；没有主数据与运营查阅读授予答未授予；空册答空列表；请求里的租户不被采信。
- 管理台一页只读这些行，用登记、停用、作业范围这三个词。

## 判断项

1. **能力面取「主数据与运营查阅读」，不取「作业事实登记」，不判准入范围。** 这是册的查阅，不形成作业事实。候选已由派单点名。越权风险点 · 待 owner 复核。
2. **查阅行不带依据。** 依据留在登记行上，不进查阅列。派单写明每行只有设备标识、作业范围、登记还是停用，以及停用时刻。越权风险点 · 待 owner 复核。
3. **一次交回该租户册上的全部行，不设页大小，按设备标识排序。** 页大小会把「册上没有」和「被截掉」混成同一张空脸。排序只为两次读回次序稳定，不表示业务优先级。越权风险点 · 待 owner 复核。
4. **线上状态词用 `REGISTERED` / `DEACTIVATED`，页面显示登记 / 停用；作业范围种类用 `NODE` / `OFFSITE`，页面显示节点 / 场外作业范围。** 页面不自造「启用」「在线」这类词。集外取值原样显示。越权风险点 · 待 owner 复核。

## 完成记录（2026-10-14，分支 `mcp2-oc21`，基 `a3187272`；代码 tip `08bbd3bf`；待非作者评审与重放；证据只记 `S`）

**各笔**

| 笔 | 内容 |
|---|---|
| `090d5067` | 认领 |
| `08bbd3bf` | 查阅行与按租户读口、只读 HTTP 处理器、管理台页 |

**完成判据逐条**（钉 `08bbd3bf`）

- ✅ **读口按租户，停用行带时刻，登记行不带，行上没有依据**：`TestCatalogueRowKeepsOnlyTheListedFields`；真库 `TestListDevicesReturnsOnlyThatTenantsRows`（甲租户两行按设备标识排序，停用行时刻读回，乙租户同名设备不在其中，空册是空切片，空租户拒绝）。查阅行的类型没有依据、型号、序列号、使用人、健康状态这些格。
- ✅ **查阅端点的四格**：`TestQueryWorkDevicesAnswerGrades`（未配置、令牌不过、未授予、依赖不可用、读册失败不伪装成空册）；`TestQueryWorkDevicesEmptyCatalogueIsAnEmptyList`；`TestQueryWorkDevicesListsTheEnvelopeTenant`（请求里的租户与请求体都不被采信）。`TestCatalogueAuthenticatorAsksForOperationsReadWithoutAdmission`：铸造请求的能力面是主数据与运营查阅读，不带租户、不带准入；只有作业事实登记授予答未授予；发行方未配置答未配置。准入范围用的是未配置实现，查阅仍铸得出信封。
- **管理台一页已立，请求未接**：列是设备标识、作业范围、状态、停用时刻；状态显示登记 / 停用，作业范围种类显示节点 / 场外作业范围（`presentation.test.ts`）。端点表待接：不改 `cmd/parcel-api/endpoints.go` 与 `assemble_*.go`。路径字面量因此不写进管理台源码，页面不发请求，打开后显示「端点表待接」。处理器路径常量是 `WorkDeviceCataloguePath`。

**落点**

- 读口：`DeviceCatalogue` / `DeviceCatalogueRow`，postgres `ListDevices`。
- 处理器：`internal/accessidentity/adapters/http` 的 `NewQueryWorkDevicesEndpoint`。未注册进端点表，生产路由到不了它。
- 页面：`apps/admin-web` 导航「作业设备册」，不在 `liveIds` 里。

**判断项**（票面「判断项」四条，作者替 owner 定了，标「越权风险点 · 待 owner 复核」）照原节，本记录不另写第二份。

**验证**（作者那层；钉 `08bbd3bf`；WSL；DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`）

- `go build ./...` 退 0，`go vet ./...` 退 0。改过的 `.go` 经 `gofmt -w` 后 `gofmt -l` 无输出。
- `go test -count=1 -p 1` 带 DSN：动过的 `internal/accessidentity`、`adapters/http`、`adapters/postgres`，加 `go list -test` 反查的反向依赖（`adapters/oidc`、`adapters/pilotgovernance`、八个上下文的 `adapters/accessidentity`、`cmd/parcel-access-register`、`cmd/parcel-api`），加 `./internal/architecture/...`。15 个包都 ok，0 FAIL。**绿（含 PG）**：DSN 已设，postgres 包与两个 `cmd` 的真库用例没有跳过。`-v` 单跑 `TestListDevicesReturnsOnlyThatTenantsRows` 为 PASS。
- 管理台三道门（Node 22）：`tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 退 0（fail 0）；`pnpm exec vite build` 退 0。

**未做**

- 端点表那一行。通道 4 正在改 `endpoints.go` 与 `assemble_*.go`，本票不碰。页面因此调不通，收在「端点表待接」。

**未验**

- 处理器没有挂到正在听的 `parcel-api` 上，没有从管理台打到真库的贯穿。
- `-race` 没跑。

**评审**：不自评。不推 main。

## 端点表已接（2026-10-14，通道 2；代码 `c7d176ef`）

`origin/main` 已到 `fd458298`（operator-channel/10 余下半边已进 main）。本分支先合并该 tip（`b39bba7d`），再把查阅行接到端点表。用户说现在可以接。

- `GET /work-devices` 进 `assembleBusinessEndpoints`。生产装配用 `buildWorkDeviceCatalogue`：同一只操作者铸造器、同一本设备册。没把处理器交进来时答未配置，装配测试因此仍能钉「端点在」。
- 管理台改为真请求，页进 `liveIds`。空册、未配置、未授予仍走查阅页那套状态。
- `TestWorkDeviceCatalogueAnswersFromTheOperatorChannel`：发行方未配置、无令牌、只有登记写授予、册读失败、空册五格都过。`TestEveryAssembledEndpointAnswersUnconfigured` 含这一行，PASS。
- 验证：`go build ./...` 退 0；`go vet` 动过的 `cmd/parcel-api` 与 `internal/accessidentity/...` 退 0；带 DSN `go test -count=1 -p 1 ./cmd/parcel-api/ ./internal/architecture/` 退 0。管理台 `tsc`、`run-tests.mjs`（fail 0）、`vite build` 退 0。清点重生成，接入面端点 140，这一行归 accessidentity。证据只记 `S`。

## Comments
