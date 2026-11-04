# 05 渠道产品目录：行详情进右栏

Category: enhancement
Status: resolved——2026-11-04 进 main `3d44ed40b`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

照 spec 的决定 1–4，用票 01 抽出的 `VersionDetailPanel`、`VersionClosureDialog`、`RegisterView` 接线；本页特有的东西（表单、选择器、地址键）保持。

## 完成判据

- 三道门退 0。
- 浏览器核：右栏详情、登记视图进出与离开守卫、收尾对话框、查看者无写入口。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `fc956259f`，提交 `3d44ed40b`，CI run 41801684643 全绿）

- 落点：`ChannelProductCatalogPage.tsx` 整页改在 `WorkbenchPageTemplate` 上（右栏开时只留服务产品与渠道两列），行详情用 `VersionDetailPanel`，历史里点某笔修订详情换成那一笔；「设置产品渠道」是 `?tab=register` 的 `RegisterView`（`channel-product-catalog-address.ts` 与测试），进视图压历史、出视图后退或替换。`RegistrationPanel` 多一个可选的 `onUnsavedChange`：快照框里有与送出去那份不同的输入时报真，本页与商业规则页的 JSON 发布视图据此在「返回列表」前问一句、刷新关页也拦（`useUnsavedGuard`）。
- ✅ 三道门退 0：tsc、node:test、vite build；CI run 41801684643 全部 job success。
- ✅ 浏览器在演示租户核（playwright 只读探针，零写请求、零页面错误）：两行映射，点开右栏（服务产品 / 渠道 / 有效期 / 登记时间与技术详情、版本历史一笔），右栏开时列收起；页头「设置产品渠道」进登记视图、工作台不在，未改动返回不问；快照框填入后「返回列表」弹「离开这一页？」，「继续填写」留在视图且输入还在，「离开」回目录；深链 `?tab=register` 直落视图、返回用替换；商业规则页 JSON 视图同样的守卫生效。查看者深链落回目录、无「设置产品渠道」、右栏无动作。
- 评审：diff 碰了 `components/registration/RegistrationPanel.tsx`，按 workflow「前端切片」请非作者评审：APPROVE，三处建议已照改——被拒后改过的草稿仍算没提交（记送出去的那份比对）、面板卸载时报假、去掉注释里的跨文件序数。证据 S。
