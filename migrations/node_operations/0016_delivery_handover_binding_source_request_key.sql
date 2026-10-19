-- 送站交接单绑定结论多一格来源请求键（UC-NO-006 结果语义契约「绑定已否认：否认时刻与来源请求键」；票 ps-external-mark-relations/12）：
-- 客户账户经首方客户渠道作结论时，首方族的推导口从接入请求标识铸出的那个键随结论落册。运营决定的确认不带它：那一路没有客户面的
-- 请求，作结论方是信封上的操作者。
--
-- 结论的重放不按它认，按主键（租户，交接单，客户账户）认（ADR-0212 决定三「同内容重投答已有结果，异内容答已有结论」），所以它不进
-- 任何唯一约束：同一个客户账户换一个接入请求再交同一结论，答的仍是在册那一份。
--
-- 库面照途径守：客户渠道必带、运营决定必不带。本迁移之前这张表没有任何写口（交接单的命令口随本票才开），既有行只可能来自直写册的
-- 夹具，不回填。

ALTER TABLE node_operations.delivery_handover_binding
    ADD COLUMN source_request_key text;

ALTER TABLE node_operations.delivery_handover_binding
    ADD CONSTRAINT delivery_handover_binding_source_request_key_with_customer_channel
        CHECK ((route = 'CUSTOMER_CHANNEL') = (source_request_key IS NOT NULL AND btrim(source_request_key) <> ''));
