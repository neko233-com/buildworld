package store

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'viewer',
		avatar_url TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_login DATETIME
	)`,
	`CREATE TABLE IF NOT EXISTS ssh_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id),
		name TEXT NOT NULL,
		public_key TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		repo_url TEXT NOT NULL,
		repo_type TEXT NOT NULL,
		default_branch TEXT DEFAULT 'main',
		config TEXT NOT NULL,
		created_by INTEGER REFERENCES users(id),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS builds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id),
		number INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		trigger TEXT NOT NULL,
		branch TEXT,
		commit_sha TEXT,
		started_at DATETIME,
		finished_at DATETIME,
		duration_ms INTEGER,
		log TEXT,
		UNIQUE(project_id, number)
	)`,
	`CREATE TABLE IF NOT EXISTS artifacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		build_id INTEGER NOT NULL REFERENCES builds(id),
		name TEXT NOT NULL,
		path TEXT NOT NULL,
		size INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS workers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		address TEXT NOT NULL,
		token_hash TEXT NOT NULL,
		labels TEXT,
		max_concurrent_builds INTEGER DEFAULT 4,
		status TEXT DEFAULT 'offline',
		last_heartbeat DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS plugins (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		version TEXT NOT NULL,
		description TEXT,
		enabled BOOLEAN DEFAULT TRUE,
		config TEXT,
		installed_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
}
