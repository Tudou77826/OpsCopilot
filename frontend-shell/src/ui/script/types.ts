/**
 * 结构化脚本数据模型 —— 与 pkg/script 的 JSON 形态一致（snake_case）。
 * steps 是首选编辑模型；commands 是录制产出的镜像，二者由后端保持同步。
 */
export interface ScriptVariable {
    name: string;
    display_name: string;
    default_value: string;
    required: boolean;
    description: string;
}

export interface ScriptStep {
    command?: string;
    comment?: string;
    delay?: number;
    enabled: boolean;
    original_index?: number;
}

export interface ScriptCommand {
    index: number;
    content: string;
    comment: string;
    delay: number;
    enabled: boolean;
}

export interface ScriptData {
    id: string;
    name: string;
    description: string;
    start_time: string;
    end_time?: string;
    updated_at?: string;
    host: string;
    user: string;
    commands: ScriptCommand[];
    variables?: ScriptVariable[];
    steps?: ScriptStep[];
}

export interface ScriptRecordingStatus {
    is_recording: boolean;
    script_id?: string;
    name?: string;
    command_count: number;
    duration: number;
}

/**
 * 脚本宿主端口：结构化脚本的增删改查、变量回放与录制控制。
 * Wails 由 window.go.main.App 适配；Sidecar 由 shell.script.* RPC 适配
 * （复用 pkg/script + pkg/recorder，录制走后端 Write 拦截）。
 * exportScript 为 Wails 专属（原生保存对话框），缺失时组件隐藏导出入口。
 */
export interface ScriptRuntime {
    list(): Promise<ScriptData[]>;
    load(scriptId: string): Promise<ScriptData>;
    update(script: ScriptData): Promise<void>;
    remove(scriptId: string): Promise<void>;
    create(name: string, description: string): Promise<ScriptData>;
    replay(scriptId: string, sessionId: string): Promise<void>;
    replayWithVars(scriptId: string, sessionId: string, values: Record<string, string>): Promise<void>;
    startRecording(name: string, description: string, sessionId: string): Promise<ScriptData>;
    stopRecording(): Promise<ScriptData>;
    recordingStatus(): Promise<ScriptRecordingStatus>;
    /** 导出 .sh 文件（宿主有原生保存对话框时提供）。 */
    exportScript?(scriptId: string): Promise<void>;
}
