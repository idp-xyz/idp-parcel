# ADR-0047: 账期方式的接受前控制形成信用暴露，不冒充资金冻结

Status: Accepted  
Date: 2026-08-12

## Context

SA 的接受前财务控制今天只有预付一条路：`PreAcceptanceControlPolicy` 二值（要求/不要求），
要求即冻结。而权威语言在三处说了另一半：

- SA CONTEXT：「本上下文形成的是估价、**冻结、信用暴露**或业务限制」；「每项费用、冻结、
  **信用暴露**和核销必须保存**实际采用的结算政策、预付/账期方式**及其适用范围」；「余额
  不足或**逾期**只向订单接受等责任上下文提供信用暴露和业务限制依据」。
- 验收矩阵 SET-02/SET-03：每一金额与控制范围唯一解析预付或账期方式并保存政策依据；预付
  冻结**不与同一客户账期范围共用余额、额度**。
- PS `AT-PS-035`：「合同同时要求**信用校验**和预付冻结」——信用校验是与冻结并列的控制种类。

ADR-0044 已让 PC 的结算政策解析交回方式（PREPAID/TERMS）与六维范围，SA 消费侧从此可观察
方式——控制策略答复却还没有承载它的字段。PS 的 `FinancialControlOutcome` 只有
HELD/RESTRICTED/NOT_APPLICABLE：账期控制通过若译成 `HELD`，就是在说一笔并未冻结的资金
被冻结了，释放语义（释放冻结 vs 释放暴露）也随之混淆。

## Decision

**一、SA 控制策略答复携带方式与采用政策。** `PreAcceptanceControlPolicy` 由二值升为：
`不要求`必带商业不适用依据（不变）；`要求`必带 `SettlementMethod`（PREPAID/TERMS，SA 自有
封闭二值）与 `AdoptedPolicyReference`（实际采用的结算政策引用，CONTEXT 硬句要求控制结果
保存它）。两头都带或都缺的形状构造期即死。

**二、账期分支形成信用暴露，走自己的账本。** 新领域对象 `CreditExposure` 与
`CreditExposureLedger`，代数与冻结账本一致（幂等重放、同身份异内容冲突、只增不删、显式
释放），但取数对象不同：预付读运营余额，账期读 `CreditStanding`（当前额度、已占用暴露、
是否逾期）。超出可用额度或账户逾期形成`业务限制`（各带原因），不是错误。两本账互不借用
（SET-03 的不共用硬句）。

**三、PS 侧词汇加第四格 `CREDIT_EXPOSED`。** `FinancialControlOutcome` 增加
`FinancialControlCreditExposed`，校验翻译与 `HELD` 同为`通过`。不冒用 `HELD`：冻结说资金
已占用、暴露说额度已占用，两者的释放对象不同，压成一格会让释放编排拿着暴露去找冻结账本。

**四、释放按原关联双账本认领。** `ReleasePreAcceptanceControlHandler` 先查冻结账本、
再查暴露账本；都查不到仍是`无可释放`。释放暴露与释放冻结同样幂等。

## Consequences

- SA 施加编排按方式分支；结果携带方式与采用政策引用，下游可按 CONTEXT 要求保存。
- PS→SA 适配器把暴露结果译成 `CREDIT_EXPOSED`，策略夹具随新构造签名更新。
- 额度、逾期与账户映射的取值仍属实例半边（`PAR-SET-*`、`PAR-COM-*` 待登记）；机制以合成
  事实钉规则，不设任何默认额度。
- 同一委托「信用校验+预付冻结」并行（`AT-PS-035` 的同时要求）作用在**不同控制范围**上，
  由范围解析各自成路；同一范围命中两种方式在 PC 解析处已是`适用冲突`（ADR-0044）。

## Alternatives considered

- **账期通过译成 HELD（不动 PS 词汇）。** 否决：说谎的引用——没有资金被冻结；释放编排
  无从分辨该去哪本账认领。
- **暴露记进冻结账本（一本账两种记录）。** 否决：CONTEXT 明写冻结与信用暴露是不同对象，
  SET-03 明写不共用余额与额度；一本账会让「预付余额」与「账期额度」在实现里合流。
- **控制策略保持二值，方式由适配器猜。** 否决：ADR-0025 适配器只翻译不判断；方式是政策
  解析的输出（ADR-0044），不带回来就只能猜。

## Links

- [ADR-0044](./0044-settlement-basis-adopts-via-settlement-policy.md)：方式与范围从 PC 可观察的前提
- [SA CONTEXT](../domain/settlement-accounting/CONTEXT.md)：冻结/信用暴露分立与控制结果保存政策的硬句
- [PILOT-ACCEPTANCE-MATRIX](../product/PILOT-ACCEPTANCE-MATRIX.md)：SET-02/SET-03
- [UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md)：`AT-PS-035`
