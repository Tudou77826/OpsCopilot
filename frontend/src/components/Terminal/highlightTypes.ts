export interface HighlightRule {
    id: string;
    name: string;
    pattern: string;
    is_enabled: boolean;
    priority: number;
    style: HighlightStyle;
}

export interface HighlightStyle {
    background_color?: string;
    color?: string;
    font_weight?: string;
    text_decoration?: string;
    opacity?: number;
}

export interface TerminalConfig {
    scrollback: number;
    search_enabled: boolean;
    highlight_enabled: boolean;
}

