package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type DB struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, dsn string) (*DB, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	config.MaxConns = 30
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (d *DB) Migrate(ctx context.Context) error {
	tx, err := d.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var applied bool
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(735091002)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, schemaV1); err != nil {
		return fmt.Errorf("apply schema v1: %w", err)
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=1)`).Scan(&applied)
	if err != nil {
		return err
	}
	if !applied {
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES(1)`); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (d *DB) BootstrapAdmin(ctx context.Context, username, password string) error {
	var id uuid.UUID
	err := d.Pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(username)=lower($1)`, username).Scan(&id)
	if err == nil {
		var hasRole bool
		if err = d.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles WHERE user_id=$1 AND role_id='system_admin')`, id).Scan(&hasRole); err != nil {
			return err
		}
		if !hasRole {
			return errors.New("bootstrap username exists without system_admin role; refusing privilege escalation")
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	id = uuid.New()
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO users(id,username,display_name,password_hash,must_change_password) VALUES($1,$2,$2,$3,true)`, id, username, string(hash))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,'system_admin')`, id)
	if err != nil {
		return err
	}
	details, _ := json.Marshal(map[string]any{"bootstrap": true})
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,actor_id,actor_name,event_type,resource_type,resource_id,action,details) VALUES($1,$2,$3,'Create','user',$4,'bootstrap_admin',$5)`, uuid.New(), id, username, id.String(), details)
	if err != nil {
		return err
	}
	slog.Info("bootstrap administrator created", "username", username)
	return tx.Commit(ctx)
}

func (d *DB) Ready(ctx context.Context) error { return d.Pool.Ping(ctx) }
