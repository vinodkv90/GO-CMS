package migrations

import (
	"context"
	"testing"

	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationChecksum(t *testing.T) {
	m1 := Migration{
		Version: "202611080001",
		Name:    "test_migration",
		UpSQL:   "CREATE TABLE test (id INT);",
	}

	checksum := m1.Checksum()
	require.NotEmpty(t, checksum)
	assert.Equal(t, 64, len(checksum)) // SHA-256 hex string

	// Same content gives same checksum
	m2 := Migration{
		Version: "202611080001",
		Name:    "test_migration",
		UpSQL:   "CREATE TABLE test (id INT);",
	}
	assert.Equal(t, checksum, m2.Checksum())

	// Different content gives different checksum
	m3 := Migration{
		Version: "202611080001",
		Name:    "test_migration",
		UpSQL:   "CREATE TABLE test (id BIGINT);",
	}
	assert.NotEqual(t, checksum, m3.Checksum())
}

func TestSplitStatements(t *testing.T) {
	script := `
		CREATE TABLE a (id INT);
		CREATE TABLE b (name TEXT);
		INSERT INTO a VALUES (1);
	`

	statements := splitStatements(script)
	assert.Len(t, statements, 3)
	assert.Contains(t, statements[0], "CREATE TABLE a")
	assert.Contains(t, statements[1], "CREATE TABLE b")
	assert.Contains(t, statements[2], "INSERT INTO a VALUES (1)")
}

func TestDefaultMigrations(t *testing.T) {
	pgMigrations := DefaultMigrations("postgres")
	assert.Len(t, pgMigrations, 4)
	assert.Equal(t, "202611080001", pgMigrations[0].Version)
	assert.Equal(t, "202611080002", pgMigrations[1].Version)
	assert.Equal(t, "202611090001", pgMigrations[2].Version)
	assert.Equal(t, "202611120001", pgMigrations[3].Version)
	assert.Contains(t, pgMigrations[0].UpSQL, "cms_roles")
	assert.Contains(t, pgMigrations[0].UpSQL, "cms_users")
	assert.Contains(t, pgMigrations[1].UpSQL, "content_types")
	assert.Contains(t, pgMigrations[2].UpSQL, "cms_media_folders")
	assert.Contains(t, pgMigrations[3].UpSQL, "cms_admin_invitations")
	assert.Contains(t, pgMigrations[3].UpSQL, "cms_app_roles")
	assert.Contains(t, pgMigrations[3].UpSQL, "cms_webhooks")
	assert.NotEmpty(t, splitStatements(pgMigrations[3].UpSQL))

	myMigrations := DefaultMigrations("mysql")
	assert.Len(t, myMigrations, 4)
	assert.Equal(t, "202611080001", myMigrations[0].Version)
	assert.Equal(t, "202611080002", myMigrations[1].Version)
	assert.Equal(t, "202611090001", myMigrations[2].Version)
	assert.Equal(t, "202611120001", myMigrations[3].Version)
	assert.Contains(t, myMigrations[2].UpSQL, "cms_media_folders")
	assert.Contains(t, myMigrations[3].UpSQL, "cms_admin_invitations")
	assert.Contains(t, myMigrations[3].UpSQL, "cms_email_templates")
	assert.NotEmpty(t, splitStatements(myMigrations[3].UpSQL))

	mongoMigrations := DefaultMigrations("mongodb")
	assert.Len(t, mongoMigrations, 4)
	assert.Equal(t, "202611080001", mongoMigrations[0].Version)
	assert.Equal(t, "202611080002", mongoMigrations[1].Version)
	assert.Equal(t, "202611090001", mongoMigrations[2].Version)
	assert.Equal(t, "202611120001", mongoMigrations[3].Version)
}

// MockDatabase for testing Runner orchestration
type mockDB struct {
	driver string
	caps   interfaces.Capabilities
}

func (m *mockDB) DriverName() string                    { return m.driver }
func (m *mockDB) Ping(ctx context.Context) error        { return nil }
func (m *mockDB) Close() error                          { return nil }
func (m *mockDB) Capabilities() interfaces.Capabilities { return m.caps }

func TestRunner_Register(t *testing.T) {
	db := &mockDB{driver: "mock", caps: interfaces.NewCapabilitySet()}
	runner := NewRunner(db)
	assert.Empty(t, runner.Migrations())

	runner.Register(DefaultMigrations("postgres")...)
	assert.Len(t, runner.Migrations(), 4)
}
