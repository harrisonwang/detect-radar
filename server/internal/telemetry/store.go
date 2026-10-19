// telemetry 使用 SQLite 保存完整的内部扫描记录。
// 公开扫描查询仍由短期接口缓存提供。
package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const schemaVersion = 1

const (
	DefaultScanRetention     = 90 * 24 * time.Hour
	DefaultFeedbackRetention = 90 * 24 * time.Hour
)

// Options 控制新增记录的过期时间和定期清理。
// 关闭清理不会清除已有的过期时间，命令行工具也遵循此规则。
type Options struct {
	Path              string
	RetentionEnabled  bool
	ScanRetention     time.Duration
	FeedbackRetention time.Duration
}

// Event 保留完整的事件 JSON。InstanceID 将事件绑定到服务端扫描实例，
// 避免客户端复用 scan_id 时关联错误。可选的 Key 用于 DNS 重试去重，反馈不设置此值。
type Event struct {
	InstanceID string
	Key        string
	Payload    json.RawMessage
}

type Store struct {
	db       *sql.DB
	options  Options
	readOnly bool
	build    string
}

const schema = `
CREATE TABLE IF NOT EXISTS scans (
 instance_id TEXT PRIMARY KEY NOT NULL,
 client_scan_id TEXT NOT NULL,
 created_at INTEGER NOT NULL,
 expires_at INTEGER,
 initial_event_id TEXT NOT NULL UNIQUE,
 initial_score INTEGER,
 initial_risk_level TEXT,
 score INTEGER,
 risk_level TEXT,
 analysis_json TEXT,
 diagnosis_json TEXT,
 intel_json TEXT,
 latest_at INTEGER NOT NULL,
 latest_seq INTEGER NOT NULL DEFAULT 0,
 dns_status TEXT NOT NULL DEFAULT 'not_started',
 dns_status_at INTEGER NOT NULL,
 dns_status_seq INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_scans_client_id ON scans(client_scan_id);
CREATE INDEX IF NOT EXISTS idx_scans_expiry ON scans(expires_at);
CREATE TABLE IF NOT EXISTS scan_events (
 seq INTEGER PRIMARY KEY AUTOINCREMENT,
 event_id TEXT NOT NULL UNIQUE,
 event_key TEXT UNIQUE,
 event_type TEXT NOT NULL CHECK(event_type IN ('scan','dns_leak','dns_update','dns_started','dns_expired','feedback')),
 instance_id TEXT REFERENCES scans(instance_id) ON DELETE SET NULL,
 client_scan_id TEXT NOT NULL,
 association_status TEXT NOT NULL CHECK(association_status IN ('matched','unmatched','ambiguous','expired')),
 occurred_at INTEGER NOT NULL,
 expires_at INTEGER,
 payload_json TEXT NOT NULL CHECK(json_valid(payload_json)),
 schema_version INTEGER NOT NULL,
 build_revision TEXT NOT NULL,
 ingested_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_scan_events_instance ON scan_events(instance_id,seq);
CREATE INDEX IF NOT EXISTS idx_scan_events_kind_time ON scan_events(event_type,occurred_at);
CREATE INDEX IF NOT EXISTS idx_scan_events_expiry ON scan_events(expires_at);
CREATE VIEW IF NOT EXISTS feedback AS
 SELECT *,json_extract(payload_json,'$.category') AS category,
 json_extract(payload_json,'$.note') AS note,
 json_extract(payload_json,'$.context') AS context
 FROM scan_events
 WHERE event_type='feedback' AND
 (expires_at IS NULL OR expires_at>CAST(strftime('%s','now') AS INTEGER)*1000);
`

func Open(options Options) (*Store, error) {
	if options.Path == "" {
		return nil, errors.New("扫描存储：数据库路径不能为空")
	}
	if options.ScanRetention < 0 || options.FeedbackRetention < 0 {
		return nil, errors.New("扫描存储：保留时长不能为负数")
	}
	if options.ScanRetention == 0 {
		options.ScanRetention = DefaultScanRetention
	}
	if options.FeedbackRetention == 0 {
		options.FeedbackRetention = DefaultFeedbackRetention
	}
	if err := os.MkdirAll(filepath.Dir(options.Path), 0o750); err != nil {
		return nil, fmt.Errorf("扫描存储：创建目录失败：%w", err)
	}
	s, err := open(options, false)
	if err != nil {
		return nil, err
	}
	if err = s.initialize(context.Background()); err != nil {
		_ = s.Close()
		return nil, err
	}
	if options.RetentionEnabled {
		if err = s.backfillExpiry(context.Background()); err != nil {
			_ = s.Close()
			return nil, err
		}
	}
	return s, nil
}

// OpenReadOnly 只读取数据库，不创建文件或修改表结构、过期时间及持久化配置。
// WAL 模式下读取的是已提交的一致状态。
func OpenReadOnly(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("扫描存储：数据库路径不能为空")
	}
	s, err := open(Options{Path: path}, true)
	if err != nil {
		return nil, err
	}
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		_ = s.Close()
		if err != nil {
			return nil, fmt.Errorf("扫描存储：读取表结构版本失败：%w", err)
		}
		return nil, fmt.Errorf("扫描存储：不支持的表结构版本 %d", version)
	}
	return s, nil
}

func open(options Options, readOnly bool) (*Store, error) {
	abs, err := filepath.Abs(options.Path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	if readOnly {
		q.Set("mode", "ro")
		q.Add("_pragma", "query_only(1)")
	} else {
		q.Set("mode", "rwc")
		q.Add("_pragma", "journal_mode(WAL)")
		q.Add("_pragma", "synchronous(FULL)")
	}
	// modernc.org/sqlite 通过重复的 _pragma 参数配置数据库，
	// 不支持 _journal_mode 和 _synchronous 参数。
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("扫描存储：打开数据库失败：%w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db, options: options, readOnly: readOnly, build: buildRevision()}
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("扫描存储：连接数据库失败：%w", err)
	}
	if !readOnly {
		var mode string
		var syncMode, foreignKeys, timeout int
		checks := []struct {
			query  string
			target any
		}{
			{"PRAGMA journal_mode", &mode}, {"PRAGMA synchronous", &syncMode},
			{"PRAGMA foreign_keys", &foreignKeys}, {"PRAGMA busy_timeout", &timeout},
		}
		for _, check := range checks {
			if err = db.QueryRow(check.query).Scan(check.target); err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("扫描存储：校验数据库配置失败：%w", err)
			}
		}
		if strings.ToLower(mode) != "wal" || syncMode != 2 || foreignKeys != 1 || timeout != 5000 {
			_ = db.Close()
			return nil, fmt.Errorf("扫描存储：必要配置未生效（日志模式=%s，同步级别=%d，外键约束=%d，繁忙等待毫秒数=%d）", mode, syncMode, foreignKeys, timeout)
		}
	}
	return s, nil
}

func (s *Store) initialize(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("扫描存储：读取表结构版本失败：%w", err)
	}
	if version != 0 && version != schemaVersion {
		return fmt.Errorf("扫描存储：不支持的表结构版本 %d", version)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("扫描存储：初始化表结构失败：%w", err)
	}
	if _, err = tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Append(ctx context.Context, event Event) error {
	if s.readOnly {
		return errors.New("扫描存储：数据库为只读模式")
	}
	p, err := parse(event.Payload)
	if err != nil {
		return err
	}
	if p.Event == "scan" && event.InstanceID == "" {
		return errors.New("扫描存储：扫描实例 ID 不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if event.Key != "" {
		var exists int
		err = tx.QueryRowContext(ctx, "SELECT 1 FROM scan_events WHERE event_key=?", event.Key).Scan(&exists)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	eventID := uuid.NewString()
	var instance any
	var expires any
	association := "unmatched"
	if p.Event == "scan" {
		instance = event.InstanceID
		association = "matched"
		if s.options.RetentionEnabled {
			expires = p.at + s.options.ScanRetention.Milliseconds()
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO scans(instance_id,client_scan_id,created_at,expires_at,initial_event_id,initial_score,initial_risk_level,score,risk_level,analysis_json,diagnosis_json,intel_json,latest_at,dns_status_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.InstanceID, p.ScanID, p.at, expires, eventID, p.Score, nullString(p.RiskLevel), p.Score, nullString(p.RiskLevel), jsonValue(p.Analysis), jsonValue(p.Diagnosis), jsonValue(p.Intel), p.at, p.at)
		if err != nil {
			return fmt.Errorf("扫描存储：写入扫描记录失败：%w", err)
		}
	} else {
		if p.BindingStatus == "ambiguous" {
			association = "ambiguous"
		}
		if event.InstanceID != "" {
			var parentExpiry sql.NullInt64
			var parentClient string
			err = tx.QueryRowContext(ctx, "SELECT expires_at,client_scan_id FROM scans WHERE instance_id=?", event.InstanceID).Scan(&parentExpiry, &parentClient)
			if err == nil {
				if parentClient != p.ScanID {
					return errors.New("扫描存储：扫描实例与客户端扫描 ID 不匹配")
				}
				instance = event.InstanceID
				association = "matched"
				if parentExpiry.Valid {
					expires = parentExpiry.Int64
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if p.Event == "feedback" {
			expires = nil
			if s.options.RetentionEnabled {
				expires = p.at + s.options.FeedbackRetention.Milliseconds()
			}
		} else if instance == nil && s.options.RetentionEnabled {
			expires = p.at + s.options.ScanRetention.Milliseconds()
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO scan_events(event_id,event_key,event_type,instance_id,client_scan_id,association_status,occurred_at,expires_at,payload_json,schema_version,build_revision,ingested_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, eventID, nullString(event.Key), p.Event, instance, p.ScanID, association, p.at, expires, string(event.Payload), schemaVersion, s.build, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("扫描存储：写入事件失败：%w", err)
	}
	seq, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if instance != nil {
		if err = updateProjection(ctx, tx, event.InstanceID, p, seq); err != nil {
			return fmt.Errorf("扫描存储：更新扫描汇总失败：%w", err)
		}
	}
	return tx.Commit()
}

type payload struct {
	Event         string          `json:"event"`
	TS            string          `json:"ts"`
	ScanID        string          `json:"scan_id"`
	Score         *int            `json:"score"`
	RiskLevel     string          `json:"risk_level"`
	Analysis      json.RawMessage `json:"analysis"`
	Diagnosis     json.RawMessage `json:"diagnosis"`
	Intel         json.RawMessage `json:"intel"`
	BindingStatus string          `json:"binding_status"`
	at            int64
}

func parse(raw []byte) (payload, error) {
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("扫描存储：事件 JSON 无效：%w", err)
	}
	switch p.Event {
	case "scan", "dns_leak", "dns_update", "dns_started", "dns_expired", "feedback":
	default:
		return p, fmt.Errorf("扫描存储：未知事件类型 %q", p.Event)
	}
	t, err := time.Parse(time.RFC3339Nano, p.TS)
	if err != nil {
		return p, fmt.Errorf("扫描存储：事件时间戳无效：%w", err)
	}
	p.at = t.UnixMilli()
	return p, nil
}

func updateProjection(ctx context.Context, tx *sql.Tx, instanceID string, p payload, seq int64) error {
	if p.Event == "scan" {
		_, err := tx.ExecContext(ctx, "UPDATE scans SET latest_seq=?,dns_status_seq=? WHERE instance_id=?", seq, seq, instanceID)
		return err
	}
	if (p.Event == "dns_leak" || p.Event == "dns_update") && (p.Score != nil || p.RiskLevel != "") {
		// 迟到的观测结果不能覆盖更新的汇总状态，缺失评分也不能清除已有评分。
		if _, err := tx.ExecContext(ctx, `UPDATE scans SET score=COALESCE(?,score),risk_level=COALESCE(?,risk_level),analysis_json=COALESCE(?,analysis_json),diagnosis_json=COALESCE(?,diagnosis_json),latest_at=?,latest_seq=? WHERE instance_id=? AND (latest_at<? OR (latest_at=? AND latest_seq<?))`, p.Score, nullString(p.RiskLevel), jsonValue(p.Analysis), jsonValue(p.Diagnosis), p.at, seq, instanceID, p.at, p.at, seq); err != nil {
			return err
		}
	}
	status := ""
	switch p.Event {
	case "dns_started":
		status = "pending"
	case "dns_leak":
		status = "observed"
	case "dns_expired":
		status = "expired"
	}
	if status != "" {
		_, err := tx.ExecContext(ctx, `UPDATE scans SET dns_status=?,dns_status_at=?,dns_status_seq=? WHERE instance_id=? AND (dns_status_at<? OR (dns_status_at=? AND dns_status_seq<?)) AND (?<>'expired' OR dns_status='pending') AND (?<>'pending' OR dns_status<>'observed')`, status, p.at, seq, instanceID, p.at, p.at, seq, status, status)
		return err
	}
	return nil
}

func (s *Store) ResolveInstance(ctx context.Context, clientID string) (string, string, error) {
	if clientID == "" {
		return "", "unmatched", nil
	}
	rows, err := s.db.QueryContext(ctx, "SELECT instance_id FROM scans WHERE client_scan_id=? LIMIT 2", clientID)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return "", "", err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return "", "", err
	}
	switch len(ids) {
	case 0:
		return "", "unmatched", nil
	case 1:
		return ids[0], "matched", nil
	default:
		return "", "ambiguous", nil
	}
}

// Walk 读取完整事件用于回放。遍历期间占用唯一连接，回调不能再次调用此 Store。
func (s *Store) Walk(ctx context.Context, visit func([]byte) error) error {
	rows, err := s.db.QueryContext(ctx, "SELECT payload_json FROM scan_events ORDER BY seq")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		if err = visit(raw); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) backfillExpiry(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE scans SET expires_at=created_at+? WHERE expires_at IS NULL", s.options.ScanRetention.Milliseconds()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE scan_events SET expires_at=CASE WHEN event_type='feedback' THEN occurred_at+? ELSE COALESCE((SELECT expires_at FROM scans WHERE scans.instance_id=scan_events.instance_id),occurred_at+?) END WHERE expires_at IS NULL`, s.options.FeedbackRetention.Milliseconds(), s.options.ScanRetention.Milliseconds()); err != nil {
		return err
	}
	return tx.Commit()
}

// Prune 每次最多清理 1000 条过期事件。非反馈事件清理完后才删除对应扫描汇总。
// 反馈仅解除扫描关联，保留自身过期时间、原始客户端扫描 ID 和完整 JSON 上下文。
func (s *Store) Prune(ctx context.Context, now time.Time) (int64, error) {
	if s.readOnly {
		return 0, errors.New("扫描存储：数据库为只读模式")
	}
	if !s.options.RetentionEnabled {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM scan_events WHERE seq IN (SELECT seq FROM scan_events WHERE expires_at IS NOT NULL AND expires_at<=? ORDER BY expires_at,seq LIMIT 1000)`, now.UnixMilli())
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE scan_events SET association_status='expired' WHERE event_type='feedback' AND instance_id IN (SELECT instance_id FROM scans WHERE expires_at<=? AND NOT EXISTS(SELECT 1 FROM scan_events e WHERE e.instance_id=scans.instance_id AND e.event_type<>'feedback'))`, now.UnixMilli()); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM scans WHERE expires_at IS NOT NULL AND expires_at<=? AND NOT EXISTS(SELECT 1 FROM scan_events e WHERE e.instance_id=scans.instance_id AND e.event_type<>'feedback')`, now.UnixMilli()); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func jsonValue(value json.RawMessage) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return string(value)
}
func buildRevision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
		if info.Main.Version != "" {
			return info.Main.Version
		}
	}
	return "unknown"
}
