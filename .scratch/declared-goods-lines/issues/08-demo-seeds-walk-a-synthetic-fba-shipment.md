# 08 演示种子：SYN-TENANT-01 走通一票 FBA 货件

Category: enhancement
Status: ready-for-agent
Blocked by: 03, 05

依据：AGENTS.md 红线「软件产品」一段（演示租户 `SYN-TENANT-01` 按真实租户对待，证据记 `S`）；ADR-0374 决定三；ADR-0364。

## 要做的

- 在 `scripts/demo-seeds` 为 `SYN-TENANT-01` 登记一个合成的 FBA 服务产品与价卡（按整票汇总重量计费），一个合成平台仓收件资料；全部用 `SYN-` 标识。
- 演示脚本提交一票 FBA 货件（如 30 箱同款加 2 箱混装）与一票多行货物的小包，走到受理。
- 演示文档补一段 FBA 动线。
- 演示租户采用 03 随产品带的「FBA 头程」接单规则包参考配置，走一票缺箱标号的货件看它停在等待受控补充。

## 完成判据

- `scripts/demo-seeds/seed.sh` 灌进一次性库退 0；两票都受理，管理台详情能看到货物明细、装箱分配与平台号。
- 证据如实记 `S`；不往参数登记册写任何真实取值。
