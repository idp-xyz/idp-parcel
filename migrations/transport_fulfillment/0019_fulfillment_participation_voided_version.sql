-- 履约参与关系链上的失效版本（ADR-0112 决定四；票 tf-segment-lifecycle-closure/11 裁决 1）。
--
-- 0016 让同段内的参与关系长替代链：来源更正形成回指前版的新版本，链尾是当前参与。那条链只覆盖「更正后仍转出
-- 控制」的替代格；`已交接`被更正为拒收或待确认时，凭前版入场的参与失去了入场依据，却没有新的控制起点可立替代
-- 版本——CONTEXT 生命周期句要求这时「形成失效……关系并重新派生当前有效控制」。失效版本与替代版本走同一条链、
-- 同一个窄写口（Supersede，只插不改），行上只多一列 voided：链尾失效即该对象在本段当前无有效参与。
--
-- 失效版本的其它列照替代版本的写法：supersedes_entry_basis 回指被失效那一版；entry_basis 取撤回控制那一版的
-- 版本引用 TRANSPORT-HANDOVER/<新版本>（TransferOutBasis 对它不给，但版本引用本身是有的，它是「这一版判断说了
-- 什么」的身份）；entry_kind 照前版；entered_at 沿用被失效那一版的起点——它记的是「起于 T 的那条参与失效了」，
-- 列保持 NOT NULL、链按入场时刻排序不乱；离场三列继承前版（交付事实没有被更正撤销，Voided 与 ended_at 各说各的）。
--
-- **原参与那一行一字不动。** 「一格状态」要 UPDATE 原参与，违只插不改；「另立一种对象」会让链断成两截。
-- 不回退到前一仍转出控制的版本：那一版已被回指，回到它就是把已被更正的判断重新当成当前
-- （CONTEXT「不能简单回填为早期原控制方」）。
ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD COLUMN voided boolean NOT NULL DEFAULT false;

-- 失效版本必回指前版（首登不能失效：对象从未进段就没有东西可失效），且只出自交接更正（揽收更正在构造期就拒
-- 「更正成失败到访」，走不到这一格）。领域重建门守同一形。
ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD CONSTRAINT fulfillment_participation_voided_coherent
        CHECK (
            NOT voided
            OR (
                supersedes_entry_basis IS NOT NULL
                AND entry_kind = 'TRANSPORT_HANDOVER'
            )
        );

-- 一条**故意没有下沉**的不变量，与 0016 尾注同一个理由（跨行条件、不立第二口径）：
--   - 「在场」= ended_at 为空 **且未失效** 且无人回指——EndParticipation 与 FindActiveSegments 两个窄口用同一段
--     谓词表达它；领域 Active() 三条同一。被失效的对象不会被结束（EndParticipation 对失效链尾答`已离场`），
--     CloseSegment 与重建门按同一判据数在场。
