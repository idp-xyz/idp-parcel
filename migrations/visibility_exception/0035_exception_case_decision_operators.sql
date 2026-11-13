-- 异常案件记下是谁接的单、谁关的案（UC-VE-005「分派、接单……分别形成」）。
--
-- 两格都可空：接单、关闭此前没有任何生产入口，既有行这两格本来就没有人；待响应直接关闭的案件没有接单人。它们是操作
-- 证据，不是案件聚合的状态——换一个人来接不改变案件处在哪一格，所以不进主状态的那几条 CHECK，只守「没接单就没有接单
-- 人、没关闭就没有关闭人」。

ALTER TABLE visibility_exception.exception_case
    ADD COLUMN first_response_by text,
    ADD COLUMN closed_by text;

ALTER TABLE visibility_exception.exception_case
    ADD CONSTRAINT exception_case_decision_operators_follow_the_decisions
        CHECK (
            (first_response_by IS NULL OR (first_response IS NOT NULL AND btrim(first_response_by) <> ''))
            AND (closed_by IS NULL OR (closed_at IS NOT NULL AND btrim(closed_by) <> ''))
        );
