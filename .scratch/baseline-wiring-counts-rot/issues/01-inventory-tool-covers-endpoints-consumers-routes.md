# 01 接线类计数留在叙述里必然烂——三处已实测失效，应并入生成物

Category: enhancement
Status: resolved——MCP-4 认领并写完工具，崩溃后由 MCP-5 接续收口（2026-09-03）；SHA 与验证见文末 Comments

取证锚 `c60ec2c`（干净 detached worktree 上量得，非工作树——当时树上有多方在途改动）。

## 缺口

[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)的「机制半边现状」一节有三处叙述与代码不符，且三处**都是接线规模的数**：

| 该节原写 | 实测（锚 `c60ec2c`） |
|---|---|
| 九个接入面端点 | 七十二个 |
| `AcceptanceConsumer` 至今是唯一的消费者 | 消费适配器二十二个生产文件（VE 十三、PS 七、NR 两个） |
| 路由表今天只有一条 | 十四条 |
| 其余各格由 `unwired*` 守卫顶住 | `cmd/parcel-api/main.go` 零处引用 |

四处已在本票之外先行改正（见 Comments），但**改正本身治不了病**。该节自己写过「此前本行复述实现进度，已经因此烂过一次」，这是同一节第二次犯同一个毛病。

病因不是谁疏忽。该节已经把可由代码算出的数交给 [机制半边清点](../../../docs/product/MECHANISM-INVENTORY.md) 并写明「引计数一律引它」，CI 每次重新生成再比对，不一致即红——**那道门禁管得住它数的那几栏，管不住它没数的**。接入面端点、消费适配器与路由表条目恰好三样都不在生成物里，于是它们只能活在叙述里，而叙述没有任何门禁。

代价具体：AGENTS.md 的开工顺序第一步就是读该节。照旧数判断，会以为主链只通了一段、消费侧几乎没开，从而把已经接通的方向重新排进工作；这个错误乘以后续每一个会话。

## 做什么

扩 `tools/mechanism-inventory`，让它多出三栏，并在 `MECHANISM-INVENTORY.md` 落成表：

- **接入面端点**：`cmd/parcel-api` 装配的业务端点总数与按上下文分布。数据源取装配点而不是 `adapters/http/` 的文件数——后者数的是处理器，一个处理器可挂多个端点（TF 的 POD 双端点即是）。
- **消费适配器**：`internal/*/adapters/` 下 `inbox`、`adoptconsume`、`finalconsume`、`veconsume` 四类目录的生产文件数，按消费方分布。
- **派发路由表**：`cmd/parcel-dispatch` 直投路由表的条目数。

三栏落定后，把该节正文里这三处改成引生成物，正文不再自带数——与该节对其余计数已经采用的做法一致。

## 完成判据

- `tools/mechanism-inventory` 生成的报告含上述三栏，`go vet`／`go test` 在该模块内退 0
- CI 的 `Mechanism inventory is current` 一步在扩栏后仍然绿（生成再比对为空）
- 开发主线该节这三处不再自带数，改为引报告
- 故意改一处接线（新增一个端点或一条路由）后重新生成，报告随之变化——证明它真的在数活的东西而不是抄了一份常量

## 注意

生成工具属「扫当前树再写回去」那一类，[并行会话](../../../docs/agents/parallel-sessions.md)点名此类命令在共享树上不得单方面跑。先例见 `e02b7ca`：它在 `HEAD` 的干净 detached worktree 里另生成一份比 SHA256，证明生成物是 `HEAD` 的纯函数、没夹带别人未提交的在途工作。本票照办。

## 相关

- [`first-tenant-runway/04`](../../first-tenant-runway/issues/04-mechanism-inventory-as-generated-report.md) 立的就是「清点改为生成物」这个模式，本票是它的自然延伸——那一票停在文件与端口两类计数上，没有覆盖接线类。
- [ADR-0049](../../../docs/adr/0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 定的「路由表只随消费者一起长」仍然有效，本票不改这条裁定，只让它的当前规模可被数出来。

## Comments

- 2026-09-02 · 先行改正已落在开发主线该节：三处叙述改为如实记述并各带取证锚 `c60ec2c`，同时保留原句里仍然成立的理由（ADR-0049 第三条的登记克制、`unwired*` 当初要守的稳定码）。改正未动任何计数生成物，也未动 `MECHANISM-INVENTORY.md`。本票要办的是防复发那一半。

- 2026-09-03 · MCP-4（开工前在 `371f6cb`——本地 main HEAD，非票面旧锚——重取一遍证据；只取证不改代码）。

  票面不过期。`tools/mechanism-inventory` 现有栏：逐上下文文件面（生产／测试／应用编排／postgres 适配器／其中 Outbox 投递／http 适配器）、`cmd/` 生产与测试、跨上下文消费缝、迁移份数、端口两口径。票面要的三栏无一在内。

  一条要先说清的口径差，免得把「跨上下文消费缝」当成已经有了的「消费适配器」栏：`CrossSeam` 按 `internal/<消费方>/adapters/<提供方>/` 认，提供方必须是另一个上下文的目录名（`crossContextProvider`）；`inbox`、`adoptconsume`、`finalconsume`、`veconsume` 四个目录名都不是上下文，因此一个都没被数进去。两栏数的是两种东西——防腐层与消费门——都要留，不合并。

  派发路由表今天在 `cmd/parcel-dispatch/assemble.go` 的 `wireDispatcher` 里，是交给 `dispatch.NewDirectPublisher` 的那个 `map[eventing.EventType]dispatch.Consumer` 字面量，本次数得 14 条（与 `c60ec2c` 同）。接入面端点的装配点在 `cmd/parcel-api`（端点表），本会话尚未细读其形状。CI 的 `Mechanism inventory is current` 一步形状未变（重新生成后 `git status --porcelain` 该文件须为空）。

- 2026-09-03 · MCP-5（MCP-4 崩溃后接续收口；用户指示）。

  **归属先说清。** 工具那一半——`wiringcensus.go`、`wiringcensus_test.go` 与 `main.go` 的 `writeWiring`——是 MCP-4 崩溃前写在共享树上的在途文件（mtime 11:30–11:31），我接手时它们在本模块内 `go build`／`go vet`／`go test`／`gofmt -l` 已全部干净，**一行没改**，原样并入本笔。我做的是余下三件：在干净树上生成报告、把开发主线三处叙述改成引报告、活性验证与收口。

  **三栏的做法值得记一句**：三处都按**字面量类型**认（`[]httpapi.BusinessEndpoint`、`map[eventing.EventType]dispatch.Consumer`、`adapters/` 下四个消费门目录名），导入按路径不按别名，只做语法解析不做类型检查；归不到上下文的条目显成「（未归类）」而不吞掉。装配点改名、拆函数、挪文件都数得对，换一种字面量类型才数不到——而那种改动本来就该顺手回来改这里。

  **生成在干净 detached worktree 上做**，钉 `13c87bd`（当时的本地 main HEAD；共享树上此刻压着 MCP-2 在 `internal/partycommercial` 的大片未提交改动，直接跑会把它们数进去）。结果：接入面端点 74（PC 18、VE 13、CC 10、NR 9、PS 8、PP 5、SA 4、TF 3、NO 2、CR 1、PG 1，无未归类）、消费适配器 23（VE 13、PS 8、NR 2；inbox 19、adoptconsume 1、finalconsume 1、veconsume 2）、直投路由表 14（VE 7、PS 5、NR 2）。与票面 `c60ec2c` 的 72／22／14 相比，端点多两个、消费门多一个，都是这两天新接的。

  **顺带修了一处漂移，要写明它不是本票的功劳。** 已提交的报告相对 `13c87bd` 早已过期（文件面 651→667、迁移 103→106、端口声明 277→282 等）：昨夜到今晨主线上累了五十余笔无人推送，而「重新生成再比对」那道门只在 CI 上咬，本机没人重跑。本笔重生成把它带回一致；**它恰好证明了本票的病因**——没有门禁的数，哪怕是生成物，只要没人跑生成器照样烂，差别只在 CI 会不会在下一次推送时把它拦下。

  **活性验证**：在同一棵干净树上加一份含一条 `parcelshipment` 端点的生产文件与一份含一条路由的生产文件，`-ports=false` 重生成，报告随之变为端点 75（PS 9）、路由 15（PS 6），消费适配器仍 23；探针文件随即删除，树回到只有报告一处改动。**工具数的是活的接线，不是抄了一份常量。**

  **开发主线三处**：路由表条目数、消费适配器数、接入面端点总数都改为引[机制半边清点](../../../docs/product/MECHANISM-INVENTORY.md)「接线面」一节，正文不再自带这三个数；`unwired*` 零引用那句仍带 `c60ec2c` 锚留着——它不是计数，是一条锚了 SHA 的事实。「所有可由代码算出的数」那一句也把接线面三栏补进了清单。

  **验证**：`tools/mechanism-inventory` 内 `gofmt -l` 空、`go build`／`go vet`／`go test -count=1` 退 0；CI 那一步的模拟——在提交后的 detached worktree 上重新生成再 `git status --porcelain` 该文件——结果见下一条。未动主模块任何 `.go`／`.sql`，不涉真库。

- 2026-09-03 · MCP-5（收口取证）。本票落 `a3941b0`（父 `568163a`，其下自 `13c87bd` 起只压着一笔 `.scratch` 改动，无 `.go`／`.sql`）。CI 门禁的模拟在钉 `a3941b0` 的 detached worktree 上做：模块内 `gofmt -l` 空、`go vet`／`go test -count=1` 退 0；用**提交里的**工具重新生成报告后 `git status --porcelain -- docs/product/MECHANISM-INVENTORY.md` 为空，整棵树 `--untracked-files=all` 零行——报告是 `a3941b0` 的纯函数。未 push。四条完成判据逐条对过：三栏在报告里、模块内 vet/test 退 0、CI 那一步在扩栏后仍绿（模拟）、开发主线三处改引报告、活性验证 74→75／14→15 成立。
