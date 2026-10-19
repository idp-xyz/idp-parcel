-- 收寄记录的交付范围核对态（ADR-0212 决定五，票 ps-external-mark-relations/11）：每一版做成了身份核对的判断记下这次推出的可验证
-- 交付范围是已证、未证（附原因，「依据不适用」再附细分）还是未配置。它是信息格，不进 outcome；收寄答复与复核目录都带它。
-- 三列分立而不并成一份文档：取值、原因与细分各有封闭词表，分列才能各自在库面封闭，复核目录也按列转写。
--
-- 库面守与新旧无关的形状：三列各自封闭；原因只随未证、未证必带原因；细分只随「依据不适用」、「依据不适用」必带细分；只有收寄已形成
-- 与待识别两格带（未收寄、待确认与并入没有做身份核对）；未核对的待识别不带（没取得 PS 的答复，这一版没做成核对）。比较前先
-- COALESCE，免得 NULL 让整条 CHECK 按三值逻辑放行。核对过的待识别与收寄已形成**不**要求必带：
--
-- **既有行留空，不回填。** 本迁移之前落下的判断都没有推导过范围（那时还没有依据引用这一格），推定成任何一格都是替当时编一个没做过
-- 的判断。空即「这一版没记」，复核目录如实不带。ADR-0196 给先到那件追加的冲突版、身份冲突处置版也不带：那几版不是对那件实物本身做
-- 的范围推导。
--
-- 来源文档里的交付范围依据引用（决定七）不另立列：它随来源文档的 NRS-2 形状存，NRS-1 的旧文档读作没有依据，不改写。

ALTER TABLE node_operations.reception
    ADD COLUMN delivery_scope_check text,
    ADD COLUMN delivery_scope_unverified_reason text,
    ADD COLUMN delivery_scope_inapplicability text;

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_delivery_scope_check_closed
        CHECK (delivery_scope_check IS NULL OR delivery_scope_check IN ('VERIFIED', 'UNVERIFIED', 'UNCONFIGURED'));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_delivery_scope_reason_closed
        CHECK (delivery_scope_unverified_reason IS NULL OR delivery_scope_unverified_reason IN
               ('NO_BASIS', 'BASIS_NOT_REGISTERED', 'BASIS_NOT_APPLICABLE', 'NUMBER_NOT_PRE_ADVISED', 'BINDINGS_NOT_ALL_CONFIRMED'));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_delivery_scope_inapplicability_closed
        CHECK (delivery_scope_inapplicability IS NULL OR delivery_scope_inapplicability IN ('NODE_MISMATCH', 'OUTSIDE_WINDOW', 'WITHDRAWN'));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_delivery_scope_reason_with_unverified
        CHECK ((COALESCE(delivery_scope_check, '') = 'UNVERIFIED') = (delivery_scope_unverified_reason IS NOT NULL));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_delivery_scope_inapplicability_with_reason
        CHECK ((COALESCE(delivery_scope_unverified_reason, '') = 'BASIS_NOT_APPLICABLE') = (delivery_scope_inapplicability IS NOT NULL));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_delivery_scope_check_checked_judgments_only
        CHECK (delivery_scope_check IS NULL
            OR (kind IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION') AND COALESCE(identity_check, '') <> 'UNCHECKED'));
