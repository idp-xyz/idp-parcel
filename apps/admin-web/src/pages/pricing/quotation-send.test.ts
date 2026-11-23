import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { quotationSendPayload } from './quotation-send';

test('发出载荷不带发出人、时刻或报价号，缺终点就缺席', () => {
  const payload = quotationSendPayload({
    lane: 'APPROVED_DRAFT',
    planId: 'PLAN',
    planVersion: 'v1',
    contentDigest: 'digest',
    counterpartyPartyId: 'party-1',
    legalEntityId: 'le-1',
    validFrom: '2026-11-23T08:00',
    validUntil: '',
    terms: ' 条款 ',
  });
  equal(payload.validUntil, undefined);
  equal(payload.validFrom, '2026-11-23T08:00:00Z');
  equal('sender' in payload, false);
  equal('sentAt' in payload, false);
  equal('quotationId' in payload, false);
  equal(payload.terms, ' 条款 ');
});

test('已登记价卡走价卡路', () => {
  const payload = quotationSendPayload({
    lane: 'REGISTERED_PRICE_CARD',
    planId: 'SYN-PLAN-CN-SG-01',
    planVersion: 'v1',
    contentDigest: 'abc',
    counterpartyPartyId: 'SYN-PARTY-SHIPPER-01',
    legalEntityId: 'SYN-LE-01',
    validFrom: '2026-11-23T00:00',
    validUntil: '2027-11-23T00:00',
    terms: '',
  });
  equal(payload.lane, 'REGISTERED_PRICE_CARD');
  ok(payload.validFrom);
  ok(payload.validUntil);
});
