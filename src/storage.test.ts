import { describe, expect, it } from 'vitest';
import { readStorage, writeStorage } from './storage';

function blockedStorage(): Storage {
  return {
    get length(): number { throw new DOMException('blocked', 'SecurityError'); },
    clear() { throw new DOMException('blocked', 'SecurityError'); },
    getItem() { throw new DOMException('blocked', 'SecurityError'); },
    key() { throw new DOMException('blocked', 'SecurityError'); },
    removeItem() { throw new DOMException('blocked', 'SecurityError'); },
    setItem() { throw new DOMException('blocked', 'SecurityError'); },
  };
}

describe('sandbox-safe storage', () => {
  it('falls back when DBX sandbox blocks localStorage reads', () => {
    expect(readStorage(blockedStorage(), 'theme', 'system')).toBe('system');
  });

  it('ignores writes when DBX sandbox blocks localStorage', () => {
    expect(() => writeStorage(blockedStorage(), 'theme', 'dark')).not.toThrow();
  });
});
