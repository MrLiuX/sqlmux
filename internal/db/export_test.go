package db

// For db_test, which takes the engines: they import db. The statements of one
// change, as Save sends them.
func UpdateSQL(e Engine, schema, table string, keyCols []string, r Row) (string, []Val) {
	s := Statements(e, schema, table, keyCols, Changes{Updates: []Row{r}})[0]
	return s.SQL, s.Args
}

func InsertSQL(e Engine, schema, table string, cols []Change) (string, []Val) {
	s := Statements(e, schema, table, nil, Changes{Inserts: [][]Change{cols}})[0]
	return s.SQL, s.Args
}
