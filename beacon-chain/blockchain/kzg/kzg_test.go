package kzg

import "testing"

func TestCellFlattenedChunked(t *testing.T) {
	cell := makeCell()
	chunkedCell := cellToCKZGCell(&cell)
	flattenedCell := ckzgCellToCell(&chunkedCell)
	if cell != flattenedCell {
		t.Errorf("cell != flattenedCell")
	}
}

func makeCell() Cell {
	var cell Cell
	for i := 0; i < fieldElementsPerCell; i++ {
		rand32 := deterministicRandomness(int64(i))
		copy(cell[i*32:], rand32[:])
	}
	return cell
}
