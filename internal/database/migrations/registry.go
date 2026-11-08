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
		migrations = append(migrations, postgresCoreMigration())
	} else if driver == "mysql" {
		migrations = append(migrations, mysqlCoreMigration())
	} else if driver == "mongodb" {
		migrations = append(migrations, mongoCoreMigration())
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
