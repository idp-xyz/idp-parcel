# 02 其余登记册逐个接在线登记口与登记签：网络、关务、商业、VE、代收

Category: enhancement
Status: ready-for-agent——形状已由 [ADR-0085](../../../docs/adr/0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)
与票 01 的切片 01a/01b 定死，本票只是逐上下文照做
Blocked by: 无

## 为什么是一张票而不是五张

票 [01](./01-registry-configuration-has-no-admin-write-face.md) 的「实施切片」写的是
「02+（后续票）：网络、关务、商业、VE、代收各上下文逐册跟进，每票照 01a 形状」。**那时立五张票
是为了让五个会话并行**；2026-09-01 起 MCP-2/3/4 相继 crash，只剩一个会话在读跟踪器，五张形状
一模一样的票只会把同一份范围抄五遍，而每抄一遍就多一处会各自变旧的描述。故合为一张，按上下文
分片（02a..02e），每片自带完成判据，做完在本票记一条。

若日后恢复多会话并行，按片拆票即可——片的边界就是票的边界，不必重写范围。

## 形状（照 01a，不重新裁）

每个上下文一片，每片做四件：

1. `adapters/http`：为该上下文的每类登记增一个 `XxxRegistrationIntake` 接口与一个端点构造函数；
   `UnconfiguredIntake` 补对应实现；传输层测试含「隔离读 Intake 装不进登记口」的编译期断言。
2. `cmd/parcel-api`：端点表加行（**字面量 `UnconfiguredIntake{}`**，写准入不另立形）、
   `businessEndpointProbes` 加探针、unwired 占位补方法。**隔离读放行表零改动**——写行不入格。
3. 生产装配：第二参接真（登记用例 + `db.Transactor()` 事务包装，形照登记 CLI 的 execute）。
4. 管理台该页加「登记」签，复用 `pages/pricing/RegistrationPanel` 的三态呈现。

## 分片
- **02a · 网络**（`cmd/parcel-ve-register` 之外的网络登记：节点、连接、线路、服务区域、
  服务日历、可用性调整、路由策略——按 `cmd/parcel-network-register` 的命令族切）
- **02b · 关务**——**范围要先分辨，见下**。立票时按读面那四个入口猜成五类，实测比这多得多，
  且其中一半按本票自己的判据不该进来。
- **02c · 商业**（`cmd/parcel-commercial-register` 的命令族：服务产品、规则包、策略、参与方身份
  与关系、产品—渠道映射）
- **02d · VE**（`cmd/parcel-ve-register` 的命令族：里程碑映射、分诊规则、通知策略、索赔资格、
  索赔授权、披露策略）
- ~~**02e · 代收**~~——**已排除**（用户 2026-09-01 裁，理由见下）。代收的在线操作面另立票，
  且先裁操作者授权模型。

片内若某类登记的用例尚不存在，如实记「无用例可接」并跳过该类——**不为了凑齐而造用例**。

### 02b 的范围实测（2026-09-01 MCP-5，核于 `844bb11`）

`internal/customscompliance/application` 下实有**十二个**用例方法，不是立票时写的五类——
立票那五类是照读面四个入口猜的，猜错了。逐个按本票的判据（改的是「这个租户怎么配置」还是
「案上此刻的事实」）分辨：

**配置类（属本票）**：`RegisterInterpretationRule`（解释规则）、`RegisterGateCatalog`（门禁
    目录）、`RegisterCandidatePort`（候选口岸）、`RegisterDeclarationPath`（申报路径）。
    这四类改的是租户的规则与目录，与价卡、参考序列同类。

**案件事实类（不属本票）**：`RegisterReadiness` / `RevokeReadiness`（某个案件此刻就绪与否）、
    `GrantSubmissionAuthority` / `RevokeSubmissionAuthority`（对某个案件的提交授权）、
    `RegisterObligationCatalog` / `RegisterObligationItem`（某个案件的关闭义务）、
    `RegisterGateFinding`（对某个案件的门禁发现）。这七类改的是**案上此刻的事实**，不是租户
    配置——它们与代收那七个用例同类，且 `Revoke*` 两个更明显：撤销不是登记，是状态推进。

**判据不是我新造的**：它就是 02e 那条裁定用的同一句，本节只是把它应用在关务片内部。**范围的
分界线不在上下文之间，在用例之间**——立票时按上下文分片是为了分工方便，不代表一个上下文里的
用例同属一类。02d（VE）做到时同样逐个分辨，02a（网络）与 02c（商业）也要先过这一遍。

**裁定（2026-09-01 用户）：02b 收敛为上述四类配置登记**——解释规则、门禁目录、候选口岸、
申报路径。那七类案件事实的在线操作面与代收的一并另立票，先裁操作者授权模型（同一条判据、
同一个未决）。

### 02e 已排除（2026-09-01 用户裁）

ADR-0085 Decision 二的措辞是「有登记用例与 CLI 先例的**运营配置册**逐上下文进端点表」。而代收那
七个用例（开分户账、记账、登记代收指令、接受代收事实、登记差异事项、形成回汇批次、汇付交接）
**不是配置登记，是业务操作**：它们改的是一本受托保管账上的资金位置与余额，不是「这个租户怎么
配置」。价卡与参考序列那两个是配置（登记一份可执行方案版本供日后计价采用），两者不同类。

若把业务命令面也按本票形状铺开，铺的就不再是「配置写面」而是**代收业务的在线操作台**——那是另一
件事，范围、授权模型与操作者角色都不同（ADR-0085 Decision 二末句把治理登记册排除在首批之外，
理由正是「操作者授权模型单独裁」）。

**裁定：排除。** 本票分片收敛为 **02a..02d**；代收的在线操作面另立票，且**先裁操作者授权模型**
再谈实施——判据与 ADR-0085 把治理登记册排除在首批之外那一条相同。

同一问对 VE（02d）的部分命令也可能成立：做到那一片时**逐个用例分辨「配置」与「业务操作」**，
不整片一刀切。分辨的判据就是本节这一条——改的是「这个租户怎么配置」还是「账上/案上此刻的事实」。

## 红线（逐字继承票 01）

- 不造任何「开发用」采信身份让表单能提交；隔离读准入（ADR-0078）不得扩到写行。
- 写面一律复用既有登记用例与命令：不可覆盖、更正走版本链、停用走状态推进；不开任何行级
  UPDATE/DELETE 面。
- 实例值留空拒默认；隔离合成只记 `S`。
- **表单不逐字段建**：「渠道原始载荷 → 登记快照」的翻译属渠道接入契约、随 `PAR-INT-01` 提供
  （ADR-0085 决定三）。收登记快照 JSON 本体，形状与各自受控登记 CLI 的 `-file` 同源
  （判据与 01b 同一条，见票 01 文末 Comment）。

## 完成判据（每片各自达成）

- 全仓 `gofmt -l` 无输出、`go build`、`go vet` 绿；`go test -count=1 ./...` 绿且注明含不含真库。
- `apps/admin-web` 的 `tsc --noEmit` 无输出、`pnpm build` 绿。
- 装配测试钉住：新端点在未配置态答 403，隔离读启用态**仍答 403**（写行不入放行表）。
- 本票记一条，写明该片接了哪几类登记、哪几类因无用例而跳过。
## 参照

[ADR-0085](../../../docs/adr/0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)、
[ADR-0055](../../../docs/adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)、
[ADR-0078](../../../docs/adr/0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)；
切片 01a 的实现（`internal/parcelpricing/adapters/http/register_price_card.go` 与
`cmd/parcel-api/assemble_pricing_registration.go`）、01b 的表单区
（`apps/admin-web/src/pages/pricing/RegistrationPanel.tsx`）。
