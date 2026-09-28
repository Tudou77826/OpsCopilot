package knowledge

import (
	"os"
	"strings"
	"testing"
)

func buildTestdataCatalog(t *testing.T) (*Catalog, string) {
	t.Helper()
	dir := "testdata"
	cat, err := BuildCatalog(dir)
	if err != nil {
		t.Fatalf("BuildCatalog error: %v", err)
	}
	return cat, dir
}

func TestScenarioIDDeterministic(t *testing.T) {
	a := ScenarioID("支付服务", "核心模块", "接口超时")
	b := ScenarioID("支付服务", "核心模块", "接口超时")
	if a != b {
		t.Fatalf("同输入应同 ID: %s != %s", a, b)
	}
	if len(a) != 10 {
		t.Fatalf("ID 长度应为 10: %s", a)
	}
	c := ScenarioID("支付服务", "核心模块", "订单未流转")
	if a == c {
		t.Fatal("不同标题应产生不同 ID")
	}
	d := ScenarioID("网关服务", "核心模块", "接口超时")
	if a == d {
		t.Fatal("不同服务应产生不同 ID")
	}
}

func TestBuildCatalogAssignsIDs(t *testing.T) {
	cat, _ := buildTestdataCatalog(t)
	for _, svc := range cat.Services {
		for _, mod := range svc.Modules {
			for _, e := range mod.Scenarios {
				if e.ID == "" {
					t.Fatalf("场景 %s/%s/%s 缺少 ID", svc.Name, mod.Name, e.Title)
				}
				if want := ScenarioID(svc.Name, mod.Name, e.Title); e.ID != want {
					t.Fatalf("场景 %s ID=%s, 期望 %s", e.Title, e.ID, want)
				}
			}
		}
	}
}

func TestFindByID(t *testing.T) {
	cat, _ := buildTestdataCatalog(t)
	var first *ScenarioEntry
	for _, svc := range cat.Services {
		for _, mod := range svc.Modules {
			for i := range mod.Scenarios {
				first = &mod.Scenarios[i]
			}
		}
	}
	if first == nil {
		t.Fatal("testdata 应有场景")
	}
	loc := cat.FindByID(first.ID)
	if loc == nil || loc.Entry.ID != first.ID {
		t.Fatalf("FindByID(%s) 未命中", first.ID)
	}
	if cat.FindByID("nonexist0") != nil {
		t.Fatal("不存在的 ID 不应命中")
	}
}

func TestSearchHitByErrorCode(t *testing.T) {
	cat, dir := buildTestdataCatalog(t)
	res, err := Search(dir, cat, "支付 504 超时", 5)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(res.Hits) == 0 {
		t.Fatalf("应命中支付 504 场景, got %+v", res)
	}
	// testdata 中 SOP 与 archive 各有一条 504 场景，三词全中的排第一
	found504 := false
	for _, h := range res.Hits {
		if strings.Contains(h.Title, "504") || strings.Contains(h.Snippet, "504") {
			found504 = true
			break
		}
	}
	if !found504 {
		t.Fatalf("命中里应存在 504 场景: %+v", res.Hits)
	}
	if res.Hits[0].ID == "" || res.Hits[0].Snippet == "" {
		t.Fatalf("命中应带 ID 与摘要: %+v", res.Hits[0])
	}
}

func TestSearchZeroHitTermMismatch(t *testing.T) {
	cat, dir := buildTestdataCatalog(t)
	// "Network" 与服务名重叠但无场景命中
	res, err := Search(dir, cat, "Network E999", 5)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(res.Hits) != 0 {
		t.Fatalf("应零命中, got %+v", res.Hits)
	}
	if res.MissKind != "term_mismatch" {
		t.Fatalf("服务名有重叠应为 term_mismatch, got %s", res.MissKind)
	}
	if len(res.CoveredServices) == 0 {
		t.Fatal("零命中应返回 covered_services")
	}
	if res.Hint == "" || len(res.WeakMatches) > weakTopN {
		t.Fatalf("零命中应带 hint 且弱命中不超过 %d: %+v", weakTopN, res)
	}
}

func TestSearchZeroHitDomainUncovered(t *testing.T) {
	cat, dir := buildTestdataCatalog(t)
	res, err := Search(dir, cat, "量子计算机崩溃", 5)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if res.MissKind != "domain_uncovered" {
		t.Fatalf("无任何重叠应为 domain_uncovered, got %s", res.MissKind)
	}
	if strings.Contains(res.Hint, "list") {
		t.Fatalf("domain_uncovered 的 hint 不应建议浏览知识库: %s", res.Hint)
	}
}

func TestReadScenario(t *testing.T) {
	cat, dir := buildTestdataCatalog(t)
	var e *ScenarioEntry
	for _, svc := range cat.Services {
		for _, mod := range svc.Modules {
			for i := range mod.Scenarios {
				if strings.Contains(mod.Scenarios[i].Title, "504") {
					e = &mod.Scenarios[i]
				}
			}
		}
	}
	if e == nil {
		t.Fatal("testdata 应有 504 场景")
	}
	r, err := ReadScenario(dir, e, 32*1024)
	if err != nil {
		t.Fatalf("ReadScenario error: %v", err)
	}
	if !strings.Contains(r.Content, "504") || !strings.Contains(r.Content, "排查") {
		t.Fatalf("场景内容应含 504 排查步骤:\n%s", r.Content)
	}
	if r.Truncated {
		t.Fatal("testdata 场景不应触发截断")
	}

	small, err := ReadScenario(dir, e, 20)
	if err != nil {
		t.Fatalf("ReadScenario error: %v", err)
	}
	if !small.Truncated {
		t.Fatal("小上限应触发截断")
	}
}

func TestEnsureScenarioIDsConflict(t *testing.T) {
	cat := &Catalog{
		Services: []ServiceEntry{{
			Name: "S",
			Modules: []ModuleEntry{{
				Name: "M",
				Scenarios: []ScenarioEntry{
					{Title: "T", ID: ScenarioID("S", "M", "T")},
					{Title: "T"}, // 同三元组 → 冲突
				},
			}},
		}},
	}
	if err := cat.EnsureScenarioIDs(); err == nil {
		t.Fatal("重复三元组应报 ID 冲突")
	}
}

func TestSearchEmptyCatalog(t *testing.T) {
	res, err := Search("testdata", &Catalog{}, "任意问题", 5)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(res.Hits) != 0 || res.MissKind != "domain_uncovered" {
		t.Fatalf("空目录应零命中且 domain_uncovered: %+v", res)
	}
}

func TestReadScenarioLineBounds(t *testing.T) {
	dir, err := os.MkdirTemp("", "knread")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	content := "# 标题\n\n## 场景：甲\n- **现象**: 甲现象\n\n## 场景：乙\n- **现象**: 乙现象\n"
	if err := os.WriteFile(dir+"/a.md", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// 无 front matter（偏移 0）：场景乙标题在文件第 6 行，内容到第 7 行
	// （LineStart=标题行、LineEnd=最后内容行，均 1-based）
	r, err := ReadScenario(dir, &ScenarioEntry{File: "a.md", LineStart: 6, LineEnd: 7}, 32*1024)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.Content, "甲现象") || !strings.Contains(r.Content, "乙现象") {
		t.Fatalf("应只含场景乙内容: %q", r.Content)
	}
}

func TestReadScenarioFrontMatterOffset(t *testing.T) {
	dir, err := os.MkdirTemp("", "knfm")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	content := "---\nservice: S\nmodule: M\n---\n\n# 手册\n\n## 场景：甲\n- **现象**: 甲现象\n\n## 场景：乙\n- **现象**: 乙现象\n"
	if err := os.WriteFile(dir+"/a.md", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// 正文基准行号：手册=正文行1，场景乙标题=正文行6、内容行7；文件绝对行号加偏移 5
	r, err := ReadScenario(dir, &ScenarioEntry{File: "a.md", LineStart: 6, LineEnd: 7}, 32*1024)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.Content, "service: S") || strings.Contains(r.Content, "甲现象") {
		t.Fatalf("不应包含 front matter 或场景甲: %q", r.Content)
	}
	if !strings.Contains(r.Content, "场景：乙") || !strings.Contains(r.Content, "乙现象") {
		t.Fatalf("应含场景乙标题与内容: %q", r.Content)
	}

	if got := BodyLineOffset(content); got != 5 {
		t.Fatalf("BodyLineOffset = %d, want 5", got)
	}
	if got := BodyLineOffset("plain doc\n## 场景：x\n"); got != 0 {
		t.Fatalf("无 front matter 偏移应为 0, got %d", got)
	}
	if got := BodyLineOffset("---\nunclosed front matter\n"); got != 0 {
		t.Fatalf("未闭合 front matter 偏移应为 0, got %d", got)
	}
}
