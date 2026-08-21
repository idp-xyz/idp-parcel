# 接入渠道登记册与首个真渠道 Intake 缺失,渠道墙想配也没处配

Category: enhancement
Status: ready-for-human

来源:SYN-WALL-DOOR-AUDIT 走通审计(基线 `49a2ab0`),对应清单 W01/W02。

## 墙

`ACCESS_CHANNEL_NOT_CONFIGURED`(403)——PS/NO/TF/VE/CC 五个上下文的 `adapters/http/unconfigured_intake.go`(`ErrAccessChannelNotConfigured`),八个业务端点在 `cmd/parcel-api/endpoints.go` 的 `assembleBusinessEndpoints` 全部装 `UnconfiguredIntake{}` + `unwired*` 编排桩。

## 现状:门四件全缺

按 ADR-0055,「空登记册就是装配点本身」:没有渠道登记册的表与迁移,没有装载口,没有写入方,没有登记口。今天「配置一个接入渠道」的唯一含义是**改装配点代码**——这在设计上是有意的(真渠道就位时逐端点替换),但它意味着 `PAR-INT-01` 的实例值没有落点,认证机制(凭据验证、来源信封铸造,ADR-0003 禁采信自报身份)整体缺席。

## 缺的最小机制件

1. 接入渠道登记册:表 + 只读装载口 + 登记口(渠道身份、凭据引用、适用客户账户/来源范围、有效区间;敏感凭据本体外置,登记册只存受控引用)。
2. 首个真渠道 Intake 实现:凭据验证 → 铸造来源信封(租户/客户账户/来源/来源请求键)→ 构造命令;替换点即 `assembleBusinessEndpoints`,路由层与处理器不动(ADR-0055 已预留)。
3. 载荷规范化摘要与准入范围装配(`PAR-GOV-03..07`)仍拦在真渠道 Intake 之前,不得绕过(ADR-0055 第五条)。

## 红线

- 不得出现任何「开发用」采信头部实现;未配置即拒的兜底在真渠道就位前不许撤。
- `PAR-INT-01` 实例值(真实渠道参数)留空是常态;本票只建门,不填值。

## 参照

ADR-0055、ADR-0003、ADR-0052;`docs/product/PILOT-PARAMETER-REGISTER.md` PAR-INT-01。

## Comments

- 2026-08-20 MCP-2：对 `3324ecb` 重核四件，**结论不变：无门，四件全缺**。仓储——全库
  78 份迁移无任何渠道登记册表（`channel` 只命中 VE 索赔/通知三表，无关）；装载口、写入
  方——无；登记口——八个业务端点仍全部装 `UnconfiguredIntake{}` + `unwired*`
  （`cmd/parcel-api/endpoints.go` 的 `assembleBusinessEndpoints`，八行原样），路由层仍只有
  RequestID/Recoverer 两个中间件（`internal/platform/httpapi/router.go` 的
  `NewWithEndpoints`）。基线 `49a2ab0` 以来 `cmd/parcel-api` 零提交，票面与代码无矛盾。
- 2026-08-21 MCP-6：对 `0ec62ea` 重核四件，**四件全缺的结论不变**；但另查出**票面第 1、2 件
  与 ADR-0055 已否决的替代方案正面冲突**，按 AGENTS.md「ADR 优先于用例与设计交接」判本票
  **不可按现状开工**。未建 worktree，未改任何代码，原样报回 MCP-1。Status 由 `ready-for-agent`
  改 `ready-for-human`：阻断要的是一份新 ADR，不是实现工。

  **重核四件（`0ec62ea`）**：仓储——`migrations/` 下十个上下文目录共 82 份迁移，无任何接入
  渠道或凭据登记表；`channel` 只命中 VE 通知策略与 ETA 缺口通知的 `channel_ref`、索赔里程碑
  取值 `CHANNEL_ACCEPTED`，`intake` 只命中 NO `reception` 的 `intake` 列、PS `intake_adoption`
  与 PC `intake_qualification_*` 三表（收寄采用资格），均与接入渠道无关。装载口、写入方——无表
  故无。登记口——八个业务端点仍全部装 `UnconfiguredIntake{}` + `unwired*`
  （`assembleBusinessEndpoints` 八行原样），路由层仍只有 RequestID/Recoverer（`NewWithEndpoints`）。
  票面与代码无矛盾。

  **前置一 载荷规范化摘要：不存在。** `domain.PayloadDigest` 只是 `requiredValue` 的非空字符串
  包装，没有任何函数按规范化业务内容产出它；PS 包内无规范化符号（`parcelpricing` 的
  `canonicalPricingPlan` 是计价上下文自己的评价指纹，不是 PS 载荷）。`SubmissionIntake` 注释
  自证「`parcel-shipment` 侧的规范化形状尚未实现」。**可建但不该并进本票**：PS CONTEXT 已定死
  摘要进出边界（`occurredAt`/`receivedAt` 属来源信封元数据不进摘要，`requestEffectiveAt` 及其
  缺失/显式存在状态进摘要），机制半边有据；但按 ADR-0014 规范化形状必须带版本号，那是独立的
  设计决定。

  **前置二 准入范围装配：不存在，且属票 02 地盘。** `domain.AdmissionScope` 同样只是 reference
  与 digest 两个非空字符串的容器，由命令入参给入，无任何东西按 `PAR-GOV-03..07` 装配它；该组
  参数在登记册全为「待提供」，且是「进入限量生产前」的门槛项。下游
  `ports.ProductionOwnershipAuthority` 全库无生产适配器——那是审计 W03 与票 02。

  **阻断三条**：

  1. **票面第 1 件正是 ADR-0055 明文否决的替代方案，无任何记录停用该否决。** 其 Alternatives
     写着「**为接入渠道建运行时登记表，读表判配置。** 否决（现在）：渠道配置的形状取决于真实
     渠道是 API、标准文件还是门户，替它拟表就是替租户拟 `PAR-INT-01` 的样子……真渠道就位时若
     需要表，由那笔工作按实际渠道形状立」。票面把列都预先拟好了（渠道身份、凭据引用、适用客户
     账户/来源范围、有效区间），正落在该否决里。解否决的前件「真渠道就位」不成立：`PAR-INT-01`
     登记为「待提供」，最低证据是「API、标准文件或门户的现行流程」，而本仓尚无租户。

  2. **票面第 2 件所要的凭据验证不归这五个上下文中任何一个。** PS CONTEXT 硬句：「共享身份认证
     与授权技术能力拥有凭据验证、通用授权策略和授权作用域签发；`parcel-shipment` 只消费已授权
     作用域并执行自身对象过滤，不建立企业身份主数据、授权引擎或跨客户查询层」；术语「授权查询
     作用域」同款写明 PS「不拥有企业身份主数据、凭据验证或通用授权策略」。party-commercial
     CONTEXT 另有「渠道账号凭据和渠道接入的技术实现不属于本领域文档的决策范围」。把渠道登记册
     与凭据验证落进这五个上下文任一个都破红线「所有权清晰」；而它该落的「共享身份认证/授权技术
     能力」在本仓既无上下文目录也无迁移目录。

  3. **因此 ADR-0068 停用 ADR-0053「现在不建表」那条解法不能照搬。** 0068 能成立靠的是「结构
     半边由 CONTEXT 硬句定死、不依赖取值」。接入渠道恰恰相反：CONTEXT 硬句不是在定死结构，而是
     把凭据验证整件推出这几个上下文之外，本仓没有任何 CONTEXT 为渠道登记册的形状背书。0068 的
     **形式**（结构先行、内容等参数）也许仍可援引，但那要先有一个上下文认领所有权，属难逆转
     取舍，按 AGENTS.md 要走新 ADR。

  **建议（未自行执行）**：拆两半。①「渠道登记册 + 真渠道 Intake」先出一份新 ADR，回答两件——
  这份能力归哪个上下文（或是否新开一个共享身份/接入上下文），以及是否按 ADR-0068 的形式部分
  停用 ADR-0055 那条否决、结构先行而凭据形态等 `PAR-INT-01`。②「PS 载荷规范化摘要」独立成票，
  机制有据且不碰 `PAR-INT-01`，但要按 ADR-0014 带规范化版本号。

  证据等级 S（静态审计：读符号、迁移与装配点；未运行进程，未建 worktree，未改任何代码）。
