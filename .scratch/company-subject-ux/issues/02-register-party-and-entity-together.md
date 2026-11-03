# 02 建档一体化：新增公司主体时一并登记往来单位；往来单位列表标出「我方公司」

Category: enhancement
Status: resolved——2026-11-03 进 main（记录见文末）
父票：[spec](../spec.md)
Blocked by: 无
出处：评估「一、概念模型与路径」第 1 条。

## 现状

建一家我们自己的公司要走三步两模块：`合作伙伴 › 往来单位 › 新增`（与客户、承运商并列，类型显示「往来单位」）→ `设置 › 公司主体 › 新增`（必填「对应的往来单位 *」）→ 详情里补资料。写口是两条：`POST /commercial-business-party-registrations`、`POST /commercial-legal-entity-registrations`（法人登记要求参与方在法人生效时点已生效）。

## 要做的

1. **新增公司主体表单**（`LegalEntityRegistrationForm.tsx`、`legal-entity-form.ts`）：
   - 「对应的往来单位」改成一个二选一：**「这家公司还没有往来单位档案，一并登记」**（缺省，当下拉里选不到时）或 **「已有往来单位档案：选择」**（下拉照旧）。一并登记时展开一组格：公司名称 *、往来单位编码（缺省取公司主体编码，可改）；来源单据与生效时间与法人共用这一笔的（往来单位生效时间 = 法人生效时间，保证法人生效时参与方已生效）。
   - 保存时串行两步：先登记往来单位，答 `REGISTERED`（或「已在册」同形答复，照 `partyIdentityOutcomeLabels` 判）再登记法人。第一步成功、第二步失败时，表单保留全部输入、把往来单位那组锁成「已登记 ✓」、只重发第二步；第一步失败照常示出答复、不发第二步。整个过程不在本地判任何领域规则（伞票 admin-write-faces/07 硬句照旧）：两步各自原样送、原样示答复。
   - 修订号建议、JSON 镜像折叠区照旧；一并登记时 JSON 镜像只镜像法人那一步。
   - 用语照 spec 表（「这家公司是谁：编码、注册地、注册号」、「注册号」label 与说明、候选说明、JSON 折叠标题、页头副题）。
   - 页头副题与「返回列表」在 `GroupLegalEntitiesPage.tsx` 的登记视图里（票 01 拥有该文件）：你只改那两句文案，提交时在票面写明，协调方合并时解冲突。
2. **往来单位列表**（`BusinessPartiesPage.tsx`、`business-party-list.ts`）：「类型」列里，是本集团公司主体的参与方显示「我方公司」（页面已为全景取过 `listGroupLegalEntities`，没有就加载一次；判读纯函数进 `business-party-list.ts`）；筛选「全部类型」下拉加「我方公司」一项。往来单位对象页「全景」里「公司主体 1」那一行加链接到 `#/group-legal-entities/<法人标识>`（若 `PartyOverviewSection.tsx` 已有 RefLink 机制就用它；该文件不在你落点里，改一行要在票面写明）。
3. 纯函数（两步串行的状态机、往来单位缺省编码、「我方公司」判读）进 `.ts` 并钉 node:test。

## 不做

- 不加后端复合命令；不在本地校验「参与方须在法人生效时点已生效」之类的领域规则。
- 不动票 01、03、04 的落点（见 spec 子票表）。

## 完成判据

- 三道门退 0；状态机用例覆盖：两步都成、第一步失败、第二步失败后只重发第二步、往来单位「已在册」视为第一步成功。
- 浏览器在演示租户（一次性库或只点到保存前）核：新增页默认展开「一并登记」；选了已有往来单位时那组收起；提交路径用 `dom-probe.mjs` 或 node:test 证两步次序（不要在 `demo_live` 上真提交）。
- 往来单位列表里五家我方公司显示「我方公司」，筛选可选。

## 完成记录（2026-11-02，作者会话，分支 `claude-csux-02`，从 main `ae9a9a763` 起）

### 落点

| 提交 | 文件 | 做了什么 |
|---|---|---|
| `22b5b3ad7` | `legal-entity-form.ts`、`legal-entity-form.test.ts` | 草稿加档案来源二选一、公司名称、往来单位编码（缺省取公司主体编码）与第一步落册后钉住的编码；`companionPartyDraftOf` / `pairedSavePlan` / `partyStepLanded` / `runPairedSave` 四件纯函数；node:test 钉两步都成、第一步失败、第二步失败后只重发第二步、已在册视为第一步成功、选已有档案只发法人 |
| `717901c86` | `LegalEntityRegistrationForm.tsx`、`party-registration-fields.tsx`、`GroupLegalEntitiesPage.tsx` | 表单二选一与一并登记那组；`useRegistrationForm` 加 `sendVia`（壳只管锁与本册答复，表单把两步流程交进来，没有绕开壳）；用语照 spec 表。**`GroupLegalEntitiesPage.tsx` 是票 01 落点，只改了登记视图页头副题那一句**（「返回列表」spec 表没有改词，未动） |
| `af1d64a63` | `business-party-list.ts`、`business-party-list.test.ts`、`BusinessPartiesPage.tsx`、`PartyOverviewSection.tsx` | `ourCompanyPartyIds` 按法人册算（只数待生效 / 已生效的法人）、`businessPartyTypeLabel` 把「我方公司」排在角色前；筛选加 OURS、NONE 不再把我方公司算成「往来单位」、计数分出 `ours`；页面读一次法人册，刷新连它一起重取。**`PartyOverviewSection.tsx` 在落点之外**：只把「公司主体」段每行的法人标识从 `span` 换成指向 `#/group-legal-entities/<法人标识>` 的 `a`（该文件没有 RefLink 机制，RefLink 去的是带检索词的列表不是对象页） |

### 判据

- 三道门退 0 ✅（在 `af1d64a63` 上：`tsc -b --noEmit` 退 0；`run-tests.mjs` 1139 通过 0 失败；`vite build` 退 0。`git merge origin/main` 后 main 未动，`Already up to date`）。
- 状态机用例覆盖 ✅：`legal-entity-form.test.ts`「两步串行」五则——两步都成 / 第一步失败则第二步不发 / 第二步失败后再保存只重发第二步 / 已在册视为第一步成功 / 选已有档案只发法人；另钉落册判据只认 `REGISTERED` 与 `ALREADY_REGISTERED` 两格线上名。
- 浏览器（演示租户 `SYN-TENANT-01`，只点到保存前，全程监听写请求 = 0）✅：
  - 新增页默认展开「一并登记」、往来单位编码占位显公司主体编码 — `scratchpad/csux02/01-new-entity-together.png`
  - 选「已有往来单位档案」那组收起、下拉照旧（13 家超阈值即带搜索的选单）— `02-new-entity-existing.png`；切回去输入仍在
  - 往来单位列表类型列 — `03-parties-list.png`；筛「我方公司」得 5 行（上海 / 深圳 / 香港 / 美国 / 运营方一号），工具条「共 13 家单位，当前显示 5 家」— `04-parties-ours-filter.png`；下拉项：全部类型 | 我方公司 | 客户 | … | 往来单位
  - 往来单位对象页全景「公司主体 1」行的 `SYN-LE-CN-SH-01 r1` 是链接，点后到 `#/group-legal-entities/SYN-LE-CN-SH-01` — `05-party-overview-link.png`、`06-entity-object-page.png`
  - 脚本 `scratchpad/csux02/shots.js`、输出 `shots.log`（scratchpad = `/tmp/claude-1000/-home-tops-workspace-idp-parcel/70733dd4-0571-48b5-8660-e52d102ae60f/scratchpad`）
- 提交路径（不对 `demo_live` 真提交，`dom-probe.mjs` 对替身 fetch）✅：`scratchpad/csux02/probe-paired-save.tsx`，`ALL OK`——先 `POST /commercial-business-party-registrations` 后 `POST /commercial-legal-entity-registrations`；往来单位载荷编码取公司主体编码、名称裁空白、修订号 1、来源单据与生效时间与法人同值、无 `tenantId`；法人载荷钉刚登记成的编码；第二步答 `NOT_ACCEPTED` 后那组显「已登记 ✓」、单选与名称锁住、两步答复各显各处、不触发列表重取；改法人编码再保存只发第二步且仍钉原往来单位；第一步答 `CONTENT_CONFLICT` 时只发一枪、答复原样示出、那组不锁、保存钮解锁；再保存答 `ALREADY_REGISTERED` 第二步照发；选已有档案只发法人并钉选中的参与方。
- 往来单位列表五家我方公司显示「我方公司」✅，筛选可选 ✅。

### 未验 / 待协调方裁

- 真库上两步的真实答复没看过（只替身）；一并登记在 `demo_live` 上未真提交。
- 一并登记时往来单位修订号固定 1（这一组的语义是首笔）、不按册算下一修订——否则编码撞上已有档案会悄悄变成「更新它的名称」。编码已在册时表单只提示改选「已有往来单位档案」，拒不拒仍由服务端答。要改口径就改 `companionPartyDraftOf`。
- 「我方公司」只数待生效 / 已生效的法人，已停用的法人钉的参与方不算（与角色只数在用关系同一条判据）。
- 没动的两处（不在落点，票面也没要）：详情栏头部的类型胶囊（`BusinessPartyDetailPanel.tsx`）仍只按角色显；命令头指标没加「我方公司」一格。`往来单位` 指标数现已不含我方公司。
- `identifierNote` 里「已在册（…）」仍经 `identityRevisionLabel` 取词，票 01 改成「登记记录 · 第 N 版」时自动跟上；合并时若票 01 删了该函数要补一处。

## 进 main 记录（2026-11-03，协调方）

- main `eb4049625` 上 `git merge --no-ff origin/claude-csux-02`（tip `d830a0053`），无冲突；合并提交 `f427f62fd`。三道门在 main 上退 0（tsc、run-tests 1139 通过、vite build）。
- 作者请裁四条：一并登记的往来单位版本号固定 1——接受（撞编码时服务端答内容冲突，不会悄悄改名）；「我方公司」不含已停用法人——接受；详情栏类型胶囊与命令头指标未加「我方公司」——不要求；真库两步真实答复未看——以替身探针与既有两口的契约测试为准。
- 共享面：本票没动 `templates/*`、`shell/*`、`components/*`，不需评审。
