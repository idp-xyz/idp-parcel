# 02 名称更正 = 对象页编辑态

Category: enhancement
Status: in-progress
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

1. 地址：`#/business-parties/<id>?edit=identity` 即编辑态（`business-parties-address.ts`，照 group-legal-entities-address 的 profileEditHash 一套）。
2. 页头「更正名称」进编辑态：概要分区换成表单（名称、来源单据、生效时间；高级：更新序号；编码锁死显示），「保存 / 取消」经 portal 钉进模板页脚；落册后 toast、回显示态、重取参与方册。壳与草稿沿用 `useRegistrationForm` + `business-party-form.ts`，不另造载荷。
3. 离开守卫：返回列表与站内链接有改动先问。详情栏不加入口（名称更正低频），对象页一处即可。

## 完成判据

- 三道门退 0；地址判读有 node:test。
- 浏览器核：进编辑态、页脚保存、预览 / 答复、取消与返回列表的确认、深链刷新仍在编辑态、查看者无入口。
