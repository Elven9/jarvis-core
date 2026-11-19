// Package sqlitestore is a metadata.Store on SQLite.
package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

// Store keeps session metadata in the session_metadata table and its child tables
// (tags, project, models, errors, metrics, tool metrics, cache misses).
// Times are stored as unix milliseconds.
type Store struct {
	db *sql.DB
}

var _ metadata.Store = (*Store)(nil)

// New returns a store on db, which must have the schema from the migrations package
// and foreign keys enabled (see sqlite.Open).
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// execer is what Replace writes through: the transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Replace checks that the session has a record, deletes its metadata (its child rows go with it, by cascade) and
// inserts m, in one transaction.
func (s *Store) Replace(ctx context.Context, m metadata.Meta) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace metadata %s: %w", m.ID, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = replace(ctx, tx, m); err != nil {
		return fmt.Errorf("replace metadata %s: %w", m.ID, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("replace metadata %s: commit: %w", m.ID, err)
	}
	return nil
}

func replace(ctx context.Context, tx execer, m metadata.Meta) error {
	k := key(m.ID)
	var recorded bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM sessions WHERE type = ? AND machine_id = ? AND session_id = ?)`, k...).
		Scan(&recorded); err != nil {
		return fmt.Errorf("look up session: %w", err)
	}
	if !recorded {
		return metadata.ErrNoSession
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM session_metadata WHERE type = ? AND machine_id = ? AND session_id = ?`, k...); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	md := m.Metadata
	var tagsSource *string
	var tagsAt *int64
	if md.Tags != nil {
		tagsSource, tagsAt = &md.Tags.Source, new(md.Tags.At.UnixMilli())
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO session_metadata (type, machine_id, session_id, parser_version, header_session_id, cwd,
			started_at, format_version, parent_session, agent_version, name, tags_source, tags_at, scenario,
			last_activity_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append(k, m.ParserVersion, md.SessionID, md.CWD, md.StartedAt.UnixMilli(), md.FormatVersion,
			md.ParentSession, md.AgentVersion, md.Name, tagsSource, tagsAt, md.Scenario,
			md.LastActivityAt.UnixMilli())...); err != nil {
		return fmt.Errorf("insert metadata: %w", err)
	}
	if p := md.Project; p != nil {
		src := p.Source
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO session_projects (type, machine_id, session_id, name, source_kind, source_dir,
				source_provider, source_repo, source_path, source_tag, source_commit, at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			append(k, p.Name, src.Kind, src.Dir, src.Provider, src.Repo, src.Path, src.Tag, src.Commit,
				p.At.UnixMilli())...); err != nil {
			return fmt.Errorf("insert project: %w", err)
		}
	}
	if md.Tags != nil {
		for i, tag := range md.Tags.Tags {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO session_tags (type, machine_id, session_id, position, tag) VALUES (?, ?, ?, ?, ?)`,
				append(k, i, tag)...); err != nil {
				return fmt.Errorf("insert tag: %w", err)
			}
		}
	}
	for i, mc := range md.Models {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO session_models (type, machine_id, session_id, position, provider, model_id, at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			append(k, i, mc.Provider, mc.ModelID, mc.At.UnixMilli())...); err != nil {
			return fmt.Errorf("insert model change: %w", err)
		}
	}
	for i, e := range md.Errors {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO session_errors (type, machine_id, session_id, position, message, at) VALUES (?, ?, ?, ?, ?, ?)`,
			append(k, i, e.Message, e.At.UnixMilli())...); err != nil {
			return fmt.Errorf("insert error: %w", err)
		}
	}
	if m.Metrics != nil {
		if err := insertMetrics(ctx, tx, k, m.Metrics); err != nil {
			return err
		}
	}
	return nil
}

func insertMetrics(ctx context.Context, tx execer, k []any, mt *sessionparser.Metrics) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO session_metrics (type, machine_id, session_id, tokens_input, tokens_output,
			tokens_cache_read, tokens_cache_write, cost_usd, turns, model_calls, ttft_ms_p50,
			latency_ms_p50, latency_ms_max, compactions, context_peak_percent, entries, entry_id, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append(k, mt.Tokens.Input, mt.Tokens.Output, mt.Tokens.CacheRead, mt.Tokens.CacheWrite,
			mt.CostUSD, mt.Turns, mt.Model.Calls, mt.Model.TTFTMsP50, mt.Model.LatencyMsP50,
			mt.Model.LatencyMsMax, mt.Compactions, mt.ContextPeakPercent, mt.Entries, mt.EntryID,
			mt.At.UnixMilli())...); err != nil {
		return fmt.Errorf("insert metrics: %w", err)
	}
	for name, tm := range mt.Tools {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO session_tool_metrics (type, machine_id, session_id, tool, calls, errors, ms_p50, ms_max)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			append(k, name, tm.Calls, tm.Errors, tm.MsP50, tm.MsMax)...); err != nil {
			return fmt.Errorf("insert tool metrics: %w", err)
		}
	}
	for i, cm := range mt.CacheMisses {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO session_cache_misses (type, machine_id, session_id, position, turn, expected, cache_read)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			append(k, i, cm.Turn, cm.Expected, cm.CacheRead)...); err != nil {
			return fmt.Errorf("insert cache miss: %w", err)
		}
	}
	return nil
}

// querier is what Get reads through: the transaction.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Get reads the session's metadata in one transaction, so it never sees half of a Replace.
func (s *Store) Get(ctx context.Context, id session.ID) (metadata.Meta, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return metadata.Meta{}, fmt.Errorf("get metadata %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }() // read only: nothing to commit
	m, err := get(ctx, tx, id)
	if err != nil {
		return metadata.Meta{}, fmt.Errorf("get metadata %s: %w", id, err)
	}
	return m, nil
}

func get(ctx context.Context, q querier, id session.ID) (metadata.Meta, error) {
	k := key(id)
	m := metadata.Meta{ID: id}
	md := &m.Metadata
	var (
		startedAt  int64
		activityAt int64
		tagsSource sql.Null[string]
		tagsAt     sql.Null[int64]
	)
	err := q.QueryRowContext(ctx, `
		SELECT parser_version, header_session_id, cwd, started_at, format_version,
			parent_session, agent_version, name, tags_source, tags_at, scenario,
			coalesce(last_activity_at, started_at)
		FROM session_metadata WHERE type = ? AND machine_id = ? AND session_id = ?`, k...).
		Scan(&m.ParserVersion, &md.SessionID, &md.CWD, &startedAt, &md.FormatVersion,
			&md.ParentSession, &md.AgentVersion, &md.Name, &tagsSource, &tagsAt, &md.Scenario, &activityAt)
	if errors.Is(err, sql.ErrNoRows) {
		return metadata.Meta{}, metadata.ErrNotFound
	}
	if err != nil {
		return metadata.Meta{}, err
	}
	md.StartedAt = fromMilli(startedAt)
	md.LastActivityAt = fromMilli(activityAt)

	if tagsSource.Valid {
		md.Tags = &sessionparser.Tags{Tags: []string{}, Source: tagsSource.V, At: fromMilli(tagsAt.V)}
		err := queryEach(ctx, q, `SELECT tag FROM session_tags
			WHERE type = ? AND machine_id = ? AND session_id = ? ORDER BY position`, k,
			func(rows *sql.Rows) error {
				var tag string
				if err := rows.Scan(&tag); err != nil {
					return err
				}
				md.Tags.Tags = append(md.Tags.Tags, tag)
				return nil
			})
		if err != nil {
			return metadata.Meta{}, fmt.Errorf("tags: %w", err)
		}
	}
	if md.Project, err = getProject(ctx, q, k); err != nil {
		return metadata.Meta{}, fmt.Errorf("project: %w", err)
	}
	err = queryEach(ctx, q, `SELECT provider, model_id, at FROM session_models
		WHERE type = ? AND machine_id = ? AND session_id = ? ORDER BY position`, k,
		func(rows *sql.Rows) error {
			var mc sessionparser.ModelChange
			var at int64
			if err := rows.Scan(&mc.Provider, &mc.ModelID, &at); err != nil {
				return err
			}
			mc.At = fromMilli(at)
			md.Models = append(md.Models, mc)
			return nil
		})
	if err != nil {
		return metadata.Meta{}, fmt.Errorf("models: %w", err)
	}
	err = queryEach(ctx, q, `SELECT message, at FROM session_errors
		WHERE type = ? AND machine_id = ? AND session_id = ? ORDER BY position`, k,
		func(rows *sql.Rows) error {
			var e sessionparser.AgentError
			var at int64
			if err := rows.Scan(&e.Message, &at); err != nil {
				return err
			}
			e.At = fromMilli(at)
			md.Errors = append(md.Errors, e)
			return nil
		})
	if err != nil {
		return metadata.Meta{}, fmt.Errorf("errors: %w", err)
	}

	if m.Metrics, err = getMetrics(ctx, q, k); err != nil {
		return metadata.Meta{}, fmt.Errorf("metrics: %w", err)
	}
	return m, nil
}

// getProject returns nil when the session has no project row.
func getProject(ctx context.Context, q querier, k []any) (*sessionparser.Project, error) {
	var (
		p  sessionparser.Project
		at int64
	)
	err := q.QueryRowContext(ctx, `
		SELECT name, source_kind, source_dir, source_provider, source_repo, source_path, source_tag,
			source_commit, at
		FROM session_projects WHERE type = ? AND machine_id = ? AND session_id = ?`, k...).
		Scan(&p.Name, &p.Source.Kind, &p.Source.Dir, &p.Source.Provider, &p.Source.Repo, &p.Source.Path,
			&p.Source.Tag, &p.Source.Commit, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.At = fromMilli(at)
	return &p, nil
}

// getMetrics returns nil when the session has no metrics row.
func getMetrics(ctx context.Context, q querier, k []any) (*sessionparser.Metrics, error) {
	var (
		mt sessionparser.Metrics
		at int64
	)
	err := q.QueryRowContext(ctx, `
		SELECT tokens_input, tokens_output, tokens_cache_read, tokens_cache_write, cost_usd, turns,
			model_calls, ttft_ms_p50, latency_ms_p50, latency_ms_max, compactions,
			context_peak_percent, entries, entry_id, at
		FROM session_metrics WHERE type = ? AND machine_id = ? AND session_id = ?`, k...).
		Scan(&mt.Tokens.Input, &mt.Tokens.Output, &mt.Tokens.CacheRead, &mt.Tokens.CacheWrite,
			&mt.CostUSD, &mt.Turns, &mt.Model.Calls, &mt.Model.TTFTMsP50, &mt.Model.LatencyMsP50,
			&mt.Model.LatencyMsMax, &mt.Compactions, &mt.ContextPeakPercent, &mt.Entries,
			&mt.EntryID, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	mt.At = fromMilli(at)

	err = queryEach(ctx, q, `SELECT tool, calls, errors, ms_p50, ms_max FROM session_tool_metrics
		WHERE type = ? AND machine_id = ? AND session_id = ?`, k,
		func(rows *sql.Rows) error {
			var name string
			var tm sessionparser.ToolMetrics
			if err := rows.Scan(&name, &tm.Calls, &tm.Errors, &tm.MsP50, &tm.MsMax); err != nil {
				return err
			}
			if mt.Tools == nil {
				mt.Tools = map[string]sessionparser.ToolMetrics{}
			}
			mt.Tools[name] = tm
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("tools: %w", err)
	}
	err = queryEach(ctx, q, `SELECT turn, expected, cache_read FROM session_cache_misses
		WHERE type = ? AND machine_id = ? AND session_id = ? ORDER BY position`, k,
		func(rows *sql.Rows) error {
			var cm sessionparser.CacheMiss
			if err := rows.Scan(&cm.Turn, &cm.Expected, &cm.CacheRead); err != nil {
				return err
			}
			mt.CacheMisses = append(mt.CacheMisses, cm)
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("cache misses: %w", err)
	}
	return &mt, nil
}

// List reads every session's summary in one query: its metadata joined with its session
// record (the upload time), its project, its metrics totals and its tags (as a JSON array in
// position order).
func (s *Store) List(ctx context.Context) ([]metadata.Summary, error) {
	out := []metadata.Summary{}
	err := queryEach(ctx, s.db, `
		SELECT m.type, m.machine_id, m.session_id, s.last_uploaded_at, m.name, m.agent_version,
			m.scenario, p.name, m.tags_source IS NOT NULL,
			(SELECT json_group_array(tag) FROM (
				SELECT tag FROM session_tags t
				WHERE t.type = m.type AND t.machine_id = m.machine_id AND t.session_id = m.session_id
				ORDER BY position)),
			mt.session_id IS NOT NULL,
			coalesce(mt.tokens_input, 0), coalesce(mt.tokens_output, 0),
			coalesce(mt.tokens_cache_read, 0), coalesce(mt.tokens_cache_write, 0),
			coalesce(mt.cost_usd, 0), coalesce(mt.turns, 0)
		FROM session_metadata m
		JOIN sessions s USING (type, machine_id, session_id)
		LEFT JOIN session_metrics mt USING (type, machine_id, session_id)
		LEFT JOIN session_projects p USING (type, machine_id, session_id)
		ORDER BY s.last_uploaded_at DESC, m.type, m.machine_id, m.session_id`, nil,
		func(rows *sql.Rows) error {
			var (
				sm                  metadata.Summary
				typ                 string
				at                  int64
				hasTags, hasMetrics bool
				tags                string
				ms                  metadata.MetricsSummary
			)
			if err := rows.Scan(&typ, &sm.ID.MachineID, &sm.ID.SessionID, &at, &sm.Name, &sm.AgentVersion,
				&sm.Scenario, &sm.Project, &hasTags, &tags, &hasMetrics,
				&ms.Tokens.Input, &ms.Tokens.Output, &ms.Tokens.CacheRead, &ms.Tokens.CacheWrite,
				&ms.CostUSD, &ms.Turns); err != nil {
				return err
			}
			sm.ID.Type = session.Type(typ)
			sm.UploadedAt = fromMilli(at)
			if hasTags {
				sm.Tags = []string{}
				if err := json.Unmarshal([]byte(tags), &sm.Tags); err != nil {
					return fmt.Errorf("tags of %s: %w", sm.ID, err)
				}
			}
			if hasMetrics {
				sm.Metrics = &ms
			}
			out = append(out, sm)
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return out, nil
}

// queryEach runs query and calls scan for every row.
func queryEach(ctx context.Context, q querier, query string, args []any, scan func(*sql.Rows) error) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// key is the session's primary key columns, in order (type, machine_id, session_id).
func key(id session.ID) []any {
	return []any{string(id.Type), id.MachineID, id.SessionID}
}

func fromMilli(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}
