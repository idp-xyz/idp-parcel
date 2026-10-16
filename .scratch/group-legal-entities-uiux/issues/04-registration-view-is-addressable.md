# 04 — 登记视图有自己的地址

**Parent:** [spec](../spec.md)

**What to build:** 现在点页头的「登记责任法人」，只是切换了页面内部的状态，带来三个问题：
- 刷新后回到列表，填了一半的草稿全部丢掉；
- 浏览器后退回不到表单；
- 法人对象页却有自己的地址和标签页，同一个模块用了两种导航方式。

本票让登记视图能通过地址直接到达：
- 进入登记视图时地址随之改变，刷新后仍停在登记视图；
- 浏览器后退回到列表，列表的检索、筛选和排序都保留（沿用票 [admin-web-workspace-form/06](../../admin-web-workspace-form/issues/06-list-detail-roundtrip-keeps-page-state.md) 的往返保持）。

**约束：** 新地址不能和对象页地址冲突。对象页地址的第二段是法人标识，而法人标识是自由文本，所以不能拿一个保留词去占第二段。

**草稿保护：** 草稿里有内容时，离开前要先确认，包括返回列表、关闭标签页和刷新。草稿为空时不打扰。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** resolved

- [x] 登记视图有能直接打开的地址；刷新后仍在登记视图
- [x] 浏览器后退回到列表，列表状态都保留
- [x] 任意法人标识的对象页地址都不受影响，包括与新地址关键字同名的标识
- [x] 草稿非空时离开会先确认；草稿为空时不弹确认
- [x] 用 dom-probe 证明事件路径；三道门退 0

## 完成记录（2026-10-16，作者会话，前端切片直接在 main 上做）

- **落点：** 代码与票面认领同在 `f66f2b1e`。
- **改了什么：**
  - 新模块 `group-legal-entities-address.ts`：`groupLegalEntitiesViewOf` 从地址判出列表、登记视图或对象页，对象地址先认；`registerViewHash` 负责进出登记视图时的地址。
  - 页面的视图由 `hash` 推出，不再存在组件状态里。进入登记视图压一条历史；页内「返回列表」在本次挂载压过那条时用后退，否则用 `location.replace`。
  - `legal-entity-form.ts` 新增 `legalEntityDraftUnsaved`。表单经 `onUnsavedChange` 把它报给页面，并记住最近一次逐字段落册送出的草稿：落册后没再改不算未提交。
  - 草稿未提交时，「返回列表」弹 `ConfirmDialog`（「继续填写 / 离开」），刷新与关页由 `beforeunload` 拦。
  - 地址键用 `register`，不用 `view`：`view` 已归保存视图。
- **判据：** 用真浏览器（Playwright Chromium，演示租户）代替 dom-probe 证事件路径。真浏览器有真实的历史栈与原生 `beforeunload` 对话框，happy-dom 里证不了这两件。逐项结果：
  - ✅ 进入登记视图后地址为 `#/group-legal-entities?register=1`；直接打开这个地址，登录后落在登记视图；刷新后（令牌只在内存，刷新会重走一次登录）仍在登记视图。
  - ✅ 浏览器后退回列表，检索框里的「SYN」还在。
  - ✅ 法人标识 `register` 的对象页：`#/group-legal-entities/register` 打开的是对象页（答「册上没有这个法人」），不是登记视图。node:test 另钉了 `/register?register=1` 仍是对象页、`%E0` 这种畸形地址退回列表。
  - ✅ 有草稿时点「返回列表」弹确认；选「继续填写」留在登记视图，草稿还在；选「离开」回到列表，地址回到 `#/group-legal-entities`。空草稿点「返回列表」直接回列表，不弹。有草稿关页时浏览器弹 `beforeunload`；空草稿关页不弹。
  - ✅ 三道门：`tsc -b --noEmit` 退 0，`run-tests` 527/527（新增 5 条），`vite build` 退 0。
- **评审：** 改动全在 `pages/**`，对 `templates/address-query.ts` 只读不改，自查判据。
- **已知边界：**
  - 浏览器后退离开登记视图不拦，草稿丢弃，与改动前一样。要拦就得改写历史栈，而壳层靠 hashchange 记每张标签的地址，改写会让两边对不上。
  - 壳层标签页的 × 关闭也不拦。那是 `shell/` 的事，不在本票范围。
