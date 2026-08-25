package api

type PayloadStatus string

const (
	PayloadStatusUnknown PayloadStatus = ""
	PayloadStatusEmpty   PayloadStatus = "empty"
	PayloadStatusFull    PayloadStatus = "full"
)

func (p PayloadStatus) String() string {
	return string(p)
}
