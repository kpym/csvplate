# csvplate

csvplate is a command-line utility that turns rows from a CSV file into rendered Go templates. You can generate a single aggregated output or one file per row.

## Usage

```text
> csvplate -h
csvplate (version: --): a CSV templated file generator

Usage: csvplate [options]

Options:
  -i, --csv string        Path to input CSV file
  -t, --template string   Path to Go template file
  -o, --out string        Output file path (may include template expressions)
  -c, --counter string    The field name to use for the row counter (default "_index_") 
  -n, --noheader          Treat CSV as having no header row
  -f, --force             Overwrite existing output files
Examples:
  csvplate --csv data.csv --template template.txt --out output.txt
  csvplate -f -i data.csv -t template.txt -o output_{{.Name}}.txt
```

When `--out` contains a template expression (e.g. `output/{{ .Name }}.txt`), csvplate renders the template once per row. Otherwise, it renders the content template once, passing the entire slice of row maps to the template.

## Template data model

- Each CSV row becomes a `map[string]string` keyed by column headers (or `C1`, `C2`, ... when `--noheader` is used).
- The special key defined by `--counter` provides a 1-based row index as a string.
- For single-output mode, the template receives a slice of those maps. In per-row mode the template receives the map for the current row.
- All [sprig](https://masterminds.github.io/sprig/) template functions are available.

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
