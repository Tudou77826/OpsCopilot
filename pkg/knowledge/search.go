package knowledge

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// 打分检索的字段权重：错误码 token 命中 Keywords 时乘 errorCodeBoost
const (
	weightKeywords   = 5.0
	weightTitle      = 4.0
	weightPhenomena  = 3.0
	weightComponents = 3.0
	weightBody       = 1.0
	errorCodeBoost   = 2.0

	hitThreshold = 0.5 // score ≥ 该值为命中，(0, 该值) 为弱命中
	weakTopN     = 3
	snippetRunes = 120
)

// errorCodeRe 识别 E215 / OOM1 这类错误码强标识
var errorCodeRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*\d+$`)

// SearchHit 单个场景的检索命中
type SearchHit struct {
	ID      string  `json:"id"`
	Service string  `json:"service"`
	Module  string  `json:"module"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// SearchResult 检索结果。零命中时 MissKind/CoveredServices/WeakMatches/Hint 描述降级路径。
type SearchResult struct {
	Query           string      `json:"query"`
	Hits            []SearchHit `json:"hits"`
	MissKind        string      `json:"miss_kind,omitempty"`        // term_mismatch | domain_uncovered，仅零命中时出现
	CoveredServices []string    `json:"covered_services,omitempty"` // 仅零命中时出现
	WeakMatches     []SearchHit `json:"weak_matches,omitempty"`     // 仅零命中时出现
	Hint            string      `json:"hint,omitempty"`             // 仅零命中时出现
}

// Search 在知识库目录上做加权词匹配检索。
// 打分规则：token 命中 Keywords×5（错误码再×2）、Title×4、Phenomena/Components×3、场景正文×1，
// score = 命中权重和 / (token 数 × 单 token 最高可得权重)。
func Search(dir string, cat *Catalog, query string, topN int) (*SearchResult, error) {
	if topN <= 0 {
		topN = 5
	}
	res := &SearchResult{Query: query}

	tokens := tokenize(query)
	if len(tokens) == 0 || cat == nil {
		res.missAsEmpty(cat)
		return res, nil
	}

	var all []SearchHit
	for i := range cat.Services {
		svc := &cat.Services[i]
		for j := range svc.Modules {
			mod := &svc.Modules[j]
			for k := range mod.Scenarios {
				e := &mod.Scenarios[k]
				score := scoreScenario(dir, e, tokens)
				if score <= 0 {
					continue
				}
				all = append(all, SearchHit{
					ID:      e.ID,
					Service: svc.Name,
					Module:  mod.Name,
					Title:   e.Title,
					Snippet: scenarioSnippet(e, loadScenarioBody(dir, e)),
					Score:   score,
				})
			}
		}
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].Score > all[b].Score })

	maxPerToken := weightKeywords * errorCodeBoost
	for _, h := range all {
		if len(res.Hits) >= topN {
			break
		}
		h.Score = h.Score / (float64(len(tokens)) * maxPerToken)
		if h.Score >= hitThreshold {
			res.Hits = append(res.Hits, h)
		}
	}
	if len(res.Hits) > 0 {
		return res, nil
	}

	// 零命中：区分检索词不匹配 / 领域未覆盖，并给出降级数据
	res.missAsEmpty(cat)
	for _, h := range all {
		h.Score = h.Score / (float64(len(tokens)) * maxPerToken)
		if len(res.WeakMatches) < weakTopN {
			res.WeakMatches = append(res.WeakMatches, h)
		}
	}
	return res, nil
}

// missAsEmpty 填充零命中时的元数据与指引
func (r *SearchResult) missAsEmpty(cat *Catalog) {
	r.Hits = []SearchHit{}
	r.MissKind = "domain_uncovered"
	if cat != nil {
		for _, svc := range cat.Services {
			r.CoveredServices = append(r.CoveredServices, svc.Name)
		}
		if r.Query != "" && domainCovered(cat, tokenize(r.Query)) {
			r.MissKind = "term_mismatch"
		}
	}
	// hint 只能引用已实现的接口：agent 会照做（实测中它真的调用了 hint 提及的命令）
	hint := "无强命中。可选：1) 换检索词重试，或用 knowledge list 浏览相关服务-模块的场景标题；2) 仍无命中则自行用 exec 只读命令取证排查。"
	if r.MissKind == "domain_uncovered" {
		hint = "知识库未覆盖该领域（covered_services 为已收录服务）。自行用 exec 只读命令取证排查。"
	}
	r.Hint = hint
}

// domainCovered 判断检索词是否与任一 服务名/模块名/组件 有重叠（忽略大小写，含子串）
func domainCovered(cat *Catalog, tokens []string) bool {
	var names []string
	for _, svc := range cat.Services {
		names = append(names, svc.Name)
		for _, mod := range svc.Modules {
			names = append(names, mod.Name)
			for _, e := range mod.Scenarios {
				names = append(names, e.Components...)
			}
		}
	}
	for i := range names {
		names[i] = strings.ToLower(names[i])
	}
	for _, t := range tokens {
		tl := strings.ToLower(t)
		for _, n := range names {
			if strings.Contains(n, tl) || strings.Contains(tl, n) {
				return true
			}
		}
	}
	return false
}

func tokenize(query string) []string {
	fields := strings.Fields(query)
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			tokens = append(tokens, f)
		}
	}
	return tokens
}

// scoreScenario 计算单场景的原始加权和（未归一化）
func scoreScenario(dir string, e *ScenarioEntry, tokens []string) float64 {
	body := loadScenarioBody(dir, e)
	joined := strings.Join(e.Keywords, "\x00")
	bodyLower := strings.ToLower(body)

	var sum float64
	for _, tok := range tokens {
		isErrorCode := errorCodeRe.MatchString(tok)
		tl := strings.ToLower(tok)
		switch {
		case containsToken(joined, tl):
			w := weightKeywords
			if isErrorCode {
				w *= errorCodeBoost
			}
			sum += w
		case containsToken(e.Title, tl):
			sum += weightTitle
		case containsToken(e.Phenomena, tl):
			sum += weightPhenomena
		case containsToken(strings.Join(e.Components, "\x00"), tl):
			sum += weightComponents
		case bodyLower != "" && strings.Contains(bodyLower, tl):
			sum += weightBody
		}
	}
	return sum
}

// containsToken 大小写不敏感的包含匹配；中文 token 无需分词，子串命中即算
func containsToken(field, tokenLower string) bool {
	if field == "" || tokenLower == "" {
		return false
	}
	return strings.Contains(strings.ToLower(field), tokenLower)
}

// loadScenarioBody 读取场景正文（File 的 LineStart..LineEnd 行）；读失败按空处理
func loadScenarioBody(dir string, e *ScenarioEntry) string {
	if dir == "" || e.File == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, e.File))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	start := e.LineStart - 1
	if start < 0 {
		start = 0
	}
	if start >= len(lines) {
		return ""
	}
	end := e.LineEnd - 1
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

// scenarioSnippet 摘要：优先现象描述，其次正文前缀
func scenarioSnippet(e *ScenarioEntry, body string) string {
	s := strings.TrimSpace(e.Phenomena)
	if s == "" {
		s = strings.TrimSpace(body)
	}
	r := []rune(s)
	if len(r) > snippetRunes {
		return string(r[:snippetRunes]) + "…"
	}
	return s
}
