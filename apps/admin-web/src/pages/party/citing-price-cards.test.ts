import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { citingAuthorizationPath, citingPriceCardPath } from './citing-price-cards';

test('引用价卡的两条读口按授权与价格政策精确筛选', () => {
  equal(
    citingPriceCardPath('SYN-AUTH-1', 'v1'),
    '/pricing-price-cards?authorizationId=SYN-AUTH-1&authorizationVersion=v1',
  );
  equal(
    citingAuthorizationPath('SYN-PRICE-1'),
    '/commercial-policies?kind=AUTHORIZATION_RULE&pricePolicy=SYN-PRICE-1',
  );
});
