package input

import (
	"database/sql"
	"strings"
	"strconv"
	// "fmt"  // for debug print

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

type Input struct {
	Provider  string
	Address   string
	Tablename string
	Startid   int
	Endid     int
	Maxid     int
	Result    []string
}

func NewExtracter(provider string, address string, tablename string, startid int, endid int) Input {
	i := Input{}
	i.Provider = provider
	i.Address = address
	i.Tablename = tablename
	i.Startid = startid
	i.Endid = endid
	return i
}

func (input *Input) getSQL() string {

	var query string

	switch input.Provider {
	case "postgres":
		query = pgSQL(input.Tablename)
	case "mysql":
		query = mySQL(input.Tablename)
	default:
		panic("unrecognized provider")
	}

	return strings.Replace(
		strings.Replace(
			query,
			"##STARTID##", strconv.Itoa(input.Startid), -1),
		    "##ENDID##", strconv.Itoa(input.Endid), -1)
}

func (input *Input) Extract() error {

	// get query
	query := input.getSQL()

	//fmt.Println(fmt.Sprintf("------------------- %s: %s", input.Tablename, query))

	// open a connection
	conn, err := sql.Open(input.Provider, input.Address)
	if err != nil {
		return err
	}
	defer conn.Close()

	rows, err := conn.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	// fetch result
	resultInline := []string{}
	var clock string
	var syncid int

	for rows.Next() {
		var result string
		if err := rows.Scan(&result, &clock, &syncid); err != nil {
			return err
		}
		resultInline = append(resultInline, result)

		// save max id from the result set if bigger then current
	  if input.Maxid < syncid {
			input.Maxid = syncid
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	input.Result = resultInline

	// fmt.Println(resultInline)

	return nil
}
