package repository

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Interface implemented by sqlx.DB and sqlx.TX to be used in transactions
type DBTX interface {
	Exec(string, ...interface{}) (sql.Result, error)
	NamedExec(query string, arg interface{}) (sql.Result, error)
	Get(dest interface{}, query string, args ...interface{}) error
	Select(dest interface{}, query string, args ...interface{}) error
}

type Transaction interface {
	Do(ctx context.Context, fn func(SqlRepo) error) error
	NextID(table string) (uint, error)
}

func (r *SqlRepo) NextID(table string) (uint, error) {
	var u uint
	if err := r.db.QueryRow("select nextval('$1_id_seq'::regclass);", table).Scan(&u); err != nil {
		return 0, err
	}
	return u, nil
}

func (r *SqlRepo) Do(ctx context.Context, fn func(SqlRepo) error) error {

	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = fn(SqlRepo{trans: tx})
	if err != nil {
		return err
	}
	return tx.Commit()

}

type SqlRepo struct {
	db    *sqlx.DB
	trans DBTX
}

func NewRepository(db *sqlx.DB) SqlRepo {
	return SqlRepo{
		db:    db,
		trans: db,
	}
}

func (r *SqlRepo) Conn(ctx context.Context) (*sql.Conn, error) {
	return r.db.DB.Conn(ctx)
}

// execDelete runs a delete/update query and returns sql.ErrNoRows if no rows were affected.
func execDelete(x DBTX, query string, args ...interface{}) error {
	res, err := x.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *SqlRepo) CheckFieldName(i any, colname string) error {
	if strings.ToLower(colname) == "id" {
		return fmt.Errorf("invalid column name: %s", colname)
	}
	rt := reflect.TypeOf(i)
	for i := 0; i < rt.NumField(); i++ {
		if string(rt.Field(i).Tag.Get("db")) == colname {
			return nil
		}
	}
	return fmt.Errorf("invalid column name: %s", colname)
}
