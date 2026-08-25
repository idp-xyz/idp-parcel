# 07 admin-web 七页接线

Category: feature
Status: resolved(253b449;封存 8cb43e4;验证种类见 Comments 末条)
Owner: WSL 队列频道 3(2026-08-25 18:02 改派;沿革见 Comments)
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
- 2026-08-25 18:02 WSL 队列频道 1(本轮调度):本票改派 WSL 队列频道 3。证据:树上 13 份
  未提交文件 mtime 止于 15:15:02(page-registry.tsx 最新),自封存 ebe9b04(14:46)后无新
  提交,截至 17:57 无动静;用户 18:00 前后经 WSL 队列频道 1 指示「启动 idp-parcel 实施」。
  新主第一步按 parallel-sessions.md 把现行未提交增量原样封存(一字不改、另起一笔、提交
  信写 mtime 证据),再在其上续工。完工报告改报 WSL 队列频道 1(上一轮「改报频道 3」口径
  随该轮结束作废)。
- 2026-08-25 18:40 WSL 队列频道 3(收口):封存与余量两笔,验证四种,数据态留集成轮。
  ①**封存** 8cb43e4:原主 13 份未提交增量原样入库(mtime 15:08:41–15:15:02 实测)。核对
  发现该增量已把 ebe9b04 时的 54 个 tsc 错清零:七页组件、四上下文 api/presentation、
  liveIds 七行与 main.tsx 前缀配置全部就位,六 GET 路径与 endpoints.go 装配表逐一相符,
  前端类型与四上下文传输层 json 字段名/outcome 串逐字段核对一致,七项裁决逐条对照落实。
  ②**余量** 253b449:五页(网络目录/服务区域/合规规则/服务产品/商业策略)过滤条计数摘要
  加 outcome 守卫——未配置态原样显示「0 个版本/0 条」,与状态区「这不是目录为空」矛盾;
  pricing 两页与全程追踪页本就有该守卫,补齐同款。③**验证**(WSL,node 20.18.2/pnpm
  10.32.1/go 1.26.5):tsc --noEmit 零输出;vite build 成功(chunk 体积告警为既有 advisory);
  curl 直连与经 vite 代理原路各打六端点(含 family/registry/kind 参数变体)全部 403
  ACCESS_CHANNEL_NOT_CONFIGURED;Edge 无头(--virtual-time-budget=8000)逐页 dump-dom
  验七页:未配置态标题/端点坐标/错误码/解锁条件在场,守卫后「0 个版本/0 条」不再出现。
  PAR-INT-01 未登记时 403 是诚实答案,不是缺陷。④**数据态留集成轮**,受阻原因两层:
  种子未就绪只是浅层(票 08 票面 17:44 记录:主库业务表零张、迁移 schema 不存在);更深
  一层按频道 7 勘察(journey-draft.md,5703b4d):七查阅端点现装配位于 UnconfiguredIntake
  之后,放行 Intake 只存在于测试替身(grantedCatalogueIntake),vite 代理无旁路——种子
  先到数据态也取不到数。数据态验证等「隔离环境读面准入」裁决(不归本票),留集成验证轮;
  本票不自造旁路、不 mock、不绕 Intake(频道 1 于 18:4X 指示,验证完成线即③所列)。
  ⑤环境注记,后人复现要紧:(a)本机 @idpxyz 六包**从来不是**从 npm.pkg.github.com 装的
  ——store 索引名显示源为本地 tarball(原在 C:\Users\topsx\AppData\Local\Temp\idp-ui-tgz,
  已复制到 WSL ~/idp-ui-tgz 防 Temp 清理);WSL 侧重装 node_modules 需临时在 package.json
  加 pnpm.overrides 指向 tarball,装完还原(本轮即如此,package.json 未入库改动)。由此新生
  的 apps/admin-web/pnpm-lock.yaml 含 file:/home/tops 机器路径,留作未跟踪产物不入库。
  (b)本机 8080 被 Windows svchost(PID 3516,镜像网络)占用,手验用 IDP_PARCEL_HTTP_ADDR
  =:18080 起 parcel-api、PARCEL_API_TARGET 指给 vite 代理。(c)手验库用遗留测试库
  parcel_test_52868_3_08c9f7dcd532(含全 schema,CheckSchema 可过;未配置 Intake 拒在读面
  之前,库内容不参与),未动频道 5 作业中的 postgres 主库;手验完 api/dev 进程均已停,防
  「api 绑错库」在集成轮被当成已接好。⑥共享文件本轮未改:page-registry.tsx/main.tsx
  接线在封存增量内已完整,无需新占号。.go/.sql 零改动。
