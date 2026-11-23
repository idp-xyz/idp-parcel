-- 全程追踪增加仓配履约这一处已接受事实来源。里程碑映射仍由租户登记，没有登记保持未归类。

ALTER TABLE visibility_exception.accepted_fact
    DROP CONSTRAINT accepted_fact_source_closed;

ALTER TABLE visibility_exception.accepted_fact
    ADD CONSTRAINT accepted_fact_source_closed
        CHECK (source_context IN (
            'PARCEL_SHIPMENT', 'NETWORK_ROUTING', 'NODE_OPERATIONS',
            'TRANSPORT_FULFILLMENT', 'CUSTOMS_COMPLIANCE', 'WAREHOUSE_FULFILLMENT'
        ));

ALTER TABLE visibility_exception.milestone_mapping_entry
    DROP CONSTRAINT milestone_mapping_entry_source_set;

ALTER TABLE visibility_exception.milestone_mapping_entry
    ADD CONSTRAINT milestone_mapping_entry_source_set
        CHECK (source_context IN (
            'PARCEL_SHIPMENT', 'NETWORK_ROUTING', 'NODE_OPERATIONS',
            'TRANSPORT_FULFILLMENT', 'CUSTOMS_COMPLIANCE', 'WAREHOUSE_FULFILLMENT'
        ));

ALTER TABLE visibility_exception.disposition_request
    DROP CONSTRAINT disposition_request_target_closed;

ALTER TABLE visibility_exception.disposition_request
    ADD CONSTRAINT disposition_request_target_closed
        CHECK (target_context IN (
            'PARCEL_SHIPMENT', 'NETWORK_ROUTING', 'NODE_OPERATIONS',
            'TRANSPORT_FULFILLMENT', 'CUSTOMS_COMPLIANCE', 'WAREHOUSE_FULFILLMENT'
        ));
