package plan

type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

type Item struct {
	Id          float64 `json:"id"`
	Description string  `json:"description"`
	Status      Status  `json:"status"`
}
