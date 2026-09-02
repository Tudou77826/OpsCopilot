import type { AIConfigRuntime, AIConfigStatus, AIConfigUpdateInput } from '../../ui';
import type { SidecarClient } from '../../core/sidecarClient';

/**
 * Sidecar 对 AI 配置卡的宿主适配。
 * shell.ai.getConfig 永不返回明文密钥（sidecar 侧脱敏）；保存时 apiKey 为空 = 保留已存密钥。
 */
export function makeSidecarAIConfigRuntime(clientGetter: () => SidecarClient | null): AIConfigRuntime {
  return {
    async status(): Promise<AIConfigStatus> {
      const client = clientGetter();
      if (!client) throw new Error('sidecar 未连接');
      return client.aiGetConfig() as unknown as Promise<AIConfigStatus>;
    },
    async save(update: AIConfigUpdateInput): Promise<AIConfigStatus> {
      const client = clientGetter();
      if (!client) throw new Error('sidecar 未连接');
      return client.aiSaveConfig(update as unknown as Record<string, unknown>) as unknown as Promise<AIConfigStatus>;
    },
  };
}
