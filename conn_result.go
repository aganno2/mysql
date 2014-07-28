package oursql

/*
#include "oursql.h"
*/
import "C"
import (
	"unsafe"
)

const (
	// NOTE(szopa): maxSize used to be 1 << 30, but that causes
	// compiler errors in some situations.
	maxSize = 1 << 20
)

// Result is the structure returned by the mysql library.
// When transmitted over the wire, the Rows all come back as strings
// and lose their original  use Fields.Type to convert
// them back if needed, using the following functions.
type connResult struct {
	c            C.OUR_RES
	conn         *Connection
	rowsAffected uint64
	insertId     uint64
}

func (res *connResult) RowsAffected() uint64 {
	return res.rowsAffected
}

func (res *connResult) InsertId() uint64 {
	return res.insertId
}

type connQueryResult struct {
	connResult
	fields []Field
}

func (res *connQueryResult) fillFields() {
	nfields := int(res.c.num_fields)
	if nfields == 0 {
		return
	}

	cfields := (*[maxSize]C.MYSQL_FIELD)(unsafe.Pointer(res.c.fields))
	totalLength := uint64(0)
	for i := 0; i < nfields; i++ {
		totalLength += uint64(cfields[i].name_length)
	}

	fields := make([]Field, nfields)
	for i := 0; i < nfields; i++ {
		length := cfields[i].name_length
		fname := (*[maxSize]byte)(unsafe.Pointer(cfields[i].name))[:length]
		fields[i].Name = string(fname)
		fields[i].Type = TypeCode(cfields[i]._type)
	}

	res.fields = fields
}

func (res *connQueryResult) fetchNext() (row []Value, err error) {
	crow := C.our_fetch_next(&res.c)
	if crow.has_error != 0 {
		return nil, res.conn.lastError("")
	}

	rowPtr := (*[maxSize]*[maxSize]byte)(unsafe.Pointer(crow.mysql_row))
	if rowPtr == nil {
		return nil, nil
	}

	cfields := (*[maxSize]C.MYSQL_FIELD)(unsafe.Pointer(res.c.fields))

	colCount := int(res.c.num_fields)
	row = make([]Value, colCount)

	lengths := (*[maxSize]uint64)(unsafe.Pointer(crow.lengths))
	totalLength := uint64(0)
	for i := 0; i < colCount; i++ {
		totalLength += lengths[i]
	}

	arena := make([]byte, 0, int(totalLength))
	for i := 0; i < colCount; i++ {
		colLength := lengths[i]
		colPtr := rowPtr[i]
		if colPtr == nil {
			continue
		}
		start := len(arena)
		arena = append(arena, colPtr[:colLength]...)
		row[i] = Value{TypeCode(cfields[i]._type), arena[start : start+int(colLength)]}
	}

	return row, nil
}

func (res *connQueryResult) close() {
	C.our_close_result(&res.c)
}

func (res *connQueryResult) Fields() []Field {
	return res.fields
}

func (res *connQueryResult) IndexOf(name string) int {
	for i, field := range res.fields {
		if field.Name == name {
			return i
		}
	}
	return -1
}

type connDataTable struct {
	connQueryResult
	rows [][]Value
}

func (res *connDataTable) fillRows() (err error) {
	rowCount := int(res.c.affected_rows)
	if rowCount == 0 {
		return nil
	}

	if rowCount < 0 {
		return res.conn.lastError("")
	}

	rows := make([][]Value, rowCount)
	for i := 0; i < rowCount; i++ {
		rows[i], err = res.fetchNext()
		if err != nil {
			return err
		}
	}

	res.rows = rows

	return nil
}

func (res *connDataTable) Rows() [][]Value {
	return res.rows
}

type connDataReader struct {
	connQueryResult
}

func (res *connDataReader) FetchNext() ([]Value, error) {
	return res.fetchNext()
}

func (res *connDataReader) Close() {
	res.close()
}