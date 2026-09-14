package capture

type Capture interface {
	CreateSnapshot() error
	ReadSnapshot() error
}
