// csvplate is a small utilisty that takes a csvfile and a golang template
// and generates one or multimple output files based on the template.
// If the output file name contains a template expression, it will be evaluated
// for every row and new files will be created, else a single file will be created.
// If single file is created the . is a slice of objects, else the . is a single object.
// The first line is assumed to be the header line and will be used as the field names, except
// if the -noheader flag is set in which case the fields will be named C1, C2, ...
// The template functions from sprig are available in the templates.
// Usage:
//
//	csvplate [-noheader] -csv input.csv -template template.txt -out output.txt
package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/sprig/v3"
	"github.com/spf13/pflag"
	"golang.org/x/text/encoding/charmap"
	"text/template"
)

var version = "dev"

type app struct {
	csvPath      string
	templatePath string
	outPath      string
	counter      string
	noHeader     bool
	force        bool
}

func printHelp() {
	// get the default error output
	var out = pflag.CommandLine.Output()
	// write the help message
	fmt.Fprintf(out, "csvplate (version: %s): a CSV templated file generator\n\n", version)
	fmt.Fprintf(out, "Usage: csvplate [options]\n\n")
	fmt.Fprintf(out, "Options:\n")
	pflag.PrintDefaults()
	fmt.Fprintf(out, "Examples:\n")
	fmt.Fprintf(out, "  csvplate --csv data.csv --template template.txt --out output.txt\n")
	fmt.Fprintf(out, "  csvplate -f -i data.csv -t template.txt -o output_{{.Name}}.txt\n")
}

func newApp() *app {
	csvPath := pflag.StringP("csv", "i", "", "Path to input CSV file")
	templatePath := pflag.StringP("template", "t", "", "Path to Go template file")
	outPath := pflag.StringP("out", "o", "", "Output file path (may include template expressions)")
	counter := pflag.StringP("counter", "c", "_index_", "The field name to use for the row counter")
	noHeader := pflag.BoolP("noheader", "n", false, "Treat CSV as having no header row")
	force := pflag.BoolP("force", "f", false, "Overwrite existing output files")
	// keep the flags order
	pflag.CommandLine.SortFlags = false
	// in case of error do not display second time
	pflag.CommandLine.Init("latex-fast-compile", pflag.ContinueOnError)
	// The help message
	pflag.Usage = printHelp
	// Parse the flags
	err := pflag.CommandLine.Parse(os.Args[1:])
	if err != nil {
		if err == pflag.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "csvplate:", err)
		os.Exit(1)
	}

	return &app{
		csvPath:      *csvPath,
		templatePath: *templatePath,
		outPath:      *outPath,
		counter:      *counter,
		noHeader:     *noHeader,
		force:        *force,
	}
}

func main() {
	a := newApp()
	if err := a.run(); err != nil {
		fmt.Fprintln(os.Stderr, "csvplate:", err)
		os.Exit(1)
	}
}

func (a *app) run() error {
	if a.csvPath == "" || a.templatePath == "" || a.outPath == "" {
		return errors.New("flags -csv, -template, and -out are required")
	}

	rows, err := a.loadCSV()
	if err != nil {
		return err
	}

	tmpl, err := parseTemplate("content", a.templatePath)
	if err != nil {
		return err
	}

	if strings.Contains(a.outPath, "{{") {
		nameTmpl, err := template.New("outfile").Funcs(sprig.FuncMap()).Parse(a.outPath)
		if err != nil {
			return fmt.Errorf("parse output template: %w", err)
		}
		return writePerRow(nameTmpl, tmpl, rows, a.force)
	}

	return writeSingle(a.outPath, tmpl, rows, a.force)
}

func (a *app) loadCSV() ([]map[string]string, error) {
	f, err := os.Open(a.csvPath)
	if err != nil {
		return nil, fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(charmap.Windows1252.NewDecoder().Reader(f))
	data, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("csv is empty")
	}

	var headers []string
	start := 0
	if a.noHeader {
		count := len(data[0])
		headers = make([]string, count)
		for i := range headers {
			headers[i] = fmt.Sprintf("C%d", i+1)
		}
	} else {
		headers = data[0]
		start = 1
	}

	result := make([]map[string]string, 0, len(data)-start)
	for c, row := range data[start:] {
		if len(row) == 0 {
			continue
		}
		entry := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(row) {
				entry[header] = row[i]
			} else {
				entry[header] = ""
			}
		}
		entry[a.counter] = fmt.Sprintf("%d", c+1)
		result = append(result, entry)
	}
	return result, nil
}

func parseTemplate(name, path string) (*template.Template, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	tmpl, err := template.New(name).Funcs(sprig.FuncMap()).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return tmpl, nil
}

func writeSingle(outPath string, tmpl *template.Template, rows []map[string]string, force bool) error {
	outDir := filepath.Dir(outPath)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}

	if !force {
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("output file %s already exists (use -force to overwrite)", outPath)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect output file %s: %w", outPath, err)
		}
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, rows); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	fmt.Printf("%s\n", outPath)
	return nil
}

func writePerRow(nameTmpl, contentTmpl *template.Template, rows []map[string]string, force bool) error {
	if len(rows) == 0 {
		return nil
	}

	var numErrors int
	for idx, row := range rows {
		var nameBuilder strings.Builder
		if err := nameTmpl.Execute(&nameBuilder, row); err != nil {
			return fmt.Errorf("render output name for row %d: %w", idx, err)
		}
		outName := nameBuilder.String()
		if outName == "" {
			return fmt.Errorf("rendered output name for row %d is empty", idx)
		}

		if !force {
			if _, statErr := os.Stat(outName); statErr == nil {
				errExists := fmt.Errorf("output file %s already exists (use -force to overwrite)", outName)
				fmt.Fprintln(os.Stderr, errExists)
				numErrors++
				continue
			} else if statErr != nil && !os.IsNotExist(statErr) {
				return fmt.Errorf("inspect output file %s: %w", outName, statErr)
			}
		}

		if err := os.MkdirAll(filepath.Dir(outName), 0o755); err != nil {
			return fmt.Errorf("create directories for %s: %w", outName, err)
		}

		f, err := os.Create(outName)
		if err != nil {
			return fmt.Errorf("create output file %s: %w", outName, err)
		}
		defer f.Close()

		if err := contentTmpl.Execute(f, row); err != nil {
			return fmt.Errorf("render template for %s: %w", outName, err)
		}
		fmt.Printf("%s\n", outName)
	}
	if numErrors > 0 {
		return fmt.Errorf("%d files not overwritten.", numErrors)
	}
	return nil
}
