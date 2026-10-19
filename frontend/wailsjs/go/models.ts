export namespace achievements {

	export class Binding {
	    baseUrl: string;
	    member: companionsocial.Member;

	    static createFrom(source: any = {}) {
	        return new Binding(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseUrl = source["baseUrl"];
	        this.member = this.convertValues(source["member"], companionsocial.Member);
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
	export class DispatchResult {
	    sent: number;
	    failed: number;
	    warning?: string;

	    static createFrom(source: any = {}) {
	        return new DispatchResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sent = source["sent"];
	        this.failed = source["failed"];
	        this.warning = source["warning"];
	    }
	}
	export class Status {
	    summary: companionsocial.AchievementSummary;
	    binding?: Binding;
	    pending: number;
	    repairs: number;
	    participationPending?: boolean;
	    selected: string[];
	    profilePending: boolean;

	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = this.convertValues(source["summary"], companionsocial.AchievementSummary);
	        this.binding = this.convertValues(source["binding"], Binding);
	        this.pending = source["pending"];
	        this.repairs = source["repairs"];
	        this.participationPending = source["participationPending"];
	        this.selected = source["selected"];
	        this.profilePending = source["profilePending"];
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

export namespace assistantsettings {

	export class Snapshot {
	    personaPrompt: string;
	    defaultPrompt: string;
	    revision: string;

	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.personaPrompt = source["personaPrompt"];
	        this.defaultPrompt = source["defaultPrompt"];
	        this.revision = source["revision"];
	    }
	}

}

export namespace companion {

	export class Card {
	    speciesId: string;
	    name: string;
	    paletteId: string;
	    level?: number;
	    stage?: string;

	    static createFrom(source: any = {}) {
	        return new Card(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.speciesId = source["speciesId"];
	        this.name = source["name"];
	        this.paletteId = source["paletteId"];
	        this.level = source["level"];
	        this.stage = source["stage"];
	    }
	}
	export class Pet {
	    id: string;
	    speciesId: string;
	    name: string;
	    paletteId: string;
	    xp: number;
	    // Go type: time
	    adoptedAt: any;

	    static createFrom(source: any = {}) {
	        return new Pet(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.speciesId = source["speciesId"];
	        this.name = source["name"];
	        this.paletteId = source["paletteId"];
	        this.xp = source["xp"];
	        this.adoptedAt = this.convertValues(source["adoptedAt"], null);
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
	export class Snapshot {
	    schemaVersion: number;
	    ruleVersion: number;
	    revision: number;
	    pet?: Pet;
	    level: number;
	    levelXP: number;
	    levelXPRequired: number;
	    todayXP: number;
	    dailyXPMax: number;
	    hatchCost: number;
	    canHatch: boolean;

	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schemaVersion = source["schemaVersion"];
	        this.ruleVersion = source["ruleVersion"];
	        this.revision = source["revision"];
	        this.pet = this.convertValues(source["pet"], Pet);
	        this.level = source["level"];
	        this.levelXP = source["levelXP"];
	        this.levelXPRequired = source["levelXPRequired"];
	        this.todayXP = source["todayXP"];
	        this.dailyXPMax = source["dailyXPMax"];
	        this.hatchCost = source["hatchCost"];
	        this.canHatch = source["canHatch"];
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

export namespace companionsocial {

	export class Medal {
	    kind: string;
	    tier: string;
	    count: number;

	    static createFrom(source: any = {}) {
	        return new Medal(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.tier = source["tier"];
	        this.count = source["count"];
	    }
	}
	export class AchievementProgress {
	    kind: string;
	    count: number;
	    medal?: Medal;
	    nextThreshold?: number;

	    static createFrom(source: any = {}) {
	        return new AchievementProgress(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.count = source["count"];
	        this.medal = this.convertValues(source["medal"], Medal);
	        this.nextThreshold = source["nextThreshold"];
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
	export class AchievementSummary {
	    ruleVersion: number;
	    progress: AchievementProgress[];

	    static createFrom(source: any = {}) {
	        return new AchievementSummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ruleVersion = source["ruleVersion"];
	        this.progress = this.convertValues(source["progress"], AchievementProgress);
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

	export class MedalDefinition {
	    kind: string;
	    name: string;
	    unit: string;
	    group: string;
	    thresholds: number[];

	    static createFrom(source: any = {}) {
	        return new MedalDefinition(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.unit = source["unit"];
	        this.group = source["group"];
	        this.thresholds = source["thresholds"];
	    }
	}
	export class Member {
	    userId: string;
	    organizationId: string;
	    displayName: string;
	    participating: boolean;

	    static createFrom(source: any = {}) {
	        return new Member(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userId = source["userId"];
	        this.organizationId = source["organizationId"];
	        this.displayName = source["displayName"];
	        this.participating = source["participating"];
	    }
	}
	export class Profile {
	    userId: string;
	    displayName: string;
	    companion?: companion.Card;
	    medals: Medal[];

	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userId = source["userId"];
	        this.displayName = source["displayName"];
	        this.companion = this.convertValues(source["companion"], companion.Card);
	        this.medals = this.convertValues(source["medals"], Medal);
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
	export class PublicProfile {
	    userId: string;
	    displayName: string;
	    companion?: companion.Card;
	    medals: Medal[];
	    achievements: AchievementSummary;

	    static createFrom(source: any = {}) {
	        return new PublicProfile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userId = source["userId"];
	        this.displayName = source["displayName"];
	        this.companion = this.convertValues(source["companion"], companion.Card);
	        this.medals = this.convertValues(source["medals"], Medal);
	        this.achievements = this.convertValues(source["achievements"], AchievementSummary);
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
	export class RankingEntry {
	    rank: number;
	    count: number;
	    profile: Profile;

	    static createFrom(source: any = {}) {
	        return new RankingEntry(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rank = source["rank"];
	        this.count = source["count"];
	        this.profile = this.convertValues(source["profile"], Profile);
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
	export class RankingPage {
	    kind: string;
	    ruleVersion: number;
	    snapshotId: string;
	    offset: number;
	    total: number;
	    entries: RankingEntry[];

	    static createFrom(source: any = {}) {
	        return new RankingPage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.ruleVersion = source["ruleVersion"];
	        this.snapshotId = source["snapshotId"];
	        this.offset = source["offset"];
	        this.total = source["total"];
	        this.entries = this.convertValues(source["entries"], RankingEntry);
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

export namespace config {

	export class SessionShareConfig {
	    enabled: boolean;
	    remote_url: string;
	    branch: string;
	    secret_key: string;

	    static createFrom(source: any = {}) {
	        return new SessionShareConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.remote_url = source["remote_url"];
	        this.branch = source["branch"];
	        this.secret_key = source["secret_key"];
	    }
	}
	export class PatchStoreConfig {
	    enabled: boolean;
	    type: string;
	    remote_url: string;
	    branch: string;

	    static createFrom(source: any = {}) {
	        return new PatchStoreConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.type = source["type"];
	        this.remote_url = source["remote_url"];
	        this.branch = source["branch"];
	    }
	}
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
	export class AppearanceConfig {
	    theme: string;

	    static createFrom(source: any = {}) {
	        return new AppearanceConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	    }
	}
	export class TerminalConfig {
	    scrollback: number;
	    search_enabled: boolean;
	    highlight_enabled: boolean;
	    font_family: string;
	    font_size: number;

	    static createFrom(source: any = {}) {
	        return new TerminalConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scrollback = source["scrollback"];
	        this.search_enabled = source["search_enabled"];
	        this.highlight_enabled = source["highlight_enabled"];
	        this.font_family = source["font_family"];
	        this.font_size = source["font_size"];
	    }
	}
	export class ExperimentalConfig {
	    companion_enabled: boolean;

	    static createFrom(source: any = {}) {
	        return new ExperimentalConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.companion_enabled = source["companion_enabled"];
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
	export class CLIConfig {
	    exec_timeout_sec: number;

	    static createFrom(source: any = {}) {
	        return new CLIConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exec_timeout_sec = source["exec_timeout_sec"];
	    }
	}
	export class ScriptsConfig {
	    dir: string;

	    static createFrom(source: any = {}) {
	        return new ScriptsConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
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
	    level?: string;

	    static createFrom(source: any = {}) {
	        return new LogConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.level = source["level"];
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
	    log: LogConfig;
	    docs: DocsConfig;
	    scripts: ScriptsConfig;
	    cli: CLIConfig;
	    quick_commands: QuickCommand[];
	    completion_delay: number;
	    command_query_shortcut: string;
	    experimental: ExperimentalConfig;
	    terminal: TerminalConfig;
	    appearance: AppearanceConfig;
	    highlight_rules: HighlightRule[];
	    patch_store: PatchStoreConfig;
	    session_share: SessionShareConfig;

	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.llm = this.convertValues(source["llm"], LLMConfig);
	        this.log = this.convertValues(source["log"], LogConfig);
	        this.docs = this.convertValues(source["docs"], DocsConfig);
	        this.scripts = this.convertValues(source["scripts"], ScriptsConfig);
	        this.cli = this.convertValues(source["cli"], CLIConfig);
	        this.quick_commands = this.convertValues(source["quick_commands"], QuickCommand);
	        this.completion_delay = source["completion_delay"];
	        this.command_query_shortcut = source["command_query_shortcut"];
	        this.experimental = this.convertValues(source["experimental"], ExperimentalConfig);
	        this.terminal = this.convertValues(source["terminal"], TerminalConfig);
	        this.appearance = this.convertValues(source["appearance"], AppearanceConfig);
	        this.highlight_rules = this.convertValues(source["highlight_rules"], HighlightRule);
	        this.patch_store = this.convertValues(source["patch_store"], PatchStoreConfig);
	        this.session_share = this.convertValues(source["session_share"], SessionShareConfig);
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

export namespace dbworkspace {

	export class Column {
	    name: string;
	    type: string;

	    static createFrom(source: any = {}) {
	        return new Column(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}
	export class JDBCJar {
	    name: string;
	    sha256: string;

	    static createFrom(source: any = {}) {
	        return new JDBCJar(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.sha256 = source["sha256"];
	    }
	}
	export class JDBCDriver {
	    id: string;
	    name: string;
	    className: string;
	    urlTemplate: string;
	    dialect: string;
	    version: string;
	    jars: JDBCJar[];

	    static createFrom(source: any = {}) {
	        return new JDBCDriver(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.className = source["className"];
	        this.urlTemplate = source["urlTemplate"];
	        this.dialect = source["dialect"];
	        this.version = source["version"];
	        this.jars = this.convertValues(source["jars"], JDBCJar);
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
	export class JDBCImport {
	    name: string;
	    className: string;
	    urlTemplate: string;
	    dialect: string;
	    paths: string[];

	    static createFrom(source: any = {}) {
	        return new JDBCImport(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.className = source["className"];
	        this.urlTemplate = source["urlTemplate"];
	        this.dialect = source["dialect"];
	        this.paths = source["paths"];
	    }
	}

	export class JDBCStatus {
	    available: boolean;
	    message: string;
	    drivers: JDBCDriver[];

	    static createFrom(source: any = {}) {
	        return new JDBCStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.message = source["message"];
	        this.drivers = this.convertValues(source["drivers"], JDBCDriver);
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
	export class Profile {
	    id: string;
	    revision: number;
	    name: string;
	    environment: string;
	    group?: string;
	    engine: string;
	    driverId?: string;
	    jdbcAllowPublicKeyRetrieval?: boolean;
	    host: string;
	    port: number;
	    user: string;
	    database: string;
	    tls: string;
	    readOnly: boolean;
	    gatewayId: string;
	    gatewayMode?: string;
	    passwordSet: boolean;

	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.revision = source["revision"];
	        this.name = source["name"];
	        this.environment = source["environment"];
	        this.group = source["group"];
	        this.engine = source["engine"];
	        this.driverId = source["driverId"];
	        this.jdbcAllowPublicKeyRetrieval = source["jdbcAllowPublicKeyRetrieval"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.database = source["database"];
	        this.tls = source["tls"];
	        this.readOnly = source["readOnly"];
	        this.gatewayId = source["gatewayId"];
	        this.gatewayMode = source["gatewayMode"];
	        this.passwordSet = source["passwordSet"];
	    }
	}
	export class ProfileInput {
	    id: string;
	    revision: number;
	    name: string;
	    environment: string;
	    group?: string;
	    engine: string;
	    driverId?: string;
	    jdbcAllowPublicKeyRetrieval?: boolean;
	    host: string;
	    port: number;
	    user: string;
	    database: string;
	    tls: string;
	    readOnly: boolean;
	    gatewayId: string;
	    gatewayMode?: string;
	    passwordSet: boolean;
	    password?: string;

	    static createFrom(source: any = {}) {
	        return new ProfileInput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.revision = source["revision"];
	        this.name = source["name"];
	        this.environment = source["environment"];
	        this.group = source["group"];
	        this.engine = source["engine"];
	        this.driverId = source["driverId"];
	        this.jdbcAllowPublicKeyRetrieval = source["jdbcAllowPublicKeyRetrieval"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.database = source["database"];
	        this.tls = source["tls"];
	        this.readOnly = source["readOnly"];
	        this.gatewayId = source["gatewayId"];
	        this.gatewayMode = source["gatewayMode"];
	        this.passwordSet = source["passwordSet"];
	        this.password = source["password"];
	    }
	}
	export class QueryRequest {
	    database: string;
	    sessionId: string;
	    requestId: string;
	    sql: string;
	    mode?: string;
	    cursorOffset?: number;
	    limit: number;
	    timeoutSeconds: number;

	    static createFrom(source: any = {}) {
	        return new QueryRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.database = source["database"];
	        this.sessionId = source["sessionId"];
	        this.requestId = source["requestId"];
	        this.sql = source["sql"];
	        this.mode = source["mode"];
	        this.cursorOffset = source["cursorOffset"];
	        this.limit = source["limit"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	    }
	}
	export class ResultSet {
	    statement: string;
	    columns: Column[];
	    rows: string[][];
	    affectedRows: number;
	    truncated: boolean;

	    static createFrom(source: any = {}) {
	        return new ResultSet(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.statement = source["statement"];
	        this.columns = this.convertValues(source["columns"], Column);
	        this.rows = source["rows"];
	        this.affectedRows = source["affectedRows"];
	        this.truncated = source["truncated"];
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
	export class QueryResult {
	    sets: ResultSet[];
	    durationMs: number;
	    error: string;
	    cancelled: boolean;
	    disconnected: boolean;

	    static createFrom(source: any = {}) {
	        return new QueryResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sets = this.convertValues(source["sets"], ResultSet);
	        this.durationMs = source["durationMs"];
	        this.error = source["error"];
	        this.cancelled = source["cancelled"];
	        this.disconnected = source["disconnected"];
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

	export class SQLTemplateRequest {
	    sessionId: string;
	    database: string;
	    table: string;
	    kind: string;
	    column?: string;

	    static createFrom(source: any = {}) {
	        return new SQLTemplateRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.database = source["database"];
	        this.table = source["table"];
	        this.kind = source["kind"];
	        this.column = source["column"];
	    }
	}
	export class SessionInfo {
	    id: string;
	    profileId: string;
	    version: string;
	    database: string;
	    readOnly: boolean;

	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.profileId = source["profileId"];
	        this.version = source["version"];
	        this.database = source["database"];
	        this.readOnly = source["readOnly"];
	    }
	}
	export class Structure {
	    columns: ResultSet;
	    indexes: ResultSet;
	    ddl: string;

	    static createFrom(source: any = {}) {
	        return new Structure(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = this.convertValues(source["columns"], ResultSet);
	        this.indexes = this.convertValues(source["indexes"], ResultSet);
	        this.ddl = source["ddl"];
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
	export class Table {
	    name: string;
	    type: string;
	    comment: string;

	    static createFrom(source: any = {}) {
	        return new Table(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.comment = source["comment"];
	    }
	}
	export class TableRequest {
	    requestId: string;
	    sessionId: string;
	    database: string;
	    table: string;
	    offset: number;
	    limit: number;

	    static createFrom(source: any = {}) {
	        return new TableRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.sessionId = source["sessionId"];
	        this.database = source["database"];
	        this.table = source["table"];
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	    }
	}

}

export namespace main {

	export class ConnectConfig {
	    name: string;
	    protocol?: string;
	    host: string;
	    port: number;
	    user: string;
	    password: string;
	    rootPassword: string;
	    hostKey?: string;
	    bastion?: ConnectConfig;
	    group: string;

	    static createFrom(source: any = {}) {
	        return new ConnectConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.protocol = source["protocol"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.rootPassword = source["rootPassword"];
	        this.hostKey = source["hostKey"];
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
	export class DatabaseGateway {
	    id: string;
	    name: string;
	    host: string;

	    static createFrom(source: any = {}) {
	        return new DatabaseGateway(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	    }
	}
	export class ImportAnalysis {
	    total: number;
	    supported: number;
	    unsupported: number;
	    existing: number;
	    withPassword: number;
	    passwordDecrypted: number;
	    passwordFailed: number;
	    groups: number;
	    protocols: Record<string, number>;
	    warnings: string[];

	    static createFrom(source: any = {}) {
	        return new ImportAnalysis(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.supported = source["supported"];
	        this.unsupported = source["unsupported"];
	        this.existing = source["existing"];
	        this.withPassword = source["withPassword"];
	        this.passwordDecrypted = source["passwordDecrypted"];
	        this.passwordFailed = source["passwordFailed"];
	        this.groups = source["groups"];
	        this.protocols = source["protocols"];
	        this.warnings = source["warnings"];
	    }
	}
	export class ImportOptions {
	    decryptPassword: boolean;
	    sourceSid?: string;
	    masterPassword?: string;

	    static createFrom(source: any = {}) {
	        return new ImportOptions(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.decryptPassword = source["decryptPassword"];
	        this.sourceSid = source["sourceSid"];
	        this.masterPassword = source["masterPassword"];
	    }
	}
	export class ImportReport {
	    imported: number;
	    skippedExisting: number;
	    skippedUnsupported: number;
	    passwordDecrypted: number;
	    passwordFailed: number;
	    warnings: string[];

	    static createFrom(source: any = {}) {
	        return new ImportReport(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.skippedExisting = source["skippedExisting"];
	        this.skippedUnsupported = source["skippedUnsupported"];
	        this.passwordDecrypted = source["passwordDecrypted"];
	        this.passwordFailed = source["passwordFailed"];
	        this.warnings = source["warnings"];
	    }
	}
	export class ModuleInfo {
	    name: string;
	    fileName: string;

	    static createFrom(source: any = {}) {
	        return new ModuleInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.fileName = source["fileName"];
	    }
	}
	export class QuickCommandSetItem {
	    name: string;
	    content: string;
	    type: string;
	    supported: boolean;
	    skipReason?: string;
	    existing: boolean;

	    static createFrom(source: any = {}) {
	        return new QuickCommandSetItem(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.content = source["content"];
	        this.type = source["type"];
	        this.supported = source["supported"];
	        this.skipReason = source["skipReason"];
	        this.existing = source["existing"];
	    }
	}
	export class QuickCommandSetRow {
	    source: string;
	    name: string;
	    buttons: number;
	    importable: number;
	    unsupported: number;
	    existing: number;
	    group: string;
	    items: QuickCommandSetItem[];

	    static createFrom(source: any = {}) {
	        return new QuickCommandSetRow(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.name = source["name"];
	        this.buttons = source["buttons"];
	        this.importable = source["importable"];
	        this.unsupported = source["unsupported"];
	        this.existing = source["existing"];
	        this.group = source["group"];
	        this.items = this.convertValues(source["items"], QuickCommandSetItem);
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
	export class QuickCommandImportAnalysis {
	    sets: number;
	    buttons: number;
	    importable: number;
	    unsupported: number;
	    existing: number;
	    groups: string[];
	    rows: QuickCommandSetRow[];
	    warnings: string[];

	    static createFrom(source: any = {}) {
	        return new QuickCommandImportAnalysis(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sets = source["sets"];
	        this.buttons = source["buttons"];
	        this.importable = source["importable"];
	        this.unsupported = source["unsupported"];
	        this.existing = source["existing"];
	        this.groups = source["groups"];
	        this.rows = this.convertValues(source["rows"], QuickCommandSetRow);
	        this.warnings = source["warnings"];
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
	export class QuickCommandImportItem {
	    name: string;
	    content: string;
	    group?: string;

	    static createFrom(source: any = {}) {
	        return new QuickCommandImportItem(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.content = source["content"];
	        this.group = source["group"];
	    }
	}
	export class QuickCommandImportOptions {
	    defaultGroup?: string;

	    static createFrom(source: any = {}) {
	        return new QuickCommandImportOptions(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaultGroup = source["defaultGroup"];
	    }
	}
	export class QuickCommandImportReport {
	    imported: number;
	    skippedExisting: number;
	    skippedUnsupported: number;
	    groups: string[];
	    warnings: string[];

	    static createFrom(source: any = {}) {
	        return new QuickCommandImportReport(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.skippedExisting = source["skippedExisting"];
	        this.skippedUnsupported = source["skippedUnsupported"];
	        this.groups = source["groups"];
	        this.warnings = source["warnings"];
	    }
	}
	export class QuickCommandImportSelection {
	    source: string;
	    group: string;
	    items: QuickCommandImportItem[];

	    static createFrom(source: any = {}) {
	        return new QuickCommandImportSelection(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.group = source["group"];
	        this.items = this.convertValues(source["items"], QuickCommandImportItem);
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


	export class ServiceInfo {
	    name: string;
	    modules: ModuleInfo[];

	    static createFrom(source: any = {}) {
	        return new ServiceInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.modules = this.convertValues(source["modules"], ModuleInfo);
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
	export class SessionTreeNode {
	    id: string;
	    name: string;
	    type: string;
	    children?: SessionTreeNode[];
	    config?: ConnectConfig;

	    static createFrom(source: any = {}) {
	        return new SessionTreeNode(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.children = this.convertValues(source["children"], SessionTreeNode);
	        this.config = this.convertValues(source["config"], ConnectConfig);
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
	export class SharedConnectResult {
	    success: boolean;
	    message?: string;
	    config?: ConnectConfig;

	    static createFrom(source: any = {}) {
	        return new SharedConnectResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.config = this.convertValues(source["config"], ConnectConfig);
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
	export class XshellCredentialStatus {
	    available: boolean;
	    maskedSid?: string;
	    windowsUser?: string;
	    message: string;

	    static createFrom(source: any = {}) {
	        return new XshellCredentialStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.maskedSid = source["maskedSid"];
	        this.windowsUser = source["windowsUser"];
	        this.message = source["message"];
	    }
	}
	export class XshellQuickButtonDir {
	    path: string;
	    version: number;
	    sets: number;
	    buttons: number;

	    static createFrom(source: any = {}) {
	        return new XshellQuickButtonDir(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.version = source["version"];
	        this.sets = source["sets"];
	        this.buttons = source["buttons"];
	    }
	}
	export class XshellSessionDir {
	    path: string;
	    version: number;
	    sessions: number;

	    static createFrom(source: any = {}) {
	        return new XshellSessionDir(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.version = source["version"];
	        this.sessions = source["sessions"];
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

	export class ScriptStep {
	    command?: string;
	    comment?: string;
	    delay?: number;
	    enabled: boolean;
	    original_index?: number;

	    static createFrom(source: any = {}) {
	        return new ScriptStep(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	        this.comment = source["comment"];
	        this.delay = source["delay"];
	        this.enabled = source["enabled"];
	        this.original_index = source["original_index"];
	    }
	}
	export class ScriptVariable {
	    name: string;
	    display_name: string;
	    default_value: string;
	    required: boolean;
	    description: string;

	    static createFrom(source: any = {}) {
	        return new ScriptVariable(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.display_name = source["display_name"];
	        this.default_value = source["default_value"];
	        this.required = source["required"];
	        this.description = source["description"];
	    }
	}
	export class ScriptCommand {
	    content: string;
	    comment: string;
	    delay: number;
	    enabled: boolean;

	    static createFrom(source: any = {}) {
	        return new ScriptCommand(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	        this.comment = source["comment"];
	        this.delay = source["delay"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Script {
	    id: string;
	    // Go type: time
	    start_time: any;
	    // Go type: time
	    end_time?: any;
	    // Go type: time
	    updated_at?: any;
	    host: string;
	    user: string;
	    name: string;
	    description: string;
	    commands: ScriptCommand[];
	    variables?: ScriptVariable[];
	    steps?: ScriptStep[];

	    static createFrom(source: any = {}) {
	        return new Script(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.end_time = this.convertValues(source["end_time"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.host = source["host"];
	        this.user = source["user"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.commands = this.convertValues(source["commands"], ScriptCommand);
	        this.variables = this.convertValues(source["variables"], ScriptVariable);
	        this.steps = this.convertValues(source["steps"], ScriptStep);
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

export namespace security {

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
	export class FileAccessPolicy {
	    id: string;
	    name: string;
	    ip_ranges: string[];
	    read_paths: string[];
	    write_paths: string[];
	    denied_paths: string[];
	    allowed_local_dirs: string[];
	    max_read_bytes: number;
	    max_write_bytes: number;

	    static createFrom(source: any = {}) {
	        return new FileAccessPolicy(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.ip_ranges = source["ip_ranges"];
	        this.read_paths = source["read_paths"];
	        this.write_paths = source["write_paths"];
	        this.denied_paths = source["denied_paths"];
	        this.allowed_local_dirs = source["allowed_local_dirs"];
	        this.max_read_bytes = source["max_read_bytes"];
	        this.max_write_bytes = source["max_write_bytes"];
	    }
	}
	export class FileAccessConfig {
	    version: string;
	    policies: FileAccessPolicy[];

	    static createFrom(source: any = {}) {
	        return new FileAccessConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.policies = this.convertValues(source["policies"], FileAccessPolicy);
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
	export class WhitelistConfig {
	    version: string;
	    policies: Policy[];

	    static createFrom(source: any = {}) {
	        return new WhitelistConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
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

export namespace servicecenter {

	export class Announcement {
	    id: string;
	    text: string;
	    url: string;
	    order: number;
	    // Go type: time
	    startsAt: any;
	    // Go type: time
	    endsAt: any;

	    static createFrom(source: any = {}) {
	        return new Announcement(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	        this.url = source["url"];
	        this.order = source["order"];
	        this.startsAt = this.convertValues(source["startsAt"], null);
	        this.endsAt = this.convertValues(source["endsAt"], null);
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
	export class Policy {
	    version: string;
	    notice: string;

	    static createFrom(source: any = {}) {
	        return new Policy(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.notice = source["notice"];
	    }
	}
	export class Settings {
	    baseUrl: string;
	    choice: string;
	    needsConsent: boolean;
	    policy: Policy;
	    ready: boolean;

	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseUrl = source["baseUrl"];
	        this.choice = source["choice"];
	        this.needsConsent = source["needsConsent"];
	        this.policy = this.convertValues(source["policy"], Policy);
	        this.ready = source["ready"];
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
