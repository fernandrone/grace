package container

type TerminationState int

func (d TerminationState) String() string {
	return [...]string{
		"GracefulSuccess", "GracefulError", "ForceKilled", "OOMKilled", "Unhandled",
	}[d]
}

const (
	// GracefulSuccess is ideally what you would want to see everywhere. It means that
	// the container terminated gracefully and the exit code was zero.
	GracefulSuccess TerminationState = iota

	// This means that the container terminated gracefully but the exit code was not
	// zero.
	GracefulError

	// The container did not terminate gracefully. Specifically, it failed to terminate
	// within the allocated StopTimeout, triggering a SIGKILL by the container daemon.
	ForceKilled

	// The container did not terminate gracefully. During the shutdown it requested more
	// memory than the limit allowed, triggering a SIGKILL by the container daemon.
	OOMKilled

	// The container did not terminate gracefully. It terminated with status code 9 or
	// 137 (which are reserved for SIGKILL) but no OOMKILL nor timeout was detected.
	//
	// This should probably only happen if the main process within the container is
	// configured to exit with one of those two status codes and this either happened by
	// chance or as a response to the SIGTERM signal.
	Unhandled
)
