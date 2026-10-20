echo "== 商业权威发布（party-commercial：发布批正文）与消费方解析键登记 =="
"$BIN/parcel-commercial" publish -input "$SEEDS/commercial/publish-batch.json"
# 解析键属消费方（parcel-shipment）的实例半边，与上一行的商业权威发布不是同一件事，
# 只因同一个 CLI 承两个入口才挨在一起。键上要结算依据、因而也要客户合同——结算政策按
# 哪一版合同选由闭包解出（ADR-0080），登记面结构上没有合同维。
#
# 上一行的发布批里 `SYN-SETTLEMENT-PREPAID-01` 的六维正是照这行键的三维（相对方、费用
# 范围、币种）加闭包会解出的合同版本配的，闭包因此解得开。六维差一维就命不中：这不是
# 巧合要维护，而是本上下文明禁借宽泛客户关系跨维归集。
"$BIN/parcel-commercial" register-resolution-key \
  -input "$SEEDS/commercial/resolution-key-syn-account-01.json"
# 第二客户 SYN-ACCOUNT-02 的键：形状照上一行，三维换成它自己的相对方 SYN-PARTY-SHIPPER-02，闭包解出它自己的合同
# SYN-CONTRACT-02、再与 SYN-SETTLEMENT-PREPAID-02 的六维命中。两户的必需依据都列 CUSTOMER_SERVICE_RULE：
# 不列，闭包就不采用任何客户服务规则版本，索赔的首次索赔期限与最低材料两维恒答未登记。
# 两户的必需依据也都列 PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY：不列，闭包就不采用合同上绑的
# SYN-FIN-CONTROL-01，受理前财务控制答策略未配置（票 product-strategy-boundary/26 实跑停在这一格）。
# 同一范围并存两版客户服务规则：挂产品的 SYN-SERVICE-RULE-01（首次索赔 14 天、LOSS 要 POD）与壳指名合同 02 的
# SYN-SERVICE-RULE-CONTRACT-02（只登首次索赔 7 天）。闭包合同优先（ADR-0176 决定一）：账户 02 采纳合同版，
# 期限答 7 天、材料行继承产品版的 POD（决定二）；账户 01 的合同层零候选，回落产品版，答复与没有合同版时相同。
"$BIN/parcel-commercial" register-resolution-key \
  -input "$SEEDS/commercial/resolution-key-syn-account-02.json"
# 注册号类型目录（票 legal-entity-profile/01、product-strategy-boundary/03）：身份层显式采用 CN、SG 两份
# 参考配置（ADR-0147），资料层各一类合成类型，另一类登记后停用。
# 排在参与方身份之前：责任法人身份登记按注册国家 / 地区在这份目录里判号（ADR-0145 决定一）。
"$BIN/parcel-commercial" register-registration-number-types \
  -input "$SEEDS/commercial/register-registration-number-types.json"
# 参与方身份四册（票 admin-remainder-mechanism-batch/01）：SYN-LE-01 与 SYN-ACCOUNT-01
# 在此获得身份册登记，与上面发布批里的同名引用同指一物。停用批单独一笔，让法人页
# 与参与方页的身份状态三格（已登记/已生效/已停用）都有真实例可显。
"$BIN/parcel-commercial" register-parties -input "$SEEDS/commercial/register-parties.json"
# 授权交付关系（PAR-NET-17，ADR-0212 决定一）：一个合成交付方替 SYN-ACCOUNT-01 向节点送站，范围只收到客户账户、不收到节点。
# 排在参与方身份之后：关系的相对方要是 SYN-ACCOUNT-01 所属的参与方，登记用例对着上一行登的账户判。演示租户的合成取值，
# 证据记 S；不登时本租户的送站交接确认答未配置。
"$BIN/parcel-commercial" register-parties -input "$SEEDS/commercial/register-authorized-delivery.json"
"$BIN/parcel-commercial" deactivate-party-identity \
  -input "$SEEDS/commercial/deactivate-party-retired-01.json"
# 法人资料（票 legal-entity-profile/03）：SYN-LE-01 两笔修订，先不带开票资料、后补抬头，让资料页的「资料不全」与
# 修订切换都有真实例可显。排在参与方身份之后：资料引用已登记的责任法人，地址国家对着它的身份层判。
"$BIN/parcel-commercial" register-legal-entity-profiles \
  -input "$SEEDS/commercial/register-legal-entity-profiles.json"
# 服务形态与产品—渠道映射（票 admin-remainder-mechanism-batch/02）：EXPRESS 配两个
# 渠道引用，ECON 显式登记「未配置」——批级判据点名实例格显式未配置要有真实例可显；
# 渠道本体不预造（ADR-0072），引用等 PAR-INT-01 的接入证据。
"$BIN/parcel-commercial" register-products -input "$SEEDS/commercial/register-products.json"
