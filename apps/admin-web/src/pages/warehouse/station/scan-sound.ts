// 扫码的提示音：扫对一声高、扫错两声低，可以关，开关记在本机。
//
// 响什么、什么时候响拆成纯数据（toneScript）与一个薄薄的播放器（playCue）：前者 node:test 直接钉，后者只有浏览器里能验。
// 播放器用 Web Audio 现合成，不带音频文件。浏览器不让响（页面上还没有过用户手势，音频上下文起不来）时安静地不响，
// 不报错、不排队——挂起的上下文上排的音，等它哪天被放行会一股脑响出来，所以不是 running 就先 resume，resume 成了才排。

import type { StorageLike } from './device-binding';

export type ScanCue = 'ok' | 'wrong';

export interface Tone {
  /** Hz */
  frequency: number;
  /** 距这一组音开始的毫秒 */
  startMs: number;
  durationMs: number;
}

/** 扫对：一声短的高音。扫错：两声低音，中间留一口气，耳朵里分得出「扫对了」还是「扫错了」，不用看屏。 */
export function toneScript(cue: ScanCue): readonly Tone[] {
  if (cue === 'ok') return [{ frequency: 1760, startMs: 0, durationMs: 90 }];
  return [
    { frequency: 330, startMs: 0, durationMs: 150 },
    { frequency: 330, startMs: 230, durationMs: 150 },
  ];
}

export const SOUND_KEY = 'idp.parcel.station.sound';

/** 默认开；只有明确记过「关」才是关，记坏了的值不把声音悄悄关掉。 */
export function readSoundOn(store: StorageLike | null): boolean {
  try {
    return store?.getItem(SOUND_KEY) !== 'off';
  } catch {
    return true;
  }
}

export function writeSoundOn(store: StorageLike | null, on: boolean): void {
  try {
    store?.setItem(SOUND_KEY, on ? 'on' : 'off');
  } catch {
    // 禁用存储时忽略：这次会话里照开关，下次进来回到默认
  }
}

type AudioContextCtor = typeof AudioContext;

let context: AudioContext | null = null;

function audioContext(): AudioContext | null {
  if (context !== null) return context;
  const host = window as unknown as { AudioContext?: AudioContextCtor; webkitAudioContext?: AudioContextCtor };
  const Ctor = host.AudioContext ?? host.webkitAudioContext;
  if (Ctor === undefined) return null;
  context = new Ctor();
  return context;
}

function schedule(audio: AudioContext, tones: readonly Tone[]): void {
  const origin = audio.currentTime + 0.01;
  for (const tone of tones) {
    const oscillator = audio.createOscillator();
    const gain = audio.createGain();
    const start = origin + tone.startMs / 1000;
    const end = start + tone.durationMs / 1000;
    oscillator.type = 'square';
    oscillator.frequency.value = tone.frequency;
    // 起止各留几毫秒的斜坡，免得方波在开头结尾「啪」一下
    gain.gain.setValueAtTime(0, start);
    gain.gain.linearRampToValueAtTime(0.12, start + 0.005);
    gain.gain.setValueAtTime(0.12, Math.max(start + 0.005, end - 0.01));
    gain.gain.linearRampToValueAtTime(0, end);
    oscillator.connect(gain);
    gain.connect(audio.destination);
    oscillator.start(start);
    oscillator.stop(end + 0.02);
  }
}

/** 响一下。任何一步出错（没有 Web Audio、上下文起不来、没有用户手势）都安静地不响。 */
export function playCue(cue: ScanCue): void {
  try {
    const audio = audioContext();
    if (audio === null) return;
    const tones = toneScript(cue);
    if (audio.state === 'running') {
      schedule(audio, tones);
      return;
    }
    audio.resume().then(
      () => schedule(audio, tones),
      () => undefined,
    );
  } catch {
    // 浏览器不让响：不响就是了
  }
}
