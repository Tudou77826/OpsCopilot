import type { ScriptRuntime, ScriptData } from '@opscopilot/shell-terminal/ui';

type WailsWindow = Window & {
    go?: { main?: { App?: Record<string, (...args: any[]) => Promise<any>> } };
};

const app = () => (window as WailsWindow).go?.main?.App;

/** Wails 对共享脚本组件的宿主适配（window.go.main.App 直通）。 */
export const wailsScriptRuntime: ScriptRuntime = {
    list: () => app()!.GetScriptList(),
    load: (scriptId) => app()!.LoadScript(scriptId),
    update: (script: ScriptData) => app()!.UpdateScript(script),
    remove: (scriptId) => app()!.DeleteScript(scriptId),
    create: (name, description) => app()!.CreateScript(name, description),
    replay: (scriptId, sessionId) => app()!.ReplayScript(scriptId, sessionId),
    replayWithVars: (scriptId, sessionId, values) => app()!.ReplayScriptWithVars(scriptId, sessionId, values),
    startRecording: (name, description, sessionId) => app()!.StartScriptRecording(name, description, sessionId),
    stopRecording: () => app()!.StopScriptRecording(),
    recordingStatus: () => app()!.GetScriptRecordingStatus(),
    // Wails 专属：后端弹原生保存对话框导出 .sh
    exportScript: (scriptId) => app()!.ExportScript(scriptId),
};
