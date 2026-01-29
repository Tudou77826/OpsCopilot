package knowledge

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type SearchHit struct {
	Path     string   `json:"path"`
	Score    float64  `json:"score"`
	Snippets []string `json:"snippets"`
}

type WeightedTerm struct {
	Term   string  `json:"term"`
	Weight float64 `json:"weight"`
}

type searchChunk struct {
	path    string
	title   string
	content string
}

func Search(dir string, query string, topK int) ([]SearchHit, error) {
	q := strings.TrimSpace(query)
	terms := make([]WeightedTerm, 0, 8)
	for _, t := range tokenizeQuery(q) {
		terms = append(terms, WeightedTerm{Term: t, Weight: 1})
	}
	return SearchWithTerms(dir, query, terms, topK)
}

func SearchWithTerms(dir string, query string, terms []WeightedTerm, topK int) ([]SearchHit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []SearchHit{}, nil
	}
	if topK <= 0 {
		topK = 5
	}
	if topK > 20 {
		topK = 20
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []SearchHit{}, err
	}

	queryLower := strings.ToLower(q)
	normalized := normalizeTerms(terms)

	var chunks []searchChunk
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(dir, path)
		rel := filepath.ToSlash(relPath)
		chunks = append(chunks, chunkMarkdown(rel, string(raw))...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	type docAgg struct {
		path     string
		score    float64
		snippets []string
	}
	byPath := map[string]*docAgg{}

	for _, c := range chunks {
		score, snippet := scoreChunk(c, normalized, queryLower)
		if score <= 0 {
			continue
		}
		a, ok := byPath[c.path]
		if !ok {
			a = &docAgg{path: c.path}
			byPath[c.path] = a
		}
		a.score += score
		if snippet != "" && len(a.snippets) < 3 {
			exists := false
			for _, s := range a.snippets {
				if s == snippet {
					exists = true
					break
				}
			}
			if !exists {
				a.snippets = append(a.snippets, snippet)
			}
		}
	}

	hits := make([]SearchHit, 0, len(byPath))
	for _, a := range byPath {
		hits = append(hits, SearchHit{
			Path:     a.path,
			Score:    a.score,
			Snippets: a.snippets,
		})
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Score > hits[j].Score
	})

	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

func normalizeTerms(terms []WeightedTerm) []WeightedTerm {
	seen := map[string]float64{}
	for _, t := range terms {
		term := strings.ToLower(strings.TrimSpace(t.Term))
		if term == "" {
			continue
		}
		w := t.Weight
		if w <= 0 {
			w = 1
		}
		if w > 5 {
			w = 5
		}
		if cur, ok := seen[term]; !ok || w > cur {
			seen[term] = w
		}
	}
	out := make([]WeightedTerm, 0, len(seen))
	for term, w := range seen {
		if len([]rune(term)) == 1 {
			continue
		}
		out = append(out, WeightedTerm{Term: term, Weight: w})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight == out[j].Weight {
			return out[i].Term < out[j].Term
		}
		return out[i].Weight > out[j].Weight
	})
	return out
}

func tokenizeQuery(q string) []string {
	parts := strings.FieldsFunc(q, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	seen := map[string]struct{}{}
	var tokens []string
	for _, p := range parts {
		t := strings.ToLower(strings.TrimSpace(p))
		if t == "" {
			continue
		}
		if len([]rune(t)) == 1 {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		tokens = append(tokens, t)
	}
	if len(tokens) == 0 {
		tokens = []string{strings.ToLower(q)}
	}
	return tokens
}

func chunkMarkdown(path string, content string) []searchChunk {
	lines := strings.Split(content, "\n")
	title := ""
	var sb strings.Builder
	var chunks []searchChunk

	flush := func() {
		txt := strings.TrimSpace(sb.String())
		if txt == "" {
			sb.Reset()
			return
		}
		chunks = append(chunks, searchChunk{
			path:    path,
			title:   title,
			content: txt,
		})
		sb.Reset()
	}

	const maxChunkChars = 900
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			flush()
			title = strings.TrimSpace(strings.TrimLeft(trim, "#"))
			continue
		}
		if trim == "" && sb.Len() >= 400 {
			flush()
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
		if sb.Len() >= maxChunkChars {
			flush()
		}
	}
	flush()
	return chunks
}

func scoreChunk(c searchChunk, terms []WeightedTerm, queryLower string) (float64, string) {
	textLower := strings.ToLower(c.content)
	titleLower := strings.ToLower(c.title)
	pathLower := strings.ToLower(c.path)

	var score float64
	bestIdx := -1
	bestTok := ""

	if idx := strings.Index(textLower, queryLower); idx >= 0 {
		score += 6
		bestIdx = idx
		bestTok = queryLower
	}

	for _, wt := range terms {
		t := wt.Term
		if t == "" || wt.Weight <= 0 {
			continue
		}
		w := wt.Weight
		cnt := strings.Count(textLower, t)
		if cnt > 0 {
			if cnt > 3 {
				cnt = 3
			}
			score += float64(cnt) * 2.0 * w
			if bestIdx < 0 {
				if idx := strings.Index(textLower, t); idx >= 0 {
					bestIdx = idx
					bestTok = t
				}
			}
		}
		if strings.Contains(titleLower, t) {
			score += 3 * w
		}
		if strings.Contains(pathLower, t) {
			score += 1 * w
		}
	}

	if score <= 0 {
		return 0, ""
	}

	snippet := ""
	if bestIdx >= 0 && bestTok != "" {
		start := bestIdx - 120
		if start < 0 {
			start = 0
		}
		end := bestIdx + 360
		if end > len(c.content) {
			end = len(c.content)
		}
		snippet = strings.TrimSpace(c.content[start:end])
	} else {
		if len(c.content) > 360 {
			snippet = strings.TrimSpace(c.content[:360])
		} else {
			snippet = strings.TrimSpace(c.content)
		}
	}
	if c.title != "" {
		snippet = strings.TrimSpace(c.title) + " — " + snippet
	}
	return score, snippet
}
