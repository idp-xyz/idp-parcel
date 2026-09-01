# ADR-0085: 登记册配置写面进端点表带未配置格——写表单属产品能力，CLI 保留为受控批量口；写准入不另立形，与其余命令面同等真渠道证据

Status: Accepted（2026-08-31，用户经 IDP 队列通道 3 授权本会话「作为业务和系统专家直接开干」，据此对 [admin-write-faces/01](../../.scratch/admin-write-faces/issues/01-registry-configuration-has-no-admin-write-face.md) 的三项待裁作出裁决。裁决能力边界：读过 ADR-0055/0072/0077/0078 全文、`assembleBusinessEndpoints` 装配表、`cmd/parcel-pricing-register` 与 parcel-pricing 登记用例先例、closure-batch spec 与产品基线「产品就绪与试点就绪」「首发范围约束」两节；未逐一重读各上下文 `CONTEXT.md`——本记录因此只裁结构（写面走哪条准入、落在哪一层），不裁任何单册的领域不变量，各册表单形状与答案代数由实施票按其登记用例照抄）
Date: 2026-08-31

## Context

管理台今天是纯查阅面。登记册配置的写入机制齐全——登记用例、真库写口、七个受控登记 CLI（`scripts/demo-seeds/seed.sh` 逐个调用）——但唯一入口是「工程师在数据库网络内手跑 CLI」。产品就绪里程碑已成立（2026-08-28 受托认可，见产品基线「机制半边现状」节），下一站是商务洽谈与租户试点：租户的运营配置员要能在管理台上登价卡、登网络、登商业载体，工程师跑 CLI 不是可交付的长期配置方式。

「写动作从页面发起」此前有三处既有文本围着：

1. [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) Decision 五的两项未决（载荷规范化摘要、准入范围装配 `PAR-GOV-03..07`）拦着真渠道 Intake 与一切命令面；[ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) Decision 五维持原句。
2. ADR-0078 为隔离环境放行的只有运营查阅面，且明文「不为写面立任何先例」、以编译期保证隔离 Intake 装不进命令端点。
3. `cmd/parcel-pricing-register` 文件头写着登记「是治理动作，不是在线请求面，走独立进程而不进 parcel-api 的端点表」——那是登记 CLI 落地票当时的口径。

但这三处拦的都是「写面今天就放行」，没有一处拦「写端点以未配置格进端点表」。ADR-0055 自己的机制正是「每个端点各以『未配置即拒』的 Intake 起步」——装配表上既有命令端点全部如此存在。缺一张记录去说清：登记写面走同一条路，而不是另立准入形。没有这张记录，「管理台缺写面」就会持续被读成「文档禁止」或「实现遗漏」（2026-08-31 频道 3 的质疑原话是「很多 crud 都没有……是不是原来的文档就有问题」），而 closure-batch 刚用读面治过的病——「尚未接线」与「已接线但登记册为空/渠道未配置」长同一张脸——在写侧原样存在。

## Decision

**一、登记册配置写面属产品能力，进 `parcel-api` 端点表。** 各上下文在 `adapters/http` 为其登记用例增设命令端点（Intake 接口 + 处理器接口 + 封闭响应形状，照 [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 状态码语义与既有命令端点同款），装配以字面量 `UnconfiguredIntake{}` 起步。`cmd/parcel-pricing-register` 文件头「不进 parcel-api 的端点表」一句随首个实施切片更正为本记录口径；登记 CLI 不退场——CLI 与端点消费同一登记用例，是同一能力的受控批量口与在线口，答案代数一致。

**二、写准入不另立准入形。** ADR-0055 Decision 五两项未决照拦一切命令面，本记录新增的登记端点也在被拦之列；ADR-0078 的隔离读准入不扩到写行，其编译期排除保持。真渠道证据（`PAR-INT-01`）到位后，登记写端点与其余命令面同一批在装配点逐端点换真 Intake——替换缝就是 ADR-0055 预留的那条。本记录不新增任何等待项，也不缩短任何一条既有等待。

同日补记（频道 2 通报「管理台拟接 gk.idp.xyz（Ory Hydra 形 OIDC，authorization_code + PKCE）」后，钉住两处易误读）：「不另立形」约束的是准入**机制**——每端点一条 Intake 缝、未配置即拒，写面不复制 ADR-0078 那样的第二条准入通道——不约束「填缝的证据同族」。管理台写面与客户业务面分属两族证据：前者是运营/租户操作者身份（OIDC 认证结果是候选形态），后者是客户接入渠道信封；两族各自要过 `PAR-INT-01` 的真证据门，不得互相顶替。「同一批」指同一条缝、同一套等待纪律，不指同日点亮——换真本就逐端点，哪个端点的证据族先立住哪个先亮。真 OIDC 校验进 parcel-api 因此不是被否决的「另立准入形」，而是替换缝里的真 Intake 候选；其落地需自己的 ADR（信任锚、校验方式、操作者—租户授权模型），且 ADR-0055 Decision 五两项未决照拦，本补记不预裁也不缩短。前端 SPA 的登录门属管理台外壳 UX，不在本记录治域——只要它不把登录态铸成后端采信的任何证据（红线同 Alternatives 第二条），与本记录无涉。

**三、管理台写表单是机制半边，随端点两阶段接线。** 表单页组装登记载荷、提交后如实呈现三态——未配置（403 `ACCESS_CHANNEL_NOT_CONFIGURED`）、登记册治理答案（已登记/幂等重放/内容冲突/形状不可比）、未决；未配置态文案说「接入渠道未配置」，不说「尚未实现」。页接真次序照 closure-batch 两阶段纪律（上下文侧交活 → 装配点持有者接线并广播 → 页与 `liveIds`）。

**四、首切片与推进序。** 首切片为 parcel-pricing 两类登记端点（价卡、参考系列）：登记用例与 CLI 先例齐、答案代数已封闭，是最小可裁样本。其余上下文逐册跟进，取舍按「登记频次 × 操作者角色」由实施票逐册裁；治理登记册（pilot-governance，无租户维，[ADR-0083](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)）不入首批——治理动作的操作者授权模型特殊，单独裁。

## Consequences

- 每个入表上下文的 `adapters/http` 增登记命令 Intake 接口、端点构造函数与 `UnconfiguredIntake` 的对应实现；装配行按共享接线文件纪律交装配点持有者逐笔接入。
- 管理台配置页面随端点两阶段增写表单；隔离演示环境里这些端点如实答 403——「渠道未配置」与「登记册为空」两态从此在写侧也分得开。
- 登记 CLI 的文件头口径随首切片更正；seed 路径不变。
- 真渠道就位那天，写面与其余命令面同一批点亮；管理台由「查阅台」变「配置台」不再需要任何新裁决。

## Alternatives considered

- **另立运营写准入形（类比 ADR-0078 再裁一次）。** 否决：ADR-0078 的豁免理由是零持久化的查阅面上「事后无物可混」，写面持久化，该理由没有落点；ADR-0055 否决采信头部的原句「事后没有任何东西能把这些信封与真实认证结果区分开」在写侧原样成立；且 0078 明文不为写面立先例。
- **隔离环境按 S 级放行写面（demo 里从页面写合成数据）。** 否决：合成操作者身份就是在 `PAR-INT-01` 拥有的位置上放替身，与 ADR-0078 Alternatives 否决「合成接入渠道」同一理由；demo 的写入照旧走登记 CLI。
- **写面长期只走 CLI（不进端点表）。** 否决：产品就绪判据是能力覆盖承接目标客户群常规业务形态，租户运营配置员经管理台配置是常规形态；把它长期留给「能进数据库网络的工程师」等于把一项产品能力错登记为一项运维动作。
- **等墙降再立本记录与端点。** 否决：那会让「尚未接线」与「渠道未配置」在写侧继续长同一张脸，墙降当天还要再铺一轮机制才点得亮；读面刚为同一个病付过一次代价（closure-batch 的立批理由）。

## Links

- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置格机制、Decision 五两项未决与替换缝——本记录的端点全部生在这套机制里
- [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)：登记册形状等真渠道证据——本记录不触碰，写面的「谁有权写」照旧等它
- [ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)：隔离读准入的覆盖面与编译期排除——本记录维持其写侧原判
- [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)：读面通例——写面响应形状与「未配置即拒」分界的对照物
- [admin-write-faces/01](../../.scratch/admin-write-faces/issues/01-registry-configuration-has-no-admin-write-face.md)：本记录裁决的三项待裁与事实基线
- [产品基线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：「产品就绪与试点就绪」与「首发范围约束」——写面属长期能力的依据
