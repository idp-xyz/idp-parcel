-- 已接受事实登记来源事实替代关系（ADR-0065）。
--
-- supersedes_version 由源上下文随更正一并给出，指名本份取代的那一版；关系只在同一
-- 源上下文、同一事实引用的版本之间成立，因此只登版本、不另存源与引用两维。无前身是
-- 常态（首登事实），列可空、无默认——替代关系只能由源上下文给出，本上下文不发明。
-- 幂等键照旧（租户+来源上下文+事实引用+来源版本），前身维进内容指纹不进键。
--
-- 指名自己为前身是坏写入：沿用原版本号就是覆盖，不是更正。领域构造期已拦，库内
-- CHECK 是第二道，两道互补不互替。

ALTER TABLE visibility_exception.accepted_fact
    ADD COLUMN supersedes_version text,
    ADD CONSTRAINT accepted_fact_supersedes_shape
        CHECK (
            supersedes_version IS NULL
            OR (btrim(supersedes_version) <> '' AND supersedes_version <> fact_version)
        );
