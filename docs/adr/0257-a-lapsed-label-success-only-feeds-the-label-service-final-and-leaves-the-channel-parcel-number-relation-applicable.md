# ADR-0257：面单成功结果按有效期不可逆失效，只进面单服务终局判断，不改渠道包裹件号关联的适用状态——外部标识关联没有`已失效`一格

Status: Accepted（2026-10-21，Claude Code 会话代 PS owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/06，出处审计 [audit/ps.md](../../.scratch/docs-consistency/audit/ps.md) PS-22（取证 [audit/ps-code.md](../../.scratch/docs-consistency/audit/ps-code.md) 第 10 条）。回答 [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 决定七「已失效没有执行器」所留的那一格，并更正它的理由，无条款停用；ADR-0184 正文不改，只在其 Status 行与 Links 加前向指针。代码不改。越权风险点见文末，待 PS owner 复核，第 1 点知会 NO owner。）<!-- 索引注记 -->｜**分工**：有效期届满是面单对渠道服务的可用性到头，判在面单服务终局判断里——那笔成功不再阻止受控关闭路径形成终局；号码仍标识那件实物，渠道包裹件号关联照旧`适用`，身份核对照旧把它当候选，消费方按包裹当前服务结果另行限制后续动作。**终态**：外部标识关联只有`已作废`与`已被替代`两种终态，`已失效`从 PS CONTEXT 生命周期删去；渠道日后若主动收回某个号，那是新的来源事实，另立记录
Date: 2026-10-21

## Context

- PS CONTEXT「外部标识关联」生命周期：「适用 → 已失效：渠道确认或接受时固定的有效期规则届满」。
- [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 决定七：「已失效没有执行器：有效期规则今天没有登记处，本记录不造。」代码 `domain.ExternalIdentifierChangeKind` 只有作废与替代两格，头注照此写。
- [ADR-0119](./0119-label-validity-is-a-declaration-slot-on-the-final-rule-content.md)（2026-09-07，早于 ADR-0184）早已把有效期立为终局规则声明上的一格：起算时刻种类加时长，缺席即不失效。代码里 `ports.LabelValidityRuleView` 由 `DeclaredLabelValidityRule` 实现，经面单服务终局判断编排喂给 `LabelServiceFinalInput.LapsedTransactions`（取证于 main `f329f5059`）。
- 于是同一件「面单成功结果已不可逆失效」：终局判断认它，渠道包裹件号关联仍是`适用`，身份核对仍把它当候选。三处说法——CONTEXT 有这一格、ADR-0184 说没有登记处、代码终局判断已经在判——互相对不上，也没有停用关系。

## Decision

**一、有效期届满只进面单服务终局判断。**

- 有效期规则判出某笔成功面单结果已不可逆失效，那笔成功不再阻止受控关闭路径形成终局（PS CONTEXT「首个面单渠道服务产品默认跨同一包裹的全部相关面单交易判断终局」一条）。这一分工 ADR-0119 与现行代码已经做到，本记录确认它。

**二、它不改外部标识关联的适用状态。**

- 渠道包裹件号关联照旧`适用`；按外部标识核对包裹身份时，号码照旧命中那件包裹。
- 理由：有效期届满说的是这张面单还能不能拿去用渠道服务，不是号码被收回。号码印在实物上，到站扫到它的仍是那一件；把它判成不再标识这件包裹，节点会把一件身份明确的实物当作待识别。
- 这与 CONTEXT「包裹取消、终局或委托后续变化不改变外部标识关联的适用状态：已取消或已终局的包裹仍按其号码被认出，由消费方据当前服务结果另行限制后续动作」是同一条理由。面单失效后包裹该不该再走渠道，由消费方按包裹当前服务结果与面单交易结果判。
- `已作废`不同：渠道作废是渠道方明确收回了那笔交易给这件包裹的号，关联照 ADR-0184 决定七转`已作废`。

**三、外部标识关联只有两种终态。**

- `已作废`与`已被替代`。`已失效`一格从 PS CONTEXT 生命周期删去，Rules「外部标识作废、失效或被替代只改变适用关系」一句随之去掉「失效」。
- 渠道日后若主动通知某个号不再标识这件包裹（号码回收），那是新的来源事实，形状与终态另立记录，不借用有效期规则。

**四、更正 ADR-0184 决定七的理由。**

- 「有效期规则今天没有登记处」与 ADR-0119 不符。不形成`已失效`的结论不变，理由改为决定二。

## 候选与反方

- **判出失效时同时形成关联的`已失效`**（加 `ExternalIdentifierLapsed`、立代码票）。否决：有效期判断今天只在终局判断时按需算出，没有一个「失效发生了」的来源事实可挂；为它造一个执行器，等于在终局判断里写副作用。更要紧的是结果不对：节点会认不出一件身份明确的实物。
- **保留 CONTEXT 的`已失效`一格作日后之用。** 否决：CONTEXT 写今天生效的规则；没有来路的终态留在生命周期里，读者会去找它的执行器。

## 对已接受记录的作用

- **回答** ADR-0184 决定七「已失效没有执行器：有效期规则今天没有登记处，本记录不造。」所留的那一格：不造，理由按本记录决定二；「今天没有登记处」一语据 ADR-0119 更正。无条款停用。ADR-0184 决定一「已作废、已被替代、已失效都不回到适用」一句不改，其中`已失效`自此没有来路。
- [ADR-0119](./0119-label-validity-is-a-declaration-slot-on-the-final-rule-content.md)：不动，本记录确认它的分工。

## Consequences

- PS CONTEXT：「外部标识关联」生命周期删去「适用 → 已失效」一行，终态一行改为两种；Rules「外部标识作废、失效或被替代只改变适用关系」改为「作废或被替代」。
- 代码不改。`ExternalIdentifierChangeKind` 头注「有效期规则今天没有登记处」一句改为本记录的理由（随 [docs-consistency/code/38](../../.scratch/docs-consistency/code/38-parcel-shipment-comments-follow-docs-consistency-06.md)）。

## 越权风险点

1. **身份核对照旧认一个面单已失效的号**（PS owner，知会 NO owner）。节点扫到它会命中那件包裹，后续动作要靠消费方读包裹当前服务结果与面单交易结果来限制。若 owner 认为失效的号应当退出核对，那是改本记录决定二，并要同时回答节点如何认出这件实物。
2. **删去`已失效`一格**（PS owner）。日后若出现渠道回收号码的来源，需另立记录定它的终态名，不必沿用`已失效`。

## Links

- [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md)：回答决定七所留的那一格并更正其理由
- [ADR-0119](./0119-label-validity-is-a-declaration-slot-on-the-final-rule-content.md)：有效期是终局规则声明上的一格
- [ADR-0185](./0185-external-identifier-relation-identity-includes-its-formation-basis.md)：关联的键带形成依据
- [PS CONTEXT](../domain/parcel-shipment/CONTEXT.md)：「外部标识关联」生命周期、Rules 外部标识各条
- 票 `.scratch/docs-consistency/issues/06-ps-docs.md`；代码票 `.scratch/docs-consistency/code/38-parcel-shipment-comments-follow-docs-consistency-06.md`
