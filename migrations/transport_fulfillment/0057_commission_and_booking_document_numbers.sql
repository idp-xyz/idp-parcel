-- 运输委托与订舱的单据号（ADR-0355 决定二、三、七、八；票 document-numbering/09）。
--
-- 两张单据表各加两列：单据号，与没有号时的原因。委托进入已提交、订舱申请登记时在同一事务里取号；号开立后不变，取消、
-- 开始与协议换版都不碰这两列。
--
-- 可空，不回填：本迁移之前开立的委托与订舱不补号（决定三）——事后多出一个号，已发给承运方的那份就对不上了。两列都空
-- 读作「本规则生效前开立」；开立时租户没登单据编号时区而无号的，记原因 NUMBERING_TIME_ZONE_NOT_CONFIGURED，之后补登时区
-- 也不补号（决定七）。有号就不得再记原因，原因只有这一个取值；号给了就不得是空白。
--
-- （租户，单据号）唯一是决定八的最后一道保险：取号编排渲染出号先查本表是否已用、撞了就跳过，约束只在那一步之外仍撞时
-- 让整笔开立回滚。单据号可空，唯一约束对旧行的 NULL 不起作用，在有数据的表上直接施加。

ALTER TABLE transport_fulfillment.transport_commission
    ADD COLUMN document_number text,
    ADD COLUMN document_number_absence text;

ALTER TABLE transport_fulfillment.transport_commission
    ADD CONSTRAINT transport_commission_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),
    ADD CONSTRAINT transport_commission_document_number_absence_known
        CHECK (document_number_absence IS NULL OR document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'),
    ADD CONSTRAINT transport_commission_document_number_or_absence
        CHECK (document_number IS NULL OR document_number_absence IS NULL),
    ADD CONSTRAINT transport_commission_document_number_unique
        UNIQUE (tenant_id, document_number);

ALTER TABLE transport_fulfillment.booking_request
    ADD COLUMN document_number text,
    ADD COLUMN document_number_absence text;

ALTER TABLE transport_fulfillment.booking_request
    ADD CONSTRAINT booking_request_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),
    ADD CONSTRAINT booking_request_document_number_absence_known
        CHECK (document_number_absence IS NULL OR document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'),
    ADD CONSTRAINT booking_request_document_number_or_absence
        CHECK (document_number IS NULL OR document_number_absence IS NULL),
    ADD CONSTRAINT booking_request_document_number_unique
        UNIQUE (tenant_id, document_number);
