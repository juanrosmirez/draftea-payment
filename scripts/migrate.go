package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	// Get database connection string from environment
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbName := getEnv("DB_NAME", "payment_system")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres123")
	dbSSLMode := getEnv("DB_SSL_MODE", "disable")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode)

	// Connect to database
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}

	fmt.Println("Connected to database successfully")

	// Create migrations table if it doesn't exist
	createMigrationsTable(db)

	// Run migrations
	if err := runMigrations(db); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}

	fmt.Println("All migrations completed successfully!")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func createMigrationsTable(db *sql.DB) {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
	`
	if _, err := db.Exec(query); err != nil {
		log.Fatal("Failed to create migrations table:", err)
	}
}

func runMigrations(db *sql.DB) error {
	// Get list of migration files
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return fmt.Errorf("failed to read migration files: %v", err)
	}

	// Sort files to ensure they run in order
	sort.Strings(files)

	// Get already applied migrations
	appliedMigrations, err := getAppliedMigrations(db)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %v", err)
	}

	// Run each migration file
	for _, file := range files {
		filename := filepath.Base(file)
		version := strings.TrimSuffix(filename, ".sql")

		// Skip if already applied
		if appliedMigrations[version] {
			fmt.Printf("Migration %s already applied, skipping\n", version)
			continue
		}

		fmt.Printf("Running migration %s...\n", version)

		// Read migration file
		content, err := ioutil.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %v", file, err)
		}

		// Execute migration
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("failed to execute migration %s: %v", version, err)
		}

		// Mark as applied
		if err := markMigrationApplied(db, version); err != nil {
			return fmt.Errorf("failed to mark migration %s as applied: %v", version, err)
		}

		fmt.Printf("Migration %s completed successfully\n", version)
	}

	return nil
}

func getAppliedMigrations(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

func markMigrationApplied(db *sql.DB, version string) error {
	_, err := db.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", version)
	return err
}
