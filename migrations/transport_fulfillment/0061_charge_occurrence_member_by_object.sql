-- 按载运对象反查仍载有它的收费发生项。主键从发生项出发，这份索引从对象出发。

CREATE INDEX transport_charge_occurrence_member_by_object
    ON transport_fulfillment.transport_charge_occurrence_member (tenant_id, object_ref);
