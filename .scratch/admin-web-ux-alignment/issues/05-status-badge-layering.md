# 05 状态 badge 五层分家：`domainStatusTones` 加「层」轴，`StatusBadgeFor` 按层取形，词一个不改

Category: enhancement
Status: in-progress
Blocked by: 无
地盘：`apps/admin-web/src/domain/status.tsx`（+ 新 `domain/status.test.ts`）；用 `StatusBadgeFor` 的页面**只在渲染形状随层自动变时被动受影响，不逐页改**
（`shipment-request/*`、`visibility/ExceptionCasesPage`、`operations/TransportFulfillmentReviewPage`、`party/detail-primitives` 等，按 `git grep StatusBadgeFor` 为准）。
出处：spec「缺口」表第五档；黄金标准「状态语义黄金标准」（Lifecycle / SLA / Risk / Severity / Flags 五层必须在设计系统层预先分开，不许一张词→色表混装）；
手册「色彩系统规范」状态语义色统一那条（已守住，本票不动色）。

## 为什么

`domainStatusTones` 今天是一张扁平的「词 → 色调」表：它把「这是什么状态」和「该显什么颜色」压成一步，没有「这是哪一层的状态」这一格。眼下表里全是生命周期
与判断结论的词，混装还没露出来；追踪 ETA（SLA 层）、异常案件的严重度（Severity 层）、关务限制（Risk / Flags 层）三类词一旦进表——它们在 CONTEXT 里都
已经是概念，只是读口还没登记格（`ExceptionCasesPage` 头注明写「严重度、优先级与工作条件在案件行上无登记格」）——同一张表同一种徽章就分不出「已提交」
与「超时 2h」哪个该更醒目。黄金标准要求这一层在**设计系统层**预先分开，等词进来再拆就是改七张页。

## 要做的

1. **层轴**：`domain/status.tsx` 加 `StatusLayer = 'lifecycle' | 'sla' | 'risk' | 'severity' | 'flag'`（对应黄金标准五层，词用英文键、中文注释各层定义），
   加 `domainStatusLayers: Record<DomainStatus, StatusLayer>`——**每个既有词恰归一层**。今天表里的词全部归 `lifecycle`（生命周期状态与判断结论——
   「已接受 / 已拒绝 / 冲突 / 无适用依据 / 可达 / 不可达」等在 CONTEXT 里都是**该判断对象**的状态，不是对象上的风险标记，理由写进条目注释；
   `资料不足`、`待补充`、`要求补充` 这类「续办提示」也仍是所在对象的状态，不是 Flags）。其余四层今天没有词——**表里留层不留词**，不为了凑层造词。
2. **形随层**：抬一个内部渲染件 `LayeredStatusBadge({ layer, tone, children })`——`lifecycle` 仍是今天的 `StatusBadge`（填充徽章）；`sla` / `risk` /
   `severity` / `flag` 各定一种形（如 `Tag` 描边、带层前缀图标或首字标）；层 → 形的映射一张表、一处定义。`StatusBadgeFor` 改为查词得层与色调后调它，
   对外签名不变。四层今天无词，映射要写、类型要过，渲染路径由合成词直接调 `LayeredStatusBadge` 走一遍（见 4）——`StatusBadgeFor` 仍只收 `DomainStatus`。
3. **色调翻译不动**：`badgeStatusByTone` 与五档 `StatusTone` 定义原样；本票只加层不改色、不改词——状态词只取 CONTEXT 原词是本仓红线（spec「红线」第三条）。
4. **演示**：`pages/template-preview` 不在本票地盘（03 / 04 在改），改在 `domain/` 同目录放一份 `status-layers.demo.tsx`（合成 S）：五层各拿一个
   `SYN-` 前缀的合成词直接调 `LayeredStatusBadge` 渲染一次，给 `run-tests` 里的 `renderToStaticMarkup` 断言「五形各不同」用；合成词一个不进
   `domainStatusTones`。若 `tsconfig.test.json` 不编 `.tsx`（今天只 include `src/**/*.test.ts`），断言改在 `.test.ts` 里用 `createElement` 调它，不改 tsconfig。
5. **`party/detail-primitives.tsx` 的 `statusBadge(table, code)`**：它按词表查词再交 `StatusBadgeFor`，不改；只确认层轴加上后它仍编译、行为同。

## 不做

- 不给任何词改层以外的东西：不改词、不改色、不加词。
- 不做 SLA 计时 / 风险评分 / 严重度分级的**计算**——那是各上下文的领域规则，读口没有就没有。
- 不做「同一格叠多层徽章」的布局（追踪摘要行将来同时显三层）——等第一张真要同时显两层的页立票时再定叠法。

## 完成判据

- 三道门绿；既有 `run-tests` 零改动仍绿。
- `domain/status.test.ts`（新）钉：词表每个词在 `domainStatusLayers` 里恰有一层；层 → 形映射覆盖全部五层；`StatusBadgeFor` 对 `lifecycle` 词渲染的
  静态标记与改前逐字节同（先在改前录一份基线字串再改——这条是「七张页不受影响」的证据）。
- `domainStatusTones` 的键集零改动：`git diff` 该对象只有注释行——评审用 `git diff -U0 -- domain/status.tsx | grep "^[-+]  '"` 为空核。
- 浏览器验收做不到如实写「未验」。

## 裁决

1. **判断结论归 lifecycle 不另立「decision」层**：黄金标准只有五层，且 CONTEXT 把接受判断 / 商业解析 / 可达性都建成有自己生命周期的**对象**，其结论
   就是该对象的状态词；另立一层等于替设计系统改规范。
2. **留层不留词**：四层空着比塞「示例词」诚实；渲染路径的正确性由合成 S 演示词在测试里走一遍来保证，演示词不进 `domainStatusTones`。

## Comments

### 认领（2026-09-20 12:5x）

通道 8 认领后 crash、树上零提交（用户 12:5x 报）；推送方通道 1 拆其空树后自接，分支 `mcp1-ux05` 基 main `86a96ab7`，地盘 `domain/`。作者 = 推送方 = 本会话，
评审需另派。
