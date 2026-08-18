-- 当前提交版本的查询投影：给「包裹 → 当前已接受委托」反查用（ADR-0060）。
--
-- 不另建投影表，也不对 snapshot jsonb 建表达式索引。两列是当前聚合快照的查询投影，
-- 由 Insert/Save 与 snapshot 同一条 SQL 写下；分两次写会在两次之间被按包裹查到一份
-- 与快照不一致的目标。
--
-- 回填走 snapshot.currentVersion 的 JSON 路径（适配器写下的形状），再收紧 NOT NULL /
-- CHECK。空数组不放行：领域要求至少一件包裹，库上镜像那条门。

ALTER TABLE parcel_shipment.shipment_request
    ADD COLUMN current_submission_version_id text,
    ADD COLUMN declared_parcel_ids text[];

UPDATE parcel_shipment.shipment_request
   SET current_submission_version_id = snapshot #>> '{currentVersion,versionId}',
       declared_parcel_ids = ARRAY(
           SELECT jsonb_array_elements_text(
               COALESCE(snapshot #> '{currentVersion,declaredParcelIds}', '[]'::jsonb)
           )
       );

ALTER TABLE parcel_shipment.shipment_request
    ALTER COLUMN current_submission_version_id SET NOT NULL,
    ALTER COLUMN declared_parcel_ids SET NOT NULL;

-- PostgreSQL 不允许 CHECK 含子查询，逐元 btrim 只能包进 IMMUTABLE 函数。
CREATE FUNCTION parcel_shipment.declared_parcel_ids_have_no_blank(ids text[])
RETURNS boolean
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT NOT EXISTS (
        SELECT 1
          FROM unnest(ids) AS parcel(id)
         WHERE btrim(parcel.id) = ''
    );
$$;

ALTER TABLE parcel_shipment.shipment_request
    ADD CONSTRAINT shipment_request_current_version_not_blank
        CHECK (btrim(current_submission_version_id) <> ''),
    ADD CONSTRAINT shipment_request_declared_parcels_present
        CHECK (cardinality(declared_parcel_ids) >= 1),
    ADD CONSTRAINT shipment_request_declared_parcels_not_blank
        CHECK (parcel_shipment.declared_parcel_ids_have_no_blank(declared_parcel_ids));

-- 反查只认已接受（state=2）。部分 GIN 让 submitted/rejected/withdrawn 不进索引。
-- 2 = ShipmentRequestAccepted。
CREATE INDEX shipment_request_accepted_parcels_gin
    ON parcel_shipment.shipment_request
    USING GIN (declared_parcel_ids)
    WHERE state = 2;
