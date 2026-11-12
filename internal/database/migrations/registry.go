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
		migrations = append(migrations, postgresCoreMigration(), postgresContentEngineMigration(), postgresMediaFoldersMigration(), postgresSettingsAdminMigration())
	} else if driver == "mysql" {
		migrations = append(migrations, mysqlCoreMigration(), mysqlContentEngineMigration(), mysqlMediaFoldersMigration(), mysqlSettingsAdminMigration())
	} else if driver == "mongodb" {
		migrations = append(migrations, mongoCoreMigration(), mongoContentEngineMigration(), mongoMediaFoldersMigration(), mongoSettingsAdminMigration())
	}

	return migrations
}

func postgresSettingsAdminMigration() Migration {
	upSQL := `
	ALTER TABLE cms_users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP WITH TIME ZONE NULL;
	ALTER TABLE cms_users ADD COLUMN IF NOT EXISTS failed_login_attempts INT NOT NULL DEFAULT 0;
	ALTER TABLE cms_users ADD COLUMN IF NOT EXISTS locked_until TIMESTAMP WITH TIME ZONE NULL;
	ALTER TABLE cms_users ADD COLUMN IF NOT EXISTS password_changed_at TIMESTAMP WITH TIME ZONE NULL;
	ALTER TABLE cms_users ADD COLUMN IF NOT EXISTS mfa_enabled BOOLEAN NOT NULL DEFAULT false;
	ALTER TABLE cms_users ADD COLUMN IF NOT EXISTS mfa_secret_enc TEXT;
	ALTER TABLE cms_users ADD COLUMN IF NOT EXISTS mfa_backup_codes_hash TEXT;
	ALTER TABLE cms_users ADD COLUMN IF NOT EXISTS sso_provider VARCHAR(100);
	ALTER TABLE cms_users ADD COLUMN IF NOT EXISTS sso_subject VARCHAR(255);

	ALTER TABLE cms_api_tokens ADD COLUMN IF NOT EXISTS kind VARCHAR(20) NOT NULL DEFAULT 'api';
	ALTER TABLE cms_api_tokens ADD COLUMN IF NOT EXISTS description TEXT;
	ALTER TABLE cms_api_tokens ADD COLUMN IF NOT EXISTS token_type VARCHAR(30) NOT NULL DEFAULT 'custom';
	ALTER TABLE cms_api_tokens ADD COLUMN IF NOT EXISTS token_prefix VARCHAR(32) NOT NULL DEFAULT '';
	ALTER TABLE cms_api_tokens ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMP WITH TIME ZONE NULL;

	ALTER TABLE cms_audit_logs ADD COLUMN IF NOT EXISTS actor_type VARCHAR(30) NOT NULL DEFAULT 'admin';
	ALTER TABLE cms_audit_logs ADD COLUMN IF NOT EXISTS user_agent VARCHAR(500);
	ALTER TABLE cms_audit_logs ADD COLUMN IF NOT EXISTS result VARCHAR(20) NOT NULL DEFAULT 'success';
	CREATE INDEX IF NOT EXISTS idx_cms_audit_logs_created_at ON cms_audit_logs (created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_cms_audit_logs_action_res ON cms_audit_logs (action, resource_type);

	CREATE TABLE IF NOT EXISTS cms_user_preferences (
		user_id VARCHAR(36) PRIMARY KEY REFERENCES cms_users(id) ON DELETE CASCADE,
		theme VARCHAR(20) NOT NULL DEFAULT 'system',
		preferred_language VARCHAR(20) NOT NULL DEFAULT 'en',
		timezone VARCHAR(100) NOT NULL DEFAULT 'UTC',
		date_format VARCHAR(50) NOT NULL DEFAULT 'YYYY-MM-DD',
		time_format VARCHAR(20) NOT NULL DEFAULT '24h',
		sidebar_collapsed BOOLEAN NOT NULL DEFAULT false,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_admin_invitations (
		id VARCHAR(36) PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		username VARCHAR(100) NOT NULL,
		first_name VARCHAR(100) NOT NULL DEFAULT '',
		last_name VARCHAR(100) NOT NULL DEFAULT '',
		role_ids JSONB NOT NULL DEFAULT '[]',
		token_hash VARCHAR(64) UNIQUE NOT NULL,
		status VARCHAR(30) NOT NULL DEFAULT 'pending',
		invited_by VARCHAR(36),
		expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
		accepted_at TIMESTAMP WITH TIME ZONE NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_admin_invitations_email ON cms_admin_invitations (email);
	CREATE INDEX IF NOT EXISTS idx_admin_invitations_status ON cms_admin_invitations (status);

	CREATE TABLE IF NOT EXISTS cms_app_roles (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		slug VARCHAR(100) UNIQUE NOT NULL,
		description TEXT,
		type VARCHAR(50) NOT NULL DEFAULT 'custom',
		is_default BOOLEAN NOT NULL DEFAULT false,
		permissions JSONB NOT NULL DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_app_users (
		id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(100) UNIQUE NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL DEFAULT '',
		provider VARCHAR(50) NOT NULL DEFAULT 'local',
		confirmed BOOLEAN NOT NULL DEFAULT false,
		blocked BOOLEAN NOT NULL DEFAULT false,
		confirmation_token_hash VARCHAR(64),
		confirmation_expires_at TIMESTAMP WITH TIME ZONE NULL,
		reset_token_hash VARCHAR(64),
		reset_expires_at TIMESTAMP WITH TIME ZONE NULL,
		role_id VARCHAR(36) REFERENCES cms_app_roles(id) ON DELETE SET NULL,
		last_login_at TIMESTAMP WITH TIME ZONE NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_app_users_role_id ON cms_app_users (role_id);

	CREATE TABLE IF NOT EXISTS cms_oauth_providers (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) UNIQUE NOT NULL,
		display_name VARCHAR(100) NOT NULL,
		provider_type VARCHAR(50) NOT NULL DEFAULT 'oauth2',
		scope VARCHAR(20) NOT NULL DEFAULT 'app',
		enabled BOOLEAN NOT NULL DEFAULT false,
		client_id VARCHAR(255) NOT NULL DEFAULT '',
		client_secret_enc TEXT NOT NULL DEFAULT '',
		auth_url VARCHAR(500) NOT NULL DEFAULT '',
		token_url VARCHAR(500) NOT NULL DEFAULT '',
		userinfo_url VARCHAR(500) NOT NULL DEFAULT '',
		issuer_url VARCHAR(500) NOT NULL DEFAULT '',
		redirect_uri VARCHAR(500) NOT NULL DEFAULT '',
		scopes VARCHAR(500) NOT NULL DEFAULT '',
		config JSONB NOT NULL DEFAULT '{}',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_email_templates (
		id VARCHAR(36) PRIMARY KEY,
		template_key VARCHAR(100) UNIQUE NOT NULL,
		name VARCHAR(150) NOT NULL,
		subject VARCHAR(255) NOT NULL,
		from_name VARCHAR(150) NOT NULL DEFAULT '',
		from_email VARCHAR(255) NOT NULL DEFAULT '',
		reply_to VARCHAR(255) NOT NULL DEFAULT '',
		body_html TEXT NOT NULL DEFAULT '',
		body_text TEXT NOT NULL DEFAULT '',
		variables JSONB NOT NULL DEFAULT '[]',
		enabled BOOLEAN NOT NULL DEFAULT true,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_webhooks (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(150) NOT NULL,
		url VARCHAR(1000) NOT NULL,
		headers JSONB NOT NULL DEFAULT '{}',
		events JSONB NOT NULL DEFAULT '[]',
		secret_enc TEXT NOT NULL DEFAULT '',
		is_enabled BOOLEAN NOT NULL DEFAULT true,
		max_retries INT NOT NULL DEFAULT 3,
		created_by VARCHAR(36),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_webhook_deliveries (
		id VARCHAR(36) PRIMARY KEY,
		webhook_id VARCHAR(36) NOT NULL REFERENCES cms_webhooks(id) ON DELETE CASCADE,
		event VARCHAR(100) NOT NULL,
		request_url VARCHAR(1000) NOT NULL,
		status_code INT NOT NULL DEFAULT 0,
		duration_ms BIGINT NOT NULL DEFAULT 0,
		attempt INT NOT NULL DEFAULT 1,
		success BOOLEAN NOT NULL DEFAULT false,
		error_message TEXT,
		response_snippet TEXT,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_wh ON cms_webhook_deliveries (webhook_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS cms_locales (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(20) UNIQUE NOT NULL,
		name VARCHAR(100) NOT NULL,
		native_name VARCHAR(100) NOT NULL DEFAULT '',
		is_default BOOLEAN NOT NULL DEFAULT false,
		is_enabled BOOLEAN NOT NULL DEFAULT true,
		fallback_code VARCHAR(20) NOT NULL DEFAULT '',
		direction VARCHAR(10) NOT NULL DEFAULT 'ltr',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS cms_plugins_state (
		name VARCHAR(100) PRIMARY KEY,
		enabled BOOLEAN NOT NULL DEFAULT true,
		settings JSONB NOT NULL DEFAULT '{}',
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`

	return Migration{
		Version: "202611120001",
		Name:    "settings_and_administration_schema",
		UpSQL:   upSQL,
	}
}

func mysqlSettingsAdminMigration() Migration {
	upSQL := `
	ALTER TABLE cms_users ADD COLUMN last_login_at TIMESTAMP NULL;
	ALTER TABLE cms_users ADD COLUMN failed_login_attempts INT NOT NULL DEFAULT 0;
	ALTER TABLE cms_users ADD COLUMN locked_until TIMESTAMP NULL;
	ALTER TABLE cms_users ADD COLUMN password_changed_at TIMESTAMP NULL;
	ALTER TABLE cms_users ADD COLUMN mfa_enabled BOOLEAN NOT NULL DEFAULT false;
	ALTER TABLE cms_users ADD COLUMN mfa_secret_enc TEXT;
	ALTER TABLE cms_users ADD COLUMN mfa_backup_codes_hash TEXT;
	ALTER TABLE cms_users ADD COLUMN sso_provider VARCHAR(100);
	ALTER TABLE cms_users ADD COLUMN sso_subject VARCHAR(255);

	ALTER TABLE cms_api_tokens ADD COLUMN kind VARCHAR(20) NOT NULL DEFAULT 'api';
	ALTER TABLE cms_api_tokens ADD COLUMN description TEXT;
	ALTER TABLE cms_api_tokens ADD COLUMN token_type VARCHAR(30) NOT NULL DEFAULT 'custom';
	ALTER TABLE cms_api_tokens ADD COLUMN token_prefix VARCHAR(32) NOT NULL DEFAULT '';
	ALTER TABLE cms_api_tokens ADD COLUMN revoked_at TIMESTAMP NULL;

	ALTER TABLE cms_audit_logs ADD COLUMN actor_type VARCHAR(30) NOT NULL DEFAULT 'admin';
	ALTER TABLE cms_audit_logs ADD COLUMN user_agent VARCHAR(500);
	ALTER TABLE cms_audit_logs ADD COLUMN result VARCHAR(20) NOT NULL DEFAULT 'success';
	CREATE INDEX idx_cms_audit_logs_created_at ON cms_audit_logs (created_at);
	CREATE INDEX idx_cms_audit_logs_action_res ON cms_audit_logs (action, resource_type);

	CREATE TABLE IF NOT EXISTS cms_user_preferences (
		user_id VARCHAR(36) PRIMARY KEY,
		theme VARCHAR(20) NOT NULL DEFAULT 'system',
		preferred_language VARCHAR(20) NOT NULL DEFAULT 'en',
		timezone VARCHAR(100) NOT NULL DEFAULT 'UTC',
		date_format VARCHAR(50) NOT NULL DEFAULT 'YYYY-MM-DD',
		time_format VARCHAR(20) NOT NULL DEFAULT '24h',
		sidebar_collapsed BOOLEAN NOT NULL DEFAULT false,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		CONSTRAINT fk_pref_user FOREIGN KEY (user_id) REFERENCES cms_users(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_admin_invitations (
		id VARCHAR(36) PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		username VARCHAR(100) NOT NULL,
		first_name VARCHAR(100) NOT NULL DEFAULT '',
		last_name VARCHAR(100) NOT NULL DEFAULT '',
		role_ids JSON,
		token_hash VARCHAR(64) UNIQUE NOT NULL,
		status VARCHAR(30) NOT NULL DEFAULT 'pending',
		invited_by VARCHAR(36),
		expires_at TIMESTAMP NOT NULL,
		accepted_at TIMESTAMP NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		KEY idx_admin_invitations_email (email),
		KEY idx_admin_invitations_status (status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_app_roles (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		slug VARCHAR(100) UNIQUE NOT NULL,
		description TEXT,
		type VARCHAR(50) NOT NULL DEFAULT 'custom',
		is_default BOOLEAN NOT NULL DEFAULT false,
		permissions JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_app_users (
		id VARCHAR(36) PRIMARY KEY,
		username VARCHAR(100) UNIQUE NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL DEFAULT '',
		provider VARCHAR(50) NOT NULL DEFAULT 'local',
		confirmed BOOLEAN NOT NULL DEFAULT false,
		blocked BOOLEAN NOT NULL DEFAULT false,
		confirmation_token_hash VARCHAR(64),
		confirmation_expires_at TIMESTAMP NULL,
		reset_token_hash VARCHAR(64),
		reset_expires_at TIMESTAMP NULL,
		role_id VARCHAR(36),
		last_login_at TIMESTAMP NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		KEY idx_app_users_role_id (role_id),
		CONSTRAINT fk_app_user_role FOREIGN KEY (role_id) REFERENCES cms_app_roles(id) ON DELETE SET NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_oauth_providers (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) UNIQUE NOT NULL,
		display_name VARCHAR(100) NOT NULL,
		provider_type VARCHAR(50) NOT NULL DEFAULT 'oauth2',
		scope VARCHAR(20) NOT NULL DEFAULT 'app',
		enabled BOOLEAN NOT NULL DEFAULT false,
		client_id VARCHAR(255) NOT NULL DEFAULT '',
		client_secret_enc TEXT NOT NULL,
		auth_url VARCHAR(500) NOT NULL DEFAULT '',
		token_url VARCHAR(500) NOT NULL DEFAULT '',
		userinfo_url VARCHAR(500) NOT NULL DEFAULT '',
		issuer_url VARCHAR(500) NOT NULL DEFAULT '',
		redirect_uri VARCHAR(500) NOT NULL DEFAULT '',
		scopes VARCHAR(500) NOT NULL DEFAULT '',
		config JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_email_templates (
		id VARCHAR(36) PRIMARY KEY,
		template_key VARCHAR(100) UNIQUE NOT NULL,
		name VARCHAR(150) NOT NULL,
		subject VARCHAR(255) NOT NULL,
		from_name VARCHAR(150) NOT NULL DEFAULT '',
		from_email VARCHAR(255) NOT NULL DEFAULT '',
		reply_to VARCHAR(255) NOT NULL DEFAULT '',
		body_html TEXT NOT NULL,
		body_text TEXT NOT NULL,
		variables JSON,
		enabled BOOLEAN NOT NULL DEFAULT true,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_webhooks (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(150) NOT NULL,
		url VARCHAR(1000) NOT NULL,
		headers JSON,
		events JSON,
		secret_enc TEXT,
		is_enabled BOOLEAN NOT NULL DEFAULT true,
		max_retries INT NOT NULL DEFAULT 3,
		created_by VARCHAR(36),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_webhook_deliveries (
		id VARCHAR(36) PRIMARY KEY,
		webhook_id VARCHAR(36) NOT NULL,
		event VARCHAR(100) NOT NULL,
		request_url VARCHAR(1000) NOT NULL,
		status_code INT NOT NULL DEFAULT 0,
		duration_ms BIGINT NOT NULL DEFAULT 0,
		attempt INT NOT NULL DEFAULT 1,
		success BOOLEAN NOT NULL DEFAULT false,
		error_message TEXT,
		response_snippet TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		KEY idx_webhook_deliveries_wh (webhook_id, created_at),
		CONSTRAINT fk_wh_del_webhook FOREIGN KEY (webhook_id) REFERENCES cms_webhooks(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_locales (
		id VARCHAR(36) PRIMARY KEY,
		code VARCHAR(20) UNIQUE NOT NULL,
		name VARCHAR(100) NOT NULL,
		native_name VARCHAR(100) NOT NULL DEFAULT '',
		is_default BOOLEAN NOT NULL DEFAULT false,
		is_enabled BOOLEAN NOT NULL DEFAULT true,
		fallback_code VARCHAR(20) NOT NULL DEFAULT '',
		direction VARCHAR(10) NOT NULL DEFAULT 'ltr',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	CREATE TABLE IF NOT EXISTS cms_plugins_state (
		name VARCHAR(100) PRIMARY KEY,
		enabled BOOLEAN NOT NULL DEFAULT true,
		settings JSON,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`

	return Migration{
		Version: "202611120001",
		Name:    "settings_and_administration_schema",
		UpSQL:   upSQL,
	}
}

func mongoSettingsAdminMigration() Migration {
	return Migration{
		Version: "202611120001",
		Name:    "settings_and_administration_schema",
		UpMongo: func(ctx context.Context, db *mongo.Database) error {
			collections := map[string][]mongo.IndexModel{
				"cms_user_preferences": {
					{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_admin_invitations": {
					{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
					{Keys: bson.D{{Key: "email", Value: 1}}},
					{Keys: bson.D{{Key: "status", Value: 1}}},
				},
				"cms_app_roles": {
					{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_app_users": {
					{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true)},
					{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)},
					{Keys: bson.D{{Key: "role_id", Value: 1}}},
				},
				"cms_oauth_providers": {
					{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_email_templates": {
					{Keys: bson.D{{Key: "template_key", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_webhook_deliveries": {
					{Keys: bson.D{{Key: "webhook_id", Value: 1}, {Key: "created_at", Value: -1}}},
				},
				"cms_locales": {
					{Keys: bson.D{{Key: "code", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_plugins_state": {
					{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)},
				},
				"cms_audit_logs": {
					{Keys: bson.D{{Key: "created_at", Value: -1}}},
					{Keys: bson.D{{Key: "action", Value: 1}, {Key: "resource_type", Value: 1}}},
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

func postgresMediaFoldersMigration() Migration {
	upSQL := `
	CREATE TABLE IF NOT EXISTS cms_media_folders (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		slug VARCHAR(100) NOT NULL,
		parent_id VARCHAR(36) REFERENCES cms_media_folders(id) ON DELETE CASCADE,
		path TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		created_by VARCHAR(36)
	);

	CREATE INDEX IF NOT EXISTS idx_media_folders_parent ON cms_media_folders(parent_id);
	CREATE INDEX IF NOT EXISTS idx_media_folders_path ON cms_media_folders(path);

	ALTER TABLE cms_media ADD COLUMN IF NOT EXISTS folder_id VARCHAR(36) REFERENCES cms_media_folders(id) ON DELETE SET NULL;
	CREATE INDEX IF NOT EXISTS idx_cms_media_folder ON cms_media(folder_id);
	`

	downSQL := `
	DROP INDEX IF EXISTS idx_cms_media_folder;
	ALTER TABLE cms_media DROP COLUMN IF EXISTS folder_id;
	DROP TABLE IF EXISTS cms_media_folders;
	`

	return Migration{
		Version: "202611090001",
		Name:    "media_folders_schema",
		UpSQL:   upSQL,
		DownSQL: downSQL,
	}
}

func mysqlMediaFoldersMigration() Migration {
	upSQL := `
	CREATE TABLE IF NOT EXISTS cms_media_folders (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		slug VARCHAR(100) NOT NULL,
		parent_id VARCHAR(36) NULL,
		path TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		created_by VARCHAR(36) NULL,
		KEY idx_media_folders_parent (parent_id),
		KEY idx_media_folders_path (path(255)),
		CONSTRAINT fk_media_folder_parent FOREIGN KEY (parent_id) REFERENCES cms_media_folders(id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

	ALTER TABLE cms_media ADD COLUMN folder_id VARCHAR(36) NULL;
	CREATE INDEX idx_cms_media_folder ON cms_media(folder_id);
	`

	downSQL := `
	DROP TABLE IF EXISTS cms_media_folders;
	`

	return Migration{
		Version: "202611090001",
		Name:    "media_folders_schema",
		UpSQL:   upSQL,
		DownSQL: downSQL,
	}
}

func mongoMediaFoldersMigration() Migration {
	return Migration{
		Version: "202611090001",
		Name:    "media_folders_schema",
		UpMongo: func(ctx context.Context, db *mongo.Database) error {
			foldersCol := db.Collection("cms_media_folders")
			mediaCol := db.Collection("cms_media")

			_, err := foldersCol.Indexes().CreateMany(ctx, []mongo.IndexModel{
				{Keys: bson.D{{Key: "parent_id", Value: 1}}},
				{Keys: bson.D{{Key: "path", Value: 1}}},
				{Keys: bson.D{{Key: "slug", Value: 1}, {Key: "parent_id", Value: 1}}},
			})
			if err != nil {
				return err
			}

			_, err = mediaCol.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "folder_id", Value: 1}},
			})
			return err
		},
		DownMongo: func(ctx context.Context, db *mongo.Database) error {
			return db.Collection("cms_media_folders").Drop(ctx)
		},
	}
}
