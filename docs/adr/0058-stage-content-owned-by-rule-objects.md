# ADR-0058: 阶段内容声明按拥有规则对象归属，产品与合同只采用

Status: Accepted
Date: 2026-08-18

## Context

`parcel-shipment` 消费三份阶段内容：收寄资格（PAR-COM-16）、终局规则与取消授权目录（PAR-COM-17）。消费侧适配器已经能翻译，但提供方三个内容类型都不携带拥有对象，取消授权的注释还把拥有对象写成「已生效产品或合同」——文档里就是未定的。

[ADR-0042](./0042-acceptance-content-declarations-by-owning-object.md) 的纪律是「声明按拥有对象归属，合成一张表会把这条归属抹掉」。拥有对象没裁定就建表，等于用一次迁移把它拍死，而迁移不可变。

产品与合同在用例里反复出现，是因为委托被接受时固定所**采用**的规则版本；采用层话语不是归属层话语。把采用方写成主键，规则正文一改就要跟着改产品/合同版本，而规则包与授权规则本就有自己的版本生命周期。

## Decision

**一、三件的拥有对象都是规则对象版本，不是产品或合同。**

| 声明 | 拥有对象 |
|---|---|
| `IntakeQualificationContent` | 接单规则包版本（`ACCEPTANCE_RULE_PACKAGE`） |
| `FinalRuleContent` | 接单规则包版本（`ACCEPTANCE_RULE_PACKAGE`） |
| `CancellationAuthorityContent` | 授权规则版本（`AUTHORIZATION_RULE`） |

产品与合同是采用方：接受时固定闭包引用这些规则版本，并不因此取得正文所有权。

**二、三族分表、三口分读，不进 `ViewRevision`。** 主键 = 租户 + 拥有规则版本四元组（`object_kind` 入 CHECK）+ 声明格。无父行 = `found=false`（未配置）；父行在场而子行空/坏 = error，不得折成未配置。读取失败是另一格。本上下文不提供默认内容。

**三、消费侧装配不得从 `SourceIdentity` 发明采用版本。** 身份键不含规则对象；没有实例参数能补上时，生产适配器停在诚实未配置。

## Consequences

- 领域构造门要求拥有对象为已生效的对应规则种类；错种类或未生效走既有 `ErrUnusableRulePackage` / 新增的 `ErrUnusableAuthorizationRule`。
- PostgreSQL 点读口按租户与拥有版本一次快照取回；显式租户必须与拥有版本同一身份（ADR-0003 / ADR-0040）。
- 规则包正文（B7）仍是另一族，本记录不把它预支进本表。

## Alternatives considered

- **挂在服务产品或客户合同上。** 否决：那把采用方写成拥有方，规则改一版就要跟着发产品/合同；CONTEXT 给授权规则独立身份「不合并为大配置」，取消授权尤其不能并进合同。
- **三件合成一张表。** 否决：归属种类不同（规则包 vs 授权规则），合成会抹掉 ADR-0042 的归属纪律。
- **未配置时由消费方代拟「客户可取消」或「有效交付即终局」。** 否决：那正是缺行作为真话要挡住的默认。

## Links

- [ADR-0042：接受内容声明按对象归属建模](./0042-acceptance-content-declarations-by-owning-object.md)
- [ADR-0003：集团租户边界](./0003-group-tenant-legal-entity-customer-account.md)
- [ADR-0040：商业版本身份键携带 TenantID](./0040-commercial-version-key-carries-tenant-id.md)
- [ADR-0025：跨上下文适配器落在消费方](./0025-cross-context-adapters-live-on-the-consumer-side.md)
- [ADR-0062：采用规则版本从已接受解析标识回指提供方持有的闭包](./0062-adopted-stage-owner-from-accepted-resolution.md)：补第三条没写的回指路径；禁止从身份发明仍有效
- [ADR-0063：收寄硬资格证明由消费侧窄口取证](./0063-intake-qualification-proof-is-a-consumer-side-evidence-port.md)：声明列出之后如何取证；本记录只管声明归属
- [ADR-0120：资料修订允许声明是接单规则包版本下的第三族阶段内容声明](./0120-source-data-amendment-allowance-is-a-third-stage-content-family-on-the-rule-package-version.md)：第四件按决定一的同一条纪律归规则包版本；本表不改，那里记形状与缺格语义
