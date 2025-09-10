package database

import (
	"database/sql"
	"fmt"

	"payment-system/internal/config"

	_ "github.com/lib/pq"
)

// DB wrapper para la conexión de base de datos
type DB struct {
	*sql.DB
}

// New crea una nueva conexión a la base de datos
func New(cfg config.DatabaseConfig) (*DB, error) {
	db, err := sql.Open("postgres", cfg.GetDSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configurar pool de conexiones
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verificar conexión
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{db}, nil
}

// Close cierra la conexión a la base de datos
func (db *DB) Close() error {
	return db.DB.Close()
}

// WithTx ejecuta una función dentro de una transacción
func (db *DB) WithTx(fn func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	err = fn(tx)
	return err
}
