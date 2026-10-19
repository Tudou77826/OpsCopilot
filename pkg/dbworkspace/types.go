// Package dbworkspace owns database profiles and bounded query sessions, independently of UI hosts.
package dbworkspace

import (
	"context"
	"database/sql"
	"net"
)

type Profile struct {
	ID                          string `json:"id"`
	Revision                    int    `json:"revision"`
	Name                        string `json:"name"`
	Environment                 string `json:"environment"`
	Group                       string `json:"group,omitempty"`
	Engine                      string `json:"engine"`
	DriverID                    string `json:"driverId,omitempty"`
	JDBCAllowPublicKeyRetrieval bool   `json:"jdbcAllowPublicKeyRetrieval,omitempty"`
	Host                        string `json:"host"`
	Port                        int    `json:"port"`
	User                        string `json:"user"`
	Database                    string `json:"database"`
	TLS                         string `json:"tls"`
	ReadOnly                    bool   `json:"readOnly"`
	GatewayID                   string `json:"gatewayId"`
	GatewayMode                 string `json:"gatewayMode,omitempty"`
	PasswordSet                 bool   `json:"passwordSet"`
}

type ProfileInput struct {
	Profile
	Password *string `json:"password"` // nil preserves a saved credential; a supplied empty string clears it.
}

type SessionInfo struct {
	ID        string `json:"id"`
	ProfileID string `json:"profileId"`
	Version   string `json:"version"`
	Database  string `json:"database"`
	ReadOnly  bool   `json:"readOnly"`
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ResultSet struct {
	Statement    string      `json:"statement"`
	Columns      []Column    `json:"columns"`
	Rows         [][]*string `json:"rows"` // preserve NULL, DECIMAL and BIGINT without JS numeric coercion.
	AffectedRows int64       `json:"affectedRows"`
	Truncated    bool        `json:"truncated"`
}

type QueryRequest struct {
	Database       string `json:"database"`
	SessionID      string `json:"sessionId"`
	RequestID      string `json:"requestId"`
	SQL            string `json:"sql"`
	Mode           string `json:"mode,omitempty"`         // statement, selection, script; empty preserves legacy script calls.
	CursorOffset   int    `json:"cursorOffset,omitempty"` // UTF-16 code units, matching the browser editor.
	Limit          int    `json:"limit"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

type QueryResult struct {
	Sets         []ResultSet `json:"sets"`
	DurationMS   int64       `json:"durationMs"`
	Error        string      `json:"error"`
	Cancelled    bool        `json:"cancelled"`
	Disconnected bool        `json:"disconnected"`
}

type Table struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment"`
}

type TableRequest struct {
	RequestID string `json:"requestId"`
	SessionID string `json:"sessionId"`
	Database  string `json:"database"`
	Table     string `json:"table"`
	Offset    int    `json:"offset"`
	Limit     int    `json:"limit"`
}

type SQLTemplateRequest struct {
	SessionID string `json:"sessionId"`
	Database  string `json:"database"`
	Table     string `json:"table"`
	Kind      string `json:"kind"`
	Column    string `json:"column,omitempty"`
}

// SQLGenerator only builds text; executing it remains a separate user action.
type SQLGenerator interface {
	GenerateSQL(context.Context, *sql.Conn, SQLTemplateRequest) (string, error)
}

type Structure struct {
	Columns ResultSet `json:"columns"`
	Indexes ResultSet `json:"indexes"`
	DDL     string    `json:"ddl"`
}

type DialContext func(context.Context, string) (net.Conn, error)

const (
	GatewayModeAuto    = "auto"
	GatewayModeForward = "forward"
	GatewayModeCommand = "command"
)

type GatewayOpener func(context.Context, string, string) (DialContext, func() error, error)

// Connector is the extension boundary for database protocols and metadata dialects.
type Connector interface {
	Open(context.Context, Profile, string, DialContext) (*sql.DB, func(), error)
	Schemas(context.Context, *sql.Conn) ([]string, error)
	Tables(context.Context, *sql.Conn, string) ([]Table, error)
	Structure(context.Context, *sql.Conn, string, string) (Structure, error)
	BrowseSQL(string, string, int, int) string
}

// QueryCanceller can ask the server to interrupt the owned physical session.
type QueryCanceller interface {
	ConnectionID(context.Context, *sql.Conn) (string, error)
	Cancel(context.Context, *sql.DB, string) error
}
