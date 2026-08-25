# 07 admin-web 七页接线

Category: feature
Status: draft
Owner: MCP-2
Blocked by: 06

七页从 UnwiredModule 骨架转真页:api 层补七个查询函数(形状收编各端点 JSON)、
presentation 词表补枚举中文化、七个页面组件(表格/空态/错误态/未配置态,风格与
已接线页同款)、page-registry liveIds 各加一行。

共享文件纪律:page-registry.tsx 与 main.tsx 上 MCP-1(追踪页票 03)在途,只加自己行、
锚自己写的字、改后重读;若其未提交先落,等其落库再动。

## 完成标准

`npm run build`(或 typecheck)零错;本地起 dev 对着真端点手验七页未配置态与
(灌种子后)数据态;自己提交,票面 resolved + SHA,回频道 2。

## Comments
