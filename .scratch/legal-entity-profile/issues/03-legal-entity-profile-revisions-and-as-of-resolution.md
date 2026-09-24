# 03 法人资料修订链与按时点解析（含「资料不全」答复）

Category: enhancement
Status: resolved——2026-09-24 通道 3 在分支 `mcp3-lep03` 上做完（基 `mcp3-lep02` tip `91949459`；代码 tip `4b9811fe`），分支已推 origin。进 main 由推送方安排非作者评审后重放，只取本票的笔：`4ea41a98` 至 `4b9811fe` 与本笔票面；`f52c15bb` 是 main 上认领笔 `60d37d65` 的拣入，重放时为空。02 已进 main（`f2c8cd57`），本分支尚未挪到 main 上。迁移编号占 party-commercial `0036`（`0035` 属 catalogue-read-pagination/02）。完成记录见文末
Blocked by: 02
地盘：party-commercial 新增的法人资料（领域、应用、端口、postgres 与 http 适配器），`migrations/` 下 party-commercial 模块的新迁移，演示种子。
出处：[ADR-0145](../../../docs/adr/0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定三、五、六；CONTEXT「法人资料」词条、Rules 中法人资料修订与
开立时固定引用的句子、Lifecycles「法人资料」一节。

## 做什么

1. 法人资料修订：注册地址（带国家 / 地区）、税务登记号（可多个，经 01 的校验，资料层）、开票资料（首版只含开票抬头）、联系人（可空）；每修订带登记
   依据与生效时点，不可覆盖。注册地址的国家 / 地区须与身份上的注册国家 / 地区一致；所属责任法人已停用时拒登。
2. 登记写面进端点表（未配置即拒）；修订历史读口，形同既有的法人修订历史读口。
3. **按时点解析端口**：给定（租户，责任法人，时点），答当时有效的那一修订的引用与内容；没有有效修订或缺开票资料时答「资料不全」——明确非成功，
   不以默认值补齐。给开立方用的是修订引用，由开立方固定在单据上（决定五）。
4. 演示种子补合成资料。

## 不做

- 不改 settlement-accounting 或 customs-compliance（见 spec「不在本 spec」）。不加币种（决定四）。

## 完成判据

- 真库用例：前后两修订按生效时点切换；未来生效的修订在生效前不参与解析；追溯生效的修订登记后，按时点重读答新值——而固定过的引用仍指向旧修订；
  法人停用后不再参与新的解析；地址国家不符、法人已停用各拒一条；「资料不全」两种成因各一条。

## 完成记录（2026-09-24，通道 3，分支 `mcp3-lep03`）

**落点**

| 笔 | 段 | 做了什么 |
|---|---|---|
| `4ea41a98` | 领域 | 法人资料修订 `LegalEntityProfileRevision`（注册地址带国家 / 地区、税务登记号、开票资料只含抬头、联系人；修订号、登记依据、生效时点）；按时点解析 `ResolveLegalEntityProfile` 与答案代数（已解析、资料不全两成因、法人未登记 / 未生效 / 已停用） |
| `697b55d7` | 应用 | 资料登记用例（领域门 → 修订连续性 → 资料门：法人在册且未停用、地址国家对得上身份、税号按目录资料层判）与解析用例；窄口 `ports.LegalEntityRegistrationLookup`；判号拒因抽成 `registrationNumberRefusal`，身份登记与资料登记共用 |
| `31036a9f` | 持久化 | 迁移 `0036` 资料修订表；登记册适配器 `LegalEntityProfiles`（写口、修订链读口、修订历史读口） |
| `aa822530` | 持久化补证据 | 写口无事务即拒的 PBC-08 负向用例——`31036a9f` 漏了它，`internal/architecture` 自那笔起红，此笔消 |
| `7c7f738b` | http | 资料登记写口（只放 POST，未配置答 403 且不读载荷）与修订历史读口（集合格恒为数组，开票资料在不在用由 `invoicingRegistered` 显式说） |
| `2c4e97ab` | 装配（在线口） | 两口进 `cmd/parcel-api` 端点表：写口挂字面量 `UnconfiguredIntake{}`，读口挂商业目录 Intake；事务壳、未接线占位、路由探针与隔离读放行表 |
| `fef5bd6a` | 装配（受控口与种子） | 受控 CLI `register-legal-entity-profiles`；演示种子 `SYN-LE-01` 两笔资料修订，`seed.sh` 与种子 README 同步 |
| `54ceebce` | 注释 | 写口注释补回「与受控 CLI 同源」 |
| `ecdfbdf1` | 端口声明 | 身份登记册适配器编译期声明实现 `LegalEntityRegistrationLookup` |
| `4b9811fe` | 清点 | 机制清点在本分支重生成 |

**完成判据**

- ✅ 真库：`TestLegalEntityProfileResolvesByEffectiveTimeAgainstTheRealRegister`，逐条对应上面那句——两修订按生效时点切换（四月答修订 1、七月答修订 2）；未来生效的修订生效前不参与（二月答资料不全 · 没有有效修订）；追溯生效的修订 3 登记后七月重读答它，固定过的修订 2 引用在修订历史里原样指向原内容；法人停用后答法人已停用；地址国家不符、法人已停用各拒一条且一个字节不写；资料不全两成因各一条（后一条不退回前一笔、不补默认值）。
- ✅ 真库：`TestLegalEntityProfileRegisterReplaysAndReadsBack`——同内容重放答重复、同修订异内容答冲突；最新修订与修订历史读回逐格对得上；跨租户零行；空集合落空数组、缺开票资料落 NULL。
- ✅ 端点与装配：资料两口的端点用例；`cmd/parcel-api` 未配置面与隔离读 / 写准入的装配用例覆盖两口；`TestTheWiredLegalEntityProfileRegistrationLandsAgainstARealDatabase` 证真编排接对了法人身份读口与注册号类型目录（目录传 nil 即红）。
- ✅ 受控口：`TestRegisterLegalEntityProfilesBatchLandsRepliesAndRefuses` 真库端到端（落定、重放、冲突、地址国家对不上、法人未登记）与翻译纪律两条。
- ✅ 领域与应用层：内容形状门；修订必带依据与生效时点；解析取法与各非成功格；读侧交来的链不可信即拒；修订连续且不可覆盖；资料门各拒；重放照册面比对；只有带税号时才要目录。
- ✅ 演示种子：一次性库上 `seed.sh` 端到端零报错，`SYN-LE-01` 两笔 `REGISTERED`、库里两行，库已删。

**门**（钉 `4b9811fe`，跑在 `f52c15bb` 上、二者只差 `.md`；WSL，go1.26.8，DSN 为门禁库 55432）：`go build ./...`、`go vet ./...` 退 0；本票改动的 `.go` 按入库字节 `gofmt -l` 无输出；迁移 `0036` 无 CR、无 BOM；先单跑真库用例 `TestLegalEntityProfileResolvesByEffectiveTimeAgainstTheRealRegister` 是 `PASS` 不是 `SKIP`。本票改动包（PC 领域、应用、端口、postgres 与 http 适配器、`migrations`、`cmd/parcel-api`、`cmd/parcel-commercial`）与 `go list` 反查的反向依赖、`internal/architecture` 共 37 包 `-p 1 -count=1 -v`：35 包 ok、2 包无测试文件、0 FAIL；`--- PASS` 3814、`--- FAIL` 0、`--- SKIP` 1（`pgtest` 的 `TestHelperTemplateOwnerProcess`，只由另一用例以子进程驱动，与 DSN 无关）。全量由推送方跑。

**判断项**

1. **有效修订取「生效时点不晚于解析时点的修订里修订号最大的那一笔」**：照 CONTEXT「后一修订生效时，前一修订自该时点起不再参与新的解析」。追溯生效的修订因此取代它生效时点之后的全部前序修订；按生效时点取最晚的那种读法，会让被取代的修订在追溯修订之后又冒出来。
2. **表里不存状态，也不存「当前修订」**：哪一笔有效是对时点导出的，存一格就是存一份会被追溯修订改写的推导结果；开立方固定的修订引用就是主键，行写下不再改。
3. **缺开票资料登记时不拦、解析时答资料不全，且不退回前一笔**（ADR-0145 决定六）：退回会让开立方拿到一份已被取代的抬头。
4. **登记侧与解析侧读法人状态的口径不同，是有意的**：登记侧按「册上已登记停用」判、不问停用时点到没到，生效时点未到照收（可以先登法人、后补资料）；解析侧按解析时点读，停用自停用时点起不参与、生效时点未到答法人未生效。前者判的是还收不收新修订，后者判的是那一刻能不能对外开立。
5. **资料门只拦新修订，重放照册面比对**：法人此后停用了、目录修订了，拿今天的门拦重放会把`已登记`答错。
6. **税号判号时点取资料修订的生效时点**：号自那一刻起随资料对外使用，类型那一刻就得在用。
7. **法人身份读口单列窄口 `LegalEntityRegistrationLookup`**：资料用例拿不到身份登记册的写口；交入的是同一只身份登记册适配器，适配器编译期声明实现它。
8. **修订历史读口落在资料登记册适配器上，不在商业目录适配器上**：修订链读口与写口同表同适配器；`cmd/parcel-api` 因此另传一个读口参数，写编排另构造一只（适配器不带状态）。这一口只交修订事实，此刻有效的是哪一笔由按时点解析回答——追溯修订会让它随时点变。
9. **写口挂字面量 `UnconfiguredIntake{}`，不进隔离写准入**：票面要未配置即拒；隔离写按 ADR-0091 逐口放行，那是另一笔，管理台资料页（票 04）要写时再议。
10. **受控 CLI 的 `invoiceTitle`：缺席即不带开票资料，给了空串拒收**：空串不当缺席，缺席也不拿法人名称去补抬头。
11. **演示种子两笔修订**：修订 1 不带开票资料、修订 2 自 3 月起补抬头，让资料不全与修订切换都有真实例可显。

**未做 / 风险**

- 按时点解析只到应用用例与端口，没有读口或端点：开立方（结算、关务）不在本票改（见「不做」），接入时按消费方的形状开口。
- 写口未进隔离写准入（判断项 9）；管理台资料页与表单归票 04。
- 地盘越出票面登记的两处（开工前已报通道 1）：`cmd/parcel-api`（端点表两行、事务壳、未接线占位与测试表）、`cmd/parcel-commercial`（新子命令）；另动了机制清点。
- 本分支基于 lep02 旧 tip `91949459`；02 已进 main（`f2c8cd57`），重放前要把本分支挪到 main 上、只取本票的笔。机制清点两边都重生成过，挪时按新 tip 再生成。
- 演示种子里注册号类型目录与 `SYN-LE-01` 那一行，票 product-strategy-boundary/03（通道 4）也要动：资料种子依赖 `CN` 资料层 `SYN-CN-TAX` 与 `SYN-LE-01` 身份层国家 `CN`，已告知通道 4。
- `scripts/demo-seeds/seed.sh` 在 git 里是 `100644`，README 写的 `./scripts/demo-seeds/seed.sh` 在 Linux 上报 permission denied，要用 `bash` 调；既有问题，不在本票改。
