# 分区主体没有登记处，跨上下文撞不撞只能靠人重读代码

Category: enhancement
Status: resolved

来源：本目录[票 01](./01-tf-object-partitions-collide-with-ve-parcel-partitions.md) 第三问取证的
副产物，基线 `9e5c5c0`。**排在**[棘轮门禁](../../production-wiring-ratchet-gate/issues/01-production-ports-wired-only-in-tests-have-no-ratchet.md)**之后做**。

## 为什么不是票 01 里那条扫描

票 01 原本设想「同一 SHA 下全仓分区键表达求值后取交集」。**已估完，做不出来**，两层都堵死
（论证原文见票 01 的「要答的」第三问）：分区键是运行期字符串拼接，静态无值可求；退成类型层求交
也不成立——就算把 `parseRepositorySources` 整体换成 `go/packages` 全量类型检查，也答不出票 01
那个问题，因为那处碰撞正是两个**不同类型**承载**同一字符串**。

所以不建扫描，建登记处。

## 要建的

一张中心表，每个交接口在其中声明自己的分区主体：`租户/包裹`、`租户/载运对象`、`租户/客户`、
`租户/案件`……形状照抄 `internal/architecture/envelope_partition_gate_test.go` 的
`allowedSameExpression`：中心清单 + 可复核的判据前缀 + 只许变短 + 自测能红。

门禁只查两件：

1. **覆盖完整**——表是否覆盖全部交接口。新写的口进不来就红，与现有门禁「清单对新来者是关的」
   同一口径。
2. **同主体名跨上下文**——是否有两个不同上下文声明了同一个主体名。声明了就要求那一行显式带上
   裁定引用（本票或后续 ADR），不允许静默共用。

## 它守不住什么——这一节不许在实现时删

**它守不住「两个主体名其实是同一个字符串」。** 票 01 那处碰撞的两口若各自诚实声明为
`租户/载运对象` 与 `租户/包裹`，本门禁**放行**——两个主体名不同，而它们的值在今天的代码里是同一
个字符串（`cmd/parcel-dispatch` 的揽收采用链里，同一个原始串既构造
`tfdomain.NewCarriedObjectReference` 也构造 `psdomain.NewDeclaredParcelID`）。

**本门禁因此不解票 01。** 它做的是另一件事：把「载运对象与包裹在排队意义上是不是一个主体」这个
判断**从散落在十几个 `PartitionKey` 表达式里，挪到一张人能一眼扫完的表上**——而那正是票 01 第一
问要 owner 拍的东西。

若实现时把这一节删掉或改软，下一个人会把本门禁当成已经守住了票 01，那比没有这张表更坏。

## 与票 01 的先后：不阻于第一问

**本票不等票 01 的第一问裁完。** 反过来才对：表存在本身就是为了让人能拍第一问，互相等就死锁。
表的第一版按今天代码**如实登记**每口的主体，包括那些今天判不准的——判不准的行按现有口径写
`待裁：<答不出的那一句>`，不许填一个好看的值。

## 边界

- 不改任何 `PartitionKey` 表达式。本票只登记与门禁，一口都不改形状。
- 不改 `internal/architecture/envelope_partition_gate_test.go` 现有的 `allowedSameExpression`
  与它守的规则；本票是并列的第二条门禁，不是它的扩展。
- 不碰 `cmd/parcel-dispatch/assemble.go`。

## 参照

票 01 的「要答的」第三问与 Comments 第三节；
`internal/architecture/envelope_partition_gate_test.go`；ADR-0065、ADR-0069。

## Comments

- 2026-08-21 MCP-2：票 01 第一问已裁（用户授权代裁），主体名的权威落
  [ADR-0074](../../../docs/adr/0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md)
  决定五：TF 对象链三口登记为「租户/载运对象/口名」，VE 四口登记为「租户/包裹」。实现本票时
  这几行**不再是「待裁」**，直接引 ADR-0074；其余口照旧如实登记、判不准的写待裁。TF 两口的
  键表达已随裁决改带口名段，登记表以当时代码为准重扫，勿抄本票写下时的形状。

- 2026-08-24 MCP-6（**本票落地**，登记转录锚 `7ac2f30`）：门禁与首版登记表落
  `internal/architecture/partition_subject_registry_test.go`，首版四十六行按当时全部键表达式
  重扫如实转录（未抄本票写下时的形状）。边界照守：零 `PartitionKey` 表达式改动、
  `envelope_partition_gate_test.go` 未动（只读复用其 `envelopeType` 与 `envelopeField`）、
  `cmd/parcel-dispatch/assemble.go` 未碰。

  **登记行形状**：三前缀（`主体：`／`逐信封：`／`待裁：`），主体行的裁定引用挂在
  `｜裁：` 段。门禁两查照票面：覆盖双向机械核（新口不登记红、烂行红）；跨上下文同名的
  每一行必须带非空裁段——内容是裁定引用或**显式待裁句**，挡的是静默共用，不是挡未决。

  **如实登记浮出的最要紧一行，留给人拍**：PS 采用链三口（final_outcome / network_intake /
  parcel_cancellation）与 VE 四口同名同值「租户/包裹」——FanOut 之下两链信封落同一分区
  （票 01 Comments 二实测过），而 ADR-0074 只裁了 TF×VE，PS×VE 共队是否有意**无裁定记录**。
  三行按第二查口径带显式待裁裁段，问题原句在行内。

  其余登记决定：TF 三口与 VE 四口引 ADR-0074 决定五（不再待裁）；ID 与分区键同表达式的口
  一律登记为逐信封，并与 `allowedSameExpression` 只读互核（同表达式必为逐信封，用例机械查）；
  PG 治理口两形（暂停/接管）皆无租户段，如实转录并注明归 PG 地盘；CC 关务案件与 VE 异常案件
  按各自 CONTEXT 原词取名——不同主体不同名，不触发裁段，若有人认为它们是同一排队主体，
  表已让两行同屏可审。

  「它守不住什么」一节照票面要求原样进了文件头，未删未软化。突变实测三向都红（缺行、烂行、
  静默共名），合成自测另覆盖提取器与分组逻辑；`gofmt`/`go vet`/整包 `-count=1` 全绿。落地时
  树上含 MCP-1 的 CC 在途改动，其对 `case_closure_handoff.go` 的改动是 `CaseRef()` 返回值
  类型强化，分区键语义不变，登记行不受影响。票转 resolved。
