# 01 PC 侧：ADR-0058 与 ADR-0059 在 main 里有没有执行器

Category: chore
Status: resolved——取证完成，答案见「答案」节；去留决定归集成方，本票不代定
Assignee: MCP-4

**只读取证票。零代码改动，不合并、不 cherry-pick、不拆树、不删分支。**

## 要答的问题

[父清单](../spec.md)第三类里有五笔实现已接受 ADR 的提交不在 main。`git cherry` 只答「这一笔的 patch 不在 main」，**不答「这件事在 main 里没做」**——main 上可能已有出自别的分支的等价实现。本票取证其中 PC 侧两笔：

| 分支 | SHA | ADR |
|---|---|---|
| `pn07-b6-stage-content` | `22bb69f` | [ADR-0058](../../../docs/adr/0058-stage-content-owned-by-rule-objects.md) 阶段内容声明按规则对象归属 |
| `product-version-closure-b7` | `9d8c09b` | [ADR-0059](../../../docs/adr/0059-rule-package-applicability-stored-not-selected.md) 规则包适用性照存不参与选择 |

逐 ADR 答三问：

1. **main 里有没有这条 ADR 决定的执行器？** 执行器指真正让那条决定生效的类型、函数或约束，不是同名文件、不是端口声明、不是注释里提到 ADR 编号。
2. **有的话，它与分支那笔是什么关系？** 等价实现 / 只覆盖一部分（说清哪一格没覆盖）/ 换了做法（说清换成什么）。
3. **没有的话，缺的是哪一格？** 按 ADR 的 Decision 逐条对，不要整篇打包判。

## 取证方法上的两个坑，务必避开

**一、「文件在」不等于「规则有执行器」。** 父仓已经栽过同型的跟头：只写测试的契约（`*_contract_test.go`）被读成实现，会高估就绪度；判据要的是**规则有执行器**。同理，端口声明是形状不是执行。

**二、「接错看着像接对」。** 一个按租户过滤的读口去查一张没有租户的表，编得过、测得过、页面也出得来，只是答的不对，没有任何测试会红。所以判「有执行器」时要往下走一层，看它实际约束住了什么。

## 写法红线

- **写证据不写结论**：每个断言锚住取证 SHA（本轮基线 `main = aeeb709`）与取数命令。命中数不作数，要打开看。
- **不用行号、不用计数引用别处**；数本身就是论点时必须锚 SHA。
- **中文**。改中文文件不要用 `Set-Content`（会写成乱码让 `go build` 报 `illegal UTF-8`）。
- 「查不到」要多问半句：是真查过，还是没想到能查。

## 地盘与禁止

- **只写 `.scratch/unmerged-branch-inventory/issues/01-*.md`**（本文件，答案追加到 `## 答案` 一节）。
- 不改任何 `.go` / `.sql` / `.tsx`；不动 `docs/**`；不改任何票面 `Status:`（本文件自己的除外）。
- 不跑 `go mod tidy`、全树 `gofmt -w` 或任何扫全树再写回的命令。
- 不合并、不 cherry-pick、不拆 worktree、不删分支。父清单里三棵有未提交内容的 worktree 一律不碰。
- 不提交，不推送。做完在频道报 MCP-2。

## 答案

取证于 `main = aeeb709`（本树 `git rev-parse HEAD main` 两值相同）。`.go` 与 `.sql` 在 `git status` 上零改动，因此下文直接读工作树等同于读 `main@aeeb709`；`git status` 里那批 ` M` 全是文档，与本票路径不交。

### 先答一件父清单没问、但答完三问才站得住的事

**这两笔的活都在 main，只是进 main 的不是这两个 SHA。** `git cherry` 报 `+` 在这两笔上是「换基座重放」，不是「活没进去」。

- `git log main --diff-filter=A -- <路径>` 逐件问「谁把它加进 main」：`stage_content_declaration.go`、`0013_stage_content_declarations.sql`、PS 侧 `stage_content_declarations.go` 三件同命中 **`6e4ccda`**；`acceptance_rule_package.go` 与 `0014_acceptance_rule_package.sql` 同命中 **`74a6c35`**。这两笔的提交标题与分支两笔逐字相同。
- **数在这里就是论点，故记数并锚 SHA**：`git show --stat` 下 `22bb69f` 与 `6e4ccda` 同为 13 文件 / +1598 / −51，`9d8c09b` 与 `74a6c35` 同为 7 文件 / +679 / −0；文件名单逐行相同。
- 逐文件比 blob（`git rev-parse <commit>:<path>`，各自比**引入时**那一版，不与今天的 main 比）：除 `internal/partycommercial/ports/ports.go` 外全部相同。`ports.go` 是累积型共享文件，两侧基座本就不同；只取该文件的新增行比对，两组均相同。
- 基座确实不同：`22bb69f^ = 5c3d03a`，`6e4ccda^ = 26864d9`。patch-id 含上下文行，换基座重放即不等价——`git cherry` 的 `+` 由此而来。

### ADR-0058：三问

**问一：main 里有执行器，逐条 Decision 对。**

*决定一（三件的拥有对象是规则对象版本，不是产品或合同）*——两侧各有一道，且都不是声明性的：

- 领域构造门在 `internal/partycommercial/domain/service_stage_content.go`：`NewIntakeQualificationContent` 与 `NewFinalRuleContent` 均要求 `owner.kind == AcceptanceRulePackageObject` 且 `owner.status == CommercialVersionEffective`，否则返回 `ErrUnusableRulePackage`；`NewCancellationAuthorityContent` 要求 `AuthorizationRuleObject` 且已生效，否则返回 `ErrUnusableAuthorizationRule`——即 Consequences 点名的那个「新增」错误，在 main 上确有其变量与其判据。
- 库侧在 `migrations/party_commercial/0013_stage_content_declarations.sql`：三张父表的主键都是 `(tenant_id, object_kind, object_id, version_label)`，子表在其后各加自己的声明格（`source_kind` / `outcome` / `party`）。`object_kind` 进 CHECK：两张规则包族父表钉 `object_kind = 4`，取消授权父表钉 `object_kind = 9`。**产品与合同不在这一族任何一张表的任何一列上**——「采用方不是拥有方」在这里是表结构拒绝，不是注释。

*决定二（三族分表、三口分读、不进 `ViewRevision`、缺行三格）*：

- 分表与分读：0013 建三父三子共六表；端口是 `IntakeQualificationView` / `FinalRuleContentView` / `CancellationAuthorityContentView` 三个，`StageContentDeclarations` 用三行 `var _` 断言分别实现。
- 无父行 = `found=false`：三个 `Load*` 都在 `errors.Is(err, pgx.ErrNoRows)` 上返回 `(零值, false, nil)`。
- **父行在场而子行空 = error，不折成未配置**：查询把子表聚合成 `COALESCE(..., '[]'::json)`，父行在而子行无就得到空数组，空数组交给上面那三道构造门必然落在「零来源／零声明」分支，于是 `Load*` 返回 error。这一格是靠「适配器不自己判空、把空交给领域拒」实现的，往下走一层看到的是这个交接，不是一句注释。
- **不进 `ViewRevision`**：`(*CommercialRegistry).ViewRevision` 的 `parts` 只由 `registry.versions`、`corrections`、`policies`（价格）、`settlementPolicies`、`products` 五处派生，函数体里没有任何一族阶段内容。这一条的执行器是「缺席」，而缺席只能靠读那个函数体确认——我读的是 `commercial_registry.go` 里 `ViewRevision` 的函数体本身。

*决定三（消费侧装配不得从 `SourceIdentity` 发明采用版本）*：

- `UnconfiguredAdoptedStageOwner` 两个方法恒返回 `(零值, false, nil)`；`NewDeclaredStageContent` 在 `owners == nil` 时装它，`DeclaredStageContent` 的三个 `*ContentFor` 在 `found == false` 时直接回未配置，根本走不到点读。
- 生产装配两处都在：`cmd/parcel-api/assemble_cancellation.go` 显式传 `nil`（即诚实未配置）；`cmd/parcel-dispatch/assemble.go` 装的是 `NewResolvedAdoptedStageOwner`，它按已接受委托的解析标识回指提供方闭包、取 `closure.AdoptedFor(...)`，仍然不从身份构造版本。后者是 ADR-0062 的回指路径，ADR-0058 自己的 Links 已把它记为「补第三条没写的回指路径；禁止从身份发明仍有效」——所以这不是偏离，是那条链按 0062 走远了一步。

*Consequences 那条容易被跳过的*：`requireOwnedVersion` 强制 `tenant == owner.Tenant()`，不等即 error 且不交内容。**这正是本票第二个坑要防的那格**：没有它，「按租户查库、按拥有对象重建」两处各写各的就能把 A 的行装进 B 的规则版本，编得过测得过页面也出得来。它在 main 上确实存在，且三个 `Load*` 入口第一句就调它。

**问二：与分支那笔是等价实现**，同一份活换基座重放（证据见上一节）。此后 main 又走远两步，不影响等价判定，但记下免得下一个人把差异读成分歧：`service_stage_rules.go` 与 `service_stage_content.go` 今天的 blob 已与分支不同；`adopted_stage_owner.go` 是 main 上后加的（ADR-0062），分支没有。

**问三：不适用**（无缺格）。

### ADR-0059：三问

**问一：main 里有执行器，逐条 Decision 对。**

*决定一（正文按族 B 点读，无 Save，缺行三格）*：`ports.AcceptanceRulePackageContentView` 只有 `LoadAcceptanceRulePackage` 一个方法——**这个端口上确无 Save**。`AcceptanceRulePackages` 实现它：无父行 → `pgx.ErrNoRows` → `found=false`；父行零子行 → `'[]'::json` → `NewAcceptanceRulePackage` 按「空包等于无条件接受」拒绝 → 上抛 error。

*决定二（五维照存，但不进 `CommercialRegistry` / `ViewRevision`，也不改候选过滤）*——三格分别有执行器：

- **照存**在 `0014_acceptance_rule_package.sql` 父表：`service_product_id`、`contract_id`、`legal_entity_ref`、`scope_ref`、`effective_starts_at`、`effective_ends_at`，且父表 CHECK 钉 `object_kind = 4`、区间 CHECK 要求终点晚于起点。
- **不进 `ViewRevision`** 同 0058 那条：`ViewRevision` 函数体的五处派生源里没有规则包正文的任何一维。
- **不改候选过滤**：`(*CommercialRegistry).applicable` 只判租户、`kind`、`scope`、`status == CommercialVersionEffective`、以及 `selectionInterval(version).Contains(key.Anchor.At())`；`ResolveCommercialBasis` 走的就是它。五维一个都没被读。这一格也是靠缺席成立的，我读的是 `applicable` 的函数体。

*Consequences 那条*：`acceptance_rule_package_rule` 的 `rule_category` CHECK 是五值封闭集，与适配器 `ruleCategoryFrom` 的 switch 五个分支同一组取值——库与代码两侧镜像同一封闭集，集外取值整行拒写。

**问二：与分支那笔是等价实现**（blob 同，证据见前节）。

**问三：Decision 三条不缺格。但「有执行器」在这一笔上要读窄一格，这是本票第二个坑的另一种形状。**

`LoadAcceptanceRulePackage` 在 main 上**没有生产调用方**：`NewAcceptanceRulePackages` 只在 `acceptance_rule_package_test.go` 与 `declaration_publication_test.go` 里构造，`cmd/**` 无一处。所以准确的说法是——**表结构、点读口、领域拒绝与「不参与选择」四件都有执行器，但今天没有任何运行中的路径去读这份正文**。这与 ADR-0059 自己 Consequences 写的「五维继续无人被选择逻辑读取——这是本决策的显式代价」一致，不是新缺口；记在这里是因为「有执行器」若被读成「有运行中的消费者」，两笔的成色就被抹平了：ADR-0058 的三口在 `cmd/parcel-dispatch/assemble.go` 有生产装配，ADR-0059 的这一口没有。

**顺带记一件 main 上后来长出来的东西**，免得下一个人拿它当 0059 的违例：`internal/partycommercial/adapters/postgres/declaration_publication.go`（syn-wall-door-audit 票 03）给六族声明表补了写入半边，其中含 `SaveAcceptanceRulePackage`、`SaveIntakeQualification`、`SaveFinalRule`、`SaveCancellationAuthority`。它挂在 `CommercialPublications` 上实现 `ports.PublicationRegistry`，**不在装载口上**——ADR-0059 说的是装载口不提供 Save，这一条仍然成立。

### 本票不答的

不答父清单第二节那个「八个切片机制半边全数达标是否仍站得住」。本票只覆盖 PC 侧两笔，另外三笔（ADR-0061 / ADR-0014 / ADR-0068）不在本票范围，不替它们推断。

### 「查不到」的那半句

- 真查过：`AcceptanceRulePackageContentView` 与 `LoadAcceptanceRulePackage` 的调用方在全仓 `.go` 上只命中端口声明、适配器自证与测试；四张声明表的表名在全仓（不限 `.go`）只命中迁移、PC 适配器与其测试、`cmd/parcel-dispatch` 的合成种子测试，以及若干 `.scratch` 票面。
- 没查、也不打算在本票查：`apps/admin-web` 侧是否有页面按别的路径展示这份正文。它即便有也不构成 Go 侧执行器，且不在本票路径上。

## Comments

- 2026-09-01 MCP-2：立票并派 MCP-4。
- 2026-09-01 MCP-4：只读取证完成，答案追加。核心更正一条：这两笔在 `git cherry` 上报 `+` 是**换基座重放**（引入 main 的是 `6e4ccda` 与 `74a6c35`，标题逐字同、stat 逐行同、blob 除累积型 `ports.go` 外全同），不是活没进 main。两条 ADR 的 Decision 在 main 上逐条有执行器；唯一要读窄一格的是 ADR-0059——四件执行器都在，但 `LoadAcceptanceRulePackage` 无生产调用方，与该 ADR 自己写明的显式代价一致。零代码改动，未合并、未拆树、未删分支、未提交。
