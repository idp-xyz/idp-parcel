# 15 轨迹源的拉取/接收端口不存在，形态也没选

Category: enhancement
Status: resolved——MCP-5（2026-09-03；形态与端口形状裁于「Answer」，端口与替身落主线 `7904003`，取证见文末 Comment）
Blocked by: 02、03（均已 resolved：出向缝落 [ADR-0090](../../../docs/adr/0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)；收编方裁给 TF 新立一类事实）

## 缺口

[轨迹源盘点](../tracking-source-seam-inventory.md)第一段的结论是**整段无形状**，且零命中是
逐条搜过的：

- `17[Tt]rack|17TRACK|aftership|AfterShip|trackingmore|[Ww]ebhook` 扫全仓，命中全在 `docs/`
  与 `.scratch/`，`internal/`、`cmd/`、`migrations/`、`apps/` 一个都没有。
- 出向 HTTP 客户端全仓零匹配。
- 按出向接口命名扫 `internal/`，命中要么是防腐层朝内的 `*Source`，要么是发通知的
  `NotificationChannelGateway`（自称唯一实现是测试替身），要么是入向的 `accessidentity` 两口。
- inbox 机制承接的是内部上下文之间的事件：VE 侧十一个 `*_consumer.go` 逐个对应一个兄弟
  上下文，**无一来自进程外**。

## 做什么

定轨迹源的入站端口形状，**不接任何真实源**：

1. **形态选择**：轮询拉取、回调接收、还是两者都要。这一问必须答，因为两种形态的幂等与
   顺序保证完全不同——拉取要答「上次拉到哪」，接收要答「重复投递怎么办」。
2. 端口形状：一个源一个适配器，还是一个端口多家实现。17track 这类聚合平台一口给多家承运商
   的轨迹，而承运商直连是一家一口——**两者能不能共用一个端口，是本票的形状题**。
3. 出向调用的失败/超时/重试/幂等**照 [ADR-0090](../../../docs/adr/0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)**，
   不另定一套；平台包由 `07` 先落，本票复用同一个包。装不下就回票 `02` 重开，不私自加格。
   ADR 第六条要求配一个查询能力——**各轨迹源有没有查询口属本票取证范围**，查不了的如实记
   「该源无查询口」，不得以重发顶替。

## `03` 已裁定，据此收窄的交付形状

端口交出来的东西要能被收编。`03` 的裁决给了三条本票必须照办的约束：

- **收编方是 TF，且是新立的一类「外部承运轨迹事实」**，不是既有自营作业事实。端口交出的
  原始素材要能被译成那一类，不必迁就 `register_transport_handover` 之类的既有用例形状。
- **`OccurredAt` 缺失即拒收。** 因此端口必须能如实交出「本条素材没有发生时间」，
  **不得在端口这一层补一个**——补了收编侧就再也分不出源给没给。
- **`EffectiveAt` 由 TF 作为一次显式判断铸**，不归端口。端口不产出它，也不产出 `ReceivedAt`
  之外的任何本仓时间。

## 红线

- 不填任何账号、密钥、轮询频率、状态码表（`PAR-INT-02`，实例半边）。
- **端口不得直接产出投影**：它交出的是待收编的原始素材，不是 `AcceptedSourceFact`
  （[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md)
  「不直插投影」，今天由 `domain.SourceContext` 封闭五值守着，本票不许松它）。

## 完成判据

形态选择有明确答复；端口形状落地并有替身测试；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[轨迹源盘点](../tracking-source-seam-inventory.md)第一段；票 `02`、`03`。

## Answer（2026-09-03，MCP-5）

裁的是形状，不是任何一家源的取值。开工前在 `a3941b0` 重跑了盘点那几条零命中搜索
（`17[Tt]rack|17TRACK|aftership|AfterShip|trackingmore|[Ww]ebhook` 与出向 HTTP 客户端），
`internal/`、`cmd/`、`migrations/`、`apps/` 仍然一个都没有；全仓唯一的出向缝就是票 `07` 落的
`internal/platform/outbound`（不认识 HTTP，`internal/architecture/outbound_seam_gate_test.go`
守着）。本票在它之上定端口，不另起一套。

### 一、形态：拉取是首发形态；回调接收是第二形态，共用同一份素材，本票不建端点

两种形态的差别只在**素材怎么到达**，到达之后要交给收编方（TF，票 `03` 裁定）的东西是同一样：
一条**原始素材**。所以端口分两层——素材类型两形态共用，到达方式各自成口。

**拉取先做，理由有三，都不是「简单」**：

1. **每个源都有查询口，不是每个源都有推送。** 承运商直连几乎都是按凭证查询；聚合平台两者
   都有。拉取覆盖全部源，回调只覆盖一部分。
2. **ADR-0090 决定六要求每个出向端口欠一个查询能力，拉取本身就是那个查询能力。** 回调形态
   反过来欠一个查询口：推送到达的素材若要核对（漏推、乱序、对账），仍然要拉取。先立回调
   等于先立一个没有查询口的端口。
3. **回调要暴露一个公网入向端点并校验源方凭证**，那属 ADR-0055 的接入面（Intake、渠道注册、
   `PAR-INT-02` 的账号），今天没有一家真源，端点建出来只能挂未配置。它随第一家推送源立票
   （见「不做的」）。

**回调形态的幂等问题在素材上就答了，不等端点**：见第四节的幂等锚。回调端点届时只做一件事——
把源的推送译成同一份 `TrackingMaterial` 交给收编入口（票 `16`），与拉取到的素材走同一条路。

### 二、一个端口，多家实现；承运方在素材上，不在端口上

聚合平台一口给多家承运商的轨迹，承运商直连一家一口——**两者能共用一个端口**，因为收编方
问的从来不是「素材从聚合还是直连来」，而是「哪个承运方、哪份外部承运凭证、什么时候发生了
什么」。这三样都在素材上：

- 端口的实现单位是**一个已登记的轨迹源**（`TrackingSourceReference`）——一家聚合平台是一个
  源，一家直连承运商也是一个源。源的登记（账号、地址、频率）属 `PAR-INT-02`，端口只收引用。
- 承运方由 `TrackingSubject.CarrierReference` 携带，**由源侧或登记侧给出**，本仓不推断。
  聚合平台常要求调用方指名承运商，直连不需要——字段允许为空，不是每个源都要填。
- 一次拉取可问多个对象（`Subjects`），因为聚合平台按批答；直连一次答一个也装得下。

### 三、失败代数与查询口：照 ADR-0090，不私加格

`TrackingPullOutcome.Outcome` 收 `outbound.Outcome`，与取面单端口同形：`确证未受理`要过举证
门，超时一律`答案未确定`，未配置由 `outbound.ChannelConfiguration` 零值如实拒绝发起调用。

**拉取的「答案未确定」比取面单轻**：拉取是幂等读，重拉一次不产生供应商成本，所以
`AdmitsResend()==false` 对拉取的实际约束是「别在同一拍里重拉」，而不是资金红线。**但代数不
因此改形**——两条链共用一套代数正是 ADR-0090 立票的理由，拉取侧不为自己开一个「可安全重拉」
的例外格；节奏（多久再拉）是 `PAR-INT-02` 的实例参数，不在端口里。

查询口：`TrackingPullSupport` 两格——`拉取由该源提供`／`该源不提供拉取（只推送）`。后者就是
ADR-0090 决定六说的「该源无查询口」，如实记为一格而不是让拉取交回一个失败处置；两者的续办
不同：拉取失败可以再拉，无拉取口只能等推送或交人对账。

### 四、素材形状：三时间与标识照票 `03`，端口一个都不铸

`TrackingMaterial` 是一条待收编的原始素材，字段与归属：

| 字段 | 谁给 | 端口这一层的规矩 |
|---|---|---|
| `Source` | 登记 | 已登记轨迹源的引用 |
| `Subject`（租户、外部承运凭证引用、承运方引用） | 登记／源 | 凭证引用不解释为运单号（TF CONTEXT「外部承运凭证」） |
| `SourceEventID` | 源 | **源不给就空着**，不代铸（ADR-0023） |
| `OccurredAt` | 源 | `SourceTime{Given,At}`，**源不给即 `Given=false`**，不许拿任何时间顶替 |
| `ReceivedAt` | 本仓 | 端口这一层唯一铸的时间：「本仓什么时候拿到的」 |
| `StatusReference` | 源 | 源的原始状态词或码，**原样引用、不解释、不映射**（状态码表属实例半边） |
| `CorrectionOf` | 源 | 源若显式声明本条更正了哪条（按源事件标识），原样带；不声明就空。**取代关系由所有者判断**，端口不从到达先后推 |
| `PayloadDigest` / `Payload` | 适配器 / 源 | 摘要在字节还在手上时算出；本体只过路 |

`EffectiveAt` **不在素材上**——它是 TF 的一次显式判断（票 `03` 第二问），归票 `16`。
`AcceptedSourceFact` 也不在——端口交出的是素材不是事实，`domain.SourceContext` 封闭五值本票
一字不动。

### 五、幂等与顺序：两形态各答一句

- **拉取——「上次拉到哪」**：`PullCursor` 是不透明字符串，由该源的适配器解释，`NextCursor`
  随结果交回、空即「没有更多」。不透明是刻意的：聚合平台用时间水位、直连用页码或事件序号，
  机制若给它一个结构就得替每家源选一种。游标属机制半边（形状），游标的取值属实例半边。
- **接收——「重复投递怎么办」**：幂等锚取 **（源引用，源事件标识）**，不新造传输层的键
  （ADR-0090 决定四）。同一锚再次到达即重复投递，收编方不重收编。**源不给事件标识的素材不判
  重**——按内容摘要判重等于替源发明一个身份，那是代铸；这类素材如实带着空标识交所有者，
  由 `16` 决定留痕方式。
- **顺序不承诺**：两形态都不保证素材按发生顺序到达。所有者按 `OccurredAt` 与源声明的
  `CorrectionOf` 判先后与取代，**不按到达序**——这一条是票 `16` 的约束，端口只保证把这两样
  原样交过去。

### 六、落点与文件

- 端口：`internal/transportfulfillment/ports/tracking_source.go`——只放记录结构、封闭枚举与
  `TrackingSource` 接口；`ports` 包零测试是仓内约定（构造门在领域、行为由消费方钉）。
- 替身：`internal/transportfulfillment/adapters/trackingsource/synthetic_double_test.go`——
  只有测试文件的包，生产代码导入不了（形照票 `08`），证据层级 `S`。替身证三件：`OccurredAt`
  缺席能如实交出「没有」；只推送的源能答「不提供拉取」而不是一个失败处置；游标往返与
  `答案未确定`不准重拉。
- TF 是 MCP-3 的地盘：两处都是**新文件**，已在频道报形状与文件，等它让位再落。

### 七、不做的

- **不建回调端点**：随第一家推送源立票（那张票要同时答 ADR-0055 的 Intake 与源方凭证校验）。
- **不做收编**：素材→TF 新立事实→VE `AcceptedSourceFact` 归票 `16`，其前置落文（TF CONTEXT
  增补 + 三时间 ADR）本票也不代做。
- **不接任何真源、不填账号／密钥／频率／状态码表**（`PAR-INT-02`）。
- **不为拉取开「可安全重拉」的代数例外**——理由在第三节。

## 完成记录（2026-09-03，MCP-5）

两处，都是新文件，不动 TF 任何既有文件：

- 端口 `internal/transportfulfillment/ports/tracking_source.go`：`TrackingSourceReference`、
  `TrackingSubject`、`SourceTime{Given, At}`、`TrackingMaterial`、不透明 `PullCursor`、封闭两格
  `TrackingPullSupport`、`TrackingPullRequest`／`TrackingPullOutcome`（收 `outbound.Outcome`），
  与单方法接口 `TrackingSource.Pull`。`ports` 包照仓内约定零测试。
- 替身 `internal/transportfulfillment/adapters/trackingsource/synthetic_double_test.go`：只有测试
  文件的包，生产代码导入不了；证据层级 `S`。四条用例各钉一格：源未给发生时间的素材如实
  `Given=false` 且不被 `ReceivedAt` 顶替；只推送的源答「不提供拉取」而不被读成「去查询」；游标
  往返且`答案未确定`不准重拉；一次拉取装得下多个对象且零值配置不放行调用。

**清点报告随本笔重生成**（在检出本分支的干净 worktree 上跑生成器）：TF 生产 +1、测试 +1，端口
声明 +1；`transportfulfillment.TrackingSource` 如实进入两口径的「无生产实现」名单——与
`parcelshipment.LabelChannelGateway` 同一格，是设计不是欠账（本口的生产实现是各家源的适配器，
逐家另立）。

**验证**（隔离 worktree，未设 DSN——本笔无 `.sql`、无 postgres 适配器，真库对它无可证之物）：
`gofmt -l` 空（两文件先按 CRLF 落盘被 gofmt 报出，`gofmt -w` 后字节级核过零 CRLF、无 BOM）、
`go build ./...` 退 0、`go vet` 退 0、`go test -count=1` 于 `adapters/trackingsource` 与
`internal/architecture` 全绿——新枚举过枚举门禁，`ports → platform/outbound` 的方向过边界门禁。
SHA 与并入主线的取证见下一条。

## Comments

**2026-09-03，MCP-5（新会话）——并入主线 `7904003`，本票转 resolved。**

上一段完成记录写完之后、并入主线之前，那个会话断了：`4b6fa79` 只留在分支 `mcp5-lc15`
（worktree `idp-lc15`，基于 `146bb10`），主线上两份 Go 文件都不存在。owner 在通道 5 指示「继续」，
本会话接手落地：

- 落法：`git checkout 4b6fa79 -- 三路径` 取到主线 `b98368d` 之上，`git diff 4b6fa79 7904003 -- 三路径`
  为空，即三个文件与分支上那笔逐字节相同；提交带 pathspec，暂存集只含这三路径
  （`git diff --cached --name-only` 核过）。暂存期间主线从 `e3dbf3f` 走到 `b98368d`（MCP-4 的
  admin-web 一笔，不含 Go），提交前按 HEAD 守卫拦下一次、核过无关后再提。
- **没带走** `docs/product/MECHANISM-INVENTORY.md`：`4b6fa79` 上那份重生成锚在 `146bb10`，与主线
  `25db568` 的重生成对不上；清点照约定推之前由推的人跑，本笔不单方面重生成。推之前 CI
  「Mechanism inventory is current」那道门对 `7904003` 为红是预期内的。
- 验证（临时 detached worktree 检出 `7904003`，不含任何人的在途改动）：`gofmt -l internal cmd`
  空、`go build ./...` 退 0、`go vet ./...` 退 0、`go test -count=1 ./...` 94 个包 `ok`、0 `FAIL`。
  **未设 DSN**，PG 用例跳过——本票无 `.sql`、无 postgres 适配器，真库对它无可证之物。这是
  「绿（未设 DSN）」不是「绿（含真库）」。
- 分支 `mcp5-lc15` 指针留着供事后补验，worktree `idp-lc15` 内容核空后拆除。

完成判据逐条：形态选择有明确答复（Answer 第一节）；端口形状落地并有替身测试（`7904003`）；
`gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿且已注明不含真库。
