# ADR-0275：受控关闭与重开只由运营侧授权角色决定、合同委派不参与；渠道服务方或代理商叫停是外部限制，不作请求方

Status: Accepted（2026-10-21，Claude Code 会话代 PS、PC owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/10（10a 半边），出处审计 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) SC·S-04、SC·S-05。无条款停用：相抵的是 parcel-shipment CONTEXT 与 SCENARIOS 场景二里「合同委派可直接决定」的例外句，已接受 ADR 里没有这一句；回答 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) Alternatives 候选丁留给 PS owner 的一问（渠道方或代理商的请求要不要先留痕），回答 [ADR-0183](./0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md) 越权风险点 1（操作者能否指名委托之外的请求方，例如代理商）与 Consequences「它们要提出关闭请求，另有来处时另行裁定」，都不停用其条款。代码只改注释与 Covers，随票 [docs-consistency/code/56](../../.scratch/docs-consistency/code/56-closure-and-reopening-comments-still-cite-the-deleted-delegation-exception.md)。越权风险点见文末，待 PS、PC owner 复核。）<!-- 索引注记 -->｜**决定方**：受控关闭与重开的正式决定只由运营企业责任法人授权的运营角色凭授权规则形成，合同委派不参与；货主、渠道服务方或代理商都不能经合同直接形成这两种决定。**货主的要求**：作为请求证据落在决定上（货主指令关闭、重开时货主新的有效授权），不另立请求册。**渠道方叫停**：渠道服务方或代理商叫停不是请求，关闭责任来源记为外部限制，请求方缺席；它的「恢复」是来源责任方对该限制的有效解除，解除只恢复可申请重开的资格，重开仍由授权角色决定。代理商本就是委托上的客户账户时，按货主算

Date: 2026-10-21

## Context

以下所引 parcel-shipment CONTEXT 与 SCENARIOS 原文是本记录接受前的文字，已随本记录改写（见 Consequences）。

- parcel-shipment CONTEXT 与 party-commercial CONTEXT 对同一件事说法相反，两份都是 Confirmed，谁也没有停用谁：
  - parcel-shipment CONTEXT：「货主、渠道服务方和代理商只能提出请求，除非客户合同或明确授权把其纳入可直接形成决定的授权角色范围」。SCENARIOS 场景二跟着它写了例外：「除非客户合同或明确授权已经把请求方委派为可直接决定的角色」「除非其已被合同明确委派为决定角色」。
  - party-commercial CONTEXT：「两者都是运营侧凭授权规则形成的决定，合同委派不参与。」
- 代码跟的是 party-commercial：`internal/partycommercial/domain/authority_grant.go` 的 `formedOnlyByOperatorRole` 对受控关闭与重开返回真，客户账户请求方答 `ErrCustomerAccountCannotDecide`。
- 例外支的裁定只落在票里：[pc-gaps/13](../../.scratch/party-commercial-context-gaps/issues/13-authorized-action-lacks-controlled-closure-and-reopening.md) 裁决 ③ 对「货主经合同或明确授权直接形成决定」那一支写「首发不开、留位」。它比 party-commercial CONTEXT 的「合同委派不参与」弱：一个是首发暂不开放，一个是规则上不存在。
- 渠道服务方或代理商的叫停有两种归法：
  - parcel-shipment CONTEXT 与场景二把它当作有请求方、先留痕的请求。
  - [ADR-0183](./0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md) 复核记录：「承运方或渠道方叫停是外部限制（`EXTERNAL-RESTRICTION`），不需要请求方。」它的 Consequences 写「操作者渠道上不开「渠道服务方或代理商作请求方」的路」。
  - [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定一「请求证据落在决定上，不另立请求册」；其 Alternatives 把「未授权的请求、渠道方或代理商的请求要不要先留痕」留给 PS owner。
  - 两种归法下重开的路不一样：外部限制要先由来源责任方解除，请求则只需授权角色重开。

## Decision

**一、受控关闭与重开的正式决定只由运营侧授权角色形成，合同委派不参与。**（产品策略）两种决定都由运营企业责任法人授权的运营角色凭授权规则形成，与人工复核、主动拒绝同一形（party-commercial CONTEXT 受控关闭与重开一条）。货主、渠道服务方或代理商不能经客户合同或明确授权直接形成这两种决定；这一条不是「首发不开」，是规则上不存在。要给货主开直接形成关闭或重开的路，须另立记录改 party-commercial 的授权规则，不是填一格租户取值。

**二、货主的要求是请求证据，落在决定上。**（产品策略）货主（委托上的客户账户）要求停止或恢复尝试时，运营角色据其形成决定；货主指令关闭落那份指令的引用，重开落货主新的有效授权的引用（ADR-0187 决定一、五）。要求本身不改变包裹级继续尝试判断。代理商下单的，代理商就是委托上的客户账户，按货主算（ADR-0183 复核记录）。

**三、渠道服务方或代理商叫停是外部限制，不作请求方。**（产品策略）渠道服务方或代理商（不是这份委托的客户账户时）要求停止尝试，运营角色形成关闭时，关闭责任来源记为 `EXTERNAL-RESTRICTION`，请求方缺席，责任主体是限制来源的业务引用（ADR-0183 决定三）。它们的「恢复」不是重开请求，而是来源责任方对该限制的有效解除；解除只恢复可以申请并授权重开的资格，重开仍由授权角色形成。只覆盖单一渠道的限制照旧不阻止包裹级重开、只禁止该渠道的新使用。

**四、不另立请求册。**（产品策略）未获授权的请求与渠道方、代理商的要求不先单独留痕：前者没有形成决定就没有可落的证据，后者落在关闭责任来源上。这回答 ADR-0187 Alternatives 候选丁留给 PS owner 的那一问。

## 候选与反方

- **例外长期存在、首发不开、留位。** 否决：留位等于在 CONTEXT 里写一条今天谁也不执行的规则，读者照它建模会在授权规则里给客户账户留一格「可直接决定」，那正是代码 `ErrCustomerAccountCannotDecide` 守住的门；真要开，是一次商业授权模型的改动，配得上一份新记录。
- **渠道方叫停也作请求、另立请求册。** 否决：渠道方叫停的效力来自它对自己渠道的控制，不来自对运营企业提要求；记成请求，重开时就只需授权角色一按，绕过了硬限制须先由来源责任方解除这一步（parcel-shipment CONTEXT 受控关闭与重开那几条）。
- **渠道方叫停一律不形成包裹级关闭，只禁该渠道。** 否决：渠道方要求的常常是整件包裹的停（例如它报告收件人地址存疑），只禁一个渠道挡不住换渠道重试；是否形成包裹级关闭由运营角色判，形成时来源记为外部限制。

## 对已接受记录的作用

- 无条款停用。ADR-0183 复核记录「承运方或渠道方叫停是外部限制」与 Consequences「操作者渠道上不开「渠道服务方或代理商作请求方」的路。它们要提出关闭请求，另有来处时另行裁定；今天的载荷形状里没有指名它们的格。」，ADR-0187 决定一「不另立请求册」，本记录都照原句采用。
- ADR-0183 越权风险点 1（PS owner 若认为操作者应能指名委托之外的请求方）与上引 Consequences 后半句留的「另行裁定」由本记录决定三回答：渠道服务方或代理商不作请求方，叫停记为外部限制；无条款停用。
- ADR-0187 Alternatives 候选丁留给 PS owner 的一问由本记录决定四回答；无条款停用。

## Consequences

- parcel-shipment CONTEXT 受控关闭与重开那一条去掉「除非客户合同或明确授权……」的例外，补渠道方叫停记为外部限制、其恢复是来源解除。
- SCENARIOS 场景二过程中停止与恢复请求、重开请求两步与预期结果「货主或渠道请求不会在缺少适用合同委派时……」一句随之改写。
- ADR-0183、ADR-0187 的 Status 行注记与 Links 各补一条由 ADR-0275 回答的来向。
- `docs/product/PRODUCT-BASELINE.md` 里有与 parcel-shipment CONTEXT 同句的例外（「除非客户合同或明确授权把其纳入可直接形成决定的授权角色范围」），归票 docs-consistency/10 的 10b 半边照决定一、三改。
- 行为不改：`formedOnlyByOperatorRole` 与操作者渠道的载荷形状已经是本记录的形。三处注释与 Covers 仍引已删的例外句，说它首发不开、日后改这道拒绝即可开，随 [code/56](../../.scratch/docs-consistency/code/56-closure-and-reopening-comments-still-cite-the-deleted-delegation-exception.md) 改为指向本记录。
- 决定三「解除只恢复可以申请并授权重开的资格」今天不落门：parcel-shipment 的 `ClosureResponsibilitySourceKind` 头注写「硬限制那一格的「解除」今天没有来源可读」，重开编排不为它加门，渠道方叫停形成的关闭今天授权角色可以直接重开。缺口记进 code/56。

## 越权风险点

1. **代 PS、PC owner 裁定例外不存在**（PS、PC owner）。若 owner 要给大客户开经合同委派直接关闭或重开的自助路，推翻的是决定一，要另起记录改授权规则与 `formedOnlyByOperatorRole`，并让 parcel-shipment 的关闭编排接受客户账户作实际决定方。
2. **渠道方叫停由运营角色转成关闭**（PS owner）。渠道方的要求今天没有自己的入口，靠运营角色读到之后形成关闭；读漏了就没有关闭。渠道方要有直连的叫停口，是另一份记录。
3. **渠道方的恢复靠来源解除**（PS owner）。渠道方说「可以继续了」，运营角色先要把它记为来源责任方的有效解除，再由授权角色重开；两步都在运营一侧，渠道方自己按不出重开。
4. **未授权请求不留痕**（PS owner）。被拒的停止或恢复要求在本产品里没有记录，只在形成它的运营渠道（客服工单、邮件）里有；要在产品里留痕，是另立请求册，本记录不立。

## Links

- [ADR-0183](./0183-operator-channel-addresses-controlled-closure-and-reopening-by-parcel-and-derives-the-shipper-from-the-accepted-request.md)：决定三的关闭责任来源种类与复核记录「承运方或渠道方叫停是外部限制」，本记录照用；越权风险点 1 与 Consequences「另行裁定」那一问由本记录决定三回答
- [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md)：决定一请求证据落在决定上；Alternatives 候选丁那一问由本记录决定四回答
- [parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md)「面单继续尝试」相关规则、[party-commercial CONTEXT](../domain/party-commercial/CONTEXT.md) 受控关闭与重开一条、[SCENARIOS](../domain/SCENARIOS.md) 场景二
- 票 `.scratch/docs-consistency/issues/10-cross-cutting-docs.md`；代码票 `.scratch/docs-consistency/code/56-closure-and-reopening-comments-still-cite-the-deleted-delegation-exception.md`
