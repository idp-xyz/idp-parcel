# 05 跨境主体资料扩展：监管登记号语言放宽、收款账户、多语言开票抬头（领域 + 后端 + 种子）

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 无
出处：评估「四、国际小包视角」。用户 2026-11-03 裁定「你自己来决策」，以下为协调方代决（代 party-commercial owner），写进 ADR 时标「代决，待 PC owner 复核」。

## 决定

1. **海关登记号不新开一组，放宽资料层登记号的语言。** 法人资料里「税务登记号」改叫**监管登记号**：税务、海关等监管机关发给法人、可后办可变的登记号——GST / VAT / IOSS / EORI、海关注册编码、AEO 认证编号都在这一组。线上字段 `taxRegistrationNumbers` 与领域类型名**不改**（兼容；ADR 写明「名窄于义」的取舍与为什么不改名），界面词「监管登记号（税务 / 海关）」。类型来自注册号类型目录的资料层，按国家配置。
2. **产品随发布给一份注册号类型参考表**（产品策略，ADR-0146 第三类）：按国家 / 地区列常用的身份层与资料层类型——CN：统一社会信用代码（身份层）、税务登记号、海关注册编码、AEO 认证编号；HK：公司注册编号 CR No.（身份层）、商业登记号 BR No.；US：EIN、州注册号；SG：UEN（身份层）、GST 登记号；EU 各国：VAT、IOSS、EORI；UK：公司编号（身份层）、VAT、EORI；JP：法人番号（身份层）、适格请求书发行事业者登记番号。参考表是**产品附带的参考配置**，放哪里照仓里既有参考配置的落法（`docs/product/PRODUCT-STORY.md`、`PILOT-PARAMETER-REGISTER.md` 里「参考配置」的写法与 `scripts/demo-seeds` 的分工）；租户采不采用是租户取值；演示租户灌 `SYN-` 版本（现有 `register-registration-number-types.json` 扩）。格式正则只给公开规范有明确格式的（如统一社会信用代码 18 位、EIN 9 位），其余留空不猜。
3. **收款账户是法人资料的可选版本化明细**（照 ADR-0351 社交账号的模式）。一个资料修订带零或多个收款账户，每个：用途标签（可选，自由文本，如「人民币对公」）、户名、开户行名称、账号或 IBAN、SWIFT / BIC（可选）、开户行地址（可选）、**币种**（ISO 4217，必填——这是账户自身的属性，不是结算币种的决定；结算币种仍由结算政策解析，ADR 里这句照 CONTEXT 原话写清分界）、备注（可选）。同一修订内（币种，账号）不重复；缺席 = 这一版不带收款账户；旧行读作「未登记收款账户」，不回填。
   - **所有权**：`party-commercial`。`settlement-accounting` 的结算账户是另一个概念（固定责任法人、相对方、方向、币种、结算政策的业务账户），不是银行账户。对账单 / 发票要印收款账户时，开单方按单据固定的资料修订引用与单据币种，经消费侧适配器（ADR-0025）向 `party-commercial` 取，不复制。本票只在 ADR 写这条边界，**不改 SA**。
4. **多语言开票抬头**：`invoiceTitle` 保留为主抬头（兼容；`NO_INVOICING_DETAILS` 的判据不变——仍只看主抬头有没有）；新增可选 `invoiceTitles`：语言标签（BCP 47，如 `zh-Hans`、`en`、`zh-Hant`）→ 抬头，同一修订内语言不重复；可选 `invoiceTitleLanguage` 标主抬头的语言。哪种语言印到单据上由开单方按收件方决定，不在本票。
5. 三类：1、3、4 的机制与缺席语义是**机制**；参考类型表与「主抬头判开票资料齐」是**产品策略**；具体账户、具体抬头、租户采用哪些类型是**租户取值**，留空，演示用 `SYN-`，证据只记 `S`。

## 要做的

1. **ADR-0359**（取 origin/main 与各 `origin/claude-*` 都没占的下一个号，动笔前核；Status Accepted（协调方代决，用户 2026-11-03 授权；越权风险点待 PC owner 复核，收款账户与 SA 的边界待 SA owner 会签）。按上面五条写 Context / Decision / Consequences / Alternatives（含「海关登记号另开一组」「收款账户归 settlement-accounting」「只做单抬头」）/ 越权风险点。若改了 ADR-0145 / 0350 / 0351 的某句，照 `docs/adr/README.md` 顶部部分停用写法挂指针、引原句逐字。
2. **CONTEXT / GLOSSARY / UC-PC-001**：`docs/domain/party-commercial/CONTEXT.md`「法人资料」语言与规则（税务登记号 → 监管登记号那一句、收款账户、抬头多语言、缺席语义）；GLOSSARY「法人资料」同步；UC-PC-001「法人资料维护补充」加一段并立 AT 行（取未占的 AT-PC 号）；AT 盘点照惯例加行，不手改总览计数。
3. **后端**（`internal/partycommercial`）：domain（`legal_entity_profile_details.go` 的模式扩两组 + 校验：币种 ISO 4217 形状、语言标签形状、同修订内不重复）、registrationjson 读入（旧载荷照收）、ports、postgres（迁移 `party_commercial` 下一个号：`legal_entity_profile_revision` 加可空 `bank_accounts jsonb`、`invoice_titles jsonb`、`invoice_title_language text`；只增，在有数据的库上可施加；快照 `snapshot` 若整份存则形状版本号照既有 `shape` 机制升一版）、解析读口与修订读口读回、HTTP 答复字段、重放幂等、旧行读作缺席的兼容测试（照 `legal_entity_profile_compatibility_test.go` 的写法）。
4. **种子**：`scripts/demo-seeds/data/commercial/register-company-demo-profiles.json`（或 social-profiles 那份）给五家各加 `SYN-` 收款账户（至少一家两个币种）与双语抬头；`register-registration-number-types.json` 扩资料层类型（CN 海关注册编码、AEO；HK BR；US EIN；EU VAT / IOSS / EORI）并给演示法人资料各带一个海关 / 税务登记号；参考类型表落在决定 2 定的地方；`steps/800-company-subject-demo.sh` 跟上。一次性库灌种子退 0。
5. **管理台不在本票**：表单、读出、差异预览、列表搜索归票 06（等票 04 进 main 后做）。

## 不做

- 不改 `settlement-accounting`；不做对账单 / 发票选账户、选抬头的逻辑。
- 不改线上字段名 `taxRegistrationNumbers`；不回填旧行。
- 不设任何默认账户、默认抬头、默认类型采用。

## 完成判据

- `go build ./...`、`go vet ./...` 退 0，`gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/partycommercial/... ./internal/architecture/... ./cmd/... ./migrations/...`（含 PG）全过；新用例 `-v` 无 SKIP：旧行读作缺席、重放答原修订、同修订内币种 + 账号重复被拒、语言标签重复被拒、迁移在有数据的库上可施加。
- 一次性库（名带 csux05）灌种子退 0，解析读口答出的第 N 版带收款账户与双语抬头。
- `adr-index -check`、`at-coverage-counts -check` 只报生成物过期。
- 照「进 main 的步骤」第 1 步交非作者评审。
