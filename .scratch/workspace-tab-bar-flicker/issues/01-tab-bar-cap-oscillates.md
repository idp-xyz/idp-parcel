# 01 顶部标签条在标签多到某个区间时来回闪烁

Category: bug
Status: in-progress
出处：2026-11-10 用户截图问「上方的 tab 为什么打开多了以后会一直闪烁」。

## 原因

标签条是随仓 vendored 的 `@idpxyz/ui-workspace` 0.1.25 的 `EditorGroup`。它在「能左右滚动」（`canScrollLeft || canScrollRight`）时给每个标签加 `max-w-[160px]` 收窄。收窄之后标签都放得下，滚不动了，于是判为不溢出、去掉收窄；标签按原宽排回去又放不下，再收窄——一个反馈环。开着的标签恰好落在「不收窄放不下、收窄放得下」的区间里时整条来回翻，就是闪烁；标签再多一些、收窄后也放不下时又稳了，所以看起来是「打开多了以后」才闪。

## 修法

收窄与否改按各标签的**自然宽度**合计判：每个标签的 `offsetWidth` 加上它截断标签文字被藏住的那一截（`scrollWidth - clientWidth`），只数标签、末尾的弹性占位块只算最小宽度 20px，合计超过标签条的 `clientWidth` 才收窄；尺寸监听除标签条外也盯每个标签。收窄不改变自然宽度，结论就不会被收窄本身推翻；左右滚动箭头只管滚动。用 pnpm 补丁带着（`apps/admin-web/patches/@idpxyz__ui-workspace@0.1.25.patch`，`pnpm.patchedDependencies` 登记），`dist` 与 `src` 两份同改；README「技术栈」写明这是上游缺陷、换版本时怎么处理。

## 完成判据

- 浏览器在 1920 宽上逐个打开标签，每次连取十二次标签宽度：修前在闪烁区间里取到两种布局，修后每个标签数都只有一种。
- 开到收窄之后再逐个关回去，标签放得下时要恢复原宽（不收窄、文字不截断）。
- 全新检出 `pnpm install --frozen-lockfile` 打得上补丁；三道门退 0。
