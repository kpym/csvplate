# csvplate

csvplate is a command-line utility that turns rows from a CSV file into rendered Go templates. You can generate a single aggregated output or one file per row.

## Usage

```text
> csvplate -h
csvplate (version: --): a CSV templated file generator

Usage: csvplate [options]
Options:
  -i, --csv string        Path to input CSV file, or the CSV content itself
  -t, --template string   Path to Go template file, or the template content itself
  -o, --out string        Output file path (may include template expressions)
  -c, --counter string    The field name to use for the row counter (default "_index_")
  -n, --noheader          Treat CSV as having no header row
  -f, --force             Overwrite existing output files

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
  cat data.csv | csvplate -n -t template.txt
```

## Template data model

- Each CSV row becomes a `map[string]string` keyed by column headers (or `C1`, `C2`, ... when `--noheader` is used).
- The special key defined by `--counter` provides a 1-based row index as a string.
- For single-output mode, the template receives a slice of those maps. In per-row mode the template receives the map for the current row.
- All [sprout](https://docs.atom.codes/sprout/registries/list-of-all-registries) template functions are available.

## Examples

Render a single file containing all rows:

```shell
csvplate -i sample.csv -t all_rows.tmpl -o output/all.txt -f
```

Render one file per CSV row using a dynamic file name:

```shell
csvplate -i sample.csv -t per_row.tmpl -o "output/{{ .Name }}.txt" -f
```

You can check the `example/` folder to see the provided examples and templates.

## Installation

### Precompiled executables

You can download the executable for your platform from the [releases](https://github.com/kpym/csvplate/releases).

### Compile it yourself

```shell
go install github.com/kpym/csvplate@latest
```

## License

Released under the [MIT License](LICENSE).
