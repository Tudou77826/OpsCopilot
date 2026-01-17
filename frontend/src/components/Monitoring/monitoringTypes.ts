export interface TerminalSessionLite {
    id: string;
    title: string;
}

export interface JavaProcess {
    pid: number;
    user: string;
    etime: string;
    cmd: string;
}

export interface CommandResult {
    command: string;
    output: string;
    error?: string;
}

export interface Snapshot {
    pid: number;
    tools: {
        jcmd: boolean;
        jstat: boolean;
        jps: boolean;
    };
    process: {
        pid: number;
        ppid?: number;
        user?: string;
        cpu?: string;
        mem?: string;
        etime?: string;
        threads?: number;
        fd_count?: number;
        cmd?: string;
    };
    jvm: {
        vm_version: CommandResult;
        heap_info: CommandResult;
        gcutil_once: CommandResult;
    };
    host: {
        uptime: CommandResult;
        meminfo: CommandResult;
    };
}

