package ts

import (
	mysql "database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/timespacegroup/go-utils"
	"strings"
	"fmt"
)

type DBClient struct {
	Config DBConfig
	Db     *mysql.DB
}

const (
	MySQL                     = "MySQL"
	SlowSqlTimeoutMillisecond = 2000
	ConnDBTimeoutMillisecond  = 2000
)

func NewDbClient(config DBConfig) *DBClient {
	var client DBClient
	client.Config = config
	client.Db = getConn(config)
	return &client
}

type DBConfig struct {
	DbHost string
	DbUser string
	DbPass string
	DbName string
}

type ORM interface {
	Row2struct(row *mysql.Row)
	Rows2struct(rows *mysql.Rows)
}

/*
  go mysql connection string:

 */
func getDbConnString(config DBConfig) string {
	builder := tsgutils.NewStringBuilder()
	builder.Append(config.DbUser).Append(":").Append(config.DbPass)
	builder.Append("@tcp(").Append(config.DbHost).Append(":").Append("3306").Append(")/")
	builder.Append(config.DbName).Append("?").Append("charset=utf8")
	return builder.ToString()
}

func getConn(config DBConfig) *mysql.DB {
	dbConnString := getDbConnString(config)
	start := tsgutils.Millisecond()
	db, err := mysql.Open(strings.ToLower(MySQL), dbConnString)
	end := tsgutils.Millisecond()
	consume := end - start
	if consume > ConnDBTimeoutMillisecond {
		PrintSlowConn(MySQL, config.DbHost, config.DbName, consume)
	}
	tsgutils.CheckAndPrintError(MySQL+" connection failed, db conn string: \n"+dbConnString, err)
	return db
}

func (client *DBClient) getStmt(sql string) *mysql.Stmt {
	stmt, err := client.Db.Prepare(sql)
	tsgutils.CheckAndPrintError(MySQL+" prepare stmt failed", err)
	PrintErrorSql(MySQL, err, sql)
	return stmt
}
func (client *DBClient) CloseConn() {
	if client.Db != nil {
		defer client.Db.Close()
	}
}

func (client *DBClient) closeStmt(stmt *mysql.Stmt) {
	if stmt != nil {
		defer stmt.Close()
	}
}

func (client *DBClient) QueryMetaData(tabName string) *mysql.Rows {
	rows, err := client.Db.Query("SELECT * FROM " + tabName + " WHERE 1=1 LIMIT 1;")
	tsgutils.CheckAndPrintError(MySQL+" Query table meta data failed", err)
	return rows
}

func (client *DBClient) QueryRow(sql string, args []interface{}, orm ORM) {
	start := tsgutils.Millisecond()
	row := client.forkQuery(sql, args...)
	end := tsgutils.Millisecond()
	consume := end - start
	if consume > SlowSqlTimeoutMillisecond {
		PrintSlowSql(MySQL, client.Config.DbHost, client.Config.DbName, consume, sql, args...)
	}
	orm.Row2struct(row)
}

func (client *DBClient) QueryList(sql string, args []interface{}, orm ORM) {
	start := tsgutils.Millisecond()
	rows := client.forkQueryList(sql, args...)
	end := tsgutils.Millisecond()
	consume := end - start
	if consume > SlowSqlTimeoutMillisecond {
		PrintSlowSql(MySQL, client.Config.DbHost, client.Config.DbName, consume, sql, args...)
	}
	orm.Rows2struct(rows)
}

func (client *DBClient) forkQuery(sql string, args ...interface{}) *mysql.Row {
	stmt := client.getStmt(sql)
	var row *mysql.Row
	if len(args) > 0 {
		row = stmt.QueryRow(args)
	} else {
		row = stmt.QueryRow()
	}
	client.closeStmt(stmt)
	return row
}

func (client *DBClient) forkQueryList(sql string, args ...interface{}) *mysql.Rows {
	stmt := client.getStmt(sql)
	var rows *mysql.Rows
	var err error
	if len(args) > 0 {
		rows, err = stmt.Query(args)
	} else {
		rows, err = stmt.Query()
	}
	tsgutils.CheckAndPrintError(MySQL+" query rows list failed", err)
	PrintErrorSql(MySQL, err, sql, args)
	client.closeStmt(stmt)
	return rows
}

func (client *DBClient) TxBegin() *mysql.Tx {
	tx, err := client.Db.Begin()
	tsgutils.CheckAndPrintError(MySQL+" begin tx failed", err)
	return tx
}

func (client *DBClient) TxCommit(tx *mysql.Tx) {
	err := tx.Commit()
	tsgutils.CheckAndPrintError(MySQL+" commit tx failed", err)
}

func (client *DBClient) TxRollback(tx *mysql.Tx) {
	err := tx.Rollback()
	tsgutils.CheckAndPrintError(MySQL+" rollback tx failed", err)
}

func (client *DBClient) Exec(sql string, args []interface{}) int64 {
	stmt := client.getStmt(sql)
	start := tsgutils.Millisecond()
	result, err := stmt.Exec(args...)
	tsgutils.CheckAndPrintError(MySQL+" exec sql failed", err)
	PrintErrorSql(MySQL, err, sql, args)
	end := tsgutils.Millisecond()
	consume := end - start
	if consume > SlowSqlTimeoutMillisecond {
		PrintSlowSql(MySQL, client.Config.DbHost, client.Config.DbName, consume, sql, args...)
	}
	client.closeStmt(stmt)
	var intResult int64
	fmt.Println(result)
	if tsgutils.NewString(sql).ContainsIgnoreCase("INSERT") {
		intResult, err = result.LastInsertId()
		tsgutils.CheckAndPrintError(MySQL+" exec and get last insert id failed", err)
		PrintErrorSql(MySQL, err, sql, args...)
	} else {
		intResult, err = result.RowsAffected()
		tsgutils.CheckAndPrintError(MySQL+" exec and get rows affected failed", err)
		PrintErrorSql(MySQL, err, sql, args...)
	}
	client.closeStmt(stmt)
	return intResult
}