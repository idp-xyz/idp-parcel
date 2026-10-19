-- 参与方关系册的角色封闭集多一格授权交付方（AUTHORIZED_DELIVERING_PARTY；ADR-0212 决定一，PC CONTEXT「参与方关系」）：
-- 一个参与方（交付方）在适用范围与有效期间内可以替某个客户账户向物流节点送站。这条 CHECK 镜像 domain.PartyRole，
-- 0015 起的那一版今天会拒新角色；已施加的迁移按校验和守着，不改 0015，放宽只能是一份新迁移（判据同 0004）。
--
-- 范围的写法（`customerAccount=…`，可再接 `&node=…`）不在这里另写一道 CHECK：它的唯一拼法由 domain.AuthorizedDeliveryScope
-- 判，SQL 里只能镜像个前缀，镜像半截的门比没有门更会让人以为库替领域守住了。
--
-- 授权交付判断按（租户，角色）问本租户登没登过，按（租户，角色，持有方）取交付方名下的关系；下面这条索引两问都用得上。

ALTER TABLE party_commercial.party_relationship_registration
    DROP CONSTRAINT party_relationship_registration_role_closed;

ALTER TABLE party_commercial.party_relationship_registration
    ADD CONSTRAINT party_relationship_registration_role_closed
        CHECK (role IN ('CUSTOMER', 'SUPPLIER', 'CARRIER_AGENT', 'RESELLER', 'ACCOUNT_HOLDER',
                        'AUTHORIZED_DELIVERING_PARTY'));

CREATE INDEX party_relationship_registration_by_role_holder
    ON party_commercial.party_relationship_registration
        (tenant_id, role, holder_party_id);
