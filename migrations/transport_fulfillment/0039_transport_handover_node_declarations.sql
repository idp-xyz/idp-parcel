-- 权威运输交接声明交出、接收与去向节点（ADR-0213；票 demo-chain-gaps/59）。
--
-- 三列各自可空：NULL 即登记方没有声明，缺席是一种答案，不是待补。存的是 network-routing 的节点代码，不透明，不设外键、
-- 也不核节点目录——节点只能声明，本上下文不核它在不在、对不对。空白串与 NULL 分开会让「没有声明」有两种写法，库面拒。
--
-- 存量行三列全为 NULL，读回与此后不声明节点的首登同形；存量摘要不回写（ADR-0014），不声明节点的首登仍写 TFC-1，原内容
-- 重放照答重放。更正行照前版承继三列，那是编排与领域的事，库面不另设跨行约束。
ALTER TABLE transport_fulfillment.transport_handover
    ADD COLUMN releasing_node text,
    ADD COLUMN receiving_node text,
    ADD COLUMN onward_node    text;

ALTER TABLE transport_fulfillment.transport_handover
    ADD CONSTRAINT transport_handover_nodes_not_blank
        CHECK (
            (releasing_node IS NULL OR btrim(releasing_node) <> '')
            AND (receiving_node IS NULL OR btrim(receiving_node) <> '')
            AND (onward_node IS NULL OR btrim(onward_node) <> '')
        );
