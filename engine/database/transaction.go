package database

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

// C++ owners:
//   - TransactionTask::Execute / TransactionWithResultTask::Execute
//     (src/server/database/Database/Transaction.cpp): async transactions retry
//     the whole transaction on ER_LOCK_DEADLOCK (1213), serialized by the
//     process-wide TransactionTask::_deadlockLock, looping until
//     DEADLOCK_MAX_RETRY_TIME_MS (60000), warning per retry, then a fatal error
//     and no commit.
//   - DatabaseWorkerPool<T>::DirectCommitTransaction
//     (src/server/database/Database/DatabaseWorkerPool.cpp:305-334):
//     synchronous transactions re-execute ExecuteTransaction on
//     ER_LOCK_DEADLOCK up to 5 times (loopBreaker = 5) on the same connection,
//     then clean up without committing.
//
// Go has no async transaction queue (AsyncPool covers single queries/execs
// only), so every tx site is the synchronous case: RunTransaction mirrors
// DirectCommitTransaction. The 60-second serialized async retry has no Go
// submission path to attach to and is documented, not ported.

const (
	// deadlockRetryAttempts mirrors DatabaseWorkerPool<T>::DirectCommitTransaction
	// (DatabaseWorkerPool.cpp:305-334): the initial ExecuteTransaction plus up to
	// loopBreaker = 5 retries on ER_LOCK_DEADLOCK (DatabaseWorkerPool.cpp:321),
	// for 6 total attempts, after which it cleans up without committing.
	deadlockRetryAttempts = 6
)

// asyncDeadlockMu is the Go analog of TransactionTask::_deadlockLock
// (Transaction.cpp:27): when an async-style transaction path is added, its
// deadlock retries must serialize on this mutex so retrying threads do not
// keep deadlocking each other (Transaction.cpp:85).
// asyncDeadlockMu is the Go analog of TransactionTask::_deadlockLock
// (Transaction.cpp:27): when an async-style transaction submission path is
// added, its deadlock retries must serialize on this mutex so retrying
// threads do not keep deadlocking each other (Transaction.cpp:85).
var asyncDeadlockMu sync.Mutex

// deadlockRetryWindow mirrors Transaction.cpp's DEADLOCK_MAX_RETRY_TIME_MS
// (60000): total time an async-style deadlock retry loop may run. Reserved
// for the async transaction path; unused while all tx sites are synchronous.
const deadlockRetryWindow = 60 * time.Second

// IsDeadlockError reports whether err is MySQL errno 1213 (ER_LOCK_DEADLOCK),
// the only error either C++ retry path acts on. The SQLite backend cannot
// produce it, so the retry loop below is a no-op there.
func IsDeadlockError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1213
	}
	return false
}

// RunTransaction executes body inside a transaction and commits, re-executing
// the whole body on MySQL deadlock (errno 1213) up to deadlockRetryAttempts
// times — the Go analog of DatabaseWorkerPool<T>::DirectCommitTransaction
// (DatabaseWorkerPool.cpp:305-334), which retries ExecuteTransaction up to 5
// times on ER_LOCK_DEADLOCK and then cleans up without committing.
//
// Like its C++ counterpart, the body is replayed from scratch, so it must be
// pure database work: on retry every previous attempt has been rolled back,
// but in-memory or network side effects inside body would repeat. Callers
// whose failure path touches in-memory state keep their manual tx handling.
func (s *Store) RunTransaction(ctx context.Context, body func(context.Context, *sql.Tx) error) error {
	var err error
	for attempt := 0; attempt < deadlockRetryAttempts; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		err = s.runTransactionOnce(ctx, body)
		if err == nil || !IsDeadlockError(err) {
			return err
		}
	}
	return err
}

func (s *Store) runTransactionOnce(ctx context.Context, body func(context.Context, *sql.Tx) error) error {
	tx, err := s.Begin(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := body(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
