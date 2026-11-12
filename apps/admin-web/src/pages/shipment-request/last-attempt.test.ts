import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { AcceptanceTaskRecord } from './api';
import { lastAttemptOf } from './last-attempt';

// 本文件钉运单详情上「最近处理结果」与「谁来续办」的写法（ADR-0380 决定五）：带与不带结算细原因、带与不带服务产品、续办方各词
// （等运营登记分价卡一侧与别处、内部重试、等客户补充照旧、复核与授权处置）、续办方缺席或认不得、任务已不在运行、认不得的码。

const running = (extra: Partial<AcceptanceTaskRecord>): AcceptanceTaskRecord => ({ state: 'RUNNING', ...extra });

const priceCardWait = {
  path: 'OPERATOR_REGISTRATION',
  text: '登记一张生效期覆盖提交时点的价卡后自动续办',
  link: { href: '#/price-card-catalog', label: '去价格表' },
};

const registrationWait = { path: 'OPERATOR_REGISTRATION', text: '运营登记后自动续办' };

test('没有处理尝试：答 null', () => {
  equal(lastAttemptOf({ state: 'RUNNING' }), null);
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
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'REJECTION_AUTHORITY_RULES_NOT_CONFIGURED', lastAttemptResumePath: 'OPERATOR_REGISTRATION' })), {
    text: '拒绝授权规则未配置，等运营登记',
    resume: registrationWait,
  });
});

test('不带结算细原因：停因照票 21 的译法；带服务产品时接在后面', () => {
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'FINANCIAL_CONTROL_NOT_CONFIGURED', lastAttemptResumePath: 'OPERATOR_REGISTRATION' })), {
    text: '财务控制缺一项登记，等运营登记',
    financialStall: '财务控制缺一项登记，等运营登记',
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
      text: '财务控制缺一项登记，等运营登记（服务产品 SYN-PROD-01）',
      financialStall: '财务控制缺一项登记，等运营登记（服务产品 SYN-PROD-01）',
      resume: registrationWait,
    },
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

test('等客户补充照旧：不另起续办那一句', () => {
  deepEqual(
    lastAttemptOf(
      running({
        lastAttemptReason: 'CUSTOMER_SUPPLEMENT_PENDING',
        lastAttemptResumePath: 'CUSTOMER_SUPPLEMENT',
        lastAttemptServiceProduct: 'SYN-PROD-FBA-01',
      }),
    ),
    { text: '等客户补充（服务产品 SYN-PROD-FBA-01）' },
  );
});

test('等人工复核与等授权处置：说由哪个角色动手', () => {
  deepEqual(lastAttemptOf(running({ lastAttemptReason: 'MANUAL_REVIEW_PENDING', lastAttemptResumePath: 'MANUAL_REVIEW' })), {
    text: '等人工复核',
    resume: { path: 'MANUAL_REVIEW', text: '复核角色记下复核后续办' },
  });
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
