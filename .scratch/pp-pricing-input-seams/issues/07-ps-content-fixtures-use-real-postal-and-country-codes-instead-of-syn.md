# pp-seams/05 评审 Standards 尾巴：四份 PS 内容用例的收件夹具用真实邮编 `10115` / `20095` / `20097` / `20099` 与国家码 `DE`，寄件格已是 `SYN-200000` 而收件格没跟；03 落 main 的两份地址要素用例同款

Category: chore
Status: ready-for-agent——**2026-09-15 14:4x 通道 1 立票**（pp-seams/05 评审 ← 通道 3 Standards ①，推送方处置「另立 chore、归 PS owner」）。只测试夹具字面，零行为改动
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

## 另记（不在本票，随 05 进 main 一并登在 spec 残差）

- 05 作者判断项 ⑥：`apps/admin-web` 提交页未长寄 / 收邮编与国家 / 地区码四格，隔离形态经页面进来的请求仍答要素缺席——归 admin-web 侧，等 PS 真渠道 Intake（PAR-INT-01）或管理台演示页票。

## 参照

[05](05-ps-submission-and-source-data-versions-carry-content.md) 红线与 Comments「评审 ← 通道 3」Standards ①；[06](06-tf-ps-ppseams01-03-review-tails-member-order-contract-count-words-and-blank-value-reading.md)（同款 A 类尾巴先例）；AGENTS「敏感实例外置」；PS `CONTEXT.md`「地址要素」。

## Comments

- 2026-09-15 14:4x · 通道 1：立票（05 评审尾巴）。只写票面，未动代码。**能力边界**：文件名与字面取自评审原文；推送方**未读**这四份用例正文与 `AddressElements` 的格式门。
