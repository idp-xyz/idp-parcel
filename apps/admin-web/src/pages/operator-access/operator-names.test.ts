import { deepEqual, equal, match, notEqual } from 'node:assert/strict';
import { test } from 'node:test';
import {
  maxOperatorNameLength,
  operatorLabel,
  operatorNameOutcomeNote,
  operatorNamePayload,
  operatorNameProblem,
  operatorNamesOf,
  operatorSubjectOf,
} from './operator-names';

const issuer = 'https://id.syn.example/dex';

test('登了姓名的示姓名，没登的示 sub，没有人答 null', () => {
  const names = operatorNamesOf([{ operator: `${issuer}#SYN-OPERATOR-01`, name: '合成甲' }]);
  equal(operatorLabel(`${issuer}#SYN-OPERATOR-01`, names), '合成甲');
  equal(operatorLabel(`${issuer}#SYN-OPERATOR-02`, names), 'SYN-OPERATOR-02');
  equal(operatorLabel(`${issuer}#SYN-OPERATOR-01`), 'SYN-OPERATOR-01');
  equal(operatorLabel(undefined, names), null);
  equal(operatorLabel('', names), null);
});

test('姓名按整个标识对，不按 sub 对：别的发行方下同一个 sub 不借用这个姓名', () => {
  const names = operatorNamesOf([{ operator: `${issuer}#SYN-OPERATOR-01`, name: '合成甲' }]);
  equal(operatorLabel('https://other.syn.example#SYN-OPERATOR-01', names), 'SYN-OPERATOR-01');
});

test('sub 那一截：没有井号、井号在末尾的原样摆', () => {
  equal(operatorSubjectOf(`${issuer}#SYN-OPERATOR-01`), 'SYN-OPERATOR-01');
  equal(operatorSubjectOf('SYN-OPERATOR-01'), 'SYN-OPERATOR-01');
  equal(operatorSubjectOf('SYN-OPERATOR-01#'), 'SYN-OPERATOR-01#');
});

test('报文去掉首尾空白；空了不带 name', () => {
  deepEqual(operatorNamePayload(issuer, 'SYN-OPERATOR-01', '  合成甲 '), { issuer, subject: 'SYN-OPERATOR-01', name: '合成甲' });
  deepEqual(operatorNamePayload(issuer, 'SYN-OPERATOR-01', '   '), { issuer, subject: 'SYN-OPERATOR-01' });
});

test('姓名按字数算上限，空算对', () => {
  equal(operatorNameProblem(''), null);
  equal(operatorNameProblem('甲'.repeat(maxOperatorNameLength)), null);
  equal(operatorNameProblem(` ${'甲'.repeat(maxOperatorNameLength)} `), null);
  notEqual(operatorNameProblem('甲'.repeat(maxOperatorNameLength + 1)), null);
});

test('答复：登上与删掉算办成，查无与不认识的不算', () => {
  equal(operatorNameOutcomeNote('OPERATOR_NAME_RECORDED').done, true);
  equal(operatorNameOutcomeNote('OPERATOR_NAME_CLEARED').done, true);
  equal(operatorNameOutcomeNote('OPERATOR_NOT_FOUND').done, false);
  const unknown = operatorNameOutcomeNote('SOMETHING_NEW');
  equal(unknown.done, false);
  match(unknown.text, /SOMETHING_NEW/);
});
