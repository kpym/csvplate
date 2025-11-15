// csvplate is a small utilisty that takes a csvfile and a golang template
// and generates one or multimple output files based on the template.
// If the output file name contains a template expression, it will be evaluated
// for every row and new files will be created, else a single file will be created.
// If single file is created the . is a slice of objects, else the . is a single object.
// The first line is assumed to be the header line and will be used as the field names, except
// if the -noheader flag is set in which case the fields will be named C1, C2, ...
// The template functions from Sprout are available in the templates.
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

	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/group/all"
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
	// if no args, print help
	if len(os.Args) == 1 {
		printHelp()
		os.Exit(0)
	}
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
		return errors.New("flags --csv, --template, and --out are required")
	}

	// Get the sprout functions to use in the templates
	funcs, err := sproutFuncMap()
	if err != nil {
		return err
	}

	// Load the CSV data
	rows, err := a.loadCSV()
	if err != nil {
		return err
	}

	// Parse the content template
	contentTmpl, err := parseTemplate(a.templatePath, funcs)
	if err != nil {
		return err
	}

	// Create one file per row if output path is a template
	if strings.Contains(a.outPath, "{{") {
		nameTmpl, err := template.New("outfile").Funcs(funcs).Parse(a.outPath)
		if err != nil {
			return fmt.Errorf("parse output template: %w", err)
		}
		return writePerRow(nameTmpl, contentTmpl, rows, a.force)
	}
	// Else create a single file
	return writeSingle(a.outPath, contentTmpl, rows, a.force)
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

	// Determine headers : either from first row or generate C1, C2, ...
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

	// Build the result slice of maps
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
		// Add the counter field
		entry[a.counter] = fmt.Sprintf("%d", c+1)

		result = append(result, entry)
	}
	return result, nil
}

// parseTemplate reads and parses a template file with the given functions.
func parseTemplate(path string, funcs template.FuncMap) (*template.Template, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	tmpl, err := template.New("content").Funcs(funcs).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return tmpl, nil
}

// sproutFuncMap creates a template.FuncMap with all sprout functions registered.
func sproutFuncMap() (template.FuncMap, error) {
	handler := sprout.New()
	if err := handler.AddGroups(all.RegistryGroup()); err != nil {
		return nil, fmt.Errorf("register sprout functions: %w", err)
	}
	return handler.Build(), nil
}

// writeSingle creates a single output file from the template and all rows.
func writeSingle(outPath string, tmpl *template.Template, rows []map[string]string, force bool) error {
	// Create output directories (if needed)
	outDir := filepath.Dir(outPath)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create directories: %w", err)
	}
	// Check if file exists
	if !force {
		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("output file %s already exists (use -force to overwrite)", outPath)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect output file %s: %w", outPath, err)
		}
	}
	// Create the output file
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()
	// Render the template
	if err := tmpl.Execute(f, rows); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	fmt.Printf("%s\n", outPath)
	return nil
}

// writePerRow creates one output file per row using the name and content templates.
func writePerRow(nameTmpl, contentTmpl *template.Template, rows []map[string]string, force bool) error {
	if len(rows) == 0 {
		return nil
	}

	var numErrors int
	var nameBuilder strings.Builder
	for idx, row := range rows {
		// Generate the output file name
		if err := nameTmpl.Execute(&nameBuilder, row); err != nil {
			return fmt.Errorf("render output name for row %d: %w", idx, err)
		}
		outName := nameBuilder.String()
		nameBuilder.Reset()
		if outName == "" {
			return fmt.Errorf("rendered output name for row %d is empty", idx)
		}
		// Check if file exists
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
		// Create output directories (if needed)
		if err := os.MkdirAll(filepath.Dir(outName), 0o755); err != nil {
			return fmt.Errorf("create directories for %s: %w", outName, err)
		}
		// Create the output file
		f, err := os.Create(outName)
		if err != nil {
			return fmt.Errorf("create output file %s: %w", outName, err)
		}
		defer f.Close()
		// Render the content template
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
