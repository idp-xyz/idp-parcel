# 分区要的起讫邮编没有读口：`parcel-shipment` 非测试代码与迁移里没有任何结构化邮编字段，`DeliveryPlaceReferenceView` 只交收件地点引用不交地址本体——`parcel-pricing` 造快照的「邮编路线」一格指不到；分区解析归 PP（ADR-0109），不归 NR

Category: enhancement
Status: ready-for-agent——**2026-09-14 22:1x 通道 1 按用户「你是业务和系统专家，自决」代裁（PS owner 口径，第 3 条兼 PP owner 口径），四条「要裁的」写入下方「裁决」节**：邮编成为 PS 领域语言里客户原始资料的一格「地址要素」（PS 定要素名的封闭集，条目列表不重构、既有快照缺格如实答缺）；**一口两段**（目的 + 起点各带资料版本锚，锚法照收件地点引用，不新造「寄件地点引用」词）；起点先答**寄件人邮编**，节点邮编另票归 NR / NO；**不过授权查询作用域**。此前 draft——2026-09-14 21:1x 通道 3 立票（sa-cc/11 裁决 4 量「分区指不到」的提供方半边；task-620bc8e7，通道 1 派单）。只写票面未动代码；取证锚 main `db480695`
Blocked by: 无（sa-cc/11 已进 main 2026-09-14 20:4x；本票是它点名的第三只读口；要裁的已裁，见「裁决」）

## 缺口（取证于 `db480695`，逐符号名）

- `git grep -n -i -E 'postal|zip|邮编|address' -- internal/parcelshipment/domain/ internal/parcelshipment/ports/ migrations/parcel_shipment/` → 非测试零命中。测试命中只有 `CONSIGNEE_ADDRESS`（`customer_source_data_test.go` 里 `SourceDataGroupReference` 的合成串——一个资料范围的**名字**，不是字段）与 `ADDRESS_REJECTED` / `ADDRESS_UNSUPPORTED`（面单交易包裹级拒收原因串）。PS 今天在类型与库上都没有「邮编」这一格。
- `git grep -n -E 'Members\s+\[\]canonicalMemberDocument|Scope\s+\[\]canonicalEntryDocument|Service\s+\[\]canonicalEntryDocument' -- internal/parcelshipment/domain/payload_canonicalization.go` → 客户请求规范化文档里成员带重量 / 尺寸，寄收件与服务是 name/value 条目列表（`canonicalEntryDocument{Name, Value}`）；`migrations/parcel_shipment/0003_customer_source_data.sql` 的 `customer_source_data_version` 存范围 + `payload_digest`。邮编若在，只能是某个租户约定的条目名下的一个值。
- `git grep -n -A8 'type DeliveryPlaceReferenceView interface' -- internal/parcelshipment/ports/delivery_place_reference_view.go` → `LoadDeliveryPlaceReference(ctx, 租户, 声明包裹身份)` 答 `domain.DeliveryPlaceResolution` 封闭四格（基线锚引用 / 已采用版本锚引用 / 收件地点未定 / 没有收件地点）。交的是引用；PS `CONTEXT.md`「收件地点引用」写「地址本体留在本上下文，持引用方按引用向本上下文取」——而「向本上下文取」的那只口：`git grep -n -i 'address' -- internal/parcelshipment/ports/` → 零。今天没有任何口按引用交出地址的任何一段。同文件头注自己预告了这只口：「今天没有第二个消费方，**不预设按委托或按引用反查的方法**——按锚解析回地址内容是第二个消费方，有自己的授权作用域问题，届时另立」——本票就是那个「届时」。
- 分区归谁：`git grep -n -E '分区|邮编' -- docs/domain/network-routing/CONTEXT.md` → 唯一命中「服务区域」词条（「带有版本和适用期的国家、行政区域、邮编范围或其他地理覆盖定义，用于把客户地址解析为候选收寄节点、交付节点或尾程注入节点」）；「分区」零命中。`git grep -n -c '分区' -- docs/domain/parcel-pricing/CONTEXT.md` → 9，含「计价参考目录」词条：「承运商的分区表（目的邮编 → 分区）与偏远档位表（目的邮编 → 档位）是它的两个实例……本上下文拥有目录的登记、版本化与发布治理，不生产其内容……绑定了目录的方案，评价从目录解析分区与档位，查不到即评价待判断，不给默认分区或默认档位；未绑定的方案保留由调用方给出分区的路径」。ADR-0109 Decision 四同句。**分区是 PP 从目录解析的，不是 NR 算的**；NR 拥有的「服务区域」是路由解析节点用的另一件东西。
- PP 侧要什么：`git grep -n -B2 'type PostalRoute struct' -- internal/parcelpricing/domain/reference_catalogue.go` → `origin` / `destination`，头注「目的邮编必备，始发邮编随目录的始发维度需要而给。它与调用方直接给的分区是两条路径（ADR-0109 Decision 四）」；`NewPostalPricingInputSnapshot` / `WithPostalRoute` 收它。`git grep -n -A4 'var missingInputReadPorts' -- internal/parcelpricing/application/form_evaluation_from_request.go` → 第三条点名 `parcel-shipment: no read port for the origin / destination postal route (zone)`。

## 语言从哪里来

- PS `CONTEXT.md`「收件地点引用」：「由租户、委托、收件资料范围与资料版本锚四段组成；锚是接受基线（该范围上尚无修订版本）或当前采用的那份客户原始资料版本……它不是地址文本、不是目的地节点，也不另立地点身份——收件地址的版本就是客户原始资料版本，引用只指向它；引用一经交出含义不变，旧锚永远解析到那一版内容」；_Avoid in this context_：目的地节点、派送地址、地点 ID。
- PS `CONTEXT.md`「客户原始资料」「客户原始资料版本」「当前客户资料版本采用判断」；规则「收件地点引用按（租户，包裹身份）答，委托与接受基线只是本上下文内部走到答案的路……`待复核`不给引用而答收件地点未定」。
- NR `CONTEXT.md` 规则：「客户地址继续由 `parcel-shipment` 保存。`network-routing` 只保存服务区域、节点覆盖版本和当次解析依据，不把客户地址建立成物流节点」。
- PP `CONTEXT.md`「计价参考目录」；ADR-0109 Decision 二（目录声明「始发维度（始发邮编前缀集或始发分区）」、「邮编前缀匹配的粒度……随首份真实分区表定，本记录不预拟」）与 Decision 四。
- GLOSSARY「收件地点引用」（「地址本体留在 `parcel-shipment`，持引用方按引用向所有者取」）、「服务区域」（所有者 NR；「客户地址仍由 `parcel-shipment` 拥有」）。
- `docs/domain/CONTEXT-MAP.md` `settlement-accounting → parcel-pricing` 那条边：「……小包托运的邮编路线……那一侧立」。

## 做法候选（两条以内，不选）

1. **按收件地点引用取邮编**：PS 新立一只只读口，收 `DeliveryPlaceReference`（PP 消费侧先经 [01](01-tf-charge-occurrence-member-object-read-view.md) 拿到成员包裹身份 → `DeliveryPlaceReferenceView` 拿引用 → 本口按引用取），答目的邮编（+ 国家 / 地区码）；引用自带资料版本锚，与 CONTEXT「旧锚永远解析到那一版内容」一致，PP 快照 `factReferences` 直接记这个引用。起点邮编（寄件资料范围）今天没有对应的引用词条，要么本口第二段、要么另长「寄件地点引用」（要裁的 2）。前提是邮编在客户原始资料里成为一格（要裁的 1）。
2. **按（租户，包裹身份）直接答邮编路线**：一只口答（起点邮编，目的邮编，两段各自的资料版本锚），内部走 `DeliveryPlaceReferenceView` 同一条包裹 → 委托 → 资料范围的路；PP 少一跳，但 PP 拿到的是邮编不是「地点」引用——与 CONTEXT「落『地点』一格只用它」的纪律要对齐：邮编不是地点，是资料的一格，能不能单独交出归 PS owner。

## 红线

- PS 不算分区、不解析服务区域、不查目录；地址本体不越出 PS——只交邮编这一格与版本锚，不交地址文本（PS `CONTEXT.md` 规则「合成日志和证据索引只保留作用域引用、对象引用……不记录客户原文」）。
- 不改 `DeliveryPlaceReferenceView` 的四格答法与 `DeliveryPlaceReference` 四段形（TF 派送任务在用）。
- PP 不 import `internal/parcelshipment/application`；邮编 → `PostalRoute` 的翻译在 PP 消费侧（ADR-0025）。
- 邮编格式校验、前缀粒度属实例半边（ADR-0109「随首份真实分区表定」）；PS 只存客户给的串，不预拟校验规则、不规范化。
- 分区归属不重开：PP 从目录解析、NR 不算分区，两处权威文档已定。

## 完成判据（待裁后写实；可 grep）

1. `git grep -n -E 'type \w+ interface' -- internal/parcelshipment/ports/` 多出邮编只读口（名作者定），方法集不含 `Save`；答案带资料版本锚（裁决 2 取锚法时）。
2. 若裁决 1 取结构化：PS 领域客户原始资料长出邮编一格（寄件 / 收件两范围），`migrations/parcel_shipment/` 新序号（`0003` 不改），既有快照读回兼容、缺席如实。
3. 真库用例：有邮编答之；无邮编 / 资料范围`待复核` / 对象不属任何已接受委托，各按 `DeliveryPlaceResolution` 同族的封闭答法如实答缺，不造默认邮编。
4. `internal/architecture` 门禁绿；`internal/parcelpricing/**`、`internal/networkrouting/**` 零 diff；`docs/product/MECHANISM-INVENTORY.md` 重生成 PS 端口 +1。

## 地盘

`internal/parcelshipment/{domain,ports,adapters/postgres}`、`migrations/parcel_shipment/`（新序号）；`docs/domain/parcel-shipment/CONTEXT.md` 若裁决 1 让邮编成词（先改 CONTEXT 再改代码，AGENTS「改文档」）。不动 `internal/parcelpricing/**`、`internal/networkrouting/**`、`internal/transportfulfillment/**`。

## 要裁的

1. **邮编在客户原始资料里是不是一格**——归 PS owner。今天寄收件资料是 name/value 条目列表，PS 领域语言里没有「邮编」一词；让它成为结构化字段（客户原始资料版本可修订的一格，进 `PayloadDigest`）是 PS 领域语言题。若不结构化，读口只能按某个条目名取值，那个条目名是租户资料 schema 的一部分（实例半边），写进代码就是写死；若结构化，接单入口与既有快照都要动。裁前 `payload_canonicalization.go` 一字不动。
2. **读哪一版、起点长不长「寄件地点引用」**——归 PS owner。目的邮编读接受基线那份、当前采用的客户原始资料版本、还是随调用方交的锚——`收件地点引用` 已给一种锚法（基线锚 / 已采用版本锚 / 待复核不给），邮编读口照它还是另定；与 [02](02-billable-weight-actual-measurement-and-declared-dimensions-read-port.md)「要裁的」3 同一张纸。起点（寄件资料范围）今天没有引用词条，是长出「寄件地点引用」与收件同形，还是邮编口一次答两段。
3. **起点邮编是寄件人地址，还是收寄 / 注入节点**——归 PS owner 与 PP owner 共。ADR-0109 的「始发维度」是承运商分区表的始发，承运商通常按注入点 / 收寄节点分始发区，不按客户寄件人邮编。若是节点，起点的提供方不是 PS——物流节点身份归 NR、节点收寄归 NO、发生项主要业务范围 / 旅程归 TF——本票只立 PS 这半（客户地址的邮编），节点邮编另立票（归 NR / NO owner）。裁的是 PP 消费侧造 `PostalRoute.origin` 该去问谁；PS 只答自己有的。
4. **跨上下文读客户资料的一格要不要过授权查询作用域**——归 PS owner。`DeliveryPlaceReferenceView` 头注把「按锚解析回地址内容」记为「有自己的授权作用域问题」的第二个消费方；PS `CONTEXT.md` 规则「查询只消费共享身份/授权能力传入的授权查询作用域」说的是对外查询面。PP 消费侧适配器是进程内的另一个上下文、不是客户端——邮编读口按（租户，引用）答还是也要一枚作用域，以及答缺时用不用「统一不可见结果」那一格，是 PS 的边界题。TF 拿收件地点引用时没过作用域（同头注「按（租户，包裹身份）问，不要求消费方持有委托、接受基线或提交版本」），邮编这一格与引用是不是同一待遇，PS owner 裁。

## 裁决（2026-09-14 22:1x 通道 1 代裁，PS owner 口径，第 3 条兼 PP owner 口径；依据是本票取证，钉 `db480695`）

1. **邮编在客户原始资料里是不是一格——是，作为 PS 领域语言的「地址要素」。** 先改 `docs/domain/parcel-shipment/CONTEXT.md`（AGENTS「改文档」第一条）：客户原始资料词条下长出「**地址要素**」——寄件与收件两个资料范围内，由本上下文定名的一组封闭要素（首批两个：**国家 / 地区码**、**邮编**；要素名字面由作者按既有 name/value 条目命名法定，例如 `POSTAL_CODE` / `COUNTRY_CODE`），它们仍是 name/value 条目列表里的条目、**不重构**规范化文档的形、不改 `PayloadDigest` 对既有载荷的算法（既有快照缺这两个要素即「未提供」，如实答缺）；租户接单资料里的字段怎么映到这组要素名是**实例半边**（接入侧适配器的题，今天无租户，夹具 `SYN-`）。**不选**「按租户约定的条目名取值」——那是把租户 schema 写死进代码；**不选**「结构化字段进快照」——要动接单入口与全部既有快照，为一格值改整个形不值，而要素名封闭集已足够让读口按名取值。
2. **读哪一版、起点长不长「寄件地点引用」——一口两段，锚法照收件地点引用，不新造词。** 一只只读口按（租户，正式包裹身份）答「目的邮编 + 国家 / 地区码 + 资料版本锚」与「起点邮编 + 国家 / 地区码 + 资料版本锚」两段，各段独立成格（一段有一段无是常态）；锚法照「收件地点引用」：接受基线 → 基线锚、当前采用版本 → 已采用版本锚、`待复核` → 该段答「未定」、无资料范围 → 该段答「无」。**不**新造「寄件地点引用」——地点引用是给派送任务持有的复合引用（ADR-0130），邮编是资料的一格；两者一个是「地点」一个是「值」，PS CONTEXT「落『地点』一格只用它」说的是前者，本口不落「地点」。与 [02](02-billable-weight-actual-measurement-and-declared-dimensions-read-port.md) 裁决 3 同一套锚法与封闭答格（作者可抽一个内部共用步骤「按包裹身份解析寄 / 收件资料范围的版本锚」，对外仍一口一问——`CommercialResolutionReferenceView` 头注「一口一问」的先例）。
3. **起点是寄件人地址还是节点——PS 只答自己有的（寄件人邮编）；节点邮编另票。** ADR-0109「始发维度（始发邮编前缀集或始发分区）」是承运商分区表的始发，承运商多按注入 / 收寄节点分始发区——那一格的提供方是 NR（物流节点身份）/ NO（节点收寄），不是 PS。**PP owner 口径**：`PostalRoute.origin` 从哪来由**绑定目录声明的始发维度**决定（ADR-0109 决定二「随首份真实分区表定」），PP 消费侧按目录声明去问 PS（寄件人）或问节点侧；两条路今天都留、都不预选。节点侧那只口在 spec「不在本目录」补一行「起点为节点邮编时的读口，归 NR / NO，等首份真实分区表声明始发维度再立」。
4. **要不要过授权查询作用域——不过，与 `DeliveryPlaceReferenceView` 同一待遇。** PP 消费侧适配器是进程内另一个限界上下文、不是客户端；PS CONTEXT「查询只消费……授权查询作用域」说的是对外查询面；`DeliveryPlaceReferenceView` 头注「按（租户，包裹身份）问，不要求消费方持有委托、接受基线或提交版本」是同一类消费方的既有先例。红线不变：只交邮编与国家 / 地区码两格值 + 版本锚，**不交地址文本**、不交其他要素。`DeliveryPlaceReferenceView` 头注「按锚解析回地址内容是第二个消费方……届时另立」——本口就是那个「届时」，头注那句改口指向本口。
5. **完成判据写实**：判据 1 照做（口名作者定，方法集不含 `Save`，两段各带锚）；判据 2 改为「PS CONTEXT 客户原始资料词条长出『地址要素』（两要素名封闭集）；`payload_canonicalization.go` 只加按要素名读值，不改文档形、不加列、不加迁移（若作者量到必须落列才能按锚读，写进判断项、迁移序号重取，`0003` 不改）；既有快照缺要素如实答缺」；判据 3 照做，加「`待复核` 答未定、非委托对象答无、要素缺席答缺」四格；判据 4 照做（PS 端口 +1）。
6. **能力边界**：裁的是要素成词、一口两段、起点归属、作用域待遇；`snapshot` 里 name/value 条目能否按资料范围版本对应上锚、`DeliveryPlaceResolution` 四格能否直接复用为答格，归作者。读过本票全文、spec、01 / 02；**没读** `payload_canonicalization.go` / `delivery_place_reference_view.go` / `0003` 正文（经取证引文）、ADR-0130 / 0133 正文。作者量到与代码不符，以代码为准并写进判断项。

**不是要裁的、记在此防重开**：「分区是 PP 算还是 NR 算」——PP `CONTEXT.md`「计价参考目录」与 ADR-0109 Decision 四已定 PP 从绑定的目录解析，NR `CONTEXT.md` 无「分区」一词、「服务区域」是路由解析节点用的；派单让核 NR 分区归属，核过，无需裁。

## 参照

sa-cc/11 [`11-pp-inbox-consumer-receives-evaluation-request-envelope.md`](../../sa-cc-funds-and-credential-seams/issues/11-pp-inbox-consumer-receives-evaluation-request-envelope.md) 裁决 4、完成记录「逐条对裁决」4；[spec](../spec.md)「取证」「不在本目录」；`internal/parcelpricing/domain/reference_catalogue.go`（`PostalRoute`、`ErrReferenceCatalogueMismatch` 头注）、`input.go`（`NewPostalPricingInputSnapshot` / `WithPostalRoute` 头注「给到一张没绑目录的卡，评价落待判断——没有任何一方能产出分区」）；`internal/parcelpricing/application/form_evaluation_from_request.go`（`missingInputReadPorts`）；`internal/parcelshipment/ports/delivery_place_reference_view.go`（`DeliveryPlaceReferenceView`——按引用交、地址本体不出的先例；头注「按锚解析回地址内容是第二个消费方……届时另立」与「不要求消费方持有委托、接受基线或提交版本」两句）；`internal/parcelshipment/ports/commercial_resolution_reference_view.go`（头注「与 DeliveryPlaceReferenceView 分开、一口一问」——第二只窄读口另立而不拓宽的先例）；`internal/parcelshipment/domain/delivery_place_reference.go`（`DeliveryPlaceReference` 四段）、`delivery_place_resolution.go`（封闭四格）、`payload_canonicalization.go`（name/value 条目）；`migrations/parcel_shipment/0003_customer_source_data.sql`；PS `CONTEXT.md`「收件地点引用」「客户原始资料版本」「当前客户资料版本采用判断」；NR `CONTEXT.md`「服务区域」与「客户地址继续由 `parcel-shipment` 保存」那条规则；PP `CONTEXT.md`「计价参考目录」；ADR-0109 Decision 二 / 四；ADR-0130 决定二（收件地点引用是委托级复合引用、锚住资料版本；窄读口另立不拓宽写口）；ADR-0133 决定二（商业解析回指窄读口——「一口一问」的第二例）；ADR-0077 决定五（伴生读端口另立）；ADR-0025。

## Comments

- 2026-09-14 21:1x · 通道 3（task-620bc8e7）：立票，未动代码。**与派单不符两处**：① 派单写「PP 无读口」，实测 PS 非测试代码与迁移里没有任何结构化邮编字段——缺的不只是一只口，是口后面那一格；`DeliveryPlaceReferenceView` 交引用、CONTEXT 说「持引用方按引用向本上下文取」，而取的那只口今天也不存在。② 派单让「查 NR CONTEXT 分区归属再写」——查了，NR 无分区一词，分区归 PP（PP CONTEXT + ADR-0109），故本票不把它列为要裁的，只记防重开；要裁的全落在 PS 侧「邮编从哪来、哪一版、起点是谁的」。能力边界：核过 PS domain / ports / 迁移零邮编、`DeliveryPlaceReferenceView` 签名与四格、规范化文档三段形、NR / PP CONTEXT 分区归属、`PostalRoute` 两格；**没读** PS `adapters/postgres` 里客户原始资料版本按范围读回的路径、接单入口（`submit_shipment_request.go`）今天收不收邮编——「要裁的」1 的代价那半靠 PS owner 与作者开工时量。
