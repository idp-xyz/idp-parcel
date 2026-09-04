# `party-commercial` 声明拥有、但今天没有执行器的四处（写侧待裁）

Category: chore
Status: in-progress——01/02/03/04/06 resolved（02 的三层随 06 落 `29085fe`），05 resolved（三处已由 MCP-3 裁，裁决落 [ADR-0104](../../docs/adr/0104-customer-service-rule-content-is-owned-by-party-commercial-and-first-ships-two-items.md)；MCP-4 于分支 `mcp4-pcgaps05` 做完，2026-09-04 由 MCP-1 重放入 main `aa0608f7`..`469bb9ba`，迁移 `party_commercial/0023`；VE 侧后继票 `ve-claims-read-seams/03` 随之立、同日派 MCP-4），07 draft（接受前财务控制策略正文表，2026-09-04 随 admin-write-faces/06 裁②立票，待 `/domain-modeling`）；状态行由 MCP-3 于 2026-09-03 对票面重核后改写，05/07 两格由通道 2 于 2026-09-04 对票面重核后对齐，05 入 main 后由 MCP-1 同日再对齐一次

## 这四处是怎么被看见的

不是专门审出来的，是一次「商务方算不算做完了」的评估撞出来的。评估结论是**机制半边的骨干质量
很高、不是骨架**：商业版本共同不变量、两阶段解析、四格失败代数（`无适用依据` / `适用冲突` /
`解析未决` / `依据未解析`）、参与方身份四表的修订式登记，都有领域守卫加库内 CHECK 双层。

问题不在这些做过的东西上，在**「CONTEXT 声明拥有、代码里只有领域模型」**那一批：类型建得住、
不变量也守得住，但它在 `internal/` 下除了自己的测试没有第二个引用——没有表、没有端口、没有
接入面。规则因此**没有执行器**，一个租户没有办法登记一条这样的事实，下游也就无从校验。

仓库对此在注释里是诚实的（`ports.go` 写着信用政策「没有独立正文表」、供应商协议目录行
「**只有壳**」），诚实不等于做完了，但也意味着**这四票不是发现了隐瞒，是把已知的诚实缺口
排成可裁的形状**。

## 取证锚点

四张票的全部举证锚 **`9d6063c`**。该 SHA 上 `go build ./...` 绿。

本目录落盘时 HEAD 已走到 `309778b`：`git diff --name-only 9d6063c..HEAD` 只有四份 `.md` 与
一份重生成的清点报告，**无 `.go` 无 `.sql`**，故上述锚点在落盘时点仍指得准。记这一句不是凑
字数——共享树上 HEAD 每几分钟就往前走，而票面被引用时身上没有任何东西说明它是在哪个时点取的证。

`.md` 引 `docs/domain/party-commercial/CONTEXT.md` 的小节名与硬句原文，`.sql` 引
`migrations/party_commercial/` 下各文件，Go 引符号名。**一律引符号名与引文，不引行号**
——这些文件正在被多条线改，行号写下去当场就在腐。

## 四票一览

| 票 | 缺口 | 一句话 |
|---|---|---|
| [01](issues/01-channel-account-use-authorization-has-no-executor.md) | 渠道账号使用授权 | CONTEXT 给了它整套规则与独立生命周期，`internal/` 下这个类型除自己的测试零引用 |
| [02](issues/02-pricing-caliber-is-a-required-reference-to-a-hollow-referent.md) | 计价商业口径 | 计价那侧**已经**强制要求口径引用，被引的商业价格政策却没有任何口径列 |
| [03](issues/03-credit-policy-and-supplier-agreement-have-no-content-table.md) | 信用政策 / 供应商协议 | 两者都能入册、能被解析选中，选中之后拿不到正文 |
| [04](issues/04-customer-service-rule-version-is-outside-the-closed-set.md) | 客户服务规则版本 | 九值封闭集里没有它，而 VE 的响应目标与索赔期限本该以它为依据 |

按国际小包业务的实际权重，建议裁决顺序是 01 → 02 → 03 → 04：前两项是业务上会立刻硌到的
（客户自有渠道账号是主流商业形态；汇率口径直接改卖价），后两项是让已有的解析结果真正可用。

## 边界（四票共用）

- **本目录不改表、不建列、不写迁移、不动领域模型、不接端口。** 四票都只写到「差什么、补与不补
  各自的连带」为止，改不改由人裁。
- **不裁实例半边。** 四处缺的都是**机制**：登记这类事实的能力。真实授权、真实牌价类型、真实
  额度属实例半边，本仓无租户因而取不到，四票一律不为它们拟默认值。这条边界与
  [开发主线](../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)
  的两半划分一致。
- 补列会动已施加的迁移，须按本仓惯例**新开序号文件重建约束**，不得改写已施加的迁移。
  `party_commercial` 当前最大序号 `0017`。
- 四处涉及的表**当前全部 0 行**（无租户），所以补不需要数据清洗——这是现在裁比以后裁便宜的
  唯一理由。
- **第三条路对四票都开着**：把出入记进 CONTEXT 或一份 ADR，写明「今天不做，理由是 X，重启
  条件是 Y」。本仓有先例把「今天不做」写成明认而不是默认沉默，**那也是一个合格的结果**，比让
  硬句继续空转好。

## 与已有目录的关系

不塞进 [`first-tenant-runway`](../first-tenant-runway/)：那条线做的是跑通首租户旅程，卡点是
既有链路上的行为缺陷；这四处是**写侧要不要补能力**，性质不同，且四票全部待人裁，塞进去会让
那条线的完成判据依赖四个未决裁断。

不塞进 [`settlement-register-context-gaps`](../settlement-register-context-gaps/)：形状同构
（都是登记册与 CONTEXT 硬句的出入），但那是 `settlement-accounting`，已 resolved。本目录是
同一种做法在 `party-commercial` 上的第二次应用。
