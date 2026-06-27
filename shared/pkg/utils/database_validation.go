package utils

import (
	"fmt"
	"regexp"
	"strings"
)

var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
var userRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

var reservedDatabaseWords = map[string]struct{}{
	"all": {}, "alter": {}, "and": {}, "any": {}, "as": {}, "asc": {}, "authorization": {},
	"backup": {}, "begin": {}, "between": {}, "break": {}, "browse": {}, "bulk": {}, "by": {},
	"cascade": {}, "case": {}, "check": {}, "checkpoint": {}, "close": {}, "clustered": {},
	"coalesce": {}, "collate": {}, "column": {}, "commit": {}, "compute": {}, "constraint": {},
	"contains": {}, "containstable": {}, "continue": {}, "convert": {}, "create": {}, "cross": {},
	"current": {}, "current_date": {}, "current_time": {}, "current_timestamp": {}, "current_user": {},
	"cursor": {}, "database": {}, "dbcc": {}, "deallocate": {}, "declare": {}, "default": {},
	"delete": {}, "deny": {}, "desc": {}, "disk": {}, "distinct": {}, "distributed": {},
	"double": {}, "drop": {}, "dump": {}, "else": {}, "end": {}, "errlvl": {}, "escape": {},
	"except": {}, "exec": {}, "execute": {}, "exists": {}, "exit": {}, "external": {},
	"fetch": {}, "file": {}, "fillfactor": {}, "for": {}, "foreign": {}, "freetext": {},
	"freetexttable": {}, "from": {}, "full": {}, "function": {}, "goto": {}, "grant": {},
	"group": {}, "having": {}, "holdlock": {}, "identity": {}, "identity_insert": {}, "identitycol": {},
	"if": {}, "in": {}, "index": {}, "inner": {}, "insert": {}, "intersect": {}, "into": {},
	"is": {}, "join": {}, "key": {}, "kill": {}, "left": {}, "like": {}, "lineno": {},
	"load": {}, "merge": {}, "national": {}, "nocheck": {}, "nonclustered": {}, "not": {},
	"null": {}, "nullif": {}, "of": {}, "off": {}, "offsets": {}, "on": {}, "once": {},
	"only": {}, "open": {}, "opendatasource": {}, "openquery": {}, "openrowset": {}, "openxml": {},
	"option": {}, "or": {}, "order": {}, "outer": {}, "over": {}, "percent": {}, "pivot": {},
	"plan": {}, "precision": {}, "primary": {}, "print": {}, "proc": {}, "procedure": {},
	"public": {}, "raiserror": {}, "read": {}, "readtext": {}, "reconfigure": {}, "references": {},
	"replication": {}, "restore": {}, "restrict": {}, "return": {}, "revert": {}, "revoke": {},
	"right": {}, "rollback": {}, "rowcount": {}, "rowguidcol": {}, "rule": {}, "save": {},
	"schema": {}, "securityaudit": {}, "select": {}, "semantickeyphrasetable": {},
	"semanticsimilaritydetailstable": {}, "semanticsimilaritytable": {}, "session_user": {}, "set": {},
	"setuser": {}, "shutdown": {}, "some": {}, "statistics": {}, "system_user": {}, "table": {},
	"tablesample": {}, "textsize": {}, "then": {}, "to": {}, "top": {}, "tran": {}, "transaction": {},
	"trigger": {}, "truncate": {}, "try_convert": {}, "tsequal": {}, "union": {}, "unique": {},
	"unpivot": {}, "update": {}, "updatetext": {}, "use": {}, "user": {}, "values": {},
	"varying": {}, "view": {}, "waitfor": {}, "when": {}, "where": {}, "while": {}, "with": {},
	"within": {}, "writetext": {}, "postgres": {}, "root": {}, "admin": {}, "system": {}, "mysql": {},
}

const DatabasePasswordInvalidCharsMessage = "Database password must not contain spaces or connection-string-breaking characters like \", ', `, \\, ;, @, #, /, or ?"

func IsReservedDatabaseWord(value string) bool {
	_, ok := reservedDatabaseWords[value]
	return ok
}

// ValidateDatabaseEngine checks if engine is mysql or postgresql
func ValidateDatabaseEngine(engine string) (string, error) {
	engine = strings.ToLower(engine)
	if engine == "postgres" {
		engine = "postgresql"
	}
	if engine != "mysql" && engine != "postgresql" {
		return "", fmt.Errorf("Invalid database engine. Supported: mysql, postgresql")
	}
	return engine, nil
}

// ValidateDatabaseName validates DB name rules
func ValidateDatabaseName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed != strings.ToLower(trimmed) {
		return fmt.Errorf("Database name must be strictly lowercase")
	}
	if !nameRegex.MatchString(trimmed) {
		return fmt.Errorf("Database name must be 2-64 characters, start with a letter, and contain only alphanumeric characters or underscores")
	}
	if IsReservedDatabaseWord(trimmed) {
		return fmt.Errorf("Database name '%s' is a reserved SQL word", trimmed)
	}
	return nil
}

// ValidateDatabaseUsername validates DB username rules
func ValidateDatabaseUsername(username string) error {
	trimmed := strings.TrimSpace(username)
	if trimmed != strings.ToLower(trimmed) {
		return fmt.Errorf("Database username must be strictly lowercase")
	}
	if !userRegex.MatchString(trimmed) {
		return fmt.Errorf("Database username must be 2-32 characters, start with a letter, and contain only alphanumeric characters or underscores")
	}
	if IsReservedDatabaseWord(trimmed) {
		return fmt.Errorf("Database username '%s' is a reserved SQL word", trimmed)
	}
	return nil
}

// ValidateDatabasePassword validates password strength
func ValidateDatabasePassword(password string) error {
	if len(password) < 12 || len(password) > 128 {
		return fmt.Errorf("Database password must be 12-128 characters long")
	}
	if strings.ContainsAny(password, " \"'`\\;@#/?") {
		return fmt.Errorf(DatabasePasswordInvalidCharsMessage)
	}
	hasUpper := false
	hasLower := false
	hasDigit := false
	for _, char := range password {
		if char >= 'A' && char <= 'Z' {
			hasUpper = true
		} else if char >= 'a' && char <= 'z' {
			hasLower = true
		} else if char >= '0' && char <= '9' {
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return fmt.Errorf("Database password must contain at least one uppercase letter, one lowercase letter, and one number")
	}
	return nil
}
