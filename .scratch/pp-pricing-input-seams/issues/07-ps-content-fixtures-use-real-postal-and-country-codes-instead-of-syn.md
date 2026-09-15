# pp-seams/05 评审 Standards 尾巴：四份 PS 内容用例的收件夹具用真实邮编 `10115` / `20095` / `20097` / `20099` 与国家码 `DE`，寄件格已是 `SYN-200000` 而收件格没跟；03 落 main 的两份地址要素用例同款

Category: chore
Status: resolved——**已进 main，2026-09-15 15:1x 通道 1 推送方**（第七批，重放 `0b3e5de9→e1840144`，批 tip `c84e81bd`；评审门推送方自审（零行为：生产零 diff、六份测试每一加行都带 `SYN-`、`-w` grep 真码零命中）；`c84e81bd` 带 DSN 全仓 115 ok / 0 FAIL，PS content / elements 真库格 PASS 非 SKIP；见 Comments「进 main 记录」）。此前 resolved——**2026-09-15 15:0x 通道 5**（task-f70d20a8；分支 `mcp5-ppseams07` 基 main `93840328`，代码 tip 即本笔；六份测试文件只换字面，零行为）。此前 ready-for-agent——**2026-09-15 14:4x 通道 1 立票**（pp-seams/05 评审 ← 通道 3 Standards ①，推送方处置「另立 chore、归 PS owner」）。只测试夹具字面，零行为改动
Blocked by: 无（05 已进 main，第六批 tip `de822820`）

## 缺口（评审钉 `a893999a`，进 main 后在 `de822820` 同形）

- `internal/parcelshipment/adapters/http/isolated_write_intake_test.go`、`domain/source_data_version_content_test.go`、`adapters/postgres/source_data_version_content_test.go`、`application/amend_customer_source_data_content_test.go` 的收件 `POSTAL_CODE` 用 `10115`（柏林）、`20095` / `20097` / `20099`（汉堡），`COUNTRY_CODE` 用 `DE`；票 05 红线「真实邮编 / 真实申报属实例半边；夹具全 `SYN-`」，同一批用例的寄件格已用 `SYN-200000`。
- 先例：03 落 main 的 `domain/address_element_test.go` / `address_elements_resolution_test.go` 已用 `10115`——评审据此判非阻断（公开邮编、非租户实例），但纪律要一致：要么全 `SYN-`，要么在 PS CONTEXT「地址要素」词条写明「公开邮政编码不算实例半边」。**本票默认取前者**（与 05 红线字面一致）；取后者归 PS owner 一句并改词条，不在本票。

## 做法

1. 六份文件里的真实邮编 → `SYN-` 前缀的合成串（如 `SYN-100000` / `SYN-200001`…，与既有 `SYN-200000` 同形）；`DE` → `SYN-CC` 一类合成国家码——**先量** `AddressElements` / `AddressElementsOf` 对国家码有没有格式门（ISO 两位）；有则用保留测试码（ISO 3166 用户自定义段 `XA`–`XZ` / `ZZ`）并在该文件头注一句为什么。
2. 断言若比对字面同改；行为零变。

## 红线

- 零行为改动：`git diff --stat -- ':!*_test.go'` 空。
- 不改 `AddressElements` 的格式门；不改 `PayloadDigest` 用例（`payload_canonicalization_test.go` 不在地盘）。

## 完成判据

1. `git grep -n -E '10115|2009[579]|"DE"' -- internal/parcelshipment` 零命中（或只剩作者写明理由的保留测试码）。
2. `gofmt -l` 空；`go vet ./internal/parcelshipment/...` 0；不带 DSN PS 全部包 ok。
3. 完成记录同笔；清点零差。

## 地盘

`internal/parcelshipment/{adapters/http/isolated_write_intake_test.go,domain/source_data_version_content_test.go,adapters/postgres/source_data_version_content_test.go,application/amend_customer_source_data_content_test.go,domain/address_element_test.go,domain/address_elements_resolution_test.go}`（只测试）。撞点：无在途分支碰 PS。

## 完成记录（2026-09-15 15:0x 通道 5，task-f70d20a8，分支 `mcp5-ppseams07` 基 main `93840328`，单笔即 tip）

**逐条对判据**：
- **判据 1 ✓** `git grep -n -E '10115|10117|2009[579]|[^-]200001' -- internal/parcelshipment` 零命中；`git grep -n -w -E 'DE|CN' -- <六份文件>` 零命中。没有用保留测试码——国家码无格式门（见判断项 ①），全部 `SYN-`。`payload_canonicalization_test.go` 里 `recipient_country` 条目的 `"DE"` 原样不动：它不在地盘（摘要哈希用例零改，票面红线），且那条目名是租户约定名、不是封闭要素——全范围上 `"DE"` 剩下的命中只有这一份。**复核提醒**：票面那条 `git grep -n -E '10115|2009[579]|"DE"'` 在 PowerShell 里内层双引号会被剥掉，`"DE"` 退化成裸 `DE`，会匹到 `UNDETERMINED` / `DELIVERY_PLACE` 一类几百行（实测于本树），零命中与否都不能拿它当国家码的证据；核国家码请用上面那条 `-w -E 'DE|CN'` 限定六份文件。
- **判据 2 ✓** `gofmt -l ./internal/ ./cmd/` 空；`go vet ./internal/parcelshipment/...` 退 0；不带 DSN `go test -count=1 -v ./internal/parcelshipment/...` **1640 PASS / 0 FAIL / 307 SKIP**，退出码 0，PS 全部包 ok（SKIP 全是真库用例无 DSN 的诚实跳过；`adapters/postgres/source_data_version_content_test.go` 那份的字面改动只由编译与 `go vet` 守，55432 按派单留给 sa-cc/31 未占，推送方全量时跑）。
- **判据 3 ✓** 本记录同笔；无新增 / 删除文件，清点零差（生成器输入是文件面与声明数，字面不影响）。

**逐条对做法**：
- **做法 1** 六份文件里的真实邮编 → `SYN-` 六位数字（与既有 `SYN-200000` 同形）：`10115` → `SYN-100115`、`10117` → `SYN-100117`、`20095` → `SYN-200095`、`20097` → `SYN-200097`、`20099` → `SYN-200099`、`200001` → `SYN-200001`；国家码 `DE` → `SYN-CC`、`CN` → `SYN-CC2`（寄件那一格与收件用的合成码分开，保住「收件范围上没报国家码不拿寄件的顶」那条断言的分辨力）。映射一对一，同一真值在六份文件里换成同一个合成串，跨文件仍能对读。
- **做法 2** 断言里的比对字面（`postal != "10115"` 一类）、`t.Fatalf` 消息里的 want 字面、头注 / 用例注释里的举例字面同改；行为零变——`git diff --stat -- ':!*_test.go' ':!.scratch'` 空。

**判断项**：
- **① 国家码有无格式门——无。** `AddressElementsOf` 只按封闭条目名挑值、`entry.Value() != ""` 判在场；`NewCanonicalContentEntry` 只对**名**去空白并要求非空，值原样收；`AddressElements.with` 直接存串。没有任何 ISO 3166 / 两位长度的校验（PS CONTEXT「地址要素」「本上下文不校验、不规范化、不去空白」在代码上成立）。所以走票面「无则 `SYN-CC` 一类」那一支，不需要 `XA`–`XZ` / `ZZ` 保留码，也不必在任何文件头注写理由。
- **② 选了哪种测试码**：邮编 `SYN-` + 六位数字，首位沿用原真值的首位（`1xxxxx` 收件基线、`2xxxxx` 修订链）只为读 diff 时对得回去，没有任何语义；国家码 `SYN-CC`（收件）/ `SYN-CC2`（寄件，仅 `address_element_test.go`）。没有一个合成串是任何国家 / 地区的真实邮编或代码形。
- **③ 改了几处字面（本笔实测，钉本 tip）**：六份文件共 62 行改、65 处字面替换（`isolated_write_intake_test.go` 6 / `domain/source_data_version_content_test.go` 28 / `adapters/postgres/source_data_version_content_test.go` 14 / `application/amend_customer_source_data_content_test.go` 5 / `domain/address_element_test.go` 8 / `domain/address_elements_resolution_test.go` 4）；`git diff --numstat` 每份文件增删行数相等。
- **④ 票面漏列三处真值，同笔一并换**：票面缺口与判据 1 的模式只点名 `10115` / `20095` / `20097` / `20099` / `DE`，而 `domain/address_element_test.go` 里另有 `10117`（柏林，同名矛盾条目那一例）、`200001`（上海，寄件邮编）、`CN`（寄件国家码）——同一份文件、同一条纪律，留着就是「全 `SYN-`」只做了一半；按票面「六份文件里的真实邮编」的字面一并换了，判据 1 的 grep 因此比票面多三个字面。
- **⑤ 留着没动的两处，不是邮编 / 国家码**：`address_element_test.go` 里 `recipient.address` 条目的值 `1 Example Street, Berlin` 与 `postal_code` 条目的 `99999`——前者是占位地址文本（`Example Street`），后者是租户约定名下的假值，两条都是「不被封闭集认」的反例条目，不在本票「真实邮编 / 国家码」的字面内；`Berlin` 一词随 `10115` 消失后不再指向任何真实邮编，留作地址文本占位。
- **⑥ 与 05 红线的关系**：05 完成记录与评审都把「夹具全 `SYN-`」当红线而 03 / 05 的收件格没跟，本票补齐后 PS 六份地址要素相关用例里再无公开邮编；CONTEXT「地址要素」词条**未改**（票面默认取「全 `SYN-`」而不是「公开邮政编码不算实例半边」那一支，后者归 PS owner）。

**验证**（隔离 worktree `%TEMP%\idp-parcel-mcp5-ppseams07` 基 `93840328`，14:5x–15:0x）：见判据 2；六份文件经 .NET UTF-8 无 BOM 读写、LF 保持（`git diff --numstat` 增删相等，无整文件差）。未占 55432（派单：31 要用）。

**能力边界**：读过票面全文、六份文件里每一处命中的上下文、`AddressElementsOf` / `NewCanonicalContentEntry` / `AddressElements.with` 正文；**没读** `apps/admin-web`、PS CONTEXT「地址要素」词条正文（本票不改它）。真库那份用例的改动没有在真库上跑过，靠编译 + vet + 推送方全量兑底。

## 另记（不在本票，随 05 进 main 一并登在 spec 残差）

- 05 作者判断项 ⑥：`apps/admin-web` 提交页未长寄 / 收邮编与国家 / 地区码四格，隔离形态经页面进来的请求仍答要素缺席——归 admin-web 侧，等 PS 真渠道 Intake（PAR-INT-01）或管理台演示页票。

## 参照

[05](05-ps-submission-and-source-data-versions-carry-content.md) 红线与 Comments「评审 ← 通道 3」Standards ①；[06](06-tf-ps-ppseams01-03-review-tails-member-order-contract-count-words-and-blank-value-reading.md)（同款 A 类尾巴先例）；AGENTS「敏感实例外置」；PS `CONTEXT.md`「地址要素」。

## Comments

- 2026-09-15 15:0x · 通道 5（task-f70d20a8）：完成记录见上，单笔。与票面不符一处：`address_element_test.go` 里另有 `10117` / `200001` / `CN` 三处真值，票面与判据 1 的模式没点名，按「六份文件里的真实邮编」字面一并换了（判断项 ④）。真库那份用例未在真库跑（55432 按派单留给 sa-cc/31），推送方全量兑底。
- 2026-09-15 14:4x · 通道 1：立票（05 评审尾巴）。只写票面，未动代码。**能力边界**：文件名与字面取自评审原文；推送方**未读**这四份用例正文与 `AddressElements` 的格式门。
- **2026-09-15 15:1x · 进 main 记录 · 通道 1 推送方**：**评审门推送方自审**（只测试夹具字面、零行为，照 [06](06-tf-ps-ppseams01-03-review-tails-member-order-contract-count-words-and-blank-value-reading.md) 先例）：`git diff --stat 93840328 0b3e5de9 -- ':!*_test.go' ':!.scratch'` 空；六份测试 `-U0` 的每一条加行都含 `SYN-`（`Select-String` 反滤零命中）；`git grep -n -w -E 'DE|CN|10115|10117|20095|20097|20099' 0b3e5de9 -- <六份>` 零命中。作者判断项 ①–⑥ 接受——④ 票面漏列的 `10117` / `200001` / `CN` 按「六份文件里的真实邮编」字面一并换，在地盘内、同一缺陷；⑥ CONTEXT 词条未改与票面默认一致。作者提醒采纳：票面判据 1 那条 grep 的 `"DE"` 在 PowerShell 里会被剥成裸 `DE` 匹到 `UNDETERMINED`，核国家码用 `-w`。**重放**：`%TEMP%\idp-replay-wave7` @ `93840328`，`cherry-pick 0b3e5de9` → `e1840144` 零冲突，`internal/parcelshipment` 对作者 tip 零 diff；同批 sa-cc/33（`60721f60→3e0f1230`）与 sa-cc/32 取证笔（`d2af6ba1→c84e81bd`），零文件重叠。清点在 tip 重生成零差。**验证（推送方全量一次，`c84e81bd`）**：`gofmt -l` 空；`go build` / `go vet` 0；15:06 占号 → 带 DSN `go test -p 1 -count=1 ./...` **115 ok / 0 FAIL / 16 无测试 / 0 cached**（136 s）；`-v` 探针 PS `adapters/postgres` + `domain` 的 `Content|Elements|OldShapeSnapshot` 各格 PASS 非 SKIP（真库那份字面改动由此兑底）→ 15:09 释号。簿记一笔在其上，纯 .md 自审；`ls-remote` 核 `93840328` 未动 → `merge --ff-only` → `push <sha>:main`。分支 `mcp5-ppseams07@0b3e5de9` 作封存出处、改名 `merged/`、远端删；作者树由通道 5 比内容后拆。
