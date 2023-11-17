package supervisor

// sup flag
type Strategy string

const (
	OneForOne  Strategy = "one_for_one"
	OneForAll  Strategy = "one_for_all"
	RestForOne Strategy = "rest_for_one"
)

// child_spec
type Restart string

const (
	Permanent Restart = "permanent"
	Temporary Restart = "temporary"
	Transient Restart = "transient"
)

// child_spec
type ShutdownOpt struct {
	BrutalKill bool
	Timeout    int
	Infinity   bool
}

type ChildType string

const (
	SupervisorChild ChildType = "supervisor"
	WorkerChild     ChildType = "worker"
)
