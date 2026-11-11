# 07 关务读口：按（资料版本锚，委托，行号）取货物明细行

Category: enhancement
Status: ready-for-agent
Blocked by: 03

依据：ADR-0373 决定九；CONTEXT-MAP「`parcel-shipment → customs-compliance`」；关务 CONTEXT Boundaries 里小包托运拥有什么那一句；UC-CC-002 输入「客户原始资料」一行。

## 要做的

- `parcel-shipment` 提供读口：按（租户，资料版本锚，委托）交出货物明细——内件性质、行（带来源标记）与装箱分配。
- `customs-compliance` 在消费侧适配器（`internal/customscompliance/adapters/parcelshipment/`，ADR-0025）里译成关务自己的输入，供正式申报资料形成时逐行溯源。

## 完成判据

- 适配器测试：单件、多件不报分配、多件报全分配三种都能取到；锚不存在与跨租户答不可见。
- 关务不把正式资料写回客户原始资料（既有不变量保持）。

## 不做的

并项、补算、折算与逐件申报的形成规则：属关务 UC-CC-002 的实现，另行排期（UC-CC-002「给开发的交接」已记）。届时一并写清：各申报口怎样从内件性质映射出口原因（FBA 头程是库存转移、不是销售给收件人，owner 复核建议 7）；中国出口申报要素（品牌、型号、材质、用途）要单列时照 ADR-0014 换形状（建议 18）。
