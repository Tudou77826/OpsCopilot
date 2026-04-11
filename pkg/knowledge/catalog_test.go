package knowledge

import (
	"fmt"
	"strings"
	"testing"
)

func buildTestCatalog() *Catalog {
	return &Catalog{
		Version: 1,
		Services: []ServiceEntry{
			{
				Name: "Payment Service",
				Modules: []ModuleEntry{
					{
						Name: "核心支付模块",
						Scenarios: []ScenarioEntry{
							{
								Title:      "API接口超时(504)",
								File:       "payment_system_sop.md",
								LineStart:  12,
								LineEnd:    31,
								Phenomena:  "前端超时,Nginx大量504",
								Keywords:   []string{"504", "timeout", "超时", "接口超时"},
								Components: []string{"Nginx", "Core Service"},
								Type:       "sop",
							},
							{
								Title:      "订单状态未流转",
								File:       "payment_system_sop.md",
								LineStart:  33,
								LineEnd:    44,
								Phenomena:  "支付成功但PENDING",
								Keywords:   []string{"PENDING", "状态", "未流转"},
								Components: []string{"callback-worker", "Redis"},
								Type:       "sop",
							},
						},
					},
					{
						Name: "回调模块",
						Scenarios: []ScenarioEntry{
							{
								Title:      "回调超时",
								File:       "payment_system_sop.md",
								LineStart:  45,
								LineEnd:    55,
								Phenomena:  "商户未收到回调",
								Keywords:   []string{"callback", "回调", "超时"},
								Components: []string{"callback-worker"},
								Type:       "sop",
							},
						},
					},
				},
			},
			{
				Name: "Network Service",
				Modules: []ModuleEntry{
					{
						Name: "基础网络",
						Scenarios: []ScenarioEntry{
							{
								Title:      "无法连接公网",
								File:       "network_troubleshooting.md",
								LineStart:  14,
								LineEnd:    19,
								Phenomena:  "ping 8.8.8.8 超时",
								Keywords:   []string{"ping", "公网", "无法上网"},
								Components: []string{"网卡", "路由器"},
								Type:       "sop",
							},
						},
					},
				},
			},
		},
		FileHash: map[string]string{
			"payment_system_sop.md":     "abc123",
			"network_troubleshooting.md": "def456",
		},
	}
}

func TestCatalogTotalScenarios(t *testing.T) {
	cat := buildTestCatalog()
	total := cat.TotalScenarios()
	if total != 4 {
		t.Errorf("TotalScenarios() = %d, want 4", total)
	}

	// 空 catalog
	empty := &Catalog{}
	if empty.TotalScenarios() != 0 {
		t.Errorf("empty catalog TotalScenarios() = %d, want 0", empty.TotalScenarios())
	}
}

func TestCatalogFindEntry(t *testing.T) {
	cat := buildTestCatalog()

	tests := []struct {
		name     string
		filePath string
		title    string
		want     string // expected title, empty means nil
	}{
		{
			name:     "精确匹配",
			filePath: "payment_system_sop.md",
			title:    "API接口超时(504)",
			want:     "API接口超时(504)",
		},
		{
			name:     "模糊匹配-部分标题",
			filePath: "payment_system_sop.md",
			title:    "API接口超时",
			want:     "API接口超时(504)",
		},
		{
			name:     "模糊匹配-entry包含在title中",
			filePath: "payment_system_sop.md",
			title:    "场景一：API接口超时(504)",
			want:     "API接口超时(504)",
		},
		{
			name:     "大小写不敏感",
			filePath: "network_troubleshooting.md",
			title:    "无法连接公网",
			want:     "无法连接公网",
		},
		{
			name:     "不匹配-错误文件",
			filePath: "other_file.md",
			title:    "API接口超时(504)",
			want:     "",
		},
		{
			name:     "不匹配-错误标题",
			filePath: "payment_system_sop.md",
			title:    "不存在的场景",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := cat.FindEntry(tt.filePath, tt.title)
			if tt.want == "" {
				if entry != nil {
					t.Errorf("FindEntry(%q, %q) = %q, want nil", tt.filePath, tt.title, entry.Title)
				}
			} else {
				if entry == nil {
					t.Errorf("FindEntry(%q, %q) = nil, want %q", tt.filePath, tt.title, tt.want)
				} else if entry.Title != tt.want {
					t.Errorf("FindEntry(%q, %q) = %q, want %q", tt.filePath, tt.title, entry.Title, tt.want)
				}
			}
		})
	}
}

func TestCatalogRenderForLLM(t *testing.T) {
	cat := buildTestCatalog()
	rendered := cat.RenderForLLM()

	// 验证关键结构
	if !strings.Contains(rendered, "## Payment Service (2 modules, 3 scenarios)") {
		t.Error("RenderForLLM missing Payment Service header with counts")
	}
	if !strings.Contains(rendered, "### 核心支付模块 (2 scenarios)") {
		t.Error("RenderForLLM missing 核心支付模块 header with count")
	}
	if !strings.Contains(rendered, "### 回调模块 (1 scenario)") {
		t.Error("RenderForLLM missing 回调模块 header with count")
	}
	if !strings.Contains(rendered, "## Network Service (1 module, 1 scenario)") {
		t.Error("RenderForLLM missing Network Service header with counts")
	}

	// 验证场景条目格式
	if !strings.Contains(rendered, "- API接口超时(504) | 前端超时,Nginx大量504 | 504,timeout,超时,接口超时 | payment_system_sop.md#L12") {
		t.Error("RenderForLLM missing or malformed API接口超时 entry")
	}
	if !strings.Contains(rendered, "- 订单状态未流转 | 支付成功但PENDING | PENDING,状态,未流转 | payment_system_sop.md#L33") {
		t.Error("RenderForLLM missing or malformed 订单状态未流转 entry")
	}
	if !strings.Contains(rendered, "- 无法连接公网 | ping 8.8.8.8 超时 | ping,公网,无法上网 | network_troubleshooting.md#L14") {
		t.Error("RenderForLLM missing or malformed 无法连接公网 entry")
	}
}

func TestCatalogRenderForLLMEmpty(t *testing.T) {
	cat := &Catalog{}
	rendered := cat.RenderForLLM()
	if rendered != "" {
		t.Errorf("empty catalog RenderForLLM() = %q, want empty string", rendered)
	}
}

func TestCatalogFindEntryLocation(t *testing.T) {
	cat := buildTestCatalog()

	// 查找已知条目
	entry := cat.FindEntry("payment_system_sop.md", "API接口超时")
	if entry == nil {
		t.Fatal("FindEntry returned nil for known entry")
	}

	loc := cat.FindEntryLocation(entry)
	expected := "Payment Service › 核心支付模块 › API接口超时(504)"
	if loc != expected {
		t.Errorf("FindEntryLocation() = %q, want %q", loc, expected)
	}

	// 查找另一个服务的条目
	entry2 := cat.FindEntry("network_troubleshooting.md", "无法连接公网")
	if entry2 == nil {
		t.Fatal("FindEntry returned nil for network entry")
	}
	loc2 := cat.FindEntryLocation(entry2)
	expected2 := "Network Service › 基础网络 › 无法连接公网"
	if loc2 != expected2 {
		t.Errorf("FindEntryLocation() = %q, want %q", loc2, expected2)
	}
}

func TestCatalogReplaceEntries(t *testing.T) {
	cat := &Catalog{
		FileHash: make(map[string]string),
	}

	// 第一次替换：添加新文件的条目
	entries := []ScenarioEntry{
		{
			Title:     "场景A",
			File:      "a.md",
			LineStart: 10,
			LineEnd:   20,
			Phenomena: "现象A",
			Keywords:  []string{"ka"},
			Type:      "sop",
		},
	}
	cat.ReplaceEntries("a.md", "Service A", "Module A", entries)

	if cat.TotalScenarios() != 1 {
		t.Fatalf("after ReplaceEntries, TotalScenarios = %d, want 1", cat.TotalScenarios())
	}

	// 验证 Service 和 Module 结构
	if len(cat.Services) != 1 || cat.Services[0].Name != "Service A" {
		t.Errorf("service name = %q, want 'Service A'", cat.Services[0].Name)
	}
	if len(cat.Services[0].Modules) != 1 || cat.Services[0].Modules[0].Name != "Module A" {
		t.Errorf("module name = %q, want 'Module A'", cat.Services[0].Modules[0].Name)
	}

	// 第二次替换：更新同一文件的条目（先删旧再插新）
	newEntries := []ScenarioEntry{
		{
			Title:     "场景A-V2",
			File:      "a.md",
			LineStart: 10,
			LineEnd:   25,
			Phenomena: "现象A-V2",
			Keywords:  []string{"ka", "ka2"},
			Type:      "sop",
		},
		{
			Title:     "场景B",
			File:      "a.md",
			LineStart: 30,
			LineEnd:   40,
			Phenomena: "现象B",
			Keywords:  []string{"kb"},
			Type:      "sop",
		},
	}
	cat.ReplaceEntries("a.md", "Service A", "Module A", newEntries)

	if cat.TotalScenarios() != 2 {
		t.Fatalf("after second ReplaceEntries, TotalScenarios = %d, want 2", cat.TotalScenarios())
	}

	// 验证旧条目被移除
	found := cat.FindEntry("a.md", "场景A-V2")
	if found == nil {
		t.Error("FindEntry after replace should find 场景A-V2")
	}
	foundOld := cat.FindEntry("a.md", "场景A")
	// "场景A" 是 "场景A-V2" 的子串，模糊匹配能命中是合理的
	if foundOld != nil && foundOld.Title == "场景A" {
		t.Error("FindEntry should NOT find exact old 场景A after replace")
	}

	// 第三次替换：不同文件的条目，加到新的 service
	otherEntries := []ScenarioEntry{
		{
			Title:     "场景C",
			File:      "b.md",
			LineStart: 5,
			LineEnd:   10,
			Phenomena: "现象C",
			Keywords:  []string{"kc"},
			Type:      "sop",
		},
	}
	cat.ReplaceEntries("b.md", "Service B", "Module B", otherEntries)

	if cat.TotalScenarios() != 3 {
		t.Fatalf("after third ReplaceEntries, TotalScenarios = %d, want 3", cat.TotalScenarios())
	}
	if len(cat.Services) != 2 {
		t.Errorf("len(Services) = %d, want 2", len(cat.Services))
	}
}

func TestCatalogRemoveDeleted(t *testing.T) {
	cat := buildTestCatalog()

	// 只保留 payment_system_sop.md
	existing := map[string]bool{
		"payment_system_sop.md": true,
	}

	cat.RemoveDeletedFiles(existing)

	// Network Service 应该被完全清理
	if cat.TotalScenarios() != 3 {
		t.Errorf("after RemoveDeletedFiles, TotalScenarios = %d, want 3", cat.TotalScenarios())
	}
	for _, svc := range cat.Services {
		if svc.Name == "Network Service" {
			t.Error("Network Service should have been removed")
		}
	}

	// FileHash 中 network 文件应被清理
	if _, ok := cat.FileHash["network_troubleshooting.md"]; ok {
		t.Error("FileHash for network_troubleshooting.md should have been removed")
	}
	// payment 文件应保留
	if _, ok := cat.FileHash["payment_system_sop.md"]; !ok {
		t.Error("FileHash for payment_system_sop.md should be kept")
	}
}

func TestCatalogRemoveDeletedAll(t *testing.T) {
	cat := buildTestCatalog()

	// 所有文件都删除
	existing := map[string]bool{}

	cat.RemoveDeletedFiles(existing)

	if cat.TotalScenarios() != 0 {
		t.Errorf("after removing all files, TotalScenarios = %d, want 0", cat.TotalScenarios())
	}
	if len(cat.Services) != 0 {
		t.Errorf("after removing all files, len(Services) = %d, want 0", len(cat.Services))
	}
}

func TestCatalogReplaceEntriesSameServiceDifferentModule(t *testing.T) {
	cat := &Catalog{
		FileHash: make(map[string]string),
	}

	entries1 := []ScenarioEntry{
		{Title: "S1", File: "a.md", LineStart: 1, LineEnd: 5, Keywords: []string{"k1"}, Type: "sop"},
	}
	cat.ReplaceEntries("a.md", "Svc", "ModA", entries1)

	entries2 := []ScenarioEntry{
		{Title: "S2", File: "b.md", LineStart: 1, LineEnd: 5, Keywords: []string{"k2"}, Type: "sop"},
	}
	cat.ReplaceEntries("b.md", "Svc", "ModB", entries2)

	if len(cat.Services) != 1 {
		t.Fatalf("len(Services) = %d, want 1", len(cat.Services))
	}
	if len(cat.Services[0].Modules) != 2 {
		t.Errorf("len(Modules) = %d, want 2", len(cat.Services[0].Modules))
	}
	if cat.TotalScenarios() != 2 {
		t.Errorf("TotalScenarios = %d, want 2", cat.TotalScenarios())
	}
}

func TestCatalogRenderForLLMCompression(t *testing.T) {
	// 构造完整渲染超 25K、压缩渲染不超的目录，验证降级到压缩级别
	cat := &Catalog{}
	// 150 条：完整 ~31K 超 25K，压缩 ~18K 不超
	entries := make([]ScenarioEntry, 150)
	for i := 0; i < 150; i++ {
		entries[i] = ScenarioEntry{
			Title:      fmt.Sprintf("场景%d-这是一个比较长的场景标题用于增加字节数", i),
			File:       "large_file.md",
			LineStart:  i*10 + 1,
			LineEnd:    i*10 + 10,
			Phenomena:  fmt.Sprintf("这是场景%d的现象描述，包含一些详细的信息用于增加总字节数量", i),
			Keywords:   []string{"keyword1", "keyword2", "keyword3"},
			Components: []string{"Component1", "Component2"},
			Type:       "sop",
		}
	}
	cat.ReplaceEntries("large_file.md", "Large Service", "Large Module", entries)

	// renderFull 应该超过 25K
	full := cat.renderFull()
	if len(full) <= maxCatalogBytes {
		t.Errorf("renderFull = %d bytes, expected > %d for this test", len(full), maxCatalogBytes)
	}

	// RenderForLLM 应该自动降级到压缩级别
	rendered := cat.RenderForLLM()
	if len(rendered) > maxCatalogBytes {
		t.Errorf("RenderForLLM = %d bytes, expected <= %d (should have degraded)", len(rendered), maxCatalogBytes)
	}

	// 压缩级别应包含服务名和标题（不含 phenomena）
	if !strings.Contains(rendered, "Large Service") {
		t.Error("degraded RenderForLLM should still contain service name")
	}
	if !strings.Contains(rendered, "场景0-") {
		t.Error("compressed level should contain scenario titles")
	}
	// 压缩级别不应包含 phenomena 内容
	if strings.Contains(rendered, "现象描述") {
		t.Error("compressed level should NOT contain phenomena text")
	}
}

func TestCatalogRenderForLLMModuleLevel(t *testing.T) {
	// 构造即使压缩也超 25K 的极端目录，迫使降到模块级
	cat := &Catalog{}
	// 每个服务 5 个模块，每个模块 10 个场景 → 总共大量条目
	for i := 0; i < 100; i++ {
		svcName := fmt.Sprintf("Service-%04d-With-A-Long-Name-To-Increase-Size", i)
		for j := 0; j < 5; j++ {
			modName := fmt.Sprintf("Module-%s-%d-With-Long-Name", svcName, j)
			entries := make([]ScenarioEntry, 10)
			for k := 0; k < 10; k++ {
				entries[k] = ScenarioEntry{
					Title:     fmt.Sprintf("场景-%s-%d-%d-标题很长用于增加字节数以达到压缩级别也超标", svcName, j, k),
					File:      "big.md",
					LineStart: 1,
					LineEnd:   10,
					Phenomena: "这是一个很长的现象描述用来测试目录大小限制是否正常工作不会出现溢出问题",
					Keywords:  []string{"kw1", "kw2", "kw3", "kw4", "kw5"},
					Type:      "sop",
				}
			}
			cat.ReplaceEntries("big.md", svcName, modName, entries)
		}
	}

	rendered := cat.RenderForLLM()
	if len(rendered) > maxCatalogBytes {
		t.Errorf("RenderForLLM = %d bytes, expected <= %d (should degrade to module level)", len(rendered), maxCatalogBytes)
	}

	// 模块级别应该只包含服务名和模块名，不包含 | 分隔的场景条目
	if strings.Contains(rendered, " | ") {
		t.Error("module-level render should not contain pipe-separated scenario entries")
	}
}
