# 07 朝外的取面单端口不存在，四处形态差异要先进类型

Category: enhancement
Status: resolved——出向缝落 `internal/platform/outbound`，取面单端口落 `parcelshipment/ports`，四处差异各有类型落点；曾与另一会话撞同一条缝，对方撤回后其两处更好的做法已并入，见文末「完成记录」与「撞车与收口」
Blocked by: 02（已 resolved，落 [ADR-0090](../../../docs/adr/0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)）

## 缺口

`internal/parcelshipment/ports/ports.go` 里与面单交易有关的端口只有两个，**都朝内**：
`LabelTransactionRepository`（存取聚合，其文档注释自称「本口今天没有生产写入方」）与
`LabelTransactionViews`（读面）。[能力形状盘点](../capability-shape-inventory.md)第二段按
`^type \w*(Gateway|Client|Channel|Carrier|Courier|Provider)\w* interface` 扫 `internal/`，
命中没有一个是「向末端渠道发起取面单请求」；`adapters/` 下的目录也全是内部上下文与传输/持久化，
**无渠道方向**。

## 做什么

只定端口形状，**不接任何真渠道**。盘点从公开开发文档量出四处形态差异，它们指向类型而不是取值，
端口要能容下而不被撑破：

1. **取面单与下单是否同一次调用**——FedEx/UPS/USPS 同次回单号与面单；UniUni 分三次
   （`create` 落 DRAFT → `purchase` 才出 `trackingId` → 另一次调用才取面单）。
2. **一次结果是否只有一份图件**——UPS 明确不是（`GraphicImage` 之外另有 `HTMLImage`、
   签名图件、`pdf417`）。
3. **面单粒度是否恒为包裹**——UniUni 的 `labelType=batching` 粒度是**批**不是包裹。
4. **成败信号在哪一层**——UniUni HTTP 一律 200，成败在 body 的 `code` 上，
   **HTTP 状态码不能用来判结果不确定**。

第 4 点与出向结果代数直接相接：**照 ADR-0090 办，不自己另定一套**。该记录明写「有没有形成
答案」由每家适配器判定、平台层不得看 HTTP 状态码代答——UniUni 恒 200 正是它举的反例。

**本票同时是 ADR-0090 平台包的落地处**：出向缝的代码随第一个消费者一起落 `internal/platform/`，
`15` 复用同一个包。若发现三格代数装不下取面单的某种情形，回票 `02` 重开，不在本票私自加格。

## 红线

- 不写任何渠道账号、字段名、报价、DPI（`PAR-INT-02`/`PAR-SET-03`，实例半边）。
- 不为某一家的形态把端口写窄——上列四处差异是端口形状的验收题，不是可选项。
- 真渠道适配器按[渠道适配缝备忘](../../../docs/design/channel-adapter-seams-design-note.md)
  「一类数据一张票」逐家另立，不在本票。

## 完成判据

端口形状定义在 `internal/parcelshipment/ports/`，四处差异各有类型上的落点；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第二段（含六家渠道公开形态表）；票 `02`。

## 完成记录

2026-09-02 由 MCP-1 落地，两处：出向缝 `internal/platform/outbound/`（ADR-0090 决定七要的共用
落位），取面单端口 `internal/parcelshipment/ports/label_channel.go`。不接任何真渠道，全仓仍无
一行出向 HTTP。

### 四处差异各自的类型落点

| 差异 | 落点 |
|---|---|
| 取面单与下单是否同一次调用 | `LabelDocumentAvailability` 分`随提交回件`／`待另一次取件`两格，另有独立的 `FetchLabelDocuments` 操作 |
| 一次结果是否只有一份图件 | `Documents []LabelDocument`，每份自带 `Role` |
| 面单粒度是否恒为包裹 | `LabelDocumentGranularity` 两格 + 每份件自带 `CoveredParcels` |
| 成败信号在哪一层 | 三个方法一律交回 `outbound.Disposition`，签名里没有任何 HTTP 类型可交 |

`LabelDocumentAvailability` 开了第三格 `LabelDocumentsNotProducedByChannel`，不是凑数：公开
文档里存在「受理了但按约定不回图件」与「回的是取件码而非面单」。**「取了面单」与「回了图件」
不是同一件事**，压成两格会把一次正常受理读成取件失败，从而触发一次本不该有的重取。

### `Disposition` 五格，而失败仍然只有三格

ADR-0090 明写不得私自加格，因此这一处要讲清。三格失败之外的两格各有出处：`Accepted` 只作
判别用而答复内容仍归各端口自己的成功类型（该记录原话），`NotConfigured` 是它决定五明写要
「自成一格作答」的未配置格。

收进同一个封闭集合而不是另设布尔，是为了让矛盾状态**表示不出来**——分开写就有
`已受理 == true` 与`对端拒绝`同时成立的那一格，而类型允许的状态迟早有人写出来。

### 举证门：`确证未受理`交不出来，除非举得出实据

`Outcome` 是不透明结构体、只经本包构造器产生。`NotAccepted(evidence)` 举不出实据时**降级**为
`答案未确定`——这是 ADR-0090 决定二的 fail-closed 方向做进结构，而不是写在注释里靠每家适配器
记得遵守。适配器要逐家写，规则却只有一条，强制点因此该在包里而不是在每一家的评审里。

降级而不是报错：报错会让适配器在举不出实据时无路可走，多半随手编一个实据字符串填上，那样这道
门就成了摆设。

`Reject` 不要求举证，与它不同——那一格的前提是对端**确实答了**，答复本身就是实据；有渠道只回
一个拒绝而不给原因，硬要求会逼适配器编。

端口因此收 `outbound.Outcome` 而不是裸的 `Disposition`，各结果结构体上原本那个 `ReasonReference`
一并去掉：依据引用归 `Outcome.Evidence()` 一处，留两处早晚对不上。

### 出向缝门禁：状态码的破法编译期不报

`internal/architecture/outbound_seam_gate_test.go` 禁止 `platform/outbound` 及其子包导入
`net/http` 与 chi。写成门禁是因为这条的破法看不出来——**顺手在缝里读一下状态码**编译期不报、
测试里也不报，它答出来的恰好就是当前唯一走得到的那一格正确答案，等第二家渠道（比如恒 200 的
那种）进来才发现，那时错误读法已经被两条链继承走了。

配套第二条测试直接测分类谓词本身：第一条门禁今天扫的包本来就不导入传输层，它**恒绿**，绿证明
不了谓词选对了包。谓词选空会被 `selectSources` fatal，但选多或选偏照样绿。

### 两条纪律做进结构，而不是留给人记住

`AdmitsResend()` 只对`确证未受理`为真，`RequiresQueryToSettle()` 只对`答案未确定`为真。
ADR-0090 说这是整条链上唯一有资金后果的判错方向，而「记住不要重发」这种约束靠人守不住。
四条测试都扫完整个 `uint8` 值域而不逐格写死：逐格写死时将来多一格会默认落进 `false`，没有
任何东西提醒作者去想清那一格——恰恰是这一想漏了会花钱。另钉了未知取值 fail-closed（会从库里
回读，也会从另一版本的适配器传进来）与两谓词互斥（各自为真而合起来失守，正是重复购买的入口）。

### 查询口：`Support` 与 `Disposition` 分开

ADR-0090 决定六要求查不了的如实记为「该源无查询口」。它单独成格而不是让查询交回一个失败
处置，因为两者续办完全不同：查询失败可以再查，无查询口只能交人对账，再查一万次也没有答案。

`LabelSubmissionQueryOutcome.Disposition` 说的是**原提交**落在哪一格，不是这次查询调用本身的
成败——这一处最容易读混，注释里点名了：查询调用自己超时该再查一次，原提交查出`确证未受理`
才是准许重发的那一格。

### 没写 ports 测试，这是按约定不是省事

全仓 `internal/*/ports/` 零测试文件——ports 包只放记录结构与封闭枚举，构造门在 domain、行为由
消费方钉。本票照此办：有行为的只有平台包，测试落在那里。新增枚举的 `String()` 由架构包那道
枚举门禁覆盖（已随本笔跑绿），平台包另有一条运行期双向扫描与它互补——那一条是语法扫描，只看
常量名被提到过。

### 守不住的那一格，写在注释里而不是假装守住了

`ChannelConfiguration` 拦不住一个绕过它、把地址写死在自己包里的适配器——本包不发起调用，没有
这个位置。它能保证的只有：凡是收它作入参的适配器，配置不齐时手上没有地址可去。评审出向适配器
时先看它走不走这条入参。这一层只能靠看，注释里如实这么写了。

### 验证

`gofmt -l` 空，`go build ./...`／`go vet ./...` 退 0，`go test -count=1 ./...` 退 0 **含真库**
（DSN 已设，同刻抽验 `TestFreezeScopesAreInvisibleToEachOther` 为 `PASS` 不是 `SKIP`）。
架构包全部门禁另单跑一遍绿（新增枚举过枚举门禁，`ports → platform` 的方向过边界门禁）。

上面这一跑在并入举证门与门禁之前。**并入之后又整跑一遍**，同样口径同样结论：`gofmt -l` 空，
`go build ./...`／`go vet ./...` 退 0，`go test -count=1 ./...` 退 0 含真库（同刻抽验
`TestFreezeScopesAreInvisibleToEachOther` 为 `PASS`）；新加的两条出向缝门禁单跑 `-v` 确认实跑
到而不是被跳过。

## 落地 SHA 与一次提交竞态

**本票实现落在 `1b7bd0d`，但那一笔的提交信不是它的。** 提交时撞上共享索引的竞态：本会话
`git add` 之后、`git commit` 之前，另一会话先跑了 `git commit`，而 `git commit` 提交的是**整个
索引**，于是本票暂存的十个文件被一并卷进它那一笔，标题是它自己的
「机制半边现状的三处接线叙述已失效」。本会话随后那次 `git commit` 因索引已空而扑空。

内容完好（逐文件比对 `git diff HEAD` 全空），丢的只是可检索性：按提交信找不到这一千行出向缝。
未改写历史——那一笔已含另一会话的改动，重写会动到别人的东西。此处记 SHA 以补回指向。

**下次用 `git commit --only -- <paths>`。** 它只对给定路径取快照、不理会索引里的其余内容，正是
共享工作树该用的形状；`git add` 加 `git commit` 这个组合在共享索引上先天带这条竞态，而
[并行会话](../../../docs/agents/parallel-sessions.md)「精确暂存」那一节记的是**卷走别人的**，
这一次是反过来——**被别人卷走**，同一个窗口，方向相反。

## 撞车与收口

本票落盘时撞上另一会话在建**同一条** ADR-0090 出向缝，落 `internal/platform/outboundcall`，其
`outcome.go` 的 mtime 与本票的 `disposition.go` 相隔 **9 秒**；对方同笔还加了
`internal/architecture/outbound_seam_gate_test.go`，把 `platform/outboundcall` 按段写死为决定七
指定的位置。两份都未提交、都编得过、测试都绿、都还没有消费方。

这正是 ADR-0090 要防的那件事——票 `02` 的全部理由就是「两条链各答一次答案就会分叉」，而分叉
出在了缝本身上。**地盘只有人能分**，故当时停手：不自行合并、不改名、不动对方文件，并把自己的
文件从共享索引里撤下（共享工作树上索引是共用的，对方一提交会把暂存的东西带走）。上报之后对方
撤回了自己那份，`outboundcall` 与那道门禁一并从树上消失。

**对方两处比本票原稿好，已并入，不随撤回一起丢掉**：不透明 `Outcome` 加举证门（见上），以及
出向缝的传输层门禁（见上，重建时改指 `platform/outbound`，并保留其「谓词自身也要测」那一条，
那是本仓「不同的绿」那类教训的又一面）。

**本票原稿三处对方没有，予以保留**：代数完整——对方常量只有`答案未确定`、`确证未受理`、
`未配置`三个，缺 ADR-0090 结果代数表第一行的`对端已答复且答复是拒绝`，而那一格恢复动作是
「不重发，交业务续办」，与另两格都不同，缺了它渠道明确拒收无处可落；两条纪律的谓词；以及未配置
判据的范围（对方 `Admit` 只判超时，本票是端点位置、凭证位置、超时三样缺一即未配置）。本票的
主交付——取面单端口与四处形态差异的类型落点——对方那边本就没有。
