package repo

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type TaskRepository struct {
	mu sync.Mutex
	db *sql.DB
}

func NewTaskRepository(taskRoot string) (*TaskRepository, error) {
	if err := os.MkdirAll(taskRoot, 0o755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(taskRoot, "task.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	repo := &TaskRepository{db: db}
	if err := repo.initialize(); err != nil {
		db.Close()
		return nil, err
	}

	return repo, nil
}

func (r *TaskRepository) initialize() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
		PRAGMA temp_store = MEMORY;

		CREATE TABLE IF NOT EXISTS article_state (
			article_id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			error_message TEXT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_article_state_status ON article_state(status);
	`)
	return err
}

func (r *TaskRepository) IsCompleted(articleID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	var status string
	err := r.db.QueryRow("SELECT status FROM article_state WHERE article_id = ? LIMIT 1", articleID).Scan(&status)
	if err != nil {
		return false
	}
	return status == "completed"
}

func (r *TaskRepository) GetCompletedIDs() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make(map[string]bool)
	rows, err := r.db.Query("SELECT article_id FROM article_state WHERE status = 'completed'")
	if err != nil {
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			result[id] = true
		}
	}
	return result
}

func (r *TaskRepository) MarkStatus(articleID, status string, errorMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errVal interface{} = nil
	if errorMsg != "" {
		errVal = errorMsg
	}

	r.db.Exec(`
		INSERT INTO article_state(article_id, status, error_message, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(article_id) DO UPDATE SET
			status = excluded.status,
			error_message = excluded.error_message,
			updated_at = excluded.updated_at;
	`, articleID, status, errVal, time.Now().Format(time.RFC3339))
}

func (r *TaskRepository) Close() error {
	return r.db.Close()
}
