-- 证据项记下是谁登的、谁评价的、什么时候评价的（UC-VE-007 步 2）。
--
-- 三格都可空：证据的登记与评价此前没有任何生产入口，既有行没有这些人；不经运营口到达的证据项也没有登记人。它们是操作
-- 证据，不是证据项本体——换一个人来登不改变这份材料是什么。只守「没评价就没有评价人与评价时刻，评价了两格同在」。

ALTER TABLE visibility_exception.evidence_item
    ADD COLUMN registered_by text,
    ADD COLUMN appraised_by text,
    ADD COLUMN appraised_at timestamptz;

ALTER TABLE visibility_exception.evidence_item
    ADD CONSTRAINT evidence_item_decision_operators_follow_the_decisions
        CHECK (
            (registered_by IS NULL OR btrim(registered_by) <> '')
            AND (appraised_by IS NULL) = (appraised_at IS NULL)
            AND (appraised_by IS NULL OR (appraisal <> 'RECEIVED' AND btrim(appraised_by) <> ''))
        );
