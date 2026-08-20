# 收寄硬资格证据口无生产实现,资格证据无处可登

Category: enhancement
Status: needs-triage

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W11 证据面。

## 墙

采用链的硬资格证据口在两处装配(`cmd/parcel-dispatch/assemble.go` 的 `networkIntakeAdoption` 与 `adoptEffectiveDeliveryConsumer`)都给 `pspartycommercial.UnconfiguredIntakeQualificationEvidence{}`——ADR-0063 的「显式未配置」实现:资格要求(如 `INTAKE-QUAL/customs-precheck`)一经声明即答未证明,采用停在 `ELIGIBILITY_UNDECIDED`。

## 现状

- 端口与「诚实无门」实现在;证据的存储、装载口、写入方、登记口全缺。
- 合成种子(`syn_pc_seed_test.go` 的 `seedIntakeQualification`)只在测试里给证据,生产路径没有任何来源可登。
- 注意与票 03 的分界:资格**要求**由 PC 声明表携带(票 03 的发布面);本票管资格**证据**——某对象已满足要求的事实从哪来、登在哪。

## 缺的最小机制件

资格证据来源:证据登记存储 + 装载口 + 写入方(按 ADR-0063 的证据语义:逐对象/逐要求、带来源与有效性;证据可能来自 CC 预检结果等源上下文,归属先对照 CONTEXT 再定)。

## 红线

- nil 与显式未配置都不得默认 `ESTABLISHED`(装配点注释原话);本票不放松。
- 证据来源归属未裁前不擅自选边;若归属要改 CONTEXT,先走文档再落码。

## 参照

ADR-0063;PAR-COM-16;`parcelshipment/adapters/partycommercial` 的 `ServiceStageRulesAdapter` 装配注释。

## Comments

- 2026-08-20 MCP-2：对 `3324ecb` 重核四件，**结论不变：证据的仓储/装载口/写入方/登记口
  全缺**。两处装配点仍显式未配置——`cmd/parcel-dispatch/assemble.go` 的终局图与
  `networkIntakeAdoption` 图都传 `UnconfiguredIntakeQualificationEvidence{}`（注释原话
  「nil 会在非空清单上变成依赖错误，两者都不得默认 ESTABLISHED」原样）；
  `seedIntakeQualification` 仍只在测试（`cmd/parcel-dispatch/syn_pc_seed_test.go`）。
  **票面一处漏记照实更正**：`KnownPrefixIntakeQualificationEvidence`（按引用前缀把证明
  路由到已登记权威口）在审计基线之前就已存在（`7cb39b6`，08-18，ADR-0063 同笔），
  票面「端口与『诚实无门』实现在」少数了这件——**组合缝已有**，今天缺的是缝后面的
  证据来源四件与把组合口装进两处装配点那一步；「缺的最小机制件」范围不因此变小，
  但落点应从「另起证据口」改读为「给 KnownPrefix 缝后面接真源」。
