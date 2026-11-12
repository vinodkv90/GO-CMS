package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-cms/cms/internal/database/interfaces"
)

// SQLRepository persists unified audit events into the cms_audit_logs table.
type SQLRepository struct {
	db interfaces.SQLDatabase
}

// NewSQLRepository wraps an SQL database for persistent audit logging.
func NewSQLRepository(db interfaces.SQLDatabase) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) rebind(query string) string {
	if r.db.DriverName() != "postgres" {
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

func (r *SQLRepository) Create(ctx context.Context, entry *AuditLog) error {
	cleanMeta := RedactMetadata(entry.Metadata)
	metaJSON := "{}"
	if len(cleanMeta) > 0 {
		if b, err := json.Marshal(cleanMeta); err == nil {
			metaJSON = string(b)
		}
	}
	result := strings.TrimSpace(entry.Result)
	if result == "" {
		result = "success"
	}
	actorType := strings.TrimSpace(entry.ActorType)
	if actorType == "" {
		actorType = "system"
	}

	query := r.rebind(`INSERT INTO cms_audit_logs (
		id, user_id, action, resource_type, resource_id, metadata, ip_address, actor_type, user_agent, result, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.db.ExecContext(ctx, query,
		entry.ID,
		entry.ActorID,
		entry.Action,
		entry.Resource,
		entry.TargetID,
		metaJSON,
		entry.IPAddress,
		actorType,
		entry.UserAgent,
		result,
		entry.Timestamp,
	)
	return err
}

func (r *SQLRepository) List(ctx context.Context, filter Filter) ([]*AuditLog, int, error) {
	whereParts := []string{"1=1"}
	args := make([]any, 0, 6)

	if filter.ActorID != "" {
		whereParts = append(whereParts, "user_id = ?")
		args = append(args, filter.ActorID)
	}
	if filter.Action != "" {
		whereParts = append(whereParts, "LOWER(action) LIKE LOWER(?)")
		args = append(args, "%"+filter.Action+"%")
	}
	if filter.Resource != "" {
		whereParts = append(whereParts, "LOWER(resource_type) LIKE LOWER(?)")
		args = append(args, "%"+filter.Resource+"%")
	}
	if filter.Result != "" {
		whereParts = append(whereParts, "LOWER(result) = LOWER(?)")
		args = append(args, filter.Result)
	}
	if filter.From != nil {
		whereParts = append(whereParts, "created_at >= ?")
		args = append(args, *filter.From)
	}
	if filter.To != nil {
		whereParts = append(whereParts, "created_at <= ?")
		args = append(args, *filter.To)
	}

	whereSQL := strings.Join(whereParts, " AND ")

	var total int
	countQuery := r.rebind(fmt.Sprintf("SELECT COUNT(*) FROM cms_audit_logs WHERE %s", whereSQL))
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	listArgs := append(append([]any(nil), args...), limit, offset)
	listQuery := r.rebind(fmt.Sprintf(`SELECT id, user_id, actor_type, action, resource_type, resource_id, metadata, ip_address, user_agent, result, created_at
		FROM cms_audit_logs
		WHERE %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`, whereSQL))

	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs := make([]*AuditLog, 0)
	for rows.Next() {
		var l AuditLog
		var userID, actorType, targetID, metaRaw, ipAddr, userAgent, res sql.NullString
		if err := rows.Scan(
			&l.ID,
			&userID,
			&actorType,
			&l.Action,
			&l.Resource,
			&targetID,
			&metaRaw,
			&ipAddr,
			&userAgent,
			&res,
			&l.Timestamp,
		); err != nil {
			return nil, 0, err
		}
		if userID.Valid {
			l.ActorID = userID.String
		}
		if actorType.Valid {
			l.ActorType = actorType.String
		}
		if targetID.Valid {
			l.TargetID = targetID.String
		}
		if ipAddr.Valid {
			l.IPAddress = ipAddr.String
		}
		if userAgent.Valid {
			l.UserAgent = userAgent.String
		}
		if res.Valid {
			l.Result = res.String
		}
		if metaRaw.Valid && strings.TrimSpace(metaRaw.String) != "" {
			var m map[string]any
			if err := json.Unmarshal([]byte(metaRaw.String), &m); err == nil {
				l.Metadata = RedactMetadata(m)
			}
		}
		logs = append(logs, &l)
	}
	return logs, total, rows.Err()
}

