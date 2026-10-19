# 30 演示租户登合成作业设备与作业事实授予：主链作业事实各口在演示上越过渠道

Category: task
Status: 完工，待评审与进 main——2026-10-19 Claude Code 会话立并实施（用户「你作为业务和系统专家自决……解决所有的问题」授权），分支 `claude-psb30`。出自 2026-10-16 在 `c68e4342` 上重走演示动线（[05](./05-demo-journey-criterion-evidence.md)）。合成集成客户端那一半拆到 [31](./31-demo-integration-client-needs-a-client-credentials-capable-issuer.md)。
Blocked by: 无
地盘：`scripts/demo-seeds/data/access/` 的行文件；`scripts/demo-seeds/seed.sh` 只加调用行（与 `idp-parcel-9b` 约定：调用行先加、加完把 SHA 告诉它）；[合成演示动线](../../../docs/design/synthetic-demo-journey-script.md)「前置」一句。
出处：[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定二「一线作业事实用合成设备……走普通登记」与决定五；[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)；[ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md)；[ADR-0180](../../../docs/adr/0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)。

## 现状（实测，钉 `c68e4342`，证据 `S`）

- **作业事实各口**：收寄；揽收登记、执行、更正；交接与交接更正；移动；派送触发；派送尝试；交付；POD 更正。
  - 照 demo-seeds README 起操作者渠道，带配置员令牌与设备三头打，答 `403 OPERATOR_NOT_GRANTED`。
  - 库里 `access_identity.device` 零行。配置员的授予里没有 `WORK_FACT_REGISTRATION`。
  - 机制都在：`parcel-access-register` 有 `device-register`，授予支持带 `workScope` 的 `WORK_FACT_REGISTRATION`。
  - [operator-channel/17](../../operator-channel/issues/17-demo-issuer-dex-and-synthetic-operators.md) 的「不做」把作业事实授予与设备推给了 10，10 收口时没灌种子，这一格因此没有票接。
- **集成客户端各口**（外部结果、监管凭证、外部资金事实）：演示发行方 Dex 签不出客户端凭据令牌，拆到 31。

## 三类归法

- **机制**：已在 main，本票不改代码。
- **租户取值（演示租户）**：合成作业设备与其作业范围、操作者的作业事实授予。全部 `SYN-` 值，证据只记 `S`，经普通登记落（ADR-0150 决定一、二）。不替任何真实租户登行。

## 做什么

1. **合成作业设备。** 至少一台节点设备（作业范围取 `SYN-NODE-SHA-HUB` 一类种子节点），一台场外揽收设备。设备号、范围种类与引用照 `translate_device.go` 的批文形状登，范围不代填。
2. **作业事实授予。** 配置员的 `WORK_FACT_REGISTRATION` 授予带作业范围，与设备范围对得上。若作业事实各口铸信封前也读准入范围对照与权威区间（ADR-0180），照运营决定那几行的先例补登：对照住 `data/access/`，区间住 `data/governance/`。
3. **动线。**
   - 作业事实各口越过渠道即答 `400 MALFORMED_REQUEST` 或 `200` 业务结果。
   - 注意：作业事实 Intake 先核三头再核令牌，空体不带三头的 400 不能当「渠道已过」。

## 不做

- 不改任何执行器、Intake 或答复形状。
- 不登真实设备或真实操作者。
- 集成客户端那一半（发行方、客户端行、授予、部署参数）归 31。

## 完成判据

- 55432 上自己命名的一次性库干净灌种子，退 0。
- 照动线「前置」起进程：
  - 带配置员令牌与设备三头，作业事实各口越过渠道。
  - 答业务校验或业务结果，不再是 `403`。
  - 证据只记 `S`。走完删库。
- 读演示种子的用例包全绿（种子摘要自校若受影响，在新快照上重钉）。

## 完成记录（2026-10-19，Claude Code 会话，分支 `claude-psb30` 基 `320d2900`）

- 种子：`data/access/04-devices.json` 登五台合成设备（演示网络四个节点各一台、华东场外揽收范围 `SYN-OFFSITE-CN-EAST` 一台）；`02-grants.json` 给配置员加五条「作业事实登记」授予，作业范围与设备一一对上；`seed.sh` 在操作者授予之后调 `device-register`。作业事实各口铸信封不判准入范围，不登对照与区间。
- 实走（`S`）：一次性库 `c8_walk30` 灌种子退 0，五台设备 `RECORDED`。照前置起 `parcel-api`（操作者渠道三件、治理自授权），取配置员令牌，带设备三头打：收寄、交接、交付、移动答 `400 MALFORMED_REQUEST`（越过渠道、走到报文校验），场外揽收答 `200 SOURCE_NOT_ACCEPTED`；对照组拿 SIN-LM 的设备报上海枢纽的范围答 `403 DEVICE_NOT_IN_USE`——设备与范围的核对是真的。走完已删库。
- 执行器、Intake、答复形状一处未改。
