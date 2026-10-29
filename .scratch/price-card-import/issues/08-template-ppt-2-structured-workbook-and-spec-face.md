# 08 模板 PPT-2、结构化工作簿载体与模板规格口（价卡配置编辑器的后端第一步）

Category: enhancement
Status: resolved——2026-10-29 复评通过并进 main（见末尾「评审记录」「进 main 记录」）。此前：in-progress——2026-10-29 作者完工，评审（钉 `8d6b6961e`）阻断 1 条已在修订轮改完，待复评与进 main，分支 claude-tmplpt2（代码最后一笔 `ad21b8ee2`）
父票：[spec](../spec.md)
Blocked by: 无
地盘：`internal/parcelpricing/{adapters/pricecardtemplate,adapters/http,application,ports,domain}`、`cmd/parcel-api` 的端点表与装配、`docs/adr`（0349 与 0348 的前向指针）、`docs/design/pp-price-card-import-template-and-validation-spec.md`、`docs/application/parcel-pricing/UC-PP-001-FORM-ESTIMATE-EVALUATIONS.md`；`apps/admin-web` 只新增一份静态模板文件，源码不动。
出处：协调方派单（价卡配置 · 第一步（后端））；[ADR-0348](../../../docs/adr/0348-a-price-card-may-declare-total-charge-bounds-and-key-a-surcharge-table-by-remote-tier-and-plans-using-them-are-written-as-ppc-6.md) 决定六留下的「导入暂不支持」；管理台价卡编辑器（下一步，前端切片）要直接提交结构化数据并照规格画表单。

## 要做的

1. 模板 `PPT-2`：新增 `total_bounds` 表与 `calculations` 的计算类型 `TABLE_LOOKUP_BY_REMOTE_TIER`；只有用到新结构的方案写 `PPC-6`；`PPT-1` 文件照收且读出一字不差。
2. `POST /pricing-price-card-previews` 与 `/pricing-price-card-drafts` 在 `Content-Type: application/json` 时收结构化工作簿，译成与读电子表格同一个内部表示再走同一套译码与校验；来源标识 `form:<规范化工作簿 SHA-256 前 12 位>`，草稿读口标明来自页面表单。
3. `GET /pricing-price-card-template/spec` 答模板版本、表清单与每一列，由读取模板的同一张列定义生成。
4. 不改变任何今天已算对的结果；ADR、UC 与规范同步改成现行。

## 完成记录

**起点** `origin/main` `2986a040c`。**提交**：`e79eb408c`（模板 PPT-2、工作簿载体、规格生成、空白模板与适配器层用例）、`f4771001e`（规格口按端点表读行同形接线，各层用例）、其后一笔文档（ADR-0349、ADR-0348 指针、规范、UC、AT 盘点行、本票）。分支 `claude-tmplpt2`，只推分支不推 main。

**怎么做的**（决定与理由在 [ADR-0349](../../../docs/adr/0349-the-price-card-import-template-advances-to-ppt-2-and-its-structured-workbook-is-a-second-carrier-read-through-the-same-path.md)）

- 偏远档位查价表**没有新造 `remote_tier_amounts` 表**：计算类型 `TABLE_LOOKUP_BY_REMOTE_TIER` 指 `tables` 里一张价表，价表行的 `zone` 列写档位。新表会丢重量分档、还得合成价表与行标识，FULL-03 回环就对不上（ADR-0349 Alternatives）。PPT-2 因此只新增一张表（`total_bounds`，列 `minimum`、`maximum`）和一个计算类型。
- 模板版本：写出 `PPT-2`，接受 `PPT-1` 与 `PPT-2`。`decodeWorkbook` 先认文件声明的版本，再按版本挑表集合与封闭取值；`PPT-1` 文件里出现新表或新取值答「自模板 PPT-2 起才有…请换用 PPT-2 模板」。封闭取值集中在 `closedVocabulary` 登记处，读取（`decoder.closedCode`）与规格口只从那里取。
- 载体：`workbook_json.go` 严格解析（重复键、多余顶层键、尾随内容、嵌套过深整份不收）、规范化（键排序、去空白、数字落成字符串）、译成 `spreadsheet.Workbook`，之后是同一个 `decodeWorkbook`。预览与录入共用 `previewOf`；`FromForm` 命令位与 `WithWorkbookReader` 可选件，原有构造函数调用点与测试替身一字未动。
- 规格口按端点表的读行同形接线（读口带 ctx 与错误返回、未接线占位如实答没接线），所以装配测试里那条「授予齐备越过 Intake 后读口未接线答 `NO_ANSWER_FORMED`」的既有用例原样覆盖了它。

**代决与越权风险点**（均写进 ADR-0349 越权风险点，待 PP owner 复核）：0 载体分叉防范；1 规格口与代码集合同源；2 `sourceFileName` 约定与「换载体再录是修订不是重放」；3 导入暂不支持项（已登记价卡反推成模板、带摘要的版本引用、一份工作簿多个版本、总额口径与联合键、表单本体不存）；4 编辑器与模板演进；**5 `PPC-6` 现在可经导入产生，ADR-0348 风险点 6 的滚动发布约束从文档约束变成真实风险（代码里仍没有门）**；6 `PPT-1` 兼容窗口没有终点；7 `zone` 列承载档位的命名；8 空 `total_bounds` 行与「明确不设」不可分。

**判据对用例**（AT 号取 `AT-PP-041` 至 `047`，核过 origin/main 与各 `claude-*` 分支无撞；UC 表在 UC-PP-001「价卡导入：模板 PPT-2、结构化工作簿与模板规格口」，盘点 ① 桶补了七行）

| AT | 判据 | 用例 |
|---|---|---|
| 041 | FULL-03 用模板重新表达，两种载体都得种子的摘要、`PPC-6`、`plan` 块逐值相等；golden 夹具不漂 | `TestFull03ReExpressedInTheTemplateImportsToTheSeedPlan`、`TestFull03GoldenFixturesAreCurrentAndImportToTheSeedDigest` |
| 042 | PPT-1 文件摘要与规范化版本不变；PPT-2 无新结构仍 `PPC-5`；旧版取值清单不泄新取值 | `TestPPT1FilesStillImportToTheSameDigestAndPPT2WithoutNewStructuresStaysPPC5`、`TestPPT1ClosedValueListingsDoNotMentionPPT2Values` |
| 043 | PPT-1 文件里出现新表 / 新取值答明确的问题；不认的版本列出认的全部版本 | `TestPPT1FilesUsingPPT2ThingsAreToldWhy`、`TestUnsupportedVersionNamesTheAcceptedOnes` |
| 044 | `total_bounds` 各写法与拒收，问题落在格上 | `TestTotalBoundsRowsAreReadAndRefusedAtTheirCells` |
| 045 | 缺目录绑定的问题落在计算行；`PPC-5` / `PPC-6` 随结构；表单草稿存入、标 FORM、批准发布、册上读回 `PPC-6` | `TestRemoteTierLookupNeedsTheCatalogueAndOnlyNewStructuresWritePPC6`、`TestADraftFromAFormSaysSoAndAFileDraftSaysNothingNew`、`TestAFormSourceFileIdentityIsNamedAfterItsOwnDigest`、`TestSubmitSavesAFormWorkbookAsADraftWhoseSourceIsTheForm`、`TestFormWorkbookTravelsTheWiredImportFacesAgainstARealDatabase`、`TestAFormDraftOfTheNewStructuresIsApprovedAndPublishedAsPPC6` |
| 046 | 两种载体同内容同读法同问题清单（含坐标）；规范化与来源标识；形状不对整份不收；`Content-Type` 分流 | `TestWorkbookJSONReadsExactlyWhatTheSpreadsheetReads`（24 组同内容对拍，其中带问题的写法 20 组）、`TestCanonicalWorkbookIdentityIgnoresWhitespaceKeyOrderAndNumberQuoting`、`TestMalformedWorkbookJSONIsRefusedWithAReason`、`TestWorkbookJSONCarrierConventions`、`TestOperatorIntakeTakesAJSONWorkbookAndLeavesOtherContentTypesToTheUploadPath`、`TestPreviewReadsAFormWorkbookThroughTheWorkbookReader`、`TestPreviewOfAFormWorkbookRefusesWhatCannotBeRead` |
| 047 | 规格与列定义逐项对应；封闭取值同源；传输面与授予、租户 | `TestSpecMirrorsTheSheetAndColumnDefinitions`、`TestSpecAllowedValuesAreTheClosedVocabulary`、`TestDecoderReadsClosedColumnsThroughTheRegistry`、`TestTemplateSpecEndpointGatesBeforeAnsweringAndTakesNoQuery`、`TestTemplateSpecEndpointAnswersTheSourceSpecInItsDocumentedShape`、`TestPriceCardTemplateSpecEndpointAnswersTheRealSpecThroughTheOperatorChannel` |

**PPT-1 零改动证据**（取证于起点 `2986a040c`，只记 `S`）

- 测试文件对 origin/main：只新增、不删不改。既有的 `*_test.go` 里只有 `cmd/parcel-api/endpoints_pricing_catalogue_test.go` 动了，在未接线占位字面量里新增一行；其余都是新文件。`git diff --numstat origin/main -- '*_test.go'` 的删除列全 0。
- 跨树对拍：在 `git worktree` 检出的 origin/main 上用既有夹具生成器（成本卡、销售卡、全结构卡，加 43 组改坏的写法：封闭取值写错、时刻、科学计数法、必填格为空、引用不存在、成环、区间重叠、方向与目的不配、缺进位段、币种不在目录、整表缺失、缺列、未知列、公式格、错误值、日期格、数值格精确与带浮点尾数……）共 46 份 PPT-1 文件，先在 origin/main 解一遍，再在本分支解一遍，两次各自生成与解；另把 origin/main 生成的 36 份 xlsx 原样交给本分支的读口解。读法的版本回显、受理与否、方案身份、内容摘要、规范化版本、登记快照全文、方向授权、问题清单每一条（表、行、列、码、话）逐位比对：**42 份逐位相同**（含 4 份已校验的卡——摘要与登记快照全文相同，规范化版本 `PPC-5`——和 38 份带问题的）；**4 份不同，且都不是可识别的 PPT-1 文件**：不认的版本 `PPT-0`、缺 `card` 表、不是工作簿、超文件上限——它们的差别只有两处：回显的模板版本由 `PPT-1` 变 `PPT-2`（读不出版本时回显最新一版），以及「本构建只认 PPT-1」一句变成「本构建认 PPT-1、PPT-2」（版本不认和缺 `card` 表两份）。生成器 xlsx 字节两树逐字节相同。对拍脚手架在草稿区，不入库。
- 既有已登记价卡与草稿的行为：本改动没碰评价、登记、草稿册与迁移；草稿本体仅在表单草稿上多一个可缺的 `sourceOrigin`，上传文件的草稿答复没有这个键。

**FULL-03 回环验收**：用模板写法重新表达 `SYN-PLAN-CN-SG-FULL-03`（`full03Card`，偏远一条规则按 `TABLE_LOOKUP_BY_REMOTE_TIER`、最低收费 120.99），电子表格与结构化工作簿两种载体导入，**内容摘要与种子卡 `dda7cfb5bc4f2374af997e41e7378625923b98d9ab648d5dfc4576edad6677da` 相同，规范化版本 `PPC-6`，登记快照里的 `plan` 块与种子 `price-card-cn-sg-full-03.json` 的 `plan` 块 `reflect.DeepEqual` 相等**。golden 夹具入库：`internal/parcelpricing/adapters/pricecardtemplate/testdata/` 下的 xlsx 与 JSON 工作簿各一份，用例核它们与测试里的写法逐字节相同（`-update-golden` 重写）。真库上又走了一遍：表单草稿 → 查阅标 `FORM` → 批准 → 发布 `RECORDED` → 册上读回 `PPC-6` 且摘要等于种子。

**自验**（`HTTPS_PROXY=http://127.0.0.1:7897`、DSN `postgres://parcel:parcel@127.0.0.1:55432/postgres?sslmode=disable`，含 PG）：`go build ./...`；`go vet ./internal/parcelpricing/... ./cmd/...`；改过的 `.go` 跑 `gofmt -l` 无输出；`go test -count=1 -p 1 ./internal/parcelpricing/... ./internal/platform/... ./internal/architecture/... ./cmd/...` 全绿（含 PG，`internal/architecture` 与 `cmd/parcel-api` 在内）；一次性库 `tmplpt2_seed_*` 灌 `scripts/demo-seeds/seed.sh` 退 0，用后删库；`go run ./tools/at-coverage-counts -check` 只报生成物过期，行齐；`go run ./tools/adr-index -check` 只报索引过期（`go run ./tools/adr-index` 在临时试跑过，ADR-0349 与 ADR-0348 两行生成无误，已还原，README 不在本分支动）。`apps/admin-web` 源码一字未动，只新增 `public/templates/price-card-import-template-PPT-2.xlsx`（`cmd/parcel-pricing-template` 的生成器逐字节比对用例绿）；`tsc -b` 与前端单测未跑（未碰前端源码，工作树里也没有 `node_modules`）。没跑全量，没碰 5173、`:19080`、共享库与库 `demo_live`。

**留给协调方**

- ADR 索引、AT 盘点计数、机制清点：进 main 时重生成（本分支没动生成物）。AT-PP-041 至 047 紧接 AT-PP-040 之后，UC 表与盘点 ① 桶里与别的分支的相邻行可能有文本冲突，重放时取并集。
- **冻结清单 2026-10-29 条目**要改：「价卡导入暂不支持总额上下限与偏远档位查表」已由本票补掉；同条「登记 `PPC-6` 价卡前要求显式部署开关」后续加固**升级**——导入从此能造出 `PPC-6` 的卡（风险点 5），任何持登记写授予的操作者都能经导入、批准、发布走到这一步，代码里仍没有门。建议在首个租户上线前、编辑器上线前把开关做掉。
- 管理台「导入价卡」签目前仍指向 `price-card-import-template-PPT-1.xlsx`（仍有效、仍被接受）。换指向 `PPT-2` 的模板文件归编辑器那一步（`apps/admin-web/src/pages/pricing/api.ts` 的 `priceCardTemplateHref`），本票不动前端。
- 编辑器要用的接口：提交体见 ADR-0349 决定四与规范「结构化工作簿：第二种载体」一节；`card` 表的字段要列全（值可为 `null`），其余表没出现按空表；问题的行号是数组下标加二；草稿查阅读口的 `sourceOrigin` 只在表单草稿上有。
- 不挡上线的新发现（不立票，请记入冻结清单）：已登记价卡反推成模板 / 工作簿的路没有，编辑器要「打开已有价卡改一版」得另立；表单草稿同电子表格草稿一样不存本体，只存名称与散列。

## 修订轮（2026-10-29 作者，应协调方转来的隔离评审意见，评审钉 `8d6b6961e`、阻断 1 条）

**B1（阻断）JSON 载体无结构上限**——已改。解析改成流式、边解边数，到上限立刻停：表数 64、每表行数 100 000（沿用）、每表不同列键数 64、总格数 1 000 000，原始请求体 10 MiB 只在入口判一次；超限整份不收（`FILE_TOO_LARGE`）。行改成「格的切片 + 列下标」、行内同名键用位掩码判，数据行只遍历各行自己的格，代价跟着总格数走。

- 评审的两个探针改写入库（`workbook_json_limits_test.go`）：一行 2 万个不同键加 99 999 个空行，在第 65 个键处即拒，耗时与分配有界；9.6 MB 的 `{"a":1},` 重复体累计分配约 39 MB（4 倍上下，旧写法 859 MB、约 90 倍），测试断言不超过体积的 12 倍。这两条在旧实现上实测为红（临时换回旧文件跑过）。
- 上限以内最大合法形状（十万行乘十键、恰在格数上限）：规范化加读取各约半秒，堆峰值约 100 多兆字节；用例放宽到 400 MiB / 15 s，防量级退化。
- 顺带取证并修：条件树与计算树的「祖先链成环」检查逐节点各走一遍祖先链，节点数的平方，一条 4 万节点的长链要一分多钟（电子表格与结构化工作簿同病，PPT-1 时代就有）。改成每个节点只走一次，报出的节点与话与逐个走一遍完全相同；用例对 5 个节点的全部父节点取法穷举核对朴素写法，并核 4 万节点的长链读取有界。

**非阻断全处理**
- N1：`required()` 去重改成按集合查；问题清单总数上限 1000 条，收满不再收集，清单最前补 `PROBLEMS_TRUNCATED`（新问题码，已入规范的问题码表）。4 万行各缺必填格，电子表格与结构化工作簿都在不到一秒内读完（旧约 12.6 秒）。
- N2：ADR-0349 补风险点 10（JSON 数字是写出来的字面量，不经 15 位有效数字门；编辑器应把金额当字符串发），规范补一段。
- N3：外层 `templateVersion` 在解析时就去首尾空白，补进 `card` 表、挑表集合、报缺表、核对都用同一个值；用例钉住 `" PPT-1 "` 不再多报 `total_bounds`。
- N4：`tables.currency` 的 `allowedValues` 取产品内置币种目录里的码；`conditions.category` **没有放进 `allowedValues`**——它对分区名、偏远档位、服务选项是开放词汇，只在 `source` 为 `ADDRESS_NATURE` 时才限定为 `COMMERCIAL`、`RESIDENTIAL`，放进去编辑器会把所有类别格画成两项下拉。改为新增的可缺字段 `allowedValuesWhen: [{"column":"source","equals":"ADDRESS_NATURE","allowedValues":[…]}]`，列说明里写清对应（**与评审的字面要求有出入，请协调方与 claude-cardeditor 确认**）。ADR 里「译码读封闭取值只走登记处」改成如实措辞：大多经 `closedCode`，表头、进位段、体积重长度单位、条件单位四处经 `weightUnit()` / `lengthUnit()` 直接读同一份切片，币种经产品内置币种目录。
- N5：JSON 整份不收的答复回显最新一版（拒收错误带回读口认的版本，应用层与读口共用 `ports.WorkbookRejectionReading`）；体积上限只在入口按原始请求体判一次，读规范化字节不再判（用例用规范化会展开的字符造出「原始体在上限以内、规范化后超过」）。
- N6：ADR-0349 风险点补 9（JSON 资源上限）、10（数值字面量语义）、11（按 `Content-Type` 分流）。

**Standards**：ADR 与本票里的跨文件序数改成引节名；`workbook()` 注释去掉「十六张」；`ports` 的「上面两个哨兵」改成指名；「拒收转问题码与说明」两处重复合成 `ports.WorkbookRejectionReading` 一份；`TestAFormSourceFileIdentityIsNamedAfterItsOwnDigest` 补 `Covers:`。AT-PP-043 / 046 / 047 的 UC 行与盘点行同步补了新用例。

**对契约形状的影响**（claude-cardeditor 在本分支之上做）：JSON 工作簿形状、预览 / 录入答复形状不变。新增的都是可缺的东西：规格口列上的 `allowedValuesWhen`，`tables.currency` 的 `allowedValues`（原先没有），问题码 `PROBLEMS_TRUNCATED`（只在问题多于 1000 条时出现，清单最前）。超限的 JSON 现在答 `NOT_ACCEPTED` 加 `FILE_TOO_LARGE`。

## 评审记录（2026-10-29，隔离评审）

首评钉 `8d6b6961e`，阻断 1 条（B1：JSON 载体无结构上限，一份 2.7 MB 请求体可让读取耗 29.9 秒）；修订后复评钉 `a9297508e`（代码最后一笔 `ad21b8ee2`），**阻断清零**。复评核过的：

- B1 全部资源探针与各上限的边界（表数 64、每表行数 100 000、每表不同列键数 64、总格数 1 000 000、请求体 10 MiB，恰在上限接受、多一即拒）；原复现探针由 29.9 秒降到 119 微秒，峰值 RSS 由 95 MB 降到 19 MB；上限以内最大合法形状读取约 0.6 至 0.9 秒、RSS 不超过 180 MB。
- 语义不变：PPT-1 的 12 145 个输入受理与否、方案身份、摘要、规范化版本、登记快照零差异；6 000 张随机变异卡的电子表格与两种 JSON 写法零差异；FULL-03 的摘要仍等于种子。
- 祖先链成环检查改线性后，4 000 份随机树的问题清单逐字节一致。
- `PROBLEMS_TRUNCATED` 的边界（恰 1 000 条不截断，超出则清单最前补一条）与 `allowedValuesWhen`、币种 `allowedValues` 的全空间对拍。
- Standards 轴通过：注释全中文、无行号、`internal/architecture` 绿、`gofmt`、`go vet` 干净。

**复评留下的非阻断项（记入冻结清单，不在本票改）**

- N-a：结构化工作簿的规范化字节对含 `<`、`>`、`&` 的字符串输出转义写法（`\u003c` 等），起点那版不转义；只改变含这三个字符内容的来源散列与 `form:` 名，读法与摘要不受影响，新旧两版从未同时上线。
- R1（既有，非本票引入）：领域层 `TriggerCondition` 校验对长嵌套的条件树是平方代价，JSON 十万节点（请求体 5.4 MB）读取约 168 秒；电子表格载体同样可达。要在模板层或领域给条件树、计算树设深度上限。
- R2：空行不计格数，64 张表各 5.4 万个空行的请求体（9.9 MiB）使 RSS 约 224 MB，耗时 0.28 秒，有界；把空行也计入总格数或另设总行数上限可压到几十 MB。

## 进 main 记录（2026-10-29，协调方 tops-20）

复评阻断清零后，把 `2986a040c..a9297508e` 重放到 `origin/main`（`4c4056dc6`），重生成 ADR 索引、AT 盘点计数与机制清点；重放后全量 `scripts/test-full.sh` 退 0（164 个包 ok），种子灌一次性库退 0，用后删库。
