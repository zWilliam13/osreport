package excel

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

func writeHeaderRow(f *excelize.File, sheet string, headers []string, styleID int) error {
	for i, h := range headers {
		cell, err := cellRef(i+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
	}
	firstCell, _ := cellRef(1, 1)
	lastCell, err := cellRef(len(headers), 1)
	if err != nil {
		return err
	}
	return f.SetCellStyle(sheet, firstCell, lastCell, styleID)
}

func setRow(f *excelize.File, sheet string, row int, values []interface{}) error {
	for i, v := range values {
		cell, err := cellRef(i+1, row)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheet, cell, v); err != nil {
			return err
		}
	}
	return nil
}

func cellRef(col, row int) (string, error) {
	colName, err := excelize.ColumnNumberToName(col)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", colName, row), nil
}
