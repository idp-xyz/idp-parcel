# 171 两种门禁问询不按问的那台设备的活限对象，运输履约这一侧也不留痕

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)，分级 R3）：不挡上线，代理不认领；解冻看首个租户的真实使用或用户点名。此前 needs-triage——2026-10-21 Claude Code 作者子代理立（用户「你作为业务和系统专家自决……解决所有的问题」授权），出自票 [146](./146-departure-gate-answers-to-devices-list-only-blocked-or-undecided-objects.md)、[94](./94-a-courier-previews-the-final-delivery-gate-before-dispatching.md) 的隔离评审。分级（上线前必修还是上线后尽快）待分诊。
Blocked by: 无（要先定段与派送任务的作业范围归属怎么立，见「三类」，那一步要一份新 ADR）
出处：[ADR-0219](../../../docs/adr/0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 越权风险点 6；[ADR-0237](../../../docs/adr/0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 越权风险点 2；[ADR-0238](../../../docs/adr/0238-a-courier-previews-the-delivery-gate-before-dispatching-and-reads-what-the-delivery-will-record.md) 越权风险点 6、7。

## 现象

票 146、94 之后两种问询都只读、不落册，回给设备的逐对象那一格只列没得出放行的对象（ADR-0237），出发意图的起讫也与段绑定了（ADR-0237 决定五）。剩下的暴露面在「谁能问哪一段、哪一件」：

- **出发前问询**（`/transport-fulfillment/departure-gate-previews`）：持任一 OFFSITE 作业范围授予的设备，可以按任一段自己的班次问本租户任一段、任一时点，拿到那一刻段里没得出放行的那几件，含别的客户被扣的包裹。一段确是跨关务区域的自营段时，段里不在案卷的每件本就答未决、全都在列；票 147 落地之后，起讫有一端是未登区域的节点时同样全列。
- **交付前问询**（`/transport-fulfillment/delivery-gate-previews`）：同样的设备可以对本租户任一件包裹问，拿到它此刻是否被扣、at 是哪个节点、或停在哪一种本侧停点。
- 两种问询运输履约这一侧都不记谁问过；关务判断册记了对象与时点，不记是哪台设备。事后答不出「这台设备问过哪些段、哪些件」。

今天段与派送任务上都没有执行方或作业范围的归属：场外设备的授予是 `OFFSITE` 作业范围，与段、派送任务之间没有可核的关系，所以「只问得到自己随车运着的段、自己派送任务上的那几件」现在收不了。

## 三类

- 机制：问询按问的那台设备的活限对象——出发前问询只答它被指派的段，交付前问询只答它派送任务上的那几件；不在它名下的答一格不泄露存在性的未受理。两种问询在运输履约这一侧留痕：设备、作业范围、问了什么、何时，读数本身不存（ADR-0219 决定三「不落记录」要随之部分停用，另立 ADR）。
- 产品策略：段、派送任务的指派从哪来（交接登记时声明接收设备或作业范围、派送任务形成时指派执行方，还是另开一种运营决定），留痕留多久、谁能查。
- 租户取值：各租户的作业范围与设备登记（已有）。

## 完成判据

- 出发前问询对不在问询设备名下的段、交付前问询对不在它派送任务上的对象，都答未受理，不问关务，答复与「段不在册」「对象不在任何段里」分不开。
- 两种问询在运输履约这一侧各留一行痕，可经运营查阅读到；出发登记与有效交付登记不受影响。
- 真库用例：两台作业范围不同的设备，各自只问得到自己的那几件；留痕可读。

## 规模

L（要一份 ADR 定指派的来源）
