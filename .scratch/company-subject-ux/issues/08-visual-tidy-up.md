# 08 视觉收拾：详情栏与对象页一套版式、去重复、宽屏不散

Category: enhancement
Status: resolved——2026-11-03 进 main（记录见文末）
父票：[spec](../spec.md)
Blocked by: 无
出处：用户 2026-11-03 看 2000px 宽屏下的列表 + 详情栏截图，认为「显示的专业性不够，有点乱」；协调方诊断见下。

## 诊断（协调方）

信息没错，乱在**同一块面上叠了太多种呈现手法**、**同一事实出现两次**、**宽屏没有宽度纪律**：

1. 详情栏从上到下换了七种手法：标识胶囊 → 全宽生命周期线 → 三张大指标卡 → 签 → 「资料状态」药丸（与指标卡重复）→ 带长横线的分节标题 + 标签 / 值网格 → 分号串起来的监管登记号 → 收款账户表格。读的人每换一段就要换一种读法。
2. 宽屏下（详情栏约 850px）：生命周期线拉成一条横贯的长线、两端各一个点；指标卡里的值只占卡的一角；标签列与值列之间空一大片；地址下另起一行「中国（CN）」颜色像链接。
3. 「监管登记号」的值是「海关注册编码 0000000001；合成税务登记号（…）SYN-CN-TAX-0001；纳税人识别号 9100…」一长串，扫不出来；标签本身还折了行。
4. 列表在宽屏下前两列过宽、状态与日期挤在右侧；「注册地与注册号」里类型名（演示目录的长名）比号还显眼。

## 要做的（详情栏与对象页同一套；对象页头区照模板，只改本页交给模板的内容）

1. **头区收成两行**：第一行名称 + 状态药丸；第二行一行小字 meta：`编码 SYN-LE-01 · 注册地 中国（CN） · 注册号 统一社会信用代码 9100…`（纯文本，不再是三枚胶囊；号可点复制照旧）。生命周期条改成紧凑的三段小步进（点 + 词，不拉长线），放头区右侧或 meta 行末尾；对象页头区同一形。
2. **指标卡撤掉，换一行关键事实**：用既有 `FactStrip` / `Fact`（`detail-primitives.tsx`）那种小标签 + 值的一行：`生效 2026-01-02 · 资料 ●可开票 · 第 5 版 · 被引用 6 份`；对象页不再传 `summary` 指标卡（模板允许不传），同一行事实放对象页头区下方。
3. **去重复**：概要签顶部那枚「资料状态：可开票」药丸撤掉（已在事实行），只留一行「当前资料 第 5 版 · 2026-11-01 起生效 · 来源单据 …」作为读出的小标题。
4. **宽度纪律**：详情栏与对象页正文区 `max-w-[52rem]` 左对齐；标签 / 值网格统一固定标签列宽（`grid-cols-[7.5rem_1fr]`），行距 6–8px，标签不折行（长标签改短：「监管登记号」，「税务 / 海关」进 title）；分节标题 13px 半粗、上边距 20px、下边距 8px，节与节之间只用一条浅色细线或只用间距，不再每节一条长横线；两列布局只在正文宽度 ≥ 44rem 时启用，否则单列。
5. **可扫的小表代替串**：监管登记号、其他语言的抬头各用两列小列表（类型名 → 号；语言 → 抬头），等宽字体只给号；地址的国家并入地址文本末尾（`· 中国`，同色弱显），不另起一行、不用强调色。收款账户表照旧但列头与正文字号与上面网格一致。
6. **列表宽屏**：公司主体列 `max-w-[22rem]`，注册地与注册号列显 `号` 为主、类型名小字在下（与名称 / 编码两层同形），状态与生效日期列靠近内容不贴右边缘（表格整体 `max-w` 或让最后一列不 `flex-1`）——看 `WorkbenchPageTemplate` 允许哪种，不改模板。
7. 用语照 spec 表；纯函数（若有新判读）钉 node:test；三道门；浏览器在 1440 与 2000 两种宽度各截一张详情栏与对象页核对。

## 不做

- 不改 `templates/*`、`components/*`、`shell/*`（壳层顶部工作区标签过多是会话状态，不在本票）。
- 不动数据与后端。

## 完成判据

- 三道门退 0；1440 与 2000 宽度下详情栏：头区两行、事实一行、无指标卡、无重复药丸、正文不超 52rem、监管登记号与抬头为两列小表、地址国家在行末；对象页同形；列表状态与日期列不再贴右边缘。

## 完成记录（2026-11-03，分支 `claude-csux-08`，一条一笔共六笔）

**落点**（都在 `apps/admin-web/src/pages/party/`）：

- 第 1 条 `LegalEntityHeaderParts.tsx`（`LegalEntityLifecycle` 改成按内容定宽的一行小步进）、`LegalEntityDetailPanel.tsx`（头区两行）、`LegalEntityDetailPage.tsx`（`buildMeta`：编码进 meta、`description` 不再传、生命周期作 meta 末格）、`detail-primitives.tsx`（`RefText` 纯文本复制、`Fact` 加 `inline` 形）。
- 第 2 条 `LegalEntityHeaderParts.tsx`（`legalEntityHeaderFacts` 一处定义事实行）、`detail-primitives.tsx`（`FactStrip` 加 `inline` 形）、两处页面接上；`legal-entity-references.ts`（`profileStatusCellOf` 带出 `effectiveRevision`、`referencedCountText` 带单位「份」）+ 其 test。
- 第 3 条 `LegalEntityProfileSummary.tsx`。
- 第 4 条 `legal-entity-profile-readout.css`、`LegalEntityProfileReadout.tsx`（`ProfileReadoutField` 加 `title`）、`LegalEntityProfileContent.tsx`（`ProfileGroup`、标签「监管登记号」）、两处页面正文 `max-w-[52rem]`。
- 第 5 条 `LegalEntityProfileReadout.tsx`（`ProfileReadoutAddress` 国家并进行末、新件 `ProfileReadoutPairs`）、`LegalEntityProfileContent.tsx`；`legal-entity-profile-history.ts` 的 `profileAddressText` 同步改成同一行 + 其 test。
- 第 6 条 `GroupLegalEntitiesPage.tsx`（`columnsOf` 前两列 `width`、主列内容 `max-w-[22rem]`、`IdentityLayerCell` 号在上）。

**判据逐条**（浏览器核验：自起 vite `:5208` 对 `:19080`，playwright 1440×900 与 2000×1020，脚本断言零写请求；截图在 `/tmp/claude-1000/-home-tops-workspace-idp-parcel/70733dd4-0571-48b5-8660-e52d102ae60f/scratchpad/csux08/after-*.png`）：

- ✅ 三道门：`tsc -b --noEmit` 退 0、`run-tests.mjs` 1163 通过 0 失败、`vite build` 退 0。
- ✅ 头区两行：详情栏第一行名称 + 状态，第二行 `编码 · 注册地 · 注册号` 纯文本（号点按复制照旧）+ 行末小步进；对象页第一行照模板，第二行 meta 同一串。
- ✅ 事实一行、无指标卡：`生效 · 资料状态 · 资料版本 · 被引用`（已停用时多一格停用）；详情栏是 `FactStrip inline` 一行，对象页 `summary` 不再传。
- ✅ 无重复药丸：概要签顶部只剩「当前资料 第 N 版 · 某日起生效 · 来源单据 …」一行。
- ✅ 正文不超 52rem：量到详情栏读出 832px（2000 宽，栏 879px）、对象页读出 832px；1440 宽详情栏 559px 单列，两列只在 ≥ 44rem 启用。
- ✅ 监管登记号与抬头两列小表、地址国家在行末弱显。
- ✅ 列表：2000 宽无栏时五列 352 / 339 / 326 / 349 / 393px，状态与日期靠左紧跟前一列；有栏时状态列 189px 不贴边。
- ◑ 对象页「事实一行放头区下方」：模板在头区与签之间只有指标卡这一格，事实行接在模板 meta 行后面（1440 宽折成两行），不是头区下另起一行——放进概要签又会与「当前资料 第 N 版」挨着重复；要不要动模板给一格，留协调方裁。

**未验**：

- 已停用 / 待补行的详情栏与对象页（演示数据里没有停用的公司主体）；`legalEntityHeaderFacts` 的「停用」格只过了类型检查。
- 往来单位详情栏与对象页借同一条 `LegalEntityLifecycle`，随之变紧凑：详情栏 1440 宽看过一眼（`bp-list-panel-1440.png`）无异常，对象页指标卡里那一格未截图。
- 版本历史对比表的地址改成同一行只过了 node:test，未开「资料版本」签看。

## 进 main 记录（2026-11-03，协调方）

- main `0531b1b78` 上 `git merge --no-ff origin/claude-csux-08`（tip `e08523e46`），无冲突。三道门在 main 上退 0（tsc、run-tests 1163、vite build）。协调方看过 2000 宽的详情栏与对象页截图（`scratchpad/csux08/after-*.png`）。
- 作者请裁：① 对象页事实行放模板 meta 行，不动模板——接受；② 事实行用 spec 词「资料状态」「资料版本 第 N 版」、国家显「中国（CN）」——接受；③ 往来单位两页随 `LegalEntityLifecycle` 变紧凑——接受，两处同形；④ `demo_live` 上 SYN-LE-01 的第 6 版（来源单据写的是税号、海关编码改成 3101234567、生效当天）不是任何票的种子，是有人在预览上用配置员账号真保存了一次——演示库本就是给人试的，不回滚。
