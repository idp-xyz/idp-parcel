# 登记册配置零管理台写面：写入长期只有工程师跑 CLI 一条路，且无任何排期票

Category: enhancement
Status: in-progress

来源：2026-08-31 MCP-3 频道问答。用户质疑「很多 CRUD 都没有，是不是原来的文档有问题」。
核对结论：现状自洽——U/D 被领域故意替换（登记不可覆盖、更正翻旧插新、停用走状态推进
不删行），C 存在但形状是命令、入口只有 CLI；文档也给长期能力留了门（产品基线
[首发范围约束](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)
明写「不应被误读为长期产品能力的永久禁止」）。真正的空档是：**没有任何一张票在规划
管理台写面**。本票补这个空档，先裁方向再谈实施。

## 事实基线（取证于 `4b35815`；计数即论点，故按例外锚 SHA）

- `cmd/parcel-api` 装配表（`assembleBusinessEndpoints`）共 39 个业务端点：31 查阅 + 8 命令。
  命令端点全部挂字面量 `UnconfiguredIntake{}`——按
  [ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)
  未配置即拒（403 `ACCESS_CHANNEL_NOT_CONFIGURED`）。隔离读准入
  （[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)）
  只经由查阅行的 Intake 变量换值，该函数注释原话：「命令面与客户查阅面的字面量
  UnconfiguredIntake{} 不经由任何变量，读这段代码就能看出它们换不了」。
- 管理台 `apps/admin-web` 的写调用只有委托演示页一处（`pages/shipment-request/api.ts`
  的 `post`，三命令：提交/撤回/取消包裹），打到的就是上述被拒的命令端点；配置类登记册
  （价卡、参考系列、网络、关务配置、商业载体与身份、VE 目录、代收、治理）**零写表单**。
- 登记册的写入机制本身是齐的，入口只有 CLI：七个登记 CLI 由 `scripts/demo-seeds/seed.sh`
  逐个调用（商业、计价、网络、关务、VE、代收、治理）。「无写入方」一类墙票已收口——
  [syn-wall-door-audit/06](../../syn-wall-door-audit/issues/06-cc-case-config-registries-have-no-writer.md)
  与 [cc-case-requirement-rule-registry/01](../../cc-case-requirement-rule-registry/issues/01-case-requirement-rule-has-no-writer.md)
  均 resolved（后者的 B 半边交付了 `cmd/parcel-customs-register` 受控 CLI）。
- 全仓剩的唯一一堵墙是接入渠道：
  [syn-wall-door-audit/01](../../syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md)
  （needs-info，重启条件 `PAR-INT-01` 最低证据；
  [ADR-0072](../../../docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)
  维持对「预先给渠道拟运行时登记表」的否决）。

## 缺口

产品就绪里程碑已成立（2026-08-28 受托认可，见基线「机制半边现状」节），下一站是商务
洽谈与租户试点。届时「工程师跑 CLI 灌 JSON」不能是长期配置方式：租户的运营配置员要能
在管理台上登价卡、登网络、登商业载体。这半边今天没有票、没有 ADR、没有排期——不是
「文档错了」，是「文档留了门但没人立票走进去」。

## 本票要的裁决（needs-triage 清单，按序）

1. **准入形**：管理台写面的「谁有权写」需要什么最低证据？两个候选方向：
   - a. 与接入渠道同一堵墙——写面整体等 `PAR-INT-01`，本票裁完即挂起，租户证据到位
     再启；
   - b. 运营侧配置写入是另一种准入形（类比 ADR-0078 为查阅面立过的先例），墙降前
     机制半边即可开工。
   这是难逆转取舍，裁 b 需要新 ADR。
2. **范围分类**：哪些登记册该有管理台写面、哪些长期留 CLI（批量种子、迁移类）、哪些
   两者都要。建议按「登记频次 × 操作者角色」裁，不按实现难度。
3. **机制/实例切分线**：表单形状、命令映射、冲突/幂等/未决答案的呈现属机制半边；提交
   入口的采信属实例半边。墙降前允许做到哪一步（例如「表单 + 校验预览、禁提交」算不算
   机制半边）。

## 红线（实施票逐字继承）

- 不造任何「开发用」采信身份让表单能提交——`assembleBusinessEndpoints` 的文件注释点名
  禁止的正是它；隔离读准入（ADR-0078）不得扩到写行。
- 写面一律复用既有登记用例与命令：不可覆盖、更正走版本链、停用走状态推进；不开任何
  行级 UPDATE/DELETE 面。
- 实例值留空拒默认；隔离合成只记 `S`。

## 参照

[ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)、
[ADR-0072](../../../docs/adr/0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)、
[ADR-0077](../../../docs/adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)、
[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)；
产品基线「产品就绪与试点就绪」「首发范围约束」两节；登记 CLI 的先例形状
（`cmd/parcel-customs-register` 的封闭命令族与退出码 0/1/2/3）。

## 裁决（2026-08-31，[ADR-0085](../../../docs/adr/0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)）

用户经频道 3 授权本会话「作为业务和系统专家直接开干」，三项按序裁定：

1. **准入形 → a（同一堵墙）**：写准入不另立形，登记写端点以字面量 `UnconfiguredIntake{}`
   进端点表，与其余命令面同等 `PAR-INT-01` 证据；ADR-0078 的隔离读准入不扩写行。
   机制半边因此**不必等墙降**——端点、Intake 接口与表单页现在就铺，墙降当天在装配点
   逐端点换真 Intake 即点亮。
2. **范围分类**：有登记用例与 CLI 先例的运营配置册逐上下文进端点表；批量/迁移类长期
   留 CLI（CLI 不退场，两口消费同一登记用例）；治理登记册（无租户维，ADR-0083）不入
   首批，操作者授权模型单独裁。
3. **机制/实例切分线**：端点 + Intake 接口 + 未配置实现 + 表单页三态呈现（403 未配置 /
   登记册治理答案 / 未决）属机制半边；「渠道原始载荷 → 登记快照」的翻译与操作者认证
   属渠道接入契约，随 `PAR-INT-01` 提供。

## 实施切片

- **01a（本票首切片，MCP-3 在做）**：parcel-pricing 两类登记端点（价卡、参考系列）——
  `adapters/http` 增 `PriceCardRegistrationIntake`/`ReferenceSeriesRegistrationIntake`
  两接口、两端点构造函数、`UnconfiguredIntake` 对应实现与传输层测试（含钉住隔离读
  Intake 装不进登记口的断言）；`cmd/parcel-pricing-register` 文件头口径随裁决更正。
  装配行交 MCP-1：建议路径 `POST /pricing-price-card-registrations`、
  `POST /pricing-reference-series-registrations`，第二参为「登记用例 + `db.Transactor()`
  事务包装」（形状照登记 CLI 的 execute），或先以 unwired 守卫顶住。
- **01b（阶段二，等 MCP-1 装配广播）**：管理台价卡/参考系列页增写表单，三态如实呈现，
  未配置态文案「接入渠道未配置」。
- **02+（后续票）**：网络、关务、商业、VE、代收各上下文逐册跟进，每票照 01a 形状。

## Comments

- 2026-08-31 · MCP-3：按频道指示立票（核对「CRUD 缺口」质疑后答「文档自洽、缺写面
  排期票」，用户指示「起吧」）。取证时点：`4b35815` 为本地 HEAD（2026-08-31 09:20 UTC 查），
  工作树除他人的 `docs/wooolink/` 外干净。
- 2026-08-31 · MCP-3：用户随后指示「直接开干」→ 裁决落 ADR-0085，本票 needs-triage →
  in-progress，切片 01a 上下文侧完成：`gofmt` 零信号、`go build`/`go vet` 计价与 CLI 包
  零信号、`go test -count=1 ./internal/parcelpricing/adapters/http/` 全绿（纯传输层，
  不含真库用例）。交付分支 `t3-admin-write-faces`（隔离 worktree，SHA 见频道交活消息）；
  README 侧只带 0085 自己那一行——0084 行是 MCP-5 未提交在途改动，按「add 前逐块核」
  不卷带。
