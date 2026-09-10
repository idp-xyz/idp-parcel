# 2026-09-05 以来 resolved 票面里的余工清单（只读扫描）

Category: chore
Status: done——只读扫描，一次交全表；不改任何票面、不动代码、不提方案。
取证锚：`c7e3522c`（远端 main，2026-09-10 14:29）。通道 5，2026-09-10 14:4x–15:1x，单 task-a5b220d1。
对照的前半张表：[unresolved-review-20260904/remaining-work-a3a4814.md](../unresolved-review-20260904/remaining-work-a3a4814.md)（通道 4 重核 09-04 那份余工清单，task-f985c4d0）；本文接它的下半——09-05 以来新收口的票。

## 范围怎么算的

- 文件集：`git log --since=2026-09-05 --name-only -- .scratch` 取路径含 `/issues/` 的 61 份，再按 `Status:` 首词筛 `resolved`，得 **57 份**（`.scratch/*/issues` 作 pathspec 时 git 的通配不进子目录，要用 `-- .scratch` 再过滤 `/issues/`；本文所有 `git grep` 命令同此）。
- 排掉的 4 份非 resolved：`auto-reroute-demo-reachability/02`（blocked）、`first-tenant-runway/03`（blocked）、`ps-port-remainder/01`、`ps-port-remainder/03`（ready-for-agent，14:2x 已派 6 / 2）。
- 收口日期早于 09-05 但文件 09-05 后有改动的 1 份：`label-channel-service-first-release/24`（Status 行「MCP-5（2026-09-04……）」，09-09 只补了 owner 复核记录）——属通道 4 那半张表，本文不重列，只记「ADR-0117 三条 owner 复核 2026-09-09 认可（票内）」。
- 除外：`label-channel-service-first-release/11` 与 `/12` 的接线余工——**归通道 3 立票**（25–28，本文一行不写）。
- 四类句子的抓法：把每份票按「。；换行」切句，按四类词表匹配后逐句人工核（词表见文末「重放」）。抓到 790 句，去掉派单流水、接手记、验证记数等非余工句后余下表里这些。
- 「已复核 / 待复核」的底：`scripts/owner-review-queue.ps1 -Out .scratch/owner-review-queue.md`（钉 `c7e3522c`，14:42；该文件在库里有旧版，本笔不带它的重写，重放方在自己检出上再跑一遍即得同一份）。它算出 15 篇 ADR 含越权风险点，其中带 `owner 复核 YYYY-MM-DD 认可` 的 9 篇（0113 / 0114 / 0116 / 0119 / 0120 / 0123 / 0124 / 0125 / 0126），不带的 6 篇（0128 / 0129 / 0130 / 0131 / 0132 / 0133）；它漏的一类是**越权风险点只写在票面、ADR 正文不含那四个字**的（0112 / 0117 / 0118 / 0122 / 0127 与票 psr/05、tf/11、wbr/03），本文逐票补核，结论见表。
- 每条的机制 / 实例按 AGENTS.md 那条判据：改代码形状、端口、票面、注释的归**机制**；等租户登记、等真实参数、等真渠道的归**实例**。

## 缩写

`awf` = `admin-write-faces`，`pcg` = `party-commercial-context-gaps`，`psr` = `ps-port-remainder`，`sa` = `sa-preacceptance-policy-view`，`tf` = `tf-segment-lifecycle-closure`，`wbr` = `wiring-baseline-remainder`，`nr` = `nr-route-evidence-views`，`ftr` = `first-tenant-runway`，`fti` = `frontline-transition-import`，`adle` = `agent-docs-local-env`，`arrf` = `acceptance-review-read-face`，`pgt` = `pgtest-template-database`，`cmdr` = `tf-carrier-master-document-register`。票号即 `.scratch/<目录>/issues/<票号>-*.md`。

判定用词封闭：`有票 <路径>`（后来有票承接，附其 Status）/ `无票` / `已复核` / `待复核` / `已被否 <依据>` / `已落 <依据>`（句子说要另做的事后来在别处做掉了）/ `痕迹可剪` / `条件未到`（句子自带「若将来 / 那天」前置，今天不成立）。

## 表

| 来源票 | 原句引文（不用行号） | 类别 | 判定 | 机制 / 实例 |
|---|---|---|---|---|
| arrf/01 | 「本票 2026-09-08 由通道 2 按用户指示做簿记收口」（原票由 admin-skeleton-closure-batch/09 兑现） | — | 无余工句；纯簿记收口 | — |
| awf/06 | 「另立 party-commercial-context-gaps/07」 | ① | 有票 `pcg/07`（resolved，ADR-0115） | 机制 |
| awf/06 | 「顺带量到、不在本票：管理台 `CommercialPolicyKind` 没有 `CUSTOMER_SERVICE_RULE` 那格」；完成记录末尾「要不要先立读面票交 MCP-1 定」 | ①③ | 有票 `awf/21`（resolved）；裁决已落（通道 1 09-09 代裁立票） | 机制 |
| awf/07 | 收口条「未做：不重开逐册『选形与理由』的复核」 | ① | 伞票收口**没有**处理子票推给它的判断题（见 awf/09 / 11 / 12 / 18 / 22 各行），那些句子因此全部落为无票 | 机制 |
| awf/08 | 「ADR-0126 的越权风险点四条仍待 owner」 | ② | 已复核（ADR-0126「owner 复核 2026-09-09 认可」，票内同刻一条） | 机制 |
| awf/08 | 「迁移头注『就是……的字节』与此不符，日后谁在 SQL 侧直接哈希列会对不上——改注释或改 `bytea`」 | ④ | 痕迹仍在：`migrations/party_commercial/0028_*.sql` 头注仍写「正文快照（content_document）就是服务端规范化文档（PCC-<n>）的字节」；无票 | 机制 |
| awf/09 | 「声明 `PCC-1:<常量>`，属『何时开始拒收无版本旧串』那一问（Decision 五归伞票收口时裁），不在本票自裁」 | ①③ | 无票；伞票 07 收口未裁（见 awf/07 行） | 机制 |
| awf/09 | 「`IsRegisterCanonicalized` 头注对无正文册失真，一句注释」（评审 Standards 非阻断） | ④ | 头注现为「答某一册今天接没接进服务端规范化。对账门（ADR-0126 Decision 二）用它分辨……」，评审点的那半句已不在——已落；不确定是哪一笔改的，列「未核」 | 机制 |
| awf/10 | 「未做（各归其票）：`cmd/parcel-commercial/translate.go` 的 `controlRequirementFrom` / `controlBindingFrom` 与领域新加的 `PreAcceptanceControlRequirementNamed` / `financialControlBindingOf` 是同一条规则的两份……合并归 CLI 侧收口」 | ① | 无票；四个函数在 `c7e3522c` 上仍各自在（`translate.go` 两只、`domain/pre_acceptance_control.go` 与 `publication_canonicalization_customer_contract.go` 两只）；awf/24 统一的是 `*Named` 一族，未碰 CLI 侧 | 机制 |
| awf/11 | 「判断类三条留给伞票收口、不在本票动：(1) `canonicalSupplierAgreementBody.body()` 与信用政策那一节的时刻解析同形，可抽成领域内一个区间折回助手……」 | ① | 无票；伞票 07 收口未裁 | 机制 |
| awf/11 | 「建议另立票：镜像后端各键、目录加正文列」 | ① | 有票 `awf/19`（resolved，进 main `2026-09-08 17:0x`） | 机制 |
| awf/12 | 「`RowFrame` 同形复制——放行、另立票（第 2 波落齐后抽到 `PublicationDraftFlow` 旁）」 | ① | 有票 `awf/22`（resolved，进 main `4209520b`）；`RowFrame` 今天只在 `PublicationFormFields.tsx` 一处导出 | 机制 |
| awf/12 | 「票面文字留给伞票 07 收口时一并改，本票不再动」 | ① | 无票；伞票 07 收口未改 | 机制 |
| awf/13 | 「`Field` / `useLoaded` / `vocabularyPlaceholder` 与整数解析……宜抬到 party 共享层……另立票」 | ① | 有票 `awf/22`（resolved） | 机制 |
| awf/13 | 「`publish_commercial_authority.go` `publicationContentOf` 与 `publication_draft.go` `declarationsOfContent` 头注『今天只有信用政策一格』已不实」 | ④ | 已落：`git grep '只有信用政策' -- internal cmd apps` 零命中 | 机制 |
| awf/13 / 15 / 11 | 「`PreAcceptanceFinancialControlPolicyPublicationForm.tsx` 头注『十册没有一类是矩阵』——数的是 `CommercialObjectKind` 封闭集（别处的东西）」 | ④ | 痕迹可剪：`git grep '没有一类是矩阵' -- apps` 命中 `PreAcceptanceFinancialControlPolicyPublicationForm.tsx` / `SettlementPolicyPublicationForm.tsx` / `SupplierAgreementPublicationForm.tsx` 三处；无票 | 机制 |
| awf/15 | 「同文件头注『决定一那三条的直接读数』数了 ADR-0101 里的条目（AGENTS「不用计数」）」 | ④ | 痕迹可剪：`git grep '直接读数' -- apps` 命中 party 四张表单 + pricing 两处；无票 | 机制 |
| awf/15 | 「票面写『币种……存在性由构造门答』、表单占位写『存不存在由服务端答』，但今天 `NewCurrencyCode`（`settlement_policy.go`）……」（评审 Spec 非阻断 1） | ④ | 未核（币种构造门今天答什么未复查） | 机制 |
| awf/15 | 「判断题 (1) 同意另立票」（`Field` / `ReferencePicker` 抬共享层）；评审两条判断题 | ① | 有票 `awf/22`（resolved）、`awf/23`（resolved，进 main `f8fa0398`） | 机制 |
| awf/14 | 「`validatePricePolicyBody` 供两处共用，另立票」 | ① | 未核：`git grep validatePricePolicyBody -- apps` 零命中，函数已改名或已抬走，未追到哪一笔 | 机制 |
| awf/14 / 21 | 「`party/policy-rows.test.ts` `pricePolicies` 夹具头注『照后端 query_commercial_catalogue_test.go 里价格政策册**那三行**的形状』」；「夹具头注『钉住的三行』是跨文件计数」 | ④ | 痕迹可剪：`git grep -n -e '那三行' -e '钉住的三行' -- apps` 命中 `policy-rows.test.ts` 两处；无票 | 机制 |
| awf/16 | 「`unconfiguredNote` 错指 `PAR-INT-01`，与同笔 api 头注相反，一行文案」 | ④ | 已落：Status 行记追笔 `mcp5-awf16-followup@9dacdbad→abdc5ef9` | 机制 |
| awf/17 | 「授权授予册（`authority_grant.go` 的 `AuthorizedAction` 一族）不经这条发布路，不在本票」 | ① | 无票：`git grep -e 授权授予册 -e authority_grant -- .scratch` 命中的六份票里没有一张是授权授予册的管理台写面；是否需要未裁 | 机制 |
| awf/18 | 「本册正文归谁未裁（pc-gaps/05），本票未开口」 | ③ | 已落：`pcg/05` Status resolved（2026-09-04，ADR-0104），归属已裁 | 机制 |
| awf/18 | 「评审两轴五条非阻断随票记，不另立票：Spec ① 判据口径归伞票 07 收口」 | ① | 无票；伞票 07 收口未处理 | 机制 |
| awf/19 | 「前端没跟上：`SupplierAgreementRecord` 止于 `publishedAt`，头注写着……」 | ④ | 已落（本票自身，`d6fdeacc`） | 机制 |
| awf/20 | 「（放行 → 500 / 不放 → 403）加第三桶（放行且不经读口 → 200）并改 ADR-0078 判据措辞，另立票」 | ① | 无票：`git grep 第三桶 -- .scratch docs` 只命中本票 | 机制 |
| awf/20 | 「判断点（归 owner 复核）：词表读口在隔离读态不放行」 | ② | 已复核（票内「owner 复核 2026-09-09 认可」） | 机制 |
| awf/20 | 「顺带一格（不在本票范围，记下免得丢）：客户合同正文的 `preAcceptanceControl.requirement` 在领域里也是封闭集……票 10 表单今天怎么供这一格，由该票或后续票自决」 | ① | 无票：`git grep 'preAcceptanceControl\.requirement' -- .scratch` 只命中本票 | 机制 |
| awf/21 | 「`api.ts` `CustomerServiceRuleRecord` 头注逐字列 `ClaimDeadlineKind` 三值，今天对，建议改符号名」 | ④ | 未核（`api.ts` 里 `ClaimDeadlineKind` 字面今天零命中，可能已改，未追笔） | 机制 |
| awf/22 | 「(2) 收集器替手抄声明与 (3) `ObjectPicker` 参数化可抬两条各可另立票，归 admin-write-faces 伞票 07 收口时一并裁」 | ① | 无票；伞票 07 收口未裁 | 机制 |
| awf/22 | 「`PublicationFormFields.tsx` / `publication-form-shared.ts` 头注『九份』『五张票』属 AGENTS「不用计数」字面（数已冻结、风险零，建议改措辞或锚 `3d90130c`）」 | ④ | 痕迹可剪：`git grep -e 九份 -e 五张票 -- apps/admin-web/src/pages/party` 两处仍在；无票 | 机制 |
| awf/23 | 「`reconcileShellReference` 的空白口径应与 `requireField` 一致用 `strings.TrimSpace`——纯空格的壳引用今天会被报两次，是头注自己说要避的第二处」（推送方处置评审非阻断） | ④ | 痕迹仍在：`publication_draft_payload_settlement_policy.go` 的 `reconcileShellReference` 仍按 `declared == ""` 判空；无票 | 机制 |
| awf/24 | 「未做（不在本票）：`CommercialObjectKindNamed` 的一行委托（通道 3 awf/18 在途地盘）」 | ① | 已落：`publication_canonicalization.go` 的 `CommercialObjectKindNamed` 今天就是 `return closedCodeNamed(...)` 一行 | 机制 |
| awf/24 | 「`pricing_caliber.go` 新加的 `TaxDisposition.valid` 与 `NewTaxCaliber` 的 default 分支是同一道边界判写在两处，作者『未做』已留待，不另立票」 | ①④ | 无票（评审明说不另立）；未核两处今天是否仍双写 | 机制 |
| adle/02 | 「判据 2 的第三个案例实测与票面预期相反且有证据，记在完成记录里等 owner 定」「头注照实写，票面预期不改，归 owner 定」 | ③ | 待 owner 定（`salvage/mcp4-tf03` 按新判据是 ABSORBED，票面预期 NOT-ABSORBED） | 机制 |
| adle/02 | 「九支 `merged/` NOT-ABSORBED 要不要追：推送方留待，不在本票」 | ① | 无票 | 机制 |
| ftr/10 | 「顺带量到一格不在本票的缺口：前一版本若已形成 `HELD`，新版本形成时它怎么处置（新版本重控是否再冻一次、旧冻结谁释放）……归受控补充编排或 SA 的下一票，本票不碰」 | ① | 无票：`git grep 旧冻结 -- .scratch` 只命中本票 | 机制 |
| fti/01 | 「`RECEIVED` 行的身份核对缝不在本票地盘（PS 侧 `ps-external-mark-relations/01`）」 | ① | 有票 `ps-external-mark-relations/01`（**needs-info**） | 机制 |
| fti/01 | 「（映射表过期）照 MCP-6 原记，不在本票地盘、不随本票收口立票——要不要立归 NO owner 与映射表所有者」 | ①③ | 无票；待裁（NO owner） | 机制 |
| fti/01 | 「`buildIntakeImporter` 的 `identity` 参数那一格……届时是一张新票，不重开本票」 | ④ | 条件未到（等 PS 身份视图解阻） | 机制 |
| nr/03 | 「替代路是给复核表加一列 `new_plan_version` 再迁移……若评审判该加列，另立票」 | ① | 已被否（评审 12:3x 未要求加列） | 机制 |
| nr/03 | 「可随票 13 或另起小笔补一例，不挡合入」 | ① | 未核（tf/13 完成记录未逐句对这一例） | 机制 |
| pcg/07 | 「SA 读路径的改动不在本票」 | ① | 有票 `sa/02`（resolved，ADR-0122） | 机制 |
| pcg/08 | 「PS 两只适配器与 `cmd/parcel-api` 两处装配仍走旧构造器（三步法 expand 已做、migrate / contract 未做），归 ps-port-remainder/03 或一张 PS 侧票」 | ① | 有票 `psr/03`（ready-for-agent，14:2x 派通道 2） | 机制 |
| pcg/08 | 「越权风险点三条……单列在 ADR-0116，等 owner 复核」 | ② | 已复核（ADR-0116 2026-09-09） | 机制 |
| pcg/09 | 「不做的：PS 适配器 `label_validity_rule.go`」 | ① | 有票 `psr/01`（ready-for-agent，14:2x 派通道 6） | 机制 |
| pcg/09 | 「越权风险点三条……单列在 ADR-0119，等 owner 复核」 | ② | 已复核（ADR-0119 2026-09-09） | 机制 |
| pcg/10 | 「不做的：PS 消费适配器与跨侧封闭集比对测试（ps-port-remainder/02 余下一段）」 | ① | 有票 `psr/02`（resolved，进 main 2026-09-08） | 机制 |
| pcg/10 | 「越权风险点四条单列在 ADR-0120」 | ② | 已复核（ADR-0120 2026-09-09） | 机制 |
| pcg/11 | 「只答有没有，不交内容——内容读口（一线作业端按引用取允许集与证据规则）是第二个消费方，UC-TF-006 步骤 5 的实施票另立」 | ① | 无票：`git grep -e 内容读口 -e 'UC-TF-006 步骤 5' -- .scratch` 只命中本票、tf/14 与 admin-web-page-wiring-frontier/07 | 机制 |
| pcg/11 | 「管理台表单（admin-write-faces）不在本票，另立」 | ① | 无票：`git grep DELIVERY_CONDITION -- apps/admin-web/src` 零命中，`git grep 交付条件 -- .scratch` 无 awf 票 | 机制 |
| pcg/11 | 「归属若 owner 复核后改（ADR-0133 越权风险点 2），走 supersede」；「ADR-0133 越权风险点 2（归属）与 3（闭包不在场答 error 而非未配置）」 | ② | 待复核（ADR-0133 无 owner 复核记录） | 机制 |
| pcg/11 | 「作者理由成立……归 owner 复核——若认可，ADR-0031 补一句『输入对着册上另一份立不住时 Save 返回 error，与构造门拒件同落点』或立 supersede」 | ② | 待复核：ADR-0031 正文无此句、无 owner 复核记录 | 机制 |
| pcg/11 | 「ADR 只为『未采用合同』立了 error 哨兵、没为『未采用产品』立……归 owner 复核（若要分格，`DeliveryConditionReferenceFor` 加一哨兵）」 | ② | 待复核 | 机制 |
| pcg/11 | 「日后闭包采用的产品版本 ≠ 合同层指名的那一版时……内容读口届时要报『版本错配』——归第二个消费方那张票」 | ④ | 条件未到；归上面「内容读口」那张无票 | 机制 |
| pgt/01 | 「放不放 `-p 1` 以 `ci.yml` 带 run 号的实测为据，另立票」 | ① | 无票：`git grep -F '`-p 1`' -- .scratch` 命中的 issues 里没有以它为题的票 | 机制 |
| pgt/01 | 「评审两条非阻断尾巴随票记，另派小票：① `ALTER DATABASE … ALLOW_CONNECTIONS false`……② 票面判据『进程退出模板库删掉』改口」 | ① | ② 已落（本票「收尾」）；① 无票：`git grep -F ALLOW_CONNECTIONS -- .scratch` 只命中本票与 tasks.md | 机制 |
| psr/02 | 「分格，留待下一次碰这两个文件时顺手，不另立票」 | ①④ | 无票（作者明说不另立） | 机制 |
| psr/04 | 「一格诚实的缝（不在本票改）：版本与意图不同事务……要做到『版本与意图同一事务』得让编排不再自己调 `Downstream`……归 PS owner 另裁」 | ①③ | 无票；待裁（PS owner） | 机制 |
| psr/05 | 「越权风险点两条，单列：(1) CC『已关闭』……(2) NO『从未关联 → 不在』」 | ② | 已复核（票内「owner 复核 2026-09-09 认可，两条逐条」；ADR-0118 正文不含四字，脚本漏此票） | 机制 |
| psr/05 | 「TF 交接入站建立不经收寄判断的关联那天补一路，届时随那张票」 | ④ | 条件未到 | 机制 |
| psr/06 | 「不做：按引用解析回地址内容的读口（第二个消费方，另立）」 | ① | 无票：`git grep -e 解析回地址 -e 地址内容 -- .scratch` 只命中本票与 tf/12 | 机制 |
| psr/06 | 「ADR-0130 越权风险点 2 / 4 未变；本票新增一条——`Undetermined` 在 TF 端口上的落法……是否加第三格归 TF owner（ADR-0130 越权风险点 1 原文）」 | ② | 待复核（ADR-0130 无 owner 复核记录；tf/12 按甲落地） | 机制 |
| psr/06 / 07 | 「`ports/delivery_place_reference_view.go` 头注……『ADR-0130 Consequences 第一条 / 第三条』是按序位指别的文件里无标签的列表项」；07 同一条「随 tf/12 / tf/14 一并改成引文」 | ④ | 痕迹可剪：`git grep 'Consequences 第' -- internal/parcelshipment/ports` 两处仍在（`delivery_place_reference_view.go`、`commercial_resolution_reference_view.go`）；tf/14 已进 main 未顺手改；无票 | 机制 |
| psr/06 | 「谱系包裹会落『没有收件地点』……谱系落地那票要补这一路」 | ④ | 条件未到（包裹身份谱系今天无模型、无票） | 机制 |
| psr/07 | 「ADR-0133 越权风险点 5（谱系包裹与集运单元同答没有）若 owner 复核后改口径，本票随之改那一格」 | ② | 待复核 | 机制 |
| sa/02 | 「越权风险点（供用户复核，不认可走 supersede）：① 决定二『第一处限制即停后续项』……」 | ② | 待复核：写在票内，ADR-0122 正文无「越权」四字、无 owner 复核记录，脚本漏此票 | 机制 |
| sa/02 | Decision 五原句「改为经本点读口读『要执行哪些控制项』属 SA 地盘，另立票」 | ① | 已落：本票即那张票；SA 适配器 `LoadControlPolicy` 今天「分三段：回指换闭包 → 闭包取合同 → 合同读声明」并持 `contents` | 机制 |
| sa/03 | 「越权风险点（供 owner 复核）」五条 | ② | 已复核（ADR-0125 2026-09-09；第 5 条过渡态 09-10 随 sa/04 解除） | 机制 |
| sa/03 | 「ADR-0047 决定三的 `HELD` / `CREDIT_EXPOSED` 两格降为投影而保留，未 supersede——若 owner 认为该合成一格，那是 ADR-0047 的改动，不在本票」 | ① | 已被否（owner 复核 1：「不 supersede……不急」） | 机制 |
| sa/04 | 「越权风险点六条待 owner 复核，任一被推翻改的是 ADR-0132 对应那一句」 | ② | 待复核（ADR-0132 无 owner 复核记录） | 机制 |
| sa/04 | 「可另立票：admin-web 授权处置队列页与处置操作（票面『边界』已划归 admin-web 另票）」 | ① | 无票：`git grep 授权处置 -- .scratch` 只命中 sa/03、sa/04、tf/14 | 机制 |
| sa/04 | 「处置授权的翻译适配器没建，只建了 `UnconfiguredAuthorizedDispositionAuthorizer`：PC 授权动作封闭集今天没有『授权处置』一格（ADR-0132 越权风险点 2）」 | ①② | 无票（PC 侧 `AuthorizedAction` 今天三格：ManualReview / ActiveRejection / SourceDataAmendment）；归属归 PC，待复核同上 | 机制（动作词）/ 实例（`PAR-COM-14` 登记） |
| sa/04 | 「`补资金后重判`不入集，SA→PS 资金事实缝或『已记录判断失效重判』任一落地时回看」 | ④ | 条件未到 | 机制 |
| cmdr/01 | 「越权风险点五条在 ADR 内单列」 | ② | 已复核（ADR-0113 2026-09-09） | 机制 |
| cmdr/01 | 「身份与版本的形状、替代关系怎么记、与运输舱单是不是分表——这三件难逆转，多半要一篇 ADR；由 owner 定要不要」 | ③ | 已落（ADR-0113） | 机制 |
| cmdr/01 | 「快照里的承运总单引用暂无生产来源——不造替身」；「改变词的认词随真渠道 Intake 一起来」 | ④ | 条件未到 | 实例 |
| cmdr/01 | 「不动 `parcel-pricing`：主单主体的形状归 shape-gaps/03」 | ① | 有票 `pricing-shape-gaps/03`（resolved 2026-09-04） | 机制 |
| tf/09 | 「越权风险点（供 owner 复核，实现票不顺手定）」四条 | ② | 已复核（ADR-0114 2026-09-09） | 机制 |
| tf/09 | 「是否该按（段，对象）铸以免更正后重开任务，owner 定」 | ③ | 已复核范围内（ADR-0114 四条逐条认可）；未核认可条目是否明指这一问 | 机制 |
| tf/10 | 「越权风险点：① 参与表主键换四元……」四条 | ② | 已复核（ADR-0112 2026-09-09，票内同刻） | 机制 |
| tf/10 / 11 | 「NR/PS 对更正后参与的消费不在本票（ADR-0112『不在本记录内』）」；11「那是消费方的票」 | ① | 无票：`git grep 更正后参与 -- .scratch` 只命中 tf/10、tf/11 | 机制 |
| tf/11 | 「归 owner 复核：越权风险点 ①–④」 | ② | 已复核（票内「owner 复核 2026-09-09 认可，①–④ 逐条」） | 机制 |
| tf/11 | 「跨文件计数那几处与替身重复留待下次触及同文件时顺手改」 | ④ | 无票（作者明说随手改）；未核今天是否仍在 | 机制 |
| tf/12 | 「本缝只管包裹这一种——集运单元的派送目的地不是本票的事（集运单元整体末端派送是否成立本身未裁）」 | ③ | 待裁；无票（`git grep 集运单元 -- .scratch` 无以它为题的票） | 机制 |
| tf/12 | 「Standards 非阻断 (1) 的『对象种类维』归 tf/14 立票时决定」→ tf/14「不在本票立种类维，若要 TF 侧先分流另立 TF 小票」 | ① | 无票：`git grep 种类维 -- .scratch` 只命中 tf/12、tf/14 | 机制 |
| tf/12 | 「ADR-0130 越权风险点 1（`待复核`在 TF 端口上的落法）本票按甲落地，原文不改」 | ② | 待复核 | 机制 |
| tf/13 | 「越权风险点五条在 ADR-0131 末节」 | ② | 待复核（ADR-0131 无 owner 复核记录） | 机制 |
| tf/13 | 「依据（`Basis`）不交：任务口今天只收首尾两点……要交得先拓任务的形，不在本票」 | ① | 无票；是否需要未裁 | 机制 |
| tf/13 | 「`ports/delivery_requirement.go` 头注……那半句在 rebase 后过时了；tf/14 落地时同一段头注要再改」 | ④ | 已落（tf/14 `595c659c`「头注一段改准」） | 机制 |
| tf/14 | 「越权风险点五条单列在 ADR-0133……1 按甲落地，原文不改」 | ② | 待复核 | 机制 |
| tf/14 | 「三条缝都接上后若要统一加子原因另起」 | ① | 无票（可选） | 机制 |
| tf/14 | 「集成用例……要在 cmd 测试里种 PS 已接受委托 + NR 计划 + PC 闭包三套夹具，是集成用例的量，另立」 | ① | 无票 | 机制 |
| tf/14 | 「`delivery_condition_source.go` 头注那句『今天 PC 没有这一族……』已不成立」 | ④ | 已落（`5dda0fb2` 头注改准，今天写「这一族的表与写口 pcg/11 已落，今天缺的是租户登的声明——实例半边」） | 机制 |
| wbr/01 | 「非阻断 ① HTTP 欠一格且拆出物无票……推送前需立票」 | ① | 有票 `wbr/11`（resolved，进 main `94893c35`） | 机制 |
| wbr/01 | 「越权风险点（单列，供 owner 复核）：① 『接管记录在前』……② ……③ 未配置自成一格」 | ② | 待复核（ADR-0128 无 owner 复核记录） | 机制 |
| wbr/01 | 「③ 若将来决定记录落库，评估一格随之（PS 迁移号按当时派单）」 | ④ | 条件未到；无票（`git grep 决定记录落库 -- .scratch` 只命中 wbr/01、wbr/11） | 机制 |
| wbr/01 | 「`application/submit_shipment_request.go` `blockedOutcome` 头注与 `ports/ports.go` `ProductionHandoffObservation` 头注各自复述 domain `HandoffObservation` 的格名……随票记，动 `blockedOutcome` 头注的下一笔顺手改」 | ④ | 痕迹仍在（`blockedOutcome` 头注今天仍逐格复述）；无票 | 机制 |
| wbr/02 | 「PSC-2 何时引入不归本票……`CanonicalizeSubmissionPayload` 注释记『届时重新导出版本出口』」 | ④ | 条件未到（有意留的提醒注，不是欠账） | 机制 |
| wbr/03 | 「未落 / 拆出：① PS 登记面……PS 另立票；② SA contract 段……；③ 比例额度基数」 | ① | 有票 `wbr/08` / `wbr/09` / `wbr/10`（均 resolved） | 机制 |
| wbr/03 | 「归 owner 复核：越权风险点 ①–④」 | ② | 已复核（票内「owner 复核 2026-09-09 认可，①–④ 逐条」；ADR-0127 正文不含四字，脚本漏此票） | 机制 |
| wbr/03 | 「④ `candidateCount` 固定 2——沿用既有取舍，三个解析器同款……要改应三处一起、另立票（不立票，等触及时顺手）」 | ① | 无票（作者明说等触及时顺手） | 机制 |
| wbr/04 | 「观察（不在本票地盘）：`ActiveRejectionAdapter` 没有对称的动作守卫」 | ① | 无票：`git grep ActiveRejectionAdapter -- .scratch` 只命中 wbr/04 与 psr/03（后者拿它当模板，不是守卫票） | 机制 |
| wbr/04 | 「`judgment_continuation.go` 的 `ErrUnexpectedAuthorizationOutcome` 头注计数变旧……交作者另笔 `mcp2-wbr04-followup` 改成不计数写法」 | ④ | 已落：头注今天写「不在这里数端口也不数调用点……（AGENTS.md「计数与行号同构」）」 | 机制 |
| wbr/05 | 「可另立票补一条闭包形态小测试」；「文件名另起一笔改（不混进纯 .md 笔）」 | ① | 前者无票；后者已落（`domain/reference_closure_test.go` 在） | 机制 |
| wbr/06 | 「越权风险点（单列，供 MCP-1 / owner 复核）」两条 | ② | 已复核（ADR-0124 2026-09-09） | 机制 |
| wbr/06 | 「W02 真要跑历史回放时缺的是实例半边（`PAR-GOV-01` 数据集、候选版本组）与一个批量驱动——驱动消费本票的 `ReplayPricingEvaluationHandler` 即可，另立票」 | ① | 无票 | 机制（驱动）/ 实例（数据集） |
| wbr/06 | 「评价册查阅面要不要透出 `replayOf` 另立票」 | ①③ | 无票；待裁（`git grep replayOf -- .scratch` 只命中本票） | 机制 |
| wbr/07 | 「越权风险点三条原样留在『裁决』节供 owner 复核」 | ② | 已复核（ADR-0123 2026-09-09） | 机制 |
| wbr/08 | 「非阻断两条随票记，不另立票（① 等第三个选择器；…）」 | ①④ | 条件未到 | 机制 |
| wbr/09 | 「(2) `IsZero()` 与 (3) 头注后半删变更史、(4) `Policy()` 读法——随 10 顺手收，不另立票」 | ① | 已落（wbr/10 `43362615`「顺手收落 wbr/09 三条」） | 机制 |
| wbr/09 | 「若评审判两格是硬要求……那是另一张票的形」 | ① | 已被否（评审未判硬要求） | 机制 |
| wbr/10 | 「越权风险点（单列）」五条（ADR-0129） | ② | 待复核（ADR-0129 无 owner 复核记录） | 机制 |
| wbr/10 | 「业务上可能还要『合同声明基数』——本票明确不收，留给将来有租户提出时另立」 | ① | 条件未到 | 实例 |
| wbr/10 | 「给 seed 补一条信用政策不在本票判据内，属合成 `S` 证据另议」 | ① | 无票 | 机制（合成 `S`） |
| wbr/10 | 「Standards ②（ADR-0129 Consequences 漏列 pending carrier 存量）随票记，不改 ADR 正文、不另立票——owner 若判要补由 SA / PC owner 各补一句」 | ②④ | 待复核 | 机制 |
| wbr/11 | 「未落 ① admin-web `handoffConfirmationReference` 一格归 awf/22 之后的 shipment-request 页面票」 | ① | 无票：`git grep handoffConfirmationReference -- apps .scratch` 零命中（只在 PS http 两文件） | 机制 |
| wbr/11 | 「通往他方权威的真通道（`PAR-GOV-05..07` 实例半边）不归本票」 | ① | 条件未到 | 实例 |

## 一、无票且机制半边（立票名单候选）

按「今天就能开工、不等租户」筛；括号内是来源票与可核回去的命令。

1. 伞票 awf/07 收口时没接的判断题四组：awf/09「何时开始拒收无版本旧串」（`PCC-1` 声明门 Decision 五）；awf/11「`canonicalSupplierAgreementBody.body()` 与信用政策时刻解析同形可抽助手」等三条；awf/22「收集器替手抄声明」「`ObjectPicker` 参数化」；awf/12 / awf/18 票面文字与判据口径。——`git grep -n '伞票.*收口' -- .scratch/admin-write-faces/issues`
2. awf/10：`cmd/parcel-commercial/translate.go` 的 `controlRequirementFrom` / `controlBindingFrom` 与领域 `PreAcceptanceControlRequirementNamed` / `financialControlBindingOf` 同规则两份，CLI 侧收口。——`git grep -n -e 'func controlRequirementFrom' -e 'func PreAcceptanceControlRequirementNamed' -- cmd internal`
3. awf/17：授权授予册（`AuthorizedAction` 一族）的管理台写面是否需要，未裁、无票。
4. awf/20：读准入第三桶（放行且不经读口 → 200）与 ADR-0078 判据措辞；客户合同正文 `preAcceptanceControl.requirement` 封闭集怎么供词表。
5. awf/23：`reconcileShellReference` 空白口径与 `requireField` 不一致（`strings.TrimSpace`）。——`git grep -n -A6 'func (payload SettlementPolicyBodyPayload) reconcileShellReference' -- internal`
6. adle/02：九支 `merged/` NOT-ABSORBED 要不要追。
7. ftr/10：前一版本已形成 `HELD`、新版本形成时旧冻结谁释放（受控补充编排或 SA）。——`git grep -n 旧冻结 -- .scratch`
8. fti/01：映射表过期一格要不要立，归 NO owner 与映射表所有者（待裁）。
9. pcg/11：交付条件**内容**读口（UC-TF-006 步骤 5 的第二个消费方）；交付条件管理台表单（admin-write-faces 第十一册）。——`git grep -n -e 内容读口 -- .scratch`；`git grep -l DELIVERY_CONDITION -- apps/admin-web/src`（零命中）
10. pgt/01：`-p 1` 放不放（以 `ci.yml` run 号实测为据）；`ALLOW_CONNECTIONS false` 小笔。
11. psr/04：资料修订编排「版本与意图不同事务」那格诚实的缝，归 PS owner 另裁。
12. psr/06：按引用解析回地址内容的读口（第二个消费方）。
13. sa/04：admin-web 授权处置队列页与处置操作；PC `AuthorizedAction` 加「授权处置」一格（归属待 ADR-0132 复核，动作词是机制、`PAR-COM-14` 登记是实例）。——`git grep -n -A4 'AuthorizedActionInvalid AuthorizedAction = iota' -- internal/partycommercial/domain/authority_grant.go`
14. tf/10 / 11：NR / PS 对更正后参与的消费（ADR-0112「不在本记录内」）。
15. tf/12：集运单元整体末端派送是否成立（待裁）；TF 侧对象种类维（tf/12 / 14 都点名、都没立）。
16. tf/13：派送任务是否要交依据（`Basis`）——拓任务的形，是否需要未裁。
17. tf/14：三缝齐后统一加子原因（可选）；PS + NR + PC 三套夹具的集成用例。
18. wbr/04：`ActiveRejectionAdapter` 对称的动作守卫。
19. wbr/05：闭包形态小测试一条。
20. wbr/06：W02 批量驱动（机制半边）；评价册查阅面要不要透出 `replayOf`（待裁）。
21. wbr/10：seed 补信用政策（合成 `S` 证据）。
22. wbr/11：admin-web `SubmitShipmentRequestPage` 的 `handoffConfirmationReference` 一格。

## 二、待 owner 复核（交用户）

脚本算出无 owner 复核记录的 6 篇 ADR + 脚本漏掉的票内越权风险点：

1. **ADR-0128**（wbr/01）越权风险点 ①②③。
2. **ADR-0129**（wbr/10）五条；另 wbr/10 评审 Standards ②「Consequences 漏列 pending carrier 存量」也归 owner。
3. **ADR-0130**（tf/12、psr/06）五条；tf/12 已按风险点 1 的甲（`待复核`译成 `RequirementMissing`）落地，psr/06 补一条「`Undetermined` 在 TF 端口上是否加第三格归 TF owner」。
4. **ADR-0131**（tf/13、nr/03）五条。
5. **ADR-0132**（sa/04）六条；其中 2「授权处置」要不要进 PC 授权动作词汇归 PC。
6. **ADR-0133**（tf/14、pcg/11、psr/07）五条；tf/14 已按风险点 1 的甲落地；pcg/11 另有两条只写在票内：ADR-0031 是否补「输入对着册上另一份立不住时 Save 返回 error」一句、`未采用产品`要不要分格哨兵。
7. **sa/02 票内**越权风险点（ADR-0122 正文不含四字）：决定二「第一处限制即停后续项」等。
8. **adle/02**：判据 2 第三案例与票面预期相反，头注照实写、票面未改，等 owner 定。

已复核、不必再交：ADR-0112 / 0113 / 0114 / 0116 / 0119 / 0120 / 0123 / 0124 / 0125 / 0126（各含「owner 复核 2026-09-09 认可」），以及票内复核的 psr/05、tf/11、wbr/03、awf/20、label-channel/24。

## 三、痕迹可剪（小笔名单）

都是评审点过、票已 resolved、`c7e3522c` 上仍在的注释：

1. `migrations/party_commercial/0028_publication_draft_and_approval_duty_rule.sql` 头注「正文快照（content_document）就是服务端规范化文档（PCC-<n>）的字节」——awf/08 记与实现不符。——`git grep -n 的字节 -- migrations/party_commercial`
2. `apps/admin-web/src/pages/party/{PreAcceptanceFinancialControlPolicy,SettlementPolicy,SupplierAgreement}PublicationForm.tsx` 头注「十册没有一类是矩阵」（跨文件计数）。——`git grep -n 没有一类是矩阵 -- apps`
3. 同目录四张表单 + `pricing/SeriesRegistrationForm.tsx` / `SeriesReviewPanel.tsx` 头注「决定一那三条的直接读数」（数 ADR-0101 条目）。——`git grep -n 直接读数 -- apps`
4. `apps/admin-web/src/pages/party/policy-rows.test.ts` 夹具头注「那三行」「钉住的三行」。——`git grep -n -e 那三行 -e 钉住的三行 -- apps`
5. `PublicationFormFields.tsx` / `publication-form-shared.ts` 头注「九份」「五张票」。——`git grep -n -e 九份 -e 五张票 -- apps/admin-web/src/pages/party`
6. `internal/parcelshipment/ports/delivery_place_reference_view.go` 与 `commercial_resolution_reference_view.go` 头注「Consequences 第一条」按序位指他文件列表项（psr/06 / 07 评审说随 tf/12 / 14 改成引文，tf/14 已进 main 未改）。——`git grep -n 'Consequences 第' -- internal/parcelshipment/ports`
7. `internal/parcelshipment/application/submit_shipment_request.go` `blockedOutcome` 头注复述 domain `HandoffObservation` 各格（wbr/01 评审 Standards ①）。

## 未核

时限内没逐句核到底、不猜：

- awf/09 `IsRegisterCanonicalized` 头注那半句是哪一笔改掉的。
- awf/14 `validatePricePolicyBody` 今天叫什么、在哪。
- awf/15 `NewCurrencyCode` 今天是否答存在性（评审 Spec 非阻断 1）。
- awf/21 `api.ts` `CustomerServiceRuleRecord` 头注是否仍逐字列三值。
- awf/24 `TaxDisposition.valid` 与 `NewTaxCaliber` default 分支今天是否仍双写。
- nr/03「可随票 13 或另起小笔补一例」指的那一例 tf/13 补没补。
- tf/09「按（段，对象）铸」那一问是否在 ADR-0114 owner 复核四条之内明指。
- tf/11「跨文件计数那几处与替身重复」今天是否仍在。
- 以下票逐句核过、无余工句：arrf/01、awf/16（追笔已进 main）、awf/19、awf/21（除上面一条）、pcg/07（除 SA 一条）、psr/02、tf/13（除表内三条）、wbr/02、wbr/07、wbr/08。

## 重放

```
git worktree add -b mcp5-followups D:/tops/idp-parcel-mcp5-followups c7e3522c
git log --since=2026-09-05 --name-only --pretty=format:'' -- .scratch | 过滤 /issues/ | Sort-Object -Unique   # 61
按 Select-String '^Status:\s*resolved' 筛                                                                       # 57
powershell -File scripts/owner-review-queue.ps1 -Out .scratch/owner-review-queue.md                           # 底稿
四类词表（按「。；换行」切句后匹配）：
  ① 另立|另起|不在本票|无票|另一张票|另张票|另开一|另成票|不属本票|不归本票|留给.{0,12}票|另派|归 ?[a-z-]+/\d+|不做的|另计
  ② 越权风险点|owner 复核|供用户复核|供复核|供 owner
  ③ 待裁|要裁的|归 owner 定|等 owner 定|owner 定|等 owner|待 owner|交 MCP-1 定|归 MCP-1 定|未裁|待定|未决|等用户|交用户|要裁
  ④ 暂留|后续剪|稍后剪|落地后改|落地后剪|落地后补|落地那票|届时|先留|留到|回看|要补这一路|替身|基线行|头注|过渡态|暂时|暂且|先按|先只|今日形状|占位|临时
```
