echo "== FBA 头程服务产品与它的接单规则包；演示合同重发不指名接单规则包（ADR-0376） =="
# 演示租户的第二个服务产品 SYN-PROD-CN-SG-FBA（票 declared-goods-lines/12）：同一个货主账户 SYN-ACCOUNT-01 同时用小包快递与
# FBA 头程两个产品，各带自己的接单规则包（ADR-0376 决定三）——规则包按委托申报的服务产品选（ADR-0153），FBA 那一份声明「必要
# 资料」组适用，并以 CROSS_FIELD_CONDITION 引用随产品发布的 FBA 头程提交条件参考配置（决定一）：缺平台货件、缺箱标号、没报装箱
# 的委托停在等待受控补充。产品的客户服务规则照快递那一份另登一份（闭包要求客户服务规则依据，挂产品的版本按产品收窄）。
#
# 演示合同 SYN-CONTRACT-01 重发 v2：外壳与正文都不再指名接单规则包（票 11）——合同指名了只作确认，指着小包那一份，申报 FBA
# 产品的委托就答指名引用未确认。结算政策的正文按合同版本配（ADR-0080），随之重发 v2 指 v2 合同。两者 v1 照 ADR-0233 决定三
# 让位给 v2，v1 原文不改。新对象单放一份发布批，不并进 100 那一份：那一份的逐项回显被受控发布口的测试逐字钉着。
#
# 产品取值全是演示租户的合成采用值，证据记 S。排在 115 之后：信用政策重发不碰合同；排在 100 之后：v1 与快递产品由那一节登进来。
"$BIN/parcel-commercial" publish -input "$SEEDS/commercial/publish-fba-product.json"
"$BIN/parcel-commercial" register-version-closures \
  -input "$SEEDS/commercial/close-fba-contract-reissue.json"
# 产品到渠道的映射照快递产品：同一张演示网络、同两个渠道引用。
"$BIN/parcel-commercial" register-products -input "$SEEDS/commercial/register-products-fba.json"
