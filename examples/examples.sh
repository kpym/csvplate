#!/bin/sh

echo "Create one file per row:"
go run .. --force --csv sample.csv --template per_row.tmpl --out "output/{{ .name }}.txt"

echo "Create single output file:"
go run .. -f -i sample.csv -t all_rows.tmpl -o "output/all.txt"

echo "Output from piped template:"
cat all_rows.tmpl | go run .. -f -i sample.csv

echo "Output from piped csv:"
cat sample.csv | go run .. -f -t all_rows.tmpl