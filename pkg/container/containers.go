package container

import (
	"context"
	"time"
)

type Containers interface {
	Stop(ctx context.Context) ([]Response, error)
}

type Config struct {
	ID          string
	Image       string
	Command     string
	StopTimeout time.Duration
}

type Response struct {
	Config    Config
	ExitCode  int
	Duration  time.Duration
	OOMKilled bool
}

func (s Response) ToTerminationState(stopTimeout time.Duration) TerminationState {
	if s.ExitCode == 0 {
		return GracefulSuccess
	}

	if s.OOMKilled {
		return OOMKilled
	}

	isSIGKILL := s.ExitCode == 137 || s.ExitCode == 9

	if isSIGKILL {

		if s.Duration >= stopTimeout {
			return ForceKilled
		}

		return Unhandled
	}

	return GracefulError
}
