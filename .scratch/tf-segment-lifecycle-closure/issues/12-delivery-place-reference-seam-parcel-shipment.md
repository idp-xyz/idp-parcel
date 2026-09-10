# 派送要求缝一：收件地点引用（`parcel-shipment`）——`DeliveryPlaceSource` 的消费侧适配器

Category: enhancement
Status: resolved——2026-09-10 14:5x 通道 2（单 task-b5823d32-0393-40a7-bf55-dbc101f543af），分支 `mcp2-tf12` 基 `8bdab82e`，代码 tip `c33ac8bd`，完成记录见 Comments 末条；待推送方派非作者评审后重放进 main。此前 in-progress（13:2x 通道 2 认领）、ready-for-agent——三问已裁（2026-09-09，通道 2，task-ddb77473，用户授权 PS owner 口径代裁；见「裁决」，理由与被否替代在 [ADR-0130](../../../docs/adr/0130-delivery-place-reference-is-a-shipment-level-composite-reference-anchored-to-a-source-data-version.md)）；做法与完成判据见下。此前 draft——由票 [09](09-arrival-triggers-dispatch-task.md) 裁决④与 ADR-0114 决定三/四拆出（2026-09-07，通道 4，task-79675845）
Blocked by: [ps-port-remainder/06](../../ps-port-remainder/issues/06-delivery-place-reference-read-face.md)（PS 侧读口与值对象；本票的适配器翻译它的四格，它没落之前本票只能写替身，不能接真）

## 缺口

末端派送任务七件里的**地点**归 `parcel-shipment`（UC-TF-006 输入契约「派送要求 ← parcel-shipment / party-commercial：……目的地……」；票 09 裁决④）。TF 侧的端口 `ports.DeliveryPlaceSource.LoadDeliveryPlace(tenant, object) → (place, resolution)` 已随执行器立住：交回的是**收件地点引用**，落进任务的 `Place`（`AttemptPlaceReference`）也是引用——地址本体留在 PS，需要它的一线作业端按引用向所有者取（ADR-0114 决定三；ADR-0075 要防的「不拥有地址的上下文长期持有明文地址」在这里守住）。

今天 TF 之外没有任何类型实现这个接口；PS 侧也**没有一个可被引用的「收件地点」身份**——寄收件范围与地址今天只在委托提交的规范化载荷里作为内容出现（`internal/parcelshipment/domain/payload_canonicalization.go` 头注写着「寄收件范围与服务要求尚无领域模型」）。缝两头都缺一格，本票记的是缝，不是其中任何一头的模型。

## 所有者

- 地点（地址、收件关系）：`parcel-shipment`。TF 只引用，不落地址本体，不判断地址对错。
- 载运对象 → 包裹身份：`CarriedObjectReference` 本就是对正式包裹身份（PS 拥有）或集运单元（NO 拥有）的引用；本缝只管包裹这一种——集运单元的派送目的地不是本票的事（集运单元整体末端派送是否成立本身未裁）。

## 缝的形状（TF 这一头已定）

- 端口：`internal/transportfulfillment/ports/delivery_requirement.go` 的 `DeliveryPlaceSource`。返回 `RequirementResolved` + 引用串，或 `RequirementMissing`（所有者说这个对象没有收件地点——业务答案，不是错误）。
- 适配器落位：`internal/transportfulfillment/adapters/parcelshipment/`（ADR-0025 消费侧；只翻译不判断；翻译是全函数，PS 读面封闭集合里的每一格都要有落点，不留 `default`）。
- 消费方：`application.TriggerDeliveryDispatchHandler`。适配器缺席时它答 `DISPATCH_UNDECIDED` / `DELIVERY_PLACE_SOURCE_NOT_WIRED`；接了线而 PS 答「没有」时它答 `REQUIREMENT_MISSING` / `DELIVERY_PLACE`，任务保持待形成，不填默认。

## 未接时 TF 停在哪

执行器按地点、时间窗、条件的顺序核缝；地点在最前，所以**今天每一拍都停在 `DELIVERY_PLACE_SOURCE_NOT_WIRED`**，续办引用非空、一个任务都不开、段与交接一行不动。这是 ADR-0114 决定三要的诚实停点，不是缺陷。本票接上之后下一拍停到票 [13](13-delivery-window-seam-network-routing.md) 的那一格。

## 要裁的（PS owner）

1. **「收件地点引用」是什么身份。** 候选：(a) PS 为每个已接受委托版本的收件范围铸一个地点身份，引用带版本；(b) 引用就是（委托、接受基线、收件槽位）的复合指针，不另铸身份；(c) 目的地节点引用（NR 的词）——ADR-0114 越权风险点 4 已把「收件地址引用 vs 目的地节点引用」留给本票，且指出那是 PS owner 与本票一起定的事。取 (c) 会让派送任务的地点变成网络节点而不是收件人处，UC-TF-006 的「目的地」读法要先对齐。
2. **从哪个读面取。** PS 今天没有按包裹身份答「收件地点引用」的读口；是在 PS `ports` 上开一个按包裹身份的窄读口（同版性问题：取哪一版委托的收件范围——当前提交版本还是接受基线），还是让 PS 在委托接受时把地点引用随既有的 TF 侧信封带过来（ADR-0075 的「随请求携带」路数，但这里 TF 的触发源在内部、没有请求可以顺路带——ADR-0114 决定三已因此选拉不选带，本问只剩「拉哪个读面」）。
3. **地址修订之后引用怎么动。** 委托修订（`amendment_stage`）改了收件地址，已形成任务上的地点引用指旧版还是自动跟到新版——任务表达工作范围，改约不改身份；地点变了是不是「新任务」是 TF 的判断，但引用是否带版本决定 TF 能不能判。

## 裁决（2026-09-09，通道 2，task-ddb77473；用户授权 PS owner 口径代裁，理由、场景与被否替代在 ADR-0130，此处只记答案）

1. **是什么身份 → (b)，不铸身份，不是节点。** 收件地点引用是 PS 拥有的**委托级复合引用**：租户、委托、收件资料范围、资料版本锚（接受基线，或当前采用的客户原始资料版本）四段；串由 PS 一处定义、自带形状版本，TF 当不透明串存进 `AttemptPlaceReference`，地址内容一字不进串。不取 (a)：收件地址的版本就是客户原始资料版本，再铸地点身份是同一份内容的第二条版本线。不取 (c)：节点是网络位置不是收件人处，待路由产品没有目的地节点却有收件人，(c) 会对它答假话；UC-TF-006 输入契约「目的地」列在 PS / PC 名下，照它读。同一委托的包裹答同一个引用（CONTEXT 硬句「同一委托中的包裹必须共享……寄收件关系」）。
2. **从哪个读面取 → PS 新开的窄读口，按（租户，包裹身份）问，答查询时刻的当前采用判断。** 供票 14 第 2 问直接引用的一句：**PS 对外一律按（租户，包裹身份）答，不要求消费方持有委托、接受基线或提交版本；委托与接受基线是 PS 内部走到答案的路，不是键。** 取「当前采用」而非「接受基线」：CONTEXT 把「当前客户资料版本采用判断」定为「派生的当前消费引用」，就是给这类消费方的读面；按基线派送等于对已更正地址照旧送。「当前提交版本」在这一拍不存在——对象进派送段时委托必已`已接受`。答法封闭四格：无修订版本 → 基线锚引用；`已采用` → 版本锚引用；`待复核` → 不给引用、答「收件地点未定」；不属任何已接受委托的成员集合（含集运单元、不可见对象）→ 按统一不可见结果答「没有收件地点」。同版性由锚承担（形同 ADR-0075 决定三以摘要留痕），不靠 PS 与 TF 同一时刻读同一版。
3. **地址修订之后 → 旧引用不动、含义不改；新查询拿到换了锚的引用；PS 不推送；是否新任务 TF 判。** 旧锚永远解析到那一版内容（版本不可覆盖）。TF 拿新旧两个引用，前两段相同、锚不同即「同一收件槽位、地址内容已修订」，按自己规则判新任务或新尝试。已形成派送任务不是资料修订阶段的事实，PS 不因 TF 手里有任务多加一道修订门。

越权风险点五条单列在 ADR-0130，其中与本票实施直接相关的是 1（`待复核`在 `RequirementResolution` 两格上怎么落——TF 地盘，本票实施时定）与 3（形成任务后 TF 何时再拉——TF 地盘）。

## 做法（顺序固定）

1. **等 [ps-port-remainder/06](../../ps-port-remainder/issues/06-delivery-place-reference-read-face.md) 进 main**：PS 值对象 `DeliveryPlaceReference` 与读口四格是本票翻译的源。它没落之前本票可以先写 TF 侧替身与执行器用例，不能接真。
2. **先定 `待复核`在 TF 端口上的落法（ADR-0130 越权风险点 1，TF owner 一句话）**：甲——译成 `RequirementMissing`，适配器头注写明「所有者说未定，不是说没有」，端口不动；乙——`RequirementResolution` 加第三格 `RequirementUndetermined`，执行器多一格结果（`REQUIREMENT_UNDETERMINED / DELIVERY_PLACE`，任务同样待形成，续办引用非空），三条端口共用这个代数所以 13 / 14 的适配器也拿到这一格。**默认甲**（票 13 / 14 都没有「未定」这种答案，为一条缝加一格代数要先证明另两条也用得上）；取乙要先改 TF CONTEXT「派送要求」词条那句「任一条缺席时任务保持待形成」补上「未定」，再动代码。
3. **适配器** `internal/transportfulfillment/adapters/parcelshipment/delivery_place_source.go`（新包，ADR-0025 消费侧；与 `adapters/partycommercial` 并列，不合并成门面）：依赖 PS 读口的**窄接口**（本包内声明、只含用到的那一个方法，先例 ps-port-remainder/05 的 `ParcelDeclarationFactsLookup`），不 import PS `application`。**全函数翻译**，源侧封闭四格逐格有落点、不留 `default`：基线锚引用 → `RequirementResolved` + `String()`；已采用版本锚引用 → `RequirementResolved` + `String()`；`收件地点未定` → 按第 2 步（默认 `RequirementMissing`）；`没有收件地点` → `RequirementMissing`；读口 error 原样上抛让执行器落它既有的「读不回」格；PS 集外取值上抛 `ErrUntranslatableAnswer`（同 05 先例）。`CarriedObjectReference` → PS 包裹身份的翻译在本适配器内做，PS 不认识 TF 的词。
4. **装配**：`cmd/parcel-api` 把 `DeliveryPlaceSource` 那一格从空换成真适配器；装配测试补一例——执行器在生产装配下不再答 `DELIVERY_PLACE_SOURCE_NOT_WIRED`，停点后移到 `DELIVERY_WINDOW_SOURCE_NOT_WIRED`（票 13 未接）或按 13 的进度更后。
5. **生产入口那一格**（本票「生产入口」节）：开工时若 13 / 14 都还没开工，执行器的生产入口归本票——形状照 ADR-0106 决定四 / ADR-0114 决定二末句的先例：谁按拍调执行器、拍频作配置不作默认、`cmd/parcel-api` 装配接线；拍频取值是实例半边，机制接上、值留空并如实答「未配置」。若 13 或 14 先开了，本票不重立，只在完成记录写一句归谁。
6. **文档**：TF CONTEXT 不改（「派送要求」词条已用「收件地点引用」这个词；取乙时例外，见第 2 步）；ADR-0114 / 0130 / 0075 正文不改；本票完成记录写 TF 侧 SHA、PS 侧依赖的 main SHA、第 2 步取甲还是乙。

## 完成判据

1. 适配器测试覆盖源侧四格各一例 + error 上抛一例 + 集外取值一例；断言翻译后的 `resolution` 与 `place` 串与 PS `String()` 逐字相等（不在 TF 侧重拼串）。
2. 执行器用例（`TriggerDeliveryDispatchHandler` 既有测试旁）补两例：PS 答基线锚引用 → 往下走到 13 的那一格；PS 答「没有收件地点」（集运单元）→ `REQUIREMENT_MISSING / DELIVERY_PLACE`，任务待形成、续办引用非空、段与交接一行不动。
3. `cmd/parcel-api` 装配测试一例（做法第 4 步）。
4. 红线逐条守住：任务 `Place` 只存引用串，TF 任何表不落地址字段；PS 答「没有」照传，不补默认、不拿目的地节点顶替；不动 `internal/parcelshipment/**`；测试只用合成引用。
5. 验证（作者层）：gofmt 空、`go build ./...` / `go vet ./...` 0、`go test -count=1` TF `adapters/parcelshipment` + `application` + `./cmd/parcel-api/`（带 DSN）+ `./internal/architecture/...`；清点在干净检出重生成。

## 生产入口

ADR-0114 决定二末句：执行器的生产入口（谁按拍调它、拍频、`cmd/parcel-api` 装配）随**第一条**派送要求缝的实施票立。本票若先于 13 / 14 开工，那一格归本票；否则归先开的那一票，本票不重立。落法见「做法」第 5 步。

## 红线

- 不落地址本体到 TF 任何表；任务 `Place` 是引用。
- 只翻译不判断：PS 答「没有」照传 `RequirementMissing`，不补默认地点、不拿目的地节点顶替。
- 不动 `internal/parcelshipment/**`（本票只立票；PS 侧读面若要开，由 PS owner 在其地盘立票）。
- 不写真实地址、真实客户；测试用合成引用。

## Comments

- 2026-09-07 · 通道 4（task-79675845）：由票 09 裁决④拆出立票，只写票面，未动代码。TF 侧端口与执行器已落在分支 `mcp4-tf09`（见票 09 完成记录）。
- 2026-09-09 · 通道 2（task-ddb77473，用户经队列授权 PS owner 口径代裁，基 main `91df9aaa`）：三问过 `/domain-modeling`（限界上下文 parcel-shipment，消费方 transport-fulfillment；三个场景——接受后地址修订再派送 / 待路由产品无目的地节点但有收件人 / 集运单元不在本缝）后裁定，落 ADR-0130 + PS CONTEXT「收件地点引用」词条与 Rules 一条 + GLOSSARY 一条；PS 侧读口另立 [ps-port-remainder/06](../../ps-port-remainder/issues/06-delivery-place-reference-read-face.md) 作本票 Blocked by；Status draft → ready-for-agent。只裁不码，`internal/**` 一行未动。票 13 / 14 未改；票 14 第 2 问请引本票「裁决」第 2 条加粗那句。分支 `mcp2-tf12`，进 main 的 SHA 由推送方重放后另记。
- 2026-09-10 14:5x · 通道 2（task-b5823d32）：**完成记录**，分支 `mcp2-tf12` 基 `8bdab82e`（main，psr/06 已在其上），代码 tip `c33ac8bd`（本笔票面在其上），每笔提交后已推 origin。
  - **PS 侧依赖的 main SHA**：`8bdab82e`（psr/06 进 main 的簿记笔；`domain.DeliveryPlaceReference` / `DeliveryPlaceResolution` 四格 / `ports.DeliveryPlaceReferenceView` / `adapters/postgres.ShipmentRequests.LoadDeliveryPlaceReference`）。本票不依赖 psr/07。
  - **每笔**：`972d4867` 适配器新包 `internal/transportfulfillment/adapters/parcelshipment/delivery_place_source{,_test}.go` + 票面 in-progress；`a06f8145` 执行器两例 `application/trigger_delivery_dispatch_parcelshipment_test.go`（新文件，不动既有测试）；`12ce0ed3` 生产入口端点 `adapters/http/trigger_delivery_dispatch{,_test}.go`；`2b22c9b2` `cmd/parcel-api/assemble_delivery_dispatch{,_test}.go`（新）+ 四份共享接线文件各加行（endpoints.go +4 / endpoints_test.go +2 / main.go +7 / unwired_orchestration.go +12，无删行）；`e6f10315` 清点在 `2b22c9b2` 干净 detached 检出重生成；`c33ac8bd` 三处头注去序位与跨包计数（作者自查）。
  - **做法第 2 步取甲**：`待复核`译成 `RequirementMissing`，适配器头注写明「所有者说的是未定，不是没有」；端口 `RequirementResolution` 不动、TF CONTEXT 不改。理由照票面默认判据：票 13 / 14 都没有「未定」这种答案，为一条缝加一格代数要先证明另两条也用得上；今天`待复核`落 `REQUIREMENT_MISSING / DELIVERY_PLACE`，任务待形成，分叉并掉后同一拍重跑即得引用。取乙的条件与改点写在适配器头注。
  - **全函数翻译**（adapters/parcelshipment）：基线锚 / 已采用版本锚 → `RequirementResolved` + `reference.String()` 逐字；没有 → `RequirementMissing`；未定 → `RequirementMissing`（甲）；读口 error 原样上抛（执行器落 `DELIVERY_PLACE_SOURCE_UNAVAILABLE`）；四格之外与「带引用的格却没带引用」→ `ErrUntranslatableAnswer`。窄口 `DeliveryPlaceReferenceLookup` 只含 `LoadDeliveryPlaceReference`，不 import PS application / ports。**`CarriedObjectReference` → PS `DeclaredParcelID` 同一串字面、不分种类**：`CarriedObjectReference` 今天没有种类维，集运单元引用原样去问 PS，PS 按统一不可见结果答没有 → `RequirementMissing`。**供 tf/14 照抄时注意**：若 tf/14 想「集运单元直接 `RequirementMissing` 不调提供方」，需要先有对象种类维；本票没有立。
  - **生产入口归本票**（做法第 5 步；tf/13 于 14:1x 才开工，tf/14 未派）：取**端点 + `UnconfiguredIntake{}`**（通道 1 14:4x 裁甲；形照 ADR-0106 决定四 / ADR-0124 与既有 `/transport-fulfillment-dispatch-task-registrations`）——`POST /transport-fulfillment-delivery-dispatch-triggers` 收（段、对象、这一拍的业务时间），租户从信封给；谁按拍调、拍频多大属调用方（实例半边，仓内不写默认），Intake 未配置即 403 `ACCESS_CHANNEL_NOT_CONFIGURED` 不读体不构造命令。**地盘外一件**：`internal/transportfulfillment/adapters/http/trigger_delivery_dispatch{,_test}.go`，推送方 13:4x 准（装配的自然延伸）；`UnconfiguredIntake` 对本口的方法声明在该文件，`unconfigured_intake.go` 不动。被否的乙（进程内按 env 拍频的循环）需要 TF 工作面读口 + postgres，越地盘、另一张票的量。
  - **装配**（cmd/parcel-api）：`buildDeliveryDispatchTrigger` 装 `TriggerDeliveryDispatchDeps{Segments 真, Places 真（窄口交 PS 委托仓储）, Dispatch 真（不包事务）}`，**Windows / Conditions 留 nil**；`transactionalDeliveryDispatchTrigger` 把一拍包进一笔事务。**tf/13 的接点**：在该 Deps 填 `Windows` 一格 + 把 `assemble_delivery_dispatch_test.go` 预期停点改为 `DELIVERY_CONDITION_SOURCE_NOT_WIRED`（已 14:2x 告知通道 5，通道 5 确认按此接、等本票进 main 后 rebase）。
  - **触及**：上列新文件 + 四份 cmd/parcel-api 共享文件（只加行）+ `docs/product/MECHANISM-INVENTORY.md` + 本票面。**未碰**：`internal/parcelshipment/**`（红线，`git diff 8bdab82e..tip -- internal/parcelshipment` 为空）；`internal/transportfulfillment/ports/**`（甲不动端口）；`application/trigger_delivery_dispatch.go` 与其既有测试；TF CONTEXT；ADR-0114 / 0130 / 0075 正文；`internal/architecture/*_baseline.txt`（新导出只有 `New*` 构造器与端点函数，两道棘轮不响）；`migrations/`。
  - **完成判据逐项**：1 ✓ `delivery_place_source_test.go`：源侧四格各一例（两格锚合并成表驱动一例两子例）+ error 上抛 + 集外取值（零值答复）+ nil 口拒；`place` 与 PS `String()` 逐字相等断言在带引用两格上。2 ✓ `trigger_delivery_dispatch_parcelshipment_test.go`：PS 答基线锚引用 → 时间窗缝未接时停 `DELIVERY_WINDOW_SOURCE_NOT_WIRED`（续办非空、无任务、PS 未被问——执行器先核未接缝）、三缝齐时任务形成且 `Place == DPR-1 串`；集运单元 → `REQUIREMENT_MISSING / [DELIVERY_PLACE]`、任务待形成、段与交接计数不变。**一处与票面字面不同**：票面写「续办引用非空」，执行器既有形状与 ADR-0114 决定三是 `REQUIREMENT_MISSING` **不留续办引用**（业务答案不是欠账；既有用例 `TestAnOwnerAnsweringMissingKeepsTheTaskUnformed` 钉着），本票照既有形状断言为空。3 ✓ `assemble_delivery_dispatch_test.go`（带 DSN）：真编排让对象凭已交接进 `FINAL_DELIVERY` 段，生产装配的执行器答 `DISPATCH_UNDECIDED / DELIVERY_WINDOW_SOURCE_NOT_WIRED`，续办非空、无任务。4 ✓ 任务 `Place` 只存引用串（`AttemptPlaceReference`），TF 任何表未加地址字段（无迁移）；「没有」照传不补默认、不拿目的地节点顶替；PS 未动；测试只用合成引用（`tenant-1 / request-1 / DELIVERY_PLACE`、`SYN-*`）。5 ✓ 见验证。
  - **验证强度**（作者层，带 DSN `-p 1 -count=1 -v`）：gofmt 空；`go build ./...` / `go vet ./...` 0；`./internal/transportfulfillment/...` + `cmd/parcel-api` + `cmd/parcel-dispatch`（tfapp 反向依赖）+ `./internal/architecture/...` **PASS 1707 / SKIP 0 / FAIL 0**。未跑全量。清点在 `2b22c9b2` 干净检出重生成、单独成笔。一处纪律失手随广播记过：13:5x 跑 TF 包时 shell 残留 DSN，TF `adapters/postgres` 碰了 55432 约 7 s 未先占号（只会慢不会错）。
  - **与 main 碰面干跑**（14:3x，`origin/main = 62c87e73`，`ls-remote` 同）：本分支自 `8bdab82e` 以来触及的文件与 main 同期改动**无同文件**。**已知将来的重叠**：sa/04（通道 6，`origin/mcp6-sa04 = 3df82d62`，待重放）同样在 `endpoints.go` / `endpoints_test.go` / `main.go` / `unwired_orchestration.go` 各加行——它加在 PS 组（紧随 rejection），本票加在 TF 组（紧随 dispatchTaskOpener），不相邻，`assembleBusinessEndpoints` 的位置参两处各插一位、两个调用点（main.go、endpoints_test.go）各自同步；先进的一方落地后，后进的一方重放应能自动合，合不上时后进方按自己的四行手工放回即可。
  - **给评审的判断题**：(a) 甲——`待复核`折成 `RequirementMissing`，字面是「所有者说没有」，任务停点与真「没有」同形不可分辨（ADR-0130 越权风险点 1 原文）；反方是加第三格。(b) 生产入口取端点而非进程内循环——「按拍」由调用方承担；反方是仓内没有任何东西会自动触发，端点未配置前执行器在生产上仍不可达（与其余所有命令面同形，ADR-0055）。(c) 判据 2「续办引用非空」照既有执行器形状断言为空——反方是票面字面。(d) 执行器一拍包一笔事务，`不是触发事实` / `要求缺失` / `未决` 也提交（空事务）——反方是这些格不该开事务；正方是任务口按框架合同无事务即拒，包在外层最简单，且执行器对段只读。(e) `CarriedObjectReference` 不分种类直接译成 `DeclaredParcelID` 去问 PS——反方是集运单元该在 TF 侧短路；正方是今天没有种类维，短路无从判。
  - **越权风险点**（不改 ADR，随票记）：ADR-0130 越权风险点 1（`待复核`在 TF 端口上的落法）本票按甲落地，原文不改；风险点 3（形成任务后 TF 何时再拉）本票未定——端点每被调一拍就重拉一次，多久调一次属调用方。
