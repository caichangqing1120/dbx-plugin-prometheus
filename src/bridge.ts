export type Context = { connectionId?: string };
export type DbxPluginApi = {
  ready: Promise<unknown>;
  context: Context;
  invoke<T>(method: string, params: Record<string, unknown>, options?: { timeoutMs: number }): Promise<T>;
  onContext(fn: (context: Context) => void): () => void;
};
declare global {
  interface Window {
    dbxPlugin?: DbxPluginApi;
  }
}

type ReadyOptions = {
  delay?: (milliseconds: number) => Promise<void>;
  signalReady?: () => void;
  retryIntervalMs?: number;
  retries?: number;
};

const delay = (milliseconds: number) => new Promise<void>(resolve => setTimeout(resolve, milliseconds));

export async function waitForPluginReady(plugin: DbxPluginApi, options: ReadyOptions = {}): Promise<Context> {
  const wait = options.delay || delay;
  const signalReady = options.signalReady || (() => {
    window.parent.postMessage({ source: 'dbx-plugin', version: 1, type: 'ready' }, '*');
  });
  const retryIntervalMs = options.retryIntervalMs ?? 500;
  const retries = options.retries ?? 30;
  let initialized = false;
  let latestContext = plugin.context;
  let resolveConnected!: (context: Context) => void;
  const connected = new Promise<Context>(resolve => { resolveConnected = resolve; });
  const completeIfConnected = () => {
    if (initialized && latestContext.connectionId) resolveConnected(latestContext);
  };
  const unsubscribe = plugin.onContext(context => {
    latestContext = context;
    completeIfConnected();
  });
  void plugin.ready.then(() => {
    initialized = true;
    latestContext = plugin.context;
    completeIfConnected();
  });

  try {
    for (let attempt = 0; attempt <= retries; attempt += 1) {
      const context = await Promise.race([connected, wait(retryIntervalMs).then(() => undefined)]);
      if (context?.connectionId) return context;
      latestContext = plugin.context;
      if (initialized && latestContext.connectionId) return latestContext;
      if (attempt < retries) signalReady();
    }
  } finally {
    unsubscribe();
  }
  throw new Error('DBX 插件桥接初始化超时，请关闭工作台后重新打开连接');
}

export async function invoke<T>(connectionId: string, method: string, params: Record<string, unknown> = {}): Promise<T> {
  if (!window.dbxPlugin || !connectionId) throw new Error('请从 DBX 连接打开工作台');
  return window.dbxPlugin.invoke<T>(method, { ...params, connectionId }, { timeoutMs: 25000 });
}
