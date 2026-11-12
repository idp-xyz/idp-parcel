import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { AcceptanceTaskRecord } from './api';
import { lastAttemptOf } from './last-attempt';

// 本文件钉运单详情上「最近处理结果」与「谁来续办」的写法（ADR-0383 决定五）：带与不带结算细原因、带与不带服务产品（只接在财务
// 控制那一步）、续办方各词（等运营登记分价卡一侧与别处、内部重试、等客户补充照旧、复核与授权处置）、续办另起一句时停因不重说
// 等运营登记、不要续办的页、续办方缺席或认不得、任务已不在运行、认不得的码；申报不全（ADR-0381 决定五）按缺项该谁补说细：
// 等客户补充列缺口清单、卡侧缺项写明这张卡做不了受理前估价与该登什么。

const running = (extra: Partial<AcceptanceTaskRecord>): AcceptanceTaskRecord => ({ state: 'RUNNING', ...extra });

const priceCardWait = {
  path: 'OPERATOR_REGISTRATION',
  text: '登记一张生效期覆盖提交时点的价卡后自动续办',
  link: { href: '#/price-card-catalog', label: '去价格表' },
};

const registrationWait = { path: 'OPERATOR_REGISTRATION', text: '运营登记后自动续办' };

test('没有处理尝试：答 null；空串同缺席', () => {
  equal(lastAttemptOf({ state: 'RUNNING' }), null);
  equal(lastAttemptOf(running({ lastAttemptReason: '', lastAttemptSettlementReason: '', lastAttemptResumePath: 'INTERNAL_RETRY' })), null);
  equal(lastAttemptOf({ state: 'COMPLETE', lastAttemptContinuation: 'CONT-abc', lastAttemptResumePath: 'INTERNAL_RETRY' }), null);
});

test('带结算细原因与服务产品：估价没形成说到细原因与产品；价卡一侧的等运营登记写明登哪种卡，链到价格表', () => {
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptSettlementReason: 'ESTIMATE_PRICE_CARD_NOT_CONFIGURED',
        lastAttemptServiceProduct: 'SYN-PROD-FBA-01',
      }),
    ),
    {
      text: '估价没形成：没有在用价卡（服务产品 SYN-PROD-FBA-01）',
      financialStall: '没有在用价卡（服务产品 SYN-PROD-FBA-01）',
      resume: priceCardWait,
    },
  );
});

test('带结算细原因、不带服务产品：只说细原因；价卡一侧的几种都链到价格表', () => {
  for (const [code, text] of [
    ['ESTIMATE_PRICE_CARD_CONFLICT', '价卡适用冲突'],
    ['ESTIMATE_TRIAL_WITHOUT_TOTAL', '有一件试算没有合计'],
    ['ESTIMATE_NOT_POSITIVE', '估价合计不是正数'],
    ['ESTIMATE_TRIAL_NEGATIVE', '有一件试算为负'],
  ] as const) {
    deepEqual(
      lastAttemptOf(
        running({ lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED', lastAttemptResumePath: 'OPERATOR_REGISTRATION', lastAttemptSettlementReason: code }),
      ),
      { text: `估价没形成：${text}`, financialStall: text, resume: priceCardWait },
    );
  }
});

test('等运营登记而登记不在价卡一侧：不说登价卡，也不链；估价以外的细原因说财务控制没形成', () => {
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptSettlementReason: 'CONTROL_POLICY_NOT_CONFIGURED',
        lastAttemptServiceProduct: 'SYN-PROD-01',
      }),
    ),
    {
      text: '财务控制没形成：财务控制策略未配置（服务产品 SYN-PROD-01）',
      financialStall: '财务控制策略未配置（服务产品 SYN-PROD-01）',
      resume: registrationWait,
    },
  );
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptSettlementReason: 'CONTROL_AMOUNT_NOT_CONFIGURED',
      }),
    ),
    { text: '估价没形成：这户没有登记受理前估价采用', financialStall: '这户没有登记受理前估价采用', resume: registrationWait },
  );
  // 续办另起一句说了登记后续办，停因就不再重说「等运营登记」。
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'REJECTION_AUTHORITY_RULES_NOT_CONFIGURED', lastAttemptResumePath: 'OPERATOR_REGISTRATION' })), {
    text: '拒绝授权规则未配置',
    resume: registrationWait,
  });
});

test('不带结算细原因：停因照票 21 的译法，续办另起一句时去掉句尾的等运营登记；停在财务控制时带服务产品', () => {
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED', lastAttemptResumePath: 'OPERATOR_REGISTRATION' })), {
    text: '财务控制缺一项登记',
    financialStall: '财务控制缺一项登记',
    resume: registrationWait,
  });
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptServiceProduct: 'SYN-PROD-01',
      }),
    ),
    {
      text: '财务控制缺一项登记（服务产品 SYN-PROD-01）',
      financialStall: '财务控制缺一项登记（服务产品 SYN-PROD-01）',
      resume: registrationWait,
    },
  );
  // 续办方缺席、或任务已不在运行：没有续办那一句，停因整句说全。
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED' })), {
    text: '财务控制缺一项登记，等运营登记',
    financialStall: '财务控制缺一项登记，等运营登记',
  });
  deepEqual(
    lastAttemptOf({ state: 'COMPLETE', lastAttemptReason: 'REJECTION_AUTHORITY_RULES_NOT_CONFIGURED', lastAttemptResumePath: 'OPERATOR_REGISTRATION' }),
    { text: '拒绝授权规则未配置，等运营登记' },
  );
});

test('不另起续办一行的页（resume: false）：不带续办，停因整句说全', () => {
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptServiceProduct: 'SYN-PROD-01',
      }),
      { resume: false },
    ),
    {
      text: '财务控制缺一项登记，等运营登记（服务产品 SYN-PROD-01）',
      financialStall: '财务控制缺一项登记，等运营登记（服务产品 SYN-PROD-01）',
    },
  );
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptSettlementReason: 'ESTIMATE_PRICE_CARD_NOT_CONFIGURED',
      }),
      { resume: false },
    ),
    { text: '估价没形成：没有在用价卡', financialStall: '没有在用价卡' },
  );
});

test('内部重试：说系统会自动重试；币种对不上不归价卡一侧，不链价格表', () => {
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_FORMED',
        lastAttemptResumePath: 'INTERNAL_RETRY',
        lastAttemptSettlementReason: 'ESTIMATE_CURRENCY_MISMATCH',
      }),
    ),
    {
      text: '估价没形成：各件试算币种不一致',
      financialStall: '各件试算币种不一致',
      resume: { path: 'INTERNAL_RETRY', text: '系统会自动重试' },
    },
  );
});

test('等客户补充照旧：不另起续办那一句，也不接服务产品', () => {
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'CUSTOMER_SUPPLEMENT_PENDING',
        lastAttemptResumePath: 'CUSTOMER_SUPPLEMENT',
        lastAttemptServiceProduct: 'SYN-PROD-FBA-01',
      }),
    ),
    { text: '等客户补充' },
  );
});

test('等人工复核与等授权处置：说由哪个角色动手，不接服务产品', () => {
  deepEqual(
    lastAttemptOf(
      running({ lastAttemptReason: 'MANUAL_REVIEW_PENDING', lastAttemptResumePath: 'MANUAL_REVIEW', lastAttemptServiceProduct: 'SYN-PROD-01' }),
    ),
    { text: '等人工复核', resume: { path: 'MANUAL_REVIEW', text: '复核角色记下复核后续办' } },
  );
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'AUTHORIZED_DISPOSITION_PENDING', lastAttemptResumePath: 'AUTHORIZED_DISPOSITION' })), {
    text: '等授权处置',
    resume: { path: 'AUTHORIZED_DISPOSITION', text: '授权处置角色选定去向后续办' },
  });
});

test('续办方只读落库的那一格：缺席或认不得都不说，也不从停因或续办引用推', () => {
  // 停因是等运营登记那一族、续办引用在场，但续办方缺席：不说。
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptContinuation: 'CONT-0123abcd',
        lastAttemptSettlementReason: 'ESTIMATE_PRICE_CARD_NOT_CONFIGURED',
      }),
    ),
    { text: '估价没形成：没有在用价卡', financialStall: '没有在用价卡' },
  );
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'MANUAL_REVIEW_PENDING', lastAttemptResumePath: 'SOMEONE_NEW' })), { text: '等人工复核' });
});

test('任务已完成或已停止：停因照留，不再说谁来续办', () => {
  for (const state of ['COMPLETE', 'STOPPED']) {
    deepEqual(
      lastAttemptOf({
        state,
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptSettlementReason: 'ESTIMATE_PRICE_CARD_NOT_CONFIGURED',
      }),
      { text: '估价没形成：没有在用价卡', financialStall: '没有在用价卡' },
    );
  }
});

test('申报不全等客户补充：括号里逐条列任务缺口（委托级在前、指件的带件），句尾说客户补充后自动续办，不另起续办一行', () => {
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'CUSTOMER_SUPPLEMENT_PENDING',
        lastAttemptResumePath: 'CUSTOMER_SUPPLEMENT',
        lastAttemptSettlementReason: 'ESTIMATE_DECLARATION_INCOMPLETE',
        lastAttemptEstimateGapCodes: ['MEASUREMENT', 'POSTAL_ROUTE'],
        supplementGaps: [
          { group: 'PRE_ACCEPTANCE_FINANCIAL_CONTROL', parcelId: 'DPCL-2', reason: 'ESTIMATE_MEASUREMENT_MISSING' },
          { group: 'PRE_ACCEPTANCE_FINANCIAL_CONTROL', reason: 'ESTIMATE_RECIPIENT_POSTAL_MISSING' },
          // 别组的缺口不混进估价这一句。
          { group: 'REQUIRED_DOCUMENT', parcelId: 'DPCL-2', reason: 'EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL' },
        ],
      }),
    ),
    {
      text: '估价没形成：申报缺了计价要的资料（委托缺收件邮编、DPCL-2 缺重量或量纲）——客户补充后自动续办',
      financialStall: '申报缺了计价要的资料（委托缺收件邮编、DPCL-2 缺重量或量纲）——客户补充后自动续办',
    },
  );
  // 服务产品照旧接在停因上；任务缺口缺席（不该有，但读面不猜）时不带括号。
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'CUSTOMER_SUPPLEMENT_PENDING',
        lastAttemptResumePath: 'CUSTOMER_SUPPLEMENT',
        lastAttemptSettlementReason: 'ESTIMATE_DECLARATION_INCOMPLETE',
        lastAttemptServiceProduct: 'SYN-PROD-FBA-01',
      }),
    ),
    {
      text: '估价没形成：申报缺了计价要的资料（服务产品 SYN-PROD-FBA-01）——客户补充后自动续办',
      financialStall: '申报缺了计价要的资料（服务产品 SYN-PROD-FBA-01）——客户补充后自动续办',
    },
  );
  // 缺口清单与服务产品同场：产品那一括接在缺口括号之后、客户补充尾句之前。
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'CUSTOMER_SUPPLEMENT_PENDING',
        lastAttemptResumePath: 'CUSTOMER_SUPPLEMENT',
        lastAttemptSettlementReason: 'ESTIMATE_DECLARATION_INCOMPLETE',
        lastAttemptServiceProduct: 'SYN-PROD-FBA-01',
        supplementGaps: [{ group: 'PRE_ACCEPTANCE_FINANCIAL_CONTROL', reason: 'ESTIMATE_ORIGIN_POSTAL_MISSING' }],
      }),
    ),
    {
      text: '估价没形成：申报缺了计价要的资料（委托缺寄件邮编）（服务产品 SYN-PROD-FBA-01）——客户补充后自动续办',
      financialStall: '申报缺了计价要的资料（委托缺寄件邮编）（服务产品 SYN-PROD-FBA-01）——客户补充后自动续办',
    },
  );
});

test('申报不全而缺项在卡侧：写明这张卡做不了受理前估价，续办说该登什么样的售价卡并链到价格表', () => {
  const zoneStalled = running({
    lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
    lastAttemptResumePath: 'OPERATOR_REGISTRATION',
    lastAttemptSettlementReason: 'ESTIMATE_DECLARATION_INCOMPLETE',
    lastAttemptEstimateGapCodes: ['ZONE'],
  });
  deepEqual(lastAttemptOf(zoneStalled), {
    text: '估价没形成：在用的卡要直接给分区，受理前估价给不出',
    financialStall: '在用的卡要直接给分区，受理前估价给不出',
    resume: {
      path: 'OPERATOR_REGISTRATION',
      text: '登记一张带分区目录、生效期覆盖提交时点的售价卡后自动续办',
      link: { href: '#/price-card-catalog', label: '去价格表' },
    },
  });
  // 缺收件地址性质，带服务产品；两码同时在场时并说。
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptSettlementReason: 'ESTIMATE_DECLARATION_INCOMPLETE',
        lastAttemptEstimateGapCodes: ['ADDRESS_NATURE', 'MEASUREMENT'],
        lastAttemptServiceProduct: 'SYN-PROD-FBA-01',
      }),
    ),
    {
      text: '估价没形成：在用的卡要收件地址性质，受理前估价给不出（服务产品 SYN-PROD-FBA-01）',
      financialStall: '在用的卡要收件地址性质，受理前估价给不出（服务产品 SYN-PROD-FBA-01）',
      resume: {
        path: 'OPERATOR_REGISTRATION',
        text: '登记一张不按收件地址性质判价、生效期覆盖提交时点的售价卡后自动续办',
        link: { href: '#/price-card-catalog', label: '去价格表' },
      },
    },
  );
  // 两码同场：停因句与续办句都按词表顺序并说（分区在前），不随来码顺序。
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptSettlementReason: 'ESTIMATE_DECLARATION_INCOMPLETE',
        lastAttemptEstimateGapCodes: ['ADDRESS_NATURE', 'ZONE'],
      }),
    ),
    {
      text: '估价没形成：在用的卡要直接给分区、要收件地址性质，受理前估价给不出',
      financialStall: '在用的卡要直接给分区、要收件地址性质，受理前估价给不出',
      resume: {
        path: 'OPERATOR_REGISTRATION',
        text: '登记一张带分区目录、不按收件地址性质判价、生效期覆盖提交时点的售价卡后自动续办',
        link: { href: '#/price-card-catalog', label: '去价格表' },
      },
    },
  );
  // 任务已不在运行：卡侧停因照留，不再说谁来续办；resume: false 的页同样取整句。
  deepEqual(lastAttemptOf({ ...zoneStalled, state: 'COMPLETE' }), {
    text: '估价没形成：在用的卡要直接给分区，受理前估价给不出',
    financialStall: '在用的卡要直接给分区，受理前估价给不出',
  });
  deepEqual(lastAttemptOf(zoneStalled, { resume: false }), {
    text: '估价没形成：在用的卡要直接给分区，受理前估价给不出',
    financialStall: '在用的卡要直接给分区，受理前估价给不出',
  });
});

test('申报不全而缺项码与续办方都说不出细的（照旧内部重试那一路）：走底句', () => {
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_FORMED',
        lastAttemptResumePath: 'INTERNAL_RETRY',
        lastAttemptSettlementReason: 'ESTIMATE_DECLARATION_INCOMPLETE',
      }),
    ),
    {
      text: '估价没形成：申报缺了计价要的资料',
      financialStall: '申报缺了计价要的资料',
      resume: { path: 'INTERNAL_RETRY', text: '系统会自动重试' },
    },
  );
});

test('认不得的码照原样：结算细原因露出服务端交来的原码，不补前缀', () => {
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
        lastAttemptResumePath: 'OPERATOR_REGISTRATION',
        lastAttemptSettlementReason: 'ESTIMATE_SOMETHING_NEW',
      }),
    ),
    { text: '财务控制没形成：ESTIMATE_SOMETHING_NEW', financialStall: 'ESTIMATE_SOMETHING_NEW', resume: registrationWait },
  );
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'SOMETHING_NEW' })), { text: 'SOMETHING_NEW' });
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'SA-SOMETHING_NEW' })), { text: 'SA-SOMETHING_NEW', financialStall: 'SA-SOMETHING_NEW' });
  // 细原因在场而未决原因缺席（不该有的组合）：照说细原因。
  deepEqual(lastAttemptOf(running({ lastAttemptSettlementReason: 'ESTIMATE_PRICE_CARD_NOT_CONFIGURED' })), {
    text: '估价没形成：没有在用价卡',
    financialStall: '没有在用价卡',
  });
});
