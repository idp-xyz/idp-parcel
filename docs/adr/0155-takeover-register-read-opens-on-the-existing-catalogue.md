# ADR-0155：对象级接管的查阅口开在既有治理登记册上——空册如实空；不造种子行；HTTP 写仍另票

Status: Accepted（2026-09-29 用户在本通道对后续工作答「继续」。通道 2 按已开的登记命令把查阅口补上，不新开写面，不填 `PAR-GOV-05..07`。）
Date: 2026-09-29

## Context

ADR-0154 已经把接管的写入口定在 `parcel-governance-register takeover`，并且写明种子不造行、HTTP 写另票。管理台 `stage-admission` 页仍把这一格写成「登记口第二批未开」。那句话在写入口落地之后不再是实话：登记命令已经在，空表是因为没有证据行，不是因为命令没开。

查阅口当时故意没开。ADR-0083 决定四要求阶段评审与接管两格说明属第二批未开，不留白也不造数。`GET /governance-registers` 的封闭集因此只有权威区间、暂停、恢复。问 `register=takeover` 是坏请求。

阶段评审的登记命令 `stage-review` 也已经另开，它的查阅口仍不在本记录里打开。两格不再绑成同一句「都没开」。

## Decision

**一、接管查阅口用既有端点，词与登记命令相同。** `GET /governance-registers?register=takeover` 上列 `pilot_governance.takeover_record`。未配置 Intake 仍 403。空册答空数组，不折成未配置。缺席 `register`、以及 `stage-review`，仍是坏请求。

**二、列面是标量，盘点 jsonb 不上列。** 区间四维、开始与结束时刻、停写证据、已接受事实、外部未决、实际控制、责任、下一步、盘点时刻、生效时刻照登。结束时刻缺席即开放区间。盘点条目仍只经 `TakeoverStore.FindByInterval` 装载，与恢复决定不把盘点 jsonb 送上列面同一纪律。

**三、不造行，不开 HTTP 写。** 种子保持空表。页面空册文案仍指向登记 CLI，本页不写。管理台写面与 Intake 族仍是 ADR-0154 留下的另票。不填 `PAR-GOV-05..07`。

**四、只停用 ADR-0083 决定四里接管那半句。** 阶段评审格仍说明查阅未开。页面其余「产品实例级、不按租户隔离、只读」不变。

## Consequences

- 读端口、PostgreSQL 上列、HTTP 转写与 `stage-admission` 页的接管区块按本记录落地。阶段评审区块改口，不再把接管说成登记口未开。
- 种子 README 写明查阅口已开、本包仍不造行。
- 本记录不改 ADR-0154 的写侧决定，也不改 ADR-0128。

## Links

- [ADR-0083](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)：决定四接管格「第二批未开」由本记录停用
- [ADR-0154](./0154-takeover-record-write-side-is-not-yet-decided.md)：写入口与「种子不造行、HTTP 写另票」仍以该记录为准
