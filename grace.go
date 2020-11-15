package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/olekukonko/tablewriter"
)

const defaultStopTimeout = time.Second * time.Duration(10)

const (
	helpMsg = `usage of %s [CONTAINER [CONTAINER ...]]

Validates simple newline and whitespace rules in all sorts of files.

positional arguments:
  CONTAINER		id or name of docker container
`
)

type Termination int

const (
	GracefulSuccess Termination = iota
	GracefulError
	ForceKilled
	OOMKilled
	Unhandled
)

func (d Termination) String() string {
	return [...]string{"GracefulSuccess", "GracefulError", "ForceKilled", "OOMKilled", "Unhandled"}[d]
}

// Input is the main input structure to the program
type Input struct {
	Containers []string
	Docker     *client.Client
}

// Output is the main output structure to the program
type Output struct {
	ShortID string
	Image   string
	Command string

	Timeout     time.Duration
	Termination Termination

	ExitCode     int
	StopDuration int
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, helpMsg, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(0)
	}

	cli, err := client.NewEnvClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	in := Input{}
	in.Containers = flag.Args()
	in.Docker = cli

	if err := run(in, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(in Input, writer io.Writer) error {
	var data []Output

	for _, c := range in.Containers {

		out, err := analyze(context.Background(), in.Docker, c)

		if err != nil {
			return err
		}

		data = append(data, out)
	}

	write(writer, data)

	return nil
}

func analyze(ctx context.Context, docker *client.Client, c string) (Output, error) {
	json, err := docker.ContainerInspect(ctx, c)

	if err != nil {
		return Output{}, err
	}

	shortID := json.ID[:12]

	// see if is runnning
	if !json.State.Running {
		return Output{}, fmt.Errorf("container %s is not running", shortID)
	}

	// try to gracefully stop the container
	stopDuration, err := stopContainer(ctx, docker, c)

	if err != nil {
		return Output{}, err
	}

	// inspect again to get new State
	json, err = docker.ContainerInspect(ctx, c)

	if err != nil {
		return Output{}, err
	}

	terms := json.Config.Entrypoint
	terms = append(terms, json.Config.Cmd...)
	command := strings.Join(terms, " ")

	if len(command) > 30 {
		command = command[0:27] + "..."
	}

	stopTimeout := defaultStopTimeout

	if json.Config.StopTimeout != nil {
		stopTimeout = time.Second * time.Duration(*json.Config.StopTimeout)
	}

	out := Output{
		ShortID:      shortID,
		ExitCode:     json.State.ExitCode,
		Image:        json.Config.Image,
		Command:      command,
		Timeout:      stopTimeout,
		Termination:  getTerminationState(*json.State, stopDuration, stopTimeout),
		StopDuration: int(stopDuration / time.Second),
	}

	return out, err
}

func getTerminationState(state types.ContainerState, stopDuration, stopTimeout time.Duration) Termination {
	if state.ExitCode == 0 {
		return GracefulSuccess
	}

	if state.OOMKilled {
		return OOMKilled
	}

	isSIGKILL := state.ExitCode == 137 || state.ExitCode == 9

	if isSIGKILL {

		if stopDuration >= stopTimeout {
			return ForceKilled
		}

		return Unhandled
	}

	return GracefulError
}

func stopContainer(ctx context.Context, docker *client.Client, c string) (time.Duration, error) {
	start := time.Now()
	err := docker.ContainerStop(ctx, c, nil)
	return time.Since(start), err
}

func write(writer io.Writer, data []Output) {
	table := tablewriter.NewWriter(writer)

	var rows [][]string

	for _, out := range data {
		rows = append(rows, []string{
			out.ShortID,
			out.Image,
			fmt.Sprintf("%5s", out.Command),
			fmt.Sprint(out.Termination.String()),
			strconv.FormatInt(int64(out.ExitCode), 10),
			fmt.Sprintf("%3ss/%ss", strconv.FormatInt(int64(out.StopDuration), 10), strconv.FormatInt(int64(out.Timeout/time.Second), 10)),
		})
	}

	table.SetHeader([]string{
		"ID", "IMAGE", "COMMAND", "TERMINATION", "EXIT CODE", "DURATION",
	})

	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t")
	table.SetNoWhiteSpace(true)

	table.AppendBulk(rows)
	table.Render()
}
