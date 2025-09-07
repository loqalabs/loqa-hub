/*
 * This file is part of Loqa (https://github.com/loqalabs/loqa).
 * Copyright (C) 2025 Loqa Labs
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed *.sql
var schemaFiles embed.FS

// Database wraps SQLite connection with Loqa-specific operations
type Database struct {
	db   *sql.DB
	path string
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Path string
}

// NewDatabase creates a new database instance with SQLite
func NewDatabase(config DatabaseConfig) (*Database, error) {
	if config.Path == "" {
		config.Path = getDefaultDBPath()
	}

	// Ensure directory exists
	if err := ensureDir(filepath.Dir(config.Path)); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite", config.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure SQLite for optimal performance
	if err := configureSQLite(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to configure SQLite: %w", err)
	}

	database := &Database{
		db:   db,
		path: config.Path,
	}

	// Run migrations
	if err := database.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	log.Printf("✅ Database connected: %s", sanitizeLogInput(config.Path))
	return database, nil
}

// getDefaultDBPath returns the default database path
func getDefaultDBPath() string {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		// Default to ./data/loqa-hub.db
		dbPath = "./data/loqa-hub.db"
	}
	return dbPath
}

// ensureDir creates directory if it doesn't exist
func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0750)
}

// configureSQLite sets optimal SQLite settings for our use case
func configureSQLite(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",    // Write-Ahead Logging for better concurrency
		"PRAGMA synchronous = NORMAL",  // Good balance of safety and performance
		"PRAGMA cache_size = 10000",    // 10MB cache
		"PRAGMA temp_store = memory",   // Store temp tables in memory
		"PRAGMA mmap_size = 268435456", // 256MB memory-mapped I/O
		"PRAGMA foreign_keys = ON",     // Enable foreign key constraints
		"PRAGMA busy_timeout = 5000",   // 5 second timeout for locks
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute pragma %q: %w", pragma, err)
		}
	}

	return nil
}

// migrate runs database migrations
func (d *Database) migrate() error {
	// Read schema from embedded file
	schemaSQL, err := schemaFiles.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema.sql: %w", err)
	}

	// Execute schema
	if _, err := d.db.Exec(string(schemaSQL)); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	log.Println("✅ Database schema migrated successfully")
	return nil
}

// DB returns the underlying sql.DB instance
func (d *Database) DB() *sql.DB {
	return d.db
}

// Close closes the database connection
func (d *Database) Close() error {
	if d.db != nil {
		log.Printf("🔌 Closing database connection: %s", sanitizeLogInput(d.path))
		return d.db.Close()
	}
	return nil
}

// Ping tests the database connection
func (d *Database) Ping() error {
	return d.db.Ping()
}

// Stats returns database statistics
func (d *Database) Stats() sql.DBStats {
	return d.db.Stats()
}

// GetPath returns the database file path
func (d *Database) GetPath() string {
	return d.path
}

// Vacuum runs VACUUM to reclaim space and optimize the database
func (d *Database) Vacuum() error {
	_, err := d.db.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("failed to vacuum database: %w", err)
	}

	log.Println("✅ Database vacuumed successfully")
	return nil
}

// Checkpoint forces a WAL checkpoint to sync data to main database file
func (d *Database) Checkpoint() error {
	_, err := d.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	if err != nil {
		return fmt.Errorf("failed to checkpoint database: %w", err)
	}

	log.Println("✅ Database checkpoint completed")
	return nil
}
