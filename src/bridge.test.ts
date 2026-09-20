import { describe, expect, it } from 'vitest';
import { waitForPluginReady, type DbxPluginApi } from './bridge';

describe('DBX plugin bridge bootstrap', () => {
  it('repeats the ready handshake when the first host init is missed', async () => {
    let resolveReady!: () => void;
    const plugin: DbxPluginApi = {
      ready: new Promise<void>(resolve => { resolveReady = resolve; }),
      context: { connectionId: 'connection-1' },
      invoke: async <T>() => undefined as T,
      onContext: () => () => {},
    };
    let handshakes = 0;

    const context = await waitForPluginReady(plugin, {
      delay: async () => {},
      signalReady: () => {
        handshakes += 1;
        resolveReady();
      },
      retries: 2,
    });

    expect(context).toEqual({ connectionId: 'connection-1' });
    expect(handshakes).toBe(1);
  });

  it('does not repeat the handshake when init already arrived', async () => {
    const plugin: DbxPluginApi = {
      ready: Promise.resolve(),
      context: { connectionId: 'connection-1' },
      invoke: async <T>() => undefined as T,
      onContext: () => () => {},
    };
    let handshakes = 0;

    await waitForPluginReady(plugin, {
      delay: async () => {},
      signalReady: () => { handshakes += 1; },
    });

    expect(handshakes).toBe(0);
  });

  it('waits for a connection context when ready resolved with an empty context', async () => {
    let contextListener: ((context: { connectionId?: string }) => void) | undefined;
    const plugin: DbxPluginApi = {
      ready: Promise.resolve(),
      context: {},
      invoke: async <T>() => undefined as T,
      onContext: (listener) => {
        contextListener = listener;
        return () => { contextListener = undefined; };
      },
    };
    let handshakes = 0;

    const context = await waitForPluginReady(plugin, {
      delay: async () => {},
      signalReady: () => {
        handshakes += 1;
        plugin.context = { connectionId: 'connection-late' };
        contextListener?.(plugin.context);
      },
      retries: 2,
    });

    expect(context).toEqual({ connectionId: 'connection-late' });
    expect(handshakes).toBe(1);
  });
});
