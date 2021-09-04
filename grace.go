package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/olekukonko/tablewriter"
)

const defaultStopTimeout = time.Second * time.Duration(10)

type Termination int

const (
	// GracefulSuccess is ideally what you would want to see everywhere. It means that
	// the container terminated gracefully and the exit code was zero.
	GracefulSuccess Termination = iota

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

func (d Termination) String() string {
	return [...]string{
		"GracefulSuccess", "GracefulError", "ForceKilled", "OOMKilled", "Unhandled",
	}[d]
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
	cli.AppHelpTemplate = `{{.Description | nindent 3 | trim}}

Usage:
  {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}} {{if .VisibleFlags}}[global options]{{end}}{{if .Commands}} command [command options]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[arguments...]{{end}}{{end}}{{if .Version}}{{if not .HideVersion}}

Version:
  {{.Version}}{{end}}{{end}}{{if .Description}}

Commands:{{range .VisibleCategories}}{{if .Name}}
  {{.Name}}:{{range .VisibleCommands}}
    {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{else}}{{range .VisibleCommands}}
  {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{end}}{{end}}{{end}}{{if .VisibleFlags}}

Global Options:
  {{range $index, $option := .VisibleFlags}}{{if $index}}
  {{end}}{{$option}}{{end}}{{end}}{{if .Copyright}}

Copyright:
  {{.Copyright}}{{end}}
`

	app := &cli.App{
		Name:        "Grace",
		Usage:       "validates if containerized applications terminate gracefully.",
		UsageText:   "grace [CONTAINER [CONTAINER ...]]",
		Description: "Validates if containerized applications terminate gracefully.",
		Action: func(c *cli.Context) error {
			if c.NArg() == 0 {
				cli.ShowAppHelpAndExit(c, 0)
			}

			docker, err := client.NewClientWithOpts(client.FromEnv)

			if err != nil {
				return err
			}

			in := Input{}
			in.Containers = c.Args().Slice()
			in.Docker = docker

			if err := run(in, os.Stdout); err != nil {
				return err
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
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
