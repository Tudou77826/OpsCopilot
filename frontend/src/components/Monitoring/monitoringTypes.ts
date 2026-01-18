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
        fd_limit?: number;
        vm_rss_kb?: number;
        vm_size_kb?: number;
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

export interface TopThread {
    tid: number;
    tid_hex: string;
    cpu: string;
    java_name?: string;
    java_state?: string;
    stack_top?: string;
}

export interface ThreadStateCounts {
    runnable: number;
    blocked: number;
    waiting: number;
    timed_waiting: number;
    new: number;
    terminated: number;
    unknown: number;
}

