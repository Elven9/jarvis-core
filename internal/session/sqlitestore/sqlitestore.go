// Package sqlitestore is a session.Store on SQLite.
package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Elven9/jarvis-core/internal/blob"
	"github.com/Elven9/jarvis-core/internal/session"
)

// Store keeps session records and the upload history in the sessions and uploads tables.
// Times are stored as unix milliseconds.
type Store struct {
	db *sql.DB
}

var _ session.Store = (*Store)(nil)

// New returns a store on db, which must have the schema from the migrations package
// (see sqlite.Open).
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// RecordUpload upserts the session and appends the upload in one transaction.
func (s *Store) RecordUpload(ctx context.Context, u session.Upload) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("record upload %s: %w", u.ID, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	hash, at := u.Blob.Hash.String(), u.UploadedAt.UnixMilli()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO sessions (type, machine_id, session_id, blob_hash, size, first_uploaded_at, last_uploaded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (type, machine_id, session_id) DO UPDATE SET
			blob_hash = excluded.blob_hash,
			size = excluded.size,
			last_uploaded_at = excluded.last_uploaded_at`,
		string(u.ID.Type), u.ID.MachineID, u.ID.SessionID, hash, u.Blob.Size, at, at)
	if err != nil {
		return fmt.Errorf("record upload %s: upsert session: %w", u.ID, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO uploads (type, machine_id, session_id, blob_hash, size, uploaded_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		string(u.ID.Type), u.ID.MachineID, u.ID.SessionID, hash, u.Blob.Size, at)
	if err != nil {
		return fmt.Errorf("record upload %s: append history: %w", u.ID, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("record upload %s: commit: %w", u.ID, err)
	}
	return nil
}

// Get returns the session's current record.
func (s *Store) Get(ctx context.Context, id session.ID) (session.Record, error) {
	var (
		hash        string
		size        int64
		first, last int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT blob_hash, size, first_uploaded_at, last_uploaded_at
		FROM sessions WHERE type = ? AND machine_id = ? AND session_id = ?`,
		string(id.Type), id.MachineID, id.SessionID).Scan(&hash, &size, &first, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return session.Record{}, fmt.Errorf("get %s: %w", id, session.ErrNotFound)
	}
	if err != nil {
		return session.Record{}, fmt.Errorf("get %s: %w", id, err)
	}
	h, err := parseHash(hash)
	if err != nil {
		return session.Record{}, fmt.Errorf("get %s: %w", id, err)
	}
	return session.Record{
		ID:              id,
		Blob:            blob.Info{Hash: h, Size: size},
		FirstUploadedAt: time.UnixMilli(first).UTC(),
		LastUploadedAt:  time.UnixMilli(last).UTC(),
	}, nil
}

// History returns the session's uploads in the order they were recorded.
func (s *Store) History(ctx context.Context, id session.ID) ([]session.Upload, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT blob_hash, size, uploaded_at
		FROM uploads WHERE type = ? AND machine_id = ? AND session_id = ?
		ORDER BY id`,
		string(id.Type), id.MachineID, id.SessionID)
	if err != nil {
		return nil, fmt.Errorf("history %s: %w", id, err)
	}
	defer func() { _ = rows.Close() }()

	var out []session.Upload
	for rows.Next() {
		var (
			hash string
			size int64
			at   int64
		)
		if err := rows.Scan(&hash, &size, &at); err != nil {
			return nil, fmt.Errorf("history %s: %w", id, err)
		}
		h, err := parseHash(hash)
		if err != nil {
			return nil, fmt.Errorf("history %s: %w", id, err)
		}
		out = append(out, session.Upload{ID: id, Blob: blob.Info{Hash: h, Size: size}, UploadedAt: time.UnixMilli(at).UTC()})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("history %s: %w", id, err)
	}
	return out, nil
}

func parseHash(s string) (blob.Hash, error) {
	var h blob.Hash
	if len(s) != hex.EncodedLen(len(h)) {
		return h, fmt.Errorf("stored blob hash %q has the wrong length", s)
	}
	if _, err := hex.Decode(h[:], []byte(s)); err != nil {
		return h, fmt.Errorf("stored blob hash %q: %w", s, err)
	}
	return h, nil
}
