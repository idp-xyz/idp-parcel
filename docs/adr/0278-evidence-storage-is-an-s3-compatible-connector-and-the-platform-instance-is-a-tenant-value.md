# ADR-0278：试点证据存储的形态是 S3 兼容对象存储与命名空间约定，归产品；用哪一个平台、谁建谁运维是租户取值，登在该租户的 `PAR-GOV-11`

Status: Accepted（2026-10-21，Claude Code 会话代决，代产品 owner 裁定：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/10（10b 半边），出处审计 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) PL·F6（归 B）。部分停用 [ADR-0008](./0008-self-hosted-minio-evidence-store.md) Decision「为 `idp-parcel` 建设公司自建的 MinIO S3 兼容对象存储，使用逻辑命名空间 `idp-parcel/pilot` 保存试点证据和脱敏映射。」一句里「建设公司自建的 MinIO」那一半，与 Consequences「公司保持证据存储和访问边界的自主控制，不依赖外部文档平台的租户或发布周期。」「自建 MinIO 的实施和运维责任由基础设施团队承担，具体运行要求在其实施文档中管理。」两句，被停原句与适用场景见「对已接受记录的作用」；ADR-0008 正文不改，只在其 Status 行与 Links 加前向指针。连接器的执行器随票 [product-strategy-boundary/13](../../.scratch/product-strategy-boundary/issues/13-pg-hard-risk-detection-and-evidence-storage-connector.md) 第 2 项。越权风险点见文末，待产品 owner 复核。）<!-- 索引注记 -->｜**产品**：正式试点证据与脱敏映射存于 S3 兼容对象存储，按平台、命名空间、对象标识与对象版本引用，逻辑命名空间约定为 `idp-parcel/pilot`；连接器形态与命名空间约定是产品策略。**租户**：用哪一个 S3 兼容平台（自建 MinIO、托管对象存储或别的实现）、实例引用、访问责任、建设与运维责任方和生效时间是租户取值，登在该租户的 `PAR-GOV-11`；没登记，证据存储就不算就绪，临时存储照旧不作正式证据的权威源。**不变**：存储只拥有证据对象的存储与访问边界，不拥有任何业务对象，不作业务数据库或跨上下文通道；项目仓库只存脱敏标识与证据索引。越权风险点含开发方若托管证据存储、命名空间约定跨租户同名、连接器尚无执行器

Date: 2026-10-21

## Context

[ADR-0008](./0008-self-hosted-minio-evidence-store.md) 写于开发方与运营方还被当作同一家的时候：「当前公司没有可复用的企业级对象存储平台。」，于是决定「为 `idp-parcel` 建设公司自建的 MinIO S3 兼容对象存储」。[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)「权威证据存储」一节与 `PAR-GOV-11` 行、[验收场景矩阵](../product/PILOT-ACCEPTANCE-MATRIX.md)的证据位置一句都照它写成「自建 MinIO」。

此后两条现行规则与它相抵，谁也没停用它：

- [AGENTS.md](../../AGENTS.md)：「**本仓开发的是卖给物流企业的软件产品，开发方不是运营企业。**」正式试点证据是某个租户的证据——它的客户合同依据、脱敏映射、外部回执与 `Go/No-Go` 决定——存在哪里由那个租户定，开发方的「公司」不在这条链上。
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 把连接器形态归产品策略、具体实例归租户取值。登记册 `PAR-GOV-11` 自己的拆分注已照此写：「S3 兼容证据存储的连接器形态与命名空间约定归产品策略」「平台实例引用、访问责任与生效时间仍是租户取值」。

照矩阵那句字面读，一个用自己托管 S3 兼容存储的租户，证据就「不能成为权威证据源」，正式验收会被误判为不成立。

## Decision

**一、证据存储的形态归产品。** 正式试点证据与脱敏映射存于 S3 兼容对象存储，按平台、命名空间、对象标识与对象版本引用；逻辑命名空间约定为 `idp-parcel/pilot`。连接器形态与命名空间约定是产品策略（ADR-0146），执行器随票 product-strategy-boundary/13。

**二、平台与实例归租户。** 用哪一个 S3 兼容平台——自建 MinIO、托管对象存储或别的实现——连同实例引用、访问责任、建设与运维责任方和生效时间，是租户取值，登在该租户的 `PAR-GOV-11`。没登记，证据存储不算就绪；平台就绪前的临时存储照旧不作正式证据的权威源（ADR-0008）。产品不预设任何平台为默认。

**三、边界照旧。** 证据存储只拥有证据对象的存储与访问边界，不拥有客户、参与方、线路、节点、包裹或其他业务对象，不作 `idp-parcel` 的业务数据库，也不作跨上下文的业务数据通道；项目仓库只保存稳定脱敏标识、证据索引、对象版本与受控访问责任（ADR-0008）。

## 候选与反方

- **维持 ADR-0008，产品绑定 MinIO。** 否决：那是替每个租户选了平台，与「租户取值留给租户」相抵；MinIO 与托管 S3 在本产品要的能力（对象版本、稳定索引、受控访问）上没有差别。
- **证据存储整个归租户，产品不定形态。** 否决：形态不定，证据索引就没有统一的引用格式，PN-08 的证据包索引与候选版本组引用不了对象版本；这正是 ADR-0146 判归产品策略的那一类。
- **只改登记册与矩阵措辞，不立记录。** 否决：两份文档照的是 ADR-0008 的 Decision，只改文档会让文档与一份未停用的已接受记录相抵。

## 对已接受记录的作用

- **部分停用** [ADR-0008](./0008-self-hosted-minio-evidence-store.md) Decision「为 `idp-parcel` 建设公司自建的 MinIO S3 兼容对象存储，使用逻辑命名空间 `idp-parcel/pilot` 保存试点证据和脱敏映射。」一句里「建设公司自建的 MinIO」那一半：平台选型与建设归租户（本记录决定二）。**适用场景**：全部租户。同句「S3 兼容对象存储」「使用逻辑命名空间 `idp-parcel/pilot` 保存试点证据和脱敏映射」与同段「物理部署和责任方必须在 `PAR-GOV-11` 中登记后才算就绪」不变。
- **部分停用** ADR-0008 Consequences「公司保持证据存储和访问边界的自主控制，不依赖外部文档平台的租户或发布周期。」与「自建 MinIO 的实施和运维责任由基础设施团队承担，具体运行要求在其实施文档中管理。」两句：控制与运维责任归登记在 `PAR-GOV-11` 的租户责任方（本记录决定二）。**适用场景**：全部租户。Decision 里「MinIO 只拥有证据对象的存储和访问边界……」「平台就绪前，临时存储不能作为正式生产证据的权威源」与 Consequences 其余各条按「证据存储」读照旧成立。

## Consequences

- 参数登记册「权威证据存储」一节与 `PAR-GOV-11` 行、验收场景矩阵的证据位置一句、[docs/README](../README.md) 的敏感实例一句与 [PN-08 交接](../design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md)里写「MinIO」的各处，改写为「S3 兼容对象存储」与「该租户 `PAR-GOV-11` 登记的平台」。
- 演示租户 `SYN-TENANT-01` 只形成 `S` 证据，不进登记册，本记录不给它配证据存储。

## 越权风险点

1. **开发方若托管证据存储**（产品 owner）。若产品将来以托管服务形式替租户存证据，平台由开发方提供，这一格仍是租户取值——租户选择用开发方托管的那一个；但责任方、访问边界与留存要在合同里另立，本记录不裁。
2. **命名空间约定跨租户同名**（产品 owner）。`idp-parcel/pilot` 是逻辑命名空间约定，不是全局唯一名；两个租户若共用同一平台，隔离靠平台实例或桶的访问边界，不靠这个名字。
3. **连接器尚无执行器**（产品、PG owner）。形态已归产品，执行器还没有（票 product-strategy-boundary/13 第 2 项待核）；在那之前证据索引只是文档里的引用格式。

## Links

- [ADR-0008：试点证据采用自建 MinIO S3 对象存储](./0008-self-hosted-minio-evidence-store.md)：部分停用 Decision 一句的一半与 Consequences 两句
- [ADR-0146：产品策略是机制与实例之间的第三类](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：连接器形态归产品策略、实例归租户取值
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md)「权威证据存储」与 `PAR-GOV-11`；[验收场景矩阵](../product/PILOT-ACCEPTANCE-MATRIX.md)
- 票 `.scratch/docs-consistency/issues/10-cross-cutting-docs.md`（10b 完成记录）；`.scratch/product-strategy-boundary/issues/13-pg-hard-risk-detection-and-evidence-storage-connector.md`
