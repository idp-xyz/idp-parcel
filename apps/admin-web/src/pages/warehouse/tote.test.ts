import { deepEqual, equal } from 'node:assert/strict';
import { test } from 'node:test';
import { outboundWaveActionSpecs } from './actions';
import { buildGuidedPayload } from './guided-forms';
import { outboundWaveGuidedForms } from './guided-schemas';
import { canRetireTote, liveToteStates, toteStateLabel } from './tote';

// Covers: 三种状态各有中文，陌生的词原样示出；只数在用与空着两队。
test('周转箱的状态词与要数的两队', () => {
  equal(toteStateLabel('FREE'), '空着');
  equal(toteStateLabel('IN_USE'), '在用');
  equal(toteStateLabel('RETIRED'), '已停用');
  equal(toteStateLabel('SOMETHING_NEW'), 'SOMETHING_NEW');
  deepEqual([...liveToteStates], ['IN_USE', 'FREE']);
});

// Covers: 只有空着的箱摆「停用」。
test('只有空着的箱能停用', () => {
  equal(canRetireTote('FREE'), true);
  equal(canRetireTote('IN_USE'), false);
  equal(canRetireTote('RETIRED'), false);
});

// Covers: 登记一批箱号拼成数组，停用点名一个箱；两个动作都走主数据登记的授予。
test('周转箱登记与停用的报文', () => {
  const registered = buildGuidedPayload(outboundWaveGuidedForms['tote-register'], {
    warehouse: 'SYN-WH-01',
    toteCodes: 'SYN-TOTE-01, SYN-TOTE-02\nSYN-TOTE-03',
  });
  equal(registered.ok, true);
  if (registered.ok) {
    deepEqual(registered.payload, { warehouse: 'SYN-WH-01', toteCodes: ['SYN-TOTE-01', 'SYN-TOTE-02', 'SYN-TOTE-03'] });
  }
  equal(buildGuidedPayload(outboundWaveGuidedForms['tote-register'], { warehouse: 'SYN-WH-01', toteCodes: '' }).ok, false);
  const retired = buildGuidedPayload(outboundWaveGuidedForms['tote-retire'], { warehouse: 'SYN-WH-01', toteCode: 'SYN-TOTE-01' });
  equal(retired.ok, true);
  if (retired.ok) deepEqual(retired.payload, { warehouse: 'SYN-WH-01', toteCode: 'SYN-TOTE-01' });
  const register = outboundWaveActionSpecs.find((spec) => spec.id === 'tote-register');
  const retire = outboundWaveActionSpecs.find((spec) => spec.id === 'tote-retire');
  equal(register?.endpoint, '/warehouse-fulfillment-tote-registrations');
  equal(retire?.endpoint, '/warehouse-fulfillment-tote-retirements');
  equal(register?.grant, 'registry');
  equal(retire?.grant, 'registry');
  // 由人放开动的是一张单在拣货里的归属，走放波次那一种运营决定，不走主数据登记。
  const release = outboundWaveActionSpecs.find((spec) => spec.id === 'tote-release');
  equal(release?.endpoint, '/warehouse-fulfillment-tote-releases');
  equal(release?.grant, 'WAVE_RELEASE');
  const released = buildGuidedPayload(outboundWaveGuidedForms['tote-release'], { warehouse: 'SYN-WH-01', toteCode: 'SYN-TOTE-01' });
  equal(released.ok, true);
  if (released.ok) deepEqual(released.payload, { warehouse: 'SYN-WH-01', toteCode: 'SYN-TOTE-01' });
});
