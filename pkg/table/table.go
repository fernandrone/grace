package table

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/olekukonko/tablewriter"
)

// Data is the main output structure of the program
type Data struct {
	ShortID string
	Image   string
	Command string

	Timeout          time.Duration
	TerminationState string

	ExitCode     int
	StopDuration int
}

func Write(writer io.Writer, data []Data) {
	table := tablewriter.NewWriter(writer)

	var rows [][]string

	for _, out := range data {
		rows = append(rows, []string{
			out.ShortID,
			out.Image,
			fmt.Sprintf("%5s", out.Command),
			fmt.Sprint(out.TerminationState),
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
