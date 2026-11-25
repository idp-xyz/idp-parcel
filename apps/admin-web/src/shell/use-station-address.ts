import { useSyncExternalStore } from 'react';
import { parseStationAddress } from './station-address';

// 此刻的地址是不是作业台的：是就交回整个 hash，不是交回 null。
//
// 快照取 hash 字符串（或 null）而不是解析结果：useSyncExternalStore 靠 Object.is 比快照，字符串按值比，所以外壳里
// 地址来回变（null → null）不会让 App 重渲一次，也就不会白白带着 Layout 重渲；只有进出作业台、或在作业台的地址之间换
// （换了工位、换了波次）才重渲。解析（URLSearchParams）每次都新造对象，不能当快照。

function subscribe(onChange: () => void): () => void {
  window.addEventListener('hashchange', onChange);
  return () => window.removeEventListener('hashchange', onChange);
}

function snapshot(): string | null {
  const hash = window.location.hash;
  return parseStationAddress(hash) === null ? null : hash;
}

export function useStationHash(): string | null {
  return useSyncExternalStore(subscribe, snapshot, () => null);
}
