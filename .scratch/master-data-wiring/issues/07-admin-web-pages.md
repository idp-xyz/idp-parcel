# 07 admin-web 七页接线

Category: feature
Status: in-progress
Owner: MCP-6(2026-08-25 13:02 改派;原派 MCP-2 自 11:40 后无提交、截至 13:02 未响应,用户经通道 3 授权 MCP-3 调度本轮)
Blocked by: 06(仅约束路径绑定、liveIds 登记与手验数据态;先行区见 Comments,不阻)

七页从 UnwiredModule 骨架转真页:api 层补七个查询函数(形状收编各端点 JSON)、
presentation 词表补枚举中文化、七个页面组件(表格/空态/错误态/未配置态,风格与
已接线页同款)、page-registry liveIds 各加一行。

共享文件纪律:page-registry.tsx 与 main.tsx 由本票占号,只加自己行、锚自己写的字、改后重读。

## 完成标准

`pnpm build`(或 typecheck)零错;本地起 dev 对着真端点手验七页未配置态与
(灌种子后)数据态;自己提交,票面 resolved + SHA,回频道 3。

## Comments

- 2026-08-25 13:02 MCP-3(调度):三点更新。①**先行区**:七个页面组件与 api 层新增
  查询函数是新文件/自有行,不依赖票 06,可立即开工;fetch 路径先按各票 http 文件的
  建议路径写成常量,等 MCP-4 广播最终路径表后对一遍再绑死。②票面「MCP-1 在途」
  提示已过时:其票 03 已收口释号(ef6e154),page-registry.tsx 与 main.tsx 当前无
  在途占用,改前自行广播占号即可。③本轮完工报告改回**频道 3**。风格参照已接线的
  tracking-projection 页(pages/visibility/)。
- 2026-08-25 MCP-6:认领开工。路径绑 MCP-4 终表(六 GET;network 两页共
  `/network-catalog`)。`page-registry.tsx`/`main.tsx` 已占号。
- 2026-08-25 MCP-3 七项裁决(摘要点留底):总则——骨架占位列向已提交读面收敛;读面有
  骨架无→上列(CONTEXT 原词中文化,JSON 字段名不改);骨架有读面无→不上列,用如实说明;
  展示层合成(并列/截断)允许;过滤只在已取回数据上。①价卡全字段上列。②参考序列同,
  口径可合显 id@version、更正合显 prior+依据。③目录页 chip 六族不露 service-area。
  ④服务区域覆盖关系保留为「尚不存在」说明态(PAR-NET-14),不发请求。⑤合规双
  registry chip+两套列。⑥服务产品只列版本壳,映射/授权不上列并说明。⑦商业策略按
  kind chip 换列;信用策略不上列并说明。
