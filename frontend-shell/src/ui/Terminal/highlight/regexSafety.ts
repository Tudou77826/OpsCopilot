// 规则高亮正则安全性统一判断（modal 与执行侧共用，保证同源）。
//
// 设计目标：
//   1. 同源——规则编辑界面与 RuleHighlightController 执行侧调用同一函数，
//      杜绝「界面判定安全、执行侧却丢弃」的脱节。
//   2. 不误杀——像 (a+)+、(error|fail)+ 这类实际安全的正则不应被判为危险，
//      否则它们被静默丢弃，表现为「该亮的不亮」。
//   3. 拦得住真·灾难——确证的灾难性回溯形态（如 (.*)+、(.+)+、.*.*）
//      统一归入 severe，既在界面禁用启用、又在执行侧丢弃，防老鼠屎卡死 worker。

export type RiskLevel = 'safe' | 'moderate' | 'high' | 'severe';

export interface PatternRisk {
    level: RiskLevel;
    issues: string[];
    // 是否允许启用：severe 不允许（界面禁用启用开关、执行侧丢弃）。
    canEnable: boolean;
    // 语法是否合法（非法时执行侧同样会丢弃，且界面可据此提示）。
    syntaxError?: string;
}

// 取出可选的 (?i) 前缀后的正则本体（与执行侧 compileRules 处理一致）。
const stripFlags = (pattern: string): string => pattern.replace(/^\(\?i\)/, '');

// 判断一条 pattern 是否确证为灾难性回溯形态。
// 与历史实现 hasUnsafeRegexShape 相比，这里收窄了判定，不再误杀 (a+)+、(a|b)+。
const isCatastrophicShape = (body: string): boolean => {
    // 1. 重复的贪婪通配量词：.*.*  /  .+.+  /  (.*){2,}
    if (/(\.\*){2,}|(\.\+){2,}/.test(body)) return true;
    // 2. 以 .* / .+ 开头紧跟量词：.*+  /  .+*
    if (/^(\.\*|\.\+)[+*]/.test(body)) return true;
    // 3. 捕获/非捕获组内为 .* 或 .+，组外再跟量词：(.*)+  /  (?:.+)*  —— 确证的指数级回溯
    if (/\((?:\?:)?(?:\.\*|\.\+)\)[+*{]/.test(body)) return true;
    return false;
};

// 复杂的嵌套量词组合：仅作 moderate 提示，不阻断。
const hasComplexNestedQuantifier = (body: string): boolean =>
    /\(\?:?[^)]*[+*][^)]*\)[+*]/.test(body);

export function assessPattern(pattern: string): PatternRisk {
    const p = pattern.trim();
    if (!p) return { level: 'safe', issues: [], canEnable: true };

    const body = stripFlags(p);
    const issues: string[] = [];
    let level: RiskLevel = 'safe';

    // 语法校验：非法正则界面会给提示、执行侧会丢弃。
    let syntaxError: string | undefined;
    try {
        new RegExp(body, 'g');
    } catch (e) {
        syntaxError = e instanceof Error ? e.message : String(e);
        return {
            level: 'severe',
            issues: [`正则语法错误：${syntaxError}`],
            canEnable: false,
            syntaxError,
        };
    }

    // 长度提示（不阻断）
    if (p.length > 500) {
        issues.push(`正则非常长 (${p.length}字符)`);
        level = 'high';
    } else if (p.length > 200) {
        issues.push(`正则较长 (${p.length}字符)`);
        level = 'moderate';
    }

    // 确证的灾难性回溯 → severe（界面禁用启用 + 执行侧丢弃）
    if (isCatastrophicShape(body)) {
        issues.push('灾难性回溯模式，会导致匹配卡死');
        level = 'severe';
    } else if (hasComplexNestedQuantifier(body)) {
        // 一般的嵌套量词 → moderate 提示
        issues.push('复杂的嵌套量词组合');
        if (level === 'safe') level = 'moderate';
    }

    return {
        level,
        issues,
        canEnable: level !== 'severe',
        syntaxError,
    };
}
