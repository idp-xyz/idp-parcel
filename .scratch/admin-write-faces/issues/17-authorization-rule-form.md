# 17 `AUTHORIZATION_RULE` 版本的运营主路径：逐字段表单（取消授权按请求方逐格可加行）

Category: enhancement
Status: resolved——通道 4 于 2026-09-08 20:1x 认领、20:4x 完工，分支 `mcp4-awf17`（树 `D:/tops/idp-parcel-mcp4-awf17`，基 `802ae400` = awf/20 tip；20 进 main 后 rebase 再交推送方），逐笔 SHA 见「完成记录」，进 main 记录待推送方广播后补入；形状已裁清（逐字段表单 + 取消授权按请求方可加行；本票无待裁问题）；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08（已进 main）；[20](./20-publication-vocabulary-read-face.md)（词表读口公共半边，2026-09-08 通道 1 代裁立票；落点广播前表单里的下拉先按票面写成占位、不内置枚举）

## 册与载荷

显示在**政策页·授权规则册**。版本壳之外归它的声明是 `declarations.cancellationAuthority[{party, rule}]`（0013 取消
授权目录：请求方是封闭二值，规则是引用）。授权授予册（`authority_grant.go` 的 `AuthorizedAction` 一族）不经这条
发布路，不在本票。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，取消授权可加行。** 频次低、配置员操作、正文是一张两列几行的表。请求方下拉由服务端词表读口供（封闭
二值，表单不内置枚举——pc-gaps/08 若给 `AuthorizedAction` 加格，那是授予册的事，与这里的请求方二值无关，别混）；
同一请求方第二行由构造门拒。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；本册无特有硬句——「pc-gaps/08 加的授权动作格与这里的请求方二值无关」已写在上面「选形与理由」里，不重复。

## 完成判据

授权规则册旁多一签「发布授权规则版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见（取消授权
按请求方逐格上列）；tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0013；不动授权授予册。

## 完成记录（通道 4，2026-09-08，分支 `mcp4-awf17` 基 `802ae400`）

逐笔（分支上的 SHA；进 main 后由推送方广播新旧对照）：

- `3264f362` 票面 Status 转 in-progress。
- `8c89f282` **地盘外纯抽取**：`domain/service_stage_content.go` 把 `NewCancellationAuthorityContent` 的行门抽成同包未导出
  `declaredCancellationAuthorityByParty`（至少一行 / 每行请求方在集内、规则非空 / 同一请求方不两行），行为与错误一字不变，
  让发布前的正文输入面与绑定到版本上的目录过同一道门——「同一请求方第二行由构造门拒」在预览上就答得出。
- `e2302ecf` 领域：新文件 `publication_canonicalization_authorization_rule.go`（+_test）——`AuthorizationRuleBody{CancellationAuthority}`、
  `validate` 走上面那道门、文档节 `authorizationRule.cancellationAuthority[{party, rule}]` 按请求方枚举序写出（表单换行序不换
  摘要）、`body()` 折回逐行过构造门、导出反查 `DeclaredCancellationPartyNamed`（扫值域按 valid() 认）。`publication_canonicalization.go`
  只加本册一格（PublicationContent 一格、kind 不符一句 + switch 一支、IsRegisterCanonicalized 一册、文档一格、Rehydrate 一支、
  PCC-1 头注一行），加册不换号。
- `d756e2d9` 应用：`publish_commercial_authority.go` publicationContentOf 一 case（目录有行即正文在场，零行是壳单独发布不对账）、
  `publication_draft.go` declarationsOfContent 一支（载体上的目录原样交回 CancellationAuthority 一通道）；新测试
  `publication_authorization_rule_test.go`。既有测试未改（`publish_commercial_authority_test.go` 没有带 sha256 串 + 目录行的授权规则例）。
- `a0c126e4` http：新文件 `publication_draft_payload_authorization_rule.go`（+_test）——`AuthorizationRuleBodyPayload` /
  `CancellationAuthorityRulePayload`，逐格问题落在 `authorizationRule.cancellationAuthority[i].party / .rule`；跨行的判（零行、同请求方
  两行）留给领域门在预览上答`未受理`带成因。`publication_draft_payload.go` 只加一格 + Publication() 一段。
- `f5fc7117` **地盘外**：`scripts/demo-seeds/data/commercial/publish-batch.json` 的 SYN-AUTH-CANCEL-01 项 `contentDigest` 由
  `sha256:syn-…` 改为算出的 `PCC-1:c2ae5729…343007ad`（本册开对账门后 seed 才能落；另两项授权规则不带目录，壳单独发布不对账，不改）。
- `fcf3d703` admin-web：新文件 `party/authorization-rule-form.ts`（+.test.ts，八例）与 `party/AuthorizationRulePublicationForm.tsx`；
  `publication-draft-api.ts` 只加 `AuthorizationRuleBodyPayload` / `CancellationAuthorityRulePayload` 两型 + 载荷上 `authorizationRule?`
  一格；`CommercialPoliciesPage.tsx` 只加一签「发布授权规则版本」+ 一个 import。请求方下拉 = `fetchPublicationVocabulary('AUTHORIZATION_RULE')`
  的 `party` 集 × `cancellationPartyLabels`（`partyOptionsOf` 三态：读到 / 403 未配置显占位 / 读不到显占位），无内置码、不预选；
  行照原样上送（没选请求方、空规则、同请求方两行、零行都由服务端答），不代判不补默认。
- `e9ffa685` 机制清点在 `fcf3d703` 干净检出上重生成——PC 生产文件 +2、测试文件 +3、**http 适配器文件 +1**（那笔提交信里写的
  「端点声明数不变，声明面 +1」措辞不准，此处更正：端点表本票一行未加，变的是 adapters/http 生产文件数）。只作取证，推送方在
  tip 上重生成兑底。

自验（`fcf3d703` 的 detached 检出 `%TEMP%\idp-verify-awf17`，未设 DSN——本票无 postgres 改动）：`gofmt -l .` 列出 0；`go build ./...`
退 0；`go vet ./...` 退 0；`go test -count=1` PC 领域的 14 个反向依赖包（`go list` 反查，含三个 `cmd/`）+ `./internal/architecture/...`
全部 `ok`；admin-web `tsc --noEmit` 退 0、`run-tests` 131/131（本票 8 例在内）。

对照票面：Go 侧只加本册规范化一格（地盘外那笔是纯抽取，不是第二格）；表单 → 预览摘要 → 待批准 → 批准 → 发布由公共半边走；
「结果在同册立刻可见」靠 `onPublished` 刷读面，读面的取消授权按请求方逐格上列是既有列（policy-rows.ts 未动）。请求方下拉遇 403
显占位而不退回手填：封闭集不该由操作者手拼原词，且那堵墙前四口同样 403、本表单本来就提交不了。

## Comments

- 2026-09-08 20:1x · 通道 1 → 通道 4：不经点名直派（完工报即在线证据），基 awf/20 tip 开，20 进 main 后 rebase。
- 2026-09-08 20:4x · 通道 4：完工报已发通道 1，评审待通道 1 另派；本票的领域 / 应用 / http 三处共享文件改动与通道 2/3/5/6 各自加的
  一格落在同一段落，撞了由推送方按「纯加行」解。
- **评审 ← 通道 2 · 钉 `9799785a` · 21:28**（非作者；基线 `802ae400`，评 `802ae400..9799785a` 六笔代码，票面笔与清点笔不评代码；
  隔离树 `%TEMP%\idp-review-awf17` 验后已拆。两轴由该会话串行独立过，20 分钟限时内未起隔离子代理。）
  - **Standards · 阻断：无。非阻断 5**：(1) `authorization-rule-form.ts` `authorizationRuleFieldPaths` 认领了 `'authorizationRule'` 节根，但
    `AuthorizationRulePublicationForm.tsx` 只渲染 `authorizationRule.cancellationAuthority` 与行内两格的 `Problems`——服务端若把问题点到节根会
    被认领又不显示；今天 http `AuthorizationRuleBodyPayload.body` 不点那一格（正文缺席走预览 NOT_ACCEPTED），只是潜在吞问题；同形
    `customerContractFieldPaths` 没认领节根。建议去掉该项或补一处 `Problems`。(2) `Field` / `Problems` 与 `CustomerContractPublicationForm.tsx`、
    `SupplierAgreementPublicationForm.tsx` 同形复制；第 2 波刻意「表单本体各进新文件」避撞，本票不该动共享文件，建议 12/13/15 落地后单开一票
    抽到 `PublicationDraftFlow` 旁。(3) `DeclaredCancellationPartyNamed` 与 `cmd/parcel-commercial/translate.go` `cancellationPartyFrom` 是同一枚举
    的两份反查；后者地盘外且先于本票，导出反查落地后可改为调它，归后续票。(4) `canonicalAuthorizationRuleBody.body()` 折回时 rule 空串报
    `NewRuleReference` 原错，同一格在 `declaredCancellationAuthorityByParty` 门里是 `ErrCancellationAuthorityNotConfigured`——两处对「规则空」
    成因不同名；快照来自已过门正文，实际到不了。(5) `patchRow` 从闭包 `draft.rows` 取行再 setDraft，非函数式更新；onChange 一次一格实际不丢，
    与兄弟表单同写法。**无发现（核过）**：注释全中文、无行号、跨文件引用一律符号名/票号；领域新文件只 import fmt/math/sort，不碰 HTTP/pgx；
    错误族只加 case；评审树 `go test -count=1 ./internal/partycommercial/... ./cmd/parcel-api/ ./internal/architecture/` 全 ok。
  - **Spec · 阻断：无。非阻断 1**：完成判据「tsc / run-tests 绿」评审树无 node_modules 未复跑，取作者自验 131/131，推送方在 tip 复跑（见下）。
    派单点名四问：① `8c89f282` 纯抽取成立——拥有对象判仍在前，零行 / 行内无效 / 同请求方两行三条错误值与顺序逐一相同，
    `NewCancellationAuthorityContent` 仍返回同一个 `CancellationAuthorityContent{owner, byParty}`；② `canonicalAuthorizationRuleBodyOf` 先 copy 再按
    Party 枚举序 sort，`validate` 在排序前过同一道门、同请求方两行由 `ErrConflictingCancellationAuthority` 拒，领域两用例与 http
    `TestAuthorizationRulePayloadProblemsAreCollectedPerRow` 钉住；③ `partyOptionsOf` 三态，选项只来自 `fetchPublicationVocabulary('AUTHORIZATION_RULE')`
    的 `party` 集，`PartyPicker` 首项「未选」、行初值 `party: ''`，无内置码无预选，`cancellationPartyLabels` 是页面中文词表不是码源，服务端
    `publication_vocabulary.go` 确答 `party` 集；④ `f5fc7117` seed 必要——本票后 `publicationContentOf` 对 SYN-AUTH-CANCEL-01 返回正文在场、
    ADR-0126 决定二对账门开，评审者用领域 `CanonicalizePublicationContent` 独立算得 `PCC-1:c2ae5729…343007ad` 与 seed 逐字节同；另两项授权规则
    无目录不对账，不改是对的。硬句逐条 ✔（表单不算摘要不裁门、预览/录入同摘要、不收不送批准人、不给默认不预选）；边界 ✔（authority_grant.go、
    0013、endpoints.go / ports.go / party/api.ts 未动）；`IsRegisterCanonicalized` 一册与 `Rehydrate` 一支是接册必需，不算越界。
  - **汇总**：Standards 0 阻断 / 5 非阻断（最重：fieldPaths 认领节根不渲染）；Spec 0 阻断 / 1 非阻断。可进 main。非阻断随票记不挡合入，
    (1) 建议作者另笔修、(2)(3) 归后续票。

## 进 main 记录（推送方通道 1，2026-09-08 21:4x）

- 重放：隔离 detached 树 `%TEMP%\idp-replay-awf17` 基 main `f722d9f0`（含 awf/20 的 main 版 `c05ee34d`，与分支基 `802ae400` 代码同字节），
  `802ae400..9799785a` 跳清点笔 `e9ffa685` 后八笔 cherry-pick **零冲突**：`3264f362→6148548e`、`8c89f282→3b15236c`、`e2302ecf→9660a360`、
  `d756e2d9→a895f003`、`a0c126e4→772c9339`、`f5fc7117→a954ac0f`、`fcf3d703→ec6289f7`、`9799785a→a9d1af33`；
  `git diff 9799785a a9d1af33 -- internal cmd apps migrations` 为空。
- 清点在 `a9d1af33` 重生成 `1d6ac983`（PC 生产 107→109 / 测试 111→114、声明面 20→21），与分支清点笔 `e9ffa685` 逐字节同。
- 验证钉 `1d6ac983`：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；含 DSN `go test -p 1 -count=1 ./...` 退 0，**100 ok / 0 FAIL /
  16 无测试 / 0 cached**（21:29→21:39，587s）；探针 `partycommercial/adapters/http -run AuthorizationRule` PASS 2；admin-web `tsc --noEmit`
  退 0、`run-tests` **131/131**（node_modules 走 junction，验完拆）。
- 本笔（票面 + 伞票 07 子票表 17 行 + tasks.md）之后 `--ff-only` 进共享 main，推前 `ls-remote` 核 `f722d9f0`。分支 `mcp4-awf17` 内容已全在
  main，指针改名 `merged/`，树 `D:/tops/idp-parcel-mcp4-awf17` 归通道 4 自拆。
