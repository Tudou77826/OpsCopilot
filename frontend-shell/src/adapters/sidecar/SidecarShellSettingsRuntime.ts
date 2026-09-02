import type { ShellSettings, ShellSettingsRuntime } from '../../ui';
import type { SidecarClient } from '../../core/sidecarClient';

/**
 * Sidecar 对共享 Shell 设置弹窗的宿主适配。
 * shell.settings.get/save 直接返回/接收 ShellSettings 同构 JSON（数据目录落盘，重启保持）。
 * clientGetter 在连接建立前后均安全（未连接时报错）。
 */
export function makeSidecarShellSettingsRuntime(clientGetter: () => SidecarClient | null): ShellSettingsRuntime {
  return {
    async load(): Promise<ShellSettings> {
      const client = clientGetter();
      if (!client) throw new Error('sidecar 未连接');
      return client.settingsGet() as unknown as Promise<ShellSettings>;
    },
    async save(next: ShellSettings): Promise<void> {
      const client = clientGetter();
      if (!client) throw new Error('sidecar 未连接');
      await client.settingsSave(next as unknown as Record<string, unknown>);
    },
  };
}
