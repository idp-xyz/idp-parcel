# 16 过期注释：运营侧接入面的认证不再写「属 `PAR-INT-01` 待提供」

Category: enhancement
Status: in-progress——2026-09-25 通道 2 随 ADR-0151 补立并认领（通道 4 于 2026-09-24 已改了一部分，未提交）
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md)
地盘：`cmd/parcel-api` 与各上下文 `adapters/http`、`adapters/registrationjson` 里的 Go 注释；只改注释，不改代码与测试逻辑。
出处：[ADR-0100](../../../docs/adr/0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定一（`PAR-INT-01` 只管客户生产委托接入渠道）；[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)；[ADR-0151](../../../docs/adr/0151-unassigned-command-faces-get-their-families.md) Consequences 第三条。

## 为什么

代码注释里运营侧各口的认证仍写着「属 `PAR-INT-01` 待提供」或「实例半边」。自 ADR-0100 起，`PAR-INT-01` 只管客户渠道，运营侧各族的渠道形态都归产品。这类注释会让读代码的 agent 以为这些口在等租户或外部证据，而它们等的是自家的操作者渠道票。

## 做什么

运营侧各口的注释改为指向各自的渠道族：

- 登记写面、目录与运营查阅面、商业发布：操作者渠道（ADR-0100）。
- 一线作业事实：操作者渠道的「作业事实登记」能力面（ADR-0149）。
- 外部结果与外部资金事实：集成客户端族（ADR-0149）；哪家监管或报关来源送回执，仍是租户取值 `PAR-INT-03`。
- 委托侧运营决定口与 TF 四个管理台写面：操作者渠道的「运营决定」能力面（ADR-0151）。
- 各包「未配置」哨兵：按提交方分族，各族真 Intake 都未就位。

## 不做

- 客户侧各口（提交、撤回、取消、补充、资料修订、客户追踪视图、索赔）照旧写 `PAR-INT-01`，直到 ADR-0139 有结论；端点表客户侧两行只把「（实例半边）」换成指向 ADR-0139 草案的说法。
- `internal/accessidentity` 里客户渠道那一半的注释（`doc.go` 那一段归 [03](./03-operator-envelope-and-answer-algebra.md) 第 4 条）。
- `PAR-COM-14` 等授权请求映射的「实例半边」口径（归 [psb/07](../../product-strategy-boundary/issues/07-pc-authorization-coordinates-and-role-models.md)）。
- 与在途分支 `mcp2-psb17` 重叠的文件。

## 完成判据

- 运营侧各口的 Go 注释里不再把认证归给 `PAR-INT-01` 或「实例半边」；剩下的 `PAR-INT-01` 都属客户侧或上列「不做」。
- `gofmt -l` 无输出，`go build ./...` 与 `go vet ./...` 退 0。
