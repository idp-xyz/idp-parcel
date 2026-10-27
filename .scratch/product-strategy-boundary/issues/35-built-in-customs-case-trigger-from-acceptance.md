# 35 customs-compliance：关务案件的内置触发——按接受决定与已采用路由派生立案

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R2）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-20 Claude Code 会话随 [18](./18-customs-case-and-declaration-submission-entry.md) 分诊立（用户「你作为业务和系统专家自决……解决所有的问题」授权）；分诊时先定「要裁的」再转 ready-for-agent
Blocked by: 无；要读的已采用路由在演示上才刚通（[routing-first-cut/11](../../routing-first-cut/issues/11-demo-network-adopted-as-reference-configuration.md) 一带），实走要它
地盘：`internal/customscompliance`（派生判断方法与它的应用编排）、`cmd/parcel-dispatch` 的一路派发路由；CC `CONTEXT.md` 经 CC owner。
出处：[18](./18-customs-case-and-declaration-submission-entry.md) 分诊裁定「立案」一问；[ADR-0227](../../../docs/adr/0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md) 决定七与越权风险点 8；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二。

## 现状

18 给了建案的显式运营命令面：运营人员指名已接受包裹与固定监管范围四维（辖区、方向、程序、法定义务），服务端按包裹查出客户归属与来源资料引用，建案要求规则册判要不要建。生产进程里没有任何东西会在委托被接受时自己建案——接受决定的扇出只到 VE 与初始路由。

## 做什么

- **产品策略**：一套派生方法——一份接受决定与它的已采用路由，怎样推出要建的案件：哪一段跨关务边界、那一侧的辖区与方向、适用哪个监管程序与法定义务范围。产品给方法与形状，租户的取值（程序代码、义务范围、哪些服务产品要关务）进登记册。
- **机制**：派发路由消费接受决定（或已采用路由形成），按方法派生建案请求，经 18 那一只建案编排与建案要求规则册落地；派生不出的格如实答未决或不适用，不建占位案件。

## 要裁的

1. 触发点是接受决定、初始路由形成，还是两者都到齐时：接受决定不带路由，初始路由可能晚到或停在未决。
2. 派生方法读什么：已采用路由的关务段、CC 的口岸与申报路径目录、还是另立一册「服务产品 × 方向 → 程序 / 义务」；与 [routing-first-cut/12](../../routing-first-cut/issues/12-cc-customs-applicability-judgment-for-route-candidates.md) 的分工按 18 分诊裁定——rfc/12 的判断口是路由判断的输入，不被建案当依据复用。
3. 内置立案与命令面并存时的身份：两路同键（租户加四维）即同一案件，命令面先到时内置一路答已有案件；同键异包裹集照建案编排答范围冲突。

## 不做

- 不碰申报就绪、授权与提交：立案不等于申报，首发生产提交只经 18 的两道运营决定。

## 完成判据

- 演示租户上一票已接受且路由已采用的委托，不经人工建案也有了它出口一侧的关务案件（证据只记 `S`）；派生不出的格有指名的未决或不适用答复。
