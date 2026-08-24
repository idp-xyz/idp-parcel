# 03 追踪页前端接线(MCP-5,blocked on 02)

Category: enhancement
Status: open

地盘、阻塞边与纪律见 ../spec.md。开工条件:票 02 报出 SHA。

## 活

1. **TrackingProjectionPage 接线**(apps/admin-web/src/pages/visibility/):照委托查阅页既定模式——api 收编票 02 端点形状(以 adapters/http 传输层为准)、presentation 收编结果词表;列保持投影并行维度原词(现有列形状即 GLOSSARY「追踪摘要」禁统一状态机那条规则的落点,不折列)。
2. **未配置态如实呈现**:PAR-INT-01 未登记前端点答 403 + ACCESS_CHANNEL_NOT_CONFIGURED,按 ADR-0055 是诚实答案非接线缺陷;viewState 参照 ShipmentRequestListPage 的同档呈现,facts 三段式写明放行条件(渠道参数登记)。
3. **裁决闸注释更新**:页面里「运营查阅作用域是否复用客户隔离读口尚未裁决」的注释与 viewState 文案已过期——裁决已落,改为引 ADR 编号(引符号名/文档名不引行号)。
4. **liveIds 自落** 'tracking-projection' 行:占号→只加自己行→释号广播。
5. **自查**:apps/admin-web 下 pnpm exec tsc --noEmit;票面更新;admin-web-uiux-20260824 票 03/04 里的裁决闸记录若需回填结论,只补一行注记不改历史。

## 完成标准

已接线模块升至 4(提交与撤回、委托查阅、取消包裹、全程追踪),tsc 绿,send_to_session 1 报 SHA;票 04 以该 SHA 为开工条件。
