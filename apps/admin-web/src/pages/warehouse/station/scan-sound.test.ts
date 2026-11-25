import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { readSoundOn, SOUND_KEY, toneScript, writeSoundOn } from './scan-sound';
import type { StorageLike } from './device-binding';

function memory(initial: Record<string, string> = {}): StorageLike {
  const data = new Map(Object.entries(initial));
  return {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      data.set(key, value);
    },
  };
}

test('扫对一声，扫错两声', () => {
  equal(toneScript('ok').length, 1);
  equal(toneScript('wrong').length, 2);
});

test('扫对的音比扫错的高，扫错的两声一样高、不重叠、中间留口气', () => {
  const [good] = toneScript('ok');
  const [first, second] = toneScript('wrong');
  ok(good.frequency > first.frequency * 2, '一声高、两声低，要分得出来');
  equal(first.frequency, second.frequency);
  ok(second.startMs > first.startMs + first.durationMs, '两声之间要有间隔');
});

test('每个音都从 0 起排、有正的时长', () => {
  for (const cue of ['ok', 'wrong'] as const) {
    const tones = toneScript(cue);
    equal(tones[0].startMs, 0, cue);
    for (const tone of tones) {
      ok(tone.durationMs > 0 && tone.durationMs < 500, `${cue}: 短响，不拖`);
      ok(tone.frequency > 0, cue);
    }
  }
});

test('声音默认开；记过「关」才是关；记坏了的值不把声音关掉', () => {
  equal(readSoundOn(memory()), true);
  equal(readSoundOn(memory({ [SOUND_KEY]: 'off' })), false);
  equal(readSoundOn(memory({ [SOUND_KEY]: 'on' })), true);
  equal(readSoundOn(memory({ [SOUND_KEY]: 'garbage' })), true);
  equal(readSoundOn(null), true);
});

test('开关记在本机，读得回', () => {
  const store = memory();
  writeSoundOn(store, false);
  equal(readSoundOn(store), false);
  writeSoundOn(store, true);
  equal(readSoundOn(store), true);
});

test('存储读不了、写不了时不报错', () => {
  const broken: StorageLike = {
    getItem: () => {
      throw new Error('存储被禁用');
    },
    setItem: () => {
      throw new Error('存储被禁用');
    },
  };
  equal(readSoundOn(broken), true);
  writeSoundOn(broken, false);
  writeSoundOn(null, false);
});
