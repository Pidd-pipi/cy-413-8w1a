package repository

import "gorm.io/gorm"

// TxManager 封装数据库事务，让 service 层可以在一次事务里同时操作多个仓储。
type TxManager interface {
	InTx(func(TxScope) error) error
}

// TxScope 是事务作用域，用于构造绑定到同一事务的仓储。
type TxScope interface {
	JournalRepo() JournalRepository
	MoodRepo() MoodRepository
}

type gormTxManager struct{ db *gorm.DB }

func NewTxManager(db *gorm.DB) TxManager { return &gormTxManager{db} }

func (m *gormTxManager) InTx(fn func(TxScope) error) error {
	return m.db.Transaction(func(tx *gorm.DB) error {
		return fn(&gormTxScope{tx: tx})
	})
}

type gormTxScope struct{ tx *gorm.DB }

func (s *gormTxScope) JournalRepo() JournalRepository { return NewJournalRepository(s.tx) }
func (s *gormTxScope) MoodRepo() MoodRepository       { return NewMoodRepository(s.tx) }
