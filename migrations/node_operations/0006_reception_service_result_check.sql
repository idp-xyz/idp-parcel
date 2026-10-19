-- 收寄记录的服务结果核对态（ADR-0186）：关联收寄向 PS 取服务结果时，取得了答复记 CHECKED，读口交回错误、没取得答复
-- 记 UNREADABLE。两者的标记都可能是空数组，只有这一列把「查过了，没有取消与终局」与「没问到」分开。
--
-- 库面守形状，绕过适配器直接写库的一行也过这道门：取值封闭；只有 INTAKE_FORMED 带（其余三种没有可问的包裹）；
-- UNREADABLE 的标记必为空数组——没取得答复就没有可合成的标记。比较前先 COALESCE，免得 NULL 让整条 CHECK 按三值
-- 逻辑放行。INTAKE_FORMED 不带核对态这一格库里不拦：编排对每条形成格都写，回填之后既有行也都带；收紧成「必带」
-- 要连带改所有直接落形成格的测试夹具，留给需要它的那一票。
--
-- **既有 INTAKE_FORMED 行回填 CHECKED**，这是推定，依据如下（取证时的 SHA 钉在这里，原因写在各自的提交说明里）：
--   - 0214a62a（收寄在身份唯一成立时向 PS 取服务结果）早于 cc52e9b6（节点身份核对接真），是它的祖先提交；
--   - cc52e9b6 之前，parcel-api 与前线导入两处装配的身份核对都是未配置桩，明确接收从不落册——见 eac94e06 的提交说明与
--     票 operator-channel/08 重走记录表里「明确接收 `RECEPTION_UNDECIDED`」那一格；所以 INTAKE_FORMED 行只可能在
--     cc52e9b6 之后写下，那时服务结果已经在问；
--   - 0214a62a 起到本迁移所在的票（operator-channel/29）之前，服务结果读不动时编排答不落册的待确认，INTAKE_FORMED
--     只在读口答复之后才落；
--   - 收寄表唯一的生产写方是 NO 的 postgres 适配器（Receptions.Save），没有别的路径写下 INTAKE_FORMED 行。
-- 于是每一条既有 INTAKE_FORMED 行都是在取得 PS 答复之后写的，回填 CHECKED 说的是当时的真话。其余三种不回填。

ALTER TABLE node_operations.reception
    ADD COLUMN service_result_check text;

UPDATE node_operations.reception
   SET service_result_check = 'CHECKED'
 WHERE kind = 'INTAKE_FORMED';

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_service_result_check_closed
        CHECK (service_result_check IS NULL OR service_result_check IN ('CHECKED', 'UNREADABLE'));

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_service_result_check_formed_only
        CHECK (kind = 'INTAKE_FORMED' OR service_result_check IS NULL);

ALTER TABLE node_operations.reception
    ADD CONSTRAINT reception_unreadable_carries_no_markers
        CHECK (COALESCE(service_result_check, '') <> 'UNREADABLE' OR service_markers = '[]'::jsonb);
