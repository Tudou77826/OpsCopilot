export namespace config {
	
	export class HighlightStyle {
	    background_color?: string;
	    color?: string;
	    font_weight?: string;
	    text_decoration?: string;
	    opacity?: number;
	
	    static createFrom(source: any = {}) {
	        return new HighlightStyle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.background_color = source["background_color"];
	        this.color = source["color"];
	        this.font_weight = source["font_weight"];
	        this.text_decoration = source["text_decoration"];
	        this.opacity = source["opacity"];
	    }
	}
	export class HighlightRule {
	    id: string;
	    name: string;
	    pattern: string;
	    is_enabled: boolean;
	    priority: number;
	    style: HighlightStyle;
	
	    static createFrom(source: any = {}) {
	        return new HighlightRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.pattern = source["pattern"];
	        this.is_enabled = source["is_enabled"];
	        this.priority = source["priority"];
	        this.style = this.convertValues(source["style"], HighlightStyle);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TerminalConfig {
	    scrollback: number;
	    search_enabled: boolean;
	    highlight_enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TerminalConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scrollback = source["scrollback"];
	        this.search_enabled = source["search_enabled"];
	        this.highlight_enabled = source["highlight_enabled"];
	    }
	}
	export class ExperimentalConfig {
	
	
	    static createFrom(source: any = {}) {
	        return new ExperimentalConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class QuickCommand {
	    id: string;
	    name: string;
	    content: string;
	    group?: string;
	
	    static createFrom(source: any = {}) {
	        return new QuickCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.content = source["content"];
	        this.group = source["group"];
	    }
	}
	export class DocsConfig {
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new DocsConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	    }
	}
	export class LogConfig {
	    dir: string;
	
	    static createFrom(source: any = {}) {
	        return new LogConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	    }
	}
	export class LLMConfig {
	    APIKey: string;
	    BaseURL: string;
	    FastModel: string;
	    ComplexModel: string;
	    Model?: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.APIKey = source["APIKey"];
	        this.BaseURL = source["BaseURL"];
	        this.FastModel = source["FastModel"];
	        this.ComplexModel = source["ComplexModel"];
	        this.Model = source["Model"];
	    }
	}
	export class AppConfig {
	    llm: LLMConfig;
	    prompts: Record<string, string>;
	    log: LogConfig;
	    docs: DocsConfig;
	    quick_commands: QuickCommand[];
	    completion_delay: number;
	    command_query_shortcut: string;
	    // Go type: ExperimentalConfig
	    experimental: any;
	    terminal: TerminalConfig;
	    highlight_rules: HighlightRule[];
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.llm = this.convertValues(source["llm"], LLMConfig);
	        this.prompts = source["prompts"];
	        this.log = this.convertValues(source["log"], LogConfig);
	        this.docs = this.convertValues(source["docs"], DocsConfig);
	        this.quick_commands = this.convertValues(source["quick_commands"], QuickCommand);
	        this.completion_delay = source["completion_delay"];
	        this.command_query_shortcut = source["command_query_shortcut"];
	        this.experimental = this.convertValues(source["experimental"], null);
	        this.terminal = this.convertValues(source["terminal"], TerminalConfig);
	        this.highlight_rules = this.convertValues(source["highlight_rules"], HighlightRule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	

}

export namespace main {
	
	export class ConnectConfig {
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	    rootPassword: string;
	    bastion?: ConnectConfig;
	    group: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.rootPassword = source["rootPassword"];
	        this.bastion = this.convertValues(source["bastion"], ConnectConfig);
	        this.group = source["group"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConnectResult {
	    success: boolean;
	    sessionId: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.sessionId = source["sessionId"];
	        this.message = source["message"];
	    }
	}

}

export namespace mcpserver {
	
	export class Command {
	    pattern: string;
	    category: string;
	    description: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Command(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pattern = source["pattern"];
	        this.category = source["category"];
	        this.description = source["description"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Policy {
	    id: string;
	    name: string;
	    description: string;
	    ip_ranges: string[];
	    commands: Command[];
	
	    static createFrom(source: any = {}) {
	        return new Policy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.ip_ranges = source["ip_ranges"];
	        this.commands = this.convertValues(source["commands"], Command);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RiskAssessment {
	    is_risky: boolean;
	    risk_level: string;
	    reason: string;
	    suggestions: string;
	
	    static createFrom(source: any = {}) {
	        return new RiskAssessment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.is_risky = source["is_risky"];
	        this.risk_level = source["risk_level"];
	        this.reason = source["reason"];
	        this.suggestions = source["suggestions"];
	    }
	}
	export class WhitelistConfig {
	    version: string;
	    llm_check_enabled: boolean;
	    policies: Policy[];
	
	    static createFrom(source: any = {}) {
	        return new WhitelistConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.llm_check_enabled = source["llm_check_enabled"];
	        this.policies = this.convertValues(source["policies"], Policy);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace recorder {
	
	export class RecordedCommand {
	    index: number;
	    content: string;
	    output?: string;
	    timestamp: number;
	    duration?: number;
	    corrected?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RecordedCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.content = source["content"];
	        this.output = source["output"];
	        this.timestamp = source["timestamp"];
	        this.duration = source["duration"];
	        this.corrected = source["corrected"];
	    }
	}
	export class TimelineEvent {
	    // Go type: time
	    timestamp: any;
	    type: string;
	    content: string;
	    metadata?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new TimelineEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.type = source["type"];
	        this.content = source["content"];
	        this.metadata = source["metadata"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RecordingSession {
	    id: string;
	    type: string;
	    // Go type: time
	    start_time: any;
	    // Go type: time
	    end_time?: any;
	    // Go type: time
	    updated_at?: any;
	    session_id: string;
	    host: string;
	    user: string;
	    commands: RecordedCommand[];
	    metadata?: Record<string, any>;
	    timeline?: TimelineEvent[];
	    problem?: string;
	    context?: string[];
	    root_cause?: string;
	    conclusion?: string;
	    suggestions?: string[];
	
	    static createFrom(source: any = {}) {
	        return new RecordingSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.end_time = this.convertValues(source["end_time"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.session_id = source["session_id"];
	        this.host = source["host"];
	        this.user = source["user"];
	        this.commands = this.convertValues(source["commands"], RecordedCommand);
	        this.metadata = source["metadata"];
	        this.timeline = this.convertValues(source["timeline"], TimelineEvent);
	        this.problem = source["problem"];
	        this.context = source["context"];
	        this.root_cause = source["root_cause"];
	        this.conclusion = source["conclusion"];
	        this.suggestions = source["suggestions"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace script {
	
	export class ScriptCommand {
	    index: number;
	    content: string;
	    output?: string;
	    timestamp: number;
	    duration?: number;
	    corrected?: boolean;
	    comment: string;
	    delay: number;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ScriptCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.content = source["content"];
	        this.output = source["output"];
	        this.timestamp = source["timestamp"];
	        this.duration = source["duration"];
	        this.corrected = source["corrected"];
	        this.comment = source["comment"];
	        this.delay = source["delay"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Script {
	    id: string;
	    type: string;
	    // Go type: time
	    start_time: any;
	    // Go type: time
	    end_time?: any;
	    // Go type: time
	    updated_at?: any;
	    session_id: string;
	    host: string;
	    user: string;
	    metadata?: Record<string, any>;
	    timeline?: recorder.TimelineEvent[];
	    problem?: string;
	    context?: string[];
	    root_cause?: string;
	    conclusion?: string;
	    suggestions?: string[];
	    name: string;
	    description: string;
	    commands: ScriptCommand[];
	
	    static createFrom(source: any = {}) {
	        return new Script(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.end_time = this.convertValues(source["end_time"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.session_id = source["session_id"];
	        this.host = source["host"];
	        this.user = source["user"];
	        this.metadata = source["metadata"];
	        this.timeline = this.convertValues(source["timeline"], recorder.TimelineEvent);
	        this.problem = source["problem"];
	        this.context = source["context"];
	        this.root_cause = source["root_cause"];
	        this.conclusion = source["conclusion"];
	        this.suggestions = source["suggestions"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.commands = this.convertValues(source["commands"], ScriptCommand);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ScriptStatus {
	    is_recording: boolean;
	    script_id?: string;
	    name?: string;
	    command_count: number;
	    duration: number;
	
	    static createFrom(source: any = {}) {
	        return new ScriptStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.is_recording = source["is_recording"];
	        this.script_id = source["script_id"];
	        this.name = source["name"];
	        this.command_count = source["command_count"];
	        this.duration = source["duration"];
	    }
	}

}

export namespace sessionmanager {
	
	export class Session {
	    id: string;
	    name: string;
	    type: string;
	    children?: Session[];
	    config?: sshclient.ConnectConfig;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.children = this.convertValues(source["children"], Session);
	        this.config = this.convertValues(source["config"], sshclient.ConnectConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace sshclient {
	
	export class ConnectConfig {
	    name: string;
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	    root_password: string;
	    bastion?: ConnectConfig;
	    group?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.root_password = source["root_password"];
	        this.bastion = this.convertValues(source["bastion"], ConnectConfig);
	        this.group = source["group"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

