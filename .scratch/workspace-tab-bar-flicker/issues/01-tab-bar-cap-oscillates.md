# 01 顶部标签条在标签多到某个区间时来回闪烁

Category: bug
Status: resolved——2026-11-10 进 main（见文末完成记录）
出处：2026-11-10 用户截图问「上方的 tab 为什么打开多了以后会一直闪烁」。

## 原因

标签条是随仓 vendored 的 `@idpxyz/ui-workspace` 0.1.25 的 `EditorGroup`。它在「能左右滚动」（`canScrollLeft || canScrollRight`）时给每个标签加 `max-w-[160px]` 收窄。收窄之后标签都放得下，滚不动了，于是判为不溢出、去掉收窄；标签按原宽排回去又放不下，再收窄——一个反馈环。开着的标签恰好落在「不收窄放不下、收窄放得下」的区间里时整条来回翻，就是闪烁；标签再多一些、收窄后也放不下时又稳了，所以看起来是「打开多了以后」才闪。

## 修法

收窄与否改按各标签的**自然宽度**合计判：每个标签的 `offsetWidth` 加上它截断标签文字被藏住的那一截（`scrollWidth - clientWidth`），只数标签、末尾的弹性占位块只算最小宽度 20px，合计超过标签条的 `clientWidth` 才收窄；尺寸监听除标签条外也盯每个标签。收窄不改变自然宽度，结论就不会被收窄本身推翻；左右滚动箭头只管滚动。用 pnpm 补丁带着（`apps/admin-web/patches/@idpxyz__ui-workspace@0.1.25.patch`，`pnpm.patchedDependencies` 登记），`dist` 与 `src` 两份同改；README「技术栈」写明这是上游缺陷、换版本时怎么处理。

## 完成判据

- 浏览器在 1920 宽上逐个打开标签，每次连取十二次标签宽度：修前在闪烁区间里取到两种布局，修后每个标签数都只有一种。
- 开到收窄之后再逐个关回去，标签放得下时要恢复原宽（不收窄、文字不截断）。
- 全新检出 `pnpm install --frozen-lockfile` 打得上补丁；三道门退 0。

## 完成记录（2026-11-10，直接推 `main`）

- 落点：`apps/admin-web/patches/@idpxyz__ui-workspace@0.1.25.patch`（`dist/index.js` 与 `src/EditorGroup.tsx` 同改，`src` 保留上游 CRLF），`package.json` 的 `pnpm.patchedDependencies` 登记，lockfile 随之更新；`.gitattributes` 让 `apps/admin-web/patches/**` 按原字节保存；README「技术栈」写明补丁来由与换版本时的处理。`pnpm patch` 要从注册表取包、本机没有凭据，补丁是对随仓 tarball 的两份副本 `git diff --no-index` 手工生成的。
- ✅ 浏览器（本树起 vite 5236 `--force` 接演示 API，1920 宽）：修前（5173 未打补丁）开到第 9、10 个标签时一秒内取到两种布局；修后开 1 到 11 个标签每步只有一种；再从 11 个逐个关回去，9 个及以上保持收窄，8 个及以下恢复原宽、标签文字不再截断。
- ✅ 全新检出（`git worktree add` 到 HEAD）`pnpm install --frozen-lockfile --offline` 打上补丁；三道门退 0。
- ✅ 非作者评审：首轮阻断一条（自然宽度把末尾弹性占位块也算进去，它吃掉收窄空出的宽度，标签一收窄就再也放不开——关标签时回归），另有非阻断（尺寸监听只盯标签条、`src` 整文件因行尾被重写、注释用了英文），全部修正；复评无阻断，在 Chromium 上对 4–15 个标签、500–2400px 宽、三种缩放共十八万余种组合做不动点模拟无一来回翻；`.gitattributes` 注释理由与匹配范围两条非阻断随之改正。
- 未验：Firefox 逐个取整宽度，标签很多时合计可能差过 1px 的容差；管理台今天以 Chromium 为准。修复应回到上游 idp-ui，换 `ui-workspace` 版本时照 README 处理补丁。
