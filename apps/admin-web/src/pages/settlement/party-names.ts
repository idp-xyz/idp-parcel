import { useEffect, useState } from 'react';
import { listBusinessParties, listCustomerAccounts } from '../party/api';

// 账务与清关各页把参与方编码、客户账户号转写成名称用的一问（设计原则二「说运营的话」，词典「客户账户 → 客户（显示名称）」）。
// 名称在参与方册上，这里只读那本册、不另存一份：参与方页也是这么转写的（PartyCell）。转写不到时交回 undefined，
// 由调用方显编码——读不到名字是诚实态，不补「未知客户」之类的假名。

export interface PartyNames {
  /** 参与方编码 → 名称。 */
  party: (partyId: string | undefined) => string | undefined;
  /** 客户账户号 → 持有它的客户名称。 */
  account: (accountId: string | undefined) => string | undefined;
  /** 两问都已回来（不论成败）；回来之前名称列先显编码，免得先空再跳。 */
  settled: boolean;
}

export function usePartyNames(): PartyNames {
  const [parties, setParties] = useState<Map<string, string>>(() => new Map());
  const [accounts, setAccounts] = useState<Map<string, string>>(() => new Map());
  const [settled, setSettled] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void Promise.all([listBusinessParties(), listCustomerAccounts()]).then(([partyAnswer, accountAnswer]) => {
      if (cancelled) return;
      if (partyAnswer.kind === 'outcome') {
        setParties(new Map(partyAnswer.body.parties.map((party) => [party.partyId, party.partyName])));
      }
      if (accountAnswer.kind === 'outcome') {
        setAccounts(
          new Map(
            accountAnswer.body.accounts
              .filter((account) => account.customerPartyNameKnown && account.customerPartyName)
              .map((account) => [account.accountId, account.customerPartyName as string]),
          ),
        );
      }
      setSettled(true);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return {
    party: (partyId) => (partyId ? parties.get(partyId) : undefined),
    account: (accountId) => (accountId ? accounts.get(accountId) : undefined),
    settled,
  };
}

/** 名称在前、编码在后的一格要用的两件；名称读不到时主字就是编码本身、次字缺席。 */
export function namedRef(name: string | undefined, code: string | undefined): { primary: string; secondary?: string } {
  if (!code) return { primary: '—' };
  return name ? { primary: name, secondary: code } : { primary: code };
}
