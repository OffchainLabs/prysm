package structs

type GetDepositContractResponse struct {
	Data *DepositContractData `json:"data"`
}

type DepositContractData struct {
	ChainId string `json:"chain_id"`
	Address string `json:"address"`
}

type GetForkScheduleResponse struct {
	Data []*Fork `json:"data"`
}

type GetSpecResponse struct {
	Data interface{} `json:"data"`
}
