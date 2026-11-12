package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-cms/cms/internal/database/interfaces"
)

// PluginStateRecord holds persisted plugin activation and configuration state in cms_plugins_state.
type PluginStateRecord struct {
	Name      string
	Enabled   bool
	Settings  map[string]any
	UpdatedAt time.Time
}

func (m *Manager) sqlDB() interfaces.SQLDatabase {
	if m.db == nil {
		return nil
	}
	sqlDB, ok := m.db.(interfaces.SQLDatabase)
	if !ok || sqlDB == nil {
		return nil
	}
	return sqlDB
}

func rebindPluginQuery(driver, query string) string {
	if driver != "postgres" {
		return query
	}
	var sb strings.Builder
	idx := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			sb.WriteString(fmt.Sprintf("$%d", idx))
			idx++
		} else {
			sb.WriteByte(query[i])
		}
	}
	return sb.String()
}

func (m *Manager) loadAllPersistedStates(ctx context.Context) map[string]PluginStateRecord {
	res := make(map[string]PluginStateRecord)
	sqlDB := m.sqlDB()
	if sqlDB == nil {
		return res
	}
	rows, err := sqlDB.QueryContext(ctx, `SELECT name, enabled, settings, updated_at FROM cms_plugins_state`)
	if err != nil {
		m.logger.Warn("Failed to read cms_plugins_state", "error", err)
		return res
	}
	defer rows.Close()

	for rows.Next() {
		var rec PluginStateRecord
		var settingsRaw string
		if err := rows.Scan(&rec.Name, &rec.Enabled, &settingsRaw, &rec.UpdatedAt); err != nil {
			continue
		}
		rec.Settings = make(map[string]any)
		if strings.TrimSpace(settingsRaw) != "" {
			_ = json.Unmarshal([]byte(settingsRaw), &rec.Settings)
		}
		res[rec.Name] = rec
	}
	return res
}

func (m *Manager) persistPluginStateLocked(ctx context.Context, entity *PluginEntity) {
	sqlDB := m.sqlDB()
	if sqlDB == nil || entity == nil {
		return
	}
	settingsMap := entity.Settings
	if settingsMap == nil {
		settingsMap = map[string]any{}
	}
	settingsBytes, err := json.Marshal(settingsMap)
	if err != nil {
		return
	}
	enabled := entity.Status == StatusEnabled
	now := time.Now().UTC()

	var query string
	if sqlDB.DriverName() == "mysql" {
		query = `INSERT INTO cms_plugins_state (name, enabled, settings, updated_at)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE enabled = VALUES(enabled), settings = VALUES(settings), updated_at = VALUES(updated_at)`
	} else {
		query = `INSERT INTO cms_plugins_state (name, enabled, settings, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (name) DO UPDATE SET enabled = EXCLUDED.enabled, settings = EXCLUDED.settings, updated_at = EXCLUDED.updated_at`
	}
	if _, err := sqlDB.ExecContext(ctx, rebindPluginQuery(sqlDB.DriverName(), query), entity.Manifest.ID, enabled, string(settingsBytes), now); err != nil {
		m.logger.Warn("Failed to persist plugin state", "plugin", entity.Manifest.ID, "error", err)
	}
}

