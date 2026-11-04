# 商业发布审批职责规则的登记入口：受控命令行登记，演示租户登一条

Category: enhancement
Status: resolved——2026-11-04 进 main
出处：commercial-registers-edit-containers 票 06 与 commercial-version-object-pages 票 02 的真实写操作补核停在批准门答 `NOT_CONFIGURED`；用户 2026-11-04 答「请你登记」。

现状：ADR-0126 立了审批职责规则的表、读口与仓储写方法，但登记入口留待治理写面那一族另裁（决定五），没有任何进程口调写方法；`PAR-COM-18` 一直是「待提供」，所有租户的批准门都答未配置。

## 决定（ADR-0362）

- 入口是 `parcel-commercial register-approval-duty-rule -input <file>`，与本上下文其余受控登记同一族；文件形状归产品定，`requiresDistinctSubjects` 必填、`requiredApproverLevel` 可缺；一租户一条，同内容重放、异内容冲突不覆盖。
- 演示租户 `SYN-TENANT-01` 在种子里登一条合成取值（单主体可批、不要求等级），证据 S；真实租户仍待提供。

机制：登记入口。租户取值：演示租户的那一条（S）。

## 子票

| 票 | 内容 | 状态 |
|---|---|---|
| [01](./issues/01-cli-entry-and-demo-seed.md) | 子命令、用例、种子、ADR、参数登记册行；对演示库登记并走通批准 → 发布 | resolved（2026-11-04 进 main） |
