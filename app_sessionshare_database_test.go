package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"opscopilot/pkg/dbworkspace"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/sessionshare"
)

type memSecretStore struct {
	data map[string]string
}

func newMemSecretStore() *memSecretStore {
	return &memSecretStore{data: make(map[string]string)}
}

func (m *memSecretStore) Set(service, user, password string) error {
	m.data[service+"\x00"+user] = password
	return nil
}

func (m *memSecretStore) Get(service, user string) (string, error) {
	v, ok := m.data[service+"\x00"+user]
	if !ok {
		return "", secretstore.ErrNotFound
	}
	return v, nil
}

func (m *memSecretStore) Delete(service, user string) error {
	delete(m.data, service+"\x00"+user)
	return nil
}

func seedTestJDBCDriver(t *testing.T, workDir, driverID string) {
	t.Helper()
	dir := filepath.Join(workDir, "jdbc-drivers", driverID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir driver dir: %v", err)
	}
	jarBytes := []byte("dummy-jar")
	if err := os.WriteFile(filepath.Join(dir, "mysql.jar"), jarBytes, 0644); err != nil {
		t.Fatalf("write jar: %v", err)
	}
	sum := sha256.Sum256(jarBytes)
	driver := dbworkspace.JDBCDriver{
		ID:          driverID,
		Name:        "MySQL 8",
		ClassName:   "com.mysql.cj.jdbc.Driver",
		URLTemplate: "jdbc:mysql://{host}:{port}/{database}",
		Dialect:     "mysql",
		Version:     "8.4.0",
		Jars:        []dbworkspace.JDBCJar{{Name: "mysql.jar", SHA256: hex.EncodeToString(sum[:])}},
	}
	b, _ := json.Marshal(driver)
	if err := os.WriteFile(filepath.Join(dir, "driver.json"), b, 0644); err != nil {
		t.Fatalf("write driver.json: %v", err)
	}
}

func TestSharedDatabaseEndToEnd(t *testing.T) {
	bare := initBareRepoForShare(t)
	const driverIDA = "11111111-1111-1111-1111-111111111111"
	const driverIDB = "22222222-2222-2222-2222-222222222222"

	// User A: saves a JDBC MySQL profile with an SSH gateway, connects (records), and pushes
	dirA := filepath.Join(t.TempDir(), "userA")
	appA := newShareTestApp(t, bare, dirA, "team-db-secret")
	appA.secretStore = newMemSecretStore()
	seedTestJDBCDriver(t, dirA, driverIDA)
	appA.initSessionShareStore()
	waitSyncDone(t, appA)

	gwNode, err := appA.savedSessionMgr.EnsureConnectionByEndpoint(remote.ConnectConfig{
		Name:     "JumpBox",
		Protocol: remote.ProtocolSSH,
		Host:     "39.108.107.148",
		Port:     22,
		User:     "root",
		Password: "ssh-jump-password",
	}, "")
	if err != nil {
		t.Fatalf("ensure gateway: %v", err)
	}

	dbPw := "mysql-root-secret"
	savedProfile, err := appA.databaseService().Save(dbworkspace.ProfileInput{
		Profile: dbworkspace.Profile{
			Name:        "订单库-只读",
			Environment: "prod",
			Engine:      "jdbc",
			DriverID:    driverIDA,
			Host:        "127.0.0.1",
			Port:        3306,
			User:        "readonly",
			Database:    "orders",
			TLS:         "off",
			ReadOnly:    true,
			GatewayID:   gwNode.ID,
			GatewayMode: dbworkspace.GatewayModeAuto,
		},
		Password: &dbPw,
	})
	if err != nil {
		t.Fatalf("save db profile: %v", err)
	}

	appA.recordSharedDatabaseLogin(savedProfile.ID)

	// Wait for async upload to complete
	deadline := time.Now().Add(10 * time.Second)
	var listA struct {
		Enabled   bool                 `json:"enabled"`
		Databases []SharedDatabaseView `json:"databases"`
	}
	for time.Now().Before(deadline) {
		_ = json.Unmarshal([]byte(appA.GetSharedDatabases()), &listA)
		if len(listA.Databases) == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !listA.Enabled || len(listA.Databases) != 1 {
		t.Fatalf("expected 1 shared database on user A, got %+v", listA)
	}
	if !listA.Databases[0].HasGateway || listA.Databases[0].GatewayHost != "39.108.107.148" {
		t.Fatalf("expected gateway metadata preserved, got %+v", listA.Databases[0])
	}

	// User B: has a DIFFERENT local DriverID for the same JDBC ClassName
	dirB := filepath.Join(t.TempDir(), "userB")
	appB := newShareTestApp(t, bare, dirB, "team-db-secret")
	appB.secretStore = newMemSecretStore()
	seedTestJDBCDriver(t, dirB, driverIDB)
	appB.initSessionShareStore()
	waitSyncDone(t, appB)

	var listB struct {
		Enabled   bool                 `json:"enabled"`
		Databases []SharedDatabaseView `json:"databases"`
	}
	if err := json.Unmarshal([]byte(appB.GetSharedDatabases()), &listB); err != nil {
		t.Fatalf("unmarshal listB: %v", err)
	}
	if len(listB.Databases) != 1 {
		t.Fatalf("expected 1 shared database on user B, got %+v", listB)
	}

	res := appB.ImportSharedDatabase(listB.Databases[0].EntryKey)
	if !res.Success || res.Profile == nil || res.Reused {
		t.Fatalf("expected fresh import success on user B, got %+v", res)
	}
	if res.Profile.DriverID != driverIDB {
		t.Fatalf("expected DriverID remapped to user B's %s, got %q", driverIDB, res.Profile.DriverID)
	}
	if res.Profile.Group != sharedDatabaseGroupName {
		t.Fatalf("expected group %q, got %q", sharedDatabaseGroupName, res.Profile.Group)
	}
	if res.Profile.GatewayID == "" {
		t.Fatalf("expected SSH gateway created and bound on user B")
	}
	if !res.Profile.ReadOnly || res.Profile.Environment != "prod" {
		t.Fatalf("expected readOnly & environment preserved, got %+v", res.Profile)
	}
	gotPw, err := appB.databaseService().Password(res.Profile.ID)
	if err != nil || gotPw != dbPw {
		t.Fatalf("expected decrypted db password %q, got %q (err=%v)", dbPw, gotPw, err)
	}

	// Second import should reuse existing local profile instead of duplicating
	res2 := appB.ImportSharedDatabase(listB.Databases[0].EntryKey)
	if !res2.Success || !res2.Reused || res2.Profile.ID != res.Profile.ID {
		t.Fatalf("expected second import to reuse profile %s, got %+v", res.Profile.ID, res2)
	}

	// User A removes the shared database
	if msg := appA.RemoveSharedDatabase(listA.Databases[0].EntryKey); msg != "" {
		t.Fatalf("RemoveSharedDatabase failed: %s", msg)
	}
	var listAfter struct {
		Databases []SharedDatabaseView `json:"databases"`
	}
	_ = json.Unmarshal([]byte(appA.GetSharedDatabases()), &listAfter)
	if len(listAfter.Databases) != 0 {
		t.Fatalf("expected 0 shared databases after removal, got %d", len(listAfter.Databases))
	}
}

func TestImportSharedDatabaseMissingJDBCDriver(t *testing.T) {
	bare := initBareRepoForShare(t)
	app := newShareTestApp(t, bare, filepath.Join(t.TempDir(), "userJDBC"), "team-secret")
	app.secretStore = newMemSecretStore()
	app.initSessionShareStore()
	waitSyncDone(t, app)

	rt := app.getSessionShare()
	db := sessionshare.SharedDatabase{
		Owner:           "alice",
		Name:            "DM Prod",
		Engine:          "jdbc",
		DriverName:      "达梦 JDBC",
		DriverClassName: "dm.jdbc.driver.DmDriver",
		DriverDialect:   "dm",
		Host:            "10.0.1.9",
		Port:            5236,
		User:            "SYSDBA",
		LastLoginAt:     time.Now(),
	}
	rt.upsertDBMergedView(db, true)

	res := app.ImportSharedDatabase(db.EntryKey())
	if res.Success || !res.MissingDriver {
		t.Fatalf("expected missing driver result, got %+v", res)
	}
	if res.DriverClassName != "dm.jdbc.driver.DmDriver" {
		t.Fatalf("unexpected driverClassName: %+v", res)
	}
}

func waitSyncDone(t *testing.T, app *App) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var st SessionShareStatus
		_ = json.Unmarshal([]byte(app.GetSessionShareStatus()), &st)
		if !st.Running && st.LastSyncAt != "" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session share sync")
}
