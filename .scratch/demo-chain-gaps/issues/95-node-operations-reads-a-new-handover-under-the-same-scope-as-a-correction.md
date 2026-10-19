# 95 同一（对象，范围）下另起一次交接：TF 当独立首登，NO 当前一次的更正，在身控制在错的那次交接转出

Category: bug
Status: needs-triage——2026-10-19 Claude Code 子代理立（演示动线重走，[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「重走（钉 `8197fa2f`，2026-10-19）」停点一）。
Blocked by: 无
地盘：二选一，见「要裁的」——TF `application/register_transport_handover.go` 首登那一步；或 NO `adapters/postgres/physical_control_transition.go` 的 `FindAdvancedInHandoverChain` 与 `application/transfer_control_on_handover.go`。文档 ADR-0214 补记、UC-NO-005。
出处：[ADR-0214](../../../docs/adr/0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md) 越权风险点「同链按（对象，范围）认」；[UC-NO-005](../../../docs/application/node-operations/UC-NO-005-ADVANCE-NODE-CONTROL-ON-AN-AUTHORITATIVE-TRANSPORT-HANDOVER.md)。

## 现象（实测，钉 `8197fa2f`，只记 `S`）

演示包裹 `DPCL-FRVC5MAXIPKBQHYKMXQGOATJUE`（一次性库，走完已删）在深圳口岸先后登两次`已交接`，`scope` 都写 `SZX`：

- `H2/v1`：自营干线交进深圳口岸，只声明 `receivingNode`。TF 答 `201 HANDOVER_REGISTERED`；NO 在深圳口岸立 `HANDOVER_IN` 控制（`IN`·`ADVANCED`）。
- `H3/v1`：深圳口岸交给干线承运商，声明 `releasingNode` 为 `SYN-NODE-SZX-GATE`、`onwardNode` 为 `SYN-NODE-SIN-HUB`。TF 照样答 `201 HANDOVER_REGISTERED`，是首登不是更正。NO 却把它读成 `H2/v1` 的同链后版：转出那一半剔掉同链转入立起的那一份，落 `OUT`·`NOT_ADVANCED`·`NO_ACTIVE_CONTROL`。
- 再下一次交接 `H4/v1`（承运商交进新加坡枢纽，`scope` 为 `SIN`，不声明交出节点）把深圳口岸那一份转出：`OUT`·`ADVANCED`，节点 `SYN-NODE-SZX-GATE`。控制在错的那次交接转出；那次交接没声明交出节点，`mismatchedReleasingNode` 也落不下，册上看不出错。

对照：第二件包裹 `DPCL-7VJZ4RRIS66IV6FQW4RYKPANDI` 走同一段，三次交接各用自己的范围（`W19B-1`、`W19B-2`、`W19B-3`），深圳口岸交给干线承运商那一次落 `OUT`·`ADVANCED`·`SYN-NODE-SZX-GATE`。

## 成因（读码）

- TF 交接的身份是（租户，对象，范围，版本）。更正经更正口登记、回指前身，`TransportHandovers.FindSuccessor` 按这条回指认后版；首登口不问同（对象，范围）下是否已有别的版本，换一个版本号就是另一次首登。
- NO 认同链按（对象，范围）：`PhysicalControlTransitions.FindAdvancedInHandoverChain` 只按对象与范围取，不看 TF 的更正回指；`TransferControlOnHandoverHandler.transferOut` 据它剔掉「这条链转入立起的控制」（`withoutControlOf`）。
- 两边对「哪几版是一条链」各认各的。ADR-0214 越权风险点「同链按（对象，范围）认」预先写明了这一格，此前没有承接票。

## 三类

- 机制：TF 与 NO 对交接链只能有一种认法。两条路：
  - TF 首登口遇到同（对象，范围）已有版本时答一个指名的结果（例如要求走更正口），不落册、不发信封；
  - NO 认链改读 TF 的更正回指，不再按（对象，范围）认。
- 产品策略：无。
- 租户取值：无。

## 完成判据

- 同（对象，范围）下另起一次首登：要么 TF 拒（指名结果，不落册，不发信封），要么 NO 把它当独立交接，前一次立起的控制被它正常转出。
- 更正链的既有各格照旧：后继版仍`已交接`时零写入、撤回控制的更正入账「控制来源链待重算」、一次交接不转出它自己立起的控制。
- 真库用例；证据记 `S`。

## 要裁的

- 走哪一条：「范围」在 TF 语言里是不是交接链的身份。是，就由首登口拒；不是，就改 NO 的认法，ADR-0214 决定五与那条越权风险点随之补记。
- 已按（对象，范围）认错的存量转移行要不要入账「控制来源链待重算」，归 [81](./81-control-source-chain-recompute-has-no-disposition-face.md) 的处置面。
- 演示动线文档要补一句「交接的 `scope` 每次交接各起一个，`version` 只给更正用」，不论本票走哪条路都要补；句子列在 psb/05 那一节的停点建议里。

## 规模

S–M
