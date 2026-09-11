package xshellimport

import (
	"archive/zip"
	"bytes"
	"sort"
	"strings"
	"testing"

	"opscopilot/pkg/remote"
)

// ── .xts 解包 ───────────────────────────────────────────────

// buildXTS 在内存里组装一个 .xts 备份包（ZIP）：
// xts.zcf 元数据 + Xshell/<分组路径>/<会话>.xsh。
func buildXTS(t *testing.T, master bool, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	zcf := "[SessionInfo]\nVersion=8.0\n"
	if master {
		zcf += "Master=1\n"
	} else {
		zcf += "Master=0\n"
	}
	f, err := w.Create("xts.zcf")
	if err != nil {
		t.Fatalf("创建 zcf 失败: %v", err)
	}
	if _, err := f.Write(utf16leBOM(t, zcf)); err != nil {
		t.Fatalf("写入 zcf 失败: %v", err)
	}

	// 按名排序保证 ZIP 内容稳定，便于排查。
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("创建成员 %s 失败: %v", name, err)
		}
		if _, err := f.Write(entries[name]); err != nil {
			t.Fatalf("写入成员 %s 失败: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
	return buf.Bytes()
}

func TestParseXTS_GroupPathAndRecords(t *testing.T) {
	entries := map[string][]byte{
		"Xshell/生产/华东/web-1.xsh": buildXSH(t, "SSH", "10.0.0.21", "22", "root", "", "", ""),
		"Xshell/生产/db-1.xsh":     buildXSH(t, "SSH", "10.0.0.22", "22", "root", "", "", ""),
		"Xshell/root-1.xsh":      buildXSH(t, "SSH", "10.0.0.23", "22", "root", "", "", ""),
		"Xshell/notes.txt":       []byte("not a session"),
	}

	records, warnings, err := ParseXTS(buildXTS(t, false, entries), ParseOptions{})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("不应有警告，实际 %v", warnings)
	}
	if len(records) != 3 {
		t.Fatalf("应解析出 3 条会话（.txt 不算），实际 %d", len(records))
	}

	byName := map[string]*SessionRecord{}
	for _, rec := range records {
		byName[rec.Name] = rec
	}
	if got := strings.Join(byName["web-1"].GroupPath, "/"); got != "生产/华东" {
		t.Errorf("多层分组路径解析错误: %q", got)
	}
	if got := strings.Join(byName["db-1"].GroupPath, "/"); got != "生产" {
		t.Errorf("单层分组路径解析错误: %q", got)
	}
	if len(byName["root-1"].GroupPath) != 0 {
		t.Errorf("根会话不应有分组，实际 %v", byName["root-1"].GroupPath)
	}
}

func TestParseXTS_CompatibleWithSessionsPrefix(t *testing.T) {
	entries := map[string][]byte{
		"Xshell/Sessions/生产/web-1.xsh": buildXSH(t, "SSH", "10.0.0.31", "22", "root", "", "", ""),
	}
	records, _, err := ParseXTS(buildXTS(t, false, entries), ParseOptions{})
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("应解析出 1 条，实际 %d", len(records))
	}
	if got := strings.Join(records[0].GroupPath, "/"); got != "生产" {
		t.Errorf("Sessions/ 前缀应被剥离，实际分组 %q", got)
	}
}

// 整包主密码加密时给出可操作的指引，而不是抛一个底层 ZIP 错误。
func TestParseXTS_MasterEncryptedReportsActionableError(t *testing.T) {
	entries := map[string][]byte{
		"Xshell/web-1.xsh": buildXSH(t, "SSH", "10.0.0.41", "22", "root", "", "", ""),
	}
	_, _, err := ParseXTS(buildXTS(t, true, entries), ParseOptions{})
	if err == nil {
		t.Fatal("主密码加密的包应报错")
	}
	if !strings.Contains(err.Error(), "主密码") || !strings.Contains(err.Error(), "会话目录") {
		t.Errorf("错误信息应说明原因并给出替代路径，实际: %v", err)
	}
}

func TestParseXTS_InvalidArchive(t *testing.T) {
	if _, _, err := ParseXTS([]byte("definitely not a zip"), ParseOptions{}); err == nil {
		t.Error("非 ZIP 内容应报错")
	}
}

// ── 导入计划 ────────────────────────────────────────────────

// fakeWriter 记录导入器的写入调用，用来验证 Analyze/Apply 的语义。
type fakeWriter struct {
	existing  map[string]bool
	added     []*SessionRecord
	addedTo   []string
	failOnAdd string
}

func newFakeWriter(existing ...string) *fakeWriter {
	w := &fakeWriter{existing: make(map[string]bool)}
	for _, e := range existing {
		w.existing[e] = true
	}
	return w
}

func (w *fakeWriter) EnsureFolderPath(names []string) (string, error) {
	return strings.Join(names, "/"), nil
}

func (w *fakeWriter) HasEndpoint(protocol, host string, port int) bool {
	return w.existing[endpointKey(protocol, host, port)]
}

func (w *fakeWriter) AddConnection(rec *SessionRecord, parentID string) error {
	if w.failOnAdd != "" && rec.Name == w.failOnAdd {
		return errFake
	}
	w.added = append(w.added, rec)
	w.addedTo = append(w.addedTo, parentID)
	return nil
}

var errFake = &fakeError{}

type fakeError struct{}

func (e *fakeError) Error() string { return "写入失败（测试）" }

func record(protocol, host string, port int, name string, group ...string) *SessionRecord {
	return &SessionRecord{Protocol: protocol, Host: host, Port: port, Name: name, GroupPath: group}
}

func TestAnalyze_Counts(t *testing.T) {
	records := []*SessionRecord{
		record(remote.ProtocolSSH, "10.0.0.1", 22, "a", "生产"),
		record(remote.ProtocolSSH, "10.0.0.2", 22, "b", "生产"),
		record(remote.ProtocolTelnet, "10.0.0.3", 23, "c"),
		record("", "10.0.0.4", 22, "unsupported-proto"),
		record(remote.ProtocolSSH, "", 22, "missing-host"),
		record(remote.ProtocolSSH, "10.0.0.9", 22, "already-there"),
	}
	records[0].PasswordEncrypted = true
	records[0].Password = "pw"
	records[0].PasswordRule = "rule"
	records[1].PasswordEncrypted = true // 解密失败

	writer := newFakeWriter(endpointKey(remote.ProtocolSSH, "10.0.0.9", 22))
	a := Analyze(records, writer)

	if a.Total != 6 {
		t.Errorf("Total 应为 6，实际 %d", a.Total)
	}
	if a.Unsupported != 2 {
		t.Errorf("Unsupported 应为 2，实际 %d", a.Unsupported)
	}
	if a.Existing != 1 {
		t.Errorf("Existing 应为 1，实际 %d", a.Existing)
	}
	if a.Supported != 3 {
		t.Errorf("Supported 应为 3，实际 %d", a.Supported)
	}
	if a.WithPassword != 2 || a.PasswordDecrypted != 1 || a.PasswordFailed != 1 {
		t.Errorf("密码统计错误: %+v", a)
	}
	if a.CanDecryptAll() {
		t.Error("存在解密失败时 CanDecryptAll 应为 false")
	}
	if a.Groups != 1 {
		t.Errorf("Groups 应为 1，实际 %d", a.Groups)
	}
	// 协议分布描述的是导出源里的构成，包含将被跳过的已存在项，
	// 因此 ssh = 3（两条待导入 + 一条已存在），telnet = 1。
	if a.Protocols[remote.ProtocolSSH] != 3 || a.Protocols[remote.ProtocolTelnet] != 1 {
		t.Errorf("协议分布错误: %v", a.Protocols)
	}
}

func TestApply_SkipsExistingAndUnsupported(t *testing.T) {
	records := []*SessionRecord{
		record(remote.ProtocolSSH, "10.0.0.1", 22, "new-one", "生产", "华东"),
		record(remote.ProtocolSSH, "10.0.0.9", 22, "already-there"),
		record("", "10.0.0.4", 22, "unsupported"),
	}
	writer := newFakeWriter(endpointKey(remote.ProtocolSSH, "10.0.0.9", 22))

	report, err := Apply(records, writer, ConflictSkip)
	if err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	if report.Imported != 1 || report.SkippedExisting != 1 || report.SkippedUnsupported != 1 {
		t.Errorf("统计错误: %+v", report)
	}
	if len(writer.added) != 1 || writer.added[0].Name != "new-one" {
		t.Fatalf("只应写入 new-one，实际 %+v", writer.added)
	}
	if writer.addedTo[0] != "生产/华东" {
		t.Errorf("应落到解析出的分组，实际 %q", writer.addedTo[0])
	}
}

// 同一次导入内的重复端点只写一次——不能依赖 writer 的实时视图。
func TestApply_DedupesWithinRun(t *testing.T) {
	records := []*SessionRecord{
		record(remote.ProtocolSSH, "10.0.0.1", 22, "first"),
		record(remote.ProtocolSSH, "10.0.0.1", 22, "second"),
		record(remote.ProtocolSSH, "10.0.0.1", 2222, "different-port"),
	}
	writer := newFakeWriter()

	report, err := Apply(records, writer, ConflictSkip)
	if err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	if report.Imported != 2 {
		t.Errorf("同端点应去重，Imported 应为 2，实际 %d", report.Imported)
	}
	if report.SkippedExisting != 1 {
		t.Errorf("SkippedExisting 应为 1，实际 %d", report.SkippedExisting)
	}
}

// 单条写入失败不应中断整次导入。
func TestApply_ContinuesAfterAddFailure(t *testing.T) {
	records := []*SessionRecord{
		record(remote.ProtocolSSH, "10.0.0.1", 22, "bad"),
		record(remote.ProtocolSSH, "10.0.0.2", 22, "good"),
	}
	writer := newFakeWriter()
	writer.failOnAdd = "bad"

	report, err := Apply(records, writer, ConflictSkip)
	if err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	if report.Imported != 1 {
		t.Errorf("应继续导入后续条目，Imported=%d", report.Imported)
	}
	if !hasWarningContaining(report.Warnings, "写入失败") {
		t.Errorf("失败条目应进入警告，实际 %v", report.Warnings)
	}
}

func TestApply_PasswordStats(t *testing.T) {
	ok := record(remote.ProtocolSSH, "10.0.0.1", 22, "with-pw")
	ok.PasswordEncrypted, ok.Password = true, "pw"
	failed := record(remote.ProtocolSSH, "10.0.0.2", 22, "no-pw")
	failed.PasswordEncrypted = true

	report, err := Apply([]*SessionRecord{ok, failed}, newFakeWriter(), ConflictSkip)
	if err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	if report.PasswordDecrypted != 1 || report.PasswordFailed != 1 {
		t.Errorf("密码统计错误: %+v", report)
	}
}

func TestApply_RejectsUnsupportedPolicyAndNilWriter(t *testing.T) {
	if _, err := Apply(nil, newFakeWriter(), "overwrite"); err == nil {
		t.Error("未知冲突策略应报错")
	}
	if _, err := Apply(nil, nil, ConflictSkip); err == nil {
		t.Error("缺少 writer 应报错")
	}
}

func TestGroupPathFromZipName(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"Xshell/a.xsh", "", true},
		{"Xshell/生产/a.xsh", "生产", true},
		{"Xshell/生产/华东/a.xsh", "生产/华东", true},
		{"Xshell/Sessions/生产/a.xsh", "生产", true},
		{"other/a.xsh", "", false},
		{"Xshell\\生产\\a.xsh", "生产", true},
	}
	for _, tc := range cases {
		got, ok := groupPathFromZipName(tc.in)
		if ok != tc.ok {
			t.Errorf("%q: ok=%v want %v", tc.in, ok, tc.ok)
			continue
		}
		if strings.Join(got, "/") != tc.want {
			t.Errorf("%q: got %q want %q", tc.in, strings.Join(got, "/"), tc.want)
		}
	}
}
