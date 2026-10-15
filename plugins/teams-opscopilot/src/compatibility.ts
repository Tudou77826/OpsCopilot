import { OpsError } from './errors.js'

export const pluginMarker = 'OpsCopilot.TeamsPlugin.Protocol1.SharedConfig1'
export const compatibility = {
  hostApi: '6', protocol: 1, apiRevision: { min: 2, max: 2 }, sharedConfig: 1, installationLifecycle: 1,
  requiredCapabilities: ['connections.v1', 'terminal.v1', 'files.v1', 'scripts.v1', 'quickCommands.v1', 'settings.v1', 'companion.hatch.v1'],
} as const

export function assertCompatible(info: any): asserts info is { version: string } {
  if (!info || info.product !== 'OpsCopilot' || info.marker !== pluginMarker ||
      info.protocol !== compatibility.protocol || info.sharedConfig !== compatibility.sharedConfig ||
      info.installationLifecycle !== compatibility.installationLifecycle ||
      !Number.isInteger(info.apiRevision) || info.apiRevision < compatibility.apiRevision.min || info.apiRevision > compatibility.apiRevision.max ||
      typeof info.version !== 'string' || !info.version.trim() ||
      !Array.isArray(info.capabilities) || !compatibility.requiredCapabilities.every(capability => info.capabilities.includes(capability))) {
    throw new OpsError('PROTOCOL_MISMATCH', '本地 Ops 与插件不兼容：需要 API v1/revision 2、共享配置 v1、升级保护 v1 及伙伴开蛋能力；请同时更新本地 Ops 和插件，不会替换或重置配置')
  }
}
