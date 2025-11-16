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
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode/utf8"

	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/group/all"
	"github.com/kpym/utf8reader"
	"github.com/spf13/pflag"
)

var version = "dev"

type app struct {
	csvPath      string
	templatePath string
	outPath      string
	counter      string
	noHeader     bool
	force        bool
	csvSep       rune
}

var prehelp = `csvplate (version: ` + version + `): a CSV templated file generator

Usage: csvplate [options]
Options:
`
var posthelp = `
Mode of operation:
  If the output file name contains template expressions ({{...}}), one file per row
  will be created, else a single file will be created with all rows.
  In single file mode, the dot (.) in the template is a slice of objects (one per row).
  In per-row mode, the dot (.) in the template is a single object (the current row).
  The first line of the CSV is assumed to be the header line and will be used as field names,
  except if the --noheader flag is set in which case the fields will be named C1, C2, ...
  The field name specified with --counter will contain the row number (starting at 1).
  If --csv or --template is omitted or empty, stdin is used.
  If --out is omitted or empty, stdout is used in single file mode.
  If the output file already exists, an error is returned unless --force is set.
  If --csv or --template is not an existing file, it is treated as the actual content.
  The template functions from Sprout are available in the templates.

Examples:
  csvplate --csv data.csv --template template.txt --out output.txt
  csvplate -f -i data.csv -t template.txt -o output_{{.Name}}.txt
  csvplate -i data.csv --csv-sep ';' -t template.txt
  cat data.csv | csvplate -n -t template.txt
`

// printHelp prints the help message to the default output.
func printHelp() {
	// get the default error output
	var out = pflag.CommandLine.Output()
	// write the help message
	fmt.Fprint(out, prehelp)
	pflag.PrintDefaults()
	fmt.Fprint(out, posthelp)
}

// newApp creates a new app instance using the command line arguments.
func newApp() *app {
	csvPath := pflag.StringP("csv", "i", "", "Path to input CSV file, or the CSV content itself")
	templatePath := pflag.StringP("template", "t", "", "Path to Go template file, or the template content itself")
	outPath := pflag.StringP("out", "o", "", "Output file path (may include template expressions)")
	counter := pflag.StringP("counter", "c", "_index_", "The field name to use for the row counter")
	noHeader := pflag.BoolP("noheader", "n", false, "Treat CSV as having no header row")
	force := pflag.BoolP("force", "f", false, "Overwrite existing output files")
	csvSep := pflag.String("csv-sep", ",", "CSV field separator")
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

	sep, size := utf8.DecodeRuneInString(*csvSep)
	if size == 0 || size != len(*csvSep) {
		fmt.Fprintln(os.Stderr, "csvplate: --csv-sep must be a single UTF-8 character")
		os.Exit(1)
	}

	return &app{
		csvPath:      *csvPath,
		templatePath: *templatePath,
		outPath:      *outPath,
		counter:      *counter,
		noHeader:     *noHeader,
		force:        *force,
		csvSep:       sep,
	}
}

// run executes the application logic.
// if the output path contains template expressions, one file per row is created,
// else a single file is created.
func (a *app) run() error {
	if a.csvPath == "" && a.templatePath == "" {
		return errors.New("one of --csv or --template is required")
	}
	if a.csvPath == "" {
		a.csvPath = "-"
	}
	if a.templatePath == "" {
		a.templatePath = "-"
	}
	if a.outPath == "" {
		a.outPath = "-"
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

// content reads the content from the given file.
// If the file name is "-", stdin is used.
// If the file does not exist, the file name is treated as the actual content.
// The file encoding is guessed and converted to UTF-8 if needed.
func content(fileName string) (string, error) {
	var f io.Reader
	if fileName == "-" {
		// Read from stdin
		f = os.Stdin
	} else {
		ff, err := os.Open(fileName)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// fileName is containing the actual data
				// read from string
				f = strings.NewReader(fileName)
			} else {
				return "", fmt.Errorf("open file: %w", err)
			}
		} else {
			defer ff.Close()
			f = ff
		}
	}
	content, err := io.ReadAll(utf8reader.New(f))
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}
	return string(content), nil
}

// loadCSV reads the CSV file and returns a slice of maps representing the rows.
func (a *app) loadCSV() ([]map[string]string, error) {
	// Open the CSV file
	csvContent, err := content(a.csvPath)
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	reader := csv.NewReader(strings.NewReader(csvContent))
	reader.Comma = a.csvSep
	// Read all data
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
	// Read the template file
	tmplContent, err := content(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	// Parse the template
	tmpl, err := template.New("content").Funcs(funcs).Parse(tmplContent)
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

// writer creates a writer for the given file name.
// If the file name is "-", stdout is used.
// If force is false and the file exists, an error is returned.
// All necessary directories are created.
// The resulting io.WriteCloser is used to write the output.
func writer(fileName string, force bool) (io.WriteCloser, error) {
	if fileName == "-" {
		// Write to stdout
		return os.Stdout, nil
	}
	// Create output directories (if needed)
	outDir := filepath.Dir(fileName)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create directories: %w", err)
	}
	// Check if file exists
	if !force {
		if _, statErr := os.Stat(fileName); statErr == nil {
			return nil, fmt.Errorf("output file %s already exists (use -force to overwrite)", fileName)
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("inspect output file %s: %w", fileName, statErr)
		}
	}
	// Create the output file
	f, err := os.Create(fileName)
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}
	return f, nil
}

// writeSingle creates a single output file from the template and all rows.
func writeSingle(outPath string, tmpl *template.Template, rows []map[string]string, force bool) error {
	// Get the file writer
	f, err := writer(outPath, force)
	if err != nil {
		return err
	}
	defer f.Close()
	// Render the template
	if err := tmpl.Execute(f, rows); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	if outPath != "-" {
		fmt.Printf("result saved in %s\n", outPath)
	}
	return nil
}

// writePerRow creates one output file per row using the name and content templates.
func writePerRow(nameTmpl, contentTmpl *template.Template, rows []map[string]string, force bool) error {
	if len(rows) == 0 {
		return nil
	}

	fmt.Println("results saved in:")
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
		// Get the file writer
		f, err := writer(outName, force)
		if err != nil {
			numErrors++
			fmt.Fprintf(os.Stderr, "  %s: %v\n", outName, err)
			continue
		} else {
			defer f.Close()
		}
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

// get the params into new app and run it
func main() {
	a := newApp()
	if err := a.run(); err != nil {
		fmt.Fprintln(os.Stderr, "csvplate:", err)
		os.Exit(1)
	}
}
