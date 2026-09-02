import type {
  SessionManagerRuntime,
  QuickCommandHost,
  QuickCommandStorageAdapter,
  QuickCommand,
  SessionNode,
  ConnectionConfig,
} from '../../ui';
import { SidecarClient, SavedSession } from '../../core/sidecarClient';

// Sidecar 的 SavedSession 与共享 SessionNode 形态一致（id/name/type/children/config）。
function toSessionNode(s: SavedSession): SessionNode {
  return {
    id: s.id,
    name: s.name,
    type: (s.type === 'folder' ? 'folder' : 'session') as SessionNode['type'],
    children: s.children ? s.children.map(toSessionNode) : undefined,
    config: s.config as ConnectionConfig | undefined,
  };
}

/** 快捷命令存储适配器：单条意图化操作，走 sidecar RPC。 */
class SidecarQuickCommandStorage implements QuickCommandStorageAdapter {
  constructor(private client: SidecarClient) {}
  async load(): Promise<QuickCommand[]> {
    return (await this.client.quickcmdsList()).commands ?? [];
  }
  add(cmd: QuickCommand): void {
    void this.client.quickcmdsSave(cmd);
  }
  update(id: string, updates: Partial<QuickCommand>): void {
    void this.client.quickcmdsSave({ id, ...updates } as QuickCommand);
  }
  remove(id: string): void {
    void this.client.quickcmdsDelete(id);
  }
  reorder(ids: string[]): void {
    void this.client.quickcmdsReorder(ids);
  }
}

export interface SidecarConfigRuntime {
  sessionRuntime: SessionManagerRuntime;
  quickCommandHost: (execute: (content: string) => void) => QuickCommandHost;
}

/** 构造 sidecar 会话树 + 快捷命令宿主适配器。 */
export function makeSidecarConfigRuntime(client: SidecarClient): SidecarConfigRuntime {
  const sessionRuntime: SessionManagerRuntime = {
    async listSessions() {
      const { sessions } = await client.listConfigs();
      return (sessions ?? []).map(toSessionNode);
    },
    async deleteSession(id) {
      await client.deleteConfig(id);
    },
    async renameSession(id, newName) {
      await client.renameConfig(id, newName);
    },
    async updateSession(id, config, group) {
      await client.updateConfig(id, config as unknown as Record<string, unknown>, group);
    },
    async createFolder(name) {
      await client.createFolder(name);
    },
  };

  return {
    sessionRuntime,
    quickCommandHost: (execute: (content: string) => void): QuickCommandHost => ({
      execute,
      storage: new SidecarQuickCommandStorage(client),
      // Sidecar 不依赖宿主事件总线。订阅期间轻量轮询列表并只在内容变化时推送，
      // 保证多个浏览器窗口不会长期持有旧快照。
      onExternalChange: (handler) => {
        let stopped = false;
        let polling = false;
        let initialized = false;
        let signature = '';
        const poll = async () => {
          if (stopped || polling) return;
          polling = true;
          try {
            const commands = (await client.quickcmdsList()).commands ?? [];
            const nextSignature = JSON.stringify(commands);
            if (initialized && nextSignature !== signature) handler(commands);
            signature = nextSignature;
            initialized = true;
          } catch {
            // 控制面短暂断开时保留最后快照；SidecarClient 会负责连接状态提示。
          } finally {
            polling = false;
          }
        };
        void poll();
        const timer = window.setInterval(() => void poll(), 1000);
        return () => {
          stopped = true;
          window.clearInterval(timer);
        };
      },
    }),
  };
}
