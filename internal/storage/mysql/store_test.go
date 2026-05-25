package mysql

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// newTestStore wires a *Store to a go-sqlmock-backed *sql.DB and returns
// the mock controller. QueryMatcherEqual matches SQL strings exactly, not
// as regex.
func newTestStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Store{db: db}, mock
}

func assertExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
