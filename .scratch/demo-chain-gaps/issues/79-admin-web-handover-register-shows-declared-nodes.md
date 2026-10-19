# 79 管理台交接册显示交出、接收、去向节点

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md) 的隔离评审（非阻断项，不归 59）。
Blocked by: [59](./59-handover-declares-releasing-receiving-and-onward-nodes.md)（三个键随它进 main）
地盘：`apps/admin-web/src/pages/operations/records-api.ts`、`TransportFulfillmentReviewPage.tsx` 与同目录的呈现与测试。diff 全在 `apps/admin-web/**`，按 [workflow.md「前端切片」](../../../docs/agents/workflow.md#前端切片一人在-main-上直接做) 在 `main` 上直接做。
出处：[ADR-0213](../../../docs/adr/0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 决定三「回显与查阅」。

## 现象

票 59 进 main 之后，`GET /transport-fulfillment-records?registry=transport-handover` 的每一行多三个可缺键：`releasingNode`、`receivingNode`、`onwardNode`。值是登记方声明的 NR 节点代码，原样字符串；没声明的那一格不出现。

管理台的 `TransportHandoverRecord` 不认这三个键，运输履约复核页的交接结果一区也不显示。多出的键对前端无害，页面照常渲染，只是看不到节点。

## 三类

机制（查阅页如实呈现在册内容）。

## 做什么

- `records-api.ts`：`TransportHandoverRecord` 加三个可缺的字符串键，注释照端点 `transportHandoverBody` 的说法：缺席是「没有声明」，不是读不出。
- 交接结果一区：每行显示三格节点，原样显示代码，可复制。没声明的格显示「未声明」，不留空白，也不拿交出方、接收方去补。节点不是参与方，不要拼进「A → B」那一格。

## 完成判据

- 三道门退 0。
- 在灌过演示种子、按演示动线登过带节点交接的库上，交接结果一区看得到声明的节点，没声明的格写「未声明」。

## 规模

XS
