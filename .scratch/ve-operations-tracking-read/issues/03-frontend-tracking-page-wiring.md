# 03 追踪页前端接线(MCP-5,blocked on 02)

Category: enhancement
Status: resolved

地盘、阻塞边与纪律见 ../spec.md。开工条件:票 02 报出 SHA(已满足:267cb44,见票 02 决议)。

接手声明:原派 MCP-5;票 02 于 2026-08-24 收口并通知后,本票至 2026-08-25 10:41 仍
Status: open、5 号通道无占号动静。用户 2026-08-25 10:33 经 IDP 队列指示「直接开工」,
由 1 号(本票收口时的记账通道)接手施工,广播随占号一并发出。

## 活

1. **TrackingProjectionPage 接线**(apps/admin-web/src/pages/visibility/):照委托查阅页既定模式——api 收编票 02 端点形状(以 adapters/http 传输层为准)、presentation 收编结果词表;列保持投影并行维度原词(现有列形状即 GLOSSARY「追踪摘要」禁统一状态机那条规则的落点,不折列)。
2. **未配置态如实呈现**:PAR-INT-01 未登记前端点答 403 + ACCESS_CHANNEL_NOT_CONFIGURED,按 ADR-0055 是诚实答案非接线缺陷;viewState 参照 ShipmentRequestListPage 的同档呈现,facts 三段式写明放行条件(渠道参数登记)。
3. **裁决闸注释更新**:页面里「运营查阅作用域是否复用客户隔离读口尚未裁决」的注释与 viewState 文案已过期——裁决已落,改为引 ADR 编号(引符号名/文档名不引行号)。
4. **liveIds 自落** 'tracking-projection' 行:占号→只加自己行→释号广播。
5. **自查**:apps/admin-web 下 pnpm exec tsc --noEmit;票面更新;admin-web-uiux-20260824 票 03/04 里的裁决闸记录若需回填结论,只补一行注记不改历史。

## 完成标准

已接线模块升至 4(提交与撤回、委托查阅、取消包裹、全程追踪),tsc 绿,send_to_session 1 报 SHA;票 04 以该 SHA 为开工条件。

## 决议

- 落库 SHA:ef6e154(6 文件:visibility/api.ts 镜像三读法与五格判别、presentation.ts
  源上下文五词+传输错误码、TrackingProjectionPage 接线版、index.ts 出口、main.tsx
  前缀注入行、page-registry liveIds 第 4 行)。
- 列形取舍(活 1「列保持并行维度原词」的落地口径):传输层把维度派生交给读侧
  「按条目的来源与类型」,而 kind 是源上下文拥有的开放词表,kind→物流进展/位置或
  控制范围/交付或退运进展的对照与标准里程碑映射同族(版本化登记、实例半边),未
  登记前页面按源上下文分列(传输层封闭五元,列词取 CONTEXT「全程追踪投影」定义句
  的五源,零发明),五源+里程碑并列不折——GLOSSARY 禁统一状态机那条仍是表形出处;
  ETA/可见性缺口/异常影响/追踪摘要是读模型未携带的独立对象,按委托查阅页先例
  「栏目跟真实读模型走,不虚构」未上列,等各自读面落地再回。
- 活 2/3/4 照做:未配置态 facts 三段式写 PAR-INT-01 放行条件;过期裁决闸注释换成
  引 ADR-0076(引记录名不引行号);liveIds 自落一行未动邻行(占号广播 2026-08-25
  10:44,随本收口释号)。活 5:tsc --noEmit 零输出(2026-08-25 10:45,树上另有三份
  他人死现场改动在场但不在编译障碍);admin-web-uiux-20260824 票 03/04 已各补一行
  结论注记,未改历史。
- 1 号即本施工通道,简报经 IDP 队列 reply 呈报,不再自发 send_to_session。
- 票 04 以 ef6e154 为开工条件。
