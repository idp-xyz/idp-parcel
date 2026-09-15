# pp-seams/01 与 03 评审非阻断尾巴：01 两处「三件」计数、成员顺序未写进口契约；03 `AddressElements` 头注「不去空白」与 `AddressElementsOf` 的 `TrimSpace` 不同口、`AddressElementsView` 头注「全部快照答缺席」过宽

Category: chore
Status: resolved——**已进 main，2026-09-15 12:3x 通道 1 推送方**（第四批，重放 `69189208→4ca2ccac`，批 tip `676cc09b`（与 sa-cc/28、sa-cc/27 取证同批）；评审 ← 通道 4 两轴 0 阻断 / Standards 1 非阻断 / Spec 2 非阻断；`4ca2ccac` 带 DSN 全仓 113 ok / 0 FAIL；见 Comments「进 main 记录」）。此前——**2026-09-15 12:3x 通道 3**（task-93aa833b；分支 `mcp3-ppseams06` 基 `7160fe67`，代码 tip = 本笔单笔（六件同笔，SHA 见交活报与推送方「进 main 记录」）；隔离树 `$env:TEMP\idp-parcel-mcp3-ppseams06`；逐条判据、判断项、验证与能力边界见下方「完成记录」）。此前 in-progress——2026-09-15 12:2x 通道 3 认领。此前 ready-for-agent——2026-09-15 11:2x 通道 1 立票（01 评审 ← 通道 6 Standards ① + Spec ①；03 评审 ← 通道 6 Standards ① + Spec ①；推送方处置「合一张 A 类尾巴」）。除 03 那一处读值分支外零行为
Blocked by: 无（01 进 main `1e74aaaf`，03 进 main `a19ac630`）

## 缺口（评审各钉 `71dd8313` / `2a637107`；进 main 后在 `c88b5e35` / `1bd50208` 同形）

**01（TF）**

1. `internal/transportfulfillment/ports/charge_occurrence_member_view.go` `ChargeOccurrenceMembers` 头注「CONTEXT『运输收费发生项』固定下来的三件」与 `ChargeOccurrenceMemberView` 头注「协议、数量与修订三件」——都已点名，「三件」是冗余的数（AGENTS「改文档」：数别处的东西）。去掉两个「三件」。
2. 成员切片的顺序：实现 `ChargeOccurrences.LoadMembers` `ORDER BY member.object_ref`，与 `FindByKey` 的 `loadMembers` 同序；对方首条用例 `TestChargeOccurrenceMemberViewAnswersMembersBusinessTimeAndScopeByKey` 断了字面顺序，而口头注没说切片有无顺序保证——断言钉的是实现细节。评审建议**写进契约**而不是删断言：PP 消费侧要拿成员清单进计价输入快照的指纹，确定序是它的需要。做法：`ChargeOccurrenceMemberView` 头注补「成员按引用字面升序，与 `FindByKey` 同序」一句；那条断言随之成契约断言，不动。

**03（PS）**

3. `internal/parcelshipment/domain/address_element.go` `AddressElements` 头注「客户给的串本上下文不去空白，一个全是空白的值与『没报』得分得开」；而 `internal/parcelshipment/domain/payload_canonicalization.go` `AddressElementsOf` 用 `strings.TrimSpace(entry.Value()) != ""` 判在场——全空白的值读作缺席，与「没报」分不开；`CanonicalContentEntry` 定的「显式清空」是值为空串（值保真不 trim），`AddressElementsOf` 头注引它却把空白串也归了进去。**三处要同口。** 本票取「以 `CanonicalContentEntry` 的定义为准」：`AddressElementsOf` 只把 `entry.Value() == ""` 读作显式清空 / 缺席，全空白的串原样在场（PS CONTEXT「地址要素」词条「本上下文只保存客户给的串，不校验、不规范化」——为判在场而 trim 也是一次规范化）；头注两处随之改口。今天值恒缺席（pp-seams/05 未落），零运行时影响；单元例 `TestAnExplicitlyClearedOrDuplicatedAddressElementReadsAsAbsent` 若含全空白一格要随之改口，`TestAddressElementsAreReadByClosedEntryNameOnly`「值原样不去空白」加一格全空白在场。**若 PS owner 认为全空白该读作缺席**，改法是只改 `AddressElements` 头注那半句并让 `AddressElementsOf` 头注写明「空白串同显式清空」——两路都行，本票默认前者，评审可翻。
4. `internal/parcelshipment/ports/address_elements_view.go` 头注「今天对全部快照两段都答『要素缺席』」——实现 `addressElementsInGroup` 先锚后内容：`已采用`答已采用版本锚、`待复核`答未定、非成员答无，只有锚在基线上才答「要素缺席」（适配器头注与 `AddressElementsOutcome` 头注写对了）。改为「锚在基线上的那一格今天恒答要素缺席」。

## 红线

- 01 两处、03 第 4 处只注释；03 第 3 处若取默认改法，行为差只在「全空白值」一格，今天无数据到得了它；不动 `PayloadDigest`、不动迁移、不动 `application` / `adapters/http`。
- 注释中文；不写行号、不数别处。
- `internal/parcelpricing/**` 零 diff。

## 完成判据

1. 01：`git grep -n '三件' -- internal/transportfulfillment/ports/charge_occurrence_member_view.go` 零命中；口头注含「按引用字面升序」一句。
2. 03：`AddressElements` / `AddressElementsOf` / `AddressElementsView` 三处头注同口；`AddressElementsOf` 的在场判定与头注一致；`go test ./internal/parcelshipment/domain/` ok（含改口 / 新增那一格）。
3. `gofmt -l` 空；`go vet ./internal/transportfulfillment/... ./internal/parcelshipment/...` 0；不带 DSN 两上下文全部包 ok；`internal/parcelpricing/**` 零 diff。
4. 完成记录同笔；清点零差（不增删文件）。

## 地盘

`internal/transportfulfillment/ports/charge_occurrence_member_view.go`（只注释）；`internal/parcelshipment/domain/address_element.go` / `payload_canonicalization.go`（`AddressElementsOf` 一处判定 + 头注）/ `address_element_test.go`；`internal/parcelshipment/ports/address_elements_view.go`（只注释）。撞点：无在途分支碰这些文件（pp-seams/05 未开工）。

## 参照

[01](01-tf-charge-occurrence-member-object-read-view.md) Comments「评审 ← 通道 6」Standards ① / Spec ① 与「进 main 记录」；[03](03-ps-origin-destination-postal-route-read-port.md) Comments「评审 ← 通道 6」Standards ① / Spec ① 与「进 main 记录」；[05](05-ps-submission-and-source-data-versions-carry-content.md)「要裁的」（「显式清空」要不要单独成格归 PS owner——本票不预裁那一格，只让三处头注与实现同口）；PS `CONTEXT.md`「地址要素」；`internal/parcelshipment/domain/payload_canonicalization.go` `CanonicalContentEntry` 头注（「显式清空」的定义）；AGENTS「写代码注释」「改文档」。

## 完成记录

分支 `mcp3-ppseams06`，基 `7160fe67`（= 派单时 `origin/main`，`fetch` 后 `worktree add`，树干净、无 untracked）。单笔：五个 `.go` + 本票 .md，`git diff --numstat 7160fe67` 六件、无增删文件。按 /implement 走，缺口 3 那一格先 red 再 green。

**逐条对完成判据**：**(1)** `git grep -c -e "$([char]0x4E09)$([char]0x4EF6)" -- internal/transportfulfillment/ports/charge_occurrence_member_view.go` **零命中**（退出码 1；针按码点拼、绕开 PowerShell 中文针失配那条坑；同针打在 `adapters/postgres/charge_occurrence_registry.go` 上作正向对照命中 1，字面量与解码路径都验过）；`ChargeOccurrenceMemberView` 头注新增一段「Members 的顺序是契约：成员按引用字面升序，与 `FindByKey` 读回的成员同序……实现者不得各排各的」，`git grep -c 按引用字面升序` 命中 1 ✓。**(2)** 三处头注同口——`AddressElements`：「没报」含显式清空 = `CanonicalContentEntry` 定义的值为空串条目，`AddressElementsOf` 读在场只认这一种缺席，空白串原样在场；`AddressElementsOf`：在场与否只看值是不是空串、**不先去空白**，全空白原样在场，理由引 PS CONTEXT「地址要素」词条「不校验、不规范化」（为判在场而 trim 也是一次规范化）；`AddressElementsView`：「不校验、不规范化、不去空白——全空白的值原样在场，只有显式清空（值为空串）读作缺席，口径与 `domain.AddressElementsOf` 头注同一句」。判定 `entry.Value() != ""` 与头注一致。`go test ./internal/parcelshipment/domain/ -count=1` **ok**，含新一格 ✓。**(3)** `gofmt -l ./internal/` **空**；`go vet ./internal/transportfulfillment/... ./internal/parcelshipment/...` **0**；不带 DSN `go test -count=1` 两上下文全部包 **ok**（TF 8 包 + PS 26 包有用例，`ports` 与 `adoptconsume` 无用例；`adapters/postgres` 真库用例因未设 DSN 跳过，包行仍 `ok`——这是「绿（未设 DSN）」不是「绿（含 PG）」，且本票没碰任何 postgres 适配器，真库答案不受影响）；`git diff --stat 7160fe67 -- internal/parcelpricing/` **空** ✓。**(4)** 完成记录同笔；`git status --short --untracked-files=all` 六行全 ` M`，零增删 ✓。

**红线核**：01 两处与 03 第 4 处只动注释（`charge_occurrence_member_view.go` +6 −2、`address_elements_view.go` +6 −4，均在 `//` 内）；03 第 3 处行为差只在 `AddressElementsOf` 一行（`strings.TrimSpace(entry.Value()) != ""` → `entry.Value() != ""`），`PayloadDigest` / `canonicalEntryDocuments` / 迁移 / `application` / `adapters/http` 零 diff；注释全中文、无行号、无跨文件计数；`git diff --check` 0，六件 `i/lf w/lf`。

**判断项**：
① **缺口 3 取票面默认口**（以 `CanonicalContentEntry` 为准：只有空串是显式清空 / 缺席，全空白原样在场）。理由除票面已写的（trim 也是规范化、`CanonicalContentEntry` 值保真）外再加一条：`AddressElements` 头注原句「一个全是空白的值与『没报』得分得开」是这个类型自己许下的话，而 `TrimSpace` 判在场恰恰让两者分不开——取默认口是让实现兑现类型自己的头注，改另一口则要同时改写那句与结构上「两格各带在场标志」的理由。PS owner 若翻，改法票面已写（只改 `AddressElements` 那半句 + `AddressElementsOf` 头注写「空白串同显式清空」+ 判定回 `TrimSpace`），一处行为、三处注释、一格用例。
② **`TestAnExplicitlyClearedOrDuplicatedAddressElementReadsAsAbsent` 未动**：它只有空串一格与同名两条矛盾一格，没有全空白格，票面「若含……要随之改口」的前件不成立。新一格按票面加在 `TestAddressElementsAreReadByClosedEntryNameOnly` 末尾（`DELIVERY_PLACE.COUNTRY_CODE` = 三个空格，断在场且值原样），单独一段条目而不混进既有那组——既有那组里「收件范围上没报国家 / 地区码，不拿寄件的顶」那格断的是 `declared == false`，往里塞一条空白 `COUNTRY_CODE` 会改掉那格的题。Red 先跑：现实现下 `declared = false` 红；改判定后绿。
③ **`AddressElementsView` 头注多补了一句「不去空白」**：判据 2 点名它是三处之一，改前它只说「不校验、不规范化」——与新口不矛盾但没说出口，同口只是不矛盾而非同词；补的是同一句的第三个字面（`AddressElements` 头注原有「不去空白」、`AddressElementsOf` 写「不先去空白」）。只注释，仍在红线内。
④ **缺口 4 收窄那句核过实现再写**：`domain.ShipmentRequest.AddressElementsFor` 对非已接受委托成员答 `NoShipmentAddressElements()`；`addressElementsInGroup` 先 `resolveSourceDataAnchor`——`undetermined` → 未定、锚为已采用版本 → 已采用版本锚、否则读基线那一版 `addressElements(group)`（今天恒 `AddressElements{}`）→ `Empty()` → 要素缺席。头注写的顺序「非成员答无、待复核答未定、已采用答已采用版本锚，只有锚落在接受基线上的那一段才去读要素」与之逐格对上；引的是导出符号 `AddressElementsFor` 不是未导出的 `addressElementsInGroup`，后者改名不会让口头注指空。
⑤ **「按引用字面升序」在库上的落法**：`LoadMembers` 与 `FindByKey` 的 `loadMembers` 两条 SQL 都是 `ORDER BY object_ref`（一处带 `member.` 前缀），同一列同一 collation，两口同序成立；「字面升序」严格说是 `text` 列在库默认 collation 下的升序，`SYN-` 一类 ASCII 引用下与字节序一致，含非 ASCII 或大小写混排的引用在非 C collation 下可能与纯字节序不同——契约句沿用票面原词，这一格记给 TF owner：真要钉字节序，改的是两条 SQL 加 `COLLATE "C"`，不在本票。
⑥ **地盘外同形一处未动**：`adapters/postgres/charge_occurrence_registry.go` `LoadMembers` 头注「协议、数量与修订三件的合法性买单」仍带「三件」——不在票面地盘与判据里（判据 1 只量 `ports/` 那一文件），本票不越界；TF owner 顺手时可去。
⑦ **/code-review 两轴串行跑**：`Task` 子代理连报鉴权错误（两次），按 skill 退路由本会话串行各跑一轴、各只看自己的 brief。Standards 一条：我新写的 `AddressElements` 头注里「三处说的是同一句话」本身是跨处计数（AGENTS「改文档」）——已删；Spec 一条：即上面判断项 ③——已补。修后两轴复审同基线 + 工作树，零可行动发现。**这不是「合入前独立评审」那一份**（那份要非作者、由推送方指派），只是 /implement 内的自审门。

**验证（隔离树，未设 `IDP_PARCEL_POSTGRES_DSN`）**：`gofmt -l ./internal/` 空；`go build ./...` 0；`go vet ./internal/transportfulfillment/... ./internal/parcelshipment/...` 0；`go test -count=1 ./internal/transportfulfillment/... ./internal/parcelshipment/...` 全 ok / 0 FAIL；`go test ./internal/parcelshipment/domain/ -run 'TestAddressElementsAreReadByClosedEntryNameOnly|TestAnExplicitlyClearedOrDuplicatedAddressElementReadsAsAbsent' -count=1 -v` 改前一红一绿、改后两绿；`git diff --stat 7160fe67 -- internal/parcelpricing/` 空；`git diff --check 7160fe67` 0。**未占 55432**（派单明令）；反向依赖里的 `cmd/*` 未跑——本票改的是 PS domain 一行判定，今天没有任何提交版本携带地址要素条目（`SubmissionVersion.addressElements` 恒返零值），`cmd/*` 真库用例的答案不可能因此改变；全量由推送方在重放 tip 上带 DSN 兑一次。

**能力边界**：读了票面全文、01 / 03 票面评审段、PS CONTEXT「地址要素」词条、六件全文、`address_elements_resolution.go` 的 `AddressElementsFor` / `addressElementsInGroup` / `SubmissionVersion.addressElements`、TF 适配器两条 SQL；**没读** pp-seams/05 票面正文（票面说「不预裁那一格」，本票只让三处头注与实现同口，不碰「显式清空要不要单独成格」）、TF CONTEXT 正文（缺口 1 / 2 只删计数与补契约句，词条内容未引）；未跑真库；`/code-review` 子代理不可用，两轴由作者本人串行代跑（见判断项 ⑦）。

## Comments

- 2026-09-15 11:2x · 通道 1：立票（两票评审尾巴合一张，推送方处置时点名）。只写票面，未动代码。
- 2026-09-15 12:3x · 通道 3（task-93aa833b）：完工，Status resolved；缺口 1–4 逐条、判据 1–4 逐条、判断项七条、验证与能力边界见「完成记录」。缺口 3 取票面默认口（全空白原样在场），PS owner 可翻；`AddressElementsView` 头注多补「不去空白」三字使三处同词。树不拆，等评审与进 main。
- **2026-09-15 12:31 · 评审 ← 通道 4 · 钉 `69189208` · 基线 `7160fe67`**（task-b8929141，只读隔离检出 `%TEMP%\idp-review-ppseams06`；由推送方代落）。范围核实：`git diff --stat 7160fe67 69189208` **六件 +51 −11**，`--name-status` 全 `M`（零增删）；`-- internal/parcelpricing` **空**；`git grep -n '三件' -- internal/transportfulfillment/ports/charge_occurrence_member_view.go` **零命中**（退出码 1）；单笔、带 `Co-authored-by` trailer；`gofmt -l ./internal/` 空；`go vet ./internal/transportfulfillment/... ./internal/parcelshipment/...` 0；不带 DSN `go test -count=1` TF 8 包 + PS 26 包 **全 ok / 0 FAIL**（`ports` / `adoptconsume` 无用例）；`go list -f '{{.Imports}}' ./internal/parcelshipment/domain/` 无 `net/http` / `pgx`；`-run` 两条地址要素用例 `-v` 各 PASS 非 SKIP。未占 55432。

  **Standards——阻断：0。非阻断：1。**【`AddressElements` 头注「AddressElementsOf 读在场只认这一种缺席」略宽】同函数还有「同名多条互相矛盾两条都不认」这一路缺席（`without`）；这句说的是**值层面**的缺席，读者若只看类型头注会以为矛盾条目也在场。措辞可收成「值层面只认这一种缺席」；不要求本票改，PS owner 下次碰该文件顺手。**无发现**：新增注释全中文；跨文件引用全符号名（`CanonicalContentEntry` / `AddressElementsOf` / `domain.ShipmentRequest.AddressElementsFor` / `DeclaredMeasurementView` / `FindByKey` / PS CONTEXT「地址要素」词条），逐行扫新增行无行号、无「第 N 行」；新计数扫描：「一条 / 一次 / 两口同序 / 两次两个指纹」皆句内自指，不数别处；「五格」为既有字面（封闭集自名），非本票新增；作者自报删的「三处」确已不在；`**…**` 强调与既有头注同款。

  **Spec——阻断：0。非阻断：2。**【① `AddressElementsOutcome` 头注仍是缺口 4 修掉的那种宽口】`domain/address_elements_resolution.go` `AddressElementsOutcome` 头注「所以对今天全部快照口都答 NOT_PROVIDED」——03 评审说它「写对了」，我读来只靠上文「今天这一格」限定，句子本身与被修的口头注同宽；适配器头注「两段对全部快照都答『要素缺席』」后半有「锚 / 未定 / 无三格……真答」兜住，尚可。地盘外，不要求本票改，记给 PS owner 随 05 一并收窄。【② 缺口 2 契约句的序权威实际在库 collation——同意作者 ⑤，补 PP 侧一句】核过：`loadMembers`（`FindByKey` 走）`ORDER BY object_ref`、`LoadMembers` `ORDER BY member.object_ref`，同表同列；TF 领域 `RehydrateChargeOccurrence` → `TransportChargeOccurrence.members` 只 `append` 复制、`Members()` 亦复制不排——**两口同序结构上成立、且序的唯一权威就是那两条 SQL**。但「字面升序」= `text` 默认 collation 升序，非字节序；PP 消费侧若拿成员清单进计价输入快照的指纹，宜自己按字节序再排一次，不把指纹稳定性押在 TF 库 collation 上——归 PP 消费侧适配器那票，不在本票。

  **缺口逐条**：**1 ✓** 两处「三件」去净，改句「都是 CONTEXT『运输收费发生项』固定下来的」「不该看见协议、数量与修订」语义不变。**2 ✓** 契约段「成员按引用字面升序，与 `FindByKey` 读回的成员同序……实现者不得各排各的」与两条 SQL 一致（见 Spec ②）；理由句点明 PP 指纹需要，对方那条顺序断言随之成契约断言。**3 ✓** 三头注同口且同词（`AddressElements`「不去空白…空白串不是空串，原样在场」/ `AddressElementsOf`「在场与否只看值是不是空串，**不先去空白**」/ `AddressElementsView`「不校验、不规范化、不去空白——全空白的值原样在场，只有显式清空（值为空串）读作缺席」）；判定 `entry.Value() != ""` 与头注一致；**唯一行为改动的副作用核过**：`AddressElementsOf` 今天**没有任何非测试调用方**（`git grep` 非 `_test.go` 只命中定义），`SubmissionVersion.addressElements` 恒还 `AddressElements{}`，运行时不可达；下游 `AddressElementsOnBaseline` 只看 `Empty()`、无人读 `PostalCode()` / `CountryCode()` 的值（非测试零命中），无人假定非空白；05 落地后全空白邮编会以基线格原样交到 PP，PP 译不出 `PostalRoute` 是 PP 的事，与「值原样交、不校验」契约一致。新格钉「三个空格 → declared 且值原样」，空串缺席由既有 `TestAnExplicitlyClearedOrDuplicatedAddressElementReadsAsAbsent` 钉——两边各有一格 ✓；新格单列一段条目、不改既有那组「收件没报国家码」格的题 ✓。**4 ✓** 口头注「非成员答无、待复核答未定、已采用答已采用版本锚，只有锚落在接受基线上的那一段才去读要素——而锚在基线上的那一格今天恒答要素缺席」与 `AddressElementsFor`（非接受 / 非成员 → `NoShipmentAddressElements`）+ `addressElementsInGroup`（`undetermined` → 未定；`AdoptedVersion` → 已采用版本锚；否则 `addressElements(group)` 空 → 要素缺席）逐格对上；引导出符号 `AddressElementsFor` 而非未导出的 `addressElementsInGroup` ✓。

  **判断项**：① 同意（取默认口，让实现兑现类型自己头注的承诺；翻法票面已写）。② 同意（核过既有用例无全空白格；新格单列不混既有组）。③ 同意（只注释、同词而非仅不矛盾）。④ 同意（逐格核过，见缺口 4）。⑤ 同意，并加一条结构事实：领域层不重排，序权威只在两条 SQL；PP 侧兜底见 Spec ②。⑥ 同意（`charge_occurrence_registry.go` `LoadMembers` 头注「三件」确在、地盘外，TF owner 顺手）。⑦ 同意（作者自审不算独立评审；本条即那一份）。

  **结论**：两轴 0 阻断，**可进 main**；三条非阻断都不要求改。未改任何文件。
- **2026-09-15 12:3x · 进 main 记录 · 通道 1 推送方**：评审三条非阻断接受为残差，均地盘外或归他票：Standards ①（`AddressElements` 头注「只认这一种缺席」收成「值层面」）与 Spec ①（`AddressElementsOutcome` 头注宽口）记给 PS owner 随 [05](05-ps-submission-and-source-data-versions-carry-content.md) 一并收窄；Spec ②（成员序权威在 TF 库 collation，PP 消费侧进指纹前自排字节序）归 PP 消费侧适配器那票；作者判断项 ⑥（`charge_occurrence_registry.go` `LoadMembers` 头注「三件」）归 TF owner 顺手。**重放**：隔离检出 `%TEMP%\idp-replay-wave4` @ `49ffc96c`（sa-cc/28 两笔已在其上），`cherry-pick 69189208` 零冲突 → `4ca2ccac`，`git diff 69189208 4ca2ccac -- internal/transportfulfillment internal/parcelshipment` 与本票 .md 均空；再叠 sa-cc/27 取证 `af2b59db→676cc09b`，批 tip `676cc09b`。清点在 `4ca2ccac` 重生成**零差**（本批零增删文件，与作者预报同）。**验证（推送方全量一次，`4ca2ccac`——之后只叠一笔纯 .md）**：`gofmt -l` 空、`go build ./...` / `go vet ./...` 0；占 55432 → 带 DSN `go test -p 1 -count=1 ./...` **113 ok / 0 FAIL / 16 无测试 / 0 cached**（134 s）；探针 `TestAddressElementsAreReadByClosedEntryNameOnly -v` PASS → 释。本簿记笔在 `676cc09b` 之上 → 共享 main `merge --ff-only` → `ls-remote` 核 `49ffc96c` 未动 → `push <sha>:main`。分支 `mcp3-ppseams06` → `merged/`、远端删；作者树由通道 3 比内容后拆。
