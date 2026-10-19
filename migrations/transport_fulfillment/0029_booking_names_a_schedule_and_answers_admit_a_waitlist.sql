-- 订舱指名具体班次，承运答复加候补（票 demo-chain-gaps/24，ADR-0202 决定五与文末补记）。
--
-- 订舱必须引用一个在册的具体班次：班次在不在册由提交编排核，库上只钉「有这一格」。不对班次表设外键：班次一版一行
-- （0024），（租户，班次）在那张表上不唯一，可引用的是身份而不是某一版。
--
-- 答复封闭集加 WAITLISTED（候补）：不成约、没有接受量、带依据，与拒绝、失效、撤回同形。候补不是结局——同一订舱在候补
-- 之后还可以有一份定局答复，两份都留，所以主键从（租户，订舱）改为（租户，订舱，答复标识），另用两条部分唯一索引钉住
-- 「至多一份候补、至多一份定局」。「定局不早于候补」「定局之后不再有后继」是跨行的条件，两条索引各钉一种答复、CHECK
-- 也够不着：由编排判（domain.CarrierAcceptance.AdmitsSuccessor），判之前先取同一订舱的事务级锁再读答复
-- （BookingAnswers.FindAnswersForUpdate）——读已提交下不锁，两份并发答复会各自读到空册、各自落库。绕过编排直插的行
-- 不受这把锁约束。
--
-- 两张表在订舱与答复两口接线之前没有任何写入方，应当没有行。有行就拒绝迁移，不替任何人补一个班次——那一次订舱申请的是
-- 哪一班，只有提交它的一方答得出。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：basis 可空，先 IS NULL / IS NOT NULL 再比。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM transport_fulfillment.booking_request)
       OR EXISTS (SELECT 1 FROM transport_fulfillment.booking_answer) THEN
        RAISE EXCEPTION USING MESSAGE =
            'booking_request 或 booking_answer 已有行，不替它们猜具体班次：请导出这些行备查，由提交它们的一方按所申请的'
            || '具体班次重新提交后清空两表，再重跑本迁移。';
    END IF;
END
$$;

ALTER TABLE transport_fulfillment.booking_request
    ADD COLUMN schedule_id text NOT NULL;

ALTER TABLE transport_fulfillment.booking_request
    ADD CONSTRAINT booking_request_schedule_not_blank
        CHECK (btrim(schedule_id) <> '');

ALTER TABLE transport_fulfillment.booking_answer
    DROP CONSTRAINT booking_answer_pkey;

ALTER TABLE transport_fulfillment.booking_answer
    ADD CONSTRAINT booking_answer_pkey
        PRIMARY KEY (tenant_id, booking_id, acceptance_id);

-- 一次订舱至多一份定局：已接受的订舱不能再被拒绝，改约走撤回与新订舱。
CREATE UNIQUE INDEX booking_answer_one_settling
    ON transport_fulfillment.booking_answer (tenant_id, booking_id)
    WHERE outcome <> 'WAITLISTED';

-- 一次订舱至多一份候补：候补之后再候补只是把同一件事又说一遍。
CREATE UNIQUE INDEX booking_answer_one_waitlist
    ON transport_fulfillment.booking_answer (tenant_id, booking_id)
    WHERE outcome = 'WAITLISTED';

ALTER TABLE transport_fulfillment.booking_answer
    DROP CONSTRAINT booking_answer_outcome_closed;

ALTER TABLE transport_fulfillment.booking_answer
    ADD CONSTRAINT booking_answer_outcome_closed
        CHECK (outcome IN ('ACCEPTED', 'REFUSED', 'EXPIRED', 'WITHDRAWN', 'WAITLISTED'));

ALTER TABLE transport_fulfillment.booking_answer
    DROP CONSTRAINT booking_answer_outcome_shape;

ALTER TABLE transport_fulfillment.booking_answer
    ADD CONSTRAINT booking_answer_outcome_shape
        CHECK (
            (outcome = 'ACCEPTED'
                AND quantity > 0
                AND basis IS NULL)
            OR (outcome IN ('REFUSED', 'EXPIRED', 'WITHDRAWN', 'WAITLISTED')
                AND quantity = 0
                AND basis IS NOT NULL AND btrim(basis) <> '')
        );
