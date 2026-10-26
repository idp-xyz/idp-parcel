-- 交接版本留存登记方的段声明（ADR-0307 决定二；票 demo-chain-gaps/130）。
--
-- 首登命令里的段、计划段与段服务动作三格此前只在那一刻用于进段、不落任何地方；待确认或拒收不进段，那份声明随之丢掉，
-- 日后更正成`已交接`时没有段可进。三列原样存登记方给的串，更正行照前版承继（那是编排的事，库面不另设跨行约束）。
--
-- 三列各自可空：NULL 即没有声明，与进段那道门的缺席同一种答案。存原串、不设外键也不核段登记册：段号立不起来、服务动作
-- 词不在封闭集合里，都由进段那一刻的那道门作答，首登与更正带对象进段时答得一样；库面只拒空白——空白与 NULL 分开会让
-- 「没有声明」有两种写法。声明不是交接内容：不进摘要，同键重放换了声明照答重放，在册的仍是先到的那一份。
--
-- 存量行三列全为 NULL，读作没有声明：存量里待确认或拒收的那一版日后更正成`已交接`，照旧结束对象此刻在场的参与、不进段。
-- 新的 CHECK 对存量恒成立，直接校验。
ALTER TABLE transport_fulfillment.transport_handover
    ADD COLUMN declared_segment                text,
    ADD COLUMN declared_planned_segment        text,
    ADD COLUMN declared_segment_service_action text;

ALTER TABLE transport_fulfillment.transport_handover
    ADD CONSTRAINT transport_handover_segment_declaration_not_blank
        CHECK (
            (declared_segment IS NULL OR btrim(declared_segment) <> '')
            AND (declared_planned_segment IS NULL OR btrim(declared_planned_segment) <> '')
            AND (declared_segment_service_action IS NULL OR btrim(declared_segment_service_action) <> '')
        );
