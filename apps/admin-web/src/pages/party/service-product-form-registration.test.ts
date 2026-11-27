import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  emptyServiceProductFormDraft,
  readServiceProductFormSnapshot,
  serviceProductFormGapSummary,
  serviceProductFormGaps,
  serviceProductFormPayload,
} from './service-product-form-registration';

test('空表不送，空格按页面顺序报', () => {
  const draft = emptyServiceProductFormDraft();
  equal(serviceProductFormPayload(draft), null);
  deepEqual(
    serviceProductFormGaps(draft).map((gap) => gap.label),
    ['产品', '版本', '适用范围', '形态'],
  );
  equal(serviceProductFormGapSummary(serviceProductFormGaps(draft)), '还有 4 项要填：产品、版本、适用范围、形态');
});

test('填齐才组载荷，形态不默认', () => {
  const draft = { productId: ' SYN-PROD ', version: ' v1 ', scope: ' SYN-SCOPE-01 ', form: 'NETWORK_SERVICE' as const };
  deepEqual(serviceProductFormPayload(draft), {
    scope: 'SYN-SCOPE-01',
    forms: [{ productId: 'SYN-PROD', version: 'v1', form: 'NETWORK_SERVICE' }],
  });
  equal(serviceProductFormPayload({ ...draft, form: '' }), null);
});

test('粘贴的快照：在线口不收 tenantId，形态只认两个词', () => {
  const ok = readServiceProductFormSnapshot(
    '{"scope":"SYN-SCOPE-01","forms":[{"productId":"SYN-PROD","version":"v1","form":"LABEL_CHANNEL_SERVICE"}]}',
  );
  deepEqual(ok, {
    ok: true,
    snapshot: {
      scope: 'SYN-SCOPE-01',
      forms: [{ productId: 'SYN-PROD', version: 'v1', form: 'LABEL_CHANNEL_SERVICE' }],
    },
  });
  equal(readServiceProductFormSnapshot('not json').ok, false);
  equal(readServiceProductFormSnapshot('{"tenantId":"IDP-DEMO","scope":"S","forms":[{"productId":"P","version":"v1","form":"NETWORK_SERVICE"}]}').ok, false);
  equal(readServiceProductFormSnapshot('{"scope":"S","forms":[{"productId":"P","version":"v1","form":"NOT_A_FORM"}]}').ok, false);
  equal(readServiceProductFormSnapshot('{"scope":"S","mappings":[]}').ok, false);
});
