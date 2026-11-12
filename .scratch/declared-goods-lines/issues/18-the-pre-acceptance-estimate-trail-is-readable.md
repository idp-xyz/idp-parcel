# 18 受理前估价的留痕可读：用了哪张卡、按哪个服务产品

Category: enhancement
Status: in-progress
Blocked by: 19、20、21（「要定的」两项已由 [ADR-0379](../../../docs/adr/0379-the-pre-acceptance-estimate-trail-records-product-weight-and-charge-lines-and-is-read-on-the-shipment-detail.md) 定下，实现拆成这三张）

依据：ADR-0179 决定六（估价连同价卡版本、逐件评价标识与版本清单随控制结果留痕）；[ADR-0378](../../../docs/adr/0378-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md) 决定三（选卡按申报的服务产品）；票 17 评审。

## 现状

- 结算把受理前估价的留痕写进库（价卡版本、逐件评价、控制金额），没有任何 HTTP 读口交出它；计价一侧不存试算。
- 演示段 `55` 因此断言不了 FBA 货件的估价用的是 FBA 卡，只能靠一次只读的库查询确认（票 17 完成记录）。
- 运营在管理台看一票被接受前财务控制挡下或放行时，也看不到估价按哪张卡、哪个产品算的。

## 要定的

- 读口放在哪一侧：结算的控制结果查阅（留痕的所有方），还是委托详情经结算的读口带出来。
- 交出哪几格：控制金额、价卡（方案与版本）、选卡用的服务产品、逐件评价标识。

## 不做的

改估价的算法；按票计价（ADR-0378 越权风险点 8）。

## 决定（2026-11-11）

按头部系统的做法定，写成 [ADR-0379](../../../docs/adr/0379-the-pre-acceptance-estimate-trail-records-product-weight-and-charge-lines-and-is-read-on-the-shipment-detail.md)：预估运费在订单详情上显示，带所用的模板与重量；下单时按估算冻结；预估与实际分开。

- 留痕补记选卡用的服务产品、逐件计价重量与费用行，只作留痕。
- 结算按委托交出运营读口：逐提交版本的估价与它的冻结或信用暴露。
- 受理判断的财务控制结论由小包托运在委托详情上交出，不带金额。
- 管理台运单详情显示「受理前财务控制」一块，两侧各交各的、页面拼。只对运营，不对客户。
