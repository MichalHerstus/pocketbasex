package main

import (
	"regexp"
	"strings"
)

// viewColumn represents a single selected column in a view query.
type viewColumn struct {
	Alias   string // column alias (AS alias) or computed name
	Table   string // source table name or alias
	Field   string // column name (* for all)
	RawExpr string // raw expression text (for non-trivial selects)
}

// viewTable represents a table in the FROM clause.
type viewTable struct {
	Name  string // table name
	Alias string // table alias
}

// viewJoin represents a JOIN clause.
type viewJoin struct {
	Type    string     // JOIN type: "JOIN", "LEFT JOIN", etc.
	Table   viewTable  // joined table
	On      viewOnCond // ON condition
}

// viewOnCond represents the ON condition of a JOIN.
type viewOnCond struct {
	LeftTable  string // left side table alias/name
	LeftField  string // left side field
	RightTable string // right side table alias/name
	RightField string // right side field
}

// viewQueryInfo is the result of parsing a view query.
type viewQueryInfo struct {
	Columns []viewColumn
	From    viewTable
	Joins   []viewJoin
}

var (
	reAlias    = regexp.MustCompile(`(?i)\s+AS\s+(\w+)$`)
	reJoinType = regexp.MustCompile(`(?i)^\s*(LEFT\s+|RIGHT\s+|INNER\s+|CROSS\s+|FULL\s+)?JOIN\s+`)
)

// parseViewQuery extracts tables, joins, and selected columns from a SQL SELECT.
func parseViewQuery(query string) viewQueryInfo {
	q := viewQueryInfo{}
	s := strings.TrimSpace(query)

	// find SELECT ... FROM boundary
	fromIdx := indexTopLevelKeywordAny(s, "FROM")
	if fromIdx < 0 {
		return q
	}
	selectPart := strings.TrimSpace(s[len("SELECT"):fromIdx])
	rest := strings.TrimSpace(s[fromIdx+len("FROM"):])

	// parse FROM table
	table, rest2 := parseTableRef(rest)
	q.From = table
	rest = strings.TrimSpace(rest2)

	// parse JOINs
	for {
		jt := reJoinType.FindString(rest)
		if jt == "" {
			break
		}
		joinType := strings.TrimSpace(strings.ToUpper(jt))
		rest = rest[len(jt):]
		jTable, rest2 := parseTableRef(rest)
		rest = strings.TrimSpace(rest2)
		onCond := viewOnCond{}
		if idx := indexTopLevelKeywordAny(rest, "ON"); idx >= 0 {
			onPart := strings.TrimSpace(rest[:idx])
			rest = strings.TrimSpace(rest[idx+len("ON"):])
			onCond = parseOnCondition(onPart)
		}
		q.Joins = append(q.Joins, viewJoin{
			Type:  joinType,
			Table: jTable,
			On:    onCond,
		})
	}

	// parse selected columns
	colStrs := splitTopLevel(selectPart, ',')
	for _, cs := range colStrs {
		cs = strings.TrimSpace(cs)
		if cs == "" {
			continue
		}
		col := viewColumn{RawExpr: cs}
		// check for alias: "expr AS alias" → keep expr part
		if m := reAlias.FindStringSubmatch(cs); m != nil {
			col.Alias = m[1]
			cs = strings.TrimSpace(cs[:len(cs)-len(m[0])])
		}
		// parse table.field or just field
		parts := strings.SplitN(cs, ".", 2)
		if len(parts) == 2 {
			col.Table = strings.Trim(parts[0], "\"'`[]")
			col.Field = strings.Trim(parts[1], "\"'`[]")
		} else {
			col.Field = strings.Trim(cs, "\"'`[]")
		}
		if col.Alias == "" {
			col.Alias = col.Field
		}
		q.Columns = append(q.Columns, col)
	}

	return q
}

// indexTopLevelKeywordAny returns the index of the first top-level occurrence of
// any of the given keywords (case-insensitive). Returns -1 if none found.
func indexTopLevelKeywordAny(s string, keywords ...string) int {
	sU := strings.ToUpper(s)
	inQuote := false
	quoteCh := byte(0)
	parenDepth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			if ch == quoteCh && (ch != '\'' || i+1 >= len(s) || s[i+1] != '\'') {
				inQuote = false
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inQuote = true
			quoteCh = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		default:
			if parenDepth > 0 {
				continue
			}
			for _, kw := range keywords {
				kwU := strings.ToUpper(kw)
				if i+len(kwU) <= len(sU) && sU[i:i+len(kwU)] == kwU {
					// check word boundaries
					if i > 0 && isWordByte(sU[i-1]) {
						continue
					}
					end := i + len(kwU)
					if end < len(sU) && isWordByte(sU[end]) {
						continue
					}
					return i
				}
			}
		}
	}
	return -1
}

func isWordByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

// splitTopLevel splits s by the given delimiter, respecting parentheses and quoted strings.
func splitTopLevel(s string, delim byte) []string {
	var parts []string
	inQuote := false
	quoteCh := byte(0)
	parenDepth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			if ch == quoteCh && (ch != '\'' || i+1 >= len(s) || s[i+1] != '\'') {
				inQuote = false
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inQuote = true
			quoteCh = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		default:
			if parenDepth == 0 && ch == delim {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// parseTableRef parses "tableName [AS alias]" and returns the table and remaining string.
func parseTableRef(s string) (viewTable, string) {
	s = strings.TrimSpace(s)
	tbl := viewTable{}
	inQuote := false
	quoteCh := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			if ch == quoteCh {
				inQuote = false
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inQuote = true
			quoteCh = ch
		default:
			if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == ',' || ch == '(' || ch == ';' {
				tbl.Name = strings.Trim(s[:i], "\"'`[]")
				rest := strings.TrimSpace(s[i:])
				// check for AS alias
				restU := strings.ToUpper(rest)
				if strings.HasPrefix(restU, "AS ") {
					aliasEnd := strings.IndexAny(rest[3:], " \t\n\r,();")
					if aliasEnd < 0 {
						aliasEnd = len(rest) - 3
					}
					tbl.Alias = strings.TrimSpace(rest[3 : 3+aliasEnd])
					rest = strings.TrimSpace(rest[3+aliasEnd:])
				} else if strings.HasPrefix(restU, "JOIN ") || strings.HasPrefix(restU, "LEFT ") || strings.HasPrefix(restU, "RIGHT ") || strings.HasPrefix(restU, "INNER ") || strings.HasPrefix(restU, "CROSS ") || strings.HasPrefix(restU, "FULL ") || strings.HasPrefix(restU, "ON ") || strings.HasPrefix(restU, "WHERE ") || strings.HasPrefix(restU, "GROUP ") || strings.HasPrefix(restU, "ORDER ") || strings.HasPrefix(restU, "LIMIT ") || strings.HasPrefix(restU, "HAVING ") || strings.HasPrefix(restU, "UNION ") || strings.HasPrefix(restU, "INTERSECT ") || strings.HasPrefix(restU, "EXCEPT ") {
					// no alias, rest is next clause
				} else if len(rest) > 0 {
					// might be an alias without AS
					aliasEnd := strings.IndexAny(rest, " \t\n\r,();")
					if aliasEnd < 0 {
						tbl.Alias = rest
						rest = ""
					} else {
						tbl.Alias = rest[:aliasEnd]
						rest = strings.TrimSpace(rest[aliasEnd:])
					}
				}
				return tbl, rest
			}
		}
	}
	tbl.Name = strings.Trim(s, "\"'`[]")
	return tbl, ""
}

// parseOnCondition parses "leftTable.leftField = rightTable.rightField".
func parseOnCondition(s string) viewOnCond {
	s = strings.TrimSpace(s)
	cond := viewOnCond{}
	// find the = sign at top level
	inQuote := false
	quoteCh := byte(0)
	parenDepth := 0
	eqIdx := -1
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			if ch == quoteCh {
				inQuote = false
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			inQuote = true
			quoteCh = ch
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		default:
			if parenDepth == 0 && ch == '=' {
				eqIdx = i
				break
			}
		}
	}
	if eqIdx < 0 {
		return cond
	}
	left := strings.TrimSpace(s[:eqIdx])
	right := strings.TrimSpace(s[eqIdx+1:])
	cond.LeftTable, cond.LeftField = parseColumnRef(left)
	cond.RightTable, cond.RightField = parseColumnRef(right)
	return cond
}

// parseColumnRef splits "table.field" or just "field".
func parseColumnRef(s string) (table, field string) {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'`[]")
	parts := strings.SplitN(s, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", s
}
