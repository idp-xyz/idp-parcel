# 单依据形态的提交前重解已被闭包形态取代：`ValidateBeforeDecision` 是留下的第一版

Category: chore
Status: draft——只读取证（MCP-6，锚 `2efef58e`），PC 地盘归 MCP-3；交 MCP-1 派
Blocked by: 无（纯删，PC 地盘内）

## 条目

`internal/partycommercial/domain ValidateBeforeDecision`（`commercial_resolution.go`）。基线理由行：「返回同型 Resolution 是变换不是构造。留着不排除是有意的」；triage spec 另注「开发主线 PN-02 行把提交前重解写成已落地」并视之为张力——**那不是张力，落地的是闭包形态**，见下。

## 它是什么

`ValidateBeforeDecision(registry, prior Resolution, standingOf) Resolution`：UC-PC-002 步 8 的提交前重解，**单依据**形态——按原查询重跑第一阶段（`ResolveCommercialBasis`），权威读不到保持`解析未决`，结果变了答`已失效`。

## 今天的生产路径

- `application/validate_commercial_basis.go` 调 `domain.ValidateClosureBeforeDecision`（`reference_closure.go`）——**闭包**形态，注释自称「与单依据侧的 ValidateBeforeDecision 是同一条规则的闭包形态」。
- 闭包形态**不调**单依据形态：两者各自内部调 `ResolveCommercialBasis` / `ResolveCommercialClosure`。所以单依据**解析**（`ResolveCommercialBasis`）活着——闭包逐项解析时调它；单依据**重解**（`ValidateBeforeDecision`）没有任何调用方，连测试外的同包引用也没有。
- `application/resolve_commercial_basis.go` 只走闭包；PS 消费端口 `ports.CommercialBasisResolver.ResolveCommercialBasis` 名字虽同，适配器（`parcelshipment/adapters/partycommercial/commercial_basis.go`）走的是 PC 的闭包用例。

## 三分

**死码**（被取代的第一版，与 PP 那对方案层快照门同一类）。基线那句「变换不是构造、留着不排除」讲的是它该不该在网内，与它还活不活是两个正交的问题——族界成立不妨碍它是死码。

## 建议的处置（PC 地盘做）

1. 删 `ValidateBeforeDecision` 及只为它写的测试；
2. `ValidateClosureBeforeDecision` 注释里「与单依据侧的 ValidateBeforeDecision 是同一条规则的闭包形态」改成历史注（曾有单依据形态，已删，理由：闭包形态是唯一生产路径）——否则那句在删掉之后指向一个不存在的名字；
3. 剪基线行，按头注纪律记数（三分成因写第一种）；
4. 顺带看一眼单依据 `Resolution` 族里还有没有同样只剩闭包内部在用的导出函数——`ResolveCommercialBasis` 有闭包这个同包调用方所以不在名单上，但若它日后也被闭包内联，会是同一格。

## 能不能归到已认可的留待

不适用：它不缺调用方，它被替代了。

## 完成判据（落地那笔连基线一起改；MCP-1 2026-09-07 裁）

1. `ValidateBeforeDecision` 及只为它写的测试删去；`ValidateClosureBeforeDecision` 的注释改成历史注（曾有单依据形态，已删；闭包形态是唯一生产路径），不再指向一个不存在的名字。
2. 剪基线行，头注记一句成因第一种（已删，被闭包形态取代）；在自己那笔的干净检出上两法同得记数、钉 SHA。
3. PC 组那句「下面两条落在网里但不是工厂：ManualReviewRequirementFor 返回 bool 是个谓词，ValidateBeforeDecision 返回同型 Resolution 是变换不是构造」改成只指 `ManualReviewRequirementFor` 一条（票 04 落地时再由它改写成理由行）——否则它在删掉之后指着一个不在名单上的名字，而 `TestWiringBaselineHasNoStaleEntry` 不读注释，不会为此红。
4. 顺带核一遍单依据 `Resolution` 族里有没有同样只剩闭包内部在用的导出函数，有就在本票 Comments 记名，不在本票删。

## 边界

本票不改代码、不改基线。
