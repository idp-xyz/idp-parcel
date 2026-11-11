# 09 接单规则包的必填与跨字段条件执行器（`BD-PS-006`）

Category: enhancement
Status: in-progress
Blocked by: 无（「要先定的」三项已由 [ADR-0376](../../../docs/adr/0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 定下；演示那一项另立 12，依赖 10、11）

依据：ADR-0374 决定三（FBA 的平台货件号、箱标号、装箱分配报全是服务产品所采用接单规则包的必填与跨字段条件，产品不内置，没配置不判）；UC-PS-001 `BD-PS-006` 一行与业务决策简报同名一节；ADR-0146 决定三、ADR-0147 决定四、决定五（参考配置显式采用才生效，构造门或样例不过的文件不能发布）。起因：票 03 欠下一份 FBA 头程提交条件参考配置；票 08 写过一份，评审指出它没有过构造门、正文又替本票还没定的事下了结论，已撤下，改由本票随执行器一起发布。

## 现状

- 接单规则包（party-commercial）只存（规则类别，规则引用）对，`AssembledRule` 结构上没有放取值的地方；`CROSS_FIELD_CONDITION` 类别今天只在规范化与落库里读，小包托运没有任何代码读规则包里的规则引用。
- 接受判断的判组里，客户关系、法人与合同、产品与服务三组，结算前资金控制一组，网络可达一组各有生产者；成员基线与必要资料两组（`MEMBER_BASELINE`、`REQUIRED_DOCUMENT`）没有生产者，规则包若声明它们适用，委托停在内部重试。
- 等待受控补充（`ResumeByCustomerSupplement`）今天只由网络可达一组的证据不足与授权处置的「客户补充」两处设起。

## 要做的

依 [ADR-0376](../../../docs/adr/0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 决定一。

- **参考配置**：写并发布 `REFCFG-1:parcel-shipment/submission-conditions/FBA-HEADHAUL@1`，与执行器、构造门测试同一笔（ADR-0147 决定五）。
  - 条件只取 ADR-0374 决定三列的几项：平台货件在场、每件有平台箱标号、装箱分配在场（报了就是报全，由提交口的形状规则保证）。
  - 「内件性质在场」不在决定三里，不列。
  - 条件指委托当前提交版本与委托上的平台声明，不指首方 HTTP 载荷的路径。
  - 正文只写判断方法，不写会过时的话。
- **构造门**：每一版已发布的提交条件参考配置都要能被执行器读成条件集，认不得的条件码答错误；测试逐版过一遍（同网络目录的做法）。
- **读规则包**：小包托运经消费侧适配器读采用的接单规则包里 `CROSS_FIELD_CONDITION` 类别的规则引用（party-commercial 的 `AcceptanceRulePackageContentView`），在受理判断的装配点接上。
- **判**：
  - 规则包声明「必要资料」组适用时，在这一组逐条判。
  - 委托级条件出一条版本级检查，逐件条件按缺的包裹各出一条。
  - 未满足判`未定`，续办方是客户（`ResumeByCustomerSupplement`）。
  - 声明了适用而没有可执行引用，这一组判通过；不带 `REFCFG-1:` 前缀的不透明引用照旧不判。
  - 引用指的那一版没发布或打不开，是依赖读不回，判`未定`、内部重试，不当成满足。
- **说清缺什么**：受理任务记下未满足的条件码与包裹，委托查阅面交出，管理台详情显示。
- **登记时核**：规则包登记时，带 `REFCFG-1:` 前缀的规则引用要打开得到已发布的那一版，打不开答登记不成立（同 ADR-0157 决定三）。

## 完成判据

- 规则包声明「必要资料」组适用并引用 FBA 头程参考配置时，缺平台货件、缺箱标号、没报装箱的委托停在等待受控补充，任务与查阅面说清缺哪一条、哪几箱；补齐后重判（经受控补充补平台号归票 10，演示归票 12）；没采用的服务产品照旧不判。
- 参考配置的条件码在执行器里逐条有测试；条件码未知或参考配置未发布时答未决（依赖读不回），不当成满足。

## 不做的

客户字段映射到条件的实例半边（租户自己写的条件）；批量导入模板的必填提示。
