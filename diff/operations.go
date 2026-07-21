package diff

type Operation string

const (
	ADD  Operation = "ADD"
	DEL  Operation = "DEL"
	KEEP Operation = "KEEP"
)
