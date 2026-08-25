# 08 合成 S 主数据种子包

Category: feature
Status: resolved
Owner: WSL 队列频道 5(2026-08-25 18:02 改派接管;此前 MCP-5 13:02 认领后至 17:57 零提交,证据见 Comments;原派 MCP-2 自 11:40 后无提交,用户经通道 3 授权 MCP-3 调度上一轮)

一套隔离合成 S 主数据种子,经四个登记 CLI(parcel-pricing-register /
parcel-network-register / parcel-customs-register / parcel-commercial)灌入本机库,
让七页在 demo 环境有数据可看。

## 纪律(红线映射)

- 命名走合成口径(SYN- 前缀,对齐 PN-02 合成任务包),不影射任何真实企业。
- 证据层级只记 S;不进参数登记册、不改任何「待提供」状态。
- 种子 JSON + 运行脚本入库(可复现),但明确标注仅限隔离环境;不写生产默认值。

## 完成标准

脚本一键灌入干净库零报错;七页(票 07 后)可见种子数据;自己提交,票面 resolved + SHA。

## Comments

- 2026-08-25 13:02 MCP-3(调度):两点补充。①种子设计一并满足演示动线的「成套成
  故事」要求(用户已放行,见 `.scratch/product-story-and-demo/issues/04-demo-journey.md`):
  同一合成租户下,服务产品/价卡/参考序列/网络目录/合规规则/商业策略互相引用,
  能讲通「建产品→配价→一单的一生」;不是每表孤立几行。②脚本与 JSON 的入库位置
  由你定并记回本票(建议 `scripts/demo-seeds/` 一类,明确标注仅限隔离环境);本轮
  完工报告改回**频道 3**。
- 2026-08-25 18:02 WSL 队列频道 1(本轮调度):本票改派 WSL 队列频道 5。证据:原主 13:02
  认领后至 17:57 零提交,git status 全树(含 untracked)亦无种子 JSON/脚本产物。开工先查
  库内有无 SYN- 残留(原主可能灌过半截)并把发现记回本票。完工报告改报 WSL 队列频道 1
  (上一条「改回频道 3」为上一轮调度口径,本轮作废)。
- 2026-08-25 17:44 WSL 队列频道 5(接管开工):查库结论——无 SYN- 残留,且比预想更空:
  idp-parcel-postgres-gate 的 postgres 库于 17:41 查得 pg_tables 业务表零张(仅两个
  parcel_test_* 遗留测试库),迁移历史 schema 也不存在。即原主未跑过迁移、未灌过任何
  数据,半截残留不存在。一键脚本因此须自带迁移步骤(migrate.Run 仅有测试调用方,
  无生产入口),才能满足「干净库零报错复灌」。
- 2026-08-25 18:35 WSL 队列频道 5(收口):**resolved,SHA `8f2d9f5`**。产物落
  `scripts/demo-seeds/`(README 标注仅限隔离环境):seed.sh 一键(--reset 复灌)、
  migrate 助手、seedgen 计价快照生成器、种子 30 份(商业发布批 8 项/计价两卡两序列/
  网络七族 14 行/关务六册 11 份),租户 SYN-TENANT-01、范围 SYN-SCOPE-01,互引钉名:
  价卡方向授权→SYN-AUTH-PRICE-DIR-01(商业授权对象)、价卡结构→SYN-SERIES-FUEL-01、
  汇率口径→SYN-PRICE-RULE-CN-SG、线路/路由策略 applicable_scope→SYN-SCOPE-01、
  规则包适用性→(产品,合同,法人,范围)四维。
  **验证种别:灌入日志(S)**——18:31 对 127.0.0.1:55432 干净库(--reset)连跑两轮,
  两轮均零报错、37 行登记全落(8×PUBLISHED_EFFECTIVE + 4×RECORDED + 14×REGISTERED
  + 11×REGISTERED);逐表点数 28 张相关表与预期全同(价卡版本 2、序列版本 2、商业版本 8、
  规则包 1+规则 5、节点版本 5、连接 3、线路 1、服务区 2、日历 1、调整 1、策略 1、
  目录修订 1、关务各册 1/1/2/1/2/1/1/2)。gofmt/go vet(scripts)/全仓 go build 绿
  (未跑全仓 go test:本票只新增 scripts/ 与数据文件,不触任何既有包)。
  **如实边界**:commercial_price_policy(0010)/commercial_settlement_policy(0011)/
  service_product_form(0008)三表持久化面在而无进程级写入口(SavePricePolicy/
  SaveSettlementPolicy/SaveServiceProduct 无 cmd 调用方,查证于 8cb43e4),册面为空
  是诚实态;商业策略页五列因此三列有据两列空,服务产品页形态列空。补写入口属机制
  半边,不归本票。页面可见性另依赖目录 Intake 配置(票 07/频道 3 地盘)。
