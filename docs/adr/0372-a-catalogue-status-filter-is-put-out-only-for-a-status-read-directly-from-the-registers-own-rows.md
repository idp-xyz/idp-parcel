# ADR-0372：目录读口的状态筛选维只放「由本册登记行直接读出」的状态——领域规则在多行之上求出的结论、按读取时点现算的读数、管理台的呈现词不放成筛选维；答复照旧带它们，管理台对它们只在页内筛并标明

Status: Proposed（2026-11-09，票 [catalogue-read-pagination-rollout/16](../../.scratch/catalogue-read-pagination-rollout/issues/16-derived-state-filter-dimensions.md) 的作者会话起草；接受与否待 TF、NO、PP、SA owner 复核，越权风险点见文末。拟收窄 ADR-0144 决定四「有状态的册至少放出状态一维」的适用范围。起草能力边界：读过 [ADR-0144](./0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 全文；该票与其 spec「横贯各子票的取舍」；下表各册的声明（各上下文 `ports/catalogue_pages.go`）、读面与答复体；`domain.CurrentCarrierAcceptance`、`DeliveryHandoverNoteState` 的 `ScopeEstablished`、`domain.JudgeIdentityBasis` 与处置编排的 `headsInConflict`、`SelectablePriceCardPeriod`；管理台 `priceCardStatus`、`billProgress` 两个函数；TF、NO、PP、SA 与 customs-compliance 的 `CONTEXT.md` 生命周期节。没读：各上下文其余读面、管理台其余页面）<!-- 索引注记 -->｜**草案**：Status 为 Proposed，尚非依据
Date: 2026-11-09

## Context

ADR-0144 决定四要求「有状态的册至少放出状态一维」，维度名取答复体里的 JSON 字段名，值属封闭词表的维按词表校验。第二批迁移（spec [catalogue-read-pagination-rollout](../../.scratch/catalogue-read-pagination-rollout/spec.md)）的几张子票给一批册子没放状态维，理由都是「状态不是登记表上的一列，是读口或页面现算的」；票 16 收拢这几处，逐册判断状态能否在 SQL 里由本册自己的行导出（`CASE` 或 `EXISTS`），能的补上，不能的回本记录改决定四的适用范围，不在代码里默默豁免。

**能导出的已补上**（票 16 的分支，各上下文一笔）。它们有一个共同的形状：状态就是本行某一列在不在，或以本行为键的留痕册上有没有那一行，领域读的也正是这一格——

| 册 | 状态维 | 落列 |
|---|---|---|
| 就绪判断、提交授权 | `effective` | 本行撤销时刻缺席（领域 `Effective()`） |
| 送站预报 | `withdrawn` | 撤回册上有这份预报那一行（领域 `Withdrawn()`） |
| 送站交接单 | `closed` | 关闭册上有这张单那一行（领域 `Closed()`） |
| 价卡 | `retired` | 退役册上有这一版那一行（CONTEXT「已发布 → 已退役」） |
| 客户对账单、待对账队列 | `voided` | 本行作废时刻在场（CONTEXT「已发布 → 已作废」） |
| 生产权威区间 | `open` | 本行终点缺席（`domain.AuthorityInterval` 的 `To` 为零值） |
| 暂停决定 | `resumed` | 恢复册上有这条暂停那一行（一个暂停至多一次恢复） |

**导不出的有五处**（取证钉 `f117de0aa`，本票动手前的 `origin/main`；这几处的派生代码本票未改），分属三类：

1. **领域规则在多行之上求出的结论。**
   - 订舱请求的状态词（`PENDING_CONFIRMATION`、`WAITLISTED`、`CONFIRMED` 等）：读口从这次订舱名下的全部承运答复里经 `domain.CurrentCarrierAcceptance` 选出当前那一份，再比接受量与申请量。那个函数的头注写明「当前那一份」只在它一处定义，查阅口读订舱状态词也经它，不各按答复值或排序另选一遍；答复凑不成一段答复史时它答错误，不挑一份凑一个词。
   - 送站交接单的范围成立（`scopeEstablished`）：`DeliveryHandoverNoteState` 的 `ScopeEstablished` 按声称的每个客户账户的绑定结论判全部已确认（[ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定三「成立」），头注写明范围成立是派生、不落列。它不是交接单的生命周期——交接单的生命周期「开放 → 已关闭」已作 `closed` 放出。
   - 身份复核单元（一件作业实物一行）此刻冲突与否、处置过没有：领域把这件实物各条判断链的当前有效版本合起来作身份判断（`domain.JudgeIdentityBasis`，处置编排的 `headsInConflict` 经它判冲突）；答复里没有实物一级的状态格，复核页据各链头自己判。
2. **按读取时点现算的读数。** 管理台价卡页的五态（生效中、未生效、已过期、已截止、已退役；`priceCardStatus(row, now)`）拿此刻对适用期与收尾各格现算。parcel-pricing `CONTEXT.md` 写明截止只收窄此后可选的区间、不是状态迁移；领域状态只有「已发布 → 已退役」，已作 `retired` 放出。
3. **册上没有领域状态，状态词是管理台的呈现词。** 供应商账单接收册的接收进度（待审核、审核人未配置、有争议待再审、已审完；`billProgress`）是页面从读口的几个和派生的，`supplier-bills.ts` 文件头与它上方的词表块注都自称呈现词、不是领域状态；其中「审核人未配置」一格还读此刻审核授权册上有没有这一组。

**三类各自为什么不能照抄进 SQL。**

- 第一类照抄就是同一条领域规则的第二处定义（AGENTS.md 红线「单一权威」）。两处判法一旦分叉，筛出的行与答复里的状态格对不上；坏行上 Go 答错误、SQL 却照样给一个词，筛选把一行说不出状态的订舱悄悄归进某一类。
- 第二类的判定随时间流过而变，册上没有任何写入：同一个游标、同一组条件，翻第一页与翻第二页之间的一刻，一行就可能从「生效中」挪进「已过期」，从后面的页里静默消失。词表也不是领域的，是页面的。
- 第三类没有领域词表可取（决定四要的是领域那份封闭词表）；把呈现词搬到服务端，就是在服务端另立一份状态规则，而票 16 明写「不改状态的判定规则本身」。

先读后筛（在 Go 里读一批再按状态筛）也不行：`total` 不再是真数，页会短，正是 ADR-0144 Context 要消灭的「在截断的一批上筛出假的没有」。

## 候选与反方

**甲 · 在 SQL 里照写一份判定。** 能让五处都有筛选维。反方：第一类是第二处定义，第二类随时间漂移，第三类凭空立了一份状态规则。否决。

**乙 · 读后在 Go 里筛。** 判定仍只在一处。反方：`total` 与页大小都不再成立，要筛全册就得读全册。否决。

**丙 · 在写面把领域判定的结果随写入落列，再按列筛。** 判定仍只在领域一处，筛与答读同一格。反方：动写口，是逐册的实施票，不是一条读口规则能代办的；对第二类不成立——时间流过不是一次写入，没有落列的时机；对第三类也不成立——没有领域状态可落。保留为第一类的出路（见决定二）。

**丁 · 收窄决定四的适用范围（本记录）。** 代价：第一类在落列之前、第二三类长期只能在页内筛。

## Decision

**一、决定四「有状态的册至少放出状态一维」里的「状态」，指能由本册登记行直接读出的状态**：本行某一列（含它在不在），或以本行为键的留痕册上有没有那一行（撤销、撤回、关闭、退役、恢复这一类，按留痕册主键探 `EXISTS`）。读面在 SQL 里导出这一维，筛与答复里的状态格、与领域读的是同一格事实；领域状态是一个布尔时，词表取答复体里那个布尔的两种写法 `true` / `false`（同 customs-compliance 提交版本册 `isCurrent` 的先例）。

**二、下面三类不放成筛选维**，各册在声明头注写明归哪一类、指到本记录：

1. 领域规则在多行之上求出的结论。要服务端按它筛，先另立写面票，把领域判出的结果随写入落成本册的列（判定仍只在领域一处），再按决定一放出。
2. 按读取时点现算的读数。不放，也不落列。
3. 册上没有领域状态、状态词是管理台呈现词的。不放；要放，先由 `CONTEXT.md` 或另一份 ADR 立这一份领域状态。

**三、答复照旧带这些格**（订舱的状态词、交接单的 `scopeEstablished` 有；复核单元、价卡五态、接收进度没有，由页面现算）。管理台可以在已取回的一页上按它们筛，但必须在页上标明只筛本页，不把页内筛的结果说成整册的（管理台已有「状态只筛本页」一注）。

**四、新迁入的册按决定一、二判**；判不准的回本记录。

## Consequences

- ADR-0144 决定四其余各句不变。各册声明头注里指到本记录的那几句，接受与否都不用改：它们只写编号，不写本记录的状态。本记录接受时，ADR-0144 的 `Status` 行加一条前向指针、并新建 Links 一节放同一条（ADR-0144 今天没有 Links 节）（被收窄的条款即「有状态的册至少放出状态一维」一句），`.scratch/catalogue-read-pagination-rollout/spec.md`「横贯各子票的取舍」里「可排维、筛选维从小放」那一条的「有状态（或同类封闭词表）的册放出那一维作筛选维」照此改读。
- 订舱册、交接单的范围成立、复核单元三处若运营确需服务端筛，各立写面落列票，归各自上下文 owner。
- 票 14 接各页时，对这几册保留页内筛并标明只筛本页；已放出状态维的那几册改为下推。

## 越权风险点

1. **「留痕册上有没有那一行」算不算本册**：`EXISTS` 也读了另一张表。本记录把界划在「有没有那一行」（领域也只看在不在）与「那几行合起来怎么判」之间；若评审认为前者也是第二处定义，决定一要收得更窄，已补的 `withdrawn`、`closed`、`retired`、`resumed` 四维随之回退。
2. **布尔词表取 `true` / `false`**：若产品要领域词（如「已就绪 / 不再就绪」的代码），要先立领域枚举，属另一项决定。
3. **价卡此刻生效与否**：若运营要服务端按它筛，可以让调用方把时点作选择器传入、读面按那一刻判——那是新的查询语义，本记录不定，归 PP owner。
4. **订舱状态落列**会动答复编排的写口，归 TF owner 复核；交接单范围成立、复核单元冲突落列归 NO owner。

## Links

- [ADR-0144](./0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md)：拟收窄其决定四「有状态的册至少放出状态一维」
- [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)：订舱答复史与「当前那一份」
- [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)：交接单范围成立
- [ADR-0234](./0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md)：价卡截止与退役
- [ADR-0357](./0357-a-supplier-bill-number-is-an-external-identifier-of-the-claim-and-an-audit-passes-disputes-or-rejects-amount-ranges-of-a-line.md)：账单审核按金额段给结果
- [票 catalogue-read-pagination-rollout/16](../../.scratch/catalogue-read-pagination-rollout/issues/16-derived-state-filter-dimensions.md)、[spec](../../.scratch/catalogue-read-pagination-rollout/spec.md)
