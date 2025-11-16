#!/bin/sh

echo "Create one file per row (comma separator):"
go run .. --force --csv sample.csv --template per_row.tmpl --out "output/{{ .name }}.txt"

echo "Create single output file (comma separator):"
go run .. -f -i sample.csv -t all_rows.tmpl -o "output/all.txt"

echo "Create single output file (semicolon separator):"
go run .. -f --csv french.csv --csv-sep ';' --template all_rows.tmpl --out "output/fr_all.txt"

echo "Output from piped template (comma separator):"
cat all_rows.tmpl | go run .. -f -i sample.csv

echo "Output from piped csv (semicolon separator):"
cat french.csv | go run .. -f --csv-sep ';' -t all_rows.tmpl