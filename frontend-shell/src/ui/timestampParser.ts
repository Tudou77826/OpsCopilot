/**
 * 时间戳解析器
 *
 * 识别用户选中的时间戳数字，转换为可读时间格式。
 * 采用保守策略：只识别落在 2000-2030 年范围内的数字，避免误判。
 */

/** 解析结果 */
export interface TimestampResult {
    /** 本地时区时间 */
    local: string;
    /** UTC 时间 */
    utc: string;
    /** 原始时间戳 */
    raw: string;
}

/** 时间戳有效范围（秒级） */
const MIN_TIMESTAMP_SEC = 946684800;   // 2000-01-01 00:00:00 UTC
const MAX_TIMESTAMP_SEC = 1893456000;  // 2030-01-01 00:00:00 UTC

/** 时间戳有效范围（毫秒级） */
const MIN_TIMESTAMP_MS = MIN_TIMESTAMP_SEC * 1000;
const MAX_TIMESTAMP_MS = MAX_TIMESTAMP_SEC * 1000;

/**
 * 尝试解析时间戳
 *
 * @param text 用户选中的文本
 * @returns 解析成功返回结果，失败返回 null
 */
export function parseTimestamp(text: string): TimestampResult | null {
    // 快速预检：必须是纯数字
    const trimmed = text.trim();
    if (!trimmed || !/^\d+$/.test(trimmed)) {
        return null;
    }

    const num = parseInt(trimmed, 10);
    if (isNaN(num) || num <= 0) {
        return null;
    }

    // 根据长度判断是秒级还是毫秒级
    let timestampMs: number;
    if (trimmed.length === 10) {
        // 秒级时间戳
        if (num < MIN_TIMESTAMP_SEC || num > MAX_TIMESTAMP_SEC) {
            return null;
        }
        timestampMs = num * 1000;
    } else if (trimmed.length === 13) {
        // 毫秒级时间戳
        if (num < MIN_TIMESTAMP_MS || num > MAX_TIMESTAMP_MS) {
            return null;
        }
        timestampMs = num;
    } else {
        // 其他长度不处理
        return null;
    }

    // 转换为时间
    const date = new Date(timestampMs);
    if (isNaN(date.getTime())) {
        return null;
    }

    // 格式化输出
    const formatOptions: Intl.DateTimeFormatOptions = {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false,
    };

    const localStr = date.toLocaleString('zh-CN', formatOptions);
    const utcStr = date.toISOString().replace('T', ' ').replace(/\.\d{3}Z$/, ' UTC');

    return {
        local: localStr,
        utc: utcStr,
        raw: trimmed,
    };
}