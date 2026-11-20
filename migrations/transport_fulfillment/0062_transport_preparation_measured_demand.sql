-- 运输准备按实测再判一份，与申报那一份并排留着（同一段、同一对象、同一规则）。
--
-- 身份从四维扩成六维：来源与实测引用进主键。申报行的实测引用是空串；实测行的引用就是那条实测的来源身份，
-- 并且与需求量锚同一串——锚是快照里的来源身份，引用是键上的那一维，两格不等就分不清这行是按哪一条判的。
-- 已有的行都是申报，引用由默认空串填上，按原来的四维再加这两个空出来的位置仍能读回。
--
-- 配对约束只约束两种认得的来源。集外的词由封闭词那一道拒：若配对约束也把集外词判失败，两道同时失败时库只报其中一道，
-- 读错误的人会以为是引用没配对。
--
-- 查阅册缺省序在判断时刻之后以行标识决胜。标识补上来源与实测引用，索引跟着补，同一段的两行在翻页上才是全序。
ALTER TABLE transport_fulfillment.transport_preparation
    ADD COLUMN measurement_ref text NOT NULL DEFAULT '';

ALTER TABLE transport_fulfillment.transport_preparation
    DROP CONSTRAINT transport_preparation_pkey;

ALTER TABLE transport_fulfillment.transport_preparation
    ADD CONSTRAINT transport_preparation_pkey
        PRIMARY KEY (tenant_id, planned_leg_ref, object_ref, rule_version, demand_source, measurement_ref);

ALTER TABLE transport_fulfillment.transport_preparation
    DROP CONSTRAINT transport_preparation_demand_source_closed;

ALTER TABLE transport_fulfillment.transport_preparation
    ADD CONSTRAINT transport_preparation_demand_source_closed
        CHECK (demand_source IN ('declaration', 'actual-measurement'));

ALTER TABLE transport_fulfillment.transport_preparation
    ADD CONSTRAINT transport_preparation_measurement_ref_matches_source
        CHECK (
            (
                demand_source = 'declaration'
                AND measurement_ref = ''
            )
            OR (
                demand_source = 'actual-measurement'
                AND btrim(measurement_ref) <> ''
                AND demand_anchor IS NOT NULL
                AND demand_anchor = measurement_ref
            )
            OR (
                demand_source <> 'declaration'
                AND demand_source <> 'actual-measurement'
            )
        );

DROP INDEX transport_fulfillment.transport_preparation_judged;
CREATE INDEX transport_preparation_judged
    ON transport_fulfillment.transport_preparation
        (tenant_id, judged_at DESC, planned_leg_ref DESC, object_ref DESC, rule_version DESC,
         demand_source DESC, measurement_ref DESC);
