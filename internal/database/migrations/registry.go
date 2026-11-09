package migrations

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DefaultMigrations returns the built-in system migrations.
func DefaultMigrations(driver string) []Migration {
	var migrations []Migration

	if driver == "postgres" {
		migrations = append(migrations, postgresCoreMigration(), postgresContentEngineMigration())
	} else if driver == "mysql" {
		migrations = append(migrations, mysqlCoreMigration(), mysqlContentEngineMigration())
	} else if driver == "mongodb" {
		migrations = append(migrations, mongoCoreMigration(), mongoContentEngineMigration())
	}

	return migrations
}

func postgresCoreMigration() Migration {
	upSQL := `
	CREATE TABLE IF NOT EXISTS cms_roles (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		slug VARCHAR(100) UNIQUE NOT NULL,
		description TEXT,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_permissions (
		id VARCHAR(36) PRIMARY KEY,
		action VARCHAR(100) NOT NULL,
		resource VARCHAR(100) NOT NULL,
		description TEXT,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT uq_action_resource UNIQUE(action, resource)
	);

	CREATE TABLE IF NOT EXISTS cms_role_permissions (
		role_id VARCHAR(36) REFERENCES cms_roles(id) ON DELETE CASCADE,
		permission_id VARCHAR(36) REFERENCES cms_permissions(id) ON DELETE CASCADE,
		PRIMARY KEY (role_id, permission_id)
	);

	CREATE TABLE IF NOT EXISTS cms_users (
		id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(100) UNIQUE NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		status VARCHAR(50) DEFAULT 'active',
		first_name VARCHAR(100),
		last_name VARCHAR(100),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_user_roles (
		user_id VARCHAR(36) REFERENCES cms_users(id) ON DELETE CASCADE,
		role_id VARCHAR(36) REFERENCES cms_roles(id) ON DELETE CASCADE,
		PRIMARY KEY (user_id, role_id)
	);

	CREATE TABLE IF NOT EXISTS cms_api_tokens (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		token_hash VARCHAR(64) UNIQUE NOT NULL,
		user_id VARCHAR(36) REFERENCES cms_users(id) ON DELETE CASCADE,
		permissions JSONB,
		expires_at TIMESTAMP WITH TIME ZONE,
		last_used_at TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_content_types (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		slug VARCHAR(100) UNIQUE NOT NULL,
		kind VARCHAR(50) NOT NULL, -- 'collection' or 'single'
		schema_definition JSONB NOT NULL,
		version INT DEFAULT 1,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_media (
		id VARCHAR(36) PRIMARY KEY,
		filename VARCHAR(255) NOT NULL,
		original_name VARCHAR(255) NOT NULL,
		mime_type VARCHAR(100) NOT NULL,
		file_size BIGINT NOT NULL,
		storage_provider VARCHAR(50) NOT NULL,
		storage_key VARCHAR(500) NOT NULL,
		metadata JSONB,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_audit_logs (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36),
		action VARCHAR(100) NOT NULL,
		resource_type VARCHAR(100) NOT NULL,
		resource_id VARCHAR(100),
		metadata JSONB,
		ip_address VARCHAR(45),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_settings (
		id VARCHAR(36) PRIMARY KEY,
		key VARCHAR(100) UNIQUE NOT NULL,
		value JSONB NOT NULL,
		is_public BOOLEAN DEFAULT false,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`

	return Migration{
		Version: "202611080001",
		Name:    "init_core_schema",
		UpSQL:   upSQL,
	}
}

func mysqlCoreMigration() Migration {
	upSQL := `
	CREATE TABLE IF NOT EXISTS cms_roles (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		slug VARCHAR(100) UNIQUE NOT NULL,
		description TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_permissions (
		id VARCHAR(36) PRIMARY KEY,
		action VARCHAR(100) NOT NULL,
		resource VARCHAR(100) NOT NULL,
		description TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE KEY uq_action_resource (action, resource)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_role_permissions (
		role_id VARCHAR(36),
		permission_id VARCHAR(36),
		PRIMARY KEY (role_id, permission_id),
		CONSTRAINT fk_rp_role FOREIGN KEY (role_id) REFERENCES cms_roles(id) ON DELETE CASCADE,
		CONSTRAINT fk_rp_perm FOREIGN KEY (permission_id) REFERENCES cms_permissions(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_users (
		id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(100) UNIQUE NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		status VARCHAR(50) DEFAULT 'active',
		first_name VARCHAR(100),
		last_name VARCHAR(100),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_user_roles (
		user_id VARCHAR(36),
		role_id VARCHAR(36),
		PRIMARY KEY (user_id, role_id),
		CONSTRAINT fk_ur_user FOREIGN KEY (user_id) REFERENCES cms_users(id) ON DELETE CASCADE,
		CONSTRAINT fk_ur_role FOREIGN KEY (role_id) REFERENCES cms_roles(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_api_tokens (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		token_hash VARCHAR(64) UNIQUE NOT NULL,
		user_id VARCHAR(36),
		permissions JSON,
		expires_at TIMESTAMP NULL,
		last_used_at TIMESTAMP NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT fk_token_user FOREIGN KEY (user_id) REFERENCES cms_users(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_content_types (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		slug VARCHAR(100) UNIQUE NOT NULL,
		kind VARCHAR(50) NOT NULL,
		schema_definition JSON NOT NULL,
		version INT DEFAULT 1,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_media (
		id VARCHAR(36) PRIMARY KEY,
		filename VARCHAR(255) NOT NULL,
		original_name VARCHAR(255) NOT NULL,
		mime_type VARCHAR(100) NOT NULL,
		file_size BIGINT NOT NULL,
		storage_provider VARCHAR(50) NOT NULL,
		storage_key VARCHAR(500) NOT NULL,
		metadata JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_audit_logs (
		id VARCHAR(36) PRIMARY KEY,
		user_id VARCHAR(36),
		action VARCHAR(100) NOT NULL,
		resource_type VARCHAR(100) NOT NULL,
		resource_id VARCHAR(100),
		metadata JSON,
		ip_address VARCHAR(45),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_settings (
		id VARCHAR(36) PRIMARY KEY,
		key VARCHAR(100) UNIQUE NOT NULL,
		value JSON NOT NULL,
		is_public BOOLEAN DEFAULT false,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`

	return Migration{
		Version: "202611080001",
		Name:    "init_core_schema",
		UpSQL:   upSQL,
	}
}

func mongoCoreMigration() Migration {
	return Migration{
		Version: "202611080001",
		Name:    "init_core_schema",
		UpMongo: func(ctx context.Context, db *mongo.Database) error {
			// Create unique indexes
			collections := map[string][]mongo.IndexModel{
				"cms_users": {
					{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true)},
					{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_roles": {
					{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_permissions": {
					{Keys: bson.D{{Key: "action", Value: 1}, {Key: "resource", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_api_tokens": {
					{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_content_types": {
					{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_settings": {
					{Keys: bson.D{{Key: "key", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
			}

			for colName, indexModels := range collections {
				col := db.Collection(colName)
				for _, idx := range indexModels {
					if _, err := col.Indexes().CreateOne(ctx, idx); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}

func postgresContentEngineMigration() Migration {
	upSQL := `
	CREATE TABLE IF NOT EXISTS content_types (
		id VARCHAR(36) PRIMARY KEY,
		uid VARCHAR(100) UNIQUE NOT NULL,
		name VARCHAR(100) NOT NULL,
		kind VARCHAR(50) NOT NULL,
		settings JSONB NOT NULL DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS content_fields (
		id VARCHAR(36) PRIMARY KEY,
		content_type_id VARCHAR(36) REFERENCES content_types(id) ON DELETE CASCADE,
		name VARCHAR(100) NOT NULL,
		field_key VARCHAR(100) NOT NULL,
		field_type VARCHAR(50) NOT NULL,
		configuration JSONB NOT NULL DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT uq_ct_field_key UNIQUE(content_type_id, field_key)
	);

	CREATE TABLE IF NOT EXISTS content_entries (
		id VARCHAR(36) PRIMARY KEY,
		content_type_id VARCHAR(36) REFERENCES content_types(id) ON DELETE CASCADE,
		slug VARCHAR(255),
		draft_data JSONB NOT NULL DEFAULT '{}',
		published_data JSONB,
		status VARCHAR(50) NOT NULL DEFAULT 'draft',
		draft_version INT NOT NULL DEFAULT 1,
		published_version INT DEFAULT 0,
		singleton_key VARCHAR(36) UNIQUE,
		created_by VARCHAR(36),
		updated_by VARCHAR(36),
		published_at TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		deleted_at TIMESTAMP WITH TIME ZONE
	);

	CREATE INDEX IF NOT EXISTS idx_entries_ct_status ON content_entries(content_type_id, status);
	CREATE INDEX IF NOT EXISTS idx_entries_slug ON content_entries(content_type_id, slug);

	CREATE TABLE IF NOT EXISTS content_versions (
		id VARCHAR(36) PRIMARY KEY,
		entry_id VARCHAR(36) REFERENCES content_entries(id) ON DELETE CASCADE,
		version_number INT NOT NULL,
		snapshot JSONB NOT NULL,
		action VARCHAR(50) NOT NULL,
		changed_by VARCHAR(36),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_versions_entry ON content_versions(entry_id, version_number DESC);

	CREATE TABLE IF NOT EXISTS content_relations (
		id VARCHAR(36) PRIMARY KEY,
		source_entry_id VARCHAR(36) REFERENCES content_entries(id) ON DELETE CASCADE,
		target_entry_id VARCHAR(36) REFERENCES content_entries(id) ON DELETE CASCADE,
		relation_key VARCHAR(100) NOT NULL
	);
	`

	return Migration{
		Version: "202611080002",
		Name:    "content_engine_schema",
		UpSQL:   upSQL,
	}
}

func mysqlContentEngineMigration() Migration {
	upSQL := `
	CREATE TABLE IF NOT EXISTS content_types (
		id VARCHAR(36) PRIMARY KEY,
		uid VARCHAR(100) UNIQUE NOT NULL,
		name VARCHAR(100) NOT NULL,
		kind VARCHAR(50) NOT NULL,
		settings JSON NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS content_fields (
		id VARCHAR(36) PRIMARY KEY,
		content_type_id VARCHAR(36) NOT NULL,
		name VARCHAR(100) NOT NULL,
		field_key VARCHAR(100) NOT NULL,
		field_type VARCHAR(50) NOT NULL,
		configuration JSON NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE KEY uq_ct_field_key (content_type_id, field_key),
		CONSTRAINT fk_cf_ct FOREIGN KEY (content_type_id) REFERENCES content_types(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS content_entries (
		id VARCHAR(36) PRIMARY KEY,
		content_type_id VARCHAR(36) NOT NULL,
		slug VARCHAR(255),
		draft_data JSON NOT NULL,
		published_data JSON,
		status VARCHAR(50) NOT NULL DEFAULT 'draft',
		draft_version INT NOT NULL DEFAULT 1,
		published_version INT DEFAULT 0,
		singleton_key VARCHAR(36) UNIQUE,
		created_by VARCHAR(36),
		updated_by VARCHAR(36),
		published_at TIMESTAMP NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		deleted_at TIMESTAMP NULL,
		KEY idx_entries_ct_status (content_type_id, status),
		KEY idx_entries_slug (content_type_id, slug),
		CONSTRAINT fk_ce_ct FOREIGN KEY (content_type_id) REFERENCES content_types(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS content_versions (
		id VARCHAR(36) PRIMARY KEY,
		entry_id VARCHAR(36) NOT NULL,
		version_number INT NOT NULL,
		snapshot JSON NOT NULL,
		action VARCHAR(50) NOT NULL,
		changed_by VARCHAR(36),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		KEY idx_versions_entry (entry_id, version_number),
		CONSTRAINT fk_cv_entry FOREIGN KEY (entry_id) REFERENCES content_entries(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS content_relations (
		id VARCHAR(36) PRIMARY KEY,
		source_entry_id VARCHAR(36) NOT NULL,
		target_entry_id VARCHAR(36) NOT NULL,
		relation_key VARCHAR(100) NOT NULL,
		CONSTRAINT fk_cr_src FOREIGN KEY (source_entry_id) REFERENCES content_entries(id) ON DELETE CASCADE,
		CONSTRAINT fk_cr_tgt FOREIGN KEY (target_entry_id) REFERENCES content_entries(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`

	return Migration{
		Version: "202611080002",
		Name:    "content_engine_schema",
		UpSQL:   upSQL,
	}
}

func mongoContentEngineMigration() Migration {
	return Migration{
		Version: "202611080002",
		Name:    "content_engine_schema",
		UpMongo: func(ctx context.Context, db *mongo.Database) error {
			collections := map[string][]mongo.IndexModel{
				"content_types": {
					{Keys: bson.D{{Key: "uid", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"content_fields": {
					{Keys: bson.D{{Key: "content_type_id", Value: 1}, {Key: "field_key", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"content_entries": {
					{Keys: bson.D{{Key: "content_type_id", Value: 1}, {Key: "status", Value: 1}}},
					{Keys: bson.D{{Key: "singleton_key", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
				},
				"content_versions": {
					{Keys: bson.D{{Key: "entry_id", Value: 1}, {Key: "version_number", Value: -1}}},
				},
			}

			for colName, indexModels := range collections {
				col := db.Collection(colName)
				for _, idx := range indexModels {
					if _, err := col.Indexes().CreateOne(ctx, idx); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}
