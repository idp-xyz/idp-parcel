# 接入渠道登记册与首个真渠道 Intake 缺失,渠道墙想配也没处配

Category: enhancement
Status: needs-triage

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
