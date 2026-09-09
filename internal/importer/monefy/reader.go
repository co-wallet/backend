package monefy

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"modernc.org/sqlite"
)

// Parse copies a bounded upload to a private temporary file, opens it read-only,
// validates the complete export, and removes the copy before returning.
// The caller must also bound HTTP body reads with a request/read timeout.
func Parse(ctx context.Context, src io.Reader, opts Options) (report Report, err error) {
	report.Deleted = make(map[string]int)
	defer func() {
		if err != nil {
			report.diagnostic(Blocking, "parse_failed", "", "", "Файл не прошёл проверку; частичный результат нельзя импортировать")
		}
	}()
	if len(opts.SupportedCurrencies) == 0 {
		return report, &Error{Code: "configuration", Cause: errors.New("не задан список валют сервера")}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	f, err := os.CreateTemp("", "cowallet-monefy-*.db")
	if err != nil {
		return report, &Error{Code: "io", Cause: err}
	}
	defer func() { err = errors.Join(err, os.Remove(f.Name())) }()
	n, copyErr := io.Copy(f, io.LimitReader(&contextReader{ctx, src}, MaxFileBytes+1))
	closeErr := f.Close()
	if err = errors.Join(copyErr, closeErr); err != nil {
		return report, &Error{Code: "io", Cause: err}
	}
	if n > MaxFileBytes {
		return report, &Error{Code: "limit", Cause: errors.New("файл превышает 64 MiB")}
	}
	header := make([]byte, 20)
	headerFile, err := os.Open(f.Name())
	if err != nil {
		return report, &Error{Code: "io", Cause: err}
	}
	_, readErr := io.ReadFull(headerFile, header)
	if err = errors.Join(readErr, headerFile.Close()); err != nil {
		return report, &Error{Code: "format", Cause: err}
	}
	if !bytes.Equal(header[:16], []byte("SQLite format 3\x00")) {
		return report, &Error{Code: "format", Cause: errors.New("нет заголовка SQLite 3")}
	}
	if header[18] != 1 || header[19] != 1 {
		return report, &Error{Code: "format", Cause: errors.New("нужен автономный экспорт SQLite без WAL-файлов")}
	}
	u := url.URL{Scheme: "file", Path: f.Name()}
	u.RawQuery = "mode=ro&immutable=1&_pragma=query_only(1)&_pragma=trusted_schema(0)"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return report, &Error{Code: "format", Cause: err}
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		return report, &Error{Code: "format", Cause: err}
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	// SQLITE_LIMIT_LENGTH and SQLITE_LIMIT_SQL_LENGTH bound hostile values/schema.
	for _, id := range []int{0, 1} {
		if _, err = sqlite.Limit(conn, id, MaxTextBytes*2); err != nil {
			return report, &Error{Code: "format", Cause: err}
		}
	}
	if err = conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&report.Version); err != nil {
		return report, &Error{Code: "format", Cause: err}
	}
	if report.Version != 11 {
		return report, &Error{Code: "version", Cause: fmt.Errorf("поддерживается версия 11, получена %d", report.Version)}
	}
	var integrity string
	if err = conn.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&integrity); err != nil {
		return report, &Error{Code: "format", Cause: err}
	}
	if integrity != "ok" {
		return report, &Error{Code: "format", Cause: errors.New("проверка целостности SQLite не пройдена")}
	}
	tables, err := readTables(ctx, conn, &report)
	if err != nil {
		return report, err
	}
	if err = normalize(tables, opts, &report); err != nil {
		return report, err
	}
	if err = ctx.Err(); err != nil {
		return report, &Error{Code: "timeout", Cause: err}
	}
	return report, nil
}

type contextReader struct {
	ctx context.Context
	src io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.src.Read(p)
}

type tableSpec struct{ name, columns string }

var schema = []tableSpec{
	{"Account", "Id,Name,Icon,CreatedOn,InitialBalanceCents,IsIncludedInTotalBalance,CurrencyId,DisabledOn,DeletedOn"},
	{"Category", "Id,Name,CategoryType,Icon,DisabledOn,DeletedOn"},
	{"Transaction", "Id,CategoryId,AccountId,AmountCents,CreatedOn,Note,ScheduleId,DeletedOn"},
	{"Transfer", "Id,AccountFromId,AccountToId,AmountCents,CreatedOn,Note,DeletedOn"},
	{"Currency", "Id,Name,AlphabeticCode,NumericCode,MinorUnits,IsBase,Symbol"},
	{"CurrencyRate", "Id,CurrencyFromId,CurrencyToId,RateCents,RateDate,CreatedOn,DeletedOn"},
	{"Schedule", "Id,ReminderType,CreatedOn,StartOn,EndOn,EntityId,ScheduleType,ExtendedData,DeletedOn"},
	{"Setting", "Id,Value"},
}

func readTables(ctx context.Context, conn *sql.Conn, report *Report) (map[string][]record, error) {
	known := make(map[string]bool)
	for _, spec := range schema {
		if e := checkColumns(ctx, conn, spec, report); e != nil {
			return nil, e
		}
		known[spec.name] = true
	}
	objects, err := conn.QueryContext(ctx, "SELECT name, type FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, &Error{Code: "schema", Cause: err}
	}
	for objects.Next() {
		var name, kind string
		if err = objects.Scan(&name, &kind); err != nil {
			break
		}
		if known[name] && kind != "table" {
			err = fmt.Errorf("%s должен быть таблицей", name)
			break
		}
		if !known[name] && kind != "index" {
			report.Diagnostics = append(report.Diagnostics, Diagnostic{Blocking, "unsupported_schema", name, "", "Неизвестный объект схемы"})
		}
	}
	err = errors.Join(err, objects.Err(), objects.Close())
	if err != nil {
		return nil, &Error{Code: "schema", Cause: err}
	}
	tables := make(map[string][]record)
	remaining := MaxRows
	for _, spec := range schema {
		rows, e := conn.QueryContext(ctx, `SELECT `+spec.columns+` FROM "`+spec.name+`" ORDER BY Id LIMIT ?`, remaining+1)
		if e != nil {
			return nil, &Error{Code: "schema", Entity: spec.name, Cause: e}
		}
		columns := strings.Split(spec.columns, ",")
		seen := make(map[any]bool)
		for rows.Next() {
			if remaining == 0 {
				e = errors.New("превышен лимит 100000 строк")
				break
			}
			remaining--
			values, dest := make([]any, len(columns)), make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if e = rows.Scan(dest...); e != nil {
				break
			}
			r := record{table: spec.name, values: make(map[string]any)}
			for i, value := range values {
				switch v := value.(type) {
				case nil, int64:
				case string:
					if len(v) > MaxTextBytes || !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
						e = errors.New("некорректная или слишком длинная строка")
					}
				default:
					e = errors.New("ожидались INTEGER, TEXT или NULL")
				}
				r.values[columns[i]] = value
			}
			if e != nil {
				break
			}
			if id := r.values["Id"]; id == nil || seen[id] {
				e = errors.New("пустой или повторный Id")
				break
			} else {
				seen[id] = true
			}
			tables[spec.name] = append(tables[spec.name], r)
		}
		e = errors.Join(e, rows.Err(), rows.Close())
		if e != nil {
			code := "data"
			if remaining == 0 {
				code = "limit"
			}
			return nil, &Error{Code: code, Entity: spec.name, Cause: e}
		}
	}
	return tables, nil
}

func checkColumns(ctx context.Context, conn *sql.Conn, spec tableSpec, report *Report) error {
	allowed := map[string]bool{"LocalHashCode": true, "RemoteHashCode": true}
	for _, column := range strings.Split(spec.columns, ",") {
		allowed[column] = true
	}
	rows, err := conn.QueryContext(ctx, `PRAGMA table_info("`+spec.name+`")`)
	if err != nil {
		return &Error{Code: "schema", Entity: spec.name, Cause: err}
	}
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			break
		}
		if !allowed[name] {
			report.diagnostic(Blocking, "unsupported_column", spec.name, "", "Неподдерживаемое поле: "+name)
		}
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return &Error{Code: "schema", Entity: spec.name, Cause: err}
	}
	return nil
}

type record struct {
	table  string
	values map[string]any
	err    error
}

func (r *record) fail(field string) {
	if r.err == nil {
		r.err = &Error{Code: "data", Entity: r.table, SourceID: fmt.Sprint(r.values["Id"]), Cause: fmt.Errorf("некорректное поле %s", field)}
	}
}
func (r *record) integer(field string) int64 {
	v, ok := r.values[field].(int64)
	if !ok {
		r.fail(field)
	}
	return v
}
func (r *record) text(field string, nullable bool) string {
	if nullable && r.values[field] == nil {
		return ""
	}
	v, ok := r.values[field].(string)
	if !ok || (!nullable && strings.TrimSpace(v) == "") {
		r.fail(field)
	}
	return v
}
func (r *record) flag(field string) bool {
	v := r.integer(field)
	if v != 0 && v != 1 {
		r.fail(field)
	}
	return v == 1
}
func (r *record) date(field string) time.Time {
	v := r.integer(field)
	if v < 0 || v > 3155378975999999999 {
		r.fail(field)
		return time.Time{}
	}
	return time.Unix(v/10000000-62135596800, v%10000000*100).UTC()
}
func (r *record) optionalDate(field string) *time.Time {
	if r.values[field] == nil {
		return nil
	}
	v := r.date(field)
	return &v
}
