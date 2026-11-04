import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import {
  isPartyContractsRegisterView,
  partyContractsListOf,
  partyContractsViewHash,
  partyContractsViewOf,
} from './party-contracts-address';

// Covers: 缺省与认不得的值都是合同目录；两本列表与两本登记视图各认各的；离开登记视图回对应的列表；写回只改这一个键。
test('客户与合同页的视图住在查询串上', () => {
  equal(partyContractsViewOf('#/party-contracts'), 'contracts');
  equal(partyContractsViewOf('#/party-contracts?tab=accounts'), 'accounts');
  equal(partyContractsViewOf('#/party-contracts?q=SYN&tab=publish-contract'), 'publish-contract');
  equal(partyContractsViewOf('#/party-contracts?tab=nope'), 'contracts');
  equal(isPartyContractsRegisterView('register'), true);
  equal(isPartyContractsRegisterView('accounts'), false);
  equal(partyContractsListOf('publish-contract'), 'contracts');
  equal(partyContractsListOf('register'), 'accounts');
  equal(partyContractsViewHash('#/party-contracts?q=SYN', 'accounts'), '#/party-contracts?q=SYN&tab=accounts');
  equal(partyContractsViewHash('#/party-contracts?q=SYN&tab=register', 'contracts'), '#/party-contracts?q=SYN');
});
