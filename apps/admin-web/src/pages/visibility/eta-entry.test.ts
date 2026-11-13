import { equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { etaEntryEndpoint, etaEntryHint, etaEntryOutcomeLabels, etaEntryProblemNote } from './eta-entry';

test('录入预计到达的提示说清两种来源、时间范围、原件必填，并说明不是录了就对客展示', () => {
  equal(etaEntryEndpoint, '/eta-prediction-manual-entries');
  for (const word of ['parcel', 'milestone', 'CARRIER_PROVIDED', 'OPERATOR_DERIVED', 'rangeFrom', 'rangeTo', 'confidence', 'original', '不是录了就对客展示']) {
    ok(etaEntryHint.includes(word), word);
  }
  ok(etaEntryOutcomeLabels.ETA_REFRESHED.includes('前一版保留'));
  ok(etaEntryOutcomeLabels.ETA_EXISTING_SUPERSEDED.includes('接替'));
  ok(etaEntryHint.includes('必须晚于'));
  ok(etaEntryProblemNote('MALFORMED_REQUEST').includes('rangeTo'));
});
