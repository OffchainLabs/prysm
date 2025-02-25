package blockchain

import (
	"encoding/hex"

	"github.com/pkg/errors"
)

var errHoleskyForbiddenRoot = errors.New("refusing to process forbidden holesky block")

// hack to prevent bad holesky block importation
var badHoleskyRoot [32]byte

func init() {
	hexStr := "2db899881ed8546476d0b92c6aa9110bea9a4cd0dbeb5519eb0ea69575f1f359"
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	badHoleskyRoot = [32]byte(bytes)
}
