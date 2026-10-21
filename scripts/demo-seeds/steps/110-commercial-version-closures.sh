echo "== 商业版本收尾（party-commercial：同起点重发的旧版按决定退役） =="
# 干线协议 SYN-SUPPLIER-TRUNK-CN-SG-01 的 v2（声明收费发生条件，ADR-0209）与 v3（再声明订舱与取消）都是对 v1 的同起点重发：
# 三版同一区间、同一份发布批里先后登进来。不收尾时三版都`已生效`，指名 v1 或 v2 的委托与发生项登记照旧被接受（票
# demo-chain-gaps/50 的现象）。这里按 ADR-0233 决定三把 v1 让位给 v2、v2 让位给 v3：退役时刻取登记时刻，重跑整份种子时两项
# 以重放回答。排在 100 之后：要退的版本与后继都由那一节的发布批登进来。演示动线指名的是 v3，不受影响；指名 v1、v2 的
# 核对此后答已终止。合成取值，证据记 S。
"$BIN/parcel-commercial" register-version-closures \
  -input "$SEEDS/commercial/close-trunk-agreement-reissues.json"
