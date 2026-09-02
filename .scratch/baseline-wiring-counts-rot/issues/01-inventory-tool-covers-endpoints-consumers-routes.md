# 01 接线类计数留在叙述里必然烂——三处已实测失效，应并入生成物

Category: enhancement
Status: ready-for-agent

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
