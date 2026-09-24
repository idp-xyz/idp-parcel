# 03 参考配置的存放、版本与显式采用路径，在注册号类型目录上立样板

Category: enhancement
Status: resolved——2026-09-24 通道 4 在分支 `mcp4-psb03` 完成（派单 task-a6b49376；基 `f2c8cd57`，代码 tip `1be757a6`）；待推送方派非作者评审后重放进 main，见文末「完成记录」
Blocked by: 无
地盘：参考配置的存放目录（本票定）；party-commercial 注册号类型目录的采用路径（`parcel-commercial` CLI 与登记用例）；演示种子里对应一行。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定三、越权风险点 4、5；[ADR-0145](../../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 越权风险点 2。

## 做什么

1. 定参考配置的形态：随产品版本发布的存放位置、标识与版本号的写法、校验方式（与登记册同一套领域构造门）。
2. 定采用路径：采用就是一次普通登记，依据格写「参考配置 标识@版本」；没采用照旧答`未配置`。
3. 在注册号类型目录上立样板：CN（统一社会信用代码）与 SG（UEN）两份公开格式作为参考配置；演示租户经采用路径登记它们，替换今天的合成格式条目。
4. 各登记册依据格能否容纳这种引用，列出与本样板不同的册，交各上下文 owner 复核。

## 不做

- 不定首版随附哪些国家与地区、是否带校验位算法——归 PC owner 与用户，本票只把 CN、SG 作样板并在完成记录里标「待定发布范围」。
- 不给任何真实租户登记。

## 完成判据

- 一份参考配置能经采用路径进入演示租户的注册号类型目录，未采用的租户照旧答`未登记`；采用记录的依据指向参考配置版本。

## Comments

### 派单前提示 ← 通道 3 · 2026-09-24（legal-entity-profile/02 作者）

本票与 [legal-entity-profile/02](../../legal-entity-profile/issues/02-legal-entity-identity-carries-registration.md) 交叠，后者在分支 `mcp3-lep02` 上，尚未进 main：

- 演示种子：02 让演示责任法人 `SYN-LE-01` 带注册国家 `CN` 与终身注册号（类型 `SYN-CN-LIFETIME`、号 `SYN-CN-000001`），登记时按注册号类型目录判号。本票把合成格式条目换成参考配置时，这个号要一并换成符合新格式的样例，否则 `register-parties` 答未受理，`seed.sh` 断在商业段。
- 判号口：02 经 `ports.RegistrationNumberTypeLookup` 的 `Check` 判终身注册号（身份层，按法人生效时点取目录修订）。采用路径若改了这个口的语义或目录修订的读法，02 的登记用例 `RegisterPartyIdentityHandler` 要一起看。

建议以 02 进 main 之后的 main 为基开工。

## 完成记录（2026-09-24，通道 4，分支 `mcp4-psb03`，基 `f2c8cd57`）

落点：ADR `a84b26e8`；参考配置包与首批两份原文 `db7ddfcf`（其后改名一笔 `1be757a6`）；登记入口采用路径 `d7c05f04`；演示种子 `b80fdb54`；本记录一笔。两条派单前提示照做：`SYN-LE-01` 的终身注册号已换成合格样例；判号口 `ports.RegistrationNumberTypeLookup` 与 `Check` 一字未动，`RegisterPartyIdentityHandler` 不受影响。

### 对完成判据

- ✅ 一份参考配置能经采用路径进入演示租户的注册号类型目录：种子里 `CN/USCC`、`SG/UEN` 两项经 `adopt` 答 `REGISTERED`；真库用例 `TestAdoptingAReferenceLandsItInTheAdoptersCatalogueOnly` 里采用方按目录判合格样例为 `ACCEPTED`。
- ✅ 未采用的租户照旧答`未登记`：同一用例里没采用的租户在同一国家 / 地区答 `COUNTRY_NOT_REGISTERED`。
- ✅ 采用记录的依据指向参考配置版本：`basis_ref` 为 `REFCFG-1:party-commercial/registration-number-types/CN@1`（用例断言，另在种子一次性库上查验，SG 同）。

### 对「做什么」1–4

1. ✅ 形态立在 [ADR-0147](../../../docs/adr/0147-reference-configuration-ships-embedded-and-is-adopted-through-ordinary-registration.md)：仓库根 `referenceconfig/<标识>@<版本>.json`，`//go:embed` 编进二进制；标识 `<上下文>/<目录>/<键>`，版本从 1 起；发布清单钉 sha256，改动探针实测改一字即答 `ErrAlteredAfterRelease`；校验走采用路径的同一段翻译过领域构造门，样例过 `Check`（`TestEveryReleasedRegistrationNumberTypeReferencePassesTheDomainGatesAndItsSamples`）。
2. ✅ 采用路径：`parcel-commercial register-registration-number-types` 批文每项可写 `adopt: <标识>@<版本>`，名称、层与格式取自参考配置，依据格写成 `REFCFG-1:<标识>@<版本>`；批文再写这四格即拒收，版本未发布、类型不在参考配置里、国家与键不符、缺生效时点都在触库前拒收（`TestAnAdoptItemRefusesContentOfItsOwnAndReferencesThatDoNotResolve`）。
3. ✅ 样板：CN 统一社会信用代码（GB 32100-2015 的编码格式）与 SG UEN（三类编码格式）各一份，只含格式正则。演示租户经采用路径登记，替换了身份层的合成条目（判断项 1）。**待定发布范围**：首版随附哪些国家与地区、是否带校验位算法，未定，归 PC owner 与用户。
4. ✅ 各登记册依据格能否装下引用串，与样板不同的列在下面「与样板不同的登记册」。

### 自验（钉 `1be757a6`）

全仓 `go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 空。带 DSN `go test -count=1 -p 1`：`referenceconfig`、`cmd/parcel-commercial`（`referenceconfig` 唯一的反向依赖，自身无反向依赖）与 `internal/architecture`，317 pass / 0 fail / 0 skip；真库用例单跑 `-v` 为 PASS 而非 SKIP。种子在一次性库 `idp_mcp4_psb03_seed` 上干净灌与 `--reset` 重灌均退 0，库已删。机制清点在 tip 重生成零差。

评审：`/code-review` 的隔离子代理今日在本机报认证错误未起跑，作者按两轴自查。Standards：注释全中文，跨文件引用用符号名与 ADR 号，无行号与跨文件计数；修订号、生效时点与租户都由批文给，不代填；领域包未改；架构门禁 `TestNoIdentityPrefixCarriesTheSeparator` 一处命名误触，按摘要串 `PSC-1` 的写法改名修正。Spec：见上两节。进 main 前请推送方派非作者评审。

### 与样板不同的登记册（第 4 步，交各上下文 owner 复核）

能装下引用串、与样板同形：依据格是非空自由串的各册，如 PC 产品—渠道映射（`MappingBasisReference`）。PC 参与方身份与关系的依据格也同形，但它们不是参考配置的候选。

不同：

- **PC 商业发布诸册**（服务产品、接单规则包、客户服务规则、授权规则等）：来历是待批准载体里的提交人与批准人（`publication_draft.go` 的 `Submitter` / `Approver`，ADR-0126），没有自由依据格。若要把某条规则族出成参考配置，采用须过批准流程，引用串无格可放——PC owner。
- **PP 计价参考序列**：一期取值带证据等级（`VERIFIABLE` / `ASSERTED`）与来源记录，在用版本由复核导出（ADR-0099）。参考配置在这里更像「来源连接器绑定」，不是一期取值——PP owner。
- **PP 计价参考目录**（邮编分区、偏远档位，ADR-0109）：与序列同族，按来源标识登记、经复核进在用；票 14 的邮编格式与单位对表若出参考配置，采用记录落哪一格待 PP owner 定。
- **VE 各目录**（里程碑映射、信号规则、披露策略等，`register_catalog.go`）：登记要求显式 `approvedBy`。引用串可以进依据，批准人仍须由租户给——两格并存还是批准可由采用代替，VE owner 定。
- **待核**（本次没查到依据格）：TF 按轨迹源登记的有效时间与收寄判读规则、CC 的门禁规则（`duty_payment_gate_rule.go`）、NR 的版本化网络目录——各 owner 在自己的首份参考配置落地时核。

### 判断项

1. **资料层合成条目未换。** 票面样板只点名 CN 统一社会信用代码与 SG UEN，二者都是身份层；两国资料层（税务登记号）的公开格式不在样板内，且 legal-entity-profile/03 正在用资料层合成条目。换它们归后续参考配置。
2. **放仓库根而不是各上下文内**，理由见 ADR-0147 候选丙；将来若按上下文归位，标识不含路径前缀，引用串不变。
3. **样例是格式合格的合成值**（行政区划码 `000000` 不存在、UEN 序号全零），与「夹具全 `SYN-`」纪律的张力写在 ADR-0147 越权风险点 2。
4. **再采用新版本走下一修订。** 同一类型日后采用 `CN@2`，就是批文给下一修订号、依据换成新引用串——与内容更正同一条路，不另设「升级」动作。
5. **发布清单写在 Go 源码里。** 新增一版要同时加文件与清单一行，漏一边 `TestEveryEmbeddedFileIsReleasedAndMatchesItsPinnedDigest` 即红。
