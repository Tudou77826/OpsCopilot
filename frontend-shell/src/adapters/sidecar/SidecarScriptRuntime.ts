import type { ScriptRuntime, ScriptData, ScriptRecordingStatus } from '../../ui';
import type { SidecarClient } from '../../core/sidecarClient';

/**
 * Sidecar 对共享脚本组件的宿主适配。
 * shell.script.* RPC 复用 pkg/script 引擎，返回对象即脚本 JSON
 * （与 Wails 绑定形状一致，直接透传）。无原生保存对话框 → 不提供 exportScript。
 */
export function makeSidecarScriptRuntime(client: SidecarClient): ScriptRuntime {
  return {
    list: () => client.scriptList() as unknown as Promise<ScriptData[]>,
    load: (id) => client.scriptLoad(id) as unknown as Promise<ScriptData>,
    update: (script) => client.scriptUpdate(script as unknown as Record<string, unknown>),
    remove: (id) => client.scriptDelete(id),
    create: async (name, description) => client.scriptCreate(name, description) as unknown as Promise<ScriptData>,
    replay: (id, sessionId) => client.scriptReplay(id, sessionId),
    replayWithVars: (id, sessionId, values) => client.scriptReplayVars(id, sessionId, values),
    startRecording: async (name, description, sessionId) =>
      client.scriptStartRecording(name, description, sessionId) as unknown as Promise<ScriptData>,
    stopRecording: () => client.scriptStopRecording() as unknown as Promise<ScriptData>,
    recordingStatus: () => client.scriptStatus() as unknown as Promise<ScriptRecordingStatus>,
  };
}
