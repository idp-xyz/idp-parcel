-- 出库请求拆单（UC-WF-012）。原请求终结为已拆单并放开预占。新请求用 parent_request_id 指名它。
-- 本迁移不种任何行。

ALTER TABLE warehouse_fulfillment.outbound_request
    DROP CONSTRAINT outbound_request_status;

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD COLUMN parent_request_id text;

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD CONSTRAINT outbound_request_status CHECK (status IN (
        'RESERVED', 'PARTIALLY_RESERVED', 'REJECTED', 'PENDING_POLICY', 'HANDED_OFF', 'SPLIT'
    )),
    ADD CONSTRAINT outbound_request_parent_fkey
        FOREIGN KEY (tenant_id, parent_request_id)
        REFERENCES warehouse_fulfillment.outbound_request (tenant_id, request_id),
    ADD CONSTRAINT outbound_request_parent_shape CHECK (
        parent_request_id IS NULL OR (status <> 'SPLIT' AND btrim(parent_request_id) <> '')
    );
