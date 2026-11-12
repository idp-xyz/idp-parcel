import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { statusVocabulary } from '../../vocabulary/status';
import { knownSettlementStallCodes, knownShipmentStallCodes, stallReasonOf } from './stall-reasons';

// 本文件钉受理任务停因的屏上写法：今天任务上记的财务控制未决原因（FINANCIAL_CONTROL_*）与结算的 `SA-` 停因都译成中文并标明停在
// 财务控制那一步；别的未决原因照样译；认不得的码照原样，`SA-` 前缀的仍算财务控制那一步。

test('任务上今天记的财务控制两格译成中文，标明停在财务控制', () => {
  deepEqual(stallReasonOf('FINANCIAL_CONTROL_NOT_FORMED'), {
    code: 'FINANCIAL_CONTROL_NOT_FORMED',
    text: '财务控制没形成（结算没能估价或没能占用）',
    known: true,
    financialControl: true,
  });
  deepEqual(stallReasonOf('FINANCIAL_CONTROL_NOT_CONFIGURED'), {
    code: 'FINANCIAL_CONTROL_NOT_CONFIGURED',
    text: '财务控制缺一项登记，等运营登记',
    known: true,
    financialControl: true,
  });
});

test('结算的 `SA-` 停因：没有在用价卡、价卡适用冲突、输入不可得', () => {
  deepEqual(stallReasonOf('SA-ESTIMATE_PRICE_CARD_NOT_CONFIGURED'), {
    code: 'SA-ESTIMATE_PRICE_CARD_NOT_CONFIGURED',
    text: '没有在用价卡',
    known: true,
    financialControl: true,
  });
  deepEqual(stallReasonOf('SA-ESTIMATE_PRICE_CARD_CONFLICT').text, '价卡适用冲突');
  deepEqual(stallReasonOf('SA-ESTIMATE_DECLARATION_INCOMPLETE').text, '输入不可得：有包裹缺测量或缺价卡要的那一格');
  deepEqual(stallReasonOf('SA-CONTROL_AMOUNT_NOT_CONFIGURED').text, '这户没有登记受理前估价采用');
});

test('财务控制以外的未决原因照样译，不算财务控制那一步', () => {
  deepEqual(stallReasonOf('MANUAL_REVIEW_PENDING'), {
    code: 'MANUAL_REVIEW_PENDING',
    text: '等人工复核',
    known: true,
    financialControl: false,
  });
  deepEqual(stallReasonOf('REACHABILITY_AS_OF_NOT_CONFIGURED').financialControl, false);
});

test('认不得的码照原样；`SA-` 前缀的仍算财务控制那一步', () => {
  deepEqual(stallReasonOf('SOMETHING_NEW'), { code: 'SOMETHING_NEW', text: 'SOMETHING_NEW', known: false, financialControl: false });
  deepEqual(stallReasonOf('SA-SOMETHING_NEW'), { code: 'SA-SOMETHING_NEW', text: 'SA-SOMETHING_NEW', known: false, financialControl: true });
});

test('用语词典里也收了的码，停因读成同一句', () => {
  const both = [...knownShipmentStallCodes, ...knownSettlementStallCodes].filter((code) => statusVocabulary[code] !== undefined);
  deepEqual(
    both.map((code) => [code, stallReasonOf(code).text]),
    both.map((code) => [code, statusVocabulary[code]!.label]),
  );
});
