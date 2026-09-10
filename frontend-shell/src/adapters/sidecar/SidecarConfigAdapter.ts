import type {
  SessionManagerRuntime,
  QuickCommandHost,
  QuickCommandStorageAdapter,
  QuickCommand,
  SessionNode,
  ConnectionConfig,
} from '../../ui';
import { SidecarClient, SavedNode } from '../../core/sidecarClient';

// Sidecar 的 SavedNode 与共享 SessionNode 形态一致（id/name/type/children/config）。
function toSessionNode(node: SavedNode): SessionNode {
  return {
    id: node.id,
    name: node.name,
    type: (node.type === 'folder' ? 'folder' : 'session') as SessionNode['type'],
    children: node.children ? node.children.map(toSessionNode) : undefined,
    config: node.config as ConnectionConfig | undefined,
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
  // 实现全部会话树方法（含 duplicateConnection），两端能力保持一致。
  // 不实现 Xshell 导入：那是桌面端专有能力，缺少这些可选方法时共享 UI 会自动
  // 隐藏导入入口——这正是 ports 里把它们声明为可选的目的。
  const sessionRuntime: SessionManagerRuntime = {
    async listTree() {
      const { nodes } = await client.listConfigs();
      return (nodes ?? []).map(toSessionNode);
    },
    async createFolder(name, parentId) {
      await client.createFolder(name, parentId);
    },
    async createConnection(config, parentId) {
      await client.createConnection(config as unknown as Record<string, unknown>, parentId);
    },
    async renameNode(id, newName) {
      await client.renameNode(id, newName);
    },
    async updateConnection(id, config) {
      await client.updateConnection(id, config as unknown as Record<string, unknown>);
    },
    async moveNode(id, newParentId, index) {
      await client.moveNode(id, newParentId, index);
    },
    async deleteNode(id) {
      await client.deleteNode(id);
    },
    async reorderNodes(parentId, orderedIds) {
      await client.reorderNodes(parentId, orderedIds);
    },
    async duplicateConnection(id) {
      await client.duplicateConnection(id);
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
