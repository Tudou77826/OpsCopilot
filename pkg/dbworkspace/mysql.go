package dbworkspace

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

type MySQL struct{}

func (MySQL) ConnectionID(ctx context.Context, c *sql.Conn) (string, error) {
	var id string
	err := c.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&id)
	return id, err
}

func (MySQL) Cancel(ctx context.Context, db *sql.DB, id string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, "KILL QUERY "+id)
	return err
}

func (MySQL) Schemas(ctx context.Context, c *sql.Conn) ([]string, error) {
	r, err := c.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out := []string{}
	for r.Next() {
		var name string
		if err = r.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, r.Err()
}

func (MySQL) Tables(ctx context.Context, c *sql.Conn, schema string) ([]Table, error) {
	r, err := c.QueryContext(ctx, "SELECT TABLE_NAME, TABLE_TYPE, TABLE_COMMENT FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME LIMIT 10001", schema)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out := []Table{}
	for r.Next() {
		var t Table
		if err = r.Scan(&t.Name, &t.Type, &t.Comment); err != nil {
			return nil, err
		}
		out = append(out, t)
		if len(out) > 10000 {
			return nil, fmt.Errorf("对象超过一万项，请缩小数据库范围")
		}
	}
	return out, r.Err()
}

func quoteIdentifier(v string) string { return "`" + strings.ReplaceAll(v, "`", "``") + "`" }
func qualified(schema, table string) string {
	return quoteIdentifier(schema) + "." + quoteIdentifier(table)
}

func (MySQL) BrowseSQL(schema, table string, offset, limit int) string {
	return fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", qualified(schema, table), limit+1, offset)
}

func (MySQL) Structure(ctx context.Context, c *sql.Conn, schema, table string) (Structure, error) {
	var out Structure
	var err error
	out.Columns, err = readResult(ctx, c, "SELECT COLUMN_NAME AS `字段`, COLUMN_TYPE AS `类型`, IS_NULLABLE AS `可空`, COLUMN_DEFAULT AS `默认值`, COLUMN_KEY AS `键`, EXTRA AS `其他`, COLUMN_COMMENT AS `注释` FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION", 2000, schema, table)
	if err != nil {
		return out, err
	}
	out.Indexes, err = readResult(ctx, c, "SELECT INDEX_NAME AS `索引`, COLUMN_NAME AS `字段`, SEQ_IN_INDEX AS `顺序`, NON_UNIQUE AS `非唯一`, INDEX_TYPE AS `类型` FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY INDEX_NAME, SEQ_IN_INDEX", 2000, schema, table)
	if err != nil {
		return out, err
	}
	ddl, err := readResult(ctx, c, "SHOW CREATE TABLE "+qualified(schema, table), 1)
	if err == nil && len(ddl.Rows) > 0 && len(ddl.Rows[0]) > 1 && ddl.Rows[0][1] != nil {
		out.DDL = *ddl.Rows[0][1]
	}
	return out, err
}

// GenerateSQL returns quoted references or editable templates, never executes them.
func (MySQL) GenerateSQL(ctx context.Context, c *sql.Conn, req SQLTemplateRequest) (string, error) {
	if req.Database == "" || req.Table == "" {
		return "", fmt.Errorf("请选择数据库和表")
	}
	name := qualified(req.Database, req.Table)
	if req.Kind == "name" {
		if req.Column != "" {
			return quoteIdentifier(req.Column), nil
		}
		return quoteIdentifier(req.Table), nil
	}
	if req.Kind == "qualified-name" {
		if req.Column != "" {
			name += "." + quoteIdentifier(req.Column)
		}
		return name, nil
	}
	switch req.Kind {
	case "select", "insert", "update", "delete":
	default:
		return "", fmt.Errorf("不支持的 SQL 模板类型")
	}
	columns, err := readResult(ctx, c, "SELECT COLUMN_NAME, EXTRA, COLUMN_KEY FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION", 2001, req.Database, req.Table)
	if err != nil {
		return "", err
	}
	if len(columns.Rows) == 0 {
		return "", fmt.Errorf("表不存在或无权读取字段")
	}
	if columns.Truncated || len(columns.Rows) > 2000 {
		return "", fmt.Errorf("字段过多，无法生成完整 SQL")
	}
	var names, writable, assignments []string
	for _, row := range columns.Rows {
		if len(row) < 3 || row[0] == nil {
			continue
		}
		col := quoteIdentifier(*row[0])
		names = append(names, col)
		extra := ""
		if row[1] != nil {
			extra = strings.ToLower(*row[1])
		}
		if strings.Contains(extra, "auto_increment") || strings.Contains(extra, "virtual generated") || strings.Contains(extra, "stored generated") {
			continue
		}
		writable = append(writable, col)
		if row[2] == nil || *row[2] != "PRI" {
			assignments = append(assignments, col+" = NULL")
		}
	}
	switch req.Kind {
	case "select":
		return "SELECT\n  " + strings.Join(names, ",\n  ") + "\nFROM " + name + "\nLIMIT 100;", nil
	case "insert":
		if len(writable) == 0 {
			return "", fmt.Errorf("没有可写字段")
		}
		values := make([]string, len(writable))
		for i := range values {
			values[i] = "NULL"
		}
		return "-- 请填写字段值后执行\nINSERT INTO " + name + " (\n  " + strings.Join(writable, ",\n  ") + "\n) VALUES (\n  " + strings.Join(values, ",\n  ") + "\n);", nil
	case "update":
		if len(assignments) == 0 {
			return "", fmt.Errorf("没有可更新的非主键字段")
		}
		return "-- 请填写更新值，并将 WHERE 1 = 0 替换为目标记录条件\nUPDATE " + name + "\nSET " + strings.Join(assignments, ",\n    ") + "\nWHERE 1 = 0;", nil
	default:
		return "-- 请将 WHERE 1 = 0 替换为目标记录条件\nDELETE FROM " + name + "\nWHERE 1 = 0;", nil
	}
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readResult(ctx context.Context, c queryer, query string, limit int, args ...any) (ResultSet, error) {
	budget := 8 << 20
	return readBoundedResult(ctx, c, query, limit, &budget, args...)
}

func readBoundedResult(ctx context.Context, c queryer, query string, limit int, budget *int, args ...any) (ResultSet, error) {
	out := ResultSet{Columns: []Column{}, Rows: [][]*string{}}
	r, err := c.QueryContext(context.WithValue(ctx, jdbcRowLimitKey{}, limit), query, args...)
	if err != nil {
		return out, err
	}
	defer r.Close()
	types, err := r.ColumnTypes()
	if err != nil {
		return out, err
	}
	for _, col := range types {
		out.Columns = append(out.Columns, Column{Name: col.Name(), Type: col.DatabaseTypeName()})
	}
	for r.Next() {
		if len(out.Rows) >= limit {
			out.Truncated = true
			break
		}
		cells := make([]sql.RawBytes, len(types))
		dest := make([]any, len(types))
		for i := range dest {
			dest[i] = &cells[i]
		}
		if err = r.Scan(dest...); err != nil {
			return out, err
		}
		row := make([]*string, len(types))
		rowBytes := len(types) * 32
		if rowBytes > *budget {
			out.Truncated = true
			return out, nil
		}
		for i, cell := range cells {
			if cell == nil {
				continue
			}
			if len(cell) > 1<<20 || rowBytes+len(cell) > *budget {
				out.Truncated = true
				return out, nil
			}
			v := string(cell)
			row[i] = &v
			rowBytes += len(cell)
		}
		out.Rows = append(out.Rows, row)
		*budget -= rowBytes
	}
	if err = r.Err(); err != nil {
		return out, err
	}
	return out, nil
}
