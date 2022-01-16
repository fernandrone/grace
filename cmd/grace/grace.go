package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fernandrone/grace/pkg/container"
	"github.com/fernandrone/grace/pkg/table"
	"github.com/urfave/cli/v2"

	"k8s.io/client-go/util/homedir"
)

// Input is the main input structure to the program
type InputOptions struct {
	Containers []string
	Kubeconfig string
	Namespace  string
	Output     io.Writer
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
	kubeconfigFlag := &cli.StringFlag{Name: "kubeconfig"}
	namespaceFlag := &cli.StringFlag{
		Name:    "namespace",
		Aliases: []string{"n"},
		Usage:   "Kubernetes namespace",
	}

	if home := homedir.HomeDir(); home != "" {
		kubeconfigFlag.Value = filepath.Join(home, ".kube", "config")
		kubeconfigFlag.Usage = "(optional) absolute path to the kubeconfig file"
	} else {
		kubeconfigFlag.Value = ""
		kubeconfigFlag.Usage = "absolute path to the kubeconfig file"
	}

	app := &cli.App{
		Name:        "Grace",
		Usage:       "validates if containerized applications terminate gracefully.",
		UsageText:   "grace [CONTAINER [CONTAINER ...]]",
		Description: "Validates if containerized applications terminate gracefully.",
		Flags: []cli.Flag{
			kubeconfigFlag,
			namespaceFlag,
		},

		Action: func(c *cli.Context) error {
			if c.NArg() == 0 {
				cli.ShowAppHelpAndExit(c, 0)
			}

			opt := InputOptions{
				Containers: c.Args().Slice(),
				Kubeconfig: c.String("kubeconfig"),
				Namespace:  c.String("namespace"),
				Output:     os.Stdout,
			}

			if err := run(opt); err != nil {
				return err
			}

			return nil
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(opt InputOptions) error {
	var data []table.Data
	var containers []container.Containers

	for _, containerID := range opt.Containers {

		var err error
		var c container.Containers
		slice := strings.Split(containerID, "/")

		// if no platform we default to docker
		if len(slice) == 1 {
			c, err = container.NewDockerContainer(slice[0])
		} else if slice[0] == "docker" {
			c, err = container.NewDockerContainer(slice[1])
		} else if slice[0] == "pod" {
			c, err = container.NewKubernetesPod(slice[1], opt.Namespace, opt.Kubeconfig)
		} else {
			return fmt.Errorf("invalid platform %s", slice[0])
		}

		if err != nil {
			return err
		}

		containers = append(containers, c)

	}

	for _, c := range containers {
		out, err := analyze(context.Background(), c)

		if err != nil {
			return err
		}

		data = append(data, out...)
	}

	table.Write(opt.Output, data)
	return nil
}

func analyze(ctx context.Context, container container.Containers) ([]table.Data, error) {
	response, err := container.Stop(ctx)

	if err != nil {
		return []table.Data{}, err
	}

	var data []table.Data

	for _, r := range response {
		data = append(data, table.Data{
			ShortID:          r.Config.ID,
			ExitCode:         r.ExitCode,
			Image:            r.Config.Image,
			Command:          r.Config.Command,
			Timeout:          r.Config.StopTimeout,
			TerminationState: r.ToTerminationState(r.Config.StopTimeout).String(),
			StopDuration:     int(r.Duration / time.Second),
		})
	}

	return data, err
}
