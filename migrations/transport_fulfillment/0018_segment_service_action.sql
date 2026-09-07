-- 段服务动作（ADR-0114 决定一；票 tf-segment-lifecycle-closure/09）。
--
-- CONTEXT「段服务动作」：控制事实登记方在实际履约段成立时对该段显式声明的运输动作，封闭为场外揽收、
-- 节点间运输、末端派送三种；由登记方声明，不从计划履约段位置、实际承运商或交接范围推导；未声明是一种
-- 答案而不是缺陷。声明为末端派送的段即派送段——末端派送任务的内部触发只认「对象凭已交接进入派送段」。
--
-- **只在段首登那一行写入，之后不改。** 列可空：NULL 就是「未声明」，不给默认值——给了默认就等于替登记方
-- 推了一次它没说过的话。后续对象加入时给出矛盾的声明由领域拒（不改这一列）；声明错了是另立新段的事，
-- 段登记册没有通用 Update（ADR-0097），这里也不为它开门。
ALTER TABLE transport_fulfillment.actual_fulfillment_segment
    ADD COLUMN service_action text;

ALTER TABLE transport_fulfillment.actual_fulfillment_segment
    ADD CONSTRAINT actual_fulfillment_segment_service_action_closed
        CHECK (service_action IS NULL OR service_action IN ('OFFSITE_PICKUP', 'LINEHAUL', 'FINAL_DELIVERY'));
