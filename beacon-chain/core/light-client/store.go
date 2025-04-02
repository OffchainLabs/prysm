package light_client

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
)

type Store struct {
	LastLCFinalityUpdate   interfaces.LightClientFinalityUpdate
	LastLCOptimisticUpdate interfaces.LightClientOptimisticUpdate
}
